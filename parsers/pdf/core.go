package pdf

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// PDFPlugin implements model.ParserPlugin for PDF files
type PDFPlugin struct {
	logger *slog.Logger
}

// NewPDFPlugin creates a new PDF language plugin
func NewPDFPlugin(logger *slog.Logger) schema.ParserPlugin {
	return &PDFPlugin{
		logger: logger,
	}
}

// Name returns "pdf".
func (p *PDFPlugin) Name() string {
	return "pdf"
}

// Extensions returns the file extensions handled by this plugin.
func (p *PDFPlugin) Extensions() []string {
	return []string{".pdf"}
}

// CanHandle returns true for PDF files.
func (p *PDFPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".pdf"
}

// IsGenerated returns false; PDF files are not source-generated.
func (p *PDFPlugin) IsGenerated(content string, path string) bool {
	return false
}

// ExtractUsedSymbols returns nil; not applicable for PDF.
func (p *PDFPlugin) ExtractUsedSymbols(content string) []string {
	return nil
}
