package fastapi

import (
	"log/slog"
	"net/http"
	"time"
)

type options struct {
	httpClient *http.Client
	logger     *slog.Logger
	task       string
}

// Option defines a function type for configuring the embedder.
type Option func(*options)

func defaultOptions() *options {
	return &options{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		logger:     slog.Default(),
		task:       "Given a web search query, retrieve relevant passages that answer the query",
	}
}

// WithHTTPClient allows providing a custom http.Client.
func WithHTTPClient(client *http.Client) Option {
	return func(o *options) {
		if client != nil {
			o.httpClient = client
		}
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) Option {
	return func(o *options) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// WithTask allows overriding the default task description sent to the API.
func WithTask(task string) Option {
	return func(o *options) {
		if task != "" {
			o.task = task
		}
	}
}
