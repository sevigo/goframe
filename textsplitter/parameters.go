package textsplitter

import (
	"context"
	"path/filepath"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// calculateEffectiveParameters determines the actual parameters to use for chunking.
func (c *CodeAwareTextSplitter) calculateEffectiveParameters(ctx context.Context, opts *model.CodeChunkingOptions, filePath string, contentLength int, modelName string) chunkingParameters {
	params := chunkingParameters{
		ChunkSize:     c.chunkSize,
		OverlapTokens: 0,
		MinChunkSize:  c.minChunkSize,
	}

	// Determine base chunk size in tokens
	if opts != nil && opts.ChunkSize > 0 {
		params.ChunkSize = opts.ChunkSize
	} else {
		params.ChunkSize = c.GetRecommendedChunkSize(ctx, filePath, modelName, contentLength)
	}

	// Determine overlap in tokens
	if opts != nil && opts.OverlapTokens > 0 {
		params.OverlapTokens = opts.OverlapTokens
	} else if modelName != "" {
		params.OverlapTokens = c.tokenizer.GetOptimalOverlapTokens(ctx, modelName)
	} else {
		params.OverlapTokens = int(float64(params.ChunkSize) * defaultOverlapRatio)
	}

	// Ensure overlap is reasonable
	if params.OverlapTokens >= params.ChunkSize {
		params.OverlapTokens = params.ChunkSize / 4
	}

	// Derive line and character limits from token-based chunk size
	params.MinCharsPerChunk = c.calculateMinCharsPerChunk(params.ChunkSize, c.estimationRatio)
	params.MaxLinesPerChunk = c.calculateMaxLinesPerChunk(params.ChunkSize, c.estimationRatio)

	return params
}

func (c *CodeAwareTextSplitter) calculateMinCharsPerChunk(chunkSize int, estimationRatio float64) int {
	minChars := int(float64(chunkSize) * estimationRatio * 0.25)
	if minChars < defaultMinChunkSize {
		minChars = defaultMinChunkSize
	}
	return minChars
}

func (c *CodeAwareTextSplitter) calculateMaxLinesPerChunk(chunkSize int, estimationRatio float64) int {
	maxLines := int(float64(chunkSize) / (estimationRatio * 2))
	if maxLines < 5 {
		maxLines = 5
	}
	if maxLines > 200 {
		maxLines = 200
	}
	return maxLines
}

// GetRecommendedChunkSize returns a recommended chunk size based on file type and content length.
func (c *CodeAwareTextSplitter) GetRecommendedChunkSize(ctx context.Context, filePath, modelName string, contentLength int) int {
	if modelName != "" {
		if rec := c.tokenizer.GetRecommendedChunkSize(ctx, modelName); rec > 0 {
			return rec
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if rec := c.getLanguageSpecificRecommendation(ext, contentLength); rec > 0 {
		return rec
	}

	return c.getContentSizeBasedRecommendation(contentLength)
}

func (c *CodeAwareTextSplitter) getLanguageSpecificRecommendation(ext string, contentLength int) int {
	var baseSize int

	switch ext {
	case ".go", ".java", ".cs", ".cpp", ".cc", ".cxx":
		baseSize = 150
	case ".js", ".ts", ".py", ".rb":
		baseSize = 120
	case ".c", ".h":
		baseSize = 100
	case ".md", ".txt", ".rst":
		baseSize = 80
	case ".json", ".xml", ".yaml", ".yml":
		baseSize = 50
	default:
		return 0
	}

	// Adjust for large files
	if contentLength > 40000 {
		return baseSize + 50
	}

	return baseSize
}

func (c *CodeAwareTextSplitter) getContentSizeBasedRecommendation(contentLength int) int {
	switch {
	case contentLength > 80000:
		return 300
	case contentLength > 20000:
		return 200
	default:
		return c.chunkSize
	}
}
