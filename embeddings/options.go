package embeddings

type options struct {
	StripNewLines bool
	BatchSize     int
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
