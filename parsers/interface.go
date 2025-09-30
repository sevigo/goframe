package parsers

import (
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/sevigo/goframe/parsers/csv"
	"github.com/sevigo/goframe/parsers/golang"
	"github.com/sevigo/goframe/parsers/json"
	"github.com/sevigo/goframe/parsers/markdown"
	"github.com/sevigo/goframe/parsers/pdf"
	"github.com/sevigo/goframe/parsers/terraform"
	"github.com/sevigo/goframe/parsers/text"
	"github.com/sevigo/goframe/parsers/typescript"
	"github.com/sevigo/goframe/parsers/yaml"
	"github.com/sevigo/goframe/schema"
)

// ParserRegistry tracks registered language plugins
type ParserRegistry interface {
	RegisterParser(plugin schema.ParserPlugin) error
	GetParser(language string) (schema.ParserPlugin, error)
	GetParserForFile(path string, info fs.FileInfo) (schema.ParserPlugin, error)
	GetParserForExtension(ext string) (schema.ParserPlugin, error)
	GetAllParsers() []schema.ParserPlugin
}

// RegisterLanguagePlugins initializes and populates a language registry
func RegisterLanguagePlugins(logger *slog.Logger) (ParserRegistry, error) {
	registry := NewRegistry(logger)

	pluginFactories := map[string]func(*slog.Logger) schema.ParserPlugin{
		"go":       golang.NewGoPlugin,
		"markdown": markdown.NewMarkdownPlugin,
		"json":     json.NewJSONPlugin,
		"yaml":     yaml.NewYamlPlugin,
		"pdf":      pdf.NewPDFPlugin,
		"text":     text.NewTextPlugin,
		"csv":      csv.NewCSVPlugin,
		"tf":       terraform.NewTerraformPlugin,
		"ts":       typescript.NewTypeScriptPlugin,
	}

	pluginsToRegister := make([]string, 0, len(pluginFactories))
	for name := range pluginFactories {
		pluginsToRegister = append(pluginsToRegister, name)
	}

	for _, name := range pluginsToRegister {
		factory, exists := pluginFactories[name]
		if !exists {
			logger.Warn("Plugin not available", "plugin", name)
			continue
		}

		pluginLogger := logger.With("plugin", name)
		plugin := factory(pluginLogger)

		if err := registry.RegisterParser(plugin); err != nil {
			return registry, fmt.Errorf("failed to register plugin %s: %w", name, err)
		}
	}

	logger.Info("Language plugins registered", "count", len(registry.GetAllParsers()))
	return registry, nil
}
