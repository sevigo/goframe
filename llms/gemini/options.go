package gemini

import (
	"log/slog"
)

type options struct {
	model  string
	apiKey string
	logger *slog.Logger
}

type Option func(*options)

// applyOptions creates a new options instance with defaults and applies the provided options.
func applyOptions(opts ...Option) options {
	o := options{
		model:  "gemini-2.5-flash",
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

func WithAPIKey(apiKey string) Option {
	return func(opts *options) {
		opts.apiKey = apiKey
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}
