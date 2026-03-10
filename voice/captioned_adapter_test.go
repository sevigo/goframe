package voice

import (
	"context"
	"errors"
	"io"
	"testing"
)

type mockSynthesizer struct {
	audioData []byte
	err       error
	called    bool
	textArg   string
}

func (m *mockSynthesizer) Synthesize(ctx context.Context, text string, opts ...Option) (*Audio, error) {
	m.called = true
	m.textArg = text
	if m.err != nil {
		return nil, m.err
	}
	return &Audio{Data: m.audioData, Format: "wav"}, nil
}

func (m *mockSynthesizer) Stream(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func createTestWAVData() []byte {
	sampleRate := 24000
	numChannels := 1
	bitsPerSample := 16
	audioDurationMs := 2000

	bytesPerSample := bitsPerSample / 8
	dataSize := (sampleRate * numChannels * bytesPerSample * audioDurationMs) / 1000

	dataSizeWithPadding := dataSize
	if dataSizeWithPadding%2 != 0 {
		dataSizeWithPadding++
	}

	fileSize := 36 + dataSizeWithPadding

	data := make([]byte, 0, 44+dataSizeWithPadding)

	data = append(data, 'R', 'I', 'F', 'F')
	data = append(data, byte(fileSize), byte(fileSize>>8), byte(fileSize>>16), byte(fileSize>>24))
	data = append(data, 'W', 'A', 'V', 'E')
	data = append(data, 'f', 'm', 't', ' ')
	data = append(data, 16, 0, 0, 0)
	data = append(data, 1, 0)
	data = append(data, byte(numChannels), byte(numChannels>>8))
	data = append(data, byte(sampleRate), byte(sampleRate>>8), byte(sampleRate>>16), byte(sampleRate>>24))
	byteRate := sampleRate * numChannels * bytesPerSample
	data = append(data, byte(byteRate), byte(byteRate>>8), byte(byteRate>>16), byte(byteRate>>24))
	blockAlign := numChannels * bytesPerSample
	data = append(data, byte(blockAlign), byte(blockAlign>>8))
	data = append(data, byte(bitsPerSample), byte(bitsPerSample>>8))
	data = append(data, 'd', 'a', 't', 'a')
	data = append(data, byte(dataSizeWithPadding), byte(dataSizeWithPadding>>8), byte(dataSizeWithPadding>>16), byte(dataSizeWithPadding>>24))

	audioData := make([]byte, dataSizeWithPadding)
	for i := range audioData {
		if i%100 == 0 {
			audioData[i] = byte(i % 256)
			if i+1 < len(audioData) {
				audioData[i+1] = byte((i * 2) % 256)
			}
		} else {
			audioData[i] = 0
		}
	}

	return append(data, audioData...)
}

func TestNewEstimatedCaptionedSynthesizer(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	if est.Syn != mock {
		t.Error("Syn not set correctly")
	}
	if est.Format != "wav" {
		t.Errorf("Format = %q, want 'wav'", est.Format)
	}
	if est.SilentThreshold != int16(SilenceThresholdDefault) {
		t.Errorf("SilentThreshold = %d, want %d", est.SilentThreshold, SilenceThresholdDefault)
	}
}

func TestWithSilentThreshold(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock).WithSilentThreshold(1000)

	if est.SilentThreshold != 1000 {
		t.Errorf("SilentThreshold = %d, want 1000", est.SilentThreshold)
	}
}

func TestSynthesizePassthrough(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	audio, err := est.Synthesize(context.Background(), "test")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if !mock.called {
		t.Error("underlying Synthesizer not called")
	}
	if mock.textArg != "test" {
		t.Errorf("text arg = %q, want 'test'", mock.textArg)
	}
	if string(audio.Data) != string(mock.audioData) {
		t.Error("audio data mismatch")
	}
}

func TestStreamPassthrough(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	_, err := est.Stream(context.Background(), "test")
	if err == nil {
		t.Error("Stream() should return error for passthrough")
	}
}

func TestSynthesizeCaptioned(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	audio, err := est.SynthesizeCaptioned(context.Background(), "Hello world")
	if err != nil {
		t.Fatalf("SynthesizeCaptioned() error = %v", err)
	}

	if audio.Format != "wav" {
		t.Errorf("Format = %q, want 'wav'", audio.Format)
	}
	if audio.DurationMs <= 0 {
		t.Errorf("DurationMs = %d, want > 0", audio.DurationMs)
	}
	if len(audio.Timestamps) != 2 {
		t.Errorf("len(Timestamps) = %d, want 2", len(audio.Timestamps))
	}
	if len(audio.Data) == 0 {
		t.Error("audio data is empty")
	}

	t.Logf("Duration: %dms, Timestamps: %d", audio.DurationMs, len(audio.Timestamps))
	for i, ts := range audio.Timestamps {
		t.Logf("  [%d] %s: %d-%dms", i, ts.Word, ts.StartMs, ts.EndMs)
		if ts.Word == "" {
			t.Errorf("timestamp[%d] has empty word", i)
		}
		if ts.StartMs >= ts.EndMs {
			t.Errorf("timestamp[%d]: start (%d) >= end (%d)", i, ts.StartMs, ts.EndMs)
		}
	}
}

func TestSynthesizeCaptionedNonWAV(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	_, err := est.SynthesizeCaptioned(context.Background(), "test", WithFormat("mp3"))
	if err == nil {
		t.Error("SynthesizeCaptioned() with mp3 format should return error")
	}
}

func TestSynthesizeCaptionedError(t *testing.T) {
	mock := &mockSynthesizer{err: errors.New("synthesis failed")}
	est := NewEstimatedCaptionedSynthesizer(mock)

	_, err := est.SynthesizeCaptioned(context.Background(), "test")
	if err == nil {
		t.Error("SynthesizeCaptioned() should return error when synthesis fails")
	}
}

func TestStreamCaptionedNotSupported(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	_, err := est.StreamCaptioned(context.Background(), "test")
	if err == nil {
		t.Error("StreamCaptioned() should return error")
	}
}

func TestTimestampQuality(t *testing.T) {
	mock := &mockSynthesizer{audioData: createTestWAVData()}
	est := NewEstimatedCaptionedSynthesizer(mock)

	text := "The quick brown fox jumps over the lazy dog"
	audio, err := est.SynthesizeCaptioned(context.Background(), text)
	if err != nil {
		t.Fatalf("SynthesizeCaptioned() error = %v", err)
	}

	t.Log("DurationMs:", audio.DurationMs)
	t.Log("Leading silence estimated by audio analysis")

	timestamps := audio.Timestamps
	if len(timestamps) != 9 {
		t.Fatalf("got %d timestamps, want 9", len(timestamps))
	}

	for i := 1; i < len(timestamps); i++ {
		if timestamps[i].StartMs < timestamps[i-1].EndMs {
			t.Errorf("timestamp[%d] overlaps previous: prev end %d, curr start %d",
				i, timestamps[i-1].EndMs, timestamps[i].StartMs)
		}
	}

	lastTS := timestamps[len(timestamps)-1]
	if lastTS.EndMs > audio.DurationMs+50 {
		t.Errorf("last timestamp end (%dms) significantly exceeds total duration (%dms)",
			lastTS.EndMs, audio.DurationMs)
	}

	expectedEnd := audio.DurationMs
	tolerance := 100
	timeDiff := abs(lastTS.EndMs - expectedEnd)
	if timeDiff > tolerance {
		t.Errorf("last word end time %dms differs from duration %dms (tolerance %dms)",
			lastTS.EndMs, expectedEnd, tolerance)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
