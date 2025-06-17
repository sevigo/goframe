package yaml

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// YamlPlugin implements language.Plugin for YAML files
type YamlPlugin struct {
	logger *slog.Logger
}

// NewYamlPlugin creates a new YAML language plugin
func NewYamlPlugin(logger *slog.Logger) schema.ParserPlugin {
	return &YamlPlugin{
		logger: logger,
	}
}

// Name returns "yaml" as the language name
func (p *YamlPlugin) Name() string {
	return "yaml"
}

// Extensions returns file extensions for YAML
func (p *YamlPlugin) Extensions() []string {
	return []string{".yaml", ".yml"}
}

// CanHandle determines if this plugin can process the given file
func (p *YamlPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
