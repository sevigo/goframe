// extractor.go - Metadata extraction using goldmark
package markdown

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	model "github.com/sevigo/goframe/schema"
)

// ExtractMetadata extracts language-specific metadata from Markdown content using goldmark
func (p *MarkdownPlugin) ExtractMetadata(
	content string,
	path string,
) (model.FileMetadata, error) {
	metadata := model.FileMetadata{
		Language:    "markdown",
		Imports:     []string{},
		Definitions: []model.CodeEntityDefinition{},
		Symbols:     []model.CodeSymbol{},
		Properties:  map[string]string{},
	}

	lines := strings.Split(content, "\n")

	// Extract frontmatter if present
	frontmatterEndIdx := p.extractFrontmatterMetadata(lines, &metadata)

	// Prepare content for goldmark parsing (skip frontmatter)
	contentToParse := content
	lineOffset := 0
	if frontmatterEndIdx > 0 {
		lineOffset = frontmatterEndIdx + 1
		if lineOffset < len(lines) {
			contentToParse = strings.Join(lines[lineOffset:], "\n")
		} else {
			contentToParse = ""
		}
	}

	// Parse with goldmark if there's content to parse
	if contentToParse != "" {
		source := []byte(contentToParse)
		reader := text.NewReader(source)
		docNode := p.markdown.Parser().Parse(reader)

		// Extract metadata from AST
		p.extractASTMetadata(docNode, source, lineOffset, &metadata)
	}

	// Calculate document statistics
	stats := p.calculateDocumentStats(lines)
	for key, value := range stats {
		metadata.Properties[key] = value
	}

	// If still no title, use filename
	if metadata.Properties["title"] == "" {
		metadata.Properties["title"] = p.deriveTitleFromFilename(path)
	}

	return metadata, nil
}

// extractFrontmatterMetadata extracts metadata from YAML frontmatter
func (p *MarkdownPlugin) extractFrontmatterMetadata(lines []string, metadata *model.FileMetadata) int {
	if len(lines) < 3 || lines[0] != frontMatterSeparator {
		return -1
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == frontMatterSeparator {
			endIdx = i
			break
		}
	}

	if endIdx == -1 || endIdx <= 1 {
		p.logger.Debug("Invalid frontmatter structure - no closing separator found")
		return -1
	}

	// Parse YAML frontmatter
	yamlContent := strings.Join(lines[1:endIdx], "\n")
	var yamlData map[string]interface{}

	if err := yaml.Unmarshal([]byte(yamlContent), &yamlData); err != nil {
		p.logger.Debug("Failed to parse YAML frontmatter", "error", err)
		// Fall back to simple key-value parsing
		p.parseSimpleMetadata(lines[1:endIdx], metadata)
	} else {
		// Convert YAML data to string properties
		for key, value := range yamlData {
			metadata.Properties[key] = fmt.Sprintf("%v", value)
		}
	}

	return endIdx
}

// parseSimpleMetadata provides fallback parsing for malformed YAML
func (p *MarkdownPlugin) parseSimpleMetadata(lines []string, metadata *model.FileMetadata) {
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "" {
				p.logger.Debug("Skipping empty key in frontmatter", "line", lineNum+2)
				continue
			}

			// Strip quotes if present
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				if len(value) >= 2 {
					value = value[1 : len(value)-1]
				}
			}

			metadata.Properties[key] = value
		}
	}
}

// extractASTMetadata extracts metadata from goldmark AST
func (p *MarkdownPlugin) extractASTMetadata(node ast.Node, source []byte, lineOffset int, metadata *model.FileMetadata) {
	var codeLanguages []string
	var headingCount int

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch astNode := n.(type) {
		case *ast.Heading:
			headingCount++
			headingText := p.extractTextFromNode(astNode, source)

			// If no title from frontmatter, use first H1 heading
			if metadata.Properties["title"] == "" && astNode.Level == 1 {
				metadata.Properties["title"] = headingText
			}

			// Calculate line number
			segment := astNode.Lines()
			if segment.Len() > 0 {
				lineNum := p.segmentToLineNumber(segment.At(0), source) + lineOffset + 1

				// Add heading as a symbol
				symbol := model.CodeSymbol{
					Name:      headingText,
					Type:      fmt.Sprintf("h%d", astNode.Level),
					LineStart: lineNum,
					LineEnd:   lineNum,
					IsExport:  true,
				}
				metadata.Symbols = append(metadata.Symbols, symbol)

				// Add heading as a definition
				def := model.CodeEntityDefinition{
					Type:       "heading",
					Name:       headingText,
					LineStart:  lineNum,
					LineEnd:    lineNum,
					Visibility: "public",
					Signature:  fmt.Sprintf("h%d", astNode.Level),
				}
				metadata.Definitions = append(metadata.Definitions, def)
			}

		case *ast.FencedCodeBlock:
			if astNode.Info != nil {
				language := strings.TrimSpace(string(astNode.Info.Text(source))) //nolint:staticcheck //SA1019
				if language != "" {
					// Add to languages if not already present
					found := slices.Contains(codeLanguages, language)
					if !found {
						codeLanguages = append(codeLanguages, language)
					}
				}
			}
		}

		return ast.WalkContinue, nil
	})

	// Store code languages
	if len(codeLanguages) > 0 {
		metadata.Properties["code_languages"] = strings.Join(codeLanguages, ",")
	}

	// Store heading count
	if headingCount > 0 {
		metadata.Properties["heading_count"] = strconv.Itoa(headingCount)
	}
}

// segmentToLineNumber converts a text segment to a line number (helper for extractor)
func (p *MarkdownPlugin) segmentToLineNumber(segment text.Segment, source []byte) int {
	return bytes.Count(source[:segment.Start], []byte("\n"))
}

// extractTextFromNode extracts plain text content from an AST node (helper for extractor)
func (p *MarkdownPlugin) extractTextFromNode(node ast.Node, source []byte) string {
	var buf bytes.Buffer

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindText {
			segment := n.(*ast.Text).Segment //nolint:errcheck //ok
			buf.Write(segment.Value(source))
		}
		return ast.WalkContinue, nil
	})

	return strings.TrimSpace(buf.String())
}

// calculateDocumentStats calculates various statistics about the markdown document
func (p *MarkdownPlugin) calculateDocumentStats(lines []string) map[string]string {
	stats := make(map[string]string)

	headingCount := 0
	codeBlockCount := 0
	tableCount := 0
	listCount := 0
	codeBlockLines := 0
	totalLines := len(lines)

	inCodeBlock := false
	headingRegex := regexp.MustCompile(`^#{1,6}\s+`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Count headings (backup method for validation)
		if headingRegex.MatchString(line) {
			headingCount++
		}

		// Count code blocks
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				codeBlockCount++
			}
			inCodeBlock = !inCodeBlock
		}

		if inCodeBlock {
			codeBlockLines++
		}

		// Count tables
		if p.isTableRow(line) {
			tableCount++
		}

		// Count lists
		if p.isListItem(line) {
			listCount++
		}
	}

	if codeBlockCount > 0 {
		stats["code_block_count"] = strconv.Itoa(codeBlockCount)
	}
	if tableCount > 0 {
		stats["table_count"] = strconv.Itoa(tableCount)
	}
	if listCount > 0 {
		stats["list_count"] = strconv.Itoa(listCount)
	}

	stats["total_lines"] = strconv.Itoa(totalLines)

	if codeBlockLines > 0 {
		stats["code_lines"] = strconv.Itoa(codeBlockLines)
		if totalLines > 0 {
			percentage := (codeBlockLines * 100) / totalLines
			stats["code_percentage"] = strconv.Itoa(percentage)
		}
	}

	return stats
}

// deriveTitleFromFilename creates a title from the filename
func (p *MarkdownPlugin) deriveTitleFromFilename(path string) string {
	filename := filepath.Base(path)
	title := strings.TrimSuffix(filename, filepath.Ext(filename))

	if title == "" {
		return "Document"
	}

	// Convert underscores and hyphens to spaces
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")

	// Convert to title case
	title = cases.Title(language.English).String(title)

	return title
}
