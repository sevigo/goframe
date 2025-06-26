package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"maps"

	"github.com/ledongthuc/pdf"

	internal_model "github.com/sevigo/goframe/schema"
)

const (
	paragraphSeparatorRegex = `(\r\n|\r|\n){2,}`
	pageMarkerTemplate      = "\n--- Page %d ---\n"
	lineSeparator           = "\n"
)

// Basic section patterns for general documents
var sectionPatterns = []*regexp.Regexp{
	// General document patterns
	regexp.MustCompile(`(?im)^\s*(Chapter\s+\d+\.?\s*.*?)$`),
	regexp.MustCompile(`(?im)^\s*(Section\s+\d+\.?\s*.*?)$`),
	regexp.MustCompile(`(?im)^\s*(\d+\.\s+[A-Z][^.]*?)$`),
	regexp.MustCompile(`(?im)^\s*([A-Z][A-Z\s]{10,})\s*$`), // All caps headers
	// Common document headers
	regexp.MustCompile(`(?im)^\s*(Introduction)\s*$`),
	regexp.MustCompile(`(?im)^\s*(Abstract)\s*$`),
	regexp.MustCompile(`(?im)^\s*(Summary)\s*$`),
	regexp.MustCompile(`(?im)^\s*(Conclusion)\s*$`),
	regexp.MustCompile(`(?im)^\s*(References)\s*$`),
	regexp.MustCompile(`(?im)^\s*(Bibliography)\s*$`),
	regexp.MustCompile(`(?im)^\s*(Appendix)\s*$`),
}

// textExtractionResult holds text and page number from extraction
type textExtractionResult struct {
	Text          string
	PageNum       int
	LineOffsetDoc int
}

// extractTextFromPDF extracts text content from a PDF file
func (p *PDFPlugin) extractTextFromPDF(filePath string) ([]textExtractionResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF file %s: %w", filePath, err)
	}
	defer f.Close()

	fsInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info for %s: %w", filePath, err)
	}

	pdfReader, err := pdf.NewReader(f, fsInfo.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to create PDF reader for %s: %w", filePath, err)
	}

	numPages := pdfReader.NumPage()
	if numPages == 0 {
		p.logger.Warn("PDF has no pages", "path", filePath)
		return []textExtractionResult{}, nil
	}

	p.logger.Debug("PDF text extraction starting", "path", filePath, "pages", numPages)

	var pageTexts []textExtractionResult
	currentDocLineOffset := 0

	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			p.logger.Warn("Skipping null page", "page", i, "path", filePath)
			currentDocLineOffset += strings.Count(fmt.Sprintf(pageMarkerTemplate, i), "\n")
			continue
		}

		pageContent := p.extractPageText(page, i, filePath)
		pageStr := strings.TrimSpace(pageContent)

		// Add line offset for page marker
		pageMarkerLines := strings.Count(fmt.Sprintf(pageMarkerTemplate, i), "\n")
		currentDocLineOffset += pageMarkerLines

		if pageStr != "" {
			pageTexts = append(pageTexts, textExtractionResult{
				Text:          pageStr,
				PageNum:       i,
				LineOffsetDoc: currentDocLineOffset,
			})
			currentDocLineOffset += strings.Count(pageStr, "\n") + 1
		} else {
			currentDocLineOffset += 1
		}
	}

	if len(pageTexts) == 0 {
		return nil, errors.New("no text extracted from PDF")
	}

	p.logger.Debug("PDF text extraction finished", "path", filePath, "pages_with_text", len(pageTexts))
	return pageTexts, nil
}

// extractPageText extracts text from a single PDF page
func (p *PDFPlugin) extractPageText(page pdf.Page, pageNum int, filePath string) string {
	if pageContent, err := page.GetPlainText(nil); err == nil && strings.TrimSpace(pageContent) != "" {
		return p.cleanExtractedText(pageContent)
	}

	var textBuilder bytes.Buffer
	content := page.Content()

	if len(content.Text) > 0 {
		for i, token := range content.Text {
			textBuilder.WriteString(token.S)

			if i < len(content.Text)-1 && !strings.HasSuffix(token.S, " ") && !strings.HasSuffix(token.S, "\n") {
				textBuilder.WriteString(" ")
			}
		}

		extractedText := textBuilder.String()
		if strings.TrimSpace(extractedText) != "" {
			return p.cleanExtractedText(extractedText)
		}
	}

	p.logger.Debug("No text extracted from page", "page", pageNum, "path", filePath)
	return ""
}

// cleanExtractedText normalizes extracted text
func (p *PDFPlugin) cleanExtractedText(text string) string {
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n[ \t]*\n`).ReplaceAllString(text, "\n\n")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")

	text = strings.ReplaceAll(text, "ﬂ", "fl")
	text = strings.ReplaceAll(text, `"`, `\"`)

	return strings.TrimSpace(text)
}

func (p *PDFPlugin) identifyMajorSections(fullTextWithMarkers string, filePath string, opts *internal_model.CodeChunkingOptions) []internal_model.CodeChunk {
	var sections []internal_model.CodeChunk

	for _, pattern := range sectionPatterns {
		sections = p.findSectionsWithPattern(fullTextWithMarkers, pattern, filePath, opts)
		if len(sections) > 0 {
			p.logger.Debug("Found sections with pattern", "count", len(sections))
			break
		}
	}

	if len(sections) == 0 {
		sections = p.identifyFormattedSections(fullTextWithMarkers, filePath, opts)
	}

	return sections
}

func (p *PDFPlugin) findSectionsWithPattern(
	fullTextWithMarkers string,
	pattern *regexp.Regexp,
	_ string,
	opts *internal_model.CodeChunkingOptions,
) []internal_model.CodeChunk {
	var sections []internal_model.CodeChunk

	allHeaderMatches := pattern.FindAllStringSubmatchIndex(fullTextWithMarkers, -1)
	if len(allHeaderMatches) == 0 {
		return sections
	}

	for i, matchIndices := range allHeaderMatches {
		headerLineText := fullTextWithMarkers[matchIndices[2]:matchIndices[3]]
		headerStartOffset := matchIndices[0]

		var sectionContent string
		if i+1 < len(allHeaderMatches) {
			nextHeaderStartOffset := allHeaderMatches[i+1][0]
			sectionContent = fullTextWithMarkers[matchIndices[1]:nextHeaderStartOffset]
		} else {
			sectionContent = fullTextWithMarkers[matchIndices[1]:]
		}

		sectionContent = strings.TrimSpace(sectionContent)

		if len(sectionContent) < opts.MinCharsPerChunk {
			continue
		}

		startLine := 1 + strings.Count(fullTextWithMarkers[:headerStartOffset], "\n")
		endLine := startLine + strings.Count(sectionContent, "\n")

		chunk := internal_model.CodeChunk{
			Identifier: sanitizeIdentifier(headerLineText),
			Content:    sectionContent,
			Type:       "document_section",
			LineStart:  startLine,
			LineEnd:    endLine,
			Annotations: map[string]string{
				"section_title": strings.TrimSpace(headerLineText),
				"source_parser": "pdf_section_pattern",
			},
		}

		textUpToSectionContent := fullTextWithMarkers[:matchIndices[1]]
		pageMatches := regexp.MustCompile(`--- Page (\d+) ---`).FindAllStringSubmatch(textUpToSectionContent, -1)
		if len(pageMatches) > 0 {
			chunk.Annotations["start_page_approx"] = pageMatches[len(pageMatches)-1][1]
		}

		sections = append(sections, chunk)
	}

	return sections
}

func (p *PDFPlugin) identifyFormattedSections(fullTextWithMarkers string, _ string, opts *internal_model.CodeChunkingOptions) []internal_model.CodeChunk {
	var sections []internal_model.CodeChunk
	lines := strings.Split(fullTextWithMarkers, "\n")

	var currentSection *internal_model.CodeChunk
	var sectionContent strings.Builder

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == "" || strings.Contains(trimmedLine, "--- Page") {
			if currentSection != nil {
				sectionContent.WriteString(line + "\n")
			}
			continue
		}

		if p.isLikelySectionHeader(trimmedLine) {
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(sectionContent.String())
				if len(currentSection.Content) >= opts.MinCharsPerChunk {
					currentSection.LineEnd = i
					sections = append(sections, *currentSection)
				}
			}

			currentSection = &internal_model.CodeChunk{
				Identifier: sanitizeIdentifier(trimmedLine),
				Type:       "document_section",
				LineStart:  i + 1,
				Annotations: map[string]string{
					"section_title": trimmedLine,
					"source_parser": "pdf_formatted_sections",
				},
			}
			sectionContent.Reset()
		} else if currentSection != nil {
			sectionContent.WriteString(line + "\n")
		}
	}

	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(sectionContent.String())
		if len(currentSection.Content) >= opts.MinCharsPerChunk {
			currentSection.LineEnd = len(lines)
			sections = append(sections, *currentSection)
		}
	}

	return sections
}

func (p *PDFPlugin) isLikelySectionHeader(line string) bool {
	if len(line) > 100 {
		return false
	}

	if len(line) < 5 {
		return false
	}

	words := strings.Fields(line)
	if len(words) < 1 || len(words) > 10 {
		return false
	}

	patterns := []string{
		`^\d+\.`,         // "1. Introduction"
		`^Chapter\s+\d+`, // "Chapter 1"
		`^Section\s+\d+`, // "Section 1"
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}

	uppercaseCount := 0
	letterCount := 0
	for _, r := range line {
		if unicode.IsLetter(r) {
			letterCount++
			if unicode.IsUpper(r) {
				uppercaseCount++
			}
		}
	}

	if letterCount > 0 && float64(uppercaseCount)/float64(letterCount) > 0.6 {
		return true
	}

	return false
}

func (p *PDFPlugin) splitContentIntoParagraphs(text string, lineOffset int, pageNum int, filePath string, sectionIdentifier string, opts *internal_model.CodeChunkingOptions) []internal_model.CodeChunk {
	var chunks []internal_model.CodeChunk
	paraRegex := regexp.MustCompile(paragraphSeparatorRegex)
	paragraphs := paraRegex.Split(text, -1)

	currentLineInDoc := lineOffset

	for i, para := range paragraphs {
		trimmedPara := strings.TrimSpace(para)
		if len(trimmedPara) < opts.MinCharsPerChunk {
			currentLineInDoc += strings.Count(para, "\n") + 1
			if i < len(paragraphs)-1 {
				currentLineInDoc++
			}
			continue
		}

		paraLineCount := strings.Count(trimmedPara, "\n") + 1
		chunkIdentifier := fmt.Sprintf("%s_pg%d_para%d", sanitizeIdentifier(sectionIdentifier), pageNum, i+1)
		if sectionIdentifier == "page_content" {
			chunkIdentifier = fmt.Sprintf("%s_pg%d_para%d", sanitizeFilename(filepath.Base(filePath)), pageNum, i+1)
		}

		chunk := internal_model.CodeChunk{
			Identifier: chunkIdentifier,
			Content:    trimmedPara,
			Type:       "paragraph",
			LineStart:  currentLineInDoc,
			LineEnd:    currentLineInDoc + paraLineCount - 1,
			Annotations: map[string]string{
				"source_parser": "pdf_paragraph_split",
			},
		}
		if pageNum > 0 {
			chunk.Annotations["page_number"] = strconv.Itoa(pageNum)
		}
		chunks = append(chunks, chunk)

		currentLineInDoc += strings.Count(para, "\n") + 1
		if i < len(paragraphs)-1 {
			currentLineInDoc++
		}
	}
	return chunks
}

func (p *PDFPlugin) splitOversizedChunks(chunks []internal_model.CodeChunk, _ string, opts *internal_model.CodeChunkingOptions) []internal_model.CodeChunk {
	var finalChunks []internal_model.CodeChunk
	for _, chunk := range chunks {
		if len(chunk.Content) > opts.MaxLinesPerChunk || (chunk.LineEnd-chunk.LineStart+1) > opts.MaxLinesPerChunk {
			p.logger.Debug("Splitting oversized chunk",
				"identifier", chunk.Identifier,
				"chars", len(chunk.Content),
				"lines", chunk.LineEnd-chunk.LineStart+1)

			lines := strings.Split(chunk.Content, "\n")
			subChunkContent := strings.Builder{}
			subChunkStartLine := chunk.LineStart
			linesInSubChunk := 0
			subChunkIndex := 0

			for lineIdx, line := range lines {
				if linesInSubChunk > 0 {
					subChunkContent.WriteString("\n")
				}
				subChunkContent.WriteString(line)
				linesInSubChunk++

				if linesInSubChunk >= opts.MaxLinesPerChunk || subChunkContent.Len() >= opts.MaxLinesPerChunk || lineIdx == len(lines)-1 {
					if strings.TrimSpace(subChunkContent.String()) != "" {
						finalChunks = append(finalChunks, internal_model.CodeChunk{
							Identifier:  fmt.Sprintf("%s_split_%d", chunk.Identifier, subChunkIndex),
							Content:     subChunkContent.String(),
							Type:        chunk.Type + "_split",
							LineStart:   subChunkStartLine,
							LineEnd:     subChunkStartLine + linesInSubChunk - 1,
							Annotations: p.copyAnnotations(chunk.Annotations),
						})
						subChunkIndex++
					}
					subChunkContent.Reset()
					subChunkStartLine += linesInSubChunk
					linesInSubChunk = 0
				}
			}
		} else {
			finalChunks = append(finalChunks, chunk)
		}
	}
	return finalChunks
}

// filterSmallChunks removes chunks that are too small to be useful
func (p *PDFPlugin) filterSmallChunks(chunks []internal_model.CodeChunk, opts *internal_model.CodeChunkingOptions) []internal_model.CodeChunk {
	var filtered []internal_model.CodeChunk
	for _, chunk := range chunks {
		if len(strings.TrimSpace(chunk.Content)) >= opts.MinCharsPerChunk {
			filtered = append(filtered, chunk)
		} else {
			p.logger.Debug("Filtering small chunk",
				"identifier", chunk.Identifier,
				"chars", len(strings.TrimSpace(chunk.Content)))
		}
	}
	return filtered
}

// ExtractMetadata extracts metadata from the PDF file
func (p *PDFPlugin) ExtractMetadata(content string, filePath string) (internal_model.FileMetadata, error) {
	_ = content // Ignored for PDFs
	p.logger.Debug("Extracting PDF metadata", "path", filePath)

	metadata := internal_model.FileMetadata{
		Language:   "pdf_document",
		FilePath:   filepath.Base(filePath),
		Properties: make(map[string]string),
	}

	f, err := os.Open(filePath)
	if err != nil {
		return metadata, fmt.Errorf("failed to open PDF file: %w", err)
	}
	defer f.Close()

	fsInfo, err := f.Stat()
	if err != nil {
		return metadata, fmt.Errorf("failed to get file info: %w", err)
	}

	pdfReader, err := pdf.NewReader(f, fsInfo.Size())
	if err != nil {
		return metadata, fmt.Errorf("failed to create PDF reader: %w", err)
	}

	numPages := pdfReader.NumPage()
	metadata.Properties["page_count"] = strconv.Itoa(numPages)
	metadata.Properties["file_size_bytes"] = strconv.FormatInt(fsInfo.Size(), 10)
	metadata.Properties["mod_time"] = fsInfo.ModTime().Format(time.RFC3339)

	// Extract title heuristic from first page
	if numPages > 0 {
		pageResults, extractErr := p.extractTextFromPDF(filePath)
		if extractErr == nil && len(pageResults) > 0 {
			firstPageText := pageResults[0].Text
			if title := extractHeuristicTitle(firstPageText); title != "" {
				metadata.Properties["title_heuristic"] = title
			}
		}
	}

	return metadata, nil
}

// Helper functions
func extractHeuristicTitle(text string) string {
	lines := strings.SplitN(text, "\n", 5)
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) > 10 && len(trimmedLine) < 150 {
			if i < 2 || isMostlyCapsOrTitleCase(trimmedLine) {
				return trimmedLine
			}
		}
	}
	return ""
}

func isMostlyCapsOrTitleCase(s string) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}
	titleCaseWords := 0
	for _, word := range words {
		if len(word) > 0 && unicode.IsUpper(rune(word[0])) {
			titleCaseWords++
		}
	}
	return float64(titleCaseWords)/float64(len(words)) >= 0.6
}

func sanitizeIdentifier(id string) string {
	id = strings.ToLower(id)
	id = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(id, "_")
	id = strings.Trim(id, "_")
	id = regexp.MustCompile(`_{2,}`).ReplaceAllString(id, "_")
	if len(id) > 60 {
		id = id[:60]
	}
	if id == "" {
		return "untitled_section"
	}
	return id
}

func sanitizeFilename(filename string) string {
	// Remove extension and sanitize
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	return sanitizeIdentifier(name)
}

func (p *PDFPlugin) copyAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return make(map[string]string)
	}
	newMap := make(map[string]string, len(annotations))
	maps.Copy(newMap, annotations)
	return newMap
}
