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

// GitLoader loads documents from a git repository on the local file system,
// filtering out common directories and binary files.
type GitLoader struct {
	path           string
	parserRegistry parsers.ParserRegistry
}

// NewGit creates a new git repository loader for the specified path.
func NewGit(path string, registry parsers.ParserRegistry) *GitLoader {
	return &GitLoader{
		path:           path,
		parserRegistry: registry,
	}
}

// Load walks the repository path recursively, loading all valid text files
// into Documents while skipping common build directories and binary files.
func (g *GitLoader) Load(ctx context.Context) ([]schema.Document, error) {
	var documents []schema.Document

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

		if shouldSkipFile(path) {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip files that can't be read
		}

		relPath, err := filepath.Rel(g.path, path)
		if err != nil {
			slog.Warn("Could not get relative path", "error", err, "full_path", path)
			relPath = path
		}
		metadata := map[string]any{
			"source": relPath,
		}

		documents = append(documents, schema.NewDocument(string(content), metadata))
		return nil
	})

	return documents, err
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
func shouldSkipFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true,
		".png": true, ".jpg": true, ".jpeg": true,
		".gif": true, ".bmp": true, ".tiff": true,
		".zip": true, ".tar": true, ".gz": true,
		".pdf": true, ".doc": true, ".docx": true,
	}
	return binaryExts[ext]
}
