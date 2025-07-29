package embeddings

type options struct {
	StripNewLines  bool
	BatchSize      int
	QueryPrefix    string
	DocumentPrefix string
}

type Option func(*options)

func WithBatchSize(size int) Option {
	return func(opts *options) {
		opts.BatchSize = size
	}
}

func WithStripNewLines(strip bool) Option {
	return func(opts *options) {
		opts.StripNewLines = strip
	}
}

func WithQueryPrefix(prefix string) Option {
	return func(opts *options) {
		opts.QueryPrefix = prefix
	}
}

func WithDocumentPrefix(prefix string) Option {
	return func(opts *options) {
		opts.DocumentPrefix = prefix
	}
}
