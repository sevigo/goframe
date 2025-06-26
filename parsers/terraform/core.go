package terraform

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sevigo/goframe/schema"
)

type TerraformPlugin struct {
	logger *slog.Logger
}

func NewTerraformPlugin(logger *slog.Logger) schema.ParserPlugin {
	if logger == nil {
		logger = slog.Default()
	}
	return &TerraformPlugin{
		logger: logger,
	}
}

func (p *TerraformPlugin) Name() string {
	return "terraform"
}

func (p *TerraformPlugin) Extensions() []string {
	return []string{".tf", ".tfvars", ".hcl"}
}

func (p *TerraformPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	return slices.Contains(p.Extensions(), ext)
}
