package parsers

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/sevigo/goframe/schema"
)

// ErrPluginNotFound is returned when a plugin is not found
var ErrPluginNotFound = errors.New("language plugin not found")

// registry implements the RegistryService interface
type registry struct {
	plugins    map[string]schema.ParserPlugin // Map of language name to plugin
	extensions map[string]schema.ParserPlugin // Map of file extension to plugin
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewRegistry creates a new language plugin registry
func NewRegistry(logger *slog.Logger) ParserRegistry {
	return &registry{
		plugins:    make(map[string]schema.ParserPlugin),
		extensions: make(map[string]schema.ParserPlugin),
		logger:     logger,
	}
}

// RegisterParser adds a language plugin to the registry
func (r *registry) RegisterParser(plugin schema.ParserPlugin) error {
	if plugin == nil {
		return errors.New("cannot register nil plugin")
	}

	name := plugin.Name()
	if name == "" {
		return errors.New("plugin must have a non-empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin with name %q already registered", name)
	}

	// Register by name
	r.plugins[name] = plugin

	// Register by extensions
	for _, ext := range plugin.Extensions() {
		// Ensure extension has a leading dot
		if ext == "" {
			continue
		}
		if ext[0] != '.' {
			ext = "." + ext
		}
		r.extensions[ext] = plugin
	}

	r.logger.Info("Registered language plugin", "language", name, "extensions", plugin.Extensions())
	return nil
}

// GetPlugin retrieves a plugin by language name
func (r *registry) GetParser(language string) (schema.ParserPlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, ok := r.plugins[language]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, language)
	}
	return plugin, nil
}

// GetPluginForFile returns the appropriate plugin for a file
func (r *registry) GetParserForFile(path string, info fs.FileInfo) (schema.ParserPlugin, error) {
	// First try to get plugin by extension
	ext := filepath.Ext(path)
	if ext != "" {
		plugin, err := r.GetParserForExtension(ext)
		if err == nil {
			return plugin, nil
		}
	}

	// If no plugin found by extension, check each plugin's CanHandle method
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, plugin := range r.plugins {
		if plugin.CanHandle(path, info) {
			return plugin, nil
		}
	}

	return nil, fmt.Errorf("%w for file %s", ErrPluginNotFound, path)
}

// GetPluginForExtension returns a plugin for a file extension
func (r *registry) GetParserForExtension(ext string) (schema.ParserPlugin, error) {
	if ext == "" {
		return nil, fmt.Errorf("%w: empty extension", ErrPluginNotFound)
	}

	// Ensure extension has leading dot
	if ext[0] != '.' {
		ext = "." + ext
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, ok := r.extensions[ext]
	if !ok {
		return nil, fmt.Errorf("%w for extension %s", ErrPluginNotFound, ext)
	}

	return plugin, nil
}

// GetAllParsers returns all registered plugins
func (r *registry) GetAllParsers() []schema.ParserPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]schema.ParserPlugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}
