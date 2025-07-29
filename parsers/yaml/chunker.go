package yaml

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/sevigo/goframe/schema"
)

// YamlStructure represents different types of YAML structures
type YamlStructure struct {
	Type      string // mapping, sequence, scalar, document
	Key       string // for mapping entries
	Value     interface{}
	Node      *yaml.Node // original YAML node for line info
	Children  []YamlStructure
	Path      string // YAML path like "database.connection.host"
	LineStart int
	LineEnd   int
	Indent    int // indentation level
}

// Chunk breaks YAML into semantic chunks based on structure and indentation
func (p *YamlPlugin) Chunk(content string, path string, _ *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	// Parse YAML to get structure with line information
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(content), &node); err != nil {
		// If YAML is invalid, create a single chunk with error info
		return p.createErrorChunk(content, path, err), nil
	}

	// Analyze YAML structure
	structure := p.analyzeStructure(&node, "", "root", 0)

	// Convert to chunks based on structure and indentation
	chunks := p.structureToChunks(structure, content, path)

	// If no meaningful chunks created, create single document chunk
	if len(chunks) == 0 {
		chunks = p.createSingleChunk(content, path, structure.Type)
	}

	p.logger.Debug("Created YAML chunks", "count", len(chunks), "path", path)
	return chunks, nil
}

// analyzeStructure recursively analyzes YAML structure with line information
func (p *YamlPlugin) analyzeStructure(node *yaml.Node, key string, path string, indent int) YamlStructure {
	structure := YamlStructure{
		Key:       key,
		Path:      path,
		Node:      node,
		LineStart: node.Line,
		LineEnd:   node.Line,
		Indent:    indent,
	}

	switch node.Kind {
	case yaml.DocumentNode:
		structure.Type = "document"
		if len(node.Content) > 0 {
			// Process document content
			child := p.analyzeStructure(node.Content[0], "", "root", indent)
			structure.Children = append(structure.Children, child)
			structure.LineEnd = child.LineEnd
		}

	case yaml.MappingNode:
		structure.Type = "mapping"
		structure.Value = make(map[string]interface{})

		// Process key-value pairs
		for i := 0; i < len(node.Content); i += 2 {
			if i+1 < len(node.Content) {
				keyNode := node.Content[i]
				valueNode := node.Content[i+1]

				mapKey := keyNode.Value
				childPath := mapKey
				if path != "root" && path != "" {
					childPath = path + "." + mapKey
				}

				child := p.analyzeStructure(valueNode, mapKey, childPath, indent+1)
				structure.Children = append(structure.Children, child)

				// Update end line to include all children
				if child.LineEnd > structure.LineEnd {
					structure.LineEnd = child.LineEnd
				}
			}
		}

	case yaml.SequenceNode:
		structure.Type = "sequence"
		var values []interface{}

		// Process sequence items
		for i, itemNode := range node.Content {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			child := p.analyzeStructure(itemNode, fmt.Sprintf("[%d]", i), childPath, indent+1)
			structure.Children = append(structure.Children, child)
			values = append(values, child.Value)

			// Update end line
			if child.LineEnd > structure.LineEnd {
				structure.LineEnd = child.LineEnd
			}
		}
		structure.Value = values

	case yaml.ScalarNode:
		structure.Type = "scalar"
		structure.Value = node.Value
		structure.LineEnd = node.Line

	case yaml.AliasNode:
		structure.Type = "alias"
		structure.Value = node.Value
		structure.LineEnd = node.Line
	}

	return structure
}

// structureToChunks converts YAML structure to chunks
func (p *YamlPlugin) structureToChunks(structure YamlStructure, content string, path string) []schema.CodeChunk {
	var chunks []schema.CodeChunk
	lines := strings.Split(content, "\n")

	// Strategy: Create chunks based on top-level sections and significant nested structures
	switch structure.Type {
	case "document":
		if len(structure.Children) > 0 {
			chunks = p.structureToChunks(structure.Children[0], content, path)
		}
	case "mapping":
		chunks = p.chunkMapping(structure, content, lines, path)
	case "sequence":
		chunks = p.chunkSequence(structure, content, path)
	default:
		// Single scalar value
		chunks = p.createSingleChunk(content, path, structure.Type)
	}

	return chunks
}

// chunkMapping creates chunks for YAML mappings (objects)
func (p *YamlPlugin) chunkMapping(structure YamlStructure, content string, lines []string, path string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	// For small mappings (3 or fewer keys), create a single chunk
	if len(structure.Children) <= 3 {
		return p.createSingleChunk(content, path, "mapping")
	}

	// For larger mappings, create chunks for significant top-level keys
	for _, child := range structure.Children {
		if p.shouldCreateChunkForKey(child) {
			chunk := p.createKeyChunk(child, content, lines)
			if chunk != nil {
				chunks = append(chunks, *chunk)
			}
		}
	}

	// If no key chunks created, create single mapping chunk
	if len(chunks) == 0 {
		return p.createSingleChunk(content, path, "mapping")
	}

	return chunks
}

// chunkSequence creates chunks for YAML sequences (arrays)
func (p *YamlPlugin) chunkSequence(structure YamlStructure, content string, path string) []schema.CodeChunk {
	var chunks []schema.CodeChunk

	// For small sequences, create a single chunk
	if len(structure.Children) <= 5 {
		return p.createSingleChunk(content, path, "sequence")
	}

	// For large sequences, create chunks for groups of items
	chunkSize := 50
	for i := 0; i < len(structure.Children); i += chunkSize {
		end := i + chunkSize
		if end > len(structure.Children) {
			end = len(structure.Children)
		}

		sequenceSlice := structure.Children[i:end]
		chunk := p.createSequenceSliceChunk(sequenceSlice, i, content)
		if chunk != nil {
			chunks = append(chunks, *chunk)
		}
	}

	return chunks
}

// shouldCreateChunkForKey determines if a YAML key deserves its own chunk
func (p *YamlPlugin) shouldCreateChunkForKey(child YamlStructure) bool {
	switch child.Type {
	case "mapping":
		// Create chunk for mappings with 2 or more keys, or any significant nesting
		return len(child.Children) >= 2 || p.hasSignificantNesting(child)
	case "sequence":
		// Create chunk for sequences with 2 or more items
		return len(child.Children) >= 2
	case "scalar":
		// Create chunk for very long scalar values (like multi-line strings)
		if str, ok := child.Value.(string); ok {
			return len(strings.Split(str, "\n")) > 5 || len(str) > 200
		}
		return false
	default:
		return false
	}
}

// hasSignificantNesting checks if a structure has significant nested content
func (p *YamlPlugin) hasSignificantNesting(structure YamlStructure) bool {
	for _, child := range structure.Children {
		if child.Type == "mapping" && len(child.Children) >= 2 {
			return true
		}
		if child.Type == "sequence" && len(child.Children) >= 2 {
			return true
		}
	}
	return false
}

// createKeyChunk creates a chunk for a significant YAML key
func (p *YamlPlugin) createKeyChunk(child YamlStructure, content string, lines []string) *schema.CodeChunk {
	// Extract content for this key section
	keyContent := p.extractKeyContent(child, content, lines)
	if keyContent == "" {
		return nil
	}

	identifier := child.Key
	if identifier == "" {
		identifier = child.Path
	}

	chunkType := "yaml_section"
	annotations := map[string]string{
		"type":       chunkType,
		"key":        child.Key,
		"path":       child.Path,
		"value_type": child.Type,
		"indent":     strconv.Itoa(child.Indent),
	}

	// Add additional metadata
	switch child.Type {
	case "mapping":
		annotations["keys_count"] = strconv.Itoa(len(child.Children))
	case "sequence":
		annotations["items_count"] = strconv.Itoa(len(child.Children))
	}

	return &schema.CodeChunk{
		Content:     keyContent,
		LineStart:   child.LineStart,
		LineEnd:     child.LineEnd,
		Type:        chunkType,
		Identifier:  identifier,
		Annotations: annotations,
	}
}

// createSequenceSliceChunk creates a chunk for a slice of sequence items
func (p *YamlPlugin) createSequenceSliceChunk(items []YamlStructure, startIndex int, content string) *schema.CodeChunk {
	if len(items) == 0 {
		return nil
	}

	// Determine line range for this slice
	startLine := items[0].LineStart
	endLine := items[len(items)-1].LineEnd

	// Extract content for this slice
	sliceContent := p.extractLinesRange(content, startLine, endLine)
	if sliceContent == "" {
		return nil
	}

	identifier := fmt.Sprintf("items[%d-%d]", startIndex, startIndex+len(items)-1)

	return &schema.CodeChunk{
		Content:    sliceContent,
		LineStart:  startLine,
		LineEnd:    endLine,
		Type:       "yaml_sequence_slice",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":        "yaml_sequence_slice",
			"start_index": strconv.Itoa(startIndex),
			"end_index":   strconv.Itoa(startIndex + len(items) - 1),
			"items_count": strconv.Itoa(len(items)),
		},
	}
}

// Helper functions for content extraction

// extractKeyContent extracts content for a specific key section
func (p *YamlPlugin) extractKeyContent(child YamlStructure, content string, lines []string) string {
	// Find the line that contains the key
	keyLine := child.LineStart - 1 // Convert to 0-based index
	if keyLine < 0 || keyLine >= len(lines) {
		return p.extractLinesRange(content, child.LineStart, child.LineEnd)
	}

	// Include the key line and all content lines for this section
	startLine := keyLine + 1 // Convert back to 1-based
	endLine := child.LineEnd

	// Ensure we capture the key line itself
	if startLine > 1 {
		startLine--
	}

	return p.extractLinesRange(content, startLine, endLine)
}

// extractLinesRange extracts lines within a specific range
func (p *YamlPlugin) extractLinesRange(content string, startLine, endLine int) string {
	lines := strings.Split(content, "\n")

	// Convert to 0-based indexing and validate bounds
	start := startLine - 1
	end := endLine

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return ""
	}

	return strings.Join(lines[start:end], "\n")
}

// createSingleChunk creates a single chunk for the entire YAML
func (p *YamlPlugin) createSingleChunk(content string, path string, yamlType string) []schema.CodeChunk {
	lines := strings.Split(content, "\n")

	// Determine identifier from filename or content
	identifier := p.deriveIdentifier(path, yamlType)

	chunk := schema.CodeChunk{
		Content:    content,
		LineStart:  1,
		LineEnd:    len(lines),
		Type:       "yaml_document",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":      "yaml_document",
			"yaml_type": yamlType,
		},
	}

	return []schema.CodeChunk{chunk}
}

// createErrorChunk creates a chunk for invalid YAML
func (p *YamlPlugin) createErrorChunk(content string, path string, err error) []schema.CodeChunk {
	lines := strings.Split(content, "\n")
	identifier := p.deriveIdentifier(path, "invalid")

	chunk := schema.CodeChunk{
		Content:    content,
		LineStart:  1,
		LineEnd:    len(lines),
		Type:       "yaml_invalid",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":  "yaml_invalid",
			"error": err.Error(),
		},
	}

	return []schema.CodeChunk{chunk}
}

// deriveIdentifier creates an identifier from filename
func (p *YamlPlugin) deriveIdentifier(path string, yamlType string) string {
	filename := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if filename == "" {
		return fmt.Sprintf("YAML %s", cases.Title(language.English).String(yamlType))
	}

	// Convert filename to proper identifier
	identifier := strings.ReplaceAll(filename, "_", " ")
	identifier = strings.ReplaceAll(identifier, "-", " ")
	identifier = cases.Title(language.English).String(identifier)

	return identifier
}
