package html_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/parsers/html"
)

func TestNewHTMLParser(t *testing.T) {
	// Test default options
	parser := html.NewHTMLParser()
	assert.NotNil(t, parser)

	// Test with options
	parser = html.NewHTMLParser(
		html.WithBaseURL("https://example.com"),
		html.WithBoilerplateRemoval(true),
		html.WithMetadataExtraction(true),
		html.WithMarkdownConversion(true),
	)
	assert.NotNil(t, parser)

	// Test disabled features
	parser = html.NewHTMLParser(
		html.WithBoilerplateRemoval(false),
		html.WithMetadataExtraction(false),
		html.WithMarkdownConversion(false),
		html.WithStructurePreservation(false),
	)
	assert.NotNil(t, parser)
}

func TestHTMLParser_BoilerplateRemoval(t *testing.T) {
	input := `
		<html>
			<head><title>Test</title></head>
			<body>
				<nav><ul><li>Home</li><li>About</li></ul></nav>
				<article>
					<h1>Article Title</h1>
					<p>This is the main content.</p>
				</article>
				<footer>Copyright 2024</footer>
				<script>alert('ads');</script>
				<aside class="sidebar">Related Links</aside>
			</body>
		</html>
	`

	parser := html.NewHTMLParser(html.WithBoilerplateRemoval(true))
	chunks, err := parser.Chunk(input, "test.html", nil)

	require.NoError(t, err)
	require.Len(t, chunks, 1)

	content := chunks[0].Content

	// Content should not include nav, footer, script, aside
	assert.NotContains(t, content, "Home")
	assert.NotContains(t, content, "Copyright")
	assert.NotContains(t, content, "alert")
	assert.NotContains(t, content, "Related Links")

	// Content should include article
	assert.Contains(t, content, "Article Title")
	assert.Contains(t, content, "main content")
}

func TestHTMLParser_LinkNormalization(t *testing.T) {
	tests := []struct {
		name             string
		baseURL          string
		html             string
		expectedContains string
	}{
		{
			name:             "relative link",
			baseURL:          "https://example.com",
			html:             `<a href="/article/123">Read More</a>`,
			expectedContains: "[Read More](https://example.com/article/123)",
		},
		{
			name:             "absolute link unchanged",
			baseURL:          "https://example.com",
			html:             `<a href="https://other.com/page">External</a>`,
			expectedContains: "[External](https://other.com/page)",
		},
		{
			name:             "anchor link unchanged",
			baseURL:          "https://example.com",
			html:             `<a href="#section">Jump</a>`,
			expectedContains: "#section",
		},
		{
			name:             "mailto link unchanged",
			baseURL:          "https://example.com",
			html:             `<a href="mailto:test@example.com">Email</a>`,
			expectedContains: "test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := html.NewHTMLParser(
				html.WithBaseURL(tt.baseURL),
				html.WithBoilerplateRemoval(false),
				html.WithMarkdownConversion(true),
			)

			chunks, err := parser.Chunk(tt.html, "test.html", nil)
			require.NoError(t, err)
			require.Len(t, chunks, 1)
			assert.Contains(t, chunks[0].Content, tt.expectedContains)
		})
	}
}

func TestHTMLParser_MetadataExtraction(t *testing.T) {
	input := `
		<html>
			<head>
				<meta property="article:author" content="John Doe">
				<meta property="article:published_time" content="2024-01-15T10:00:00Z">
				<meta property="og:title" content="Article Title">
				<meta name="keywords" content="go, programming, tutorial">
				<meta name="description" content="A great article">
			</head>
			<body>
				<h1>Article Title</h1>
				<p>Content here.</p>
			</body>
		</html>
	`

	parser := html.NewHTMLParser(html.WithMetadataExtraction(true))

	// Test Chunk with metadata
	chunks, err := parser.Chunk(input, "test.html", nil)
	require.NoError(t, err)
	require.Len(t, chunks, 1)

	annotations := chunks[0].Annotations
	assert.Equal(t, "John Doe", annotations["author"])
	assert.Equal(t, "Article Title", annotations["title"])
	assert.Contains(t, annotations["keywords"], "go")
	assert.Contains(t, annotations["keywords"], "programming")
	assert.Contains(t, annotations["published_date"], "2024-01-15")

	// Test ExtractMetadata
	metadata, err := parser.ExtractMetadata(input, "test.html")
	require.NoError(t, err)

	assert.Equal(t, "html", metadata.Language)
	assert.Equal(t, "John Doe", metadata.Properties["author"])
	assert.Equal(t, "Article Title", metadata.Properties["title"])
	assert.Contains(t, metadata.Properties["keywords"], "go")
}

func TestHTMLParser_MarkdownConversion(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "heading h1",
			html:     `<h1>Title</h1>`,
			expected: "# Title",
		},
		{
			name:     "heading h2",
			html:     `<h2>Subtitle</h2>`,
			expected: "## Subtitle",
		},
		{
			name:     "paragraph",
			html:     `<p>This is text.</p>`,
			expected: "This is text.",
		},
		{
			name:     "link",
			html:     `<a href="https://example.com">Link Text</a>`,
			expected: "[Link Text](https://example.com)",
		},
		{
			name:     "bold",
			html:     `<strong>bold text</strong>`,
			expected: "**bold text**",
		},
		{
			name:     "italic",
			html:     `<em>italic text</em>`,
			expected: "*italic text*",
		},
		{
			name:     "unordered list",
			html:     `<ul><li>Item 1</li><li>Item 2</li></ul>`,
			expected: "- Item 1\n- Item 2",
		},
		{
			name:     "ordered list",
			html:     `<ol><li>First</li><li>Second</li></ol>`,
			expected: "1. First\n2. Second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := html.NewHTMLParser(
				html.WithBoilerplateRemoval(false),
				html.WithMarkdownConversion(true),
			)

			chunks, err := parser.Chunk(tt.html, "test.html", nil)
			require.NoError(t, err)
			require.Len(t, chunks, 1)
			assert.Contains(t, chunks[0].Content, tt.expected)
		})
	}
}

func TestHTMLParser_ComplexDocument(t *testing.T) {
	input := `
		<!DOCTYPE html>
		<html lang="en">
		<head>
			<meta charset="UTF-8">
			<meta property="og:title" content="Test Article">
			<meta property="article:author" content="Jane Doe">
			<meta property="article:published_time" content="2024-03-06T12:00:00Z">
			<meta name="keywords" content="test, example, demo">
		</head>
		<body>
			<nav>
				<ul>
					<li><a href="/">Home</a></li>
					<li><a href="/about">About</a></li>
				</ul>
			</nav>

			<article>
				<h1>Test Article</h1>
				<p class="byline">By Jane Doe</p>

				<p>This is the <strong>first paragraph</strong> with a <a href="/link">relative link</a>.</p>

				<h2>Section 1</h2>
				<p>Content for section 1.</p>

				<ul>
					<li>Item 1</li>
					<li>Item 2</li>
				</ul>

				<blockquote>
					<p>A famous quote here.</p>
				</blockquote>

				<pre><code>func main() {
					fmt.Println("Hello")
				}</code></pre>
			</article>

			<footer>
				<p>Copyright 2024</p>
			</footer>
		</body>
		</html>
	`

	parser := html.NewHTMLParser(
		html.WithBaseURL("https://example.com"),
		html.WithBoilerplateRemoval(true),
		html.WithMetadataExtraction(true),
		html.WithMarkdownConversion(true),
	)

	chunks, err := parser.Chunk(input, "test.html", nil)
	require.NoError(t, err)
	require.Len(t, chunks, 1)

	content := chunks[0].Content

	// Verify boilerplate removed
	assert.NotContains(t, content, "Home")
	assert.NotContains(t, content, "About")
	assert.NotContains(t, content, "Copyright")

	// Verify content present
	assert.Contains(t, content, "# Test Article")
	assert.Contains(t, content, "## Section 1")
	assert.Contains(t, content, "**first paragraph**")
	assert.Contains(t, content, "[relative link](https://example.com/link)")
	assert.Contains(t, content, "- Item 1")
	assert.Contains(t, content, "> A famous quote")
	assert.Contains(t, content, "```")

	// Verify metadata
	assert.Equal(t, "Jane Doe", chunks[0].Annotations["author"])
	assert.Equal(t, "Test Article", chunks[0].Annotations["title"])
	assert.Contains(t, chunks[0].Annotations["keywords"], "test")
}

func TestHTMLParser_Name(t *testing.T) {
	parser := html.NewHTMLParser()
	assert.Equal(t, "html", parser.Name())
}

func TestHTMLParser_Extensions(t *testing.T) {
	parser := html.NewHTMLParser()
	extensions := parser.Extensions()
	assert.Contains(t, extensions, ".html")
	assert.Contains(t, extensions, ".htm")
}

func TestHTMLParser_CanHandle(t *testing.T) {
	parser := html.NewHTMLParser()

	tests := []struct {
		path     string
		expected bool
	}{
		{"test.html", true},
		{"test.htm", true},
		{"test.HTML", true},
		{"test.txt", false},
		{"test.md", false},
		{"/path/to/file.html", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := parser.CanHandle(tt.path, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTMLParser_IsGenerated(t *testing.T) {
	parser := html.NewHTMLParser()
	assert.False(t, parser.IsGenerated("<html></html>", "test.html"))
}

func TestHTMLParser_ExtractUsedSymbols(t *testing.T) {
	parser := html.NewHTMLParser()
	assert.Nil(t, parser.ExtractUsedSymbols("<html></html>"))
}
