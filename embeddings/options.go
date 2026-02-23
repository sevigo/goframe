package embeddings

// options configures embedding behavior.
type options struct {
	// StripNewLines removes newlines from text before embedding.
	StripNewLines bool
	// BatchSize is the number of texts to embed in a single batch.
	BatchSize int
	// QueryPrefix is prepended to query texts before embedding.
	QueryPrefix string
	// DocumentPrefix is prepended to document texts before embedding.
	DocumentPrefix string
}

// Option configures an Embedder.
type Option func(*options)

// WithBatchSize sets the batch size for embedding operations.
func WithBatchSize(size int) Option {
	return func(opts *options) {
		opts.BatchSize = size
	}
}

// WithStripNewLines sets whether to strip newlines from text before embedding.
func WithStripNewLines(strip bool) Option {
	return func(opts *options) {
		opts.StripNewLines = strip
	}
}

// WithQueryPrefix sets the prefix prepended to query texts.
// Some embedding models perform better with prefixed queries.
func WithQueryPrefix(prefix string) Option {
	return func(opts *options) {
		opts.QueryPrefix = prefix
	}
}

// WithDocumentPrefix sets the prefix prepended to document texts.
// Some embedding models perform better with prefixed documents.
func WithDocumentPrefix(prefix string) Option {
	return func(opts *options) {
		opts.DocumentPrefix = prefix
	}
}
