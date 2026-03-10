package voice

import (
	"testing"
)

func TestComputeWAVDuration(t *testing.T) {
	tests := []struct {
		name          string
		sampleRate    int
		channels      int
		bitsPerSample int
		dataSize      int
		wantMs        int
	}{
		{
			name:          "1 second mono 16-bit",
			sampleRate:    24000,
			channels:      1,
			bitsPerSample: 16,
			dataSize:      48000,
			wantMs:        1000,
		},
		{
			name:          "500ms mono 16-bit",
			sampleRate:    24000,
			channels:      1,
			bitsPerSample: 16,
			dataSize:      24000,
			wantMs:        500,
		},
		{
			name:          "1 second stereo 16-bit",
			sampleRate:    44100,
			channels:      2,
			bitsPerSample: 16,
			dataSize:      176400,
			wantMs:        1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := wavInfo{
				sampleRate:     tt.sampleRate,
				numChannels:    tt.channels,
				bitsPerSample:  tt.bitsPerSample,
				bytesPerSample: tt.bitsPerSample / 8,
				dataOffset:     0,
			}
			data := make([]byte, tt.dataSize)

			got := ComputeWAVDuration(data, info)
			if got != tt.wantMs {
				t.Errorf("ComputeWAVDuration() = %dms, want %dms", got, tt.wantMs)
			}
		})
	}
}

func TestComputeWAVDurationInvalid(t *testing.T) {
	tests := []struct {
		name string
		info wavInfo
		want int
	}{
		{"zero sample rate", wavInfo{sampleRate: 0}, 0},
		{"zero bytes per sample", wavInfo{sampleRate: 24000, bytesPerSample: 0}, 0},
		{"zero channels", wavInfo{sampleRate: 24000, bytesPerSample: 2, numChannels: 0}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeWAVDuration([]byte{0, 0, 0, 0}, tt.info)
			if got != tt.want {
				t.Errorf("ComputeWAVDuration() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDetectTrailingSilence(t *testing.T) {
	info := wavInfo{
		sampleRate:     24000,
		numChannels:    1,
		bitsPerSample:  16,
		bytesPerSample: 2,
		dataOffset:     0,
	}

	silenceSamples := 480
	data := make([]byte, silenceSamples*2)
	for i := range data {
		data[i] = 0
	}

	thresh := int16(100)
	silenceMs := DetectTrailingSilence(data, info, thresh)

	expectedMs := (silenceSamples * 1000) / info.sampleRate
	if silenceMs < expectedMs-5 || silenceMs > expectedMs+5 {
		t.Errorf("DetectTrailingSilence() = %dms, want approximately %dms", silenceMs, expectedMs)
	}
}

func TestDetectLeadingSilence(t *testing.T) {
	info := wavInfo{
		sampleRate:     24000,
		numChannels:    1,
		bitsPerSample:  16,
		bytesPerSample: 2,
		dataOffset:     0,
	}

	silenceSamples := 480
	data := make([]byte, silenceSamples*2)
	for i := range data {
		data[i] = 0
	}

	thresh := int16(100)
	silenceMs := DetectLeadingSilence(data, info, thresh)

	expectedMs := (silenceSamples * 1000) / info.sampleRate
	if silenceMs < expectedMs-5 || silenceMs > expectedMs+5 {
		t.Errorf("DetectLeadingSilence() = %dms, want approximately %dms", silenceMs, expectedMs)
	}
}

func TestFormatMismatchError(t *testing.T) {
	err := newFormatMismatchError(3, "sample rate", 44100, 24000)
	expected := "voice: audio format mismatch"
	if err.Error() != expected {
		t.Errorf("FormatMismatchError.Error() = %q, want %q", err.Error(), expected)
	}
	if err.SegmentIndex != 3 {
		t.Errorf("SegmentIndex = %d, want 3", err.SegmentIndex)
	}
	if err.Property != "sample rate" {
		t.Errorf("Property = %q, want 'sample rate'", err.Property)
	}
	if err.Expected != 44100 {
		t.Errorf("Expected = %d, want 44100", err.Expected)
	}
	if err.Actual != 24000 {
		t.Errorf("Actual = %d, want 24000", err.Actual)
	}
}
