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

func (p *PDFPlugin) Name() string {
	return "pdf"
}

func (p *PDFPlugin) Extensions() []string {
	return []string{".pdf"}
}

func (p *PDFPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".pdf"
}
