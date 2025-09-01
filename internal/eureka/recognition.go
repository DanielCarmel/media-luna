package eureka

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/media-luna/eureka/internal/database/mysql"
	fingerprint "github.com/media-luna/eureka/internal/fingerprint"
	"github.com/media-luna/eureka/utils/logger"
)

// Match represents a potential song match
type Match struct {
	SongID    int
	SongName  string
	Artist    string
	Score     float64
	Offset    int
	Timestamp float64
}

// Recognize processes an audio sample and tries to find matches in the database
func (e *Eureka) Recognize(audioPath string) ([]Match, error) {
	logger.Info(fmt.Sprintf("Recognizing audio file: %s", audioPath))

	// Convert audio to WAV
	filePath, err := fingerprint.ConvertToWAV(audioPath, "recognize_output.wav")
	if err != nil {
		return nil, fmt.Errorf("error converting to WAV: %v", err)
	}

	// Read wav info
	wavInfo, err := fingerprint.ReadWavInfo(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading WAV info: %v", err)
	}

	logger.Info(fmt.Sprintf("Original audio: %d samples at %d Hz (%.2f seconds)", len(wavInfo.Samples), wavInfo.SampleRate, float64(len(wavInfo.Samples))/float64(wavInfo.SampleRate)))

	// For recognition, only use first 30 seconds to avoid too many fingerprints
	maxSamples := wavInfo.SampleRate * 30 // 30 seconds
	originalLength := len(wavInfo.Samples)
	if originalLength > maxSamples {
		wavInfo.Samples = wavInfo.Samples[:maxSamples]
		logger.Info(fmt.Sprintf("Limited audio from %d to %d samples (first 30 seconds for recognition)", originalLength, len(wavInfo.Samples)))
	}

	logger.Info("Generating spectrogram for recognition...")
	// Generate spectrogram
	spectrogram, err := fingerprint.SamplesToSpectrogram(wavInfo.Samples, wavInfo.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("error creating spectrogram: %v", err)
	}

	// Extract peaks
	peaks := fingerprint.PickPeaks(spectrogram, wavInfo.SampleRate)
	logger.Info(fmt.Sprintf("Found %d peaks for recognition", len(peaks)))

	// Generate fingerprints
	fingerprints := fingerprint.GenerateFingerprints(peaks)
	logger.Info(fmt.Sprintf("Generated %d fingerprints for recognition", len(fingerprints)))

	if len(fingerprints) == 0 {
		return []Match{}, nil
	}

	// Create a map of sample fingerprints with their time offsets
	sampleFingerprintMap := make(map[string]int)
	for _, fp := range fingerprints {
		sampleFingerprintMap[fp.Hash] = fp.Offset
	}

	// Query database for matching fingerprints
	matches, err := e.findMatches(sampleFingerprintMap, false)
	if err != nil {
		return nil, fmt.Errorf("error finding matches: %v", err)
	}

	return matches, nil
}

// findMatches searches for fingerprint matches in the database and scores them
func (e *Eureka) findMatches(sampleFingerprints map[string]int, isFromMicrophone bool) ([]Match, error) {
	// Get all sample fingerprint hashes
	hashes := make([]string, 0, len(sampleFingerprints))
	for hash := range sampleFingerprints {
		hashes = append(hashes, hash)
	}

	if len(hashes) == 0 {
		return []Match{}, nil
	}

	logger.Info(fmt.Sprintf("Starting fingerprint matching with %d hashes", len(hashes)))

	// Process in batches to avoid MySQL placeholder limit
	const maxBatchSize = 1000 // Very conservative limit
	var allDbMatches []mysql.FingerprintMatch

	logger.Info(fmt.Sprintf("Will process in %d batches of max %d hashes each", (len(hashes)+maxBatchSize-1)/maxBatchSize, maxBatchSize))

	for i := 0; i < len(hashes); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}

		batchHashes := hashes[i:end]
		logger.Info(fmt.Sprintf("Processing batch %d: %d hashes", (i/maxBatchSize)+1, len(batchHashes)))
		dbMatches, err := e.database.QueryFingerprints(batchHashes)
		if err != nil {
			return nil, err
		}

		// Add all matches to our collection
		allDbMatches = append(allDbMatches, dbMatches...)
	}

	if len(allDbMatches) == 0 {
		logger.Info("No matches found in database")
		return []Match{}, nil
	}

	// Group matches by song and calculate relative timing
	songMatches := make(map[int][]TimeMatch)
	for _, dbMatch := range allDbMatches {
		sampleOffset := sampleFingerprints[dbMatch.Hash]
		timeDiff := dbMatch.Offset - sampleOffset

		songMatches[dbMatch.SongID] = append(songMatches[dbMatch.SongID], TimeMatch{
			SampleTime: sampleOffset,
			DbTime:     dbMatch.Offset,
			TimeDiff:   timeDiff,
		})
	}

	// Score each song based on temporal alignment
	var matches []Match
	minMatches := 5 // Default minimum for file recognition
	if isFromMicrophone {
		minMatches = 3 // Lower threshold for microphone (more tolerant)
	}

	for songID, timeMatches := range songMatches {
		if len(timeMatches) < minMatches {
			continue
		}

		score := calculateTemporalScore(timeMatches, isFromMicrophone)
		scoreThreshold := 0.1
		if isFromMicrophone {
			scoreThreshold = 0.05 // Lower threshold for microphone
		}

		if score > scoreThreshold {
			// Get song info
			songInfo, err := e.database.GetSongByID(songID)
			if err != nil {
				logger.Info(fmt.Sprintf("Error getting song info for ID %d: %v", songID, err))
				continue
			}

			matches = append(matches, Match{
				SongID:   songID,
				SongName: songInfo.Name,
				Artist:   songInfo.Artist,
				Score:    score,
				Offset:   findMostCommonTimeDiff(timeMatches),
			})
		}
	}

	// Sort matches by score (highest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Return top matches
	maxResults := 5
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	return matches, nil
}

// TimeMatch represents a time alignment between sample and database
type TimeMatch struct {
	SampleTime int
	DbTime     int
	TimeDiff   int
}

// calculateTemporalScore calculates a score based on temporal alignment of matches
func calculateTemporalScore(timeMatches []TimeMatch, isFromMicrophone bool) float64 {
	minMatches := 5
	if isFromMicrophone {
		minMatches = 3 // More tolerant for microphone
	}

	if len(timeMatches) < minMatches {
		return 0.0
	}

	// Group by time difference (temporal alignment)
	timeDiffCounts := make(map[int]int)
	for _, tm := range timeMatches {
		timeDiffCounts[tm.TimeDiff]++
	}

	// Find the most common time difference
	maxCount := 0
	for _, count := range timeDiffCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	// Score is based on:
	// 1. Number of aligned matches
	// 2. Ratio of aligned matches to total matches
	// 3. Temporal consistency
	alignedRatio := float64(maxCount) / float64(len(timeMatches))
	rawScore := float64(maxCount) * alignedRatio

	// More generous scoring for microphone
	normalizationFactor := 100.0
	if isFromMicrophone {
		normalizationFactor = 50.0 // More generous scoring
	}

	score := rawScore / normalizationFactor
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// findMostCommonTimeDiff finds the most frequent time difference
func findMostCommonTimeDiff(timeMatches []TimeMatch) int {
	timeDiffCounts := make(map[int]int)
	for _, tm := range timeMatches {
		timeDiffCounts[tm.TimeDiff]++
	}

	maxCount := 0
	bestTimeDiff := 0
	for timeDiff, count := range timeDiffCounts {
		if count > maxCount {
			maxCount = count
			bestTimeDiff = timeDiff
		}
	}

	return bestTimeDiff
}

// RecognizeFromMicrophone starts real-time recognition from microphone
// Works like Shazam: listens until a match is found or 30 seconds timeout
func (e *Eureka) RecognizeFromMicrophone() error {
	logger.Info("Starting microphone recognition...")

	// Create microphone recorder
	recorder, err := fingerprint.NewMicrophoneRecorder()
	if err != nil {
		return fmt.Errorf("failed to create microphone recorder: %v", err)
	}
	defer recorder.Cleanup()

	// Start recording
	err = recorder.StartRecording()
	if err != nil {
		return fmt.Errorf("failed to start recording: %v", err)
	}

	// Set up signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Set up 30-second timeout like Shazam
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	// Channel to receive match found signal
	matchFoundChan := make(chan Match, 1)

	logger.Info("🎤 Listening for audio... (30s timeout)")

	// Main recognition loop
	recognitionTicker := time.NewTicker(2 * time.Second) // Check every 2 seconds
	defer recognitionTicker.Stop()

	for {
		select {
		case <-signalChan:
			logger.Info("Received interrupt signal, stopping...")
			recorder.StopRecording()
			return nil

		case <-timeout.C:
			logger.Info("⏰ No match found within 30 seconds, stopping...")
			recorder.StopRecording()
			return nil

		case match := <-matchFoundChan:
			logger.Info(fmt.Sprintf("🎵 SONG FOUND: %s by %s (Score: %.3f)",
				match.SongName, match.Artist, match.Score))
			recorder.StopRecording()
			return nil

		case <-recognitionTicker.C:
			// Get current audio buffer
			audioBuffer := recorder.GetAudioBuffer()

			// Only process if we have enough audio (at least 3 seconds)
			minSamples := 44100 * 3 // 3 seconds at 44.1kHz
			if len(audioBuffer) >= minSamples {
				go e.processRealtimeAudioWithMatch(audioBuffer, matchFoundChan)
			}

		case result := <-recorder.GetResultChannel():
			if result.Error != nil {
				logger.Info(fmt.Sprintf("Recognition error: %v", result.Error))
			}
		}
	}
}

// processRealtimeAudioWithMatch processes audio buffer for real-time recognition with match detection
func (e *Eureka) processRealtimeAudioWithMatch(audioBuffer []float64, matchFoundChan chan<- Match) {
	defer func() {
		if r := recover(); r != nil {
			logger.Info(fmt.Sprintf("Recovery in processRealtimeAudioWithMatch: %v", r))
		}
	}()

	// Use last 5 seconds of audio for recognition
	sampleRate := 44100
	windowSamples := sampleRate * 5 // 5 seconds

	if len(audioBuffer) < windowSamples {
		logger.Info(fmt.Sprintf("🔧 Audio buffer too small: %d < %d", len(audioBuffer), windowSamples))
		return
	}

	// Take the most recent window
	audioWindow := audioBuffer[len(audioBuffer)-windowSamples:]

	// Calculate audio levels for debugging
	var maxLevel, avgLevel float64
	for _, sample := range audioWindow {
		absVal := sample
		if absVal < 0 {
			absVal = -absVal
		}
		if absVal > maxLevel {
			maxLevel = absVal
		}
		avgLevel += absVal
	}
	avgLevel /= float64(len(audioWindow))

	logger.Info(fmt.Sprintf("🎚️ Audio levels - Max: %.4f, Avg: %.4f", maxLevel, avgLevel))

	// Generate spectrogram
	spectrogram, err := fingerprint.SamplesToSpectrogram(audioWindow, sampleRate)
	if err != nil {
		logger.Info(fmt.Sprintf("Spectrogram generation failed: %v", err))
		return
	}

	// Extract peaks
	peaks := fingerprint.PickPeaks(spectrogram, sampleRate)
	logger.Info(fmt.Sprintf("🎯 Found %d peaks from audio", len(peaks)))
	if len(peaks) < 20 { // Lowered from 50 to be more tolerant
		logger.Info("❌ Not enough peaks for reliable recognition (need 20+)")
		return
	}

	// Generate fingerprints with microphone tolerance
	fingerprints := fingerprint.GenerateFingerprintsForMicrophone(peaks)
	logger.Info(fmt.Sprintf("🔑 Generated %d fingerprints (with microphone tolerance)", len(fingerprints)))
	if len(fingerprints) < 50 { // Lowered from 100 to be more tolerant
		logger.Info("❌ Not enough fingerprints for reliable recognition (need 50+)")
		return
	}

	// Create fingerprint map
	sampleFingerprintMap := make(map[string]int)
	for _, fp := range fingerprints {
		sampleFingerprintMap[fp.Hash] = fp.Offset
	}

	// Try to find matches with microphone-specific parameters
	matches, err := e.findMatches(sampleFingerprintMap, true)
	if err != nil {
		logger.Info(fmt.Sprintf("Match finding failed: %v", err))
		return
	}

	logger.Info(fmt.Sprintf("📊 Found %d potential matches", len(matches)))

	// Check for high-confidence matches and send to channel if found
	for _, match := range matches {
		if match.Score > 0.3 { // Lower threshold for microphone recognition
			logger.Info(fmt.Sprintf("🎵 MATCH FOUND: %s by %s (Score: %.3f)",
				match.SongName, match.Artist, match.Score))

			// Send match to channel (non-blocking)
			select {
			case matchFoundChan <- match:
			default:
				// Channel is full, match already found
			}
			return
		} else if match.Score > 0.1 { // Lower threshold for progress indication
			logger.Info(fmt.Sprintf("🔍 Possible match: %s by %s (Score: %.3f)",
				match.SongName, match.Artist, match.Score))
		} else if match.Score > 0.05 { // Even lower threshold to see any activity
			logger.Info(fmt.Sprintf("🔍 Weak match: %s by %s (Score: %.3f)",
				match.SongName, match.Artist, match.Score))
		}
	}

	if len(matches) == 0 {
		logger.Info("❌ No matches found in database")
	}
}
