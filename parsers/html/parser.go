// Package html provides an HTML parser plugin for transforming HTML content
// into clean Markdown suitable for LLM consumption and RAG applications.
//
// The parser handles:
//   - Boilerplate removal (nav, footer, scripts, ads)
//   - Link normalization (relative URLs to absolute URLs)
//   - Metadata extraction (author, date, title from Open Graph, Schema.org, Dublin Core)
//   - HTML to Markdown conversion (preserving structure)
//
// This is particularly useful for RSS feeds and web content where HTML content
// needs to be cleaned and structured before being embedded in vector databases.
package html

import (
	"io/fs"
	"strings"
	"time"
)

// HTMLParser implements the ParserPlugin interface for HTML content.
// It transforms HTML into clean Markdown while preserving semantic structure
// and extracting metadata for governance annotations.
type HTMLParser struct {
	baseURL               string
	boilerplateRemoval    bool
	metadataExtraction    bool
	markdownConversion    bool
	structurePreservation bool
}

// HTMLMetadata represents metadata extracted from HTML documents.
// It supports multiple metadata formats including Open Graph, Schema.org, and Dublin Core.
type HTMLMetadata struct {
	Author        string    `json:"author"`
	PublishedDate time.Time `json:"published_date"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	CanonicalURL  string    `json:"canonical_url"`
	Keywords      []string  `json:"keywords"`
}

// Option configures the HTMLParser.
type Option func(*HTMLParser)

// NewHTMLParser creates a new HTML parser with the given options.
//
// Example:
//
//	parser := html.NewHTMLParser(
//	    html.WithBaseURL("https://example.com"),
//	    html.WithBoilerplateRemoval(true),
//	    html.WithMarkdownConversion(true),
//	)
func NewHTMLParser(opts ...Option) *HTMLParser {
	p := &HTMLParser{
		boilerplateRemoval:    true,
		metadataExtraction:    true,
		markdownConversion:    true,
		structurePreservation: true,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithBaseURL sets the base URL for resolving relative links.
// This enables conversion of relative URLs to absolute URLs.
func WithBaseURL(baseURL string) Option {
	return func(p *HTMLParser) {
		p.baseURL = baseURL
	}
}

// WithBoilerplateRemoval enables or removes non-content elements.
// When true (default), removes nav, footer, scripts, ads, etc.
func WithBoilerplateRemoval(remove bool) Option {
	return func(p *HTMLParser) {
		p.boilerplateRemoval = remove
	}
}

// WithMetadataExtraction enables extraction of author, date, and other metadata.
// Extracts from Open Graph, Schema.org, Dublin Core, and standard meta tags.
func WithMetadataExtraction(extract bool) Option {
	return func(p *HTMLParser) {
		p.metadataExtraction = extract
	}
}

// WithMarkdownConversion enables conversion of HTML to Markdown.
// When true (default), preserves semantic structure as Markdown.
func WithMarkdownConversion(convert bool) Option {
	return func(p *HTMLParser) {
		p.markdownConversion = convert
	}
}

// WithStructurePreservation enables preservation of semantic structure.
// When true (default), maintains headers, lists, code blocks as Markdown.
func WithStructurePreservation(preserve bool) Option {
	return func(p *HTMLParser) {
		p.structurePreservation = preserve
	}
}

// Name returns the parser name identifier.
func (p *HTMLParser) Name() string {
	return "html"
}

// Extensions returns the file extensions this parser handles.
func (p *HTMLParser) Extensions() []string {
	return []string{".html", ".htm"}
}

// CanHandle determines if this parser can handle the given file.
func (p *HTMLParser) CanHandle(path string, info fs.FileInfo) bool {
	ext := strings.ToLower(path)
	return strings.HasSuffix(ext, ".html") || strings.HasSuffix(ext, ".htm")
}

// IsGenerated returns false as HTML files are typically not auto-generated code.
func (p *HTMLParser) IsGenerated(content string, path string) bool {
	return false
}

// ExtractUsedSymbols returns nil as HTML doesn't have symbol references.
func (p *HTMLParser) ExtractUsedSymbols(content string) []string {
	return nil
}
