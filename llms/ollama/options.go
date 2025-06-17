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

// WithModel sets the model name to use for requests.
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithServerURL sets the Ollama server URL.
// If the provided URL is invalid, the option will be ignored silently.
func WithServerURL(rawURL string) Option {
	return func(opts *options) {
		if parsedURL, err := url.Parse(rawURL); err == nil {
			opts.ollamaServerURL = parsedURL
		}
	}
}

// WithHTTPClient sets a custom HTTP client for making requests.
func WithHTTPClient(client *http.Client) Option {
	return func(opts *options) {
		opts.httpClient = client
	}
}

// GetModel returns the configured model name.
func (o *options) GetModel() string {
	return o.model
}

// GetServerURL returns the configured server URL.
func (o *options) GetServerURL() *url.URL {
	return o.ollamaServerURL
}

// GetHTTPClient returns the configured HTTP client.
func (o *options) GetHTTPClient() *http.Client {
	return o.httpClient
}

// HasServerURL returns true if a server URL is configured.
func (o *options) HasServerURL() bool {
	return o.ollamaServerURL != nil
}

// HasHTTPClient returns true if a custom HTTP client is configured.
func (o *options) HasHTTPClient() bool {
	return o.httpClient != nil
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}
