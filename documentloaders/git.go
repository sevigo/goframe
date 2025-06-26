package documentloaders

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

type Loader interface {
	Load(ctx context.Context) ([]schema.Document, error)
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
	Logger      *slog.Logger
}

// GitLoaderOption configures a GitLoader.
type GitLoaderOption func(*gitLoaderOptions)

// WithLogger sets the logger for the GitLoader.
func WithLogger(logger *slog.Logger) GitLoaderOption {
	return func(opts *gitLoaderOptions) {
		if logger != nil {
			opts.Logger = logger
		}
	}
}

// WithIncludeExts sets a whitelist of file extensions to load.
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

// NewGit creates a new git repository loader.
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

// Load walks the repository, parses files, and returns a slice of documents.
func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	g.logger.InfoContext(ctx, "Starting repository load", "path", g.path)
	var documents []schema.Document

	textParser, _ := g.parserRegistry.GetParser("text")

	err := filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			g.logger.WarnContext(ctx, "Skipping unreadable path", "path", path, "error", err)
			return nil // Continue walking.
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				g.logger.Debug("Skipping excluded directory", "dir", d.Name())
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
	content := string(contentBytes)

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
	if err != nil || len(chunks) == 0 {
		g.logger.Warn("Chunking failed or returned no chunks, treating as single document", "path", path, "parser", parser.Name(), "error", err)
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}

	documents := make([]schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		chunkMetadata := buildChunkMetadata(baseMetadata, chunk, i, len(chunks))
		documents = append(documents, schema.NewDocument(chunk.Content, chunkMetadata))
	}
	return documents
}

func buildChunkMetadata(baseMetadata map[string]any, chunk schema.CodeChunk, chunkIndex, totalChunks int) map[string]any {
	chunkMetadata := make(map[string]any, len(baseMetadata)+len(chunk.Annotations)+6)
	for k, v := range baseMetadata {
		chunkMetadata[k] = v
	}

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

// shouldSkipDir returns true for common directories to exclude.
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

// shouldSkipFile returns true for files that shouldn't be loaded.
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
