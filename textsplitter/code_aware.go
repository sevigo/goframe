package textsplitter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"sync"

	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

const (
	// MaxParentTextLength defines the default limit for parent context storage
	MaxParentTextLength = 2000
	// DefaultChunkSize is the fallback size if not provided
	DefaultChunkSize = 2048
)

type ParentContextConfig struct {
	Enabled       bool
	MaxTextLength int
}

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

	parentConfig  ParentContextConfig
	parentIDCache sync.Map // cache for deterministic IDs: key -> hash
}

var _ TextSplitter = (*CodeAwareTextSplitter)(nil)

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

	splitterOpts := options{
		chunkSize:       DefaultChunkSize,
		chunkOverlap:    200,
		minChunkSize:    50,
		maxChunkSize:    16000,
		estimationRatio: 4.0,
	}
	for _, opt := range opts {
		opt(&splitterOpts)
	}

	return &CodeAwareTextSplitter{
		parserRegistry:  registry,
		tokenizer:       tokenizer,
		logger:          logger.With("component", "code_aware_splitter"),
		chunkSize:       splitterOpts.chunkSize,
		chunkOverlap:    splitterOpts.chunkOverlap,
		modelName:       splitterOpts.modelName,
		minChunkSize:    splitterOpts.minChunkSize,
		maxChunkSize:    splitterOpts.maxChunkSize,
		estimationRatio: splitterOpts.estimationRatio,
		parentConfig:    splitterOpts.parentConfig,
	}, nil
}

// SplitDocuments takes a slice of documents and returns a new slice with split content.
func (c *CodeAwareTextSplitter) SplitDocuments(ctx context.Context, docs []schema.Document) ([]schema.Document, error) {
	var finalDocs []schema.Document
	for _, doc := range docs {
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

func (c *CodeAwareTextSplitter) splitSingleDocument(ctx context.Context, doc schema.Document) ([]schema.Document, error) {
	source, ok := doc.Metadata["source"].(string)
	if !ok {
		return nil, errors.New("document metadata is missing 'source' key")
	}

	// 1. Identify Parser once
	parser, err := c.parserRegistry.GetParserForFile(source, nil)
	if err != nil {
		c.logger.Debug("No specific parser found for metadata extraction", "source", source)
	}

	// 2. Extract specific metadata (Package, Imports, and Test status)
	extraMetadata := make(map[string]any)
	if parser != nil {
		if meta, err := parser.ExtractMetadata(doc.PageContent, source); err == nil {
			if meta.PackageName != "" {
				extraMetadata["package_name"] = meta.PackageName
			}
			if len(meta.Imports) > 0 {
				extraMetadata["imports"] = meta.Imports
			}
		}
	}

	// Quick Win: Mark as test file based on naming conventions
	if isTestFile(source) {
		extraMetadata["is_test"] = true
	}

	// 3. Perform Chunking
	codeChunks, err := c.ChunkFileWithFileInfo(ctx, doc.PageContent, source, c.modelName, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk document content for source %q: %w", source, err)
	}

	// 4. Map chunks back to full Documents with merged metadata
	splitDocs := make([]schema.Document, 0, len(codeChunks))
	for _, chunk := range codeChunks {
		newMetadata := make(map[string]any, len(doc.Metadata)+len(extraMetadata)+5)

		// Priority 1: Original Doc Metadata
		for k, v := range doc.Metadata {
			newMetadata[k] = v
		}
		// Priority 2: Inferred Language Metadata
		for k, v := range extraMetadata {
			newMetadata[k] = v
		}

		// Priority 3: Chunk-specific data
		newMetadata["line_start"] = chunk.LineStart
		newMetadata["line_end"] = chunk.LineEnd
		if chunk.ParentID != "" {
			newMetadata["parent_id"] = chunk.ParentID
		}
		if chunk.FullParentText != "" {
			newMetadata["full_parent_text"] = chunk.FullParentText
		}
		for k, v := range chunk.Annotations {
			newMetadata[k] = v
		}

		splitDocs = append(splitDocs, schema.NewDocument(chunk.Content, newMetadata))
	}
	return splitDocs, nil
}

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

	// Try language-aware chunking
	if chunks, err := c.tryLanguageSpecificChunking(ctx, content, filePath, fileInfo, pluginOpts, modelName); err == nil && len(chunks) > 0 {
		validChunks := c.postProcessChunks(ctx, chunks, params, modelName, filePath)
		if len(validChunks) > 0 {
			return validChunks, nil
		}
	}

	// Fallback to character-based recursive splitting
	return c.intelligentFallbackChunk(ctx, content, filePath, params, modelName)
}

func (c *CodeAwareTextSplitter) generateParentID(filePath, identifier string, lineStart int) string {
	key := fmt.Sprintf("%s:%s:%d", filePath, identifier, lineStart)
	if id, ok := c.parentIDCache.Load(key); ok {
		return id.(string)
	}

	h := sha256.New()
	h.Write([]byte(key))
	id := hex.EncodeToString(h.Sum(nil))[:16] // 16 chars is usually enough for collisions in local repos
	c.parentIDCache.Store(key, id)
	return id
}

func (c *CodeAwareTextSplitter) truncateParentText(text string) string {
	maxLen := MaxParentTextLength
	if c.parentConfig.MaxTextLength > 0 {
		maxLen = c.parentConfig.MaxTextLength
	}
	return TruncateParentText(text, maxLen)
}

// TruncateParentText reduces text length while preserving start and end context.
func TruncateParentText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}

	// Keep beginning and end for context, add an ellipsis bridge
	separator := "\n...\n"
	half := (maxLen - len(separator)) / 2
	if half < 1 {
		return string(runes[:maxLen])
	}

	return string(runes[:half]) + separator + string(runes[len(runes)-half:])
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

// isTestFile checks if the filename follows common testing conventions across supported languages.
func isTestFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, "_test.go") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".spec.ts") ||
		strings.HasSuffix(lower, ".test.tsx") ||
		strings.HasSuffix(lower, ".spec.tsx") ||
		strings.HasSuffix(lower, ".test.js") ||
		strings.HasSuffix(lower, ".spec.js")
}
