package ollama

import (
	"log/slog"
	"net/http"
	"net/url"
)

// options holds configuration settings for the Ollama client.
type options struct {
	model           string
	ollamaServerURL *url.URL
	httpClient      *http.Client
	logger          *slog.Logger
}

// Option is a function type for configuring Ollama client options.
type Option func(*options)

// applyOptions creates a new options instance with defaults and applies the provided options.
func applyOptions(opts ...Option) options {
	o := options{
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

func WithServerURL(rawURL string) Option {
	return func(opts *options) {
		if parsedURL, err := url.Parse(rawURL); err == nil {
			opts.ollamaServerURL = parsedURL
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(opts *options) {
		opts.httpClient = client
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}
