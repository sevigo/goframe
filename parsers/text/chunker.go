package text

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	model "github.com/sevigo/goframe/schema"
)

// Chunk breaks text into semantic chunks based on paragraphs and sections
func (p *TextPlugin) Chunk(content string, path string, opts *model.CodeChunkingOptions) ([]model.CodeChunk, error) {
	if strings.TrimSpace(content) == "" {
		return []model.CodeChunk{}, nil
	}

	p.logger.Debug("Starting text chunking", "path", path, "content_length", len(content))

	structure := p.analyzeTextStructure(content)

	chunks := p.createChunks(structure, content, path, opts)

	p.logger.Debug("Text chunking completed", "path", path, "chunks_created", len(chunks))
	return chunks, nil
}

// TextStructure represents the analyzed structure of a text document
type TextStructure struct {
	Type      string
	Sections  []TextSection
	HasTitles bool
	HasLists  bool
}

// TextSection represents a logical section in the text
type TextSection struct {
	Title     string
	Content   string
	LineStart int
	LineEnd   int
	Type      string // heading, paragraph, list, code_block
}

// analyzeTextStructure analyzes the text to understand its structure
func (p *TextPlugin) analyzeTextStructure(content string) TextStructure {
	lines := strings.Split(content, "\n")
	structure := TextStructure{
		Type:     "simple",
		Sections: []TextSection{},
	}

	var currentSection *TextSection
	currentParagraph := []string{}
	lineNum := 0

	for i, line := range lines {
		lineNum = i + 1
		trimmed := strings.TrimSpace(line)

		// Empty line - end current paragraph
		if trimmed == "" {
			if len(currentParagraph) > 0 {
				p.finalizeParagraph(currentParagraph, &structure, currentSection, lineNum-len(currentParagraph), lineNum-1)
				currentParagraph = []string{}
			}
			continue
		}

		// Check for section headers
		if p.isHeading(trimmed) {
			// Finalize current paragraph and section
			if len(currentParagraph) > 0 {
				p.finalizeParagraph(currentParagraph, &structure, currentSection, lineNum-len(currentParagraph), lineNum-1)
				currentParagraph = []string{}
			}

			// Start new section
			currentSection = &TextSection{
				Title:     p.cleanHeading(trimmed),
				LineStart: lineNum,
				Type:      "heading",
			}
			structure.HasTitles = true
			structure.Type = "sectioned"
			continue
		}

		// Check for list items
		if p.isListItem(trimmed) {
			structure.HasLists = true
			if len(currentParagraph) > 0 {
				p.finalizeParagraph(currentParagraph, &structure, currentSection, lineNum-len(currentParagraph), lineNum-1)
				currentParagraph = []string{}
			}

			// Create list item as separate section
			listSection := TextSection{
				Content:   line,
				LineStart: lineNum,
				LineEnd:   lineNum,
				Type:      "list_item",
			}
			structure.Sections = append(structure.Sections, listSection)
			continue
		}

		// Regular content line
		currentParagraph = append(currentParagraph, line)
	}

	// Finalize last paragraph
	if len(currentParagraph) > 0 {
		p.finalizeParagraph(currentParagraph, &structure, currentSection, lineNum-len(currentParagraph)+1, lineNum)
	}

	// Finalize last section
	if currentSection != nil {
		currentSection.LineEnd = lineNum
		structure.Sections = append(structure.Sections, *currentSection)
	}

	return structure
}

// isHeading checks if a line looks like a heading
func (p *TextPlugin) isHeading(line string) bool {
	// Common heading patterns
	patterns := []string{
		`^[A-Z][A-Z\s]+$`,  // ALL CAPS
		`^#+\s+.*`,         // Markdown style
		`^=+$`,             // Underline with =
		`^-+$`,             // Underline with -
		`^\d+\.\s+[A-Z].*`, // Numbered sections
		`^[A-Z][^.!?]*:$`,  // Title with colon
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}

	// Check if line is short and likely a title
	if len(line) < 60 && len(strings.Fields(line)) <= 8 {
		words := strings.Fields(line)
		if len(words) > 0 {
			// Check if most words are capitalized (title case)
			capitalized := 0
			for _, word := range words {
				if len(word) > 0 && strings.ToUpper(word[:1]) == word[:1] {
					capitalized++
				}
			}
			return float64(capitalized)/float64(len(words)) > 0.6
		}
	}

	return false
}

// isListItem checks if a line is a list item
func (p *TextPlugin) isListItem(line string) bool {
	patterns := []string{
		`^\s*[-*+]\s+.*`,    // Bullet points
		`^\s*\d+\.\s+.*`,    // Numbered lists
		`^\s*[a-z]\)\s+.*`,  // Lettered lists
		`^\s*[IVX]+\.\s+.*`, // Roman numerals
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}
	return false
}

// cleanHeading removes formatting from headings
func (p *TextPlugin) cleanHeading(line string) string {
	// Remove markdown #
	cleaned := regexp.MustCompile(`^#+\s*`).ReplaceAllString(line, "")

	// Remove trailing colons
	cleaned = strings.TrimSuffix(cleaned, ":")

	// Remove extra whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// finalizeParagraph creates a paragraph section
func (p *TextPlugin) finalizeParagraph(lines []string, structure *TextStructure, currentSection *TextSection, startLine, endLine int) {
	content := strings.Join(lines, "\n")

	paragraph := TextSection{
		Content:   content,
		LineStart: startLine,
		LineEnd:   endLine,
		Type:      "paragraph",
	}

	if currentSection != nil {
		// Add to current section
		if currentSection.Content == "" {
			currentSection.Content = content
		} else {
			currentSection.Content += "\n" + content
		}
		if currentSection.LineEnd < endLine {
			currentSection.LineEnd = endLine
		}
	} else {
		// Add as standalone section
		structure.Sections = append(structure.Sections, paragraph)
	}
}

// createChunks converts the analyzed structure into chunks
func (p *TextPlugin) createChunks(structure TextStructure, content string, path string, opts *model.CodeChunkingOptions) []model.CodeChunk {
	var chunks []model.CodeChunk

	if len(structure.Sections) == 0 {
		// Fallback: create single chunk
		return []model.CodeChunk{p.createSingleChunk(content, path)}
	}

	for i, section := range structure.Sections {
		// Skip very small sections
		if len(strings.TrimSpace(section.Content)) < 20 {
			continue
		}

		chunk := p.createChunkFromSection(section, i, path, opts)

		// Split large chunks if needed
		if opts != nil && opts.MaxLinesPerChunk > 0 {
			lineCount := section.LineEnd - section.LineStart + 1
			if lineCount > opts.MaxLinesPerChunk {
				subChunks := p.splitLargeChunk(chunk, opts)
				chunks = append(chunks, subChunks...)
				continue
			}
		}

		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		// Fallback if no valid chunks created
		return []model.CodeChunk{p.createSingleChunk(content, path)}
	}

	return chunks
}

// createChunkFromSection creates a chunk from a text section
func (p *TextPlugin) createChunkFromSection(section TextSection, index int, path string, opts *model.CodeChunkingOptions) model.CodeChunk {
	identifier := p.generateIdentifier(section, index, path)

	annotations := map[string]string{
		"type":          "text_section",
		"section_type":  section.Type,
		"section_index": strconv.Itoa(index),
	}

	if section.Title != "" {
		annotations["title"] = section.Title
		annotations["has_title"] = "true"
	}

	return model.CodeChunk{
		Content:     section.Content,
		LineStart:   section.LineStart,
		LineEnd:     section.LineEnd,
		Type:        "text_section",
		Identifier:  identifier,
		Annotations: annotations,
	}
}

// generateIdentifier creates a meaningful identifier for the chunk
func (p *TextPlugin) generateIdentifier(section TextSection, index int, path string) string {
	if section.Title != "" {
		return section.Title
	}

	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	switch section.Type {
	case "heading":
		return fmt.Sprintf("%s - Section %d", baseName, index+1)
	case "list_item":
		return fmt.Sprintf("%s - List Item %d", baseName, index+1)
	case "paragraph":
		return fmt.Sprintf("%s - Paragraph %d", baseName, index+1)
	default:
		return fmt.Sprintf("%s - Part %d", baseName, index+1)
	}
}

// createSingleChunk creates a single chunk for the entire text
func (p *TextPlugin) createSingleChunk(content string, path string) model.CodeChunk {
	lines := strings.Split(content, "\n")
	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	identifier := cases.Title(language.English).String(strings.ReplaceAll(baseName, "_", " "))

	return model.CodeChunk{
		Content:    content,
		LineStart:  1,
		LineEnd:    len(lines),
		Type:       "text_document",
		Identifier: identifier,
		Annotations: map[string]string{
			"type": "text_document",
		},
	}
}

// splitLargeChunk splits a chunk that's too large
func (p *TextPlugin) splitLargeChunk(chunk model.CodeChunk, opts *model.CodeChunkingOptions) []model.CodeChunk {
	lines := strings.Split(chunk.Content, "\n")
	var subChunks []model.CodeChunk

	linesPerChunk := opts.MaxLinesPerChunk
	if linesPerChunk <= 0 {
		linesPerChunk = 50 // Default
	}

	for i := 0; i < len(lines); i += linesPerChunk {
		end := i + linesPerChunk
		if end > len(lines) {
			end = len(lines)
		}

		subContent := strings.Join(lines[i:end], "\n")
		if strings.TrimSpace(subContent) == "" {
			continue
		}

		subChunk := model.CodeChunk{
			Content:     subContent,
			LineStart:   chunk.LineStart + i,
			LineEnd:     chunk.LineStart + end - 1,
			Type:        chunk.Type + "_part",
			Identifier:  fmt.Sprintf("%s (Part %d)", chunk.Identifier, (i/linesPerChunk)+1),
			Annotations: chunk.Annotations,
		}

		subChunks = append(subChunks, subChunk)
	}

	return subChunks
}
