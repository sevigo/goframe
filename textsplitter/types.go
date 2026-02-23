package textsplitter

import "errors"

// ChunkType represents the type of content in a chunk.
type ChunkType string

// Chunk type constants.
const (
	// ChunkTypeFunction represents a function or method chunk.
	ChunkTypeFunction ChunkType = "function"
	// ChunkTypeClass represents a class or struct chunk.
	ChunkTypeClass ChunkType = "class"
	// ChunkTypeImports represents an import block chunk.
	ChunkTypeImports ChunkType = "imports"
	// ChunkTypeComment represents a comment block chunk.
	ChunkTypeComment ChunkType = "comment"
	// ChunkTypeCode represents a generic code block chunk.
	ChunkTypeCode ChunkType = "code"
	// ChunkTypeText represents a text block chunk.
	ChunkTypeText ChunkType = "text"
)

// Constants for chunking parameters.
const (
	defaultFallbackChunkSize = 50
	defaultMinChunkSize      = 20
	defaultOverlapRatio      = 0.1
	maxChunkSize             = 16000
	defaultEstimationRatio   = 4.0

	// Content analysis thresholds
	shortContentLineThreshold = 5
	shortContentCharThreshold = 200
	minContentThreshold       = 10
	minSignificanceRatio      = 0.25
	minSignificantChars       = 3
	commentRatioThreshold     = 0.5
)

// Error variables for text splitting.
var (
	// ErrInvalidChunkSize is returned when the chunk size is invalid.
	ErrInvalidChunkSize = errors.New("invalid chunk size")
	// ErrEmptyContent is returned when the content is empty or whitespace only.
	ErrEmptyContent = errors.New("content is empty or contains only whitespace")
	// ErrTokenizerNotConfigured is returned when a tokenizer is required but not configured.
	ErrTokenizerNotConfigured = errors.New("tokenizer service is not configured")
	// ErrModelRequired is returned when a model name is required but not provided.
	ErrModelRequired = errors.New("model name is required")
)

// chunkingParameters holds the effective parameters for chunking.
type chunkingParameters struct {
	ChunkSize        int
	OverlapTokens    int
	MinChunkSize     int
	MaxLinesPerChunk int
	MinCharsPerChunk int
}
