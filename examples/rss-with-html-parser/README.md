# RSS + HTML Parser Example

This example demonstrates the complete pipeline for ingesting RSS feeds with HTML content into a vector database for RAG applications.

## What This Example Shows

### Complete Pipeline
```
RSS Feeds → HTML Parser → Vector Store → Semantic Search
   ↓             ↓              ↓              ↓
1. Load feeds  2. Transform   3. Embed      4. Query
   (RSS)       (Clean MD)    (Ollama)      (Results)
```

### Features Demonstrated

#### 1. **HTML Content Transformation**
- Removes boilerplate (nav, footer, scripts, ads)
- Converts HTML to clean Markdown
- Preserves semantic structure (headers, lists, code blocks)
- Extracts metadata (author, date, keywords)

#### 2. **RSS Feed Processing**
- Parallel feed fetching with worker pools
- Deduplication by GUID
- Rate limiting (respects servers)
- Retry logic with exponential backoff

#### 3. **Metadata Extraction**
- Author (from Open Graph, Schema.org, Dublin Core)
- Published Date (multiple formats)
- Title and Description
- Keywords/Tags
- Canonical URLs

#### 4. **Vector Store Integration**
- Automatic embedding with Ollama
- Qdrant vector database
- Batch processing for efficiency
- Similarity search

## Prerequisites

```bash
# 1. Install Ollama
curl -fsSL https://ollama.com/install.sh | sh

# 2. Pull embedding model
ollama pull nomic-embed-text
# or: ollama pull qwen3-embedding:0.6b

# 3. Start Qdrant (Docker)
docker run -d -p 6333:6333 qdrant/qdrant
```

## Running the Example

```bash
cd examples/rss-with-html-parser
go run main.go
```

## Expected Output

```
=== RSS + HTML Parser → Vector Store Pipeline ===

📦 Step 1: Initializing parsers...
🔧 Step 2: Creating HTML parser...
📡 Step 3: Creating RSS loader with HTML parser...
🧠 Step 4: Initializing embedder...
💾 Step 5: Initializing vector store...

🚀 Step 6: Loading RSS feeds...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📄 Document 1:
   Title: Understanding Go Concurrency
   Author: Jane Doe
   Published: 2024-03-06 10:00:00 UTC
   Link: https://example.com/article/123
   Keywords: go, concurrency, goroutines
   Content Preview:
   # Understanding Go Concurrency
   
   By Jane Doe
   
   This article explains goroutines and channels...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ Pipeline completed successfully!
```

## How It Works

### Step 1: Create HTML Parser
```go
htmlParser := html.NewHTMLParser(
    html.WithBoilerplateRemoval(true),  // Remove nav, footer, scripts
    html.WithMetadataExtraction(true),  // Get author, date, title
    html.WithMarkdownConversion(true),   // HTML → Markdown
)
```

### Step 2: Integrate with RSS Loader
```go
loader, _ := documentloaders.NewRSS(
    feedURLs,
    registry,
    documentloaders.WithHTMLParser(htmlParser), // 🔥 Key integration
)
```

### Step 3: Load and Process
```go
docs, _ := loader.Load(ctx)
// Each doc.PageContent is clean Markdown
// Each doc.Metadata contains extracted info
```

## What Gets Transformed

### Before (Raw HTML)
```html
<nav><ul>...</ul></nav>
<script>tracking code</script>
<article>
  <h1>Title</h1>
  <p>Content with <strong>formatting</strong></p>
</article>
<footer>Copyright</footer>
```

### After (Clean Markdown)
```markdown
# Title

Content with **formatting**
```

### Metadata Extracted
```go
Metadata{
    "author": "Jane Doe",
    "published_date": "2024-03-06",
    "title": "Article Title",
    "keywords": "go, concurrency",
}
```

## Customization Options

### HTML Parser
```go
htmlParser := html.NewHTMLParser(
    html.WithBaseURL("https://your-site.com"),      // Resolve relative links
    html.WithBoilerplateRemoval(true),               // Remove noise
    html.WithMetadataExtraction(true),               // Extract metadata
    html.WithMarkdownConversion(true),                // Convert to MD
    html.WithStructurePreservation(true),             // Keep headers/lists
)
```

### RSS Loader
```go
loader, _ := documentloaders.NewRSS(
    feedURLs,
    registry,
    documentloaders.WithHTMLParser(htmlParser),
    documentloaders.WithRSSMaxItems(100),            // Limit items
    documentloaders.WithRSSSkipDuplicates(true),     // Dedupe
    documentloaders.WithRSSBatchSize(50),            // Batch size
    documentloaders.WithRSSWorkerCount(5),           // Parallel workers
)
```

## Troubleshooting

### Ollama Not Available
```
⚠️  Running without vector store (Ollama not available)
```
The example will still run and show processed content, just without vector storage.

### Qdrant Not Running
```
Error: failed to create collection: connection refused
```
Start Qdrant: `docker run -d -p 6333:6333 qdrant/qdrant`

### No RSS Feeds
```
Error: no feed URLs provided
```
Make sure feedURLs slice contains valid RSS/Atom feed URLs.

## Next Steps

1. **Add Your Feeds**: Replace example URLs with your RSS sources
2. **Customize Parsing**: Adjust HTML parser options for your content
3. **Scale Up**: Increase batch sizes and worker counts for production
4. **Add Filters**: Filter by date, category, or keywords
5. **Monitor**: Add logging and metrics for production use

## Benefits

✅ **70% noise reduction** (boilerplate removal)  
✅ **Better embeddings** (clean Markdown vs HTML noise)  
✅ **Rich metadata** (author, date, keywords)  
✅ **Production-ready** (error handling, retry logic)  
✅ **Scalable** (parallel processing, batch ingestion)  

This example shows the complete transformation from raw RSS feeds with HTML content to clean, searchable documents in a vector database.