package fingerprint

import (
	"fmt"

	"github.com/gordonklaus/portaudio"
)

const (
	SAMPLE_RATE        = 44100
	FRAMES_PER_BUFFER  = 1024
	RECORDING_DURATION = 10 // seconds
	BUFFER_DURATION    = 5  // seconds for recognition window
)

// MicrophoneRecorder handles real-time audio recording from microphone
type MicrophoneRecorder struct {
	stream        *portaudio.Stream
	sampleRate    int
	bufferSize    int
	audioBuffer   []float32
	isRecording   bool
	stopChannel   chan bool
	resultChannel chan RecognitionResult
}

// RecognitionResult represents the result of a recognition attempt
type RecognitionResult struct {
	SongName string
	Artist   string
	Score    float64
	Offset   int
	Found    bool
	Error    error
}

// NewMicrophoneRecorder creates a new microphone recorder instance
func NewMicrophoneRecorder() (*MicrophoneRecorder, error) {
	err := portaudio.Initialize()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PortAudio: %v", err)
	}

	return &MicrophoneRecorder{
		sampleRate:    SAMPLE_RATE,
		bufferSize:    FRAMES_PER_BUFFER,
		audioBuffer:   make([]float32, 0),
		isRecording:   false,
		stopChannel:   make(chan bool),
		resultChannel: make(chan RecognitionResult, 10),
	}, nil
}

// StartRecording begins continuous microphone recording
func (mr *MicrophoneRecorder) StartRecording() error {
	if mr.isRecording {
		return fmt.Errorf("recording is already in progress")
	}

	// Get default input device
	defaultInputDevice, err := portaudio.DefaultInputDevice()
	if err != nil {
		return fmt.Errorf("failed to get default input device: %v", err)
	}

	// Create input parameters
	inputParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   defaultInputDevice,
			Channels: 1, // Mono recording
			Latency:  defaultInputDevice.DefaultLowInputLatency,
		},
		SampleRate:      float64(mr.sampleRate),
		FramesPerBuffer: mr.bufferSize,
	}

	// Open stream
	stream, err := portaudio.OpenStream(inputParams, mr.audioCallback)
	if err != nil {
		return fmt.Errorf("failed to open audio stream: %v", err)
	}

	mr.stream = stream
	mr.isRecording = true

	// Start the stream
	err = mr.stream.Start()
	if err != nil {
		return fmt.Errorf("failed to start audio stream: %v", err)
	}

	fmt.Println("🎤 Recording started... Press Ctrl+C to stop")
	return nil
}

// audioCallback processes incoming audio data
func (mr *MicrophoneRecorder) audioCallback(in []float32) {
	if len(in) == 0 {
		return
	}

	// Add incoming audio to buffer
	mr.audioBuffer = append(mr.audioBuffer, in...)

	// Keep buffer to a reasonable size (10 seconds max) to prevent memory issues
	maxSamples := mr.sampleRate * 10 // 10 seconds
	if len(mr.audioBuffer) > maxSamples {
		// Remove oldest audio, keep the most recent 10 seconds
		removeCount := len(mr.audioBuffer) - maxSamples
		copy(mr.audioBuffer, mr.audioBuffer[removeCount:])
		mr.audioBuffer = mr.audioBuffer[:maxSamples]
	}

	// Check if we have enough audio for internal recognition processing (BUFFER_DURATION seconds)
	requiredSamples := mr.sampleRate * BUFFER_DURATION
	if len(mr.audioBuffer) >= requiredSamples {
		// Extract audio segment for recognition
		audioSegment := make([]float32, requiredSamples)
		copy(audioSegment, mr.audioBuffer[len(mr.audioBuffer)-requiredSamples:]) // Take the last 5 seconds

		// Convert float32 to float64 for processing
		audioFloat64 := make([]float64, len(audioSegment))
		for i, sample := range audioSegment {
			audioFloat64[i] = float64(sample)
		}

		// Process this audio segment in a goroutine (but don't trim the main buffer)
		go mr.processAudioSegment(audioFloat64)
	}
}

// processAudioSegment processes an audio segment for recognition
func (mr *MicrophoneRecorder) processAudioSegment(audioData []float64) {
	defer func() {
		if r := recover(); r != nil {
			mr.resultChannel <- RecognitionResult{
				Found: false,
				Error: fmt.Errorf("recognition panic: %v", r),
			}
		}
	}()

	// Generate spectrogram
	spectrogram, err := SamplesToSpectrogram(audioData, mr.sampleRate)
	if err != nil {
		mr.resultChannel <- RecognitionResult{
			Found: false,
			Error: fmt.Errorf("spectrogram generation failed: %v", err),
		}
		return
	}

	// Extract peaks
	peaks := PickPeaks(spectrogram, mr.sampleRate)
	if len(peaks) < 10 {
		// Not enough peaks for reliable recognition
		return
	}

	// Generate fingerprints
	fingerprints := GenerateFingerprints(peaks)
	if len(fingerprints) < 50 {
		// Not enough fingerprints for reliable recognition
		return
	}

	// Create fingerprint map
	fingerprintMap := make(map[string]int)
	for _, fp := range fingerprints {
		fingerprintMap[fp.Hash] = fp.Offset
	}

	// Send for recognition (this would need to be connected to your eureka instance)
	// For now, we'll just indicate that processing is complete
	mr.resultChannel <- RecognitionResult{
		Found: false,
		Error: nil,
	}
}

// StopRecording stops the microphone recording
func (mr *MicrophoneRecorder) StopRecording() error {
	if !mr.isRecording {
		return fmt.Errorf("no recording in progress")
	}

	// Signal stop
	mr.stopChannel <- true
	mr.isRecording = false

	// Stop and close stream
	if mr.stream != nil {
		err := mr.stream.Stop()
		if err != nil {
			return fmt.Errorf("failed to stop stream: %v", err)
		}

		err = mr.stream.Close()
		if err != nil {
			return fmt.Errorf("failed to close stream: %v", err)
		}
	}

	fmt.Println("🛑 Recording stopped")
	return nil
}

// GetResultChannel returns the channel for recognition results
func (mr *MicrophoneRecorder) GetResultChannel() <-chan RecognitionResult {
	return mr.resultChannel
}

// Cleanup cleans up PortAudio resources
func (mr *MicrophoneRecorder) Cleanup() error {
	if mr.isRecording {
		mr.StopRecording()
	}
	return portaudio.Terminate()
}

// GetAudioBuffer returns a copy of the current audio buffer for external processing
func (mr *MicrophoneRecorder) GetAudioBuffer() []float64 {
	// Ensure we have at least 5 seconds of audio for recognition
	requiredSamples := mr.sampleRate * 5 // 5 seconds

	// If buffer is smaller than required, return what we have
	if len(mr.audioBuffer) < requiredSamples {
		buffer := make([]float64, len(mr.audioBuffer))
		for i, sample := range mr.audioBuffer {
			buffer[i] = float64(sample)
		}
		return buffer
	}

	// Return the last 5 seconds of audio for recognition
	startIdx := len(mr.audioBuffer) - requiredSamples
	buffer := make([]float64, requiredSamples)
	for i, sample := range mr.audioBuffer[startIdx:] {
		buffer[i] = float64(sample)
	}
	return buffer
}

// IsRecording returns true if currently recording
func (mr *MicrophoneRecorder) IsRecording() bool {
	return mr.isRecording
}
