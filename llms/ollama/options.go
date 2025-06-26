package ollama

import (
	"log/slog"
	"net/http"
	"net/url"
)

type options struct {
	model           string
	ollamaServerURL *url.URL
	httpClient      *http.Client
	logger          *slog.Logger
}

type Option func(*options)

func applyOptions(opts ...Option) options {
	o := options{
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

// WithModel sets the model name to use for requests.
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithServerURL sets the Ollama server URL.
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
