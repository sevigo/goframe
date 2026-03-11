package textsplitter

import (
	"context"
	"fmt"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// EnrichChunkWithContext adds file and hierarchical context to a chunk.
func (c *CodeAwareTextSplitter) EnrichChunkWithContext(
	ctx context.Context,
	chunk model.CodeChunk,
	fileContent string,
	metadata model.FileMetadata,
	parentChunks []model.CodeChunk,
	modelName string,
) model.CodeChunk {
	if modelName == "" {
		enrichedChunk := chunk
		enrichedChunk.EnrichedContent = chunk.Content
		return enrichedChunk
	}

	maxEnrichedTokens := c.tokenizer.GetMaxContextWindow(ctx, modelName)
	if maxEnrichedTokens == 0 {
		maxEnrichedTokens = 4096
	}

	enrichedContent := c.buildEnrichedContent(ctx, chunk, metadata, parentChunks, modelName, maxEnrichedTokens)

	enrichedChunk := chunk
	enrichedChunk.EnrichedContent = enrichedContent
	enrichedChunk.TokenCount = c.tokenizer.CountTokens(ctx, modelName, enrichedContent)

	return enrichedChunk
}

func (c *CodeAwareTextSplitter) buildEnrichedContent(
	ctx context.Context,
	chunk model.CodeChunk,
	metadata model.FileMetadata,
	parentChunks []model.CodeChunk,
	modelName string,
	maxTokens int,
) string {
	mainContentTokens := c.getTokenCount(ctx, modelName, chunk.Content)

	if mainContentTokens >= maxTokens {
		truncatedContent := c.truncateToTokenLimit(ctx, chunk.Content, maxTokens-100, modelName)
		return truncatedContent + "\n// ... (content truncated)"
	}

	remainingTokens := maxTokens - mainContentTokens

	// Collect enrichment parts in priority order: Header > Parent > Overlap > Content
	// But we build them carefully based on remaining tokens.
	var header, parentCtx, overlap string

	// 1. Prepare Header (high priority)
	if remainingTokens > 30 {
		fileHeader := c.buildFileContextHeader(metadata)
		headerTokens := c.getTokenCount(ctx, modelName, fileHeader)
		if headerTokens <= remainingTokens {
			header = fileHeader
			remainingTokens -= headerTokens
		}
	}

	// 2. Prepare Parent Context
	if chunk.ParentContext != "" && remainingTokens > 50 {
		parentCtx, remainingTokens = c.getValidParentContext(ctx, chunk.ParentContext, modelName, remainingTokens)
	}

	// 3. Prepare Strategic Overlap
	if len(parentChunks) > 0 && remainingTokens > 20 {
		overlap, _ = c.getValidStrategicOverlap(ctx, parentChunks, modelName, remainingTokens)
	}

	// Assemble final content: Header -> Parent -> Overlap -> Content
	var parts []string
	if header != "" {
		parts = append(parts, header)
	}
	if parentCtx != "" {
		parts = append(parts, parentCtx)
	}
	if overlap != "" {
		parts = append(parts, overlap)
	}
	parts = append(parts, chunk.Content)

	return strings.Join(parts, "\n")
}

func (c *CodeAwareTextSplitter) getValidParentContext(
	ctx context.Context,
	parentContext, modelName string,
	remainingTokens int,
) (string, int) {
	parentTokens := c.getTokenCount(ctx, modelName, parentContext)

	if parentTokens <= remainingTokens {
		return parentContext, remainingTokens - parentTokens
	}

	truncatedParent := c.truncateToTokenLimit(ctx, parentContext, remainingTokens-10, modelName)
	if len(strings.TrimSpace(truncatedParent)) > 20 {
		finalParent := truncatedParent + "..."
		return finalParent, remainingTokens - c.getTokenCount(ctx, modelName, finalParent)
	}

	return "", remainingTokens
}

func (c *CodeAwareTextSplitter) getValidStrategicOverlap(
	ctx context.Context,
	parentChunks []model.CodeChunk,
	modelName string,
	remainingTokens int,
) (string, int) {
	prevChunk := parentChunks[len(parentChunks)-1]
	overlap := c.calculateStrategicOverlap(prevChunk)

	if overlap == "" {
		return "", remainingTokens
	}

	overlapWithMarkers := "// Previous context:\n" + overlap + "\n// Current section:"
	overlapTokens := c.getTokenCount(ctx, modelName, overlapWithMarkers)

	if overlapTokens <= remainingTokens {
		return overlapWithMarkers, remainingTokens - overlapTokens
	}

	return "", remainingTokens
}

func (c *CodeAwareTextSplitter) getTokenCount(ctx context.Context, modelName, content string) int {
	if tokens := c.tokenizer.CountTokens(ctx, modelName, content); tokens > 0 {
		return tokens
	}
	return c.tokenizer.EstimateTokens(ctx, modelName, content)
}

func (c *CodeAwareTextSplitter) truncateToTokenLimit(ctx context.Context, content string, maxTokens int, modelName string) string {
	if split, err := c.tokenizer.SplitTextByTokens(ctx, modelName, content, maxTokens); err == nil && len(split) > 0 {
		return split[0]
	}

	lines := strings.Split(content, "\n")
	var result strings.Builder

	for _, line := range lines {
		testContent := result.String()
		if result.Len() > 0 {
			testContent += "\n"
		}
		testContent += line

		if c.tokenizer.EstimateTokens(ctx, modelName, testContent) > maxTokens {
			break
		}

		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(line)
	}

	return strings.TrimSpace(result.String())
}

func (c *CodeAwareTextSplitter) buildFileContextHeader(metadata model.FileMetadata) string {
	var header strings.Builder

	fmt.Fprintf(&header, "// File: %s", metadata.FilePath)

	if metadata.Language != "" {
		fmt.Fprintf(&header, " [%s]", metadata.Language)
	}

	if purpose, exists := metadata.Properties["file_purpose"]; exists {
		fmt.Fprintf(&header, " - %s", purpose)
	}

	header.WriteString("\n")

	if len(metadata.Imports) > 0 {
		header.WriteString("// Imports: ")
		imports := metadata.Imports
		if len(imports) > 5 {
			imports = imports[:5]
		}
		header.WriteString(strings.Join(imports, ", "))
		header.WriteString("\n")
	}

	return header.String()
}

func (c *CodeAwareTextSplitter) calculateStrategicOverlap(previous model.CodeChunk) string {
	prevLines := strings.Split(previous.Content, "\n")
	if len(prevLines) < 2 {
		return ""
	}

	meaningfulLines := make([]string, 0, 3)
	for i := len(prevLines) - 1; i >= 0 && len(meaningfulLines) < 3; i-- {
		line := strings.TrimSpace(prevLines[i])
		if c.isMeaningfulLine(line) {
			meaningfulLines = append([]string{prevLines[i]}, meaningfulLines...)
		}
	}

	if len(meaningfulLines) == 0 {
		return ""
	}

	return strings.Join(meaningfulLines, "\n")
}

func (c *CodeAwareTextSplitter) isMeaningfulLine(line string) bool {
	return line != "" &&
		!strings.HasPrefix(line, "}") &&
		!strings.HasPrefix(line, "//") &&
		!strings.HasPrefix(line, "/*")
}
