package documentloaders

import (
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

// NormalizationConfig configures how RSS content is normalized and cleaned.
type NormalizationConfig struct {
	// StripHTML removes all HTML tags. If false, HTML is sanitized instead.
	StripHTML bool
	// MaxContentLength is the maximum length for content (0 = unlimited).
	MaxContentLength int
	// MinContentLength is the minimum length for content (items below this are skipped).
	MinContentLength int
	// RemoveTracking removes tracking parameters from URLs (UTM, fbclid, etc.).
	RemoveTracking bool
	// NormalizeURLs removes fragments and normalizes URL structure.
	NormalizeURLs bool
	// DefaultTimezone is used for parsing dates without timezone info.
	DefaultTimezone *time.Location
	// DateFormats are custom date formats to try when parsing.
	DateFormats []string
	// MinTitleLength is the minimum title length.
	MinTitleLength int
	// FallbackToURL uses URL path as title if title is too short.
	FallbackToURL bool
	// NormalizeAuthors cleans author names (removes emails, quotes).
	NormalizeAuthors bool
	// DeduplicationField specifies which field to use for deduplication ("guid" or "link").
	DeduplicationField string
}

// RSSNormalizer handles content normalization and sanitization for RSS feeds.
// It provides HTML sanitization, URL cleaning, date parsing, and metadata normalization.
type RSSNormalizer struct {
	htmlPolicy *bluemonday.Policy
	config     NormalizationConfig
}

// NewRSSNormalizer creates a new RSS normalizer with the given configuration.
// If config values are zero, sensible defaults are applied.
func NewRSSNormalizer(config NormalizationConfig) *RSSNormalizer {
	if config.MaxContentLength == 0 {
		config.MaxContentLength = 10000
	}
	if config.MinContentLength == 0 {
		config.MinContentLength = 50
	}
	if config.MinTitleLength == 0 {
		config.MinTitleLength = 3
	}
	if config.DeduplicationField == "" {
		config.DeduplicationField = "guid"
	}

	policy := bluemonday.UGCPolicy()
	policy.AllowStandardURLs()
	policy.RequireNoFollowOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)

	return &RSSNormalizer{
		htmlPolicy: policy,
		config:     config,
	}
}

// SanitizeHTML sanitizes HTML content using a safe whitelist policy.
// It removes dangerous elements (script, iframe) and attributes (onclick, etc.).
// Adds rel="nofollow noopener" and target="_blank" to external links.
func (n *RSSNormalizer) SanitizeHTML(html string) string {
	if html == "" {
		return ""
	}
	sanitized := n.htmlPolicy.Sanitize(html)
	return strings.TrimSpace(sanitized)
}

// StripHTMLTags removes all HTML tags, returning plain text.
func (n *RSSNormalizer) StripHTMLTags(html string) string {
	if html == "" {
		return ""
	}
	stripper := bluemonday.StrictPolicy()
	text := stripper.Sanitize(html)
	return strings.TrimSpace(text)
}

// NormalizeContent normalizes content by sanitizing/stripping HTML and truncating.
// It removes excess whitespace and enforces length limits.
func (n *RSSNormalizer) NormalizeContent(content string) string {
	if content == "" {
		return ""
	}

	var text string
	if n.config.StripHTML {
		text = n.StripHTMLTags(content)
	} else {
		text = n.SanitizeHTML(content)
	}

	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	if n.config.MaxContentLength > 0 && len(text) > n.config.MaxContentLength {
		text = n.truncateText(text, n.config.MaxContentLength)
	}

	return text
}

func (n *RSSNormalizer) truncateText(text string, maxLen int) string {
	if utf8.RuneCountInString(text) <= maxLen {
		return text
	}

	// Convert to runes to avoid splitting multi-byte characters
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}

	// Truncate to maxLen runes
	truncated := runes[:maxLen]

	// Try to find last space to avoid cutting words
	for i := len(truncated) - 1; i > maxLen/2; i-- {
		if truncated[i] == ' ' {
			return string(truncated[:i]) + "..."
		}
	}

	// No space found, just truncate at rune boundary
	return string(truncated) + "..."
}

// NormalizeURL cleans a URL by removing tracking parameters and fragments.
// It can remove UTM parameters, fbclid, gclid, and other tracking identifiers.
func (n *RSSNormalizer) NormalizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if n.config.RemoveTracking {
		q := parsedURL.Query()
		trackingParams := []string{
			"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
			"fbclid", "gclid", "msclkid", "ref", "source", "_ga",
		}
		for _, param := range trackingParams {
			q.Del(param)
		}
		parsedURL.RawQuery = q.Encode()
	}

	if n.config.NormalizeURLs {
		parsedURL.Fragment = ""
	}

	return parsedURL.String()
}

// ParseDate attempts to parse a date string using multiple common formats.
// Supports RFC1123, RFC1123Z, RFC822, RFC822Z, RFC3339, ISO8601, and custom formats.
// Returns a zero time if parsing fails.
func (n *RSSNormalizer) ParseDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}

	if n.config.DefaultTimezone != nil {
		if t, err := time.ParseInLocation(time.RFC1123, dateStr, n.config.DefaultTimezone); err == nil {
			return t
		}
		if t, err := time.ParseInLocation(time.RFC1123Z, dateStr, n.config.DefaultTimezone); err == nil {
			return t
		}
		if t, err := time.ParseInLocation(time.RFC822, dateStr, n.config.DefaultTimezone); err == nil {
			return t
		}
		if t, err := time.ParseInLocation(time.RFC822Z, dateStr, n.config.DefaultTimezone); err == nil {
			return t
		}
	}

	formats := []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	formats = append(formats, n.config.DateFormats...)

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

// NormalizeAuthor cleans author names by removing email addresses and quotes.
// Example: "John Doe <john@example.com>" becomes "John Doe".
func (n *RSSNormalizer) NormalizeAuthor(author string) string {
	if author == "" {
		return ""
	}

	author = strings.TrimSpace(author)

	if n.config.NormalizeAuthors {
		re := regexp.MustCompile(`(.+?)\s*<.*?>`)
		matches := re.FindStringSubmatch(author)
		if len(matches) > 1 {
			author = strings.TrimSpace(matches[1])
		}

		author = strings.TrimPrefix(author, "\"")
		author = strings.TrimSuffix(author, "\"")
		author = strings.TrimPrefix(author, "'")
		author = strings.TrimSuffix(author, "'")
	}

	return author
}

// NormalizeCategories normalizes and deduplicates category strings.
// Converts to lowercase and removes empty categories.
func (n *RSSNormalizer) NormalizeCategories(categories []string) []string {
	if len(categories) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(categories))
	seen := make(map[string]bool)

	for _, cat := range categories {
		cat = strings.TrimSpace(cat)
		cat = strings.ToLower(cat)
		if cat != "" && !seen[cat] {
			seen[cat] = true
			normalized = append(normalized, cat)
		}
	}

	return normalized
}

// NormalizeTitle normalizes the title or generates one from the URL if too short.
// Extracts the last path segment as a fallback title.
func (n *RSSNormalizer) NormalizeTitle(title string, fallbackURL string) string {
	title = strings.TrimSpace(title)

	if len(title) < n.config.MinTitleLength && n.config.FallbackToURL && fallbackURL != "" {
		parsedURL, err := url.Parse(fallbackURL)
		if err == nil {
			path := parsedURL.Path
			path = strings.TrimSuffix(path, "/")
			parts := strings.Split(path, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				if len(lastPart) > n.config.MinTitleLength {
					lastPart = strings.ReplaceAll(lastPart, "-", " ")
					lastPart = strings.ReplaceAll(lastPart, "_", " ")
					title = lastPart
				}
			}
		}
	}

	return title
}

// ShouldSkipItem determines if an item should be skipped based on content quality.
// Returns true if title or content is too short.
func (n *RSSNormalizer) ShouldSkipItem(title, content string) bool {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)

	if n.config.MinContentLength > 0 && len(content) < n.config.MinContentLength {
		return true
	}

	if len(title) < n.config.MinTitleLength {
		return true
	}

	return false
}

// ResolveURL resolves a relative URL against a base URL.
// If the URL is already absolute, it's returned as-is.
func (n *RSSNormalizer) ResolveURL(baseURL, relativeURL string) string {
	if relativeURL == "" {
		return ""
	}

	if strings.HasPrefix(relativeURL, "http://") || strings.HasPrefix(relativeURL, "https://") {
		return relativeURL
	}

	if baseURL == "" {
		return relativeURL
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return relativeURL
	}

	rel, err := url.Parse(relativeURL)
	if err != nil {
		return relativeURL
	}

	resolved := base.ResolveReference(rel)
	return resolved.String()
}
