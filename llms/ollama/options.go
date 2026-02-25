package ollama

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Default HTTP client configuration constants.
const (
	DefaultTimeout             = 120 * time.Second
	DefaultMaxIdleConns        = 100
	DefaultMaxIdleConnsHost    = 20
	DefaultIdleConnTimeout     = 30 * time.Second
	DefaultTLSHandshakeTimeout = 10 * time.Second

	// Retry configuration defaults.
	DefaultRetryAttempts = 3
	DefaultRetryDelay    = 2 * time.Second
	DefaultMaxRetryDelay = 30 * time.Second
	DefaultRetryJitter   = 1 * time.Second
)

type options struct {
	model           string
	ollamaServerURL *url.URL
	httpClient      *http.Client
	logger          *slog.Logger
	thinking        *bool
	reasoningEffort string
	apiKey          string
	keepAlive       time.Duration

	// Retry configuration.
	retryAttempts int
	retryDelay    time.Duration
	maxRetryDelay time.Duration
	retryJitter   time.Duration
}

type Option func(*options)

func applyOptions(opts ...Option) options {
	o := options{
		logger:        slog.Default(),
		retryAttempts: DefaultRetryAttempts,
		retryDelay:    DefaultRetryDelay,
		maxRetryDelay: DefaultMaxRetryDelay,
		retryJitter:   DefaultRetryJitter,
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

func WithAPIKey(apiKey string) Option {
	return func(opts *options) {
		opts.apiKey = apiKey
	}
}

// WithRetryAttempts sets the number of retry attempts for failed API calls.
// Set to 0 to disable retries.
func WithRetryAttempts(attempts int) Option {
	return func(opts *options) {
		if attempts >= 0 {
			opts.retryAttempts = attempts
		}
	}
}

// WithRetryDelay sets the initial delay between retry attempts.
func WithRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.retryDelay = delay
		}
	}
}

// WithMaxRetryDelay sets the maximum delay between retry attempts.
func WithMaxRetryDelay(delay time.Duration) Option {
	return func(opts *options) {
		if delay > 0 {
			opts.maxRetryDelay = delay
		}
	}
}

// WithRetryJitter sets the random jitter added to retry delays.
func WithRetryJitter(jitter time.Duration) Option {
	return func(opts *options) {
		if jitter >= 0 {
			opts.retryJitter = jitter
		}
	}
}

// WithKeepAlive sets how long the model stays loaded in memory after a request.
// Examples: "5m", "10m", "1h", "0" to unload immediately.
func WithKeepAlive(keepAlive string) Option {
	return func(opts *options) {
		if keepAlive == "" {
			return
		}
		if keepAlive == "0" {
			opts.keepAlive = 0
			return
		}
		d, err := time.ParseDuration(keepAlive)
		if err != nil {
			slog.Warn("Failed to parse keep_alive duration", "keep_alive", keepAlive, "error", err)
			return
		}
		opts.keepAlive = d
	}
}
