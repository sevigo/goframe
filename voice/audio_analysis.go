package voice

import (
	"encoding/binary"
	"math"
)

const (
	// SilenceThresholdDefault is the default amplitude threshold for silence detection.
	SilenceThresholdDefault = 500
	// SilenceWindowMs is the analysis window size in milliseconds.
	SilenceWindowMs = 10
)

// ComputeWAVDuration returns the duration of WAV audio data in milliseconds.
func ComputeWAVDuration(data []byte, info wavInfo) int {
	if info.sampleRate <= 0 || info.bytesPerSample <= 0 || info.numChannels <= 0 {
		return 0
	}
	dataSize := len(data) - info.dataOffset
	if dataSize <= 0 {
		return 0
	}
	bytesPerSecond := info.sampleRate * info.bytesPerSample * info.numChannels
	durationMs := (dataSize * 1000) / bytesPerSecond
	return durationMs
}

// DetectTrailingSilence returns the duration of trailing silence in milliseconds.
func DetectTrailingSilence(data []byte, info wavInfo, threshold ...int16) int {
	if info.bitsPerSample != 16 || info.dataOffset >= len(data) {
		return 0
	}

	thresh := int16(SilenceThresholdDefault)
	if len(threshold) > 0 {
		thresh = threshold[0]
	}

	pcmData := data[info.dataOffset:]
	if len(pcmData) < 2 {
		return 0
	}

	silenceFrames := 0
	for i := len(pcmData) - 2; i >= 0; i -= 2 {
		sample := int16(binary.LittleEndian.Uint16(pcmData[i : i+2]))
		if math.Abs(float64(sample)) > float64(thresh) {
			break
		}
		silenceFrames++
	}

	silenceMs := (silenceFrames * 1000) / info.sampleRate
	return silenceMs
}

// DetectLeadingSilence returns the duration of leading silence in milliseconds.
func DetectLeadingSilence(data []byte, info wavInfo, threshold ...int16) int {
	if info.bitsPerSample != 16 || info.dataOffset >= len(data) {
		return 0
	}

	thresh := int16(SilenceThresholdDefault)
	if len(threshold) > 0 {
		thresh = threshold[0]
	}

	pcmData := data[info.dataOffset:]
	if len(pcmData) < 2 {
		return 0
	}

	silenceFrames := 0
	for i := 0; i < len(pcmData)-1; i += 2 {
		sample := int16(binary.LittleEndian.Uint16(pcmData[i : i+2]))
		if math.Abs(float64(sample)) > float64(thresh) {
			break
		}
		silenceFrames++
	}

	silenceMs := (silenceFrames * 1000) / info.sampleRate
	return silenceMs
}

// AnalyzeWAVAudio computes duration, leading silence, and trailing silence for WAV data.
func AnalyzeWAVAudio(data []byte) (int, int, int, error) {
	info, err := parseWAVHeader(data)
	if err != nil {
		return 0, 0, 0, err
	}

	durationMs := ComputeWAVDuration(data, info)
	leadingSilenceMs := DetectLeadingSilence(data, info)
	trailingSilenceMs := DetectTrailingSilence(data, info)

	silenceBuffer := 50
	if leadingSilenceMs+trailingSilenceMs > durationMs-silenceBuffer {
		proportionalLead := durationMs / 4
		if leadingSilenceMs > proportionalLead {
			leadingSilenceMs = proportionalLead
		}
		trailingSilenceMs = durationMs - leadingSilenceMs - silenceBuffer
		if trailingSilenceMs < 0 {
			trailingSilenceMs = 0
		}
	}

	return durationMs, leadingSilenceMs, trailingSilenceMs, nil
}

// ValidateWAVConsistency checks that all segments share the same audio format.
func ValidateWAVConsistency(segments [][]byte, expectedInfo wavInfo) error {
	for i, data := range segments {
		info, err := parseWAVHeader(data)
		if err != nil {
			return err
		}
		if info.sampleRate != expectedInfo.sampleRate {
			return newFormatMismatchError(i, "sample rate", expectedInfo.sampleRate, info.sampleRate)
		}
		if info.numChannels != expectedInfo.numChannels {
			return newFormatMismatchError(i, "channels", expectedInfo.numChannels, info.numChannels)
		}
		if info.bitsPerSample != expectedInfo.bitsPerSample {
			return newFormatMismatchError(i, "bits per sample", expectedInfo.bitsPerSample, info.bitsPerSample)
		}
	}
	return nil
}

// FormatMismatchError indicates audio format mismatch across segments.
type FormatMismatchError struct {
	SegmentIndex int
	Property     string
	Expected     int
	Actual       int
}

// Error returns the error message.
func (e *FormatMismatchError) Error() string {
	return "voice: audio format mismatch"
}

func newFormatMismatchError(idx int, prop string, expected, actual int) *FormatMismatchError {
	return &FormatMismatchError{
		SegmentIndex: idx,
		Property:     prop,
		Expected:     expected,
		Actual:       actual,
	}
}
