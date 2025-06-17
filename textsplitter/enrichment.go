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

	var contentBuilder strings.Builder
	contentBuilder.WriteString(chunk.Content)
	remainingTokens := maxTokens - mainContentTokens

	// Add parent context
	if chunk.ParentContext != "" && remainingTokens > 50 {
		remainingTokens = c.addParentContext(&contentBuilder, chunk.ParentContext, ctx, modelName, remainingTokens)
	}

	// Add file header
	if remainingTokens > 30 {
		fileHeader := c.buildFileContextHeader(metadata)
		remainingTokens = c.addFileHeader(&contentBuilder, fileHeader, ctx, modelName, remainingTokens)
	}

	// Add strategic overlap
	if len(parentChunks) > 0 && remainingTokens > 20 {
		c.addStrategicOverlap(&contentBuilder, parentChunks, ctx, modelName, remainingTokens)
	}

	return contentBuilder.String()
}

func (c *CodeAwareTextSplitter) addParentContext(
	builder *strings.Builder,
	parentContext string,
	ctx context.Context,
	modelName string,
	remainingTokens int,
) int {
	parentTokens := c.getTokenCount(ctx, modelName, parentContext)

	if parentTokens <= remainingTokens {
		*builder = c.prependContent(builder.String(), parentContext)
		return remainingTokens - parentTokens
	}

	// Truncate parent context to fit
	truncatedParent := c.truncateToTokenLimit(ctx, parentContext, remainingTokens-10, modelName)
	if len(strings.TrimSpace(truncatedParent)) > 20 {
		*builder = c.prependContent(builder.String(), truncatedParent+"...")
		return remainingTokens - c.getTokenCount(ctx, modelName, truncatedParent)
	}

	return remainingTokens
}

func (c *CodeAwareTextSplitter) addFileHeader(
	builder *strings.Builder,
	fileHeader string,
	ctx context.Context,
	modelName string,
	remainingTokens int,
) int {
	headerTokens := c.getTokenCount(ctx, modelName, fileHeader)

	if headerTokens <= remainingTokens {
		*builder = c.prependContent(builder.String(), fileHeader)
		return remainingTokens - headerTokens
	}

	return remainingTokens
}

func (c *CodeAwareTextSplitter) addStrategicOverlap(
	builder *strings.Builder,
	parentChunks []model.CodeChunk,
	ctx context.Context,
	modelName string,
	remainingTokens int,
) {
	prevChunk := parentChunks[len(parentChunks)-1]
	overlap := c.calculateStrategicOverlap(prevChunk)

	if overlap == "" {
		return
	}

	overlapWithMarkers := "// Previous context:\n" + overlap + "\n// Current section:\n"
	overlapTokens := c.getTokenCount(ctx, modelName, overlapWithMarkers)

	if overlapTokens <= remainingTokens {
		*builder = c.prependContent(builder.String(), overlapWithMarkers)
	}
}

func (c *CodeAwareTextSplitter) getTokenCount(ctx context.Context, modelName, content string) int {
	if tokens := c.tokenizer.CountTokens(ctx, modelName, content); tokens > 0 {
		return tokens
	}
	return c.tokenizer.EstimateTokens(ctx, modelName, content)
}

func (c *CodeAwareTextSplitter) prependContent(mainContent, contextContent string) strings.Builder {
	var result strings.Builder
	result.WriteString(contextContent)
	if !strings.HasSuffix(contextContent, "\n") {
		result.WriteString("\n")
	}
	result.WriteString(mainContent)
	return result
}

func (c *CodeAwareTextSplitter) truncateToTokenLimit(ctx context.Context, content string, maxTokens int, modelName string) string {
	// Try token-based splitting first
	if split, err := c.tokenizer.SplitTextByTokens(ctx, modelName, content, maxTokens); err == nil && len(split) > 0 {
		return split[0]
	}

	// Fallback to line-based truncation
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

	header.WriteString(fmt.Sprintf("// File: %s", metadata.FilePath))

	if metadata.Language != "" {
		header.WriteString(fmt.Sprintf(" [%s]", metadata.Language))
	}

	if purpose, exists := metadata.Properties["file_purpose"]; exists {
		header.WriteString(fmt.Sprintf(" - %s", purpose))
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
