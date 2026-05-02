package openai

import (
	"log/slog"
	"time"
)

const (
	DefaultTimeout             = 120 * time.Second
	DefaultRetryAttempts       = 3
	DefaultRetryDelay          = 2 * time.Second
	DefaultMaxRetryDelay       = 30 * time.Second
	DefaultRetryJitter         = 1 * time.Second
	DefaultEmbeddingModel      = "text-embedding-3-small"
)

type options struct {
	model           string
	embeddingModel  string
	apiKey          string
	baseURL         string
	logger          *slog.Logger
	retryAttempts   int
	retryDelay      time.Duration
	maxRetryDelay   time.Duration
	retryJitter     time.Duration
	requestTimeout  time.Duration
	thinking        *bool
	reasoningEffort string
	organization    string
	project         string
}

type Option func(*options)

func applyOptions(opts ...Option) options {
	o := options{
		model:          "gpt-4o",
		embeddingModel: DefaultEmbeddingModel,
		logger:         slog.Default(),
		retryAttempts:  DefaultRetryAttempts,
		retryDelay:     DefaultRetryDelay,
		maxRetryDelay:  DefaultMaxRetryDelay,
		retryJitter:    DefaultRetryJitter,
		requestTimeout: DefaultTimeout,
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

func WithBaseURL(baseURL string) Option {
	return func(opts *options) {
		opts.baseURL = baseURL
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}

func WithRetryAttempts(attempts int) Option {
	return func(opts *options) {
		if attempts >= 0 {
			opts.retryAttempts = attempts
		}
	}
}

func WithRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.retryDelay = delay
		}
	}
}

func WithMaxRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.maxRetryDelay = delay
		}
	}
}

func WithRetryJitter(jitter time.Duration) Option {
	return func(opts *options) {
		if jitter >= 0 {
			opts.retryJitter = jitter
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

func WithOrganization(org string) Option {
	return func(opts *options) {
		opts.organization = org
	}
}

func WithProject(project string) Option {
	return func(opts *options) {
		opts.project = project
	}
}
