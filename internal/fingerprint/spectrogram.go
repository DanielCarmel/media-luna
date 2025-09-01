package fingerprint

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/cmplx"
	"os"

	"github.com/maddyblue/go-dsp/fft"
	"github.com/maddyblue/go-dsp/window"
)

// Spectrogram computes the spectrogram of a WAV file using proper STFT.
func SamplesToSpectrogram(samples []float64, sampleRate int) ([][]complex128, error) {
	// Apply low-pass filter (optional)
	filteredSamples := lowPassFilter(samples, WINDOW_SIZE)

	// Downsample
	downsampledSamples, err := downsample(filteredSamples, sampleRate, sampleRate/DOWNSAMPLE_RATIO)
	if err != nil {
		return nil, err
	}

	// Compute STFT with proper overlapping windows
	hopSize := WINDOW_SIZE / 4 // 75% overlap as per Shazam paper
	spectrogram := [][]complex128{}

	// Create Hamming window once
	hammingWindow := window.Hamming(WINDOW_SIZE)

	for i := 0; i <= len(downsampledSamples)-WINDOW_SIZE; i += hopSize {
		// Extract frame
		frame := make([]float64, WINDOW_SIZE)
		copy(frame, downsampledSamples[i:i+WINDOW_SIZE])

		// Apply Hamming window to each frame
		for j := range frame {
			frame[j] *= hammingWindow[j]
		}

		// Compute FFT
		fftOut := fft.FFTReal(frame)

		// Only keep the first half (positive frequencies)
		halfSize := len(fftOut) / 2
		spectrogram = append(spectrogram, fftOut[:halfSize])
	}

	return spectrogram, nil
}

// GenerateSpectrogramImage generates a spectrogram image from the given spectrogram data.
func SpectrogramToImage(spectrogram [][]complex128, peaks []Peak, sampleRate int, path string) error {
	// Calculate dimensions
	numFrames := len(spectrogram)
	if numFrames == 0 {
		return fmt.Errorf("empty spectrogram")
	}
	numFreqs := len(spectrogram[0]) // Already using positive frequencies only
	imgWidth := numFrames
	imgHeight := numFreqs

	// Create image
	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Normalize magnitudes using RMS
	rms := calculateRMS(spectrogram)
	if rms == 0 {
		rms = 1 // Avoid division by zero
	}

	for x, frame := range spectrogram {
		for y := 0; y < numFreqs && y < len(frame); y++ {
			mag := cmplx.Abs(frame[y]) / rms
			// Use log scale for better visualization
			if mag > 0 {
				mag = math.Log10(1 + mag*9) // Log scale mapping
			}
			gray := uint8(math.Min(255, 255*mag))
			img.Set(x, imgHeight-y-1, color.RGBA{gray, gray, gray, 255}) // Invert y-axis
		}
	}

	// Draw peaks
	peakColor := color.RGBA{255, 0, 0, 255} // Red color for peaks
	for _, peak := range peaks {
		x := int(peak.Time)
		y := peak.FreqBin
		if x >= 0 && x < imgWidth && y >= 0 && y < imgHeight {
			img.Set(x, imgHeight-y-1, peakColor) // Invert y-axis
		}
	}

	// Save file
	f, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer f.Close()

	err = png.Encode(f, img)
	if err != nil {
		fmt.Println("Error encoding image:", err)
		return err
	}

	fmt.Println("Spectrogram image saved to spectrogram.png")

	return nil
}

// lowPassFilter applies a low-pass filter to the given samples using a moving average filter.
// It smooths the input signal by averaging each sample with its neighboring samples within the specified window size.
//
// Parameters:
// - samples: A slice of float64 representing the input signal samples.
// - windowSize: An integer specifying the size of the moving average window. It should be an odd number.
//
// Returns:
// - A slice of float64 containing the filtered samples.
func lowPassFilter(samples []float64, windowSize int) []float64 {
	filteredSamples := make([]float64, len(samples))
	cumulativeSum := make([]float64, len(samples)+1)

	for i := 1; i <= len(samples); i++ {
		cumulativeSum[i] = cumulativeSum[i-1] + samples[i-1]
	}

	halfWindow := windowSize / 2
	for i := halfWindow; i < len(samples)-halfWindow; i++ {
		filteredSamples[i] = (cumulativeSum[i+halfWindow+1] - cumulativeSum[i-halfWindow]) / float64(windowSize)
	}

	return filteredSamples
}

// calculateRMS calculates the Root Mean Square (RMS) value of a given spectrogram.
// The spectrogram is represented as a 2D slice of complex128 values, where each
// element represents a frequency bin in a specific time frame.
//
// The RMS value is computed by taking the square root of the average of the
// squared magnitudes of the complex values in the spectrogram.
//
// Parameters:
// - spectogram: A 2D slice of complex128 values representing the spectrogram.
//
// Returns:
// - A float64 value representing the RMS of the spectrogram.
func calculateRMS(spectogram [][]complex128) float64 {
	rms := 0.0
	for _, frame := range spectogram {
		for _, complexVal := range frame {
			rms += cmplx.Abs(complexVal) * cmplx.Abs(complexVal)
		}
	}

	rms = math.Sqrt(rms / float64(len(spectogram)*len(spectogram[0])))
	return rms
}

// Downsample downsamples the input audio from originalSampleRate to targetSampleRate
// downsample reduces the sample rate of the input signal to the target sample rate.
// It takes an input slice of float64 representing the original signal, the original sample rate,
// and the target sample rate. It returns a new slice of float64 representing the downsampled signal
// and an error if the sample rates are invalid.
//
// Parameters:
// - input: []float64 - the original signal to be downsampled
// - originalSampleRate: int - the sample rate of the original signal
// - targetSampleRate: int - the desired sample rate after downsampling
//
// Returns:
// - []float64 - the downsampled signal
// - error - an error if the sample rates are invalid or if the target sample rate is greater than the original sample rate
//
// The function ensures that the target sample rate is less than or equal to the original sample rate
// and that both sample rates are positive. It calculates the ratio of the original sample rate to the
// target sample rate and uses this ratio to average the input signal over intervals, producing the downsampled signal.
func downsample(input []float64, originalSampleRate, targetSampleRate int) ([]float64, error) {
	if targetSampleRate <= 0 || originalSampleRate <= 0 {
		return nil, errors.New("sample rates must be positive")
	}
	if targetSampleRate > originalSampleRate {
		return nil, errors.New("target sample rate must be less than or equal to original sample rate")
	}

	ratio := originalSampleRate / targetSampleRate
	if ratio <= 0 {
		return nil, errors.New("invalid ratio calculated from sample rates")
	}

	var resampled []float64
	for i := 0; i < len(input); i += ratio {
		end := i + ratio
		if end > len(input) {
			end = len(input)
		}

		sum := 0.0
		for j := i; j < end; j++ {
			sum += input[j]
		}
		avg := sum / float64(end-i)
		resampled = append(resampled, avg)
	}

	return resampled, nil
}
