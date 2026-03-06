package documentloaders_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sevigo/goframe/documentloaders"
)

func TestRSSNormalizer_SanitizeHTML(t *testing.T) {
	config := documentloaders.NormalizationConfig{
		StripHTML: false,
	}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Safe HTML",
			input:    `<p>This is <b>bold</b> text</p>`,
			expected: `<p>This is <b>bold</b> text</p>`,
		},
		{
			name:     "Script removal",
			input:    `<p>Text</p><script>alert('xss')</script>`,
			expected: `<p>Text</p>`,
		},
		{
			name:     "Onclick removal",
			input:    `<a href="http://example.com" onclick="alert('xss')">Link</a>`,
			expected: `<a href="http://example.com" rel="nofollow noopener" target="_blank">Link</a>`,
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "Whitespace trimming",
			input:    "  <p>Text</p>  ",
			expected: "<p>Text</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.SanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_StripHTMLTags(t *testing.T) {
	config := documentloaders.NormalizationConfig{}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Remove all tags",
			input:    `<p>This is <b>bold</b> text</p>`,
			expected: "This is bold text",
		},
		{
			name:     "Complex HTML",
			input:    `<div><h1>Title</h1><p>Paragraph</p></div>`,
			expected: "TitleParagraph",
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.StripHTMLTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_NormalizeContent(t *testing.T) {
	t.Run("Strip HTML mode", func(t *testing.T) {
		config := documentloaders.NormalizationConfig{
			StripHTML:        true,
			MaxContentLength: 100,
		}
		normalizer := documentloaders.NewRSSNormalizer(config)

		input := `<p>This is <b>HTML</b> content with <a href="#">links</a></p>`
		result := normalizer.NormalizeContent(input)
		assert.Equal(t, "This is HTML content with links", result)
	})

	t.Run("Sanitize HTML mode", func(t *testing.T) {
		config := documentloaders.NormalizationConfig{
			StripHTML:        false,
			MaxContentLength: 100,
		}
		normalizer := documentloaders.NewRSSNormalizer(config)

		input := `<p>This is <b>HTML</b> content</p>`
		result := normalizer.NormalizeContent(input)
		assert.Contains(t, result, "<p>")
		assert.Contains(t, result, "<b>")
	})

	t.Run("Truncation", func(t *testing.T) {
		config := documentloaders.NormalizationConfig{
			StripHTML:        true,
			MaxContentLength: 50,
		}
		normalizer := documentloaders.NewRSSNormalizer(config)

		input := "This is a very long piece of content that should be truncated at the maximum length limit"
		result := normalizer.NormalizeContent(input)
		assert.LessOrEqual(t, len(result), 53) // 50 chars + "..."
		assert.Contains(t, result, "...")
	})

	t.Run("Whitespace normalization", func(t *testing.T) {
		config := documentloaders.NormalizationConfig{
			StripHTML: true,
		}
		normalizer := documentloaders.NewRSSNormalizer(config)

		input := "This  has   multiple    spaces\nand\ttabs"
		result := normalizer.NormalizeContent(input)
		assert.Equal(t, "This has multiple spaces and tabs", result)
	})
}

func TestRSSNormalizer_NormalizeURL(t *testing.T) {
	config := documentloaders.NormalizationConfig{
		RemoveTracking: true,
		NormalizeURLs:  true,
	}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Remove UTM parameters",
			input:    "https://example.com/article?utm_source=rss&utm_medium=feed",
			expected: "https://example.com/article",
		},
		{
			name:     "Remove fbclid",
			input:    "https://example.com/page?fbclid=abc123",
			expected: "https://example.com/page",
		},
		{
			name:     "Keep non-tracking params",
			input:    "https://example.com/page?id=123&category=tech",
			expected: "https://example.com/page?category=tech&id=123",
		},
		{
			name:     "Remove fragment",
			input:    "https://example.com/page#section",
			expected: "https://example.com/page",
		},
		{
			name:     "Empty URL",
			input:    "",
			expected: "",
		},
		{
			name:     "Invalid URL",
			input:    "not a valid url",
			expected: "not%20a%20valid%20url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_ParseDate(t *testing.T) {
	config := documentloaders.NormalizationConfig{}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name    string
		input   string
		notZero bool
	}{
		{name: "RFC1123", input: "Mon, 02 Jan 2006 15:04:05 MST", notZero: true},
		{name: "RFC1123Z", input: "Mon, 02 Jan 2006 15:04:05 -0700", notZero: true},
		{name: "RFC822", input: "02 Jan 06 15:04 MST", notZero: true},
		{name: "RFC3339", input: "2006-01-02T15:04:05+07:00", notZero: true},
		{name: "ISO8601", input: "2006-01-02T15:04:05Z", notZero: true},
		{name: "Simple date", input: "2006-01-02", notZero: true},
		{name: "Empty string", input: "", notZero: false},
		{name: "Invalid date", input: "not a date", notZero: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.ParseDate(tt.input)
			if tt.notZero {
				assert.False(t, result.IsZero(), "Expected non-zero time for %s", tt.input)
			} else {
				assert.True(t, result.IsZero(), "Expected zero time for %s", tt.input)
			}
		})
	}
}

func TestRSSNormalizer_NormalizeAuthor(t *testing.T) {
	config := documentloaders.NormalizationConfig{
		NormalizeAuthors: true,
	}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Name with email",
			input:    "John Doe <john@example.com>",
			expected: "John Doe",
		},
		{
			name:     "Name only",
			input:    "Jane Smith",
			expected: "Jane Smith",
		},
		{
			name:     "Quoted name",
			input:    `"John Doe"`,
			expected: "John Doe",
		},
		{
			name:     "Whitespace trimming",
			input:    "  John Doe  ",
			expected: "John Doe",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeAuthor(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_NormalizeCategories(t *testing.T) {
	config := documentloaders.NormalizationConfig{}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Basic normalization",
			input:    []string{"Tech", "NEWS", "programming"},
			expected: []string{"tech", "news", "programming"},
		},
		{
			name:     "Remove duplicates",
			input:    []string{"Tech", "tech", "TECH"},
			expected: []string{"tech"},
		},
		{
			name:     "Remove empty",
			input:    []string{"Tech", "", "News", "  "},
			expected: []string{"tech", "news"},
		},
		{
			name:     "Empty input",
			input:    []string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeCategories(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_NormalizeTitle(t *testing.T) {
	config := documentloaders.NormalizationConfig{
		MinTitleLength: 3,
		FallbackToURL:  true,
	}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name        string
		title       string
		fallbackURL string
		expected    string
	}{
		{
			name:        "Valid title",
			title:       "  Good Title  ",
			fallbackURL: "https://example.com/path",
			expected:    "Good Title",
		},
		{
			name:        "Title too short with URL fallback",
			title:       "AB",
			fallbackURL: "https://example.com/my-article-title",
			expected:    "my article title",
		},
		{
			name:        "Empty title with URL fallback",
			title:       "",
			fallbackURL: "https://example.com/article-title-here",
			expected:    "article title here",
		},
		{
			name:        "Too short with short URL",
			title:       "X",
			fallbackURL: "https://example.com/ab",
			expected:    "X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.NormalizeTitle(tt.title, tt.fallbackURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_ShouldSkipItem(t *testing.T) {
	config := documentloaders.NormalizationConfig{
		MinContentLength: 50,
		MinTitleLength:   3,
	}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name     string
		title    string
		content  string
		expected bool
	}{
		{
			name:     "Valid item",
			title:    "Good Title",
			content:  "This is a sufficiently long piece of content that meets the minimum requirement",
			expected: false,
		},
		{
			name:     "Content too short",
			title:    "Good Title",
			content:  "Too short",
			expected: true,
		},
		{
			name:     "Title too short",
			title:    "AB",
			content:  "This is a sufficiently long piece of content that meets the minimum requirement",
			expected: true,
		},
		{
			name:     "Both too short",
			title:    "X",
			content:  "Short",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.ShouldSkipItem(tt.title, tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSNormalizer_ResolveURL(t *testing.T) {
	config := documentloaders.NormalizationConfig{}
	normalizer := documentloaders.NewRSSNormalizer(config)

	tests := []struct {
		name        string
		baseURL     string
		relativeURL string
		expected    string
	}{
		{
			name:        "Absolute URL",
			baseURL:     "https://example.com",
			relativeURL: "https://other.com/page",
			expected:    "https://other.com/page",
		},
		{
			name:        "Relative path",
			baseURL:     "https://example.com/feed",
			relativeURL: "/article/123",
			expected:    "https://example.com/article/123",
		},
		{
			name:        "Relative with path",
			baseURL:     "https://example.com/feed/",
			relativeURL: "article/123",
			expected:    "https://example.com/feed/article/123",
		},
		{
			name:        "Empty relative URL",
			baseURL:     "https://example.com",
			relativeURL: "",
			expected:    "",
		},
		{
			name:        "Empty base URL",
			baseURL:     "",
			relativeURL: "/article",
			expected:    "/article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizer.ResolveURL(tt.baseURL, tt.relativeURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}
