package text

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// TextPlugin implements model.ParserPlugin for plain text files
type TextPlugin struct {
	logger *slog.Logger
}

// NewTextPlugin creates a new text file parser plugin
func NewTextPlugin(logger *slog.Logger) schema.ParserPlugin {
	return &TextPlugin{
		logger: logger,
	}
}

// Name returns "text" as the language name
func (p *TextPlugin) Name() string {
	return "text"
}

// Extensions returns file extensions for text files
func (p *TextPlugin) Extensions() []string {
	return []string{".txt", ".text", ".log", ".readme", ".changelog", ".license"}
}

// CanHandle determines if this plugin can process the given file
func (p *TextPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))

	// Handle known text extensions
	for _, validExt := range p.Extensions() {
		if ext == validExt {
			return true
		}
	}

	// Handle files without extensions that are likely text
	if ext == "" {
		baseName := strings.ToLower(filepath.Base(path))
		textFiles := []string{"readme", "license", "changelog", "authors", "contributors", "makefile"}
		for _, textFile := range textFiles {
			if baseName == textFile {
				return true
			}
		}
	}

	return false
}
