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
type Loader interface {
	Load(ctx context.Context) ([]schema.Document, error)
}

// GitLoader loads and chunks documents from a git repository on the local file system.
// It uses a ParserRegistry to apply language-specific chunking for supported file types
// and a fallback text chunker for others.
type GitLoader struct {
	path           string
	parserRegistry parsers.ParserRegistry
}

// NewGit creates a new git repository loader for the specified path.
// It requires a parserRegistry to correctly chunk files based on their language.
func NewGit(path string, registry parsers.ParserRegistry) *GitLoader {
	return &GitLoader{
		path:           path,
		parserRegistry: registry,
	}
}

// Load walks the repository path, finds the appropriate language parser for each file,
// chunks the content, and returns a list of semantically meaningful documents.
// It skips common build directories and binary files.
func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	var documents []schema.Document

	// Get the fallback text parser once for efficiency. This will be used for any
	// file type that doesn't have a specific language plugin.
	textParser, _ := g.parserRegistry.GetParser("text")

	err := filepath.WalkDir(g.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("Skipping unreadable file", "path", path, "error", err)
			return err
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			slog.Warn("Could not get file info, skipping file.", "path", path, "error", err)
			return nil
		}

		if shouldSkipFile(path, fileInfo) {
			slog.Debug("Skipping binary or excluded file", "path", path)
			return nil
		}

		// Read file content.
		contentBytes, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("Skipping file that cannot be read", "path", path, "error", err)
			return nil
		}
		content := string(contentBytes)

		// Get base metadata for the file.
		relPath, err := filepath.Rel(g.path, path)
		if err != nil {
			slog.Warn("Could not get relative path", "error", err, "full_path", path)
			relPath = path
		}
		baseMetadata := map[string]any{
			"source": relPath,
		}

		// Find the appropriate language-specific parser for the file.
		plugin, err := g.parserRegistry.GetParserForFile(path, fileInfo)
		if err != nil {
			// If no specific parser is found, use the text parser as a fallback.
			slog.Debug("No specific parser found for file, using fallback text parser.", "path", path)
			plugin = textParser
		}

		// If even the fallback is not available, treat the file as a single document.
		if plugin == nil {
			slog.Warn("No suitable parser, including fallback, found for file. Treating as a single document.", "path", path)
			documents = append(documents, schema.NewDocument(content, baseMetadata))
			return nil
		}

		// Use the selected plugin to chunk the content.
		// A nil options object should be handled gracefully by chunkers to use their defaults.
		chunks, chunkErr := plugin.Chunk(content, path, nil)
		if chunkErr != nil || len(chunks) == 0 {
			// If chunking fails or returns no chunks, fall back to creating a single document.
			if chunkErr != nil {
				slog.Warn("Language-aware chunking failed, treating as a single document.", "path", path, "plugin", plugin.Name(), "error", chunkErr)
			}
			documents = append(documents, schema.NewDocument(content, baseMetadata))
			return nil
		}

		// Convert each semantically divided chunk into a schema.Document.
		for _, chunk := range chunks {
			// Create a new metadata map for each chunk to ensure they are independent.
			chunkMetadata := make(map[string]any, len(baseMetadata)+len(chunk.Annotations)+4)
			for k, v := range baseMetadata {
				chunkMetadata[k] = v
			}

			// Add chunk-specific metadata for better context during retrieval.
			chunkMetadata["identifier"] = chunk.Identifier
			chunkMetadata["chunk_type"] = chunk.Type
			chunkMetadata["line_start"] = chunk.LineStart
			chunkMetadata["line_end"] = chunk.LineEnd

			// Copy any other annotations from the chunker.
			for k, v := range chunk.Annotations {
				chunkMetadata[k] = v
			}

			documents = append(documents, schema.NewDocument(chunk.Content, chunkMetadata))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	slog.Info("GitLoader finished loading.", "total_documents", len(documents))
	return documents, nil
}

// shouldSkipDir returns true for common directories that should be excluded
// from document loading (build artifacts, dependencies, version control).
func shouldSkipDir(name string) bool {
	skipDirs := []string{
		".git", "vendor", "node_modules",
		"build", "dist", "target",
	}
	return slices.Contains(skipDirs, name)
}

// shouldSkipFile returns true for binary files and other non-text files
// that shouldn't be loaded as documents.
func shouldSkipFile(path string, info fs.FileInfo) bool {
	if info.Size() > 10*1024*1024 {
		return true
	}

	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true,
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".bmp": true, ".tiff": true,
		".zip": true, ".tar": true, ".gz": true,
		// Note: The PDF parser handles .pdf, so it is not excluded here.
		// ".pdf": true,
		".doc": true, ".docx": true,
	}
	return binaryExts[ext]
}
