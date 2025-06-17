package textsplitter

// options holds configuration settings for the text splitter.
type options struct {
	chunkSize       int
	chunkOverlap    int
	minChunkSize    int
	maxChunkSize    int
	modelName       string
	estimationRatio float64
}

// Option is a function type for configuring the splitter.
type Option func(*options)

// WithChunkSize sets the target chunk size.
func WithChunkSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.chunkSize = size
		}
	}
}

// WithChunkOverlap sets the chunk overlap.
func WithChunkOverlap(overlap int) Option {
	return func(o *options) {
		if overlap >= 0 {
			o.chunkOverlap = overlap
		}
	}
}

// WithModelName sets the model name for token-aware splitting.
func WithModelName(name string) Option {
	return func(o *options) {
		o.modelName = name
	}
}

// WithMinChunkSize sets the minimum number of characters for a chunk to be valid.
func WithMinChunkSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.minChunkSize = size
		}
	}
}

func WithMaxChunkSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.maxChunkSize = size
		}
	}
}

// WithEstimationRatio sets the character-to-token estimation ratio.
func WithEstimationRatio(ratio float64) Option { // <<< ADD THIS FUNCTION
	return func(o *options) {
		if ratio > 0 {
			o.estimationRatio = ratio
		}
	}
}
