// Package documentloaders provides document loading utilities for RAG applications.
// It includes loaders for git repositories and other document sources with support
// for streaming, batch processing, and memory protection.
package documentloaders

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/textsplitter"
)

const (
	defaultBatchSize    = 256
	maxFileSize         = 10 * 1024 * 1024  // 10MB
	maxMemoryBuffer     = 100 * 1024 * 1024 // 100MB total buffer
	defaultWorkerCount  = 4
	maxParentTextLength = 2000
)

// skipDirs contains directory names to skip during loading.
var skipDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"vendor": true, "node_modules": true, "__pycache__": true,
	"build": true, "dist": true, "target": true, "out": true, "bin": true,
	".vscode": true, ".idea": true, ".vs": true,
	".DS_Store": true, "Thumbs.db": true,
}

// binaryExts contains file extensions for binary files to skip.
var binaryExts = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".tiff": true, ".svg": true, ".ico": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true,
	".7z": true, ".bz2": true, ".xz": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
	".wav": true, ".flac": true, ".ogg": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true,
	".bin": true, ".dat": true, ".db": true, ".sqlite": true,
}

// Error variables for document loading operations.
var (
	// ErrInvalidPath is returned when the repository path is invalid.
	ErrInvalidPath = errors.New("documentloaders: invalid repository path")
	// ErrNilRegistry is returned when the parser registry is nil.
	ErrNilRegistry = errors.New("documentloaders: parser registry is nil")
	// ErrPathNotExist is returned when the path does not exist.
	ErrPathNotExist = errors.New("documentloaders: path does not exist")
	// ErrMemoryLimitExceeded is returned when memory limit is exceeded during loading.
	ErrMemoryLimitExceeded = errors.New("documentloaders: memory limit exceeded")
)

// FileData is an in-memory representation of a file to be processed.
type FileData struct {
	// Path is the file path relative to the repository root.
	Path string
	// Content is the file content.
	Content string
	// FileInfo contains file metadata.
	FileInfo fs.FileInfo
}

// Loader is the interface for document loaders.
type Loader interface {
	// Load loads all documents from the source.
	Load(ctx context.Context) ([]schema.Document, error)
	// LoadAndProcessStream loads documents in batches and processes them.
	LoadAndProcessStream(ctx context.Context, processFn func(ctx context.Context, docs []schema.Document) error) error
}

// GitLoader loads and processes documents from a git repository on the local file system.
// It supports batch processing, parallel file processing, and memory protection.
type GitLoader struct {
	path           string
	parserRegistry parsers.ParserRegistry
	logger         *slog.Logger
	options        gitLoaderOptions
}

type gitLoaderOptions struct {
	IncludeExts         map[string]bool
	ExcludeExts         map[string]bool
	ExcludeDirs         map[string]bool
	Logger              *slog.Logger
	BatchSize           int
	WorkerCount         int
	MaxMemBuffer        int64
	DetectGeneratedCode bool
}

// GitLoaderOption configures a GitLoader.
type GitLoaderOption func(*gitLoaderOptions)

// WithLogger sets the logger for the loader.
func WithLogger(logger *slog.Logger) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if logger != nil {
			opts.Logger = logger
		}
	}
}

// WithBatchSize sets the batch size for document processing.
func WithBatchSize(size int) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if size > 0 {
			opts.BatchSize = size
		}
	}
}

// WithWorkerCount sets the number of parallel workers for file processing.
func WithWorkerCount(count int) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if count > 0 {
			opts.WorkerCount = count
		}
	}
}

// WithMaxMemoryBuffer sets the maximum memory buffer in bytes.
// Processing will pause when memory usage exceeds this limit.
func WithMaxMemoryBuffer(bytes int64) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if bytes > 0 {
			opts.MaxMemBuffer = bytes
		}
	}
}

// WithExcludeExts sets file extensions to exclude from loading.
// Extensions can be provided with or without the leading dot.
func WithExcludeExts(exts []string) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if opts.ExcludeExts == nil {
			opts.ExcludeExts = make(map[string]bool, len(exts))
		}
		for _, ext := range exts {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			opts.ExcludeExts[strings.ToLower(ext)] = true
		}
	}
}

// WithExcludeDirs sets directory names to exclude from loading.
func WithExcludeDirs(dirs []string) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if opts.ExcludeDirs == nil {
			opts.ExcludeDirs = make(map[string]bool, len(dirs))
		}
		for _, dir := range dirs {
			opts.ExcludeDirs[dir] = true
		}
	}
}

// WithIncludeExts sets file extensions to include in loading.
// If set, only files with these extensions will be loaded.
func WithIncludeExts(exts []string) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if opts.IncludeExts == nil {
			opts.IncludeExts = make(map[string]bool, len(exts))
		}
		for _, ext := range exts {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			opts.IncludeExts[strings.ToLower(ext)] = true
		}
	}
}

// WithGeneratedCodeDetection enables or disables detection of auto-generated code.
// When enabled, files detected as generated will be skipped.
func WithGeneratedCodeDetection(enable bool) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		opts.DetectGeneratedCode = enable
	}
}

// NewGit creates a new GitLoader for the specified repository path.
// Returns an error if the path is invalid, registry is nil, or path doesn't exist.
func NewGit(path string, registry parsers.ParserRegistry, opts ...GitLoaderOption) (*GitLoader, error) {
	if path == "" {
		return nil, ErrInvalidPath
	}

	if registry == nil {
		return nil, ErrNilRegistry
	}

	// Check if path exists
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrPathNotExist, path)
		}
		return nil, fmt.Errorf("failed to access path: %w", err)
	}

	loaderOpts := gitLoaderOptions{
		Logger:       slog.Default(),
		BatchSize:    defaultBatchSize,
		WorkerCount:  defaultWorkerCount,
		MaxMemBuffer: maxMemoryBuffer,
	}

	for _, opt := range opts {
		opt(&loaderOpts)
	}

	return &GitLoader{
		path:           path,
		parserRegistry: registry,
		options:        loaderOpts,
		logger:         loaderOpts.Logger.With("component", "git_loader", "path", path),
	}, nil
}

// LoadAndProcessStream uses a pipeline with controlled memory usage
func (g *GitLoader) LoadAndProcessStream(ctx context.Context, processFn func(ctx context.Context, docs []schema.Document) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	g.logger.InfoContext(ctx, "Starting streaming repository load")

	// Create pipeline channels
	fileChan := make(chan FileData, g.options.WorkerCount*2)
	docChan := make(chan schema.Document, g.options.BatchSize*2)
	errChan := make(chan error, 1)

	// Context for cancellation
	pipelineCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	// Stage 1: File discovery and reading
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(fileChan)

		if err := g.walkAndReadFiles(pipelineCtx, fileChan); err != nil {
			select {
			case errChan <- fmt.Errorf("file walking failed: %w", err):
			default:
			}
			cancel()
		}
	}()

	var processorWg sync.WaitGroup
	for i := range g.options.WorkerCount {
		processorWg.Add(1)
		go func(workerID int) {
			defer processorWg.Done()
			g.processFilesWorker(pipelineCtx, fileChan, docChan)
		}(i)
	}

	// Close docChan when all processors are done
	go func() {
		processorWg.Wait()
		close(docChan)
	}()

	// Stage 3: Batch and process documents
	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := g.batchAndProcess(pipelineCtx, docChan, processFn); err != nil {
			select {
			case errChan <- fmt.Errorf("batch processing failed: %w", err):
			default:
			}
			cancel()
		}
	}()

	// Wait for all stages to complete
	wg.Wait()

	// Check for errors
	select {
	case err := <-errChan:
		return err
	default:
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	g.logger.InfoContext(ctx, "Streaming repository load completed")
	return nil
}

func (g *GitLoader) walkAndReadFiles(ctx context.Context, fileChan chan<- FileData) error {
	var currentMemUsage int64

	return filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		// Check context frequently
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			g.logger.WarnContext(ctx, "Skipping unreadable path", "path", path, "error", err)
			return nil
		}

		if d.IsDir() {
			return g.handleDirectory(d)
		}

		// Check file filters
		if !g.shouldProcessFile(path) {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			g.logger.WarnContext(ctx, "Could not get file info", "path", path, "error", err)
			return nil
		}

		if shouldSkipFile(path, fileInfo) {
			g.logger.DebugContext(ctx, "Skipping file", "path", path, "size", fileInfo.Size())
			return nil
		}

		// Check memory limit
		if currentMemUsage+fileInfo.Size() > g.options.MaxMemBuffer {
			g.logger.WarnContext(ctx, "Memory buffer limit reached, skipping file",
				"path", path,
				"current_usage", currentMemUsage,
				"limit", g.options.MaxMemBuffer)
			return nil
		}

		contentBytes, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir callback within repo directory
		if err != nil {
			g.logger.WarnContext(ctx, "Cannot read file", "path", path, "error", err)
			return nil
		}

		currentMemUsage += fileInfo.Size()

		select {
		case fileChan <- FileData{
			Path:     path,
			Content:  string(contentBytes),
			FileInfo: fileInfo,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	})
}

func (g *GitLoader) processFilesWorker(ctx context.Context, fileChan <-chan FileData, docChan chan<- schema.Document) {
	textParser, _ := g.parserRegistry.GetParser("text")

	for {
		select {
		case <-ctx.Done():
			return
		case fileData, ok := <-fileChan:
			if !ok {
				return
			}

			docs := g.processFile(fileData.Path, fileData.FileInfo, textParser, fileData.Content)

			for _, doc := range docs {
				select {
				case docChan <- doc:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (g *GitLoader) batchAndProcess(ctx context.Context, docChan <-chan schema.Document, processFn func(ctx context.Context, docs []schema.Document) error) error {
	batch := make([]schema.Document, 0, g.options.BatchSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case doc, ok := <-docChan:
			if !ok {
				// Channel closed, process final batch
				if len(batch) > 0 {
					if err := processFn(ctx, batch); err != nil {
						return fmt.Errorf("final batch processing failed: %w", err)
					}
				}
				return nil
			}

			batch = append(batch, doc)

			if len(batch) >= g.options.BatchSize {
				if err := processFn(ctx, batch); err != nil {
					return fmt.Errorf("batch processing failed: %w", err)
				}
				batch = make([]schema.Document, 0, g.options.BatchSize)
			}
		}
	}
}

func (g *GitLoader) handleDirectory(d fs.DirEntry) error {
	dirName := d.Name()
	if g.options.ExcludeDirs != nil && g.options.ExcludeDirs[dirName] {
		g.logger.Debug("Skipping user-excluded directory", "dir", dirName)
		return filepath.SkipDir
	}
	if shouldSkipDir(dirName) {
		g.logger.Debug("Skipping default-excluded directory", "dir", dirName)
		return filepath.SkipDir
	}
	return nil
}

func (g *GitLoader) shouldProcessFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	// Check include list first (whitelist)
	if len(g.options.IncludeExts) > 0 {
		return g.options.IncludeExts[ext]
	}

	// Check exclude list (blacklist)
	if len(g.options.ExcludeExts) > 0 {
		return !g.options.ExcludeExts[ext]
	}

	return true
}

func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	g.logger.InfoContext(ctx, "Starting full repository load")

	var documents []schema.Document
	var mu sync.Mutex

	err := g.LoadAndProcessStream(ctx, func(ctx context.Context, docs []schema.Document) error {
		mu.Lock()
		documents = append(documents, docs...)
		mu.Unlock()
		return nil
	})

	if err != nil {
		return nil, err
	}

	g.logger.InfoContext(ctx, "Repository load completed", "total_documents", len(documents))
	return documents, nil
}

func (g *GitLoader) processFile(path string, fileInfo fs.FileInfo, textParser schema.ParserPlugin, content string) []schema.Document {
	validContent := strings.ToValidUTF8(content, "\uFFFD")

	relPath, err := filepath.Rel(g.path, path)
	if err != nil {
		g.logger.Warn("Could not get relative path, using absolute", "error", err, "full_path", path)
		relPath = path
	}
	relPath = filepath.ToSlash(relPath)

	parser, err := g.parserRegistry.GetParserForFile(path, fileInfo)
	if err != nil {
		g.logger.Debug("No specific parser found, using text fallback", "path", relPath)
		parser = textParser
	}

	if parser == nil {
		g.logger.Warn("No parser available, treating as single document", "path", relPath)
		return []schema.Document{schema.NewDocument(validContent, map[string]any{
			"source":    relPath,
			"file_size": fileInfo.Size(),
			"mod_time":  fileInfo.ModTime(),
		})}
	}

	// Check for generated code if enabled (using parser logic)
	if g.options.DetectGeneratedCode && parser.IsGenerated(validContent, path) {
		g.logger.Debug("Skipping generated file", "path", relPath, "parser", parser.Name())
		return nil
	}

	baseMetadata := map[string]any{
		"source":    relPath,
		"file_size": fileInfo.Size(),
		"mod_time":  fileInfo.ModTime(),
	}

	// Try to extract language-specific metadata (e.g. imports, package name)
	if meta, metadataErr := parser.ExtractMetadata(validContent, path); metadataErr == nil {
		if meta.PackageName != "" {
			baseMetadata["package_name"] = meta.PackageName
		}
		if len(meta.Imports) > 0 {
			baseMetadata["imports"] = meta.Imports
		}
	} else {
		// Just debug log, not critical
		g.logger.Debug("Metadata extraction skipped or failed", "path", relPath, "error", metadataErr)
	}

	chunks, err := parser.Chunk(validContent, path, nil)
	if err != nil || len(chunks) == 0 {
		g.logger.Info("Chunking failed or returned no chunks",
			"path", relPath,
			"parser", parser.Name(),
			"error", err)
		return []schema.Document{schema.NewDocument(validContent, baseMetadata)}
	}

	documents := make([]schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		chunkMetadata := buildChunkMetadata(baseMetadata, chunk, i, len(chunks))

		var enrichedContentBuilder strings.Builder
		source, _ := chunkMetadata["source"].(string)
		identifier, _ := chunkMetadata["identifier"].(string)
		chunkType, _ := chunkMetadata["chunk_type"].(string)

		fmt.Fprintf(&enrichedContentBuilder, "File: %s\n", source)
		if chunkType != "" && identifier != "" {
			fmt.Fprintf(&enrichedContentBuilder, "Type: %s\nIdentifier: %s\n", chunkType, identifier)
		}
		enrichedContentBuilder.WriteString("---\n")
		enrichedContentBuilder.WriteString(chunk.Content)

		doc := schema.NewDocument(enrichedContentBuilder.String(), chunkMetadata)
		documents = append(documents, doc)
	}

	return documents
}

func buildChunkMetadata(baseMetadata map[string]any, chunk schema.CodeChunk, chunkIndex, totalChunks int) map[string]any {
	chunkMetadata := make(map[string]any, len(baseMetadata)+len(chunk.Annotations)+9)
	maps.Copy(chunkMetadata, baseMetadata)

	chunkMetadata["identifier"] = chunk.Identifier
	chunkMetadata["chunk_type"] = chunk.Type
	chunkMetadata["line_start"] = chunk.LineStart
	chunkMetadata["line_end"] = chunk.LineEnd
	chunkMetadata["chunk_index"] = chunkIndex
	chunkMetadata["total_chunks"] = totalChunks
	chunkMetadata["parent_id"] = chunk.ParentID
	chunkMetadata["is_definition"] = chunk.IsDefinition
	chunkMetadata["symbol_type"] = chunk.SymbolType
	if chunk.FullParentText != "" {
		chunkMetadata["full_parent_text"] = truncateParentText(chunk.FullParentText)
	}

	for k, v := range chunk.Annotations {
		chunkMetadata[k] = v
	}

	return chunkMetadata
}

func shouldSkipDir(name string) bool {
	return skipDirs[name]
}

func shouldSkipFile(path string, info fs.FileInfo) bool {
	if info.Size() > maxFileSize {
		return true
	}

	ext := strings.ToLower(filepath.Ext(path))
	return binaryExts[ext]
}

func truncateParentText(text string) string {
	return textsplitter.TruncateParentText(text, maxParentTextLength)
}
