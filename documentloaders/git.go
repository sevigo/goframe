// Package documentloaders provides interfaces and implementations for loading
// documents from various sources into a format suitable for RAG pipelines.
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

// Loader defines the interface for loading documents from various sources.
// Implementations should handle source-specific logic while returning
// a consistent document format for downstream processing.
type Loader interface {
	// Load retrieves documents from the source and returns them as a slice
	// of schema.Document objects. The context can be used for cancellation
	// and timeout control during the loading process.
	Load(ctx context.Context) ([]schema.Document, error)
}

// GitLoader loads and processes documents from a git repository on the local file system.
// It intelligently handles different file types using a ParserRegistry to apply
// language-specific chunking strategies, falling back to text chunking when needed.
//
// The loader automatically:
//   - Skips common build directories and binary files
//   - Applies language-aware chunking for supported file types
//   - Creates semantic chunks with rich metadata for better retrieval
//   - Handles parsing failures gracefully with fallback strategies
type GitLoader struct {
	// path is the root directory of the git repository to load
	path string

	// parserRegistry provides language-specific parsers for different file types
	parserRegistry parsers.ParserRegistry

	// logger handles structured logging throughout the loading process
	logger *slog.Logger
}

// GitLoaderOption defines functional options for configuring GitLoader.
type GitLoaderOption func(*GitLoader)

// WithLogger sets a custom logger for the GitLoader.
// If not provided, slog.Default() will be used.
func WithLogger(logger *slog.Logger) GitLoaderOption {
	return func(g *GitLoader) {
		g.logger = logger
	}
}

// NewGit creates a new git repository loader for the specified path.
//
// Parameters:
//   - path: Root directory of the git repository to load
//   - registry: ParserRegistry for language-specific file processing
//   - opts: Functional options for configuration
//
// The loader will recursively walk through the repository, applying appropriate
// parsers based on file extensions and content types.
func NewGit(path string, registry parsers.ParserRegistry, opts ...GitLoaderOption) *GitLoader {
	loader := &GitLoader{
		path:           path,
		parserRegistry: registry,
		logger:         slog.Default(),
	}

	// Apply functional options
	for _, opt := range opts {
		opt(loader)
	}

	return loader
}

// Load walks the repository path, processes each file with appropriate parsers,
// and returns semantically meaningful document chunks.
//
// Process flow:
//  1. Walk through all files in the repository
//  2. Skip build directories, binary files, and large files
//  3. Read file content and extract metadata
//  4. Find appropriate language parser or use text fallback
//  5. Chunk content into semantic units
//  6. Create documents with rich metadata for each chunk
//
// The method handles various failure scenarios gracefully:
//   - Unreadable files are skipped with warnings
//   - Parser failures fall back to single-document treatment
//   - Missing parsers use text fallback when available
//
// Returns a slice of schema.Document objects, each representing a semantic
// chunk of content with metadata including source file, line numbers, and
// chunk type information.
func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	g.logger.Info("Starting git repository load", "path", g.path)

	var documents []schema.Document

	// Get the fallback text parser once for efficiency
	// This will be used for any file type without a specific language parser
	textParser, _ := g.parserRegistry.GetParser("text")
	if textParser != nil {
		g.logger.Debug("Text fallback parser available", "parser", textParser.Name())
	} else {
		g.logger.Warn("No text fallback parser available - files without specific parsers will be treated as single documents")
	}

	err := filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		// Handle walk errors (permissions, broken symlinks, etc.)
		if err != nil {
			g.logger.Warn("Skipping unreadable path", "path", path, "error", err)
			return nil // Continue walking, don't abort entire process
		}

		// Handle directories - skip common build/dependency directories
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				g.logger.Debug("Skipping excluded directory", "dir", d.Name(), "path", path)
				return filepath.SkipDir
			}
			return nil
		}

		// Get file info for size/type checking
		fileInfo, err := d.Info()
		if err != nil {
			g.logger.Warn("Could not get file info, skipping", "path", path, "error", err)
			return nil
		}

		// Skip binary files, large files, and other excluded types
		if shouldSkipFile(path, fileInfo) {
			g.logger.Debug("Skipping excluded file", "path", path, "size", fileInfo.Size())
			return nil
		}

		// Process the file and add resulting documents
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

// processFile handles the loading and chunking of a single file.
// It attempts to use language-specific parsers first, falling back to
// text parsing, and finally to single-document treatment if all else fails.
func (g *GitLoader) processFile(path string, fileInfo fs.FileInfo, textParser schema.ParserPlugin) []schema.Document {
	// Read file content
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		g.logger.Warn("Cannot read file, skipping", "path", path, "error", err)
		return nil
	}
	content := string(contentBytes)

	// Create base metadata that will be shared across all chunks from this file
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

	// Find appropriate parser for this file type
	parser, err := g.parserRegistry.GetParserForFile(path, fileInfo)
	if err != nil {
		// No language-specific parser found, try text fallback
		g.logger.Debug("No specific parser found, using text fallback",
			"path", path,
			"error", err,
		)
		parser = textParser
	}

	// If no parser available at all, create single document
	if parser == nil {
		g.logger.Warn("No parser available, treating as single document", "path", path)
		return []schema.Document{schema.NewDocument(content, baseMetadata)}
	}

	// Attempt to chunk the content using the selected parser
	g.logger.Debug("Chunking file", "path", path, "parser", parser.Name())
	chunks, err := parser.Chunk(content, path, nil)
	if err != nil || len(chunks) == 0 {
		// Chunking failed, fall back to single document
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

	// Convert chunks to documents with enhanced metadata
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

// buildChunkMetadata creates comprehensive metadata for a document chunk,
// combining base file metadata with chunk-specific information.
func (g *GitLoader) buildChunkMetadata(baseMetadata map[string]any, chunk schema.CodeChunk, chunkIndex, totalChunks int) map[string]any {
	// Create new metadata map with sufficient capacity
	chunkMetadata := make(map[string]any, len(baseMetadata)+len(chunk.Annotations)+6)

	// Copy base file metadata
	for k, v := range baseMetadata {
		chunkMetadata[k] = v
	}

	// Add chunk-specific metadata for better retrieval context
	chunkMetadata["identifier"] = chunk.Identifier
	chunkMetadata["chunk_type"] = chunk.Type
	chunkMetadata["line_start"] = chunk.LineStart
	chunkMetadata["line_end"] = chunk.LineEnd
	chunkMetadata["chunk_index"] = chunkIndex
	chunkMetadata["total_chunks"] = totalChunks

	// Copy any additional annotations from the chunker
	// These might include language-specific metadata like function names,
	// class names, import statements, etc.
	for k, v := range chunk.Annotations {
		chunkMetadata[k] = v
	}

	return chunkMetadata
}

// shouldSkipDir returns true for common directories that should be excluded
// from document loading. These typically include:
//   - Version control directories (.git)
//   - Dependency directories (vendor, node_modules)
//   - Build output directories (build, dist, target)
//   - IDE and editor directories (.vscode, .idea)
func shouldSkipDir(name string) bool {
	skipDirs := []string{
		// Version control
		".git", ".svn", ".hg",

		// Dependencies
		"vendor", "node_modules", "__pycache__",

		// Build outputs
		"build", "dist", "target", "out", "bin",

		// IDE/Editor
		".vscode", ".idea", ".vs",

		// OS
		".DS_Store", "Thumbs.db",
	}
	return slices.Contains(skipDirs, name)
}

// shouldSkipFile returns true for files that shouldn't be loaded as documents.
// This includes:
//   - Binary files (executables, images, archives)
//   - Very large files (>10MB by default)
//   - System files and temporary files
//
// Note: PDF files are not excluded here as they may have dedicated parsers
// in the ParserRegistry that can handle them appropriately.
func shouldSkipFile(path string, info fs.FileInfo) bool {
	// Skip very large files to avoid memory issues
	const maxFileSize = 10 * 1024 * 1024 // 10MB
	if info.Size() > maxFileSize {
		return true
	}

	// Skip files with binary extensions
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		// Executables and libraries
		".exe": true, ".dll": true, ".so": true, ".dylib": true,

		// Images
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".bmp": true, ".tiff": true, ".svg": true, ".ico": true,

		// Archives and compressed files
		".zip": true, ".tar": true, ".gz": true, ".rar": true,
		".7z": true, ".bz2": true, ".xz": true,

		// Media files
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".wav": true, ".flac": true, ".ogg": true,

		// Office documents (may need specialized parsers)
		".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".ppt": true, ".pptx": true,

		// Other binary formats
		".bin": true, ".dat": true, ".db": true, ".sqlite": true,

		// Note: PDF is intentionally not included here as PDF parsers
		// in the registry should handle these files appropriately
	}

	return binaryExts[ext]
}
