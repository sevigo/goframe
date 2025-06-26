package terraform

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/sevigo/goframe/schema"
)

// Chunk performs semantic chunking on Terraform files with enhanced error handling.
func (p *TerraformPlugin) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	if strings.TrimSpace(content) == "" {
		p.logger.Debug("Empty content for chunking", "path", path)
		return []schema.CodeChunk{}, nil
	}

	file, diags := hclsyntax.ParseConfig([]byte(content), path, hcl.Pos{Line: 1, Column: 1})
	hasParseErrors := diags.HasErrors()

	if hasParseErrors {
		p.logger.Warn("Parse errors during chunking", "path", path, "errors", diags.Error())

		if file == nil {
			return []schema.CodeChunk{
				{
					Content:    content,
					LineStart:  1,
					LineEnd:    len(strings.Split(content, "\n")),
					Type:       "terraform_file",
					Identifier: filepath.Base(path),
					Annotations: map[string]string{
						"parse_error": "true",
						"file_type":   "terraform",
					},
				},
			}, nil
		}
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("HCL file %s has an invalid body type", path)
	}

	var chunks []schema.CodeChunk
	contentLines := strings.Split(content, "\n")

	for _, block := range body.Blocks {
		chunk, err := p.createChunkFromBlock(block, content, contentLines)
		if err != nil {
			p.logger.Warn("Skipping HCL block due to creation error",
				"path", path,
				"block_type", block.Type,
				"error", err)
			continue
		}

		if hasParseErrors {
			if chunk.Annotations == nil {
				chunk.Annotations = make(map[string]string)
			}
			chunk.Annotations["parse_error"] = "true"
		}

		chunks = append(chunks, chunk)
	}

	if hasParseErrors && len(chunks) == 0 {
		chunks = append(chunks, schema.CodeChunk{
			Content:    content,
			LineStart:  1,
			LineEnd:    len(contentLines),
			Type:       "terraform_file",
			Identifier: filepath.Base(path),
			Annotations: map[string]string{
				"parse_error": "true",
				"file_type":   "terraform",
			},
		})
	}

	return chunks, nil
}

// createChunkFromBlock creates a chunk with enhanced content extraction and validation.
func (p *TerraformPlugin) createChunkFromBlock(block *hclsyntax.Block, originalContent string, contentLines []string) (schema.CodeChunk, error) {
	blockRange := block.Range()

	if blockRange.Start.Byte < 0 || blockRange.End.Byte > len(originalContent) || blockRange.Start.Byte > blockRange.End.Byte {
		return schema.CodeChunk{}, fmt.Errorf("invalid block range: start=%d, end=%d, content_length=%d",
			blockRange.Start.Byte, blockRange.End.Byte, len(originalContent))
	}

	content := originalContent[blockRange.Start.Byte:blockRange.End.Byte]
	identifier := buildIdentifier(block)
	annotations := p.createBlockAnnotations(block, content)

	chunk := schema.CodeChunk{
		Content:     content,
		LineStart:   blockRange.Start.Line,
		LineEnd:     blockRange.End.Line,
		Type:        block.Type,
		Identifier:  identifier,
		Annotations: annotations,
	}

	return chunk, nil
}

// createBlockAnnotations creates comprehensive annotations for a block.
func (p *TerraformPlugin) createBlockAnnotations(block *hclsyntax.Block, content string) map[string]string {
	annotations := map[string]string{
		"block_type": block.Type,
		"line_count": strconv.Itoa(strings.Count(content, "\n") + 1),
	}

	switch block.Type {
	case "resource":
		if len(block.Labels) >= 1 {
			annotations["resource_type"] = block.Labels[0]
		}
		if len(block.Labels) >= 2 {
			annotations["resource_name"] = block.Labels[1]
		}
	case "data":
		if len(block.Labels) >= 1 {
			annotations["data_type"] = block.Labels[0]
		}
		if len(block.Labels) >= 2 {
			annotations["data_name"] = block.Labels[1]
		}
	case "module":
		if len(block.Labels) >= 2 {
			annotations["module_name"] = block.Labels[0]
		}
		if sourceAttr, exists := block.Body.Attributes["source"]; exists {
			if val := p.safeEvalAttribute(sourceAttr); val != "" {
				annotations["module_source"] = strings.Trim(val, "\"'")
			}
		}
	case "variable", "output":
		if len(block.Labels) >= 1 {
			annotations[block.Type+"_name"] = block.Labels[0]
		}
	case "locals":
		annotations["local_count"] = strconv.Itoa(len(block.Body.Attributes))
	case "provider":
		if len(block.Labels) >= 1 {
			annotations["provider_name"] = block.Labels[0]
		}
	}

	annotations["attribute_count"] = strconv.Itoa(len(block.Body.Attributes))
	annotations["nested_block_count"] = strconv.Itoa(len(block.Body.Blocks))

	return annotations
}

// buildIdentifier creates an enhanced identifier with better handling.
func buildIdentifier(block *hclsyntax.Block) string {
	switch block.Type {
	case "resource", "data":
		if len(block.Labels) >= 2 {
			return fmt.Sprintf("%s.%s", block.Labels[0], block.Labels[1])
		} else if len(block.Labels) >= 1 {
			return fmt.Sprintf("%s.unnamed", block.Labels[0])
		}
		return fmt.Sprintf("%s.invalid", block.Type)
	case "module", "variable", "output":
		if len(block.Labels) >= 1 {
			return fmt.Sprintf("%s.%s", block.Type, block.Labels[0])
		}
		return fmt.Sprintf("%s.unnamed", block.Type)
	case "provider":
		if len(block.Labels) >= 1 {
			provider := block.Labels[0]
			if _, exists := block.Body.Attributes["alias"]; exists {
				return fmt.Sprintf("provider.%s.alias", provider)
			}
			return fmt.Sprintf("provider.%s", provider)
		}
		return "provider.unnamed"
	case "terraform":
		return "terraform"
	case "locals":
		return "locals"
	default:
		if len(block.Labels) > 0 {
			parts := append([]string{block.Type}, block.Labels...)
			return strings.Join(parts, ".")
		}
		return block.Type
	}
}
