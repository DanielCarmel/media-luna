package fingerprint

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"math/cmplx"
	"os"
)

const (
	PEAK_THRESHOLD         = 0.02 // Lowered further for microphone audio - samples will be considered as peak when reaching this value
	MIN_HASH_TIME_DELTA    = 0    // Min milliseconds between 2 peaks to considered fingerprint
	MAX_HASH_TIME_DELTA    = 2000 // Max milliseconds between 2 peaks to considered fingerprint
	FAN_VALUE              = 15   // Size of the target zone for peak pairing in the fingerprinting process
	WINDOW_SIZE            = 4096 // Size of the window used for the STFT (power of 2)
	DOWNSAMPLE_RATIO       = 1    // Downsampling ratio for the audio samples(devide the amount of samples by N)
	MIN_WAV_BYTES          = 44   // Minimum number of bytes required for a valid WAV file
	HEADER_BITS_PER_SAMPLE = 16   // Number of bits per sample in the WAV file header

	// Frequency bands for peak detection (in Hz)
	FREQ_BANDS = 6

	// Maximum peaks per time frame
	MAX_PEAKS_PER_FRAME = 3
)

// Fingerprint represents a single audio fingerprint
type Fingerprint struct {
	Hash   string
	SongID int
	Offset int
}

// Fingerprints
type Fingerprints struct {
	TimeMs float64
	Hash   string
}

type Peak struct {
	Time      float64
	TimeMS    float64
	Magnitude float64
	FreqBin   int // Frequency bin index instead of complex frequency
}

// FrequencyBand represents a frequency band for peak detection
type FrequencyBand struct {
	Start int
	End   int
}

// getFrequencyBands returns frequency bands for peak detection
func getFrequencyBands(sampleRate, windowSize int) []FrequencyBand {
	nyquist := sampleRate / 2
	binSize := float64(nyquist) / float64(windowSize/2)

	bands := []FrequencyBand{
		{Start: int(40 / binSize), End: int(80 / binSize)},     // 40-80 Hz
		{Start: int(80 / binSize), End: int(120 / binSize)},    // 80-120 Hz
		{Start: int(120 / binSize), End: int(180 / binSize)},   // 120-180 Hz
		{Start: int(180 / binSize), End: int(300 / binSize)},   // 180-300 Hz
		{Start: int(300 / binSize), End: int(2000 / binSize)},  // 300-2000 Hz
		{Start: int(2000 / binSize), End: int(5000 / binSize)}, // 2000-5000 Hz
	}

	// Ensure bands don't exceed available frequency bins
	maxBin := windowSize/2 - 1
	for i := range bands {
		if bands[i].End > maxBin {
			bands[i].End = maxBin
		}
		if bands[i].Start > maxBin {
			bands[i].Start = maxBin
		}
	}

	return bands
}

// PickPeaks identifies and extracts peaks from a given spectrogram using Shazam's approach.
// It finds the strongest peaks in frequency bands rather than using a simple threshold.
//
// Parameters:
//   - spectrogram: A 2D slice of complex128 values representing the spectrogram data.
//   - sampleRate: The sample rate of the audio
//
// Returns:
//   - A slice of Peak structs, each representing a detected peak with its time and frequency bin.
func PickPeaks(spectrogram [][]complex128, sampleRate int) []Peak {
	if len(spectrogram) == 0 || len(spectrogram[0]) == 0 {
		return []Peak{}
	}

	hopSize := WINDOW_SIZE / 4 // Same as used in spectrogram generation
	magnitudes := getMagnitudes(spectrogram)
	var peaks []Peak

	// Get frequency bands for peak detection
	bands := getFrequencyBands(sampleRate, WINDOW_SIZE)

	// For each time frame
	for t, frame := range magnitudes {
		timeMS := float64(t) * float64(hopSize) / float64(sampleRate) * 1000

		// Find peaks in each frequency band
		for _, band := range bands {
			if band.Start >= len(frame) || band.End >= len(frame) {
				continue
			}

			// Find the maximum in this band
			maxMag := 0.0
			maxBin := -1

			for f := band.Start; f <= band.End; f++ {
				if frame[f] > maxMag && isLocalPeak(magnitudes, t, f) {
					maxMag = frame[f]
					maxBin = f
				}
			}

			// Add peak if it exceeds threshold
			if maxBin != -1 && maxMag > PEAK_THRESHOLD {
				peaks = append(peaks, Peak{
					Time:      float64(t),
					TimeMS:    timeMS,
					FreqBin:   maxBin,
					Magnitude: maxMag,
				})
			}
		}
	}

	return peaks
}

// getMagnitudes computes the magnitudes of a given 2D spectrogram.
// Each element in the spectrogram is a complex number, and the magnitude
// is calculated using the absolute value of the complex number.
//
// Parameters:
//
//	spectrogram [][]complex128 - A 2D slice of complex128 numbers representing the spectrogram.
//
// Returns:
//
//	[][]float64 - A 2D slice of float64 numbers representing the magnitudes of the spectrogram.
func getMagnitudes(spectrogram [][]complex128) [][]float64 {
	magnitudes := make([][]float64, len(spectrogram))
	for i, row := range spectrogram {
		magnitudes[i] = make([]float64, len(row))
		for j, val := range row {
			magnitudes[i][j] = cmplx.Abs(val)
		}
	}
	return magnitudes
}

// isLocalPeak determines if the magnitude at a given time-frequency point (t, f)
// is a local peak in the spectrogram. A local peak is defined as a point that has
// a higher magnitude than all of its immediate neighbors.
//
// Parameters:
// - magnitudes: A 2D slice of float64 representing the magnitudes in the spectrogram.
// - t: The time index of the point to check.
// - f: The frequency index of the point to check.
//
// Returns:
// - bool: True if the point (t, f) is a local peak, false otherwise.
func isLocalPeak(magnitudes [][]float64, t, f int) bool {
	deltaT := []int{-1, 0, 1}
	deltaF := []int{-1, 0, 1}

	peakValue := magnitudes[t][f]
	for _, dt := range deltaT {
		for _, df := range deltaF {
			if dt == 0 && df == 0 {
				continue
			}
			tt, ff := t+dt, f+df
			if tt >= 0 && tt < len(magnitudes) && ff >= 0 && ff < len(magnitudes[0]) {
				if peakValue <= magnitudes[tt][ff] {
					return false
				}
			}
		}
	}
	return true
}

// CalculateFileHash generates a SHA1 hash of the file contents
func CalculateFileHash(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}

	return hex.EncodeToString(h.Sum(nil))
}

// GenerateFingerprints generates fingerprints from spectrogram peaks using Shazam's constellation map approach
func GenerateFingerprints(peaks []Peak) []Fingerprint {
	return generateFingerprintsWithTolerance(peaks, false)
}

// GenerateFingerprintsForMicrophone generates fingerprints with tolerance for microphone audio
func GenerateFingerprintsForMicrophone(peaks []Peak) []Fingerprint {
	// For now, use the same algorithm but generate slightly more variations
	// Generate original fingerprints plus a small tolerance set
	baseFingerprints := generateFingerprintsWithTolerance(peaks, false)

	// For microphone audio, limit the tolerance to avoid MySQL issues
	// Only generate a subset with minimal tolerance
	toleranceFingerprints := generateFingerprintsWithMinimalTolerance(peaks)

	// Combine both sets
	allFingerprints := append(baseFingerprints, toleranceFingerprints...)
	return allFingerprints
}

// generateFingerprintsWithMinimalTolerance generates a small set of tolerance fingerprints
func generateFingerprintsWithMinimalTolerance(peaks []Peak) []Fingerprint {
	var fingerprints []Fingerprint

	// Only process every Nth peak to limit fingerprint count
	skipFactor := 4 // Only process every 4th peak pair for tolerance
	processed := 0

	// Fan out from each peak (anchor point)
	for i := 0; i < len(peaks); i += skipFactor {
		anchor := peaks[i]

		// Look at the next peaks within the target zone as target points
		for j := i + 1; j < i+FAN_VALUE && j < len(peaks); j++ {
			target := peaks[j]

			// Create hash using frequency bins and time delta
			timeDelta := target.TimeMS - anchor.TimeMS
			if timeDelta <= float64(MIN_HASH_TIME_DELTA) || timeDelta > float64(MAX_HASH_TIME_DELTA) {
				continue
			}

			// Generate only ±1 frequency bin tolerance (minimal set)
			tolerances := [][]int{
				{-1, 0}, // Anchor -1
				{1, 0},  // Anchor +1
				{0, -1}, // Target -1
				{0, 1},  // Target +1
			}

			for _, tol := range tolerances {
				anchorBin := anchor.FreqBin + tol[0]
				targetBin := target.FreqBin + tol[1]

				// Ensure bins are within reasonable range
				if anchorBin < 0 || targetBin < 0 || anchorBin > 2048 || targetBin > 2048 {
					continue
				}

				hashInput := fmt.Sprintf("%d|%d|%d",
					anchorBin,
					targetBin,
					int(timeDelta))

				hasher := sha1.New()
				hasher.Write([]byte(hashInput))
				hashBytes := hasher.Sum(nil)
				hashStr := hex.EncodeToString(hashBytes)

				fingerprints = append(fingerprints, Fingerprint{
					Hash:   hashStr,
					Offset: int(anchor.TimeMS),
				})

				processed++
				// Limit total tolerance fingerprints to avoid MySQL issues
				if processed > 10000 {
					return fingerprints
				}
			}
		}
	}

	return fingerprints
}

// generateFingerprintsWithTolerance generates fingerprints with optional frequency tolerance for microphone audio
func generateFingerprintsWithTolerance(peaks []Peak, microphoneTolerance bool) []Fingerprint {
	var fingerprints []Fingerprint

	// Fan out from each peak (anchor point)
	for i, anchor := range peaks {
		// Look at the next peaks within the target zone as target points
		for j := i + 1; j < i+FAN_VALUE && j < len(peaks); j++ {
			target := peaks[j]

			// Create hash using frequency bins and time delta
			timeDelta := target.TimeMS - anchor.TimeMS
			if timeDelta <= float64(MIN_HASH_TIME_DELTA) || timeDelta > float64(MAX_HASH_TIME_DELTA) {
				continue
			}

			// Always use original exact matching now
			hashInput := fmt.Sprintf("%d|%d|%d",
				anchor.FreqBin,
				target.FreqBin,
				int(timeDelta))

			hasher := sha1.New()
			hasher.Write([]byte(hashInput))
			hashBytes := hasher.Sum(nil)
			hashStr := hex.EncodeToString(hashBytes)

			fingerprints = append(fingerprints, Fingerprint{
				Hash:   hashStr,
				Offset: int(anchor.TimeMS),
			})
		}
	}

	return fingerprints
}
