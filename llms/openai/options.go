package openai

import (
	"log/slog"
	"time"
)

// Default configuration constants for the OpenAI provider.
const (
	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 120 * time.Second
	// DefaultRetryAttempts is the default number of retry attempts for transient failures.
	DefaultRetryAttempts = 3
	// DefaultRetryDelay is the initial delay between retries.
	DefaultRetryDelay = 2 * time.Second
	// DefaultMaxRetryDelay is the maximum delay between retries.
	DefaultMaxRetryDelay = 30 * time.Second
	// DefaultRetryJitter is the maximum random jitter added to retry delays.
	DefaultRetryJitter = 1 * time.Second
	// DefaultEmbeddingModel is the default model used for embeddings.
	DefaultEmbeddingModel = "text-embedding-3-small"
)

// options holds the configuration for an OpenAI LLM instance.
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

// Option configures an OpenAI LLM via the functional options pattern.
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

// WithModel sets the chat completion model (e.g., "gpt-4o", "gpt-4o-mini").
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithEmbeddingModel sets the embedding model (e.g., "text-embedding-3-small").
func WithEmbeddingModel(model string) Option {
	return func(opts *options) {
		opts.embeddingModel = model
	}
}

// WithAPIKey sets the OpenAI API key. Required.
func WithAPIKey(apiKey string) Option {
	return func(opts *options) {
		opts.apiKey = apiKey
	}
}

// WithBaseURL sets a custom base URL for the OpenAI API (useful for proxies).
func WithBaseURL(baseURL string) Option {
	return func(opts *options) {
		opts.baseURL = baseURL
	}
}

// WithLogger sets the logger for the LLM instance.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}

// WithRetryAttempts sets the number of retry attempts for transient failures.
// Negative values are ignored.
func WithRetryAttempts(attempts int) Option {
	return func(opts *options) {
		if attempts >= 0 {
			opts.retryAttempts = attempts
		}
	}
}

// WithRetryDelay sets the initial retry delay.
// Values less than or equal to zero are ignored.
func WithRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.retryDelay = delay
		}
	}
}

// WithMaxRetryDelay sets the maximum retry delay.
// Values less than or equal to zero are ignored.
func WithMaxRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.maxRetryDelay = delay
		}
	}
}

// WithRetryJitter sets the maximum random jitter added to retry delays.
func WithRetryJitter(jitter time.Duration) Option {
	return func(opts *options) {
		if jitter >= 0 {
			opts.retryJitter = jitter
		}
	}
}

// WithRequestTimeout sets the HTTP request timeout.
// Values less than or equal to zero are ignored.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(opts *options) {
		if timeout > 0 {
			opts.requestTimeout = timeout
		}
	}
}

// WithThinking enables reasoning/thinking mode (e.g., for o3-mini).
func WithThinking(enabled bool) Option {
	return func(opts *options) {
		opts.thinking = &enabled
	}
}

// WithReasoningEffort sets the reasoning effort level ("low", "medium", "high").
func WithReasoningEffort(effort string) Option {
	return func(opts *options) {
		opts.reasoningEffort = effort
	}
}

// WithOrganization sets the OpenAI organization ID.
func WithOrganization(org string) Option {
	return func(opts *options) {
		opts.organization = org
	}
}

// WithProject sets the OpenAI project ID.
func WithProject(project string) Option {
	return func(opts *options) {
		opts.project = project
	}
}
