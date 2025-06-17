// extractor - Fixed version
package yaml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	model "github.com/sevigo/goframe/schema"
)

// ExtractMetadata extracts language-specific metadata from YAML content
func (p *YamlPlugin) ExtractMetadata(content string, path string) (model.FileMetadata, error) {
	metadata := model.FileMetadata{
		Language:    "yaml",
		Imports:     []string{}, // YAML doesn't have imports traditionally
		Definitions: []model.CodeEntityDefinition{},
		Symbols:     []model.CodeSymbol{},
		Properties:  map[string]string{},
	}

	// Parse YAML to analyze structure
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(content), &node); err != nil {
		// Invalid YAML
		metadata.Properties["error"] = err.Error()
		metadata.Properties["valid"] = "false"
		return metadata, err
	}

	metadata.Properties["valid"] = "true"

	// Analyze the YAML structure
	p.analyzeYamlMetadata(&node, "", &metadata)

	// Add file-level statistics
	p.addYamlStatistics(&node, content, &metadata)

	// Extract special YAML features
	p.extractYamlFeatures(content, &metadata)

	return metadata, nil
}

// analyzeYamlMetadata recursively analyzes YAML structure for metadata
func (p *YamlPlugin) analyzeYamlMetadata(node *yaml.Node, path string, metadata *model.FileMetadata) {
	switch node.Kind {
	case yaml.DocumentNode:
		// Process document content
		if len(node.Content) > 0 {
			p.analyzeYamlMetadata(node.Content[0], "root", metadata)
		}

	case yaml.MappingNode:
		p.analyzeMappingNode(node, path, metadata)

	case yaml.SequenceNode:
		p.analyzeSequenceNode(node, path, metadata)

	case yaml.ScalarNode:
		p.analyzeScalarNode(node, path, metadata)

	case yaml.AliasNode:
		// Track alias usage
		metadata.Properties["has_aliases"] = "true"
	}
}

// analyzeMappingNode analyzes YAML mapping nodes
func (p *YamlPlugin) analyzeMappingNode(node *yaml.Node, path string, metadata *model.FileMetadata) {
	// Process key-value pairs
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			key := keyNode.Value
			fullPath := key
			if path != "root" && path != "" {
				fullPath = path + "." + key
			}

			// Add key as a symbol
			symbol := model.CodeSymbol{
				Name:      key,
				Type:      p.getYamlNodeType(valueNode),
				LineStart: keyNode.Line,
				LineEnd:   p.getNodeEndLine(valueNode),
				IsExport:  true, // All YAML keys are "exported"
			}
			metadata.Symbols = append(metadata.Symbols, symbol)

			// Add key as a definition for complex types
			if p.isComplexYamlNode(valueNode) {
				def := model.CodeEntityDefinition{
					Type:       p.getDefinitionType(valueNode),
					Name:       key,
					LineStart:  keyNode.Line,
					LineEnd:    p.getNodeEndLine(valueNode),
					Visibility: "public",
					Signature:  p.getYamlNodeSignature(valueNode),
				}
				metadata.Definitions = append(metadata.Definitions, def)
			}

			// Recurse into nested structures
			p.analyzeYamlMetadata(valueNode, fullPath, metadata)
		}
	}
}

// analyzeSequenceNode analyzes YAML sequence nodes
func (p *YamlPlugin) analyzeSequenceNode(node *yaml.Node, path string, metadata *model.FileMetadata) {
	// Analyze sequence items (limit to avoid overwhelming metadata)
	for i, itemNode := range node.Content {
		if i >= 10 { // Only analyze first 10 items
			break
		}
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		p.analyzeYamlMetadata(itemNode, itemPath, metadata)
	}
}

// analyzeScalarNode analyzes YAML scalar nodes
func (p *YamlPlugin) analyzeScalarNode(node *yaml.Node, path string, metadata *model.FileMetadata) {
	// Could analyze scalar patterns, but for now we skip
	// Future: detect URLs, emails, version numbers, etc.
}

// addYamlStatistics adds overall statistics about the YAML
func (p *YamlPlugin) addYamlStatistics(node *yaml.Node, content string, metadata *model.FileMetadata) {
	lines := strings.Split(content, "\n")
	metadata.Properties["total_lines"] = strconv.Itoa(len(lines))

	var rootType string
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		contentNode := node.Content[0]
		rootType = p.getYamlNodeType(contentNode)
	} else {
		rootType = p.getYamlNodeType(node)
	}
	metadata.Properties["root_type"] = rootType

	// Count different elements
	stats := p.countYamlElements(node)
	for key, value := range stats {
		metadata.Properties[key] = strconv.Itoa(value)
	}

	// Calculate depth
	depth := p.calculateYamlDepth(node)
	metadata.Properties["max_depth"] = strconv.Itoa(depth)

	// Estimate size
	metadata.Properties["size_bytes"] = strconv.Itoa(len(content))

	// Count lines by type
	lineCounts := p.analyzeLineTypes(content)
	for key, value := range lineCounts {
		metadata.Properties[key] = strconv.Itoa(value)
	}
}

// extractYamlFeatures extracts special YAML features
func (p *YamlPlugin) extractYamlFeatures(content string, metadata *model.FileMetadata) {
	// Check for YAML directives
	if strings.Contains(content, "%YAML") {
		metadata.Properties["has_yaml_directive"] = "true"
	}
	if strings.Contains(content, "%TAG") {
		metadata.Properties["has_tag_directive"] = "true"
	}

	// Check for document separators
	if strings.Contains(content, "---") {
		metadata.Properties["has_document_separator"] = "true"
	}
	if strings.Contains(content, "...") {
		metadata.Properties["has_document_terminator"] = "true"
	}

	// Check for multi-line strings
	if strings.Contains(content, "|") || strings.Contains(content, ">") {
		metadata.Properties["has_multiline_strings"] = "true"
	}

	// Check for anchors and aliases
	anchorRegex := regexp.MustCompile(`&\w+`)
	aliasRegex := regexp.MustCompile(`\*\w+`)
	if anchorRegex.MatchString(content) {
		metadata.Properties["has_anchors"] = "true"
	}
	if aliasRegex.MatchString(content) {
		metadata.Properties["has_aliases"] = "true"
	}

	// Detect common configuration patterns
	p.detectConfigurationPatterns(content, metadata)
}

// detectConfigurationPatterns detects common YAML configuration patterns
func (p *YamlPlugin) detectConfigurationPatterns(content string, metadata *model.FileMetadata) {
	lowerContent := strings.ToLower(content)

	// Common configuration sections
	configPatterns := map[string]string{
		"database":    "has_database_config",
		"server":      "has_server_config",
		"api":         "has_api_config",
		"logging":     "has_logging_config",
		"auth":        "has_auth_config",
		"cache":       "has_cache_config",
		"environment": "has_environment_config",
		"deployment":  "has_deployment_config",
	}

	for pattern, property := range configPatterns {
		if strings.Contains(lowerContent, pattern) {
			metadata.Properties[property] = "true"
		}
	}

	// Kubernetes/Docker patterns
	if strings.Contains(lowerContent, "apiversion") || strings.Contains(lowerContent, "kind:") {
		metadata.Properties["kubernetes_manifest"] = "true"
	}
	if strings.Contains(lowerContent, "docker") || strings.Contains(lowerContent, "image:") {
		metadata.Properties["has_docker_config"] = "true"
	}

	// CI/CD patterns
	ciPatterns := []string{"pipeline", "workflow", "job:", "stage:", "script:", "deploy"}
	for _, pattern := range ciPatterns {
		if strings.Contains(lowerContent, pattern) {
			metadata.Properties["has_ci_config"] = "true"
			break
		}
	}
}

// countYamlElements counts different types of YAML elements
func (p *YamlPlugin) countYamlElements(node *yaml.Node) map[string]int {
	counts := map[string]int{
		"mappings":  0,
		"sequences": 0,
		"scalars":   0,
		"aliases":   0,
		"keys":      0,
	}

	p.countElementsRecursive(node, counts)
	return counts
}

// countElementsRecursive recursively counts YAML elements
func (p *YamlPlugin) countElementsRecursive(node *yaml.Node, counts map[string]int) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			p.countElementsRecursive(child, counts)
		}

	case yaml.MappingNode:
		counts["mappings"]++
		counts["keys"] += len(node.Content) / 2
		for _, child := range node.Content {
			p.countElementsRecursive(child, counts)
		}

	case yaml.SequenceNode:
		counts["sequences"]++
		for _, child := range node.Content {
			p.countElementsRecursive(child, counts)
		}

	case yaml.ScalarNode:
		counts["scalars"]++

	case yaml.AliasNode:
		counts["aliases"]++
	}
}

// calculateYamlDepth calculates the maximum nesting depth
func (p *YamlPlugin) calculateYamlDepth(node *yaml.Node) int {
	switch node.Kind {
	case yaml.DocumentNode, yaml.MappingNode, yaml.SequenceNode:
		maxDepth := 0
		for _, child := range node.Content {
			depth := p.calculateYamlDepth(child)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth + 1

	default:
		return 0
	}
}

// analyzeLineTypes analyzes different types of lines in YAML
func (p *YamlPlugin) analyzeLineTypes(content string) map[string]int {
	lines := strings.Split(content, "\n")
	counts := map[string]int{
		"empty_lines":   0,
		"comment_lines": 0,
		"content_lines": 0,
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			counts["empty_lines"]++
		case strings.HasPrefix(trimmed, "#"):
			counts["comment_lines"]++
		default:
			counts["content_lines"]++
		}
	}

	return counts
}

// Helper functions for type analysis

func (p *YamlPlugin) getYamlNodeType(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	case yaml.DocumentNode:
		return "document"
	default:
		return "unknown"
	}
}

func (p *YamlPlugin) isComplexYamlNode(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return true
	case yaml.ScalarNode:
		return len(strings.Split(node.Value, "\n")) > 1
	default:
		return false
	}
}

func (p *YamlPlugin) getDefinitionType(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	default:
		return "unknown"
	}
}

func (p *YamlPlugin) getYamlNodeSignature(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return fmt.Sprintf("mapping{%d keys}", len(node.Content)/2)
	case yaml.SequenceNode:
		return fmt.Sprintf("sequence[%d items]", len(node.Content))
	case yaml.ScalarNode:
		if len(node.Value) > 50 {
			return fmt.Sprintf("scalar(%d chars)", len(node.Value))
		}
		return "scalar"
	default:
		return p.getYamlNodeType(node)
	}
}

func (p *YamlPlugin) getNodeEndLine(node *yaml.Node) int {
	endLine := node.Line

	// For complex nodes, find the last line recursively
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		for _, child := range node.Content {
			childEnd := p.getNodeEndLine(child)
			if childEnd > endLine {
				endLine = childEnd
			}
		}
	}

	return endLine
}
