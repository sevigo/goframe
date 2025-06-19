package gemini

import (
	"log/slog"
)

// options holds configuration for the Gemini client.
type options struct {
	model  string
	apiKey string
	logger *slog.Logger
}

// Option is a function type for configuring the client.
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

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithAPIKey sets the Gemini API key.
func WithAPIKey(apiKey string) Option {
	return func(opts *options) {
		opts.apiKey = apiKey
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}
