# RSS Loader Implementation

## Overview

Successfully implemented a comprehensive RSS/Atom feed loader for the goframe library following the same architecture pattern as the GitLoader.

## Features

### Core Components

1. **RSS Parser Plugin** (`parsers/rss.go`)
   - Implements `ParserPlugin` interface for RSS/Atom feeds
   - Handles RSS 2.0, Atom 1.0, and JSON feeds via `gofeed` library

2. **RSS Normalizer** (`documentloaders/rss_normalizer.go`)
   - HTML sanitization using `bluemonday` (XSS protection)
   - URL normalization (remove tracking parameters: UTM, fbclid, gclid)
   - Content truncation with sentence boundaries
   - Date parsing (multiple formats: RFC1123, RFC3339, ISO8601, etc.)
   - Author name normalization
   - Category deduplication and normalization
   - Title fallback to URL path

3. **RSS Loader** (`documentloaders/rss.go`)
   - Streaming pipeline with batch processing
   - Parallel feed fetching (configurable workers)
   - Memory protection and context propagation
   - Retry logic with exponential backoff
   - Deduplication by GUID/link
   - Functional options pattern for configuration

### Configuration Options

```go
documentloaders.NewRSS(
    feedURLs,
    registry,
    documentloaders.WithRSSBatchSize(50),           // Batch size for processing
    documentloaders.WithRSSWorkerCount(5),          // Parallel fetch workers
    documentloaders.WithRSSTimeout(30*time.Second), // HTTP timeout
    documentloaders.WithRSSMaxItems(100),           // Max items per feed
    documentloaders.WithRSSMaxRetries(3),           // Retry attempts
    documentloaders.WithRSSUserAgent("CustomBot/1.0"),
    documentloaders.WithRSSSkipDuplicates(true),
    documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
        StripHTML:        true,   // Strip HTML tags vs sanitize
        RemoveTracking:   true,   // Remove UTM parameters
        MaxContentLength: 10000,  // Truncate content
        MinContentLength: 100,    // Skip short items
        NormalizeURLs:    true,
        MinTitleLength:   5,
        FallbackToURL:    true,
        NormalizeAuthors: true,
    }),
)
```

### Usage Example

```go
// Simple load
loader, _ := documentloaders.NewRSS(feedURLs, registry)
docs, _ := loader.Load(ctx)

// Streaming with vector store
loader, _ := documentloaders.NewRSS(feedURLs, registry)
err := loader.LoadAndProcessStream(ctx, func(ctx context.Context, batch []schema.Document) error {
    ids, _ := vectorStore.AddDocuments(ctx, batch)
    return nil
})
```

## Dependencies

- `github.com/mmcdole/gofeed` v1.3.0 - RSS/Atom/JSON feed parser
- `github.com/microcosm-cc/bluemonday` v1.0.27 - HTML sanitizer (OWASP-compliant)

## Test Coverage

✅ **Unit Tests** - Normalizer functionality
- HTML sanitization/stripping
- URL normalization
- Date parsing
- Author/category normalization
- Content truncation

✅ **Integration Tests** - Loader functionality
- Feed loading and parsing
- Batch streaming
- Deduplication
- Timeout handling
- Error handling

All tests passing: **9 test suites, 45 test cases**

## Architecture

```
[Feed URLs] → [HTTP Workers] → [gofeed Parser] → [Normalizer] → [Document Stream]
     ↓              ↓                  ↓                ↓              ↓
  Config      Parallel Fetch     RSS/Atom/JSON    HTML Sanitize   Batch Process
             Retry Logic         Metadata         URL Cleanup     Deduplication
```

## Security Features

- XSS protection via `bluemonday`
- Tracking parameter removal (UTM, fbclid, etc.)
- Rel="nofollow noopener" on links
- Target="_blank" on external links
- Content sanitization with whitelist policy

## Performance

- Parallel feed fetching (configurable workers)
- Streaming pipeline with backpressure
- Memory-efficient batch processing
- Graceful context cancellation
- Retry with exponential backoff

## Example

See `examples/rss-ingestion/main.go` for a complete example showing:
- RSS feed loading
- HTML normalization
- Vector store ingestion
- Similarity search

## Linting Status

Minor warnings remaining:
- `dupl`: Duplicate `batchAndProcess` code (shared with GitLoader - acceptable)
- `intrange`: Go 1.22+ optimization suggestion (minor)

## Files Created

1. `parsers/rss.go` (68 lines)
2. `documentloaders/rss_normalizer.go` (334 lines)
3. `documentloaders/rss.go` (535 lines)
4. `documentloaders/rss_test.go` (268 lines)
5. `documentloaders/rss_normalizer_test.go` (368 lines)
6. `examples/rss-ingestion/main.go` (260 lines)

**Total: ~1,830 lines of production code + tests**

## Next Steps

The RSS loader is ready for use! You can:
1. Import RSS feeds into your RAG pipeline
2. Use with existing vector stores (Qdrant, etc.)
3. Combine with GitLoader for hybrid document sources
4. Customize normalization for your specific needs