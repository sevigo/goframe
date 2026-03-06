// Package parsers provides language-specific parsing plugins for code analysis.
package parsers

import (
	"io/fs"
	"strings"
	"time"

	"github.com/sevigo/goframe/schema"
)

// RSSParser implements the ParserPlugin interface for RSS/Atom feed content.
// It handles RSS 2.0, Atom 1.0, and JSON feeds, treating each feed item as a document.
type RSSParser struct{}

// NewRSSParser creates a new RSS parser instance.
func NewRSSParser() *RSSParser {
	return &RSSParser{}
}

// Name returns the parser name identifier.
func (p *RSSParser) Name() string {
	return "rss"
}

// Extensions returns the file extensions this parser handles.
func (p *RSSParser) Extensions() []string {
	return []string{".rss", ".atom", ".xml"}
}

// CanHandle determines if this parser can handle the given file.
func (p *RSSParser) CanHandle(path string, info fs.FileInfo) bool {
	ext := strings.ToLower(path)
	for _, e := range p.Extensions() {
		if strings.HasSuffix(ext, e) {
			return true
		}
	}
	return false
}

// Chunk divides RSS content into processable chunks.
// For RSS feeds, the entire item content is treated as a single chunk.
func (p *RSSParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	chunks := []schema.CodeChunk{
		{
			Content:      content,
			LineStart:    1,
			LineEnd:      strings.Count(content, "\n") + 1,
			Type:         "rss_content",
			Identifier:   "rss_item",
			IsDefinition: false,
		},
	}
	return chunks, nil
}

// ExtractMetadata extracts metadata from RSS feed content.
// Returns basic file metadata; actual RSS metadata extraction happens in the loader.
func (p *RSSParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	return schema.FileMetadata{
		FilePath:   path,
		Language:   "rss",
		Properties: make(map[string]string),
	}, nil
}

// IsGenerated returns false as RSS feeds are typically not auto-generated code.
func (p *RSSParser) IsGenerated(content string, path string) bool {
	return false
}

// ExtractUsedSymbols returns nil as RSS feeds don't have symbol references.
func (p *RSSParser) ExtractUsedSymbols(content string) []string {
	return nil
}

// RSSItemMetadata represents metadata extracted from an RSS feed item.
type RSSItemMetadata struct {
	Title       string    `json:"title"`       // Item title
	Link        string    `json:"link"`        // Item URL
	PubDate     time.Time `json:"pub_date"`    // Publication date
	Author      string    `json:"author"`      // Author name
	Categories  []string  `json:"categories"`  // Categories/tags
	GUID        string    `json:"guid"`        // Unique identifier
	Description string    `json:"description"` // Short description
	Content     string    `json:"content"`     // Full content
}

// RSSFeedMetadata represents metadata extracted from an RSS feed channel.
type RSSFeedMetadata struct {
	Title       string `json:"title"`       // Feed title
	Link        string `json:"link"`        // Feed website URL
	Language    string `json:"language"`    // Feed language
	Description string `json:"description"` // Feed description
}
