package csv

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// CSVPlugin implements model.ParserPlugin for CSV files
type CSVPlugin struct {
	logger *slog.Logger
}

// NewCSVPlugin creates a new CSV file parser plugin
func NewCSVPlugin(logger *slog.Logger) model.ParserPlugin {
	return &CSVPlugin{
		logger: logger,
	}
}

// Name returns "csv" as the language name
func (p *CSVPlugin) Name() string {
	return "csv"
}

// Extensions returns file extensions for CSV files
func (p *CSVPlugin) Extensions() []string {
	return []string{".csv", ".tsv"}
}

// CanHandle determines if this plugin can process the given file
func (p *CSVPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".csv" || ext == ".tsv"
}
