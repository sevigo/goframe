// Package httpclient provides a shared HTTP client with sensible defaults
// for connection pooling, timeouts, and retry logic.
package httpclient

import (
	"net/http"
	"time"
)

// Default configuration values for HTTP clients.
const (
	// Default timeout for HTTP requests.
	DefaultTimeout = 120 * time.Second

	// Default maximum number of idle connections across all hosts.
	DefaultMaxIdleConns = 100

	// Default maximum number of idle connections per host.
	DefaultMaxIdleConnsPerHost = 20

	// Default timeout for idle connections.
	DefaultIdleConnTimeout = 30 * time.Second

	// Default timeout for TLS handshakes.
	DefaultTLSHandshakeTimeout = 10 * time.Second

	// Default timeout for connection attempts.
	DefaultDialTimeout = 30 * time.Second

	// Default keep-alive timeout for connections.
	DefaultKeepAlive = 30 * time.Second

	// Default response header timeout.
	DefaultResponseHeaderTimeout = 30 * time.Second

	// Default timeout for expecting a 100-continue response.
	DefaultExpectContinueTimeout = 1 * time.Second

	// Default retry configuration.
	DefaultRetryAttempts = 3
	DefaultRetryDelay    = 2 * time.Second
	DefaultMaxRetryDelay = 30 * time.Second
	DefaultRetryJitter   = 1 * time.Second
)

// Config holds configuration for creating an HTTP client.
type Config struct {
	// Timeout is the total timeout for HTTP requests.
	Timeout time.Duration

	// MaxIdleConns is the maximum number of idle connections across all hosts.
	MaxIdleConns int

	// MaxIdleConnsPerHost is the maximum number of idle connections per host.
	MaxIdleConnsPerHost int

	// IdleConnTimeout is the timeout for idle connections.
	IdleConnTimeout time.Duration

	// TLSHandshakeTimeout is the timeout for TLS handshakes.
	TLSHandshakeTimeout time.Duration

	// DialTimeout is the timeout for connection attempts.
	DialTimeout time.Duration

	// KeepAlive is the keep-alive timeout for connections.
	KeepAlive time.Duration

	// ResponseHeaderTimeout is the timeout for response headers.
	ResponseHeaderTimeout time.Duration

	// ExpectContinueTimeout is the timeout for expecting 100-continue responses.
	ExpectContinueTimeout time.Duration

	// RetryAttempts is the number of retry attempts for transient errors.
	// Set to 0 to disable retries.
	RetryAttempts int

	// RetryDelay is the initial delay between retry attempts.
	RetryDelay time.Duration

	// MaxRetryDelay is the maximum delay between retry attempts.
	MaxRetryDelay time.Duration

	// RetryJitter is the random jitter added to retry delays.
	RetryJitter time.Duration
}

// Option is a function that modifies a Config.
type Option func(*Config)

// NewConfig creates a Config with default values.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Timeout:               DefaultTimeout,
		MaxIdleConns:          DefaultMaxIdleConns,
		MaxIdleConnsPerHost:   DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		TLSHandshakeTimeout:   DefaultTLSHandshakeTimeout,
		DialTimeout:           DefaultDialTimeout,
		KeepAlive:             DefaultKeepAlive,
		ResponseHeaderTimeout: DefaultResponseHeaderTimeout,
		ExpectContinueTimeout: DefaultExpectContinueTimeout,
		RetryAttempts:         DefaultRetryAttempts,
		RetryDelay:            DefaultRetryDelay,
		MaxRetryDelay:         DefaultMaxRetryDelay,
		RetryJitter:           DefaultRetryJitter,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *Config) {
		if timeout > 0 {
			cfg.Timeout = timeout
		}
	}
}

// WithMaxIdleConns sets the maximum number of idle connections.
func WithMaxIdleConns(n int) Option {
	return func(cfg *Config) {
		if n > 0 {
			cfg.MaxIdleConns = n
		}
	}
}

// WithMaxIdleConnsPerHost sets the maximum number of idle connections per host.
func WithMaxIdleConnsPerHost(n int) Option {
	return func(cfg *Config) {
		if n > 0 {
			cfg.MaxIdleConnsPerHost = n
		}
	}
}

// WithIdleConnTimeout sets the idle connection timeout.
func WithIdleConnTimeout(timeout time.Duration) Option {
	return func(cfg *Config) {
		if timeout > 0 {
			cfg.IdleConnTimeout = timeout
		}
	}
}

// WithTLSHandshakeTimeout sets the TLS handshake timeout.
func WithTLSHandshakeTimeout(timeout time.Duration) Option {
	return func(cfg *Config) {
		if timeout > 0 {
			cfg.TLSHandshakeTimeout = timeout
		}
	}
}

// WithRetryAttempts sets the number of retry attempts.
func WithRetryAttempts(attempts int) Option {
	return func(cfg *Config) {
		if attempts >= 0 {
			cfg.RetryAttempts = attempts
		}
	}
}

// WithRetryDelay sets the initial retry delay.
func WithRetryDelay(delay time.Duration) Option {
	return func(cfg *Config) {
		if delay > 0 {
			cfg.RetryDelay = delay
		}
	}
}

// WithMaxRetryDelay sets the maximum retry delay.
func WithMaxRetryDelay(delay time.Duration) Option {
	return func(cfg *Config) {
		if delay > 0 {
			cfg.MaxRetryDelay = delay
		}
	}
}

// WithRetryJitter sets the retry jitter.
func WithRetryJitter(jitter time.Duration) Option {
	return func(cfg *Config) {
		if jitter >= 0 {
			cfg.RetryJitter = jitter
		}
	}
}

// WithResponseHeaderTimeout sets the timeout for response headers.
func WithResponseHeaderTimeout(timeout time.Duration) Option {
	return func(cfg *Config) {
		if timeout > 0 {
			cfg.ResponseHeaderTimeout = timeout
		}
	}
}

// NewClient creates a new HTTP client with the given configuration.
func NewClient(cfg *Config) *http.Client {
	if cfg == nil {
		cfg = NewConfig()
	}

	transport := &http.Transport{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
	}

	return &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}
}

// DefaultClient returns an HTTP client with sensible defaults.
// This is a shared instance that can be used across the application.
var DefaultClient = NewClient(nil)

// NewDefaultClient creates a new HTTP client with default settings.
// Use this when you need a separate client instance from the shared DefaultClient.
func NewDefaultClient() *http.Client {
	return NewClient(NewConfig())
}

// DownloadClient returns an HTTP client optimized for downloading large files.
// It has a longer timeout and optimized settings for file transfers.
func DownloadClient() *http.Client {
	return NewClient(NewConfig(
		WithTimeout(10*time.Minute),
		WithMaxIdleConns(10),
		WithMaxIdleConnsPerHost(5),
	))
}
