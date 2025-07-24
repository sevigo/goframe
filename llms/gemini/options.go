package gemini

import (
	"log/slog"
)

// options holds configuration for the Gemini client.
type options struct {
	model          string
	embeddingModel string
	apiKey         string
	logger         *slog.Logger
}

// Option is a function type for configuring the client.
type Option func(*options)

// applyOptions creates a new options instance with defaults and applies the provided options.
func applyOptions(opts ...Option) options {
	o := options{
		model:          "gemini-2.5-flash",
		embeddingModel: "gemini-embedding-001",
		logger:         slog.Default(),
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithModel sets the generation model name.
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithEmbeddingModel sets the embedding model name.
func WithEmbeddingModel(model string) Option {
	return func(opts *options) {
		opts.embeddingModel = model
	}
}

// WithAPIKey sets the Gemini API key.
func WithAPIKey(apiKey string) Option {
	return func(opts *options) {
		opts.apiKey = apiKey
	}
}

// WithLogger sets a custom logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}
