package qdrant

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/sevigo/goframe/embeddings"
	"google.golang.org/grpc"
)

const (
	defaultContentKey = "content"
	defaultHost       = "localhost"
	defaultPort       = 6334

	// Default gRPC connection settings.
	defaultTimeout          = 30 * time.Second
	defaultKeepaliveTime    = 10 * time.Second
	defaultKeepaliveTimeout = 2 * time.Second
	defaultPoolSize         = 10
)

var ErrInvalidOptions = errors.New("qdrant: invalid options provided")

type options struct {
	collectionName     string
	qdrantURL          url.URL
	embedder           embeddings.Embedder
	apiKey             string
	contentKey         string
	logger             *slog.Logger
	useTLS             bool
	timeout            time.Duration
	retryAttempts      int
	retryDelay         time.Duration
	maxRetryDelay      time.Duration
	retryJitter        time.Duration
	batchSize          int
	batchConfig        *BatchConfig
	binaryQuantization bool
	payloadIndexes     []string
	sparseVectors      []string

	// gRPC connection settings.
	keepaliveTime    time.Duration
	keepaliveTimeout time.Duration
	poolSize         int
	grpcOptions      []grpc.DialOption
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
		if opts.qdrantURL.Host != "" {
			if useTLS {
				opts.qdrantURL.Scheme = "https"
			} else {
				opts.qdrantURL.Scheme = "http"
			}
		}
	}
}

// WithTimeout sets the connection timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(opts *options) {
		if timeout > 0 {
			opts.timeout = timeout
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

// WithKeepaliveTime sets the interval between keepalive pings.
// This helps maintain connection health in long-running scenarios.
func WithKeepaliveTime(d time.Duration) Option {
	return func(opts *options) {
		if d > 0 {
			opts.keepaliveTime = d
		}
	}
}

// WithKeepaliveTimeout sets the timeout for keepalive ping responses.
func WithKeepaliveTimeout(d time.Duration) Option {
	return func(opts *options) {
		if d > 0 {
			opts.keepaliveTimeout = d
		}
	}
}

// WithPoolSize sets the gRPC connection pool size.
// A larger pool can handle more concurrent requests.
func WithPoolSize(size int) Option {
	return func(opts *options) {
		if size > 0 {
			opts.poolSize = size
		}
	}
}

// WithGrpcOptions sets custom gRPC dial options for advanced configuration.
// These are appended to the default dial options.
func WithGrpcOptions(opts ...grpc.DialOption) Option {
	return func(o *options) {
		o.grpcOptions = append(o.grpcOptions, opts...)
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

// WithBinaryQuantization enables binary quantization for the collection.
func WithBinaryQuantization(enabled bool) Option {
	return func(opts *options) {
		opts.binaryQuantization = enabled
	}
}

// WithPayloadIndex specifies keys to be indexed in the payload.
func WithPayloadIndex(keys ...string) Option {
	return func(opts *options) {
		opts.payloadIndexes = append(opts.payloadIndexes, keys...)
	}
}

// WithSparseVector adds a named sparse vector configuration.
func WithSparseVector(name string) Option {
	return func(opts *options) {
		opts.sparseVectors = append(opts.sparseVectors, name)
	}
}

func applyDefaults(opts *options) {
	if opts.logger == nil {
		opts.logger = slog.Default()
	}

	if opts.contentKey == "" {
		opts.contentKey = defaultContentKey
	}

	if opts.timeout == 0 {
		opts.timeout = defaultTimeout
	}

	if opts.retryAttempts == 0 {
		opts.retryAttempts = 3
	}

	if opts.retryDelay == 0 {
		opts.retryDelay = 2 * time.Second
	}

	if opts.maxRetryDelay == 0 {
		opts.maxRetryDelay = 30 * time.Second
	}

	if opts.retryJitter == 0 {
		opts.retryJitter = 1 * time.Second
	}

	if opts.batchSize == 0 {
		opts.batchSize = 100
	}

	if opts.keepaliveTime == 0 {
		opts.keepaliveTime = defaultKeepaliveTime
	}

	if opts.keepaliveTimeout == 0 {
		opts.keepaliveTimeout = defaultKeepaliveTimeout
	}

	if opts.poolSize == 0 {
		opts.poolSize = defaultPoolSize
	}

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

	if opts.qdrantURL.Host != "" {
		if opts.qdrantURL.Scheme != "http" && opts.qdrantURL.Scheme != "https" {
			return errors.New("URL scheme must be http or https")
		}
	}

	return nil
}

func parseOptions(opts ...Option) (options, error) {
	o := options{}

	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	applyDefaults(&o)

	if err := o.validate(); err != nil {
		return o, err
	}

	return o, nil
}

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

func (opts *options) Clone() options {
	return options{
		collectionName:     opts.collectionName,
		qdrantURL:          opts.qdrantURL,
		embedder:           opts.embedder,
		apiKey:             opts.apiKey,
		contentKey:         opts.contentKey,
		logger:             opts.logger,
		useTLS:             opts.useTLS,
		timeout:            opts.timeout,
		retryAttempts:      opts.retryAttempts,
		retryDelay:         opts.retryDelay,
		maxRetryDelay:      opts.maxRetryDelay,
		retryJitter:        opts.retryJitter,
		batchSize:          opts.batchSize,
		batchConfig:        opts.batchConfig,
		binaryQuantization: opts.binaryQuantization,
		payloadIndexes:     append([]string{}, opts.payloadIndexes...),
		sparseVectors:      append([]string{}, opts.sparseVectors...),
		keepaliveTime:      opts.keepaliveTime,
		keepaliveTimeout:   opts.keepaliveTimeout,
		poolSize:           opts.poolSize,
		grpcOptions:        append([]grpc.DialOption{}, opts.grpcOptions...),
	}
}
