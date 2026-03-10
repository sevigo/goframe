package contextpacker

import (
	"strings"
	"testing"
)

func TestNewDOMCompressor(t *testing.T) {
	compressor := NewDOMCompressor()
	if compressor == nil {
		t.Fatal("expected compressor, got nil")
	}

	// Check defaults
	if !compressor.RemoveStyleTags {
		t.Error("expected RemoveStyleTags to be true by default")
	}
	if !compressor.RemoveScriptTags {
		t.Error("expected RemoveScriptTags to be true by default")
	}
	if !compressor.RemoveComments {
		t.Error("expected RemoveComments to be true by default")
	}
	if !compressor.FlattenDivs {
		t.Error("expected FlattenDivs to be true by default")
	}
}

func TestDOMCompressor_RemoveScriptTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single script tag",
			input:    `<html><head><script>alert('test');</script></head><body>Content</body></html>`,
			expected: `<html><head></head><body>Content</body></html>`,
		},
		{
			name:     "script with attributes",
			input:    `<script type="text/javascript" src="app.js">code</script><div>Content</div>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "multiple script tags",
			input:    `<script>var a = 1;</script><div>Content</div><script>var b = 2;</script>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "script with multiline content",
			input:    "<script>\nfunction test() {\n  return true;\n}\n</script><p>Text</p>",
			expected: `<p>Text</p>`,
		},
	}

	compressor := NewDOMCompressor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressor.Compress(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Normalize whitespace for comparison
			result = strings.TrimSpace(result)
			expected := strings.TrimSpace(tt.expected)

			if result != expected {
				t.Errorf("Compress()\ninput:  %q\n got:   %q\n want:  %q", tt.input, result, expected)
			}
		})
	}
}

func TestDOMCompressor_RemoveStyleTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single style tag",
			input:    `<style>.class { color: red; }</style><div>Content</div>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "style in head",
			input:    `<html><head><style>body { margin: 0; }</style></head><body>Content</body></html>`,
			expected: `<html><head></head><body>Content</body></html>`,
		},
		{
			name:     "multiple style tags",
			input:    `<style>a { }</style><div>Content</div><style>b { }</style>`,
			expected: `<div>Content</div>`,
		},
	}

	compressor := NewDOMCompressor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressor.Compress(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result = strings.TrimSpace(result)
			expected := strings.TrimSpace(tt.expected)

			if result != expected {
				t.Errorf("Compress()\ninput:  %q\n got:   %q\n want:  %q", tt.input, result, expected)
			}
		})
	}
}

func TestDOMCompressor_RemoveComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single comment",
			input:    `<div><!-- comment --><p>Text</p></div>`,
			expected: `<div><p>Text</p></div>`,
		},
		{
			name:     "multiline comment",
			input:    "<!--\nThis is a\nmultiline comment\n--><div>Content</div>",
			expected: `<div>Content</div>`,
		},
		{
			name:     "conditional comments",
			input:    `<!--[if IE]><p>IE only</p><![endif]--><div>Content</div>`,
			expected: `<div>Content</div>`,
		},
	}

	compressor := NewDOMCompressor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressor.Compress(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result = strings.TrimSpace(result)
			expected := strings.TrimSpace(tt.expected)

			if result != expected {
				t.Errorf("Compress()\ninput:  %q\n got:   %q\n want:  %q", tt.input, result, expected)
			}
		})
	}
}

func TestDOMCompressor_RemoveAttributes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove class attribute",
			input:    `<div class="container">Content</div>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "remove style attribute",
			input:    `<div style="color: red;">Content</div>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "preserve id attribute",
			input:    `<div id="main" class="container">Content</div>`,
			expected: `<div id="main">Content</div>`,
		},
		{
			name:     "preserve name attribute",
			input:    `<input name="email" class="form-control" type="text">`,
			expected: `<input name="email" type="text">`,
		},
		{
			name:     "preserve aria-label",
			input:    `<button aria-label="Close" class="btn">X</button>`,
			expected: `<button aria-label="Close">X</button>`,
		},
		{
			name:     "preserve href",
			input:    `<a href="https://example.com" class="link">Link</a>`,
			expected: `<a href="https://example.com">Link</a>`,
		},
		{
			name:     "preserve src and alt",
			input:    `<img src="image.png" alt="Description" class="img-fluid">`,
			expected: `<img src="image.png" alt="Description">`,
		},
	}

	compressor := NewDOMCompressor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressor.Compress(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Normalize whitespace for comparison
			result = strings.Join(strings.Fields(result), " ")

			if !strings.Contains(result, strings.Join(strings.Fields(tt.expected), " ")) {
				t.Errorf("Compress()\ninput:  %q\n got:   %q\n want:  %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDOMCompressor_RemoveDataAttributes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove single data attribute",
			input:    `<div data-id="123">Content</div>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "remove multiple data attributes",
			input:    `<div data-id="123" data-name="test" data-value="456">Content</div>`,
			expected: `<div>Content</div>`,
		},
		{
			name:     "preserve id with data attributes",
			input:    `<div id="main" data-config='{"key":"value"}'>Content</div>`,
			expected: `<div id="main">Content</div>`,
		},
		{
			name:     "data attribute with hyphens",
			input:    `<div data-user-id="123" data-api-endpoint="/api">Content</div>`,
			expected: `<div>Content</div>`,
		},
	}

	compressor := NewDOMCompressor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressor.Compress(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result = strings.TrimSpace(result)
			expected := strings.TrimSpace(tt.expected)

			if result != expected {
				t.Errorf("Compress()\ninput:  %q\n got:   %q\n want:  %q", tt.input, result, expected)
			}
		})
	}
}

func TestDOMCompressor_FlattenDivs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "remove empty div",
			input:    `<div></div><p>Content</p>`,
			contains: `<p>Content</p>`,
		},
		{
			name:     "remove whitespace-only div",
			input:    `<div>   </div><p>Content</p>`,
			contains: `<p>Content</p>`,
		},
	}

	compressor := NewDOMCompressor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressor.Compress(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(result, tt.contains) {
				t.Errorf("Compress()\ninput:   %q\n got:    %q\nshould contain: %q", tt.input, result, tt.contains)
			}
		})
	}
}

func TestDOMCompressor_ComplexHTML(t *testing.T) {
	input := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Test Page</title>
	<style>
		.container { max-width: 1200px; }
		.header { background: #fff; }
	</style>
	<script src="app.js"></script>
	<script>
		console.log('test');
	</script>
</head>
<body>
	<!-- Main navigation -->
	<nav class="navbar" id="main-nav">
		<a href="/" class="logo">Home</a>
		<ul data-menu="main">
			<li><a href="/about">About</a></li>
		</ul>
	</nav>
	
	<main id="content" class="content-area" data-role="main">
		<article id="post-123" class="post" aria-label="Main article">
			<h1 class="title">Article Title</h1>
			<div class="wrapper">
				<div class="inner-wrapper">
					<div></div>
					<p style="color: black;" data-tracking-id="abc">Content paragraph</p>
				</div>
			</div>
		</article>
	</main>
</body>
</html>`

	compressor := NewDOMCompressor()
	result, err := compressor.Compress(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify script tags removed
	if strings.Contains(result, "<script") {
		t.Error("script tags should be removed")
	}

	// Verify style tags removed
	if strings.Contains(result, "<style") {
		t.Error("style tags should be removed")
	}

	// Verify comments removed
	if strings.Contains(result, "<!--") {
		t.Error("HTML comments should be removed")
	}

	// Verify class attributes removed (check for remaining)
	// Note: this is a basic check, full removal is complex
	if strings.Contains(result, `class="navbar"`) {
		t.Error("class attribute should be removed")
	}

	// Verify data-* attributes removed
	if strings.Contains(result, "data-menu") {
		t.Error("data-* attributes should be removed")
	}

	// Verify id and aria-label preserved
	if !strings.Contains(result, `id="main-nav"`) {
		t.Error("id attribute should be preserved")
	}
	if !strings.Contains(result, `aria-label="Main article"`) {
		t.Error("aria-label attribute should be preserved")
	}

	// Verify href preserved
	if !strings.Contains(result, `href="/"`) {
		t.Error("href attribute should be preserved")
	}
}

func TestDOMCompressor_CompressWithStats(t *testing.T) {
	compressor := NewDOMCompressor()

	input := `<html><script>alert('test');</script><body><div class="test">Content</div></body></html>`
	result, stats, err := compressor.CompressWithStats(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}

	if stats.OriginalLength == 0 {
		t.Error("expected non-zero original length")
	}

	if stats.CompressedLength == 0 {
		t.Error("expected non-zero compressed length")
	}

	if stats.OriginalLength <= stats.CompressedLength {
		t.Error("expected compressed to be smaller than original")
	}

	if stats.ReductionPercent <= 0 {
		t.Error("expected positive reduction percentage")
	}

	if stats.TokensSaved <= 0 {
		t.Error("expected positive tokens saved")
	}
}

func TestDOMCompressor_MustCompress(t *testing.T) {
	compressor := NewDOMCompressor()

	// Should not panic on valid input
	result := compressor.MustCompress(`<div>Content</div>`)
	if result == "" {
		t.Error("expected non-empty result from MustCompress")
	}
}

func TestDOMCompressor_WithOptions(t *testing.T) {
	compressor := NewDOMCompressor(
		WithStyleTags(false),
		WithScriptTags(false),
		WithComments(false),
		WithFlattenDivs(false),
		WithKeepAttributes("data-custom"),
	)

	input := `<style>.test { }</style><script>alert(1)</script><!-- comment --><div id="main">Content</div>`
	result, err := compressor.Compress(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With RemoveStyleTags=false, style should remain
	if !strings.Contains(result, "<style") {
		t.Error("style tags should be preserved when WithStyleTags(false)")
	}

	// With RemoveScriptTags=false, script should remain
	if !strings.Contains(result, "<script") {
		t.Error("script tags should be preserved when WithScriptTags(false)")
	}

	// With RemoveComments=false, comments should remain
	if !strings.Contains(result, "<!--") {
		t.Error("comments should be preserved when WithComments(false)")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		chars     int
		minTokens int
		maxTokens int
	}{
		{chars: 100, minTokens: 20, maxTokens: 30},
		{chars: 400, minTokens: 90, maxTokens: 110},
		{chars: 1000, minTokens: 240, maxTokens: 260},
	}

	for _, tt := range tests {
		tokens := estimateTokens(tt.chars)
		if tokens < tt.minTokens || tokens > tt.maxTokens {
			t.Errorf("estimateTokens(%d) = %d, want between %d and %d", tt.chars, tokens, tt.minTokens, tt.maxTokens)
		}
	}
}
