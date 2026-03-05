// Package voice provides interfaces and types for Text-to-Speech synthesis.
// It defines a modular interface that supports multiple TTS backends.
package voice

import (
	"context"
	"io"
)

// Audio represents synthesized audio data with its format.
type Audio struct {
	// Data contains the raw audio bytes.
	Data []byte
	// Format specifies the audio format (e.g., "mp3", "wav", "opus").
	Format string
}

// Synthesizer is the interface for Text-to-Speech providers.
// Implementations convert text into audio data, supporting both
// buffered synthesis and streaming modes.
type Synthesizer interface {
	// Synthesize generates audio from text and returns the complete audio data.
	// Use this for shorter texts where buffering the entire response is acceptable.
	Synthesize(ctx context.Context, text string, opts ...Option) (*Audio, error)

	// Stream generates audio from text and returns a stream for reading audio chunks.
	// Use this for longer texts or when you want to process audio as it arrives.
	// The caller is responsible for closing the returned ReadCloser.
	Stream(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error)
}

// SynthesizeOptions configures text-to-speech synthesis parameters.
type SynthesizeOptions struct {
	// Model specifies the TTS model to use (e.g., "tts-1", "kokoro").
	Model string
	// Voice specifies the voice identifier (e.g., "alloy", "af_bella").
	Voice string
	// Format specifies the output audio format (e.g., "mp3", "wav").
	Format string
	// Speed specifies the speech speed (0.25 to 4.0, where 1.0 is normal).
	Speed float64
}

// Option is a functional option for configuring synthesis parameters.
type Option func(*SynthesizeOptions)

// WithModel sets the TTS model for synthesis.
func WithModel(model string) Option {
	return func(o *SynthesizeOptions) {
		if model != "" {
			o.Model = model
		}
	}
}

// WithVoice sets the voice identifier for synthesis.
func WithVoice(voice string) Option {
	return func(o *SynthesizeOptions) {
		if voice != "" {
			o.Voice = voice
		}
	}
}

// WithFormat sets the output audio format.
func WithFormat(format string) Option {
	return func(o *SynthesizeOptions) {
		if format != "" {
			o.Format = format
		}
	}
}

// WithSpeed sets the speech speed multiplier.
// Valid range is 0.25 to 4.0, where 1.0 is normal speed.
func WithSpeed(speed float64) Option {
	return func(o *SynthesizeOptions) {
		if speed > 0 {
			o.Speed = speed
		}
	}
}
