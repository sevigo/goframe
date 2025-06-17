package markdown

import (
	"fmt"
	"strconv"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// HeadingSection represents a hierarchical section based on headings
type HeadingSection struct {
	Heading   *MarkdownElement
	Elements  []MarkdownElement
	Children  []*HeadingSection
	LineStart int
	LineEnd   int
}

// Chunk breaks Markdown into semantic chunks based on hierarchical structure
func (p *MarkdownPlugin) Chunk(content string, path string, opt *model.CodeChunkingOptions) ([]model.CodeChunk, error) {
	// Parse the markdown into structured elements
	doc, err := p.parseMarkdown(content)
	if err != nil {
		return nil, err
	}

	// Ensure doc.Title has a fallback from filename if not set by frontmatter or H1
	if doc.Title == "" {
		doc.Title = p.deriveTitleFromFilename(path)
	}

	// Build hierarchical structure
	sections := p.buildHierarchicalSections(doc)

	// Convert sections to chunks
	chunks := p.sectionsToChunks(sections, doc, path)

	// If no chunks were created from sections, but there is content,
	// create a single chunk for the entire document.
	// This typically happens for plain text files without any markdown headings,
	// or files where parsing didn't yield distinct sections.
	if len(chunks) == 0 && content != "" {
		lines := strings.Split(content, "\n")
		// doc.Title should already be populated (from frontmatter, H1, or filename)
		chunk := model.CodeChunk{
			Content:    content,
			LineStart:  1,
			LineEnd:    len(lines),
			Type:       "document",
			Identifier: doc.Title, // Use the pre-populated doc.Title
			Annotations: map[string]string{
				"type": "document",
			},
		}
		chunks = append(chunks, chunk)
	}

	p.logger.Debug("Created markdown chunks", "count", len(chunks), "path", path)
	return chunks, nil
}

// buildHierarchicalSections creates a hierarchical structure from flat elements
func (p *MarkdownPlugin) buildHierarchicalSections(doc *DocumentStructure) []*HeadingSection {
	var sections []*HeadingSection
	var stack []*HeadingSection
	var currentElements []MarkdownElement

	var frontmatterToPrepend *MarkdownElement
	if doc.FrontMatter != nil {
		fmElement := MarkdownElement{
			Type:      "frontmatter",
			Content:   doc.FrontMatter.Content,
			LineStart: doc.FrontMatter.LineStart,
			LineEnd:   doc.FrontMatter.LineEnd,
			Annotations: map[string]string{
				"type": "frontmatter",
			},
		}
		frontmatterToPrepend = &fmElement
	}

	for i := range doc.Elements {
		elementAtIndex := &doc.Elements[i]
		if elementAtIndex.Type == "heading" {
			// Flush accumulated non-heading elements to the current section on stack or as a pre-section
			if len(currentElements) > 0 {
				if len(stack) > 0 {
					activeSection := stack[len(stack)-1]
					activeSection.Elements = append(activeSection.Elements, currentElements...)
					if len(currentElements) > 0 {
						lastEl := currentElements[len(currentElements)-1]
						if lastEl.LineEnd > activeSection.LineEnd {
							activeSection.LineEnd = lastEl.LineEnd
						}
					}
				} else {
					// Content before the first heading (this shouldn't include frontmatter if handled below)
					preSection := &HeadingSection{
						Elements:  currentElements,
						LineStart: currentElements[0].LineStart,
						LineEnd:   currentElements[len(currentElements)-1].LineEnd,
					}
					sections = append(sections, preSection)
				}
				currentElements = []MarkdownElement{}
			}

			// Adjust stack for the new heading's level
			newLevel := elementAtIndex.Level
			for len(stack) > 0 && stack[len(stack)-1].Heading.Level >= newLevel {
				stack = stack[:len(stack)-1] // Pop sections of same or higher level
			}

			// Create new section for this heading
			newSection := &HeadingSection{
				Heading:   elementAtIndex,
				Elements:  []MarkdownElement{},
				Children:  []*HeadingSection{},
				LineStart: elementAtIndex.LineStart,
				LineEnd:   elementAtIndex.LineEnd,
			}

			// If frontmatter exists and this is the very first "structural" section, prepend frontmatter to it.
			if frontmatterToPrepend != nil && len(stack) == 0 && len(sections) == 0 {
				newSection.Elements = append(newSection.Elements, *frontmatterToPrepend)
				if frontmatterToPrepend.LineStart < newSection.LineStart {
					newSection.LineStart = frontmatterToPrepend.LineStart
				}
				frontmatterToPrepend = nil // Consumed
			}

			// Add to parent's children or to root sections
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, newSection)
			} else {
				sections = append(sections, newSection)
			}
			stack = append(stack, newSection)
		} else {
			currentElements = append(currentElements, *elementAtIndex)
		}
	}

	// Handle any remaining elements after the last heading
	if len(currentElements) > 0 {
		if len(stack) > 0 {
			activeSection := stack[len(stack)-1]
			activeSection.Elements = append(activeSection.Elements, currentElements...)
			if len(currentElements) > 0 {
				lastEl := currentElements[len(currentElements)-1]
				if lastEl.LineEnd > activeSection.LineEnd {
					activeSection.LineEnd = lastEl.LineEnd
				}
			}
		} else {
			// No headings at all in the document, but there were elements (e.g., plain text file)
			// Or, only frontmatter was present and handled below.
			// If frontmatterToPrepend is still here, it means it was the ONLY thing.
			if frontmatterToPrepend != nil && len(currentElements) == 0 {
				currentElements = append(currentElements, *frontmatterToPrepend)
				frontmatterToPrepend = nil
			}
			if len(currentElements) > 0 {
				section := &HeadingSection{
					Elements:  currentElements,
					LineStart: currentElements[0].LineStart,
					LineEnd:   currentElements[len(currentElements)-1].LineEnd,
				}
				sections = append(sections, section)
			}
		}
	}

	// If only frontmatter existed and nothing else
	if frontmatterToPrepend != nil && len(sections) == 0 && len(stack) == 0 && len(currentElements) == 0 {
		section := &HeadingSection{
			Elements:  []MarkdownElement{*frontmatterToPrepend},
			LineStart: frontmatterToPrepend.LineStart,
			LineEnd:   frontmatterToPrepend.LineEnd,
		}
		sections = append(sections, section)
	}

	// Update section line ranges recursively to ensure they encompass all children and elements.
	p.updateSectionRanges(sections)

	return sections
}

// updateSectionRanges updates the line ranges for all sections recursively
func (p *MarkdownPlugin) updateSectionRanges(sections []*HeadingSection) {
	for _, section := range sections {
		p.updateSectionRange(section) // Update children first, then parent
	}
}

// updateSectionRange updates the line range for a single section based on its direct elements and children
func (p *MarkdownPlugin) updateSectionRange(section *HeadingSection) {
	// Recursively update children first to ensure their ranges are finalized
	p.updateSectionRanges(section.Children)

	minLine := -1 // Use -1 to indicate not yet found
	maxLine := -1

	// Consider the heading of the current section
	if section.Heading != nil {
		minLine = section.Heading.LineStart
		maxLine = section.Heading.LineEnd
	}

	// Consider all direct elements within this section
	for _, element := range section.Elements {
		if minLine == -1 || element.LineStart < minLine {
			minLine = element.LineStart
		}
		if element.LineEnd > maxLine {
			maxLine = element.LineEnd
		}
	}

	// Consider all child sections (their ranges are already updated recursively)
	for _, child := range section.Children {
		if minLine == -1 || child.LineStart < minLine {
			minLine = child.LineStart
		}
		if child.LineEnd > maxLine {
			maxLine = child.LineEnd
		}
	}

	// Apply the determined range to the section
	if minLine != -1 { // Only update if any content/structure was found
		section.LineStart = minLine
		section.LineEnd = maxLine
	} else {
		// If a section truly has no content (e.g., a heading with no text, no elements, no children),
		// set its range to 0 or leave it as default to indicate emptiness.
		section.LineStart = 0
		section.LineEnd = 0
	}
}

// sectionsToChunks converts hierarchical sections to flat chunks
func (p *MarkdownPlugin) sectionsToChunks(sections []*HeadingSection, doc *DocumentStructure, path string) []model.CodeChunk {
	var chunks []model.CodeChunk

	for _, section := range sections {
		chunks = append(chunks, p.sectionToChunks(section, doc, path)...)
	}

	return chunks
}

// sectionToChunks converts a single section (and its children recursively) to chunks
func (p *MarkdownPlugin) sectionToChunks(section *HeadingSection, doc *DocumentStructure, path string) []model.CodeChunk {
	var chunks []model.CodeChunk

	var identifier string
	chunkType := "document" // Default for sections without a heading
	level := 0

	if section.Heading == nil {
		identifier = doc.Title
	} else {
		identifier = section.Heading.Identifier
		chunkType = "heading_section"
		level = section.Heading.Level
	}

	if section.Heading == nil {
		identifier = doc.Title // doc.Title is pre-populated with filename fallback by Chunk()
		chunkType = "document"
	}

	var contentParts []string
	var separatedCodeBlocks []model.CodeChunk

	// Add heading content if present
	if section.Heading != nil {
		contentParts = append(contentParts, section.Heading.Content)
	}

	// Add elements of the current section
	// Frontmatter, if part of Elements (for the first section), will be included here.
	for _, element := range section.Elements {
		if element.Type == "code_block" && p.shouldSeparateCodeBlock(&element) {
			codeChunk := model.CodeChunk{
				Content:    element.Content,
				LineStart:  element.LineStart,
				LineEnd:    element.LineEnd,
				Type:       "code_block",
				Identifier: p.getCodeBlockIdentifier(&element, identifier),
				Annotations: map[string]string{
					"type":           "code_block",
					"language":       element.Language,
					"parent_section": identifier,
				},
			}
			separatedCodeBlocks = append(separatedCodeBlocks, codeChunk)
		} else {
			contentParts = append(contentParts, element.Content)
		}
	}

	// Create chunk for the main content of this section (excluding separated code blocks)
	if len(contentParts) > 0 {
		currentSectionContent := strings.Join(contentParts, "\n")
		// Ensure content ends with a newline for consistency if it's not just a heading
		if len(section.Elements) > 0 && !strings.HasSuffix(currentSectionContent, "\n") {
			currentSectionContent += "\n"
		}

		// Determine line start/end for this specific chunk
		// It should be based on the actual content included, but section.LineStart/End reflects the whole section including children.
		// For simplicity, we'll use section.LineStart and section.LineEnd for the main section chunk.
		// More precise line numbers would require tracking lines of contentParts.
		chunkLineStart := section.LineStart
		chunkLineEnd := section.LineEnd

		annotations := map[string]string{"type": chunkType}
		if level > 0 {
			annotations["level"] = strconv.Itoa(level)
		}

		chunk := model.CodeChunk{
			Content:       currentSectionContent,
			LineStart:     chunkLineStart, // Use the overall section's start/end
			LineEnd:       chunkLineEnd,   // for the main textual content of this level.
			Type:          chunkType,
			Identifier:    identifier,
			Annotations:   annotations,
			ParentContext: p.buildMarkdownParentContext(section, doc),
			ContextLevel:  level,
		}
		chunks = append(chunks, chunk)
	}

	// Add the separated code blocks that belonged to this section level
	chunks = append(chunks, separatedCodeBlocks...)

	// Recursively process children sections
	for _, child := range section.Children {
		chunks = append(chunks, p.sectionToChunks(child, doc, path)...)
	}

	return chunks
}

// shouldSeparateCodeBlock determines if a code block should be its own chunk
func (p *MarkdownPlugin) shouldSeparateCodeBlock(element *MarkdownElement) bool {
	lines := strings.Split(element.Content, "\n")
	contentLines := 0
	if len(lines) > 2 { // Needs at least ```, content, ```
		contentLines = len(lines) - 2
	}

	// Separate if code block is large (>15 lines of actual content)
	if contentLines > 15 {
		return true
	}

	// Separate if it's a complete program/function (has certain keywords)
	// This is heuristic and might need refinement
	content := strings.ToLower(element.Content)
	if strings.Contains(content, "func ") || // Go
		strings.Contains(content, "function ") || // JS, PHP
		strings.Contains(content, "class ") || // Python, Java, JS, C++, etc.
		strings.Contains(content, "def ") || // Python, Ruby
		strings.Contains(content, "public static void main") || // Java
		strings.Contains(content, "module.exports") || // Node.js
		(strings.Contains(content, "const ") && strings.Contains(content, "async")) { // JS async functions
		return true
	}

	return false
}

// getCodeBlockIdentifier creates an identifier for a separated code block
func (p *MarkdownPlugin) getCodeBlockIdentifier(element *MarkdownElement, parentIdentifier string) string {
	if element.Language != "" {
		return parentIdentifier + " (" + element.Language + " code)"
	}
	return parentIdentifier + " (code block)"
}

func (p *MarkdownPlugin) buildMarkdownParentContext(section *HeadingSection, doc *DocumentStructure) string {
	var context strings.Builder

	if doc.Title != "" {
		context.WriteString(fmt.Sprintf("// Document: %s\n", doc.Title))
	}

	if section.Heading != nil {
		context.WriteString(fmt.Sprintf("// Section: %s (Level %d)\n",
			section.Heading.Identifier, section.Heading.Level))
	}

	return context.String()
}
