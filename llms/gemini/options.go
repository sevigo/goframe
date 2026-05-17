package gemini

import (
	"log/slog"
	"net/http"
)

type options struct {
	model          string
	embeddingModel string
	apiKey         string
	logger         *slog.Logger
	httpClient     *http.Client
}

type Option func(*options)

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

func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

func WithEmbeddingModel(model string) Option {
	return func(opts *options) {
		opts.embeddingModel = model
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

func WithHTTPClient(client *http.Client) Option {
	return func(opts *options) {
		if client != nil {
			opts.httpClient = client
		}
	}
}
