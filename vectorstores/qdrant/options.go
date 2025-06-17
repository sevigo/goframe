package qdrant

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/sevigo/goframe/embeddings"
)

const (
	defaultContentKey = "content"
	defaultHost       = "localhost"
	defaultPort       = 6334
)

var ErrInvalidOptions = errors.New("qdrant: invalid options provided")

// options holds all configuration options for the Qdrant store.
type options struct {
	collectionName string
	qdrantURL      url.URL
	embedder       embeddings.Embedder
	apiKey         string
	contentKey     string
	logger         *slog.Logger
	useTLS         bool
	timeout        int // in seconds
	retryAttempts  int
	batchSize      int
}

// Option defines a function type for configuring Qdrant store options.
type Option func(*options)

// WithCollectionName sets the collection name for the Qdrant store.
func WithCollectionName(name string) Option {
	return func(opts *options) {
		opts.collectionName = strings.TrimSpace(name)
	}
}

// WithLogger sets the logger for the Qdrant store.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.logger = logger
		}
	}
}

// WithURL sets the Qdrant server URL.
func WithURL(qdrantURL url.URL) Option {
	return func(opts *options) {
		opts.qdrantURL = qdrantURL
	}
}

// WithHost sets the Qdrant server host and constructs the URL.
func WithHost(host string) Option {
	return func(opts *options) {
		if host != "" {
			opts.qdrantURL = url.URL{
				Scheme: "http",
				Host:   host,
			}
		}
	}
}

// WithHostAndPort sets the Qdrant server host and port.
func WithHostAndPort(host string, port int) Option {
	return func(opts *options) {
		if host != "" && port > 0 {
			opts.qdrantURL = url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", host, port),
			}
		}
	}
}

// WithEmbedder sets the embedder for generating vector embeddings.
func WithEmbedder(embedder embeddings.Embedder) Option {
	return func(opts *options) {
		opts.embedder = embedder
	}
}

// WithAPIKey sets the API key for Qdrant authentication.
func WithAPIKey(apiKey string) Option {
	return func(opts *options) {
		opts.apiKey = strings.TrimSpace(apiKey)
	}
}

// WithContentKey sets the key used to store document content in Qdrant payload.
func WithContentKey(contentKey string) Option {
	return func(opts *options) {
		if contentKey != "" {
			opts.contentKey = strings.TrimSpace(contentKey)
		}
	}
}

// WithTLS enables or disables TLS for the Qdrant connection.
func WithTLS(useTLS bool) Option {
	return func(opts *options) {
		opts.useTLS = useTLS
		// Update URL scheme if already set
		if opts.qdrantURL.Host != "" {
			if useTLS {
				opts.qdrantURL.Scheme = "https"
			} else {
				opts.qdrantURL.Scheme = "http"
			}
		}
	}
}

// WithTimeout sets the connection timeout in seconds.
func WithTimeout(timeoutSeconds int) Option {
	return func(opts *options) {
		if timeoutSeconds > 0 {
			opts.timeout = timeoutSeconds
		}
	}
}

// WithRetryAttempts sets the number of retry attempts for failed operations.
func WithRetryAttempts(attempts int) Option {
	return func(opts *options) {
		if attempts >= 0 {
			opts.retryAttempts = attempts
		}
	}
}

// WithBatchSize sets the batch size for bulk operations.
func WithBatchSize(size int) Option {
	return func(opts *options) {
		if size > 0 {
			opts.batchSize = size
		}
	}
}

// applyDefaults sets default values for options that weren't explicitly configured.
func applyDefaults(opts *options) {
	if opts.logger == nil {
		opts.logger = slog.Default()
	}

	if opts.contentKey == "" {
		opts.contentKey = defaultContentKey
	}

	if opts.timeout == 0 {
		opts.timeout = 30 // 30 seconds default
	}

	if opts.retryAttempts == 0 {
		opts.retryAttempts = 3
	}

	if opts.batchSize == 0 {
		opts.batchSize = 100
	}

	// Set default URL if not provided
	if opts.qdrantURL.Host == "" {
		scheme := "http"
		if opts.useTLS {
			scheme = "https"
		}
		opts.qdrantURL = url.URL{
			Scheme: scheme,
			Host:   fmt.Sprintf("%s:%d", defaultHost, defaultPort),
		}
	}
}

// validate checks if the options are valid and returns an error if not.
func (opts *options) validate() error {
	if strings.TrimSpace(opts.collectionName) == "" {
		return errors.New("collection name is required")
	}

	if opts.timeout <= 0 {
		return errors.New("timeout must be positive")
	}

	if opts.retryAttempts < 0 {
		return errors.New("retry attempts cannot be negative")
	}

	if opts.batchSize <= 0 {
		return errors.New("batch size must be positive")
	}

	// Validate URL if provided
	if opts.qdrantURL.Host != "" {
		if opts.qdrantURL.Scheme != "http" && opts.qdrantURL.Scheme != "https" {
			return errors.New("URL scheme must be http or https")
		}
	}

	return nil
}

// parseOptions processes the provided options and returns a configured options struct.
func parseOptions(opts ...Option) (options, error) {
	o := options{}

	// Apply all provided options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	// Apply defaults for unset values
	applyDefaults(&o)

	// Validate the final configuration
	if err := o.validate(); err != nil {
		return o, err
	}

	return o, nil
}

// String returns a string representation of the options (excluding sensitive data).
func (opts *options) String() string {
	var parts []string

	parts = append(parts, "collection="+opts.collectionName)
	parts = append(parts, "host="+opts.qdrantURL.Host)
	parts = append(parts, "content_key="+opts.contentKey)

	if opts.apiKey != "" {
		parts = append(parts, "has_api_key=true")
	}

	if opts.embedder != nil {
		parts = append(parts, "has_embedder=true")
	}

	return "QdrantOptions{" + strings.Join(parts, ", ") + "}"
}

// Clone creates a copy of the options.
func (opts *options) Clone() options {
	return options{
		collectionName: opts.collectionName,
		qdrantURL:      opts.qdrantURL,
		embedder:       opts.embedder,
		apiKey:         opts.apiKey,
		contentKey:     opts.contentKey,
		logger:         opts.logger,
		useTLS:         opts.useTLS,
		timeout:        opts.timeout,
		retryAttempts:  opts.retryAttempts,
		batchSize:      opts.batchSize,
	}
}
