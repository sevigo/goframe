package json

import (
	"io/fs"
	"log/slog"
	"path/filepath"

	model "github.com/sevigo/goframe/schema"
)

// JSONPlugin implements language.Plugin for JSON files
type JSONPlugin struct {
	logger *slog.Logger
}

// NewJSONPlugin creates a new JSON language plugin
func NewJSONPlugin(logger *slog.Logger) model.ParserPlugin {
	return &JSONPlugin{
		logger: logger,
	}
}

// Name returns "json" as the language name
func (p *JSONPlugin) Name() string {
	return "json"
}

// Extensions returns file extensions for JSON
func (p *JSONPlugin) Extensions() []string {
	return []string{".json"}
}

// CanHandle determines if this plugin can process the given file
func (p *JSONPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := filepath.Ext(path)
	return ext == ".json"
}
