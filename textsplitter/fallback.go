package textsplitter

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// intelligentFallbackChunk now uses a proper recursive text splitter.
func (c *CodeAwareTextSplitter) intelligentFallbackChunk(
	ctx context.Context,
	content, path string,
	params chunkingParameters,
	modelName string,
) ([]schema.CodeChunk, error) {
	if !c.hasSignificantContent(content) {
		return nil, fmt.Errorf("%w: content lacks significant characters", ErrEmptyContent)
	}

	splitter := NewRecursiveCharacter(
		WithChunkSize(params.ChunkSize),
		WithChunkOverlap(params.OverlapTokens),
	)

	splitContents, err := splitter.SplitText(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("fallback splitting failed: %w", err)
	}

	if len(splitContents) == 0 {
		return nil, fmt.Errorf("%w: fallback splitter produced no chunks", ErrEmptyContent)
	}

	chunks := make([]schema.CodeChunk, len(splitContents))
	totalLines := len(strings.Split(content, "\n"))

	for i, text := range splitContents {
		chunk := schema.CodeChunk{
			Content:    text,
			LineStart:  1,
			LineEnd:    totalLines,
			Type:       "text_fallback",
			Identifier: fmt.Sprintf("%s_part_%d", path, i),
			Annotations: map[string]string{
				"chunker":     "recursive_character_fallback",
				"has_overlap": strconv.FormatBool(params.OverlapTokens > 0),
			},
		}
		if modelName != "" {
			chunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, chunk.Content)
		}
		chunks[i] = chunk
	}

	return chunks, nil
}
