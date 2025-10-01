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
	thinking        *bool
	reasoningEffort string
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

func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

func WithServerURL(rawURL string) Option {
	return func(opts *options) {
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			slog.Warn("Failed to parse server URL", "url", rawURL, "error", err)
			return
		}
		opts.ollamaServerURL = parsedURL
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(opts *options) {
		if client != nil {
			opts.httpClient = client
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}

func WithThinking(enabled bool) Option {
	return func(opts *options) {
		opts.thinking = &enabled
	}
}

func WithReasoningEffort(effort string) Option {
	return func(opts *options) {
		opts.reasoningEffort = effort
	}
}
