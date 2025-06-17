// core.go - Main plugin file with goldmark integration
package markdown

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	model "github.com/sevigo/goframe/schema"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

const frontMatterSeparator = "---"

// MarkdownPlugin implements model.ParserPlugin for Markdown files using goldmark
type MarkdownPlugin struct {
	logger   *slog.Logger
	markdown goldmark.Markdown
}

// NewMarkdownPlugin creates a new Markdown language plugin with goldmark
func NewMarkdownPlugin(logger *slog.Logger) model.ParserPlugin {
	plugin := &MarkdownPlugin{
		logger: logger,
	}

	// Initialize goldmark parser
	plugin.markdown = plugin.initializeGoldmark()

	return plugin
}

// initializeGoldmark creates and configures the goldmark parser
func (p *MarkdownPlugin) initializeGoldmark() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,   // GitHub Flavored Markdown
			extension.Table, // Tables
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // Automatically generate heading IDs
		),
	)
}

// Name returns "markdown" as the language name
func (p *MarkdownPlugin) Name() string {
	return "markdown"
}

// Extensions returns file extensions for Markdown
func (p *MarkdownPlugin) Extensions() []string {
	return []string{".md", ".markdown"}
}

// CanHandle determines if this plugin can process the given file
func (p *MarkdownPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}
