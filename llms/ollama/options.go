package ollama

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Default HTTP client configuration constants.
const (
	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 120 * time.Second
	// DefaultMaxIdleConns is the default maximum idle connections across all hosts.
	DefaultMaxIdleConns = 100
	// DefaultMaxIdleConnsHost is the default maximum idle connections per host.
	DefaultMaxIdleConnsHost = 20
	// DefaultIdleConnTimeout is the default idle connection timeout.
	DefaultIdleConnTimeout = 30 * time.Second
	// DefaultTLSHandshakeTimeout is the default TLS handshake timeout.
	DefaultTLSHandshakeTimeout = 10 * time.Second

	// DefaultRetryAttempts is the default number of retry attempts.
	DefaultRetryAttempts = 3
	// DefaultRetryDelay is the initial delay between retry attempts.
	DefaultRetryDelay = 2 * time.Second
	// DefaultMaxRetryDelay is the maximum delay between retry attempts.
	DefaultMaxRetryDelay = 30 * time.Second
	// DefaultRetryJitter is the random jitter added to retry delays.
	DefaultRetryJitter = 1 * time.Second
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

// Option configures an Ollama LLM.
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

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(opts *options) {
		opts.model = model
	}
}

// WithServerURL sets the Ollama server URL.
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

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(opts *options) {
		if client != nil {
			opts.httpClient = client
		}
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}

// WithThinking enables or disables model thinking/reasoning output.
func WithThinking(enabled bool) Option {
	return func(opts *options) {
		opts.thinking = &enabled
	}
}

// WithReasoningEffort sets the reasoning effort level.
func WithReasoningEffort(effort string) Option {
	return func(opts *options) {
		opts.reasoningEffort = effort
	}
}

// WithAPIKey sets the API key for authentication.
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
