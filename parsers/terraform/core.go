package terraform

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// TerraformPlugin implements schema.ParserPlugin for Terraform/HCL files.
type TerraformPlugin struct {
	logger *slog.Logger
}

// NewTerraformPlugin creates a new Terraform parser plugin.
func NewTerraformPlugin(logger *slog.Logger) schema.ParserPlugin {
	if logger == nil {
		logger = slog.Default()
	}
	return &TerraformPlugin{
		logger: logger,
	}
}

// Name returns "terraform".
func (p *TerraformPlugin) Name() string {
	return "terraform"
}

// Extensions returns the file extensions handled by this plugin.
func (p *TerraformPlugin) Extensions() []string {
	return []string{".tf", ".tfvars", ".hcl"}
}

// CanHandle returns true for Terraform/HCL files.
func (p *TerraformPlugin) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	return slices.Contains(p.Extensions(), ext)
}

// IsGenerated returns false; Terraform files are always hand-written.
func (p *TerraformPlugin) IsGenerated(content string, path string) bool {
	return false
}

// ExtractUsedSymbols returns nil; not yet implemented for Terraform.
func (p *TerraformPlugin) ExtractUsedSymbols(content string) []string {
	// TODO: Implement more robust HCL parsing
	return nil
}
