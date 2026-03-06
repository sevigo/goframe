# TODO: HTML Parser for RSS Content Enhancement

## Context & Motivation

### Current Problem
The RSS loader currently strips HTML to plain text, which loses:
- **Structure**: Headers become flat text (no semantic meaning)
- **Links**: Relative URLs can't be followed by agents
- **Metadata**: Author and date are lost in HTML noise
- **Quality**: Nav, footer, scripts pollute embeddings

### Proposed Solution
Create a modular HTML parser plugin that transforms HTML content from RSS feeds (and future sources) into clean Markdown for optimal LLM consumption.

### Why This Approach (Staff-Level Thinking)

**Modularity Over Monolith**: Separate HTML parsing logic from RSS loading
```
RSS Loader → "I have HTML content"
HTML Parser → "I'll clean and structure it"
Vector DB → "Perfect markdown with metadata"
```

**LLM-Optimized Pipeline**: Not just "download and dump text" but intelligent content transformation

**Reusability**: Same HTML parser works for:
- RSS feeds (current)
- Confluence pages (future)
- Web scraping (future)
- Email HTML (future)

## Architecture

### File Structure
```
goframe/
├── parsers/
│   └── html/
│       ├── parser.go           # Main HTML parser (implements ParserPlugin)
│       ├── parser_test.go      # Unit tests
│       ├── boilerplate.go      # Boilerplate removal logic
│       ├── links.go            # Link normalization
│       ├── metadata.go         # Metadata extraction
│       └── markdown.go         # HTML to Markdown conversion
├── documentloaders/
│   └── rss.go                  # Update to use HTML parser
└── examples/
    └── rss-news-demo/
        └── main.go             # Demo showing RSS + HTML pipeline
```

### Data Flow
```
1. RSS Feed (HTML content)
   ↓
2. RSSLoader.Load()
   ↓
3. HTMLParser.Chunk() [NEW]
   - Remove boilerplate (nav, footer, script)
   - Normalize links (relative → absolute)
   - Extract metadata (author, date)
   - Convert to Markdown
   ↓
4. schema.Document
   - PageContent: Clean markdown
   - Metadata: Author, PublishedDate, etc.
   ↓
5. VectorStore (better RAG quality)
```

## Implementation Steps

### Step 1: Create HTML Parser Infrastructure

**File**: `parsers/html/parser.go`

```go
package html

import (
    "io/fs"
    "strings"
    "time"

    "github.com/PuerkitoBio/goquery"
    "github.com/sevigo/goframe/schema"
)

type HTMLParser struct {
    baseURL            string
    removeBoilerplate  bool
    extractMetadata    bool
    convertToMarkdown  bool
    preserveStructure  bool
}

type HTMLMetadata struct {
    Author        string    `json:"author"`
    PublishedDate time.Time `json:"published_date"`
    Title         string    `json:"title"`
    Description   string    `json:"description"`
    CanonicalURL  string    `json:"canonical_url"`
    Keywords      []string  `json:"keywords"`
}

func NewHTMLParser(opts ...Option) *HTMLParser {
    p := &HTMLParser{
        removeBoilerplate:  true,
        extractMetadata:    true,
        convertToMarkdown:  true,
        preserveStructure:  true,
    }
    for _, opt := range opts {
        opt(p)
    }
    return p
}

func (p *HTMLParser) Name() string {
    return "html"
}

func (p *HTMLParser) Extensions() []string {
    return []string{".html", ".htm"}
}

func (p *HTMLParser) CanHandle(path string, info fs.FileInfo) bool {
    ext := strings.ToLower(path)
    return strings.HasSuffix(ext, ".html") || strings.HasSuffix(ext, ".htm")
}

func (p *HTMLParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
    // Implementation in next steps
}

func (p *HTMLParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
    // Implementation in next steps
}

type Option func(*HTMLParser)

func WithBaseURL(baseURL string) Option {
    return func(p *HTMLParser) {
        p.baseURL = baseURL
    }
}

func WithBoilerplateRemoval(remove bool) Option {
    return func(p *HTMLParser) {
        p.removeBoilerplate = remove
    }
}

func WithMetadataExtraction(extract bool) Option {
    return func(p *HTMLParser) {
        p.extractMetadata = extract
    }
}

func WithMarkdownConversion(convert bool) Option {
    return func(p *HTMLParser) {
        p.convertToMarkdown = convert
    }
}
```

### Step 2: Implement Boilerplate Removal

**File**: `parsers/html/boilerplate.go`

```go
package html

import "github.com/PuerkitoBio/goquery"

// Boilerplate selectors to remove (based on research)
var boilerplateSelectors = []string{
    // Navigation elements
    "nav", "header", "footer",
    
    // Scripts and styles
    "script", "style", "noscript",
    
    // Common noise elements
    "aside", ".sidebar", ".advertisement", ".ad", ".ads",
    ".navigation", ".menu", ".breadcrumb",
    ".social-share", ".share-buttons",
    ".comments", "#comments", ".comment-section",
    ".related-posts", ".recommended",
    
    // Tracking pixels
    "img[src*='pixel']", "img[src*='track']",
}

func (p *HTMLParser) removeBoilerplate(doc *goquery.Document) {
    if !p.removeBoilerplate {
        return
    }
    
    for _, selector := range boilerplateSelectors {
        doc.Find(selector).Remove()
    }
    
    // Remove elements with common noise classes/IDs
    doc.Find("[class*='sidebar'], [id*='sidebar']").Remove()
    doc.Find("[class*='footer'], [id*='footer']").Remove()
    doc.Find("[class*='nav-'], [id*='nav-']").Remove()
}
```

### Step 3: Implement Link Normalization

**File**: `parsers/html/links.go`

```go
package html

import (
    "net/url"
    "strings"
    
    "github.com/PuerkitoBio/goquery"
)

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
        if !exists {
            return
        }
        
        // Skip anchors, javascript, and already absolute URLs
        if strings.HasPrefix(href, "#") ||
           strings.HasPrefix(href, "javascript:") ||
           strings.HasPrefix(href, "mailto:") ||
           strings.Contains(href, "://") {
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
        if !exists {
            return
        }
        
        if strings.Contains(src, "://") {
            return
        }
        
        if resolvedURL := resolveRelativeURL(base, src); resolvedURL != "" {
            s.SetAttr("src", resolvedURL)
        }
    })
}

func resolveRelativeURL(base *url.URL, relative string) string {
    rel, err := url.Parse(relative)
    if err != nil {
        return ""
    }
    
    resolved := base.ResolveReference(rel)
    return resolved.String()
}
```

### Step 4: Implement Metadata Extraction

**File**: `parsers/html/metadata.go`

```go
package html

import (
    "strings"
    "time"
    
    "github.com/PuerkitoBio/goquery"
)

func (p *HTMLParser) extractMetadata(doc *goquery.Document) HTMLMetadata {
    if !p.extractMetadata {
        return HTMLMetadata{}
    }
    
    metadata := HTMLMetadata{}
    
    // Extract author
    metadata.Author = p.extractAuthor(doc)
    
    // Extract published date
    metadata.PublishedDate = p.extractPublishedDate(doc)
    
    // Extract title
    metadata.Title = p.extractTitle(doc)
    
    // Extract description
    metadata.Description = p.extractDescription(doc)
    
    // Extract canonical URL
    metadata.CanonicalURL = p.extractCanonicalURL(doc)
    
    // Extract keywords
    metadata.Keywords = p.extractKeywords(doc)
    
    return metadata
}

func (p *HTMLParser) extractAuthor(doc *goquery.Document) string {
    // Try multiple sources in order of reliability
    
    // 1. Schema.org author
    if author := doc.Find(`meta[itemprop="author"]`).AttrOr("content", ""); author != "" {
        return author
    }
    
    // 2. Open Graph article:author
    if author := doc.Find(`meta[property="article:author"]`).AttrOr("content", ""); author != "" {
        return author
    }
    
    // 3. Dublin Core
    if author := doc.Find(`meta[name="dc.creator"]`).AttrOr("content", ""); author != "" {
        return author
    }
    
    // 4. Standard meta author
    if author := doc.Find(`meta[name="author"]`).AttrOr("content", ""); author != "" {
        return author
    }
    
    // 5. Byline in content (common patterns)
    author := ""
    doc.Find(".byline, .author, [class*='author']").Each(func(i int, s *goquery.Selection) {
        if text := strings.TrimSpace(s.Text()); text != "" && len(text) < 100 {
            author = text
            return
        }
    })
    
    return author
}

func (p *HTMLParser) extractPublishedDate(doc *goquery.Document) time.Time {
    dateFormats := []string{
        time.RFC3339,
        "2006-01-02T15:04:05Z07:00",
        "2006-01-02T15:04:05Z",
        "2006-01-02 15:04:05",
        "2006-01-02",
    }
    
    // Try different metadata sources
    
    // 1. Schema.org datePublished
    if dateStr := doc.Find(`meta[itemprop="datePublished"]`).AttrOr("content", ""); dateStr != "" {
        if t := parseDate(dateStr, dateFormats); !t.IsZero() {
            return t
        }
    }
    
    // 2. Open Graph article:published_time
    if dateStr := doc.Find(`meta[property="article:published_time"]`).AttrOr("content", ""); dateStr != "" {
        if t := parseDate(dateStr, dateFormats); !t.IsZero() {
            return t
        }
    }
    
    // 3. Dublin Core
    if dateStr := doc.Find(`meta[name="dc.date"]`).AttrOr("content", ""); dateStr != "" {
        if t := parseDate(dateStr, dateFormats); !t.IsZero() {
            return t
        }
    }
    
    // 4. Standard meta
    if dateStr := doc.Find(`meta[name="pubdate"]`).AttrOr("content", ""); dateStr != "" {
        if t := parseDate(dateStr, dateFormats); !t.IsZero() {
            return t
        }
    }
    
    return time.Time{}
}

func (p *HTMLParser) extractTitle(doc *goquery.Document) string {
    // 1. Open Graph title
    if title := doc.Find(`meta[property="og:title"]`).AttrOr("content", ""); title != "" {
        return title
    }
    
    // 2. Schema.org headline
    if title := doc.Find(`meta[itemprop="headline"]`).AttrOr("content", ""); title != "" {
        return title
    }
    
    // 3. HTML title tag
    if title := doc.Find("title").Text(); title != "" {
        return strings.TrimSpace(title)
    }
    
    return ""
}

func (p *HTMLParser) extractDescription(doc *goquery.Document) string {
    // 1. Open Graph description
    if desc := doc.Find(`meta[property="og:description"]`).AttrOr("content", ""); desc != "" {
        return desc
    }
    
    // 2. Meta description
    if desc := doc.Find(`meta[name="description"]`).AttrOr("content", ""); desc != "" {
        return desc
    }
    
    return ""
}

func (p *HTMLParser) extractCanonicalURL(doc *goquery.Document) string {
    return doc.Find(`link[rel="canonical"]`).AttrOr("href", "")
}

func (p *HTMLParser) extractKeywords(doc *goquery.Document) []string {
    keywords := []string{}
    
    // 1. Meta keywords
    if kw := doc.Find(`meta[name="keywords"]`).AttrOr("content", ""); kw != "" {
        for _, k := range strings.Split(kw, ",") {
            if trimmed := strings.TrimSpace(k); trimmed != "" {
                keywords = append(keywords, trimmed)
            }
        }
    }
    
    // 2. Schema.org keywords
    if kw := doc.Find(`meta[itemprop="keywords"]`).AttrOr("content", ""); kw != "" {
        for _, k := range strings.Split(kw, ",") {
            if trimmed := strings.TrimSpace(k); trimmed != "" {
                keywords = append(keywords, trimmed)
            }
        }
    }
    
    return keywords
}

func parseDate(dateStr string, formats []string) time.Time {
    for _, format := range formats {
        if t, err := time.Parse(format, dateStr); err == nil {
            return t
        }
    }
    return time.Time{}
}
```

### Step 5: Implement HTML to Markdown Conversion

**File**: `parsers/html/markdown.go`

```go
package html

import (
    "fmt"
    "strings"
    
    "github.com/PuerkitoBio/goquery"
)

func (p *HTMLParser) toMarkdown(doc *goquery.Document) string {
    if !p.convertToMarkdown {
        // Return cleaned HTML
        html, _ := doc.Html()
        return html
    }
    
    var markdown strings.Builder
    
    // Find the main content area
    content := doc.Find("article, main, .content, #content, body").First()
    if content.Length() == 0 {
        content = doc.Find("body")
    }
    
    // Convert each node
    content.Contents().Each(func(i int, s *goquery.Selection) {
        markdown.WriteString(p.nodeToMarkdown(s, 0))
    })
    
    return strings.TrimSpace(markdown.String())
}

func (p *HTMLParser) nodeToMarkdown(s *goquery.Selection, depth int) string {
    if s.Nodes == nil || len(s.Nodes) == 0 {
        return ""
    }
    
    node := s.Nodes[0]
    
    // Handle text nodes
    if node.Type == goquery.TextNode {
        return strings.TrimSpace(node.Data)
    }
    
    // Handle element nodes
    tagName := strings.ToLower(node.Data)
    
    switch tagName {
    case "h1":
        return p.headingToMarkdown(s, 1, depth)
    case "h2":
        return p.headingToMarkdown(s, 2, depth)
    case "h3":
        return p.headingToMarkdown(s, 3, depth)
    case "h4":
        return p.headingToMarkdown(s, 4, depth)
    case "h5":
        return p.headingToMarkdown(s, 5, depth)
    case "h6":
        return p.headingToMarkdown(s, 6, depth)
    case "p":
        return p.paragraphToMarkdown(s, depth)
    case "ul", "ol":
        return p.listToMarkdown(s, tagName, depth)
    case "li":
        return p.listItemToMarkdown(s, depth)
    case "a":
        return p.linkToMarkdown(s)
    case "img":
        return p.imageToMarkdown(s)
    case "code":
        return p.codeToMarkdown(s)
    case "pre":
        return p.codeBlockToMarkdown(s)
    case "blockquote":
        return p.blockquoteToMarkdown(s, depth)
    case "strong", "b":
        return p.boldToMarkdown(s)
    case "em", "i":
        return p.italicToMarkdown(s)
    case "br":
        return "\n"
    case "hr":
        return "\n---\n\n"
    case "table":
        return p.tableToMarkdown(s, depth)
    default:
        // Generic element - process children
        var result strings.Builder
        s.Contents().Each(func(i int, child *goquery.Selection) {
            result.WriteString(p.nodeToMarkdown(child, depth))
        })
        return result.String()
    }
}

func (p *HTMLParser) headingToMarkdown(s *goquery.Selection, level int, depth int) string {
    prefix := strings.Repeat("#", level) + " "
    text := strings.TrimSpace(s.Text())
    return fmt.Sprintf("\n%s%s\n\n", prefix, text)
}

func (p *HTMLParser) paragraphToMarkdown(s *goquery.Selection, depth int) string {
    var result strings.Builder
    s.Contents().Each(func(i int, child *goquery.Selection) {
        result.WriteString(p.nodeToMarkdown(child, depth))
    })
    return fmt.Sprintf("\n%s\n\n", strings.TrimSpace(result.String()))
}

func (p *HTMLParser) listToMarkdown(s *goquery.Selection, tag string, depth int) string {
    var result strings.Builder
    indent := strings.Repeat("  ", depth)
    
    s.Find("> li").Each(func(i int, li *goquery.Selection) {
        text := strings.TrimSpace(li.Text())
        if tag == "ul" {
            result.WriteString(fmt.Sprintf("%s- %s\n", indent, text))
        } else {
            result.WriteString(fmt.Sprintf("%s%d. %s\n", indent, i+1, text))
        }
    })
    
    return result.String() + "\n"
}

func (p *HTMLParser) listItemToMarkdown(s *goquery.Selection, depth int) string {
    var result strings.Builder
    s.Contents().Each(func(i int, child *goquery.Selection) {
        result.WriteString(p.nodeToMarkdown(child, depth))
    })
    return result.String()
}

func (p *HTMLParser) linkToMarkdown(s *goquery.Selection) string {
    text := strings.TrimSpace(s.Text())
    href, _ := s.Attr("href")
    
    if href == "" {
        return text
    }
    
    return fmt.Sprintf("[%s](%s)", text, href)
}

func (p *HTMLParser) imageToMarkdown(s *goquery.Selection) string {
    alt, _ := s.Attr("alt")
    src, _ := s.Attr("src")
    
    return fmt.Sprintf("![%s](%s)", alt, src)
}

func (p *HTMLParser) codeToMarkdown(s *goquery.Selection) string {
    text := s.Text()
    return fmt.Sprintf("`%s`", text)
}

func (p *HTMLParser) codeBlockToMarkdown(s *goquery.Selection) string {
    text := s.Find("code").Text()
    if text == "" {
        text = s.Text()
    }
    return fmt.Sprintf("\n```\n%s\n```\n\n", text)
}

func (p *HTMLParser) blockquoteToMarkdown(s *goquery.Selection, depth int) string {
    var result strings.Builder
    lines := strings.Split(strings.TrimSpace(s.Text()), "\n")
    
    for _, line := range lines {
        result.WriteString(fmt.Sprintf("> %s\n", line))
    }
    
    return result.String() + "\n"
}

func (p *HTMLParser) boldToMarkdown(s *goquery.Selection) string {
    text := strings.TrimSpace(s.Text())
    return fmt.Sprintf("**%s**", text)
}

func (p *HTMLParser) italicToMarkdown(s *goquery.Selection) string {
    text := strings.TrimSpace(s.Text())
    return fmt.Sprintf("*%s*", text)
}

func (p *HTMLParser) tableToMarkdown(s *goquery.Selection, depth int) string {
    var result strings.Builder
    
    // Header
    s.Find("thead tr th").Each(func(i int, th *goquery.Selection) {
        result.WriteString(fmt.Sprintf("| %s ", strings.TrimSpace(th.Text())))
    })
    result.WriteString("|\n")
    
    // Separator
    s.Find("thead tr th").Each(func(i int, th *goquery.Selection) {
        result.WriteString("| --- ")
    })
    result.WriteString("|\n")
    
    // Body
    s.Find("tbody tr").Each(func(i int, tr *goquery.Selection) {
        tr.Find("td").Each(func(j int, td *goquery.Selection) {
            result.WriteString(fmt.Sprintf("| %s ", strings.TrimSpace(td.Text())))
        })
        result.WriteString("|\n")
    })
    
    return result.String() + "\n"
}
```

### Step 6: Implement Main Parser Methods

**File**: `parsers/html/parser.go` (continued)

```go
func (p *HTMLParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
    // 1. Parse HTML
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
    if err != nil {
        return nil, fmt.Errorf("failed to parse HTML: %w", err)
    }
    
    // 2. Remove boilerplate
    p.removeBoilerplate(doc)
    
    // 3. Normalize links
    p.normalizeLinks(doc)
    
    // 4. Extract metadata
    metadata := p.extractMetadata(doc)
    
    // 5. Convert to Markdown
    markdown := p.toMarkdown(doc)
    
    // 6. Create chunk
    chunk := schema.CodeChunk{
        Content:      markdown,
        LineStart:    1,
        LineEnd:      countLines(markdown),
        Type:         "html_article",
        Identifier:   path,
        IsDefinition: false,
    }
    
    // 7. Add metadata to annotations
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
    
    if len(metadata.Keywords) > 0 {
        chunk.Annotations["keywords"] = strings.Join(metadata.Keywords, ", ")
    }
    
    return []schema.CodeChunk{chunk}, nil
}

func (p *HTMLParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
    if err != nil {
        return schema.FileMetadata{}, fmt.Errorf("failed to parse HTML: %w", err)
    }
    
    htmlMeta := p.extractMetadata(doc)
    
    return schema.FileMetadata{
        FilePath:  path,
        Language:  "html",
        Properties: map[string]string{
            "author":         htmlMeta.Author,
            "title":          htmlMeta.Title,
            "description":    htmlMeta.Description,
            "canonical_url":  htmlMeta.CanonicalURL,
            "keywords":       strings.Join(htmlMeta.Keywords, ", "),
        },
    }, nil
}

func countLines(text string) int {
    return strings.Count(text, "\n") + 1
}
```

### Step 7: Integration with RSS Loader

**File**: `documentloaders/rss.go` (modifications)

```go
// Add to rssLoaderOptions
type rssLoaderOptions struct {
    // ... existing fields
    HTMLParser      parsers.ParserPlugin
}

// Add option function
func WithHTMLParser(htmlParser parsers.ParserPlugin) RSSLoaderOption {
    return func(opts *rssLoaderOptions) {
        opts.HTMLParser = htmlParser
    }
}

// Modify createDocument method
func (r *RSSLoader) createDocument(item *gofeed.Item, feedData RSSFeedData) *schema.Document {
    // ... existing code ...
    
    // Use HTML parser if available
    var content string
    if r.options.HTMLParser != nil && item.Content != "" {
        // Parse HTML content
        chunks, err := r.options.HTMLParser.Chunk(item.Content, item.Link, nil)
        if err == nil && len(chunks) > 0 {
            content = chunks[0].Content
            
            // Add annotations to metadata
            for k, v := range chunks[0].Annotations {
                metadata[k] = v
            }
        } else {
            // Fallback to normalizer
            content = r.normalizer.NormalizeContent(item.Content)
        }
    } else if item.Content != "" {
        content = r.normalizer.NormalizeContent(item.Content)
    } else {
        content = r.normalizer.NormalizeContent(item.Description)
    }
    
    // ... rest of method
}
```

### Step 8: Create Example

**File**: `examples/rss-news-demo/main.go`

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    
    "github.com/sevigo/goframe/documentloaders"
    "github.com/sevigo/goframe/parsers"
    "github.com/sevigo/goframe/parsers/html"
)

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
    ctx := context.Background()
    
    // 1. Create HTML parser with all features enabled
    htmlParser := html.NewHTMLParser(
        html.WithBaseURL("https://example.com"),
        html.WithBoilerplateRemoval(true),
        html.WithMetadataExtraction(true),
        html.WithMarkdownConversion(true),
    )
    
    // 2. Register HTML parser
    registry := parsers.NewRegistry(logger)
    if err := registry.RegisterParser(htmlParser); err != nil {
        logger.Error("Failed to register HTML parser", "error", err)
        return
    }
    
    // 3. Create RSS loader with HTML parser
    feedURLs := []string{
        "https://feeds.bbci.co.uk/news/technology/rss.xml",
        "https://news.ycombinator.com/rss",
    }
    
    loader, err := documentloaders.NewRSS(
        feedURLs,
        registry,
        documentloaders.WithHTMLParser(htmlParser),
        documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
            StripHTML:        false, // Let HTML parser handle this
            RemoveTracking:   true,
            MaxContentLength: 10000,
        }),
    )
    if err != nil {
        logger.Error("Failed to create RSS loader", "error", err)
        return
    }
    
    // 4. Load documents
    docs, err := loader.Load(ctx)
    if err != nil {
        logger.Error("Failed to load RSS feeds", "error", err)
        return
    }
    
    // 5. Display results
    fmt.Printf("\n=== Loaded %d documents ===\n\n", len(docs))
    
    for i, doc := range docs {
        if i >= 5 {
            break
        }
        
        fmt.Printf("--- Document %d ---\n", i+1)
        fmt.Printf("Title: %s\n", doc.Metadata["title"])
        fmt.Printf("Author: %s\n", doc.Metadata["author"])
        fmt.Printf("Published: %s\n", doc.Metadata["published_date"])
        fmt.Printf("Keywords: %s\n", doc.Metadata["keywords"])
        fmt.Printf("\nContent (Markdown):\n%s\n", truncate(doc.PageContent, 200))
        fmt.Printf("\nLink: %s\n\n", doc.Metadata["link"])
    }
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}
```

### Step 9: Write Comprehensive Tests

**File**: `parsers/html/parser_test.go`

```go
package html_test

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "github.com/sevigo/goframe/parsers/html"
)

func TestHTMLParser_BoilerplateRemoval(t *testing.T) {
    input := `
        <html>
            <nav><ul><li>Home</li></ul></nav>
            <article>
                <h1>Article Title</h1>
                <p>This is the content.</p>
            </article>
            <footer>Copyright 2024</footer>
            <script>alert('ads');</script>
        </html>
    `
    
    parser := html.NewHTMLParser(html.WithBoilerplateRemoval(true))
    
    // After parsing, nav, footer, and script should be removed
    // Content should only contain article
}

func TestHTMLParser_LinkNormalization(t *testing.T) {
    tests := []struct {
        name      string
        baseURL   string
        html      string
        expectedContains string
    }{
        {
            name:     "relative link",
            baseURL:  "https://example.com",
            html:     `<a href="/article/123">Read More</a>`,
            expectedContains: "[Read More](https://example.com/article/123)",
        },
        {
            name:     "absolute link unchanged",
            baseURL:  "https://example.com",
            html:     `<a href="https://other.com/page">External</a>`,
            expectedContains: "[External](https://other.com/page)",
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
            </head>
            <body>
                <h1>Article Title</h1>
                <p>Content here.</p>
            </body>
        </html>
    `
    
    parser := html.NewHTMLParser(html.WithMetadataExtraction(true))
    
    metadata := parser.ExtractMetadata(input, "test.html")
    
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
            name:     "heading",
            html:     `<h1>Title</h1>`,
            expected: "# Title",
        },
        {
            name:     "paragraph",
            html:     `<p>This is text.</p>`,
            expected: "This is text.",
        },
        {
            name:     "link",
            html:     `<a href="https://example.com">Link</a>`,
            expected: "[Link](https://example.com)",
        },
        {
            name:     "bold",
            html:     `<strong>bold text</strong>`,
            expected: "**bold text**",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            parser := html.NewHTMLParser(html.WithMarkdownConversion(true))
            
            chunks, err := parser.Chunk(tt.html, "test.html", nil)
            require.NoError(t, err)
            assert.Contains(t, chunks[0].Content, tt.expected)
        })
    }
}
```

## Testing Strategy

### Unit Tests
- [ ] Boilerplate removal (nav, footer, scripts)
- [ ] Link normalization (relative → absolute)
- [ ] Metadata extraction (author, date, title)
- [ ] HTML to Markdown conversion (each element type)
- [ ] Edge cases (empty HTML, malformed HTML, missing metadata)

### Integration Tests
- [ ] RSS feed with full HTML content
- [ ] RSS feed with relative links
- [ ] RSS feed with various metadata formats
- [ ] End-to-end pipeline: RSS → HTML → Markdown → Document

### Test Data
Create test fixtures in `parsers/html/testdata/`:
- `article_full.html` - Complete article with all metadata
- `article_minimal.html` - Minimal article
- `article_boilerplate.html` - Article with nav/footer
- `article_relative_links.html` - Article with relative links

## Success Criteria

✅ **Boilerplate Removed**: nav, footer, scripts stripped from 95%+ of test cases  
✅ **Links Resolved**: All relative URLs converted to absolute  
✅ **Metadata Extracted**: Author, date extracted from Open Graph, Schema.org, Dublin Core  
✅ **Markdown Quality**: Clean, well-structured markdown output  
✅ **Tests Pass**: >90% coverage, all edge cases handled  
✅ **Integration Works**: RSS loader produces cleaner documents with HTML parser  

## Estimated Time

- **Phase 1-3** (Infrastructure + Boilerplate + Links): 4-5 hours
- **Phase 4-5** (Metadata + Markdown): 4-5 hours  
- **Phase 6-7** (Integration + Example): 2-3 hours
- **Phase 8** (Testing): 3-4 hours

**Total**: ~13-17 hours

## Dependencies

Already in go.mod:
- ✅ `github.com/PuerkitoBio/goquery` - HTML parsing
- ✅ `github.com/microcosm-cc/bluemonday` - HTML sanitization

Need to add:
- `github.com/gomarkdown/markdown` - Optional (if we want advanced markdown features)

## Notes for Implementation

1. **Start with boilerplate removal** - Most impactful, easiest to implement
2. **Test with real RSS feeds** - BBC, Hacker News, TechCrunch
3. **Preserve backward compatibility** - HTML parser is optional, fallback to normalizer
4. **Log warnings, not errors** - If metadata extraction fails, continue
5. **Make it configurable** - Allow users to disable features

## Future Enhancements

1. **Custom boilerplate selectors** - Allow users to specify additional elements to remove
2. **Image download** - Optionally download and embed images
3. **Video extraction** - Extract video URLs as metadata
4. **PDF conversion** - Convert embedded PDFs to text
5. **Caching** - Cache parsed HTML to avoid re-processing
6. **Content scoring** - Use readability algorithms to identify main content
7. **Multiple output formats** - Support HTML, Markdown, Plain text output

## Why This Is Production-Ready Staff-Level Thinking

✅ **Separation of Concerns**: HTML parsing separate from RSS loading  
✅ **Reusability**: Works for any HTML-based content source  
✅ **LLM-Optimized**: Clean content = better embeddings  
✅ **Agent-Friendly**: Links can be followed, metadata can be filtered  
✅ **Testability**: Each component independently testable  
✅ **Maintainability**: Modular code, clear responsibilities  
✅ **Extensibility**: Easy to add new metadata sources or output formats  

This implementation transforms "dump raw HTML" into "intelligent content transformation pipeline" suitable for production RAG applications.