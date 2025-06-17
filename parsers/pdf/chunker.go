package pdf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	internal_model "github.com/sevigo/goframe/schema"
)

// Chunk breaks PDF content into semantic chunks
func (p *PDFPlugin) Chunk(_ string, filePath string, opts *internal_model.CodeChunkingOptions) ([]internal_model.CodeChunk, error) {
	p.logger.Debug("PDF Chunking called", "path", filePath)

	pageTextResults, _, err := p.extractTextFromPDF(filePath)
	if err != nil {
		return nil, fmt.Errorf("pdfplugin: could not extract text from PDF %s: %w", filePath, err)
	}
	if len(pageTextResults) == 0 {
		p.logger.Warn("No text found in PDF to chunk", "path", filePath)
		return []internal_model.CodeChunk{}, nil
	}

	// Combine all page texts with page markers
	var fullTextBuilder strings.Builder
	for _, ptr := range pageTextResults {
		fullTextBuilder.WriteString(fmt.Sprintf(pageMarkerTemplate, ptr.PageNum))
		fullTextBuilder.WriteString(ptr.Text)
		fullTextBuilder.WriteString(lineSeparator)
	}
	fullDocumentTextWithMarkers := fullTextBuilder.String()

	var chunks []internal_model.CodeChunk

	// Try to identify major sections first
	identifiedSections := p.identifyMajorSections(fullDocumentTextWithMarkers, filePath, opts)

	if len(identifiedSections) > 0 {
		p.logger.Debug("Identified major sections", "count", len(identifiedSections), "path", filePath)
		for _, section := range identifiedSections {
			startPage, _ := strconv.Atoi(section.Annotations["start_page_approx"])
			paragraphChunks := p.splitContentIntoParagraphs(
				section.Content,
				section.LineStart,
				startPage,
				filePath,
				section.Identifier,
				opts,
			)
			chunks = append(chunks, paragraphChunks...)
		}
	} else {
		p.logger.Debug("No major sections identified, chunking by pages and paragraphs", "path", filePath)
		// Fallback: chunk page by page
		for _, ptr := range pageTextResults {
			pageParagraphs := p.splitContentIntoParagraphs(
				ptr.Text,
				ptr.LineOffsetDoc,
				ptr.PageNum,
				filePath,
				"page_content",
				opts,
			)
			chunks = append(chunks, pageParagraphs...)
		}
	}

	finalChunks := p.splitOversizedChunks(chunks, filePath, opts)
	finalChunks = p.filterSmallChunks(finalChunks, opts)

	// If no chunks found, return error to trigger fallback
	if len(finalChunks) == 0 {
		return nil, errors.New("no chunks found in PDF file")
	}

	p.logger.Info("PDF chunking completed", "path", filePath, "initial_sections", len(identifiedSections), "final_chunks", len(finalChunks))
	return finalChunks, nil
}
