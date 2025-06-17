package textsplitter

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"maps"

	model "github.com/sevigo/goframe/schema"
)

// tryLanguageSpecificChunking attempts to use a language plugin for chunking.
func (c *CodeAwareTextSplitter) tryLanguageSpecificChunking(
	ctx context.Context,
	content, filePath string,
	fileInfo fs.FileInfo,
	pluginOpts *model.CodeChunkingOptions,
	modelName string,
) ([]model.CodeChunk, error) {
	plugin, err := c.parserRegistry.GetParserForFile(filePath, fileInfo)
	if err != nil {
		return nil, fmt.Errorf("no language plugin available: %w", err)
	}

	chunks, err := plugin.Chunk(content, filePath, pluginOpts)
	if err != nil {
		return nil, fmt.Errorf("language plugin chunking failed: %w", err)
	}

	return c.enrichChunksWithTokenCounts(ctx, chunks, modelName), nil
}

func (c *CodeAwareTextSplitter) enrichChunksWithTokenCounts(
	ctx context.Context,
	chunks []model.CodeChunk,
	modelName string,
) []model.CodeChunk {
	enrichedChunks := make([]model.CodeChunk, 0, len(chunks))

	for _, chunk := range chunks {
		if chunk.TokenCount <= 0 {
			if modelName != "" {
				chunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, chunk.Content)
			} else {
				chunk.TokenCount = len(chunk.Content) / 4 // Fallback estimate
			}
		}

		if c.isValidChunk(chunk) {
			enrichedChunks = append(enrichedChunks, chunk)
		}
	}

	return enrichedChunks
}

// postProcessChunks applies post-processing to chunks from language plugins.
func (c *CodeAwareTextSplitter) postProcessChunks(ctx context.Context, chunks []model.CodeChunk, params chunkingParameters, modelName string) []model.CodeChunk {
	var processedChunks []model.CodeChunk

	for _, chunk := range chunks {
		if !c.isValidChunk(chunk) {
			continue
		}

		// Split oversized chunks
		if chunk.TokenCount > params.ChunkSize*2 {
			subChunks := c.splitOversizedChunk(ctx, chunk, params, modelName)
			for _, subChunk := range subChunks {
				if c.isValidChunk(subChunk) {
					processedChunks = append(processedChunks, subChunk)
				}
			}
		} else {
			processedChunks = append(processedChunks, chunk)
		}
	}

	return processedChunks
}

// splitOversizedChunk splits a chunk that's too large into smaller pieces.
func (c *CodeAwareTextSplitter) splitOversizedChunk(ctx context.Context, chunk model.CodeChunk, params chunkingParameters, modelName string) []model.CodeChunk {
	// Try token-based splitting first
	if modelName != "" {
		if subChunks := c.tryTokenBasedSplit(ctx, chunk, params, modelName); len(subChunks) > 1 {
			return subChunks
		}
	}

	// Fall back to line-based splitting
	return c.lineBasedSplit(ctx, chunk, params, modelName)
}

func (c *CodeAwareTextSplitter) tryTokenBasedSplit(ctx context.Context, chunk model.CodeChunk, params chunkingParameters, modelName string) []model.CodeChunk {
	splitByTokens, err := c.tokenizer.SplitTextByTokens(ctx, modelName, chunk.Content, params.ChunkSize)
	if err != nil || len(splitByTokens) <= 1 {
		return nil
	}

	var newChunks []model.CodeChunk
	for i, subContent := range splitByTokens {
		subChunk := c.createSubChunk(chunk, subContent, 0, 0, i)
		subChunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, subChunk.Content)
		newChunks = append(newChunks, subChunk)
	}

	return newChunks
}

func (c *CodeAwareTextSplitter) lineBasedSplit(ctx context.Context, chunk model.CodeChunk, params chunkingParameters, modelName string) []model.CodeChunk {
	lines := strings.Split(chunk.Content, "\n")
	if len(lines) <= 1 {
		return []model.CodeChunk{chunk}
	}

	var subChunks []model.CodeChunk
	linesPerSubChunk := params.MaxLinesPerChunk

	for i := 0; i < len(lines); i += linesPerSubChunk {
		end := i + linesPerSubChunk
		if end > len(lines) {
			end = len(lines)
		}

		subContent := strings.Join(lines[i:end], "\n")
		if len(strings.TrimSpace(subContent)) >= params.MinCharsPerChunk {
			subChunk := c.createSubChunk(chunk, subContent, chunk.LineStart+i, chunk.LineStart+end-1, len(subChunks))
			if modelName != "" {
				subChunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, subChunk.Content)
			} else {
				subChunk.TokenCount = len(subChunk.Content) / 4
			}
			subChunks = append(subChunks, subChunk)
		}
	}

	if len(subChunks) == 0 {
		return []model.CodeChunk{chunk}
	}

	return subChunks
}

// createSubChunk creates a new CodeChunk from a part of an original chunk.
func (c *CodeAwareTextSplitter) createSubChunk(originalChunk model.CodeChunk, content string, startLine, endLine int, chunkIndex int) model.CodeChunk {
	annotations := make(map[string]string, len(originalChunk.Annotations))
	maps.Copy(annotations, originalChunk.Annotations)

	return model.CodeChunk{
		Content:     content,
		LineStart:   startLine,
		LineEnd:     endLine,
		Type:        originalChunk.Type,
		Identifier:  fmt.Sprintf("%s_part_%d", originalChunk.Identifier, chunkIndex+1),
		Annotations: annotations,
	}
}
