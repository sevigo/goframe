package html

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// normalizeLinks converts relative URLs to absolute URLs using the base URL.
// This enables agents to follow links and access additional context.
//
// Transformations:
//   - relative://article/123 → https://base.com/article/123
//   - Skips anchors (#foo), javascript:, mailto:, already-absolute URLs
//   - Normalizes both <a href=""> and <img src="">
func (p *HTMLParser) normalizeLinks(doc *goquery.Document) {
	if p.baseURL == "" {
		return
	}

	base, err := url.Parse(p.baseURL)
	if err != nil {
		return
	}

	// Normalize all links
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		// Skip anchors, javascript, mailto, and already-absolute URLs
		if shouldSkipURL(href) {
			return
		}

		// Resolve relative URL
		if resolvedURL := resolveRelativeURL(base, href); resolvedURL != "" {
			s.SetAttr("href", resolvedURL)
		}
	})

	// Normalize image sources
	doc.Find("img[src]").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || src == "" {
			return
		}

		// Skip data URIs and already-absolute URLs
		if strings.HasPrefix(src, "data:") || strings.Contains(src, "://") {
			return
		}

		// Resolve relative URL
		if resolvedURL := resolveRelativeURL(base, src); resolvedURL != "" {
			s.SetAttr("src", resolvedURL)
		}
	})
}

// shouldSkipURL returns true for URLs that should not be normalized.
func shouldSkipURL(href string) bool {
	lower := strings.ToLower(href)
	return strings.HasPrefix(href, "#") ||
		strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.Contains(href, "://")
}

// resolveRelativeURL resolves a relative URL against a base URL.
// Returns the absolute URL or empty string if resolution fails.
func resolveRelativeURL(base *url.URL, relative string) string {
	rel, err := url.Parse(relative)
	if err != nil {
		return ""
	}

	resolved := base.ResolveReference(rel)

	// Remove tracking parameters if configured
	// Note: This is separate from RSS normalizer's RemoveTracking
	// This handles HTML-specific tracking (onclick, etc.)
	return resolved.String()
}
