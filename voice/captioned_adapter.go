package voice

import (
	"context"
	"errors"
	"io"
)

var _ CaptionedSynthesizer = (*EstimatedCaptionedSynthesizer)(nil)

// EstimatedCaptionedSynthesizer adds estimated word timestamps to any Synthesizer.
type EstimatedCaptionedSynthesizer struct {
	Syn             Synthesizer
	Format          string
	SilentThreshold int16
}

// NewEstimatedCaptionedSynthesizer wraps a Synthesizer with estimated captioning.
func NewEstimatedCaptionedSynthesizer(syn Synthesizer, format ...string) *EstimatedCaptionedSynthesizer {
	f := "wav"
	if len(format) > 0 && format[0] != "" {
		f = format[0]
	}
	return &EstimatedCaptionedSynthesizer{
		Syn:             syn,
		Format:          f,
		SilentThreshold: int16(SilenceThresholdDefault),
	}
}

// WithSilentThreshold sets the silence threshold for timestamp estimation.
func (e *EstimatedCaptionedSynthesizer) WithSilentThreshold(threshold int16) *EstimatedCaptionedSynthesizer {
	e.SilentThreshold = threshold
	return e
}

// Synthesize delegates to the underlying Synthesizer.
func (e *EstimatedCaptionedSynthesizer) Synthesize(ctx context.Context, text string, opts ...Option) (*Audio, error) {
	return e.Syn.Synthesize(ctx, text, opts...)
}

// Stream delegates to the underlying Synthesizer.
func (e *EstimatedCaptionedSynthesizer) Stream(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error) {
	return e.Syn.Stream(ctx, text, opts...)
}

// SynthesizeCaptioned generates audio with estimated word timestamps.
func (e *EstimatedCaptionedSynthesizer) SynthesizeCaptioned(ctx context.Context, text string, opts ...Option) (*CaptionedAudio, error) {
	options := &SynthesizeOptions{
		Format: e.Format,
	}
	for _, opt := range opts {
		opt(options)
	}
	if options.Format == "" {
		options.Format = e.Format
	}

	audio, err := e.Syn.Synthesize(ctx, text, opts...)
	if err != nil {
		return nil, err
	}

	if options.Format != "wav" {
		return nil, errors.New("voice: estimated captions require WAV format; use WithFormat(\"wav\")")
	}

	durationMs, leadingSilenceMs, _, err := AnalyzeWAVAudio(audio.Data)
	if err != nil {
		return nil, err
	}

	timestamps := EstimateWordTimestamps(text, durationMs, leadingSilenceMs)

	return &CaptionedAudio{
		Data:       audio.Data,
		Format:     audio.Format,
		DurationMs: durationMs,
		Timestamps: timestamps,
	}, nil
}

// StreamCaptioned is not implemented; use SynthesizeCaptioned instead.
func (e *EstimatedCaptionedSynthesizer) StreamCaptioned(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error) {
	return nil, errors.New("voice: estimated captions do not support streaming; use SynthesizeCaptioned")
}
