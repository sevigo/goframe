package documentloaders

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

type Loader interface {
	Load(ctx context.Context) ([]schema.Document, error)
	LoadAndProcessStream(ctx context.Context, processFn func(ctx context.Context, docs []schema.Document) error) error
}

// GitLoader loads and processes documents from a git repository on the local file system.
// It uses a ParserRegistry to apply language-specific chunking strategies.
type GitLoader struct {
	path           string
	parserRegistry parsers.ParserRegistry
	logger         *slog.Logger
	options        gitLoaderOptions
}

type gitLoaderOptions struct {
	IncludeExts map[string]bool
	ExcludeExts map[string]bool
	ExcludeDirs map[string]bool
	Logger      *slog.Logger
}

type GitLoaderOption func(*gitLoaderOptions)

func WithLogger(logger *slog.Logger) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if logger != nil {
			opts.Logger = logger
		}
	}
}

func WithExcludeExts(exts []string) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if opts.ExcludeExts == nil {
			opts.ExcludeExts = make(map[string]bool)
		}
		for _, ext := range exts {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			opts.ExcludeExts[strings.ToLower(ext)] = true
		}
	}
}

func WithExcludeDirs(dirs []string) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if opts.ExcludeDirs == nil {
			opts.ExcludeDirs = make(map[string]bool)
		}
		for _, dir := range dirs {
			opts.ExcludeDirs[dir] = true
		}
	}
}

func WithIncludeExts(exts []string) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if opts.IncludeExts == nil {
			opts.IncludeExts = make(map[string]bool)
		}
		for _, ext := range exts {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			opts.IncludeExts[strings.ToLower(ext)] = true
		}
	}
}

func NewGit(path string, registry parsers.ParserRegistry, opts ...GitLoaderOption) *GitLoader {
	loaderOpts := gitLoaderOptions{
		Logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(&loaderOpts)
	}

	return &GitLoader{
		path:           path,
		parserRegistry: registry,
		options:        loaderOpts,
		logger:         loaderOpts.Logger.With("component", "git_loader"),
	}
}

func (g *GitLoader) LoadAndProcessStream(ctx context.Context, processFn func(ctx context.Context, docs []schema.Document) error) error {
	g.logger.InfoContext(ctx, "Starting repository stream processing", "path", g.path)
	documentBatch := make([]schema.Document, 0, 256)
	textParser, _ := g.parserRegistry.GetParser("text")

	err := filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			g.logger.WarnContext(ctx, "Skipping unreadable path", "path", path, "error", err)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			dirName := d.Name()
			if g.options.ExcludeDirs != nil && g.options.ExcludeDirs[dirName] {
				g.logger.Debug("Skipping user-excluded directory", "path", path)
				return filepath.SkipDir
			}
			if shouldSkipDir(dirName) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if len(g.options.IncludeExts) > 0 && !g.options.IncludeExts[ext] {
			return nil
		}
		if len(g.options.ExcludeExts) > 0 && g.options.ExcludeExts[ext] {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			g.logger.WarnContext(ctx, "Could not get file info, skipping", "path", path, "error", err)
			return nil
		}
		if shouldSkipFile(path, fileInfo) {
			g.logger.Debug("Skipping excluded file", "path", path, "size", fileInfo.Size())
			return nil
		}

		fileDocuments := g.processFile(path, fileInfo, textParser)
		documentBatch = append(documentBatch, fileDocuments...)

		if len(documentBatch) >= 256 {
			if err := processFn(ctx, documentBatch); err != nil {
				return fmt.Errorf("processing function failed for batch: %w", err)
			}
			documentBatch = make([]schema.Document, 0, 256) // Reset batch
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(documentBatch) > 0 {
		if err := processFn(ctx, documentBatch); err != nil {
			return fmt.Errorf("processing function failed for final batch: %w", err)
		}
	}
	g.logger.InfoContext(ctx, "Repository stream processing completed", "path", g.path)
	return nil
}

func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	g.logger.InfoContext(ctx, "Starting repository load", "path", g.path)
	var documents []schema.Document

	textParser, _ := g.parserRegistry.GetParser("text")

	err := filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			g.logger.WarnContext(ctx, "Skipping unreadable path", "path", path, "error", err)
			return nil
		}

		if d.IsDir() {
			dirName := d.Name()
			if g.options.ExcludeDirs != nil && g.options.ExcludeDirs[dirName] {
				g.logger.Debug("Skipping user-excluded directory", "path", path)
				return filepath.SkipDir
			}
			if shouldSkipDir(dirName) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if len(g.options.IncludeExts) > 0 && !g.options.IncludeExts[ext] {
			return nil
		}
		if len(g.options.ExcludeExts) > 0 && g.options.ExcludeExts[ext] {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			g.logger.WarnContext(ctx, "Could not get file info, skipping", "path", path, "error", err)
			return nil
		}

		if shouldSkipFile(path, fileInfo) {
			g.logger.Debug("Skipping excluded file", "path", path, "size", fileInfo.Size())
			return nil
		}

		fileDocuments := g.processFile(path, fileInfo, textParser)
		documents = append(documents, fileDocuments...)

		return nil
	})

	if err != nil {
		g.logger.ErrorContext(ctx, "Repository walk failed", "error", err)
		return nil, err
	}

	g.logger.InfoContext(ctx, "Repository load completed", "path", g.path, "total_documents", len(documents))
	return documents, nil
}

func (g *GitLoader) processFile(path string, fileInfo fs.FileInfo, textParser schema.ParserPlugin) []schema.Document {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		g.logger.Warn("Cannot read file, skipping", "path", path, "error", err)
		return nil
	}
	// Replace any invalid byte sequences with the Unicode replacement character.
	content := strings.ToValidUTF8(string(contentBytes), "\uFFFD")

	relPath, err := filepath.Rel(g.path, path)
	if err != nil {
		g.logger.Warn("Could not get relative path, using absolute", "error", err, "full_path", path)
		relPath = path
	}

	baseMetadata := map[string]any{
		"source":    relPath,
		"file_size": fileInfo.Size(),
		"mod_time":  fileInfo.ModTime(),
	}

	parser, err := g.parserRegistry.GetParserForFile(path, fileInfo)
	if err != nil {
		g.logger.Debug("No specific parser found, using text fallback", "path", path)
		parser = textParser
	}

	if parser == nil {
		g.logger.Warn("No parser available, treating as single document", "path", path)
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}

	chunks, err := parser.Chunk(content, path, nil)
	if err != nil {
		g.logger.Warn("Chunking failed or returned no chunks, treating as single document", "path", path, "parser", parser.Name(), "error", err)
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}
	if len(chunks) == 0 {
		// The parser ran successfully but found no semantic units to chunk.
		// This is a normal edge case, so we log it at INFO level and use the fallback.
		g.logger.Info("No semantic chunks found by parser, treating as single document", "path", path, "parser", parser.Name())
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}

	documents := make([]schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		chunkMetadata := buildChunkMetadata(baseMetadata, chunk, i, len(chunks))

		var enrichedContentBuilder strings.Builder
		source, _ := chunkMetadata["source"].(string)
		identifier, _ := chunkMetadata["identifier"].(string)
		chunkType, _ := chunkMetadata["chunk_type"].(string)

		enrichedContentBuilder.WriteString(fmt.Sprintf("File: %s\n", source))
		if chunkType != "" && identifier != "" {
			enrichedContentBuilder.WriteString(fmt.Sprintf("Type: %s\nIdentifier: %s\n", chunkType, identifier))
		}
		enrichedContentBuilder.WriteString("---\n")
		enrichedContentBuilder.WriteString(chunk.Content)

		doc := schema.NewDocument(enrichedContentBuilder.String(), chunkMetadata)
		documents = append(documents, doc)
	}
	return documents
}

func buildChunkMetadata(baseMetadata map[string]any, chunk schema.CodeChunk, chunkIndex, totalChunks int) map[string]any {
	chunkMetadata := make(map[string]any, len(baseMetadata)+len(chunk.Annotations)+6)
	maps.Copy(chunkMetadata, baseMetadata)

	chunkMetadata["identifier"] = chunk.Identifier
	chunkMetadata["chunk_type"] = chunk.Type
	chunkMetadata["line_start"] = chunk.LineStart
	chunkMetadata["line_end"] = chunk.LineEnd
	chunkMetadata["chunk_index"] = chunkIndex
	chunkMetadata["total_chunks"] = totalChunks

	for k, v := range chunk.Annotations {
		chunkMetadata[k] = v
	}
	return chunkMetadata
}

func shouldSkipDir(name string) bool {
	skipDirs := []string{
		".git", ".svn", ".hg",
		"vendor", "node_modules", "__pycache__",
		"build", "dist", "target", "out", "bin",
		".vscode", ".idea", ".vs",
		".DS_Store", "Thumbs.db",
	}
	return slices.Contains(skipDirs, name)
}

func shouldSkipFile(path string, info fs.FileInfo) bool {
	const maxFileSize = 10 * 1024 * 1024 // 10MB
	if info.Size() > maxFileSize {
		return true
	}

	ext := strings.ToLower(filepath.Ext(path))
	// A list of common binary file extensions.
	// PDF is intentionally not included, as it can be parsed.
	binaryExts := map[string]bool{
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

	return binaryExts[ext]
}
