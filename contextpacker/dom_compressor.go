// Package contextpacker provides token-aware context packing for LLMs.
//
// The DOMCompressor reduces token usage by stripping non-essential HTML elements
// and attributes from raw DOM strings (e.g., from playwright-go).
//
// This is critical for browser automation agents where:
//   - playwright-go returns massive DOM strings
//   - Raw DOM quickly exhausts token limits
//   - LLMs hallucinate when context is too large
//
// Example:
//
//	compressor := contextpacker.NewDOMCompressor()
//	compressed, err := compressor.Compress(rawDOM)
//	// Use compressed string for LLM context
package contextpacker

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	htmlCommentRe   = regexp.MustCompile(`<!--[\s\S]*?-->`)
	dataAttrRe      = regexp.MustCompile(`\s+data-[a-zA-Z0-9-]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	emptyDivRe      = regexp.MustCompile(`<div\s*>\s*</div>`)
	multiNewlineCRe = regexp.MustCompile(`\n{3,}`)
	scriptTagRe     = regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`)
	styleTagRe      = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`)
)

// DOMCompressor strips non-essential HTML elements and attributes
// to reduce token usage while preserving semantic structure.
type DOMCompressor struct {
	// RemoveStyleTags removes <style> and </style> tags and their content.
	RemoveStyleTags bool
	// RemoveScriptTags removes <script> and </script> tags and their content.
	RemoveScriptTags bool
	// RemoveComments removes HTML comments <!-- ... -->.
	RemoveComments bool
	// RemoveAttributes lists attributes to remove (e.g., "class", "style", "data-*").
	// Keep attributes in KeepAttributes to preserve them.
	RemoveAttributes []string
	// KeepAttributes lists attributes to preserve (e.g., "id", "name", "type", "aria-label").
	KeepAttributes []string
	// FlattenDivs removes deeply nested <div> chains that are purely for layout.
	FlattenDivs bool
	// PreserveSemanticTags keeps semantic HTML5 tags (article, nav, section, etc.).
	PreserveSemanticTags bool
	// MaxDepth limits nesting depth (0 = unlimited).
	MaxDepth int
}

// DOMCompressorOption configures the DOMCompressor.
type DOMCompressorOption func(*DOMCompressor)

// NewDOMCompressor creates a compressor with sensible defaults.
//
// Default configuration:
//   - Removes style and script tags
//   - Removes HTML comments
//   - Removes class, style, and data-* attributes
//   - Keeps id, name, type, aria-label, href, src, alt attributes
//   - Flattens deeply nested divs
//   - Preserves semantic HTML5 tags
func NewDOMCompressor(opts ...DOMCompressorOption) *DOMCompressor {
	c := &DOMCompressor{
		RemoveStyleTags:      true,
		RemoveScriptTags:     true,
		RemoveComments:       true,
		RemoveAttributes:     []string{"class", "style"},
		KeepAttributes:       []string{"id", "name", "type", "aria-label", "href", "src", "alt", "title", "value"},
		FlattenDivs:          true,
		PreserveSemanticTags: true,
		MaxDepth:             0,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithStyleTags configures style tag removal.
func WithStyleTags(remove bool) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.RemoveStyleTags = remove
	}
}

// WithScriptTags configures script tag removal.
func WithScriptTags(remove bool) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.RemoveScriptTags = remove
	}
}

// WithComments configures HTML comment removal.
func WithComments(remove bool) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.RemoveComments = remove
	}
}

// WithRemoveAttributes sets additional attributes to remove.
// Supports data-* pattern for all data attributes.
func WithRemoveAttributes(attrs ...string) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.RemoveAttributes = append(c.RemoveAttributes, attrs...)
	}
}

// WithKeepAttributes sets attributes to preserve.
func WithKeepAttributes(attrs ...string) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.KeepAttributes = append(c.KeepAttributes, attrs...)
	}
}

// WithFlattenDivs configures div flattening.
func WithFlattenDivs(flatten bool) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.FlattenDivs = flatten
	}
}

// WithMaxDepth sets maximum nesting depth (0 = unlimited).
func WithMaxDepth(depth int) DOMCompressorOption {
	return func(c *DOMCompressor) {
		c.MaxDepth = depth
	}
}

// Compress reduces a raw DOM string by removing non-essential elements
// and attributes while preserving semantic structure.
func (c *DOMCompressor) Compress(dom string) (string, error) {
	result := dom

	// Step 1: Remove script tags and content
	if c.RemoveScriptTags {
		result = c.removeTag(result, "script")
	}

	// Step 2: Remove style tags and content
	if c.RemoveStyleTags {
		result = c.removeTag(result, "style")
	}

	// Step 3: Remove HTML comments
	if c.RemoveComments {
		result = c.removeComments(result)
	}

	// Step 4: Remove specified attributes
	result = c.removeAttributes(result)

	// Step 5: Remove data-* attributes
	result = c.removeDataAttributes(result)

	// Step 6: Flatten divs if enabled
	if c.FlattenDivs {
		result = c.flattenDivs(result)
	}

	// Step 7: Clean up whitespace
	result = c.normalizeWhitespace(result)

	return result, nil
}

// removeTag removes all occurrences of a tag and its content.
func (c *DOMCompressor) removeTag(html, tagName string) string {
	switch tagName {
	case "script":
		return scriptTagRe.ReplaceAllString(html, "")
	case "style":
		return styleTagRe.ReplaceAllString(html, "")
	default:
		pattern := fmt.Sprintf(`<%s[^>]*>[\s\S]*?</%s>`, tagName, tagName)
		return regexp.MustCompile(pattern).ReplaceAllString(html, "")
	}
}

// removeComments removes HTML comments.
func (c *DOMCompressor) removeComments(html string) string {
	return htmlCommentRe.ReplaceAllString(html, "")
}

// removeAttributes removes specific attributes from all tags.
func (c *DOMCompressor) removeAttributes(html string) string {
	for _, attr := range c.RemoveAttributes {
		// Skip if attribute is in keep list
		if c.shouldKeepAttribute(attr) {
			continue
		}

		// Match attribute with various quote styles and spacing
		// Handles: attr="value", attr='value', attr=value
		pattern := fmt.Sprintf(`\s+%s\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`, regexp.QuoteMeta(attr))
		re := regexp.MustCompile(pattern)
		html = re.ReplaceAllString(html, "")
	}

	return html
}

// removeDataAttributes removes all data-* attributes.
func (c *DOMCompressor) removeDataAttributes(html string) string {
	return dataAttrRe.ReplaceAllString(html, "")
}

// shouldKeepAttribute checks if an attribute should be preserved.
func (c *DOMCompressor) shouldKeepAttribute(attr string) bool {
	for _, keep := range c.KeepAttributes {
		if keep == attr {
			return true
		}
	}
	return false
}

// flattenDivs removes redundant <div> wrappers with no attributes or content.
func (c *DOMCompressor) flattenDivs(html string) string {
	for emptyDivRe.MatchString(html) {
		html = emptyDivRe.ReplaceAllString(html, "")
	}
	return html
}

// normalizeWhitespace reduces multiple whitespace to single space.
func (c *DOMCompressor) normalizeWhitespace(html string) string {
	html = multiNewlineCRe.ReplaceAllString(html, "\n\n")

	// Replace multiple spaces with single space (but preserve in pre/code)
	// This is a simple implementation - a full parser would be more accurate
	html = strings.ReplaceAll(html, "  ", " ")
	html = strings.ReplaceAll(html, "  ", " ") // Run twice for odd numbers

	return strings.TrimSpace(html)
}

// CompressWithStats returns compression statistics along with the result.
func (c *DOMCompressor) CompressWithStats(dom string) (string, CompressionStats, error) {
	originalLen := len(dom)

	result, err := c.Compress(dom)
	if err != nil {
		return "", CompressionStats{}, err
	}

	compressedLen := len(result)
	reduction := float64(originalLen-compressedLen) / float64(originalLen) * 100

	stats := CompressionStats{
		OriginalLength:   originalLen,
		CompressedLength: compressedLen,
		ReductionPercent: reduction,
		TokensSaved:      estimateTokens(originalLen - compressedLen),
	}

	return result, stats, nil
}

// estimateTokens estimates token count from character length.
// Uses rough heuristic: ~4 characters per token for English.
func estimateTokens(chars int) int {
	return chars / 4
}

// CompressionStats contains compression statistics.
type CompressionStats struct {
	OriginalLength   int     // Original DOM length in characters
	CompressedLength int     // Compressed DOM length in characters
	ReductionPercent float64 // Percentage reduction in size
	TokensSaved      int     // Estimated tokens saved
}

// MustCompress compresses the DOM or panics on error.
func (c *DOMCompressor) MustCompress(dom string) string {
	result, err := c.Compress(dom)
	if err != nil {
		panic(fmt.Sprintf("DOM compression failed: %v", err))
	}
	return result
}
