package terraform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/sevigo/goframe/schema"
)

// ExtractMetadata fully implements metadata extraction for Terraform files.
func (p *TerraformPlugin) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	metadata := schema.FileMetadata{
		Language:    p.Name(),
		FilePath:    path,
		Imports:     []string{},
		Definitions: []schema.CodeEntityDefinition{},
		Symbols:     []schema.CodeSymbol{},
		Properties:  make(map[string]string),
	}

	if strings.TrimSpace(content) == "" {
		p.logger.Debug("Empty content for Terraform file", "path", path)
		return metadata, nil
	}

	file, diags := hclsyntax.ParseConfig([]byte(content), path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		p.logger.Warn("Parse errors in Terraform file", "path", path, "errors", diags.Error())
		metadata.Properties["parse_errors"] = strconv.Itoa(len(diags))

		if file == nil {
			return metadata, fmt.Errorf("failed to parse HCL for metadata extraction: %w", diags)
		}
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return metadata, fmt.Errorf("HCL file %s has an invalid body type", path)
	}

	counters := make(map[string]int)
	resourceTypes := make(map[string]int)
	providers := make(map[string]bool)

	for _, block := range body.Blocks {
		p.processBlockForMetadata(block, &metadata, counters, resourceTypes, providers)
	}

	p.storeAnalysisResults(&metadata, counters, resourceTypes, providers)

	return metadata, nil
}

func (p *TerraformPlugin) processBlockForMetadata(
	block *hclsyntax.Block,
	metadata *schema.FileMetadata,
	counters map[string]int,
	resourceTypes map[string]int,
	providers map[string]bool,
) {
	counters[block.Type]++
	blockRange := block.Range()
	identifier := buildIdentifier(block)

	def := schema.CodeEntityDefinition{
		Type:       block.Type,
		Name:       identifier,
		LineStart:  blockRange.Start.Line,
		LineEnd:    blockRange.End.Line,
		Visibility: "public",
	}

	sym := schema.CodeSymbol{
		Name:      identifier,
		Type:      block.Type,
		LineStart: blockRange.Start.Line,
		LineEnd:   blockRange.End.Line,
		IsExport:  false,
	}

	// Enhanced block processing
	switch block.Type {
	case "resource":
		p.processResourceBlock(block, &def, resourceTypes, providers)
	case "data":
		p.processDataBlock(block, &def, resourceTypes)
	case "module":
		p.processModuleBlock(block, &def, metadata)
	case "variable":
		p.processVariableBlock(block, &def)
	case "output":
		p.processOutputBlock(block, &def)
		sym.IsExport = true
	case "locals":
		p.processLocalsBlock(block, &def, &sym, metadata)
	case "terraform":
		p.processTerraformBlock(block, metadata)
		return
	case "provider":
		p.processProviderBlock(block, &def, providers)
	}

	metadata.Definitions = append(metadata.Definitions, def)
	metadata.Symbols = append(metadata.Symbols, sym)
}

// processResourceBlock handles resource block analysis.
func (p *TerraformPlugin) processResourceBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
	resourceTypes map[string]int,
	providers map[string]bool,
) {
	if len(block.Labels) >= 1 {
		resourceType := block.Labels[0]
		resourceTypes[resourceType]++

		if parts := strings.SplitN(resourceType, "_", 2); len(parts) > 0 {
			providers[parts[0]] = true
		}

		def.Signature = fmt.Sprintf("type = %s", resourceType)

		var sigParts []string
		for _, attrName := range []string{"name", "id", "ami", "instance_type", "image"} {
			if attr, exists := block.Body.Attributes[attrName]; exists {
				if val := p.safeEvalAttribute(attr); val != "" {
					sigParts = append(sigParts, fmt.Sprintf("%s = %s", attrName, val))
				}
			}
		}
		if len(sigParts) > 0 {
			def.Signature += ", " + strings.Join(sigParts, ", ")
		}
	}
}

func (p *TerraformPlugin) processDataBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
	resourceTypes map[string]int,
) {
	if len(block.Labels) >= 1 {
		dataType := block.Labels[0]
		resourceTypes["data."+dataType]++
		def.Signature = fmt.Sprintf("type = %s", dataType)
	}
}

func (p *TerraformPlugin) processModuleBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
	metadata *schema.FileMetadata,
) {
	if sourceAttr, exists := block.Body.Attributes["source"]; exists {
		if val := p.safeEvalAttribute(sourceAttr); val != "" {
			source := strings.Trim(val, "\"'")
			metadata.Imports = append(metadata.Imports, source)
			def.Signature = fmt.Sprintf("source = %s", val)
		}
	}

	if versionAttr, exists := block.Body.Attributes["version"]; exists {
		if val := p.safeEvalAttribute(versionAttr); val != "" {
			if def.Signature != "" {
				def.Signature += ", "
			}
			def.Signature += fmt.Sprintf("version = %s", val)
		}
	}
}

func (p *TerraformPlugin) processVariableBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
) {
	var parts []string

	if typeAttr, exists := block.Body.Attributes["type"]; exists {
		if val := p.safeEvalAttribute(typeAttr); val != "" {
			parts = append(parts, fmt.Sprintf("type = %s", val))
		}
	}

	if _, exists := block.Body.Attributes["default"]; exists {
		parts = append(parts, "has_default = true")
	}

	if descAttr, exists := block.Body.Attributes["description"]; exists {
		if val := p.safeEvalAttribute(descAttr); val != "" {
			parts = append(parts, fmt.Sprintf("description = %s", val))
		}
	}

	def.Signature = strings.Join(parts, ", ")
}

func (p *TerraformPlugin) processOutputBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
) {
	var parts []string

	if descAttr, exists := block.Body.Attributes["description"]; exists {
		if val := p.safeEvalAttribute(descAttr); val != "" {
			parts = append(parts, fmt.Sprintf("description = %s", val))
		}
	}

	if sensAttr, exists := block.Body.Attributes["sensitive"]; exists {
		if val := p.safeEvalAttribute(sensAttr); val == "true" {
			parts = append(parts, "sensitive = true")
		}
	}

	def.Signature = strings.Join(parts, ", ")
}

// processLocalsBlock handles locals block analysis.
func (p *TerraformPlugin) processLocalsBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
	sym *schema.CodeSymbol,
	metadata *schema.FileMetadata,
) {
	localCount := len(block.Body.Attributes)
	def.Signature = fmt.Sprintf("local_values = %d", localCount)

	for name := range block.Body.Attributes {
		localSym := schema.CodeSymbol{
			Name:      fmt.Sprintf("local.%s", name),
			Type:      "local",
			LineStart: sym.LineStart,
			LineEnd:   sym.LineEnd,
			IsExport:  false,
		}
		metadata.Symbols = append(metadata.Symbols, localSym)
	}
}

// processTerraformBlock handles terraform configuration block.
func (p *TerraformPlugin) processTerraformBlock(block *hclsyntax.Block, metadata *schema.FileMetadata) {
	if reqVerAttr, exists := block.Body.Attributes["required_version"]; exists {
		if val := p.safeEvalAttribute(reqVerAttr); val != "" {
			metadata.Properties["required_version"] = strings.Trim(val, "\"'")
		}
	}

	if reqProvBlock := p.findNestedBlock(block.Body, "required_providers"); reqProvBlock != nil {
		var providers []string
		for name := range reqProvBlock.Body.Attributes {
			providers = append(providers, name)
		}
		if len(providers) > 0 {
			metadata.Properties["required_providers"] = strings.Join(providers, ",")
		}
	}
}

// processProviderBlock handles provider block analysis.
func (p *TerraformPlugin) processProviderBlock(
	block *hclsyntax.Block,
	def *schema.CodeEntityDefinition,
	providers map[string]bool,
) {
	if len(block.Labels) >= 1 {
		providerName := block.Labels[0]
		providers[providerName] = true
		def.Signature = fmt.Sprintf("provider = %s", providerName)

		if aliasAttr, exists := block.Body.Attributes["alias"]; exists {
			if val := p.safeEvalAttribute(aliasAttr); val != "" {
				def.Signature += fmt.Sprintf(", alias = %s", val)
			}
		}
	}
}

func (p *TerraformPlugin) safeEvalAttribute(attr *hclsyntax.Attribute) string {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return "<expression>"
	}

	switch val.Type() {
	case cty.String:
		return fmt.Sprintf("\"%s\"", val.AsString())
	case cty.Number:
		var num float64
		if err := gocty.FromCtyValue(val, &num); err == nil {
			if num == float64(int64(num)) {
				return strconv.FormatInt(int64(num), 10)
			}
			return strconv.FormatFloat(num, 'f', -1, 64)
		}
		return val.GoString()
	case cty.Bool:
		if val.True() {
			return "true"
		}
		return "false"
	default:
		return val.GoString()
	}
}

func (p *TerraformPlugin) findNestedBlock(body *hclsyntax.Body, blockType string) *hclsyntax.Block {
	for _, block := range body.Blocks {
		if block.Type == blockType {
			return block
		}
	}
	return nil
}

func (p *TerraformPlugin) storeAnalysisResults(
	metadata *schema.FileMetadata,
	counters map[string]int,
	resourceTypes map[string]int,
	providers map[string]bool,
) {
	for key, count := range counters {
		metadata.Properties[key+"_count"] = strconv.Itoa(count)
	}

	if len(resourceTypes) > 0 {
		metadata.Properties["resource_types_count"] = strconv.Itoa(len(resourceTypes))
		var types []string
		for rType := range resourceTypes {
			types = append(types, rType)
		}
		metadata.Properties["resource_types"] = strings.Join(types, ",")
	}

	if len(providers) > 0 {
		metadata.Properties["providers_count"] = strconv.Itoa(len(providers))
		var provList []string
		for prov := range providers {
			provList = append(provList, prov)
		}
		metadata.Properties["providers"] = strings.Join(provList, ",")
	}

	complexity := len(metadata.Definitions) + len(metadata.Imports)
	metadata.Properties["complexity_score"] = strconv.Itoa(complexity)
}
