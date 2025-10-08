package textsplitter

import "errors"

// ChunkType represents the type of content in a chunk
type ChunkType string

const (
	ChunkTypeFunction ChunkType = "function"
	ChunkTypeClass    ChunkType = "class"
	ChunkTypeImports  ChunkType = "imports"
	ChunkTypeComment  ChunkType = "comment"
	ChunkTypeCode     ChunkType = "code"
	ChunkTypeText     ChunkType = "text"
)

// Constants for chunking parameters
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

var (
	ErrInvalidChunkSize       = errors.New("invalid chunk size")
	ErrEmptyContent           = errors.New("content is empty or contains only whitespace")
	ErrTokenizerNotConfigured = errors.New("tokenizer service is not configured")
	ErrModelRequired          = errors.New("model name is required")
)

// chunkingParameters holds the effective parameters for chunking
type chunkingParameters struct {
	ChunkSize        int
	OverlapTokens    int
	MinChunkSize     int
	MaxLinesPerChunk int
	MinCharsPerChunk int
}
