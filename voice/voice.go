package voice

import (
	"context"
)

type Audio struct {
	Data   []byte
	Format string
}

type Synthesizer interface {
	Synthesize(ctx context.Context, text string, opts ...Option) (*Audio, error)
}

type SynthesizeOptions struct {
	Model  string
	Voice  string
	Format string
	Speed  float64
}

type Option func(*SynthesizeOptions)

func WithModel(model string) Option {
	return func(o *SynthesizeOptions) {
		if model != "" {
			o.Model = model
		}
	}
}

func WithVoice(voice string) Option {
	return func(o *SynthesizeOptions) {
		if voice != "" {
			o.Voice = voice
		}
	}
}

func WithFormat(format string) Option {
	return func(o *SynthesizeOptions) {
		if format != "" {
			o.Format = format
		}
	}
}

func WithSpeed(speed float64) Option {
	return func(o *SynthesizeOptions) {
		if speed > 0 {
			o.Speed = speed
		}
	}
}
