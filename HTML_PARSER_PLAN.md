# HTML Parser Implementation Plan

## Overview
Create a modular HTML parser that transforms HTML content from RSS feeds (and future sources like Confluence, Web) into clean Markdown for optimal LLM consumption.

## Architecture Goals

✅ **Modularity**: Separate HTML parsing logic from RSS loading  
✅ **Reusability**: Use for any HTML-based content source  
✅ **Clean Separation**: RSS loader → HTML parser → Markdown → Vector DB  
✅ **Pipeline Design**: Content transformation pipeline, not just data extraction  

## Why This Makes Sense for RSS

### Current RSS Implementation
- RSS feeds contain HTML in `<description>` and `<content:encoded>` fields
- Current normalizer only strips/sanitizes HTML (loses structure)
- No link resolution (relative URLs → absolute URLs)
- No governance metadata extraction (author, date)
- Raw HTML noise (nav, footer, scripts) pollutes embeddings

### Value Added by HTML Parser
1. **Cleaner Content**: Remove boilerplate that degrades LLM performance
2. **Structure Preservation**: Convert HTML structure to Markdown (headers, lists, code blocks)
3. **Link Resolution**: Enable agents to follow links for additional context
4. **Metadata Extraction**: Add author/date annotations to chunks
5. **Reusability**: Same parser works for Confluence, websites, etc.

## Implementation Plan

### Phase 1: Core Infrastructure (Priority: High)

#### 1.1 Add Dependencies
```bash
go get github.com/PuerkitoBio/goquery  # HTML parsing
go get github.com/gomarkdown/markdown    # Markdown conversion
```

#### 1.2 Create Parser Structure
**File**: `parsers/html/parser.go`

```go
package html

type HTMLParser struct {
    baseURL         string
    removeBoilerplate bool
    extractMetadata  bool
    toMarkdown       bool
}

type HTMLMetadata struct {
    Author         string
    PublishedDate  time.Time
    Title          string
    Description    string
    CanonicalURL   string
    Keywords       []string
}
```

### Phase 2: Core Features (Priority: High)

#### 2.1 Boilerplate Removal
**Purpose**: Remove navigation, footers, scripts, styles  
**Implementation**: Use goquery to strip unwanted tags

```go
func (p *HTMLParser) removeBoilerplate(doc *goquery.Document) {
    // Remove navigation
    doc.Find("nav, header, footer").Remove()
    
    // Remove scripts and styles
    doc.Find("script, style, noscript").Remove()
    
    // Remove common noise elements
    doc.Find("aside, .sidebar, .advertisement").Remove()
}
```

#### 2.2 Link Normalization
**Purpose**: Convert relative URLs to absolute URLs using base URL  
**Implementation**: Resolve links against RSS feed's base URL

```go
func (p *HTMLParser) normalizeLinks(doc *goquery.Document, baseURL string) {
    doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
        href, _ := s.Attr("href")
        if strings.HasPrefix(href, "/") || !strings.Contains(href, "://") {
            absoluteURL := resolveURL(baseURL, href)
            s.SetAttr("href", absoluteURL)
        }
    })
}
```

#### 2.3 Governance Annotations
**Purpose**: Extract author and published date from HTML metadata  
**Implementation**: Parse common metadata formats (Open Graph, Schema.org, Dublin Core)

```go
func (p *HTMLParser) extractMetadata(doc *goquery.Document) HTMLMetadata {
    metadata := HTMLMetadata{}
    
    // Author extraction
    if author := doc.Find("meta[name='author']").AttrOr("content", ""); author != "" {
        metadata.Author = author
    }
    
    // Published date extraction
    if date := doc.Find("meta[property='article:published_time']").AttrOr("content", ""); date != "" {
        metadata.PublishedDate = parseDate(date)
    }
    
    // Open Graph metadata
    metadata.Title = doc.Find("meta[property='og:title']").AttrOr("content", "")
    metadata.Description = doc.Find("meta[property='og:description']").AttrOr("content", "")
    
    return metadata
}
```

### Phase 3: HTML to Markdown Conversion (Priority: High)

#### 3.1 Structured Conversion
**Purpose**: Preserve HTML structure as Markdown for better LLM understanding  
**Benefits**: 
- Headers become `##` (semantic meaning preserved)
- Lists become `-` (structure maintained)
- Code blocks become ``` (formatting preserved)
- Links become `[text](url)` (clickable in markdown)

#### 3.2 Implementation

```go
func (p *HTMLParser) toMarkdown(html string) string {
    // 1. Parse HTML
    doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
    
    // 2. Remove boilerplate
    p.removeBoilerplate(doc)
    
    // 3. Normalize links
    p.normalizeLinks(doc, p.baseURL)
    
    // 4. Convert to Markdown
    var markdown strings.Builder
    doc.Find("body").Contents().Each(func(i int, s *goquery.Selection) {
        markdown.WriteString(p.nodeToMarkdown(s))
    })
    
    return markdown.String()
}
```

### Phase 4: Parser Plugin Integration (Priority: High)

#### 4.1 Implement ParserPlugin Interface

```go
func (p *HTMLParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
    // 1. Parse HTML
    doc, _ := goquery.NewDocumentFromReader(strings.NewReader(content))
    
    // 2. Remove boilerplate
    p.removeBoilerplate(doc)
    
    // 3. Normalize links
    p.normalizeLinks(doc, p.baseURL)
    
    // 4. Extract metadata
    metadata := p.extractMetadata(doc)
    
    // 5. Convert to Markdown
    markdown := p.toMarkdown(content)
    
    // 6. Create chunks with metadata
    chunks := []schema.CodeChunk{{
        Content:      markdown,
        Type:         "html_content",
        Identifier:   path,
        IsDefinition: false,
        Annotations: map[string]string{
            "author":         metadata.Author,
            "published_date": metadata.PublishedDate.String(),
        },
    }}
    
    return chunks, nil
}
```

### Phase 5: RSS Loader Integration (Priority: High)

#### 5.1 Update RSSLoader to Use HTML Parser

**Current Approach**:
```go
// RSS normalizer strips HTML to plain text
content = r.normalizer.NormalizeContent(item.Content)
```

**New Approach**:
```go
// Use HTML parser for full content
if item.Content != "" && r.htmlParser != nil {
    chunks, _ := r.htmlParser.Chunk(item.Content, item.Link, nil)
    // Convert chunks to documents
} else {
    // Fallback to normalizer
}
```

#### 5.2 Configuration Option

```go
func WithHTMLParser(parser *html.HTMLParser) RSSLoaderOption {
    return func(opts *rssLoaderOptions) {
        opts.HTMLParser = parser
    }
}
```

### Phase 6: Testing Strategy (Priority: Medium)

#### 6.1 Unit Tests
- Boilerplate removal (nav, footer, scripts)
- Link normalization (relative → absolute)
- Metadata extraction (author, date from various formats)
- HTML to Markdown conversion

#### 6.2 Integration Tests
- RSS feed with full HTML content
- RSS feed with partial HTML
- RSS feed with relative links
- RSS feed with various metadata formats

#### 6.3 Test Data
```html
<!-- Test case: Relative links -->
<a href="/article/123">Read More</a>

<!-- Test case: Boilerplate -->
<nav><ul>...</ul></nav>
<footer>...</footer>
<script>alert('noise')</script>
```

### Phase 7: Example Implementation (Priority: Low)

#### 7.1 Create Example
**File**: `examples/rss-news-demo/main.go`

```go
// 1. Initialize HTML parser
htmlParser := html.NewParser(
    html.WithBaseURL(feedBaseURL),
    html.WithBoilerplateRemoval(true),
    html.WithMarkdownOutput(true),
)

// 2. Initialize RSS loader
rssLoader := documentloaders.NewRSS(
    feedURLs,
    registry,
    documentloaders.WithHTMLParser(htmlParser),
)

// 3. Load and process
docs, _ := rssLoader.Load(ctx)

// 4. Show clean output
for _, doc := range docs {
    fmt.Printf("Title: %s\n", doc.Metadata["title"])
    fmt.Printf("Author: %s\n", doc.Metadata["author"])
    fmt.Printf("Date: %s\n", doc.Metadata["published_date"])
    fmt.Printf("Content:\n%s\n", doc.PageContent)
}
```

## File Structure

```
goframe/
├── parsers/
│   └── html/
│       ├── parser.go           # HTML parser implementation
│       ├── parser_test.go      # Unit tests
│       ├── boilerplate.go       # Boilerplate removal logic
│       ├── links.go            # Link normalization
│       ├── metadata.go          # Metadata extraction
│       └── markdown.go          # HTML to Markdown conversion
├── documentloaders/
│   └── rss.go                  # Updated to use HTML parser
└── examples/
    └── rss-news-demo/
        └── main.go             # Demo showing RSS + HTML pipeline
```

## Dependencies

```go
import (
    "github.com/PuerkitoBio/goquery"     // HTML parsing (already exists)
    "github.com/gomarkdown/markdown"      // Markdown conversion
    "github.com/microcosm-cc/bluemonday"  // Already used for sanitization
)
```

## Success Metrics

✅ **Cleaner Content**: HTML noise reduced by 70%+  
✅ **Better RAG**: Link resolution enables agent "follow links" capability  
✅ **Rich Metadata**: Author and date annotations added to 90%+ of documents  
✅ **Modular Design**: HTML parser works independently from RSS  
✅ **Reusability**: Same parser can be used for Confluence, websites, etc.  

## Timeline Estimate

- **Phase 1-2**: 2-3 hours (core infrastructure)
- **Phase 3-4**: 3-4 hours (HTML to Markdown + integration)
- **Phase 5-6**: 2-3 hours (RSS integration + tests)
- **Phase 7**: 1 hour (example)

**Total**: ~8-11 hours

## Staff-Level Thinking Principles Applied

1. **Modularity Over Monolith**: Separate HTML parsing from RSS loading
2. **Pipeline Design**: Content transformation pipeline, not just scripts
3. **Code Reuse**: Works for RSS, Confluence, and future sources
4. **LLM-First**: Clean content = better embeddings = better RAG
5. **Agent-Friendly**: Link resolution enables "follow links" capability
6. **Governance**: Author and date annotations improve search and filtering

## Questions to Resolve

1. **Should we preserve image alt text?** (Yes - useful for context)
2. **How to handle embedded videos?** (Extract video URLs as metadata)
3. **Markdown table support?** (Yes - tables preserve structure)
4. **Custom boilerplate selectors?** (Configurable via options)
5. **Max content length?** (Inherit from RSS normalizer config)

## Next Steps

1. Create `parsers/html/` directory structure
2. Implement core HTML parser with goquery
3. Add boilerplate removal
4. Implement link normalization
5. Add metadata extraction
6. Convert to Markdown
7. Integrate with RSS loader
8. Add comprehensive tests
9. Create example demonstrating the pipeline

This plan transforms the RSS loader from a simple "dump text" approach into a **content transformation pipeline** suitable for production RAG applications.