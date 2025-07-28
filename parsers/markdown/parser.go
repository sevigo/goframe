// parser.go - Goldmark-based Markdown parser
package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// MarkdownElement represents different types of markdown elements
type MarkdownElement struct {
	Type        string            `json:"type"`
	Level       int               `json:"level,omitempty"`
	Content     string            `json:"content"`
	Language    string            `json:"language,omitempty"`
	LineStart   int               `json:"line_start"`
	LineEnd     int               `json:"line_end"`
	Identifier  string            `json:"identifier,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type FrontMatter struct {
	Content    string
	Properties map[string]string
	LineStart  int
	LineEnd    int
}

// DocumentStructure represents the parsed markdown document
type DocumentStructure struct {
	FrontMatter *FrontMatter
	Elements    []MarkdownElement
	Title       string
}

// parseMarkdown parses markdown content using goldmark into structured elements
func (p *MarkdownPlugin) parseMarkdown(content string) *DocumentStructure {
	lines := strings.Split(content, "\n")
	doc := &DocumentStructure{
		Elements: make([]MarkdownElement, 0),
	}

	contentToParse := content
	startLineOffset := 0

	if len(lines) > 2 && lines[0] == frontMatterSeparator {
		frontMatter, endIdx := p.parseFrontMatter(lines)
		if frontMatter != nil {
			doc.FrontMatter = frontMatter
			startLineOffset = endIdx + 1
			if startLineOffset < len(lines) {
				contentToParse = strings.Join(lines[startLineOffset:], "\n")
			} else {
				contentToParse = ""
			}
		}
	}

	if contentToParse != "" {
		source := []byte(contentToParse)
		reader := text.NewReader(source)
		docNode := p.markdown.Parser().Parse(reader)

		elements := p.convertASTToElements(docNode, source, lines, startLineOffset)
		doc.Elements = elements
	}

	doc.Title = p.deriveTitle(doc)

	return doc
}

func (p *MarkdownPlugin) convertASTToElements(node ast.Node, source []byte, originalLines []string, lineOffset int) []MarkdownElement {
	var elements []MarkdownElement

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		element := p.nodeToElement(child, source, originalLines, lineOffset)
		if element != nil {
			elements = append(elements, *element)
		}
	}

	return elements
}

// nodeToElement converts a goldmark AST node to a MarkdownElement
func (p *MarkdownPlugin) nodeToElement(node ast.Node, source []byte, originalLines []string, lineOffset int) *MarkdownElement {
	// TextBlocks are just containers for paragraphs, which are processed individually as separate nodes.
	// Processing the container itself is redundant and causes noisy warnings for empty TextBlocks.
	if _, ok := node.(*ast.TextBlock); ok {
		return nil
	}

	segment := node.Lines()

	if segment.Len() == 0 {
		return p.nodeToElementWithoutSegments(node, source, originalLines, lineOffset)
	}

	sourceStartLine := p.segmentToLineNumber(segment.At(0), source)
	sourceEndLine := p.segmentToLineNumber(segment.At(segment.Len()-1), source)

	actualStartLine := sourceStartLine + lineOffset + 1
	actualEndLine := sourceEndLine + lineOffset + 1

	var contentLines []string
	startIdx := actualStartLine - 1
	endIdx := actualEndLine - 1

	if startIdx >= 0 && endIdx < len(originalLines) && startIdx <= endIdx {
		contentLines = originalLines[startIdx : endIdx+1]
	}
	content := strings.Join(contentLines, "\n")

	return p.createElementForNode(node, content, actualStartLine, actualEndLine, source)
}

func (p *MarkdownPlugin) nodeToElementWithoutSegments(node ast.Node, source []byte, originalLines []string, lineOffset int) *MarkdownElement {
	minOffset := len(source)
	maxOffset := 0
	hasContent := false

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Prioritize getting the full segment of a child block if it has one.
		if n.Lines().Len() > 0 {
			seg := n.Lines().At(0)
			if seg.Start < minOffset {
				minOffset = seg.Start
			}
			seg = n.Lines().At(n.Lines().Len() - 1)
			if seg.Stop > maxOffset {
				maxOffset = seg.Stop
			}
			hasContent = true
			return ast.WalkSkipChildren, nil
		}

		if n.Kind() == ast.KindText {
			textNode := n.(*ast.Text) //nolint:errcheck //ok
			segment := textNode.Segment
			if segment.Len() > 0 {
				if segment.Start < minOffset {
					minOffset = segment.Start
				}
				if segment.Stop > maxOffset {
					maxOffset = segment.Stop
				}
				hasContent = true
			}
		}
		return ast.WalkContinue, nil
	})

	var extractedContent string
	var actualStartLine, actualEndLine int

	if hasContent && maxOffset >= minOffset {
		// Calculate source line numbers based on derived byte offsets within the `source` (content without frontmatter)
		sourceStartLine := bytes.Count(source[:minOffset], []byte("\n"))
		sourceEndLine := bytes.Count(source[:maxOffset], []byte("\n"))

		// Convert these to actual line numbers in the `originalLines` (full document) context (1-based)
		actualStartLine = sourceStartLine + lineOffset + 1
		actualEndLine = sourceEndLine + lineOffset + 1

		// Extract content from `originalLines` using the determined line range
		startIdx := actualStartLine - 1 // Convert to 0-based index for slice
		endIdx := actualEndLine - 1     // Convert to 0-based index for slice

		// Ensure indices are within bounds before slicing
		if startIdx >= 0 && endIdx < len(originalLines) && startIdx <= endIdx {
			extractedContent = strings.Join(originalLines[startIdx:endIdx+1], "\n")
		} else {
			// Fallback: If line mapping fails, use the raw bytes from the `source`
			// This should be a rare case, indicating a more fundamental line number derivation issue.
			p.logger.Warn("Failed to map derived byte range to original lines; falling back to raw source bytes for node without segments",
				"type", fmt.Sprintf("%T", node), "kind", node.Kind().String(),
				"minOffset", minOffset, "maxOffset", maxOffset,
				"sourceStartLine", sourceStartLine, "sourceEndLine", sourceEndLine,
				"actualStartLine", actualStartLine, "actualEndLine", actualEndLine)
			// Ensure slice is valid
			if minOffset <= maxOffset && maxOffset <= len(source) {
				extractedContent = string(source[minOffset:maxOffset])
			} else {
				extractedContent = "" // No valid content to extract
			}
		}
	} else {
		// No valid content range found (e.g., node has no text children, or offsets are invalid)
		p.logger.Warn("No valid content range found for node without segments",
			"type", fmt.Sprintf("%T", node), "kind", node.Kind().String())
		return nil // Cannot create a meaningful element
	}

	return p.createElementForNode(node, extractedContent, actualStartLine, actualEndLine, source)
}

// createElementForNode creates a MarkdownElement for a given node type
func (p *MarkdownPlugin) createElementForNode(node ast.Node, content string, startLine, endLine int, source []byte) *MarkdownElement {
	switch n := node.(type) {
	case *ast.Heading:
		headingText := p.extractTextFromNode(n, source)
		return &MarkdownElement{
			Type:       "heading",
			Level:      n.Level,
			Content:    content,
			Identifier: headingText,
			LineStart:  startLine,
			LineEnd:    endLine,
			Annotations: map[string]string{
				"type":  "heading",
				"level": strconv.Itoa(n.Level),
			},
		}

	case *ast.CodeBlock:
		return &MarkdownElement{
			Type:      "code_block",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type": "code_block",
			},
		}

	case *ast.FencedCodeBlock:
		language := ""
		if n.Info != nil {
			language = strings.TrimSpace(string(n.Info.Text(source))) //nolint:staticcheck // SA1019
		}
		return &MarkdownElement{
			Type:      "code_block",
			Content:   content,
			Language:  language,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type":     "code_block",
				"language": language,
			},
		}

	case *extast.Table:
		return &MarkdownElement{
			Type:      "table",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type": "table",
			},
		}

	case *ast.List:
		listType := "unordered"
		if n.IsOrdered() {
			listType = "ordered"
		}
		return &MarkdownElement{
			Type:      "list",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type":      "list",
				"list_type": listType,
			},
		}

	case *ast.Paragraph:
		return &MarkdownElement{
			Type:      "paragraph",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type": "paragraph",
			},
		}

	case *ast.Blockquote:
		return &MarkdownElement{
			Type:      "blockquote",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type": "blockquote",
			},
		}

	case *ast.ThematicBreak:
		return &MarkdownElement{
			Type:      "thematic_break",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type": "thematic_break",
			},
		}

	default:
		// Handle other node types as generic elements
		return &MarkdownElement{
			Type:      "other",
			Content:   content,
			LineStart: startLine,
			LineEnd:   endLine,
			Annotations: map[string]string{
				"type":      "other",
				"node_type": fmt.Sprintf("%T", node),
			},
		}
	}
}

// parseFrontMatter extracts YAML frontmatter with improved error handling
func (p *MarkdownPlugin) parseFrontMatter(lines []string) (*FrontMatter, int) {
	if len(lines) < 3 || lines[0] != frontMatterSeparator {
		return nil, -1
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
		return nil, -1
	}

	frontMatter := &FrontMatter{
		Content:    strings.Join(lines[0:endIdx+1], "\n"),
		Properties: make(map[string]string),
		LineStart:  1,
		LineEnd:    endIdx + 1,
	}

	// Parse YAML frontmatter using proper YAML parser
	yamlContent := strings.Join(lines[1:endIdx], "\n")
	var yamlData map[string]interface{}

	if err := yaml.Unmarshal([]byte(yamlContent), &yamlData); err != nil {
		p.logger.Debug("Failed to parse YAML frontmatter", "error", err)
		p.parseSimpleFrontMatter(lines[1:endIdx], frontMatter)
	} else {
		for key, value := range yamlData {
			frontMatter.Properties[key] = fmt.Sprintf("%v", value)
		}
	}

	return frontMatter, endIdx
}

// parseSimpleFrontMatter provides fallback parsing for malformed YAML
func (p *MarkdownPlugin) parseSimpleFrontMatter(lines []string, frontMatter *FrontMatter) {
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

			frontMatter.Properties[key] = value
		}
	}
}

// deriveTitle determines the document title from various sources
func (p *MarkdownPlugin) deriveTitle(doc *DocumentStructure) string {
	if doc.FrontMatter != nil {
		if title, exists := doc.FrontMatter.Properties["title"]; exists && title != "" {
			return title
		}
	}

	for _, element := range doc.Elements {
		if element.Type == "heading" && element.Level == 1 {
			return element.Identifier
		}
	}

	return ""
}

// isTableRow helper method for validation/testing
func (p *MarkdownPlugin) isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}

	if !strings.Contains(trimmed, "|") {
		return false
	}

	pipeCount := strings.Count(trimmed, "|")
	if pipeCount < 2 {
		return false
	}

	// Check if it's a separator row
	separatorPattern := regexp.MustCompile(`^\|?(\s*:?-+:?\s*\|)+\s*:?-+:?\s*\|?$`)
	if separatorPattern.MatchString(trimmed) {
		return true
	}

	// Regular table row - should have content between pipes
	parts := strings.Split(trimmed, "|")
	hasContent := false
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			hasContent = true
			break
		}
	}

	return hasContent
}

func (p *MarkdownPlugin) isListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}

	// Bulleted list
	if strings.HasPrefix(trimmed, "- ") ||
		strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "+ ") {
		return true
	}

	// Numbered list
	numberedListRegex := regexp.MustCompile(`^\d+\.\s+`)
	return numberedListRegex.MatchString(trimmed)
}
