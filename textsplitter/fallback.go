package textsplitter

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// intelligentFallbackChunk implements smart line-based chunking.
func (c *CodeAwareTextSplitter) intelligentFallbackChunk(
	ctx context.Context,
	content, _ string,
	params chunkingParameters,
	modelName string,
) ([]schema.CodeChunk, error) {
	if !c.hasSignificantContent(content) {
		return nil, fmt.Errorf("%w: content lacks significant characters", ErrEmptyContent)
	}

	lines := strings.Split(content, "\n")

	if c.isShortContent(content, lines) {
		if len(strings.TrimSpace(content)) >= params.MinCharsPerChunk {
			chunk := c.createSingleChunk(ctx, content, len(lines), "fallback_single", modelName)
			if c.isValidChunk(chunk) {
				return []schema.CodeChunk{chunk}, nil
			}
		}
		return nil, fmt.Errorf("%w: content too short after validation", ErrEmptyContent)
	}

	return c.chunkWithOverlap(ctx, lines, params, "fallback", modelName)
}

func (c *CodeAwareTextSplitter) isShortContent(content string, lines []string) bool {
	return len(lines) <= shortContentLineThreshold ||
		len(strings.TrimSpace(content)) < shortContentCharThreshold
}

func (c *CodeAwareTextSplitter) createSingleChunk(
	ctx context.Context,
	content string,
	lineCount int,
	chunkerType,
	modelName string,
) schema.CodeChunk {
	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   1,
		LineEnd:     lineCount,
		Type:        string(ChunkTypeText),
		Identifier:  "",
		Annotations: map[string]string{"chunker": chunkerType},
	}

	if modelName != "" {
		chunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, chunk.Content)
	} else {
		chunk.TokenCount = len(chunk.Content) / 4
	}

	return chunk
}

func (c *CodeAwareTextSplitter) chunkWithOverlap(
	ctx context.Context,
	lines []string,
	params chunkingParameters,
	chunkerType, modelName string,
) ([]schema.CodeChunk, error) {
	var chunks []schema.CodeChunk

	effectiveLinesPerChunk := params.MaxLinesPerChunk
	if effectiveLinesPerChunk == 0 {
		effectiveLinesPerChunk = max(5, int(float64(params.ChunkSize)/defaultEstimationRatio))
	}

	linesOverlap := int(float64(params.OverlapTokens) / defaultEstimationRatio)
	if linesOverlap >= effectiveLinesPerChunk {
		linesOverlap = effectiveLinesPerChunk / 4
	}
	if linesOverlap < 1 {
		linesOverlap = 1
	}

	step := effectiveLinesPerChunk - linesOverlap
	if step <= 0 {
		step = 1
	}

	for i := 0; i < len(lines); i += step {
		endLineIndex := min(i+effectiveLinesPerChunk, len(lines))
		chunkContent := strings.Join(lines[i:endLineIndex], "\n")
		trimmedContent := strings.TrimSpace(chunkContent)

		if len(trimmedContent) >= params.MinCharsPerChunk && c.hasSignificantContent(trimmedContent) {
			chunk := c.createChunkWithOverlap(ctx, chunkContent, i, endLineIndex, chunkerType, len(chunks), params.OverlapTokens > 0, modelName)
			if c.isValidChunk(chunk) {
				chunks = append(chunks, chunk)
			}
		}

		if endLineIndex == len(lines) {
			break
		}
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("%w: no valid chunks could be created", ErrEmptyContent)
	}

	return chunks, nil
}

func (c *CodeAwareTextSplitter) createChunkWithOverlap(ctx context.Context, content string, startLineIndex, endLineIndex int, chunkerType string, chunkIndex int, hasOverlap bool, modelName string) schema.CodeChunk {
	chunk := schema.CodeChunk{
		Content:    content,
		LineStart:  startLineIndex + 1,
		LineEnd:    endLineIndex,
		Type:       c.detectChunkType(content),
		Identifier: "",
		Annotations: map[string]string{
			"chunker":     chunkerType,
			"chunk_index": strconv.Itoa(chunkIndex),
			"has_overlap": strconv.FormatBool(hasOverlap),
		},
	}

	if modelName != "" {
		chunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, chunk.Content)
	} else {
		chunk.TokenCount = len(chunk.Content) / 4
	}

	return chunk
}

func (c *CodeAwareTextSplitter) detectChunkType(content string) string {
	lowerContent := strings.ToLower(content)

	for _, pattern := range contentPatterns {
		for _, p := range pattern.patterns {
			if strings.Contains(lowerContent, p) {
				return string(pattern.chunkType)
			}
		}
	}

	if c.isCommentDominatedContent(content) {
		return string(ChunkTypeComment)
	}

	return string(ChunkTypeCode)
}
