package json

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// ExtractMetadata extracts language-specific metadata from JSON content
func (p *JSONPlugin) ExtractMetadata(content string, path string) (model.FileMetadata, error) {
	metadata := model.FileMetadata{
		Language:    "json",
		Imports:     []string{}, // JSON doesn't have imports
		Definitions: []model.CodeEntityDefinition{},
		Symbols:     []model.CodeSymbol{},
		Properties:  map[string]string{},
	}

	// Parse JSON to analyze structure
	var data any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		// Invalid JSON
		metadata.Properties["error"] = err.Error()
		metadata.Properties["valid"] = "false"
		return metadata, err
	}

	metadata.Properties["valid"] = "true"

	// Analyze the JSON structure
	p.analyzeJSONMetadata(data, "", &metadata)

	// Add file-level statistics
	p.addJSONStatistics(data, content, &metadata)

	return metadata, nil
}

// analyzeJsonMetadata recursively analyzes JSON structure for metadata
func (p *JSONPlugin) analyzeJSONMetadata(data interface{}, path string, metadata *model.FileMetadata) {
	switch v := data.(type) {
	case map[string]interface{}:
		p.analyzeObject(v, path, metadata)
	case []interface{}:
		p.analyzeArray(v, path, metadata)
	default:
		p.analyzePrimitive(v, path, metadata)
	}
}

// analyzeObject analyzes JSON objects
func (p *JSONPlugin) analyzeObject(obj map[string]interface{}, path string, metadata *model.FileMetadata) {
	for key, value := range obj {
		fullPath := key
		if path != "" {
			fullPath = path + "." + key
		}

		// Add property as a symbol
		symbol := model.CodeSymbol{
			Name:      key,
			Type:      p.getJSONType(value),
			LineStart: 1,
			LineEnd:   1,
			IsExport:  true,
		}
		metadata.Symbols = append(metadata.Symbols, symbol)

		// Add property as a definition for complex types
		if p.isComplexType(value) {
			def := model.CodeEntityDefinition{
				Type:       p.getDefinitionType(value),
				Name:       key,
				LineStart:  1,
				LineEnd:    1,
				Visibility: "public",
				Signature:  p.getTypeSignature(value),
			}
			metadata.Definitions = append(metadata.Definitions, def)
		}

		// Recurse into nested structures
		p.analyzeJSONMetadata(value, fullPath, metadata)
	}
}

// analyzeArray analyzes JSON arrays
func (p *JSONPlugin) analyzeArray(arr []interface{}, path string, metadata *model.FileMetadata) {
	// Analyze array items
	for i, item := range arr {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		p.analyzeJSONMetadata(item, itemPath, metadata)

		// Only analyze first few items to avoid overwhelming metadata
		if i >= 10 {
			break
		}
	}
}

// analyzePrimitive analyzes primitive JSON values
func (p *JSONPlugin) analyzePrimitive(value any, path string, metadata *model.FileMetadata) {
	// For primitives, we don't add much metadata
	// Could potentially track value patterns, ranges, etc.
}

// addJsonStatistics adds overall statistics about the JSON
func (p *JSONPlugin) addJSONStatistics(data any, content string, metadata *model.FileMetadata) {
	lines := strings.Split(content, "\n")
	metadata.Properties["total_lines"] = strconv.Itoa(len(lines))

	// Determine root type
	metadata.Properties["root_type"] = p.getJSONType(data)

	// Count different elements
	stats := p.countJSONElements(data)
	for key, value := range stats {
		metadata.Properties[key] = strconv.Itoa(value)
	}

	// Calculate depth
	depth := p.calculateDepth(data)
	metadata.Properties["max_depth"] = strconv.Itoa(depth)

	// Estimate size
	metadata.Properties["size_bytes"] = strconv.Itoa(len(content))
}

// countJsonElements counts different types of JSON elements
func (p *JSONPlugin) countJSONElements(data any) map[string]int {
	counts := map[string]int{
		"objects":    0,
		"arrays":     0,
		"strings":    0,
		"numbers":    0,
		"booleans":   0,
		"nulls":      0,
		"properties": 0,
	}

	p.countElementsRecursive(data, counts)
	return counts
}

// countElementsRecursive recursively counts JSON elements
func (p *JSONPlugin) countElementsRecursive(data interface{}, counts map[string]int) {
	switch v := data.(type) {
	case map[string]interface{}:
		counts["objects"]++
		counts["properties"] += len(v)
		for _, value := range v {
			p.countElementsRecursive(value, counts)
		}

	case []interface{}:
		counts["arrays"]++
		for _, item := range v {
			p.countElementsRecursive(item, counts)
		}

	case string:
		counts["strings"]++
	case float64, int, int64, int32:
		counts["numbers"]++
	case bool:
		counts["booleans"]++
	case nil:
		counts["nulls"]++
	}
}

// calculateDepth calculates the maximum nesting depth
func (p *JSONPlugin) calculateDepth(data interface{}) int {
	switch v := data.(type) {
	case map[string]interface{}:
		maxDepth := 0
		for _, value := range v {
			depth := p.calculateDepth(value)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth + 1

	case []interface{}:
		maxDepth := 0
		for _, item := range v {
			depth := p.calculateDepth(item)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth + 1

	default:
		return 0
	}
}

func (p *JSONPlugin) getJSONType(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64, int, int64, int32:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

func (p *JSONPlugin) isComplexType(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func (p *JSONPlugin) getDefinitionType(value any) string {
	switch value.(type) {
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	default:
		return "property"
	}
}

func (p *JSONPlugin) getTypeSignature(value any) string {
	switch v := value.(type) {
	case map[string]interface{}:
		return fmt.Sprintf("object{%d properties}", len(v))
	case []interface{}:
		if len(v) > 0 {
			itemType := p.getJSONType(v[0])
			return fmt.Sprintf("array[%d] of %s", len(v), itemType)
		}
		return fmt.Sprintf("array[%d]", len(v))
	default:
		return p.getJSONType(value)
	}
}
