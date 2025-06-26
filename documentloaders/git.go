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

// Loader defines the interface for loading documents.
type Loader interface {
	Load(ctx context.Context) ([]schema.Document, error)
}

// It uses a ParserRegistry to apply language-specific chunking strategies.
type GitLoader struct {
	path           string
	parserRegistry parsers.ParserRegistry
	logger         *slog.Logger
}

type GitLoaderOption func(*GitLoader)

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) GitLoaderOption {
	return func(g *GitLoader) {
		g.logger = logger
	}
}

func NewGit(path string, registry parsers.ParserRegistry, opts ...GitLoaderOption) *GitLoader {
	loader := &GitLoader{
		path:           path,
		parserRegistry: registry,
		logger:         slog.Default(),
	}

	for _, opt := range opts {
		opt(loader)
	}

	return loader
}

func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	g.logger.Info("Starting git repository load", "path", g.path)

	var documents []schema.Document

	textParser, _ := g.parserRegistry.GetParser("text")
	if textParser != nil {
		g.logger.Debug("Text fallback parser available", "parser", textParser.Name())
	} else {
		g.logger.Warn("No text fallback parser available - files without specific parsers will be treated as single documents")
	}

	err := filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			g.logger.Warn("Skipping unreadable path", "path", path, "error", err)
			return nil
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				g.logger.Debug("Skipping excluded directory", "dir", d.Name(), "path", path)
				return filepath.SkipDir
			}
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			g.logger.Warn("Could not get file info, skipping", "path", path, "error", err)
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
		g.logger.Error("Repository walk failed", "error", err)
		return nil, err
	}

	g.logger.Info("Git repository load completed",
		"path", g.path,
		"total_documents", len(documents),
	)

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
		g.logger.Warn("Could not get relative path, using absolute",
			"error", err,
			"full_path", path,
		)
		relPath = path
	}

	baseMetadata := map[string]any{
		"source":    relPath,
		"file_size": fileInfo.Size(),
		"mod_time":  fileInfo.ModTime(),
	}

	parser, err := g.parserRegistry.GetParserForFile(path, fileInfo)
	if err != nil {
		g.logger.Debug("No specific parser found, using text fallback",
			"path", path,
			"error", err,
		)
		parser = textParser
	}

	if parser == nil {
		g.logger.Warn("No parser available, treating as single document", "path", path)
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}

	g.logger.Debug("Chunking file", "path", path, "parser", parser.Name())
	chunks, err := parser.Chunk(content, path, nil)
	if err != nil || len(chunks) == 0 {
		if err != nil {
			g.logger.Warn("Chunking failed, treating as single document",
				"path", path,
				"parser", parser.Name(),
				"error", err,
			)
		} else {
			g.logger.Warn("Chunking returned no chunks, treating as single document",
				"path", path,
				"parser", parser.Name(),
			)
		}
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}

	documents := make([]schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		chunkMetadata := g.buildChunkMetadata(baseMetadata, chunk, i, len(chunks))
		documents = append(documents, schema.NewDocument(chunk.Content, chunkMetadata))
	}

	g.logger.Debug("File processed successfully",
		"path", path,
		"parser", parser.Name(),
		"chunks_created", len(documents),
	)

	return documents
}

func (g *GitLoader) buildChunkMetadata(baseMetadata map[string]any, chunk schema.CodeChunk, chunkIndex, totalChunks int) map[string]any {
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

// shouldSkipDir returns true for common directories that should be excluded from document loading.
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

// This includes binary files and very large files.
func shouldSkipFile(path string, info fs.FileInfo) bool {
	const maxFileSize = 10 * 1024 * 1024 // 10MB
	if info.Size() > maxFileSize {
		return true
	}

	ext := strings.ToLower(filepath.Ext(path))
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
		// Note: PDF is intentionally not included here as PDF parsers
		// in the registry should handle these files appropriately
	}

	return binaryExts[ext]
}
