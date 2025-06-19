package golang

import (
	"go/ast"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/sevigo/goframe/schema"
)

type GoPlugin struct {
	logger *slog.Logger
}

func NewGoPlugin(logger *slog.Logger) schema.ParserPlugin {
	return &GoPlugin{
		logger: logger,
	}
}

func (p *GoPlugin) Name() string {
	return "go"
}

func (p *GoPlugin) Extensions() []string {
	return []string{".go"}
}

func (p *GoPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := filepath.Ext(path)
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	return ext == ".go"
}

// getVisibility determines if a Go identifier is public or private
func (p *GoPlugin) getVisibility(name string) string {
	if ast.IsExported(name) {
		return "public"
	}
	return "private"
}
