package textsplitter

// options holds configuration settings for the text splitter.
type options struct {
	chunkSize       int
	chunkOverlap    int
	minChunkSize    int
	maxChunkSize    int
	modelName       string
	estimationRatio float64
	parentConfig    ParentContextConfig
}

// Option is a function type for configuring the splitter.
type Option func(*options)

// WithChunkSize sets the target chunk size in tokens.
func WithChunkSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.chunkSize = size
		}
	}
}

// WithChunkOverlap sets the number of overlapping tokens between chunks.
func WithChunkOverlap(overlap int) Option {
	return func(o *options) {
		if overlap >= 0 {
			o.chunkOverlap = overlap
		}
	}
}

// WithModelName sets the model name for token-aware splitting.
// When set, the splitter uses the model's tokenizer for accurate chunk sizing.
func WithModelName(name string) Option {
	return func(o *options) {
		o.modelName = name
	}
}

// WithMinChunkSize sets the minimum number of characters for a chunk to be valid.
// Chunks smaller than this may be merged with adjacent content.
func WithMinChunkSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.minChunkSize = size
		}
	}
}

// WithMaxChunkSize sets the maximum chunk size in tokens.
// Chunks larger than this will be split further.
func WithMaxChunkSize(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.maxChunkSize = size
		}
	}
}

// WithEstimationRatio sets the character-to-token estimation ratio.
// Used when a tokenizer is not available. Default is 4.0 (4 chars per token).
func WithEstimationRatio(ratio float64) Option {
	return func(o *options) {
		if ratio > 0 {
			o.estimationRatio = ratio
		}
	}
}

// WithParentContextConfig sets the parent context configuration.
// When enabled, chunks include context from their parent code structure.
func WithParentContextConfig(config ParentContextConfig) Option {
	return func(o *options) {
		o.parentConfig = config
	}
}
