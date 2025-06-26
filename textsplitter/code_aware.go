package textsplitter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

// CodeAwareTextSplitter implements intelligent chunking using language-specific parsers.
type CodeAwareTextSplitter struct {
	tokenizer      Tokenizer
	parserRegistry parsers.ParserRegistry
	logger         *slog.Logger

	chunkSize       int
	chunkOverlap    int
	minChunkSize    int
	maxChunkSize    int
	modelName       string
	estimationRatio float64
}

var _ TextSplitter = (*CodeAwareTextSplitter)(nil)

// NewCodeAware creates the splitter.
func NewCodeAware(
	registry parsers.ParserRegistry,
	tokenizer Tokenizer,
	logger *slog.Logger,
	opts ...Option,
) (*CodeAwareTextSplitter, error) {
	if registry == nil {
		return nil, errors.New("parser registry cannot be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Apply functional options for configuration
	splitterOpts := options{
		chunkSize:       1024,
		chunkOverlap:    100,
		minChunkSize:    25,
		maxChunkSize:    16000,
		estimationRatio: 4.0,
	}
	for _, opt := range opts {
		opt(&splitterOpts)
	}

	return &CodeAwareTextSplitter{
		parserRegistry: registry,
		tokenizer:      tokenizer,
		logger:         logger.With("component", "code_aware_splitter"),
		chunkSize:      splitterOpts.chunkSize,
		chunkOverlap:   splitterOpts.chunkOverlap,
		modelName:      splitterOpts.modelName,
		minChunkSize:   splitterOpts.minChunkSize,
		maxChunkSize:   splitterOpts.maxChunkSize,
	}, nil
}

// SplitDocuments is the primary public method.
func (c *CodeAwareTextSplitter) SplitDocuments(ctx context.Context, docs []schema.Document) ([]schema.Document, error) {
	finalDocs := make([]schema.Document, 0)
	for _, doc := range docs {
		// Use a private helper to do the actual work for each document.
		chunks, err := c.splitSingleDocument(ctx, doc)
		if err != nil {
			c.logger.WarnContext(ctx, "Could not split document, using original.", "source", doc.Metadata["source"], "error", err)
			finalDocs = append(finalDocs, doc)
			continue
		}
		finalDocs = append(finalDocs, chunks...)
	}
	return finalDocs, nil
}

// splitSingleDocument contains the core logic for processing one document.
func (c *CodeAwareTextSplitter) splitSingleDocument(ctx context.Context, doc schema.Document) ([]schema.Document, error) {
	source, ok := doc.Metadata["source"].(string)
	if !ok {
		return nil, errors.New("document metadata is missing 'source' key")
	}

	codeChunks, err := c.chunkContent(ctx, doc.PageContent, source, nil)
	if err != nil {
		return nil, err
	}

	// Convert the specific CodeChunks into generic schema.Documents
	splitDocs := make([]schema.Document, 0, len(codeChunks))
	for _, chunk := range codeChunks {
		newMetadata := make(map[string]any)
		for k, v := range doc.Metadata { // Inherit metadata
			newMetadata[k] = v
		}
		newMetadata["line_start"] = chunk.LineStart
		newMetadata["line_end"] = chunk.LineEnd
		for k, v := range chunk.Annotations {
			newMetadata[k] = v
		}
		splitDocs = append(splitDocs, schema.NewDocument(chunk.Content, newMetadata))
	}
	return splitDocs, nil
}

func (c *CodeAwareTextSplitter) chunkContent(ctx context.Context, content, filePath string, fileInfo fs.FileInfo) ([]schema.CodeChunk, error) {
	plugin, err := c.parserRegistry.GetParserForFile(filePath, fileInfo)
	if err != nil {
		c.logger.DebugContext(ctx, "No specific parser found, using fallback.", "file", filePath)
		return nil, err
	}

	// Construct the options for the plugin
	// This logic can be enhanced later. For now, it's a simple mapping.
	opts := &schema.CodeChunkingOptions{
		ChunkSize: c.chunkSize,
	}

	// Use the plugin's sophisticated chunking method
	return plugin.Chunk(content, filePath, opts)
}

// ChunkFileWithFileInfo chunks content with file info for enhanced language detection.
func (c *CodeAwareTextSplitter) ChunkFileWithFileInfo(
	ctx context.Context,
	content, filePath, modelName string,
	fileInfo fs.FileInfo,
	opts *schema.CodeChunkingOptions,
) ([]schema.CodeChunk, error) {
	if err := c.ValidateChunkingOptions(opts); err != nil {
		return nil, fmt.Errorf("invalid chunking options: %w", err)
	}

	if err := c.validateContent(content, filePath); err != nil {
		return nil, err
	}

	params := c.calculateEffectiveParameters(ctx, opts, filePath, len(content), modelName)
	pluginOpts := c.createPluginOptions(opts, params)

	// Try language-specific chunking first
	if chunks, err := c.tryLanguageSpecificChunking(ctx, content, filePath, fileInfo, pluginOpts, modelName); err == nil && len(chunks) > 0 {
		validChunks := c.postProcessChunks(ctx, chunks, params, modelName)
		if len(validChunks) > 0 {
			return validChunks, nil
		}
	}

	// Fall back to intelligent generic chunking
	return c.intelligentFallbackChunk(ctx, content, filePath, params, modelName)
}

func (c *CodeAwareTextSplitter) createPluginOptions(opts *schema.CodeChunkingOptions, params chunkingParameters) *schema.CodeChunkingOptions {
	pluginOpts := &schema.CodeChunkingOptions{
		ChunkSize:        params.ChunkSize,
		OverlapTokens:    params.OverlapTokens,
		MaxLinesPerChunk: params.MaxLinesPerChunk,
		MinCharsPerChunk: params.MinCharsPerChunk,
	}

	if opts != nil {
		pluginOpts.PreserveStructure = opts.PreserveStructure
		pluginOpts.LanguageHints = opts.LanguageHints
	}

	return pluginOpts
}

func (c *CodeAwareTextSplitter) validateContent(content, filePath string) error {
	trimmedContent := strings.TrimSpace(content)
	if len(trimmedContent) == 0 {
		return fmt.Errorf("%w: file %s", ErrEmptyContent, filePath)
	}

	if !c.hasSignificantContent(trimmedContent) {
		return fmt.Errorf("%w: content lacks significant characters in file %s", ErrEmptyContent, filePath)
	}

	return nil
}

func (c *CodeAwareTextSplitter) isCommentDominatedContent(content string) bool {
	commentLines, totalLines := 0, 0

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 {
			totalLines++
			for _, prefix := range commentPrefixes {
				if strings.HasPrefix(trimmed, prefix) {
					commentLines++
					break
				}
			}
		}
	}

	if totalLines == 0 {
		return false
	}

	return float64(commentLines)/float64(totalLines) > commentRatioThreshold
}
