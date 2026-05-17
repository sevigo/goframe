package gemini

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/sevigo/goframe/httpclient"
)

const (
	DefaultTimeout       = 120 * time.Second
	DefaultRetryAttempts = httpclient.DefaultRetryAttempts
	DefaultRetryDelay    = httpclient.DefaultRetryDelay
	DefaultMaxRetryDelay = httpclient.DefaultMaxRetryDelay
	DefaultRetryJitter   = httpclient.DefaultRetryJitter
)

type options struct {
	model          string
	embeddingModel string
	apiKey         string
	logger         *slog.Logger
	httpClient     *http.Client
	requestTimeout time.Duration
	retry          httpclient.RetryConfig
}

type Option func(*options)

func applyOptions(opts ...Option) options {
	o := options{
		model:          "gemini-2.5-flash",
		embeddingModel: "gemini-embedding-001",
		logger:         slog.Default(),
		requestTimeout: DefaultTimeout,
		retry: httpclient.RetryConfig{
			Attempts: DefaultRetryAttempts,
			Delay:    DefaultRetryDelay,
			MaxDelay: DefaultMaxRetryDelay,
			Jitter:   DefaultRetryJitter,
		},
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

func WithRequestTimeout(timeout time.Duration) Option {
	return func(opts *options) {
		if timeout > 0 {
			opts.requestTimeout = timeout
		}
	}
}

func WithRetryAttempts(attempts int) Option {
	return func(opts *options) {
		if attempts >= 0 {
			opts.retry.Attempts = attempts
		}
	}
}

func WithRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.retry.Delay = delay
		}
	}
}

func WithMaxRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.retry.MaxDelay = delay
		}
	}
}

func WithRetryJitter(jitter time.Duration) Option {
	return func(opts *options) {
		if jitter >= 0 {
			opts.retry.Jitter = jitter
		}
	}
}
