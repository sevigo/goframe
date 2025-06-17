package text

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sevigo/goframe/schema"
)

// ExtractMetadata extracts metadata from text content
func (p *TextPlugin) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	metadata := schema.FileMetadata{
		Language:    "text",
		FilePath:    path,
		Imports:     []string{}, // Text files don't have imports
		Definitions: []schema.CodeEntityDefinition{},
		Symbols:     []schema.CodeSymbol{},
		Properties:  map[string]string{},
	}

	lines := strings.Split(content, "\n")
	metadata.Properties["total_lines"] = strconv.Itoa(len(lines))
	metadata.Properties["size_bytes"] = strconv.Itoa(len(content))

	// Analyze text structure
	p.analyzeTextContent(content, lines, &metadata)

	// Detect document type
	p.detectDocumentType(content, path, &metadata)

	return metadata, nil
}

// analyzeTextContent analyzes the content for various text characteristics
func (p *TextPlugin) analyzeTextContent(content string, lines []string, metadata *schema.FileMetadata) {
	var (
		emptyLines    int
		headings      int
		listItems     int
		longLines     int
		shortLines    int
		wordCount     int
		sentenceCount int
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			emptyLines++
			continue
		}

		if len(line) > 80 {
			longLines++
		} else if len(trimmed) < 20 {
			shortLines++
		}

		if p.isHeading(trimmed) {
			headings++
			// Add heading as a symbol
			symbol := schema.CodeSymbol{
				Name:     p.cleanHeading(trimmed),
				Type:     "heading",
				IsExport: true,
			}
			metadata.Symbols = append(metadata.Symbols, symbol)
		}

		if p.isListItem(trimmed) {
			listItems++
		}

		// Count words and sentences
		words := strings.Fields(trimmed)
		wordCount += len(words)

		// Simple sentence counting
		sentences := regexp.MustCompile(`[.!?]+`).Split(trimmed, -1)
		sentenceCount += len(sentences) - 1
	}

	// Store statistics
	metadata.Properties["empty_lines"] = strconv.Itoa(emptyLines)
	metadata.Properties["content_lines"] = strconv.Itoa(len(lines) - emptyLines)
	metadata.Properties["heading_count"] = strconv.Itoa(headings)
	metadata.Properties["list_items"] = strconv.Itoa(listItems)
	metadata.Properties["long_lines"] = strconv.Itoa(longLines)
	metadata.Properties["short_lines"] = strconv.Itoa(shortLines)
	metadata.Properties["word_count"] = strconv.Itoa(wordCount)
	metadata.Properties["sentence_count"] = strconv.Itoa(sentenceCount)

	// Calculate ratios
	totalLines := len(lines)
	if totalLines > 0 {
		metadata.Properties["empty_line_ratio"] = strconv.FormatFloat(float64(emptyLines)/float64(totalLines), 'f', 2, 64)
	}

	if wordCount > 0 {
		metadata.Properties["avg_words_per_line"] = strconv.FormatFloat(float64(wordCount)/float64(totalLines-emptyLines), 'f', 1, 64)
	}

	switch {
	case headings > 0:
		metadata.Properties["document_structure"] = "sectioned"
	case listItems > 0:
		metadata.Properties["document_structure"] = "list_based"
	default:
		metadata.Properties["document_structure"] = "simple"
	}
}

// detectDocumentType tries to identify what kind of text document this is
func (p *TextPlugin) detectDocumentType(content string, path string, metadata *schema.FileMetadata) {
	lowerContent := strings.ToLower(content)
	fileName := strings.ToLower(strings.TrimSuffix(path, filepath.Ext(path)))

	switch {
	case strings.Contains(fileName, "readme"):
		metadata.Properties["document_type"] = "readme"
	case strings.Contains(fileName, "license"):
		metadata.Properties["document_type"] = "license"
	case strings.Contains(fileName, "changelog"), strings.Contains(fileName, "history"):
		metadata.Properties["document_type"] = "changelog"
	case strings.Contains(fileName, "log"):
		metadata.Properties["document_type"] = "log"
	case strings.Contains(fileName, "todo"), strings.Contains(fileName, "task"):
		metadata.Properties["document_type"] = "todo"
	}

	// Check content patterns
	if strings.Contains(lowerContent, "copyright") || strings.Contains(lowerContent, "license") {
		metadata.Properties["has_legal_content"] = "true"
	}

	if strings.Contains(lowerContent, "installation") || strings.Contains(lowerContent, "getting started") {
		metadata.Properties["has_instructions"] = "true"
	}

	if strings.Contains(lowerContent, "version") || strings.Contains(lowerContent, "v1.") || strings.Contains(lowerContent, "release") {
		metadata.Properties["has_version_info"] = "true"
	}

	// Detect language/locale hints
	if strings.Contains(lowerContent, "english") || strings.Contains(lowerContent, "en-us") {
		metadata.Properties["language_hint"] = "english"
	}

	// Detect format hints
	if strings.Count(content, "\t") > strings.Count(content, "    ") {
		metadata.Properties["indentation_style"] = "tabs"
	} else {
		metadata.Properties["indentation_style"] = "spaces"
	}
}
