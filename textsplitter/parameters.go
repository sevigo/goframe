package textsplitter

import (
	"context"
	"path/filepath"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// calculateEffectiveParameters determines the actual parameters to use for chunking.
func (c *CodeAwareTextSplitter) calculateEffectiveParameters(
	ctx context.Context,
	opts *model.CodeChunkingOptions,
	filePath string,
	contentLength int,
	modelName string,
) chunkingParameters {
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

	switch {
	case opts != nil && opts.OverlapTokens > 0:
		params.OverlapTokens = opts.OverlapTokens
	case modelName != "":
		params.OverlapTokens = c.tokenizer.GetOptimalOverlapTokens(ctx, modelName)
	default:
		params.OverlapTokens = int(float64(params.ChunkSize) * defaultOverlapRatio)
	}

	// Ensure overlap is reasonable
	if params.OverlapTokens >= params.ChunkSize {
		params.OverlapTokens = params.ChunkSize / 4
	}

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
		baseSize = 1024
	case ".js", ".ts", ".py", ".rb":
		baseSize = 768
	case ".c", ".h":
		baseSize = 512
	case ".md", ".txt", ".rst":
		baseSize = 1024
	case ".json", ".xml", ".yaml", ".yml":
		baseSize = 512
	default:
		return 0
	}

	if contentLength > 200000 { // ~200KB
		return baseSize * 2
	}
	if contentLength > 50000 { // ~50KB
		return int(float64(baseSize) * 1.5)
	}

	return baseSize
}

func (c *CodeAwareTextSplitter) getContentSizeBasedRecommendation(contentLength int) int {
	switch {
	case contentLength > 200000: // ~200KB
		return 4096
	case contentLength > 50000: // ~50KB
		return 2048
	default:
		return c.chunkSize
	}
}
