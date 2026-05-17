package html

import (
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/sevigo/goframe/schema"
)

// Chunk parses HTML content and returns code chunks with metadata.
// This method:
//  1. Parses HTML using goquery
//  2. Extracts metadata (author, date, title) - MUST be before boilerplate removal
//  3. Removes boilerplate (nav, footer, scripts)
//  4. Normalizes links (relative → absolute)
//  5. Converts to Markdown
//  6. Returns chunks with governance annotations
func (p *HTMLParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	// 1. Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 2. Extract metadata BEFORE removing boilerplate (head section contains metadata)
	metadata := p.extractMetadata(doc)

	// 3. Remove boilerplate
	p.removeBoilerplate(doc)

	// 4. Normalize links
	p.normalizeLinks(doc)

	// 5. Convert to Markdown
	markdown := p.toMarkdown(doc)

	// 6. Create chunk with metadata
	chunk := schema.CodeChunk{
		Content:      markdown,
		LineStart:    1,
		LineEnd:      countLines(markdown),
		Type:         "html_article",
		Identifier:   path,
		IsDefinition: false,
	}

	// 7. Add metadata as annotations
	if chunk.Annotations == nil {
		chunk.Annotations = make(map[string]string)
	}

	if metadata.Author != "" {
		chunk.Annotations["author"] = metadata.Author
	}

	if !metadata.PublishedDate.IsZero() {
		chunk.Annotations["published_date"] = metadata.PublishedDate.Format(time.RFC3339)
	}

	if metadata.Title != "" {
		chunk.Annotations["title"] = metadata.Title
	}

	if metadata.Description != "" {
		chunk.Annotations["description"] = metadata.Description
	}

	if metadata.CanonicalURL != "" {
		chunk.Annotations["canonical_url"] = metadata.CanonicalURL
	}

	if len(metadata.Keywords) > 0 {
		chunk.Annotations["keywords"] = strings.Join(metadata.Keywords, ", ")
	}

	// Add base URL as source context
	if p.baseURL != "" {
		chunk.Annotations["base_url"] = p.baseURL
	}

	return []schema.CodeChunk{chunk}, nil
}

// ExtractMetadata extracts file-level metadata from HTML content.
// Returns metadata about the HTML document including author, title, and other governance info.
func (p *HTMLParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return schema.FileMetadata{}, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract metadata
	htmlMeta := p.extractMetadata(doc)

	// Build file metadata
	properties := make(map[string]string)

	if htmlMeta.Author != "" {
		properties["author"] = htmlMeta.Author
	}

	if htmlMeta.Title != "" {
		properties["title"] = htmlMeta.Title
	}

	if htmlMeta.Description != "" {
		properties["description"] = htmlMeta.Description
	}

	if htmlMeta.CanonicalURL != "" {
		properties["canonical_url"] = htmlMeta.CanonicalURL
	}

	if len(htmlMeta.Keywords) > 0 {
		properties["keywords"] = strings.Join(htmlMeta.Keywords, ", ")
	}

	if !htmlMeta.PublishedDate.IsZero() {
		properties["published_date"] = htmlMeta.PublishedDate.Format(time.RFC3339)
	}

	return schema.FileMetadata{
		FilePath:   path,
		Language:   "html",
		Properties: properties,
	}, nil
}

// countLines counts the number of lines in a string.
func countLines(text string) int {
	if text == "" {
		return 1
	}
	return strings.Count(text, "\n") + 1
}
