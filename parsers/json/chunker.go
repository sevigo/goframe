package json

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	model "github.com/sevigo/goframe/schema"
)

// JSONStructure represents different types of JSON structures
type JSONStructure struct {
	Type      string // object, array, primitive
	Key       string // for object properties
	Value     any    // the actual JSON value
	Children  []JSONStructure
	Path      string // JSON path like "users[0].name"
	LineStart int
	LineEnd   int
}

// Chunk breaks JSON into semantic chunks based on structure
func (p *JSONPlugin) Chunk(content string, path string, opts *model.CodeChunkingOptions) ([]model.CodeChunk, error) {
	// Parse JSON to get structure
	var data interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		// If JSON is invalid, create a single chunk with error info
		return p.createErrorChunk(content, path, err), nil
	}

	// Analyze JSON structure
	structure := p.analyzeStructure(data, "", "root")

	// Convert to chunks based on structure
	chunks := p.structureToChunks(structure, content, path, opts)

	// If no meaningful chunks created, create single document chunk
	if len(chunks) == 0 {
		chunks = p.createSingleChunk(content, path, structure.Type)
	}

	p.logger.Debug("Created JSON chunks", "count", len(chunks), "path", path)
	return chunks, nil
}

// analyzeStructure recursively analyzes JSON structure
func (p *JSONPlugin) analyzeStructure(data interface{}, key string, path string) JSONStructure {
	structure := JSONStructure{
		Key:   key,
		Path:  path,
		Value: data,
	}

	switch v := data.(type) {
	case map[string]any:
		structure.Type = "object"
		for objKey, objValue := range v {
			childPath := objKey
			if path != "root" {
				childPath = path + "." + objKey
			}
			child := p.analyzeStructure(objValue, objKey, childPath)
			structure.Children = append(structure.Children, child)
		}

	case []any:
		structure.Type = "array"
		for i, item := range v {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			child := p.analyzeStructure(item, fmt.Sprintf("[%d]", i), childPath)
			structure.Children = append(structure.Children, child)
		}

	default:
		structure.Type = "primitive"
		structure.Value = v
	}

	return structure
}

// structureToChunks converts JSON structure to chunks
func (p *JSONPlugin) structureToChunks(
	structure JSONStructure,
	content,
	path string,
	opts *model.CodeChunkingOptions,
) []model.CodeChunk {
	var chunks []model.CodeChunk
	lines := strings.Split(content, "\n")

	// Strategy: Create chunks for top-level objects/arrays and significant nested structures
	switch structure.Type {
	case "object":
		chunks = p.chunkObject(structure, content, lines, path, opts)
	case "array":
		chunks = p.chunkArray(structure, content, path, opts)
	default:
		// Single primitive value
		chunks = p.createSingleChunk(content, path, structure.Type)
	}

	return chunks
}

// chunkObject creates chunks for JSON objects
func (p *JSONPlugin) chunkObject(structure JSONStructure, content string, lines []string, path string, opts *model.CodeChunkingOptions) []model.CodeChunk {
	var chunks []model.CodeChunk

	// For small objects, create a single chunk
	if len(structure.Children) <= 5 {
		return p.createSingleChunk(content, path, "object")
	}

	// For larger objects, create chunks for significant properties
	for _, child := range structure.Children {
		if p.shouldCreateChunkForProperty(child, opts) {
			chunk := p.createPropertyChunk(child, content, lines)
			if chunk != nil {
				chunks = append(chunks, *chunk)
			}
		}
	}

	// If no property chunks created, create single object chunk
	if len(chunks) == 0 {
		return p.createSingleChunk(content, path, "object")
	}

	return chunks
}

// chunkArray creates chunks for JSON arrays
func (p *JSONPlugin) chunkArray(
	structure JSONStructure,
	content string,
	path string,
	opts *model.CodeChunkingOptions,
) []model.CodeChunk {
	var chunks []model.CodeChunk

	// For small arrays, create a single chunk
	if len(structure.Children) <= 10 {
		return p.createSingleChunk(content, path, "array")
	}

	chunkSize := opts.MaxLinesPerChunk / 2
	if chunkSize < 5 { // Ensure a minimum group size for array items
		chunkSize = 5
	}
	if chunkSize > 20 { // Prevent excessively large groups for array items
		chunkSize = 20
	}
	for i := 0; i < len(structure.Children); i += chunkSize {
		end := i + chunkSize
		if end > len(structure.Children) {
			end = len(structure.Children)
		}

		arraySlice := structure.Children[i:end]
		chunk := p.createArraySliceChunk(arraySlice, i)
		if chunk != nil {
			chunks = append(chunks, *chunk)
		}
	}

	return chunks
}

// shouldCreateChunkForProperty determines if a property deserves its own chunk
func (p *JSONPlugin) shouldCreateChunkForProperty(child JSONStructure, opts *model.CodeChunkingOptions) bool {
	switch child.Type {
	case "object":
		// Create chunk for objects with multiple properties
		return len(child.Children) > 2 || len(p.extractPropertyContent(child)) > opts.MinCharsPerChunk*2
	case "array":
		// Create chunk for arrays with multiple items
		return len(child.Children) > 3 || len(p.extractArraySliceContent([]JSONStructure{child})) > opts.MinCharsPerChunk*2
	default:
		if str, ok := child.Value.(string); ok && len(str) > opts.MinCharsPerChunk {
			return true
		}
		return false
	}
}

// createPropertyChunk creates a chunk for a significant JSON property
func (p *JSONPlugin) createPropertyChunk(child JSONStructure, _ string, lines []string) *model.CodeChunk {
	// Find the property in the content
	propertyContent := p.extractPropertyContent(child)
	if propertyContent == "" {
		return nil
	}

	// Determine line numbers (simplified approach)
	startLine, endLine := p.findPropertyLines(child.Key, propertyContent, lines)

	identifier := child.Key
	if identifier == "" {
		identifier = child.Path
	}

	chunkType := "json_property"
	annotations := map[string]string{
		"type":       chunkType,
		"property":   child.Key,
		"path":       child.Path,
		"value_type": child.Type,
	}

	// Add additional metadata
	switch child.Type {
	case "object":
		annotations["properties_count"] = strconv.Itoa(len(child.Children))
	case "array":
		annotations["items_count"] = strconv.Itoa(len(child.Children))
	}

	return &model.CodeChunk{
		Content:     propertyContent,
		LineStart:   startLine,
		LineEnd:     endLine,
		Type:        chunkType,
		Identifier:  identifier,
		Annotations: annotations,
	}
}

// createArraySliceChunk creates a chunk for a slice of array items
func (p *JSONPlugin) createArraySliceChunk(items []JSONStructure, startIndex int) *model.CodeChunk {
	// Extract content for this slice (simplified)
	sliceContent := p.extractArraySliceContent(items)
	if sliceContent == "" {
		return nil
	}

	startLine, endLine := p.estimateLines(sliceContent)

	identifier := fmt.Sprintf("items[%d-%d]", startIndex, startIndex+len(items)-1)

	return &model.CodeChunk{
		Content:    sliceContent,
		LineStart:  startLine,
		LineEnd:    endLine,
		Type:       "json_array_slice",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":        "json_array_slice",
			"start_index": strconv.Itoa(startIndex),
			"end_index":   strconv.Itoa(startIndex + len(items) - 1),
			"items_count": strconv.Itoa(len(items)),
		},
	}
}

// Helper functions for content extraction and line estimation
func (p *JSONPlugin) extractPropertyContent(child JSONStructure) string {
	// Simplified: re-marshal the property
	propertyData := map[string]interface{}{
		child.Key: child.Value,
	}

	if bytes, err := json.MarshalIndent(propertyData, "", "  "); err == nil {
		return string(bytes)
	}
	return ""
}

func (p *JSONPlugin) extractArraySliceContent(items []JSONStructure) string {
	var values []any
	for _, item := range items {
		values = append(values, item.Value)
	}

	if bytes, err := json.MarshalIndent(values, "", "  "); err == nil {
		return string(bytes)
	}
	return ""
}

func (p *JSONPlugin) findPropertyLines(key string, propertyContent string, lines []string) (int, int) {
	// Simplified line detection - look for the key in the content
	propertyLines := strings.Split(propertyContent, "\n")
	startLine := 1
	endLine := len(propertyLines)

	// Try to find where this property appears in the original content
	for i, line := range lines {
		if strings.Contains(line, `"`+key+`"`) {
			startLine = i + 1
			endLine = startLine + len(propertyLines) - 1
			break
		}
	}

	return startLine, endLine
}

func (p *JSONPlugin) estimateLines(chunkContent string) (int, int) {
	chunkLines := strings.Split(chunkContent, "\n")
	return 1, len(chunkLines)
}

// createSingleChunk creates a single chunk for the entire JSON
func (p *JSONPlugin) createSingleChunk(content string, path string, jsonType string) []model.CodeChunk {
	lines := strings.Split(content, "\n")

	// Determine identifier from filename or content
	identifier := p.deriveIdentifier(path, jsonType)

	chunk := model.CodeChunk{
		Content:    content,
		LineStart:  1,
		LineEnd:    len(lines),
		Type:       "json_document",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":      "json_document",
			"json_type": jsonType,
		},
	}

	return []model.CodeChunk{chunk}
}

// createErrorChunk creates a chunk for invalid JSON
func (p *JSONPlugin) createErrorChunk(content string, path string, err error) []model.CodeChunk {
	lines := strings.Split(content, "\n")
	identifier := p.deriveIdentifier(path, "invalid")

	chunk := model.CodeChunk{
		Content:    content,
		LineStart:  1,
		LineEnd:    len(lines),
		Type:       "json_invalid",
		Identifier: identifier,
		Annotations: map[string]string{
			"type":  "json_invalid",
			"error": err.Error(),
		},
	}

	return []model.CodeChunk{chunk}
}

// deriveIdentifier creates an identifier from filename
func (p *JSONPlugin) deriveIdentifier(path string, jsonType string) string {
	filename := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if filename == "" {
		return fmt.Sprintf("JSON %s", cases.Title(language.English).String(jsonType))
	}

	// Convert filename to proper identifier
	identifier := strings.ReplaceAll(filename, "_", " ")
	identifier = strings.ReplaceAll(identifier, "-", " ")
	identifier = cases.Title(language.English).String(identifier)

	return identifier
}
