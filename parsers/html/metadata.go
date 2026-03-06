package html

import (
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// extractMetadata extracts governance annotations from HTML metadata.
// Supports multiple metadata formats: Open Graph, Schema.org, Dublin Core, and standard meta tags.
//
// Priority order for extraction:
//  1. Schema.org ( itemprop)
//  2. Open Graph (property="og:...")
//  3. Dublin Core (name="dc:...")
//  4. Standard HTML (<meta name="author">)
//  5. Content-based (.byline, .author)
func (p *HTMLParser) extractMetadata(doc *goquery.Document) HTMLMetadata {
	if !p.metadataExtraction {
		return HTMLMetadata{}
	}

	metadata := HTMLMetadata{}

	// Extract each metadata field
	metadata.Author = p.extractAuthor(doc)
	metadata.PublishedDate = p.extractPublishedDate(doc)
	metadata.Title = p.extractTitle(doc)
	metadata.Description = p.extractDescription(doc)
	metadata.CanonicalURL = p.extractCanonicalURL(doc)
	metadata.Keywords = p.extractKeywords(doc)

	return metadata
}

// extractAuthor extracts author name from HTML metadata.
// Tries multiple sources in order of reliability.
func (p *HTMLParser) extractAuthor(doc *goquery.Document) string {
	sources := []string{
		// 1. Schema.org author
		doc.Find(`meta[itemprop="author"]`).AttrOr("content", ""),
		// 2. Open Graph article:author
		doc.Find(`meta[property="article:author"]`).AttrOr("content", ""),
		// 3. Dublin Core creator
		doc.Find(`meta[name="dc.creator"]`).AttrOr("content", ""),
		// 4. Standard meta author
		doc.Find(`meta[name="author"]`).AttrOr("content", ""),
	}

	for _, author := range sources {
		if author != "" {
			return strings.TrimSpace(author)
		}
	}

	// 5. Content-based extraction (byline, .author)
	author := ""
	doc.Find(".byline, .author, [class*='author']").Each(func(i int, s *goquery.Selection) {
		if text := strings.TrimSpace(s.Text()); text != "" && len(text) < 100 {
			author = text
			return
		}
	})

	return author
}

// extractPublishedDate extracts publication date from HTML metadata.
// Tries multiple date formats and metadata sources.
func (p *HTMLParser) extractPublishedDate(doc *goquery.Document) time.Time {
	sources := []string{
		// 1. Schema.org datePublished
		doc.Find(`meta[itemprop="datePublished"]`).AttrOr("content", ""),
		// 2. Open Graph article:published_time
		doc.Find(`meta[property="article:published_time"]`).AttrOr("content", ""),
		// 3. Dublin Core date
		doc.Find(`meta[name="dc.date"]`).AttrOr("content", ""),
		// 4. Standard meta
		doc.Find(`meta[name="pubdate"]`).AttrOr("content", ""),
		doc.Find(`meta[name="publish-date"]`).AttrOr("content", ""),
	}

	dateFormats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC1123,
		time.RFC1123Z,
	}

	for _, dateStr := range sources {
		if dateStr == "" {
			continue
		}

		for _, format := range dateFormats {
			if t, err := time.Parse(format, dateStr); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// extractTitle extracts title from HTML metadata.
func (p *HTMLParser) extractTitle(doc *goquery.Document) string {
	sources := []string{
		// 1. Open Graph title
		doc.Find(`meta[property="og:title"]`).AttrOr("content", ""),
		// 2. Schema.org headline
		doc.Find(`meta[itemprop="headline"]`).AttrOr("content", ""),
		// 3. Twitter title
		doc.Find(`meta[name="twitter:title"]`).AttrOr("content", ""),
		// 4. HTML title tag
		doc.Find("title").Text(),
	}

	for _, title := range sources {
		if title = strings.TrimSpace(title); title != "" {
			return title
		}
	}

	return ""
}

// extractDescription extracts description from HTML metadata.
func (p *HTMLParser) extractDescription(doc *goquery.Document) string {
	sources := []string{
		// 1. Open Graph description
		doc.Find(`meta[property="og:description"]`).AttrOr("content", ""),
		// 2. Schema.org description
		doc.Find(`meta[itemprop="description"]`).AttrOr("content", ""),
		// 3. Standard meta description
		doc.Find(`meta[name="description"]`).AttrOr("content", ""),
		// 4. Twitter description
		doc.Find(`meta[name="twitter:description"]`).AttrOr("content", ""),
	}

	for _, desc := range sources {
		if desc = strings.TrimSpace(desc); desc != "" {
			return desc
		}
	}

	return ""
}

// extractCanonicalURL extracts canonical URL from HTML metadata.
func (p *HTMLParser) extractCanonicalURL(doc *goquery.Document) string {
	// 1. Canonical link
	if url := doc.Find(`link[rel="canonical"]`).AttrOr("href", ""); url != "" {
		return url
	}

	// 2. Open Graph URL
	if url := doc.Find(`meta[property="og:url"]`).AttrOr("content", ""); url != "" {
		return url
	}

	return ""
}

// extractKeywords extracts keywords/tags from HTML metadata.
func (p *HTMLParser) extractKeywords(doc *goquery.Document) []string {
	keywords := []string{}

	// Extract from multiple sources
	extractFromMeta := func(selector, attr string) {
		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			if kw, exists := s.Attr(attr); exists {
				for _, k := range strings.Split(kw, ",") {
					if trimmed := strings.TrimSpace(k); trimmed != "" && len(keywords) < 20 {
						keywords = append(keywords, trimmed)
					}
				}
			}
		})
	}

	// 1. Meta keywords
	extractFromMeta(`meta[name="keywords"]`, "content")

	// 2. Schema.org keywords
	extractFromMeta(`meta[itemprop="keywords"]`, "content")

	// 3. Open Graph article:tag
	doc.Find(`meta[property="article:tag"]`).Each(func(i int, s *goquery.Selection) {
		if tag := s.AttrOr("content", ""); tag != "" {
			keywords = append(keywords, strings.TrimSpace(tag))
		}
	})

	// Deduplicate
	seen := make(map[string]bool)
	unique := []string{}
	for _, kw := range keywords {
		lower := strings.ToLower(kw)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, kw)
		}
	}

	return unique
}
