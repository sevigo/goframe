# Wiki/Confluence RAG System Design

> Architecture for building a Q&A system on internal wiki/confluence using goframe.

## Executive Summary

This document outlines how to build a RAG-based Q&A system for internal Wiki/Confluence using goframe as the core library, extracting reusable patterns from code-warden.

**Key Decision**: Build on goframe, not code-warden. code-warden is specialized for codebases/GitHub; wiki systems need different ingestion (HTML/Markdown, API fetching, page hierarchies).

---

## 1. Architectural Overview

### 1.1 Separation of Concerns

```
┌─────────────────────────────────────────────────────────────────┐
│                    wiki-warden (NEW APP)                         │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ cmd/wiki-server/                                           │ │
│  │  - HTTP server with /chat, /ingest endpoints               │ │
│  │  - Slack/Teams bot integration                             │ │
│  │  - Ingestion cron jobs                                     │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                         goframe                                  │
│  ┌──────────────────┐  ┌──────────────────┐                    │
│  │ documentloaders/ │  │ vectorstores/    │                    │
│  │  - GitLoader     │  │  - QdrantStore   │                    │
│  │  - Confluence ★  │  │  - Retrievers    │                    │
│  │  - Markdown ★    │  └──────────────────┘                    │
│  └──────────────────┘  ┌──────────────────┐                    │
│  ┌──────────────────┐  │ chains/          │                    │
│  │ textsplitters/   │  │  - LLMChain      │                    │
│  │  - RecursiveChar │  │  - RetrievalQA   │                    │
│  │  - CodeAware     │  │  - MapReduce ★   │                    │
│  │  - Markdown ★    │  └──────────────────┘                    │
│  └──────────────────┘  ┌──────────────────┐                    │
│  ┌──────────────────┐  │ contextpacker/   │                    │
│  │ embeddings/      │  │  - Token budget  │                    │
│  │  - Sparse vectors│  └──────────────────┘                    │
│  └──────────────────┘                                           │
└─────────────────────────────────────────────────────────────────┘

★ = NEW components to build
```

### 1.2 Data Flow

```
                    ┌─────────────────┐
                    │   Confluence    │
                    │   REST API      │
                    └────────┬────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    INGESTION PIPELINE                        │
├─────────────────────────────────────────────────────────────┤
│  1. ConfluenceLoader.fetchPages(spaceKeys)                  │
│     - Stream pages with metadata (space, parent, labels)   │
│     - Handle pagination and rate limiting                   │
│                                                             │
│  2. MarkdownHeaderSplitter.split(content)                   │
│     - Split by H1/H2/H3 headers                             │
│     - Preserve hierarchy in metadata                        │
│                                                             │
│  3. Embedder.embed(batch)                                   │
│     - Generate embeddings for each chunk                    │
│     - Use sparse vectors for BM25 hybrid                    │
│                                                             │
│  4. QdrantStore.addDocuments(docs)                          │
│     - Store vectors with metadata                            │
│     - Enable filtering by space, page, labels               │
└─────────────────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    QUERY PIPELINE                            │
├─────────────────────────────────────────────────────────────┤
│  1. User question → RetrievalQA                              │
│                                                             │
│  2. RetrievalQA.retrieve(query)                              │
│     - Hybrid search (dense + sparse/BM25)                   │
│     - Rerank with cross-encoder                             │
│     - Filter by space/label if specified                    │
│                                                             │
│  3. ContextPacker.pack(docs, tokenBudget)                   │
│     - Fit documents into context window                     │
│     - Prioritize by relevance score                         │
│                                                             │
│  4. LLMChain.generate(prompt + context)                     │
│     - Answer with citations                                  │
│     - Fallback if no relevant context                      │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. Components to Extract from code-warden

### 2.1 MapReduce Chain (HIGH PRIORITY)

**Source**: `code-warden/internal/rag/contextpkg/reduce.go`

**Pattern**:
- MAP: Generate per-document summaries (directory summaries in code-warden, page summaries in wiki)
- STORE: Persist intermediate results in vector store with metadata
- REDUCE: LLM synthesizes final output from all summaries

**Proposed Interface**:

```go
// goframe/chains/mapreduce.go
package chains

import (
    "context"

    "github.com/sevigo/goframe/schema"
    "github.com/sevigo/goframe/vectorstores"
)

// MapFunc transforms input into documents.
type MapFunc func(ctx context.Context, input any) ([]schema.Document, error)

// ReduceFunc synthesizes multiple documents into a single output.
type ReduceFunc func(ctx context.Context, docs []schema.Document) (string, error)

// MapReduceChain implements the MapReduce pattern for document processing.
type MapReduceChain struct {
    mapper      MapFunc
    reducer     ReduceFunc
    store       vectorstores.VectorStore
    storeFilter map[string]any  // Filter for retrieving mapped documents
    llm         llms.Model
    prompt      string
    batchSize   int
}

// NewMapReduceChain creates a new MapReduce chain.
func NewMapReduceChain(opts ...MapReduceOption) (*MapReduceChain, error) {
    // Configure with functional options
}

// Execute runs the full MapReduce pipeline.
func (c *MapReduceChain) Execute(ctx context.Context, input any) (string, error) {
    // 1. MAP phase
    docs, err := c.mapper(ctx, input)
    if err != nil {
        return "", err
    }

    // 2. STORE phase
    if c.store != nil {
        if err := c.storeBatch(ctx, docs); err != nil {
            return "", err
        }
    }

    // 3. REDUCE phase
    return c.reducer(ctx, docs)
}

// Options
type MapReduceOption func(*MapReduceChain)

func WithStore(store vectorstores.VectorStore) MapReduceOption { ... }
func WithBatchSize(size int) MapReduceOption { ... }
func WithStoreFilter(filter map[string]any) MapReduceOption { ... }
```

**Use Cases**:
- Code-warden: Directory summaries → Project context
- Wiki-warden: Page summaries → Space overview
- Document summarization: Section summaries → Document summary

### 2.2 Smart Indexing Pattern (MEDIUM PRIORITY)

**Source**: `code-warden/internal/rag/index/indexer.go`

**Pattern**:
- Hash-based change detection (skip unchanged files)
- Parallel workers with rate limiting
- Batch processing for embeddings
- Progress tracking and resumability

**Proposed Interface**:

```go
// goframe/documentloaders/smart_index.go
package documentloaders

import (
    "context"
    "crypto/sha256"

    "github.com/sevigo/goframe/schema"
)

// IndexerConfig holds configuration for smart indexing.
type IndexerConfig struct {
    Workers        int           // Number of parallel workers
    BatchSize      int           // Documents per embedding batch
    HashAlgorithm  HashAlgorithm // sha256, md5, etc.
    ResumeEnabled  bool          // Enable resumable indexing
}

// SmartIndexer provides incremental document indexing with change detection.
type SmartIndexer struct {
    store    VectorStore
    embedder Embedder
    hasher   Hasher
    config   IndexerConfig
}

// IndexResult contains indexing statistics.
type IndexResult struct {
    Total     int
    New       int
    Updated   int
    Skipped   int
    Duration  time.Duration
}

// Index performs smart indexing with change detection.
func (i *SmartIndexer) Index(ctx context.Context, loader Loader) (*IndexResult, error) {
    // 1. Stream documents from loader
    // 2. Compute hash for each
    // 3. Compare with stored hashes
    // 4. Only process changed documents
    // 5. Batch embeddings
    // 6. Store vectors with hash metadata
}
```

### 2.3 File Filtering (LOW PRIORITY)

**Source**: `code-warden/internal/rag/index/filter.go`

**Pattern**: Already generic enough. Could move to goframe as `documentloaders/filter.go`.

---

## 3. New Components for Wiki/Confluence

### 3.1 Confluence Loader

**Location**: `goframe/documentloaders/confluence.go`

```go
package documentloaders

import (
    "context"

    "github.com/sevigo/goframe/schema"
)

// ConfluenceConfig holds configuration for Confluence API access.
type ConfluenceConfig struct {
    BaseURL       string   // e.g., "https://company.atlassian.net/wiki"
    Username      string   // API username
    APIToken      string   // API token
    SpaceKeys     []string // Spaces to index (empty = all)
    PageIDs       []string // Specific pages (optional)
    IncludeAttachments bool
    Workers       int      // Parallel workers for fetching
}

// ConfluenceLoader streams Confluence pages as documents.
type ConfluenceLoader struct {
    config ConfluenceConfig
    client *http.Client
}

// NewConfluenceLoader creates a new Confluence loader.
func NewConfluenceLoader(config ConfluenceConfig) (*ConfluenceLoader, error) {
    // Validate config, create HTTP client
}

// Load streams pages from Confluence.
func (l *ConfluenceLoader) Load(ctx context.Context) (<-chan schema.Document, error) {
    out := make(chan schema.Document, l.config.Workers*10)

    go func() {
        defer close(out)

        // For each space
        for _, spaceKey := range l.config.SpaceKeys {
            // Fetch pages with pagination
            pages := l.fetchPages(ctx, spaceKey)
            for page := range pages {
                // Convert to Document
                doc := schema.Document{
                    PageContent: l.extractContent(page),
                    Metadata: map[string]any{
                        "source":       page.ID,
                        "source_url":   page.Links.WebUI,
                        "title":        page.Title,
                        "space_key":    spaceKey,
                        "page_id":      page.ID,
                        "parent_id":    page.ParentID,
                        "author":       page.Version.Author,
                        "updated_at":   page.Version.When,
                        "labels":       page.Labels,
                        "ancestors":    page.Ancestors,
                    },
                }
                select {
                case out <- doc:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()

    return out, nil
}

// extractContent converts Confluence HTML to clean Markdown.
func (l *ConfluenceLoader) extractContent(page Page) string {
    // 1. Strip Confluence macros (info panels, etc.)
    // 2. Convert HTML to Markdown
    // 3. Extract code blocks properly
    // 4. Preserve heading hierarchy
}
```

**API Endpoints Needed**:
- `GET /rest/api/space/{spaceKey}/content` - List pages in space
- `GET /rest/api/content/{id}` - Get page content
- `GET /rest/api/content/search` - Search with CQL
- `GET /rest/api/content/{id}/child/attachment` - Get attachments

### 3.2 Markdown Header Splitter

**Location**: `goframe/textsplitters/markdown.go`

```go
package textsplitters

import (
    "strings"

    "github.com/sevigo/goframe/schema"
)

// MarkdownHeaderSplitter splits markdown by headers while preserving hierarchy.
type MarkdownHeaderSplitter struct {
    headersToSplitOn []Header
    chunkSize         int
    chunkOverlap       int
}

type Header struct {
    Level int    // 1 for #, 2 for ##, etc.
    Name  string // Header text
}

// NewMarkdownHeaderSplitter creates a splitter that respects markdown structure.
func NewMarkdownHeaderSplitter(chunkSize, chunkOverlap int) *MarkdownHeaderSplitter {
    return &MarkdownHeaderSplitter{
        headersToSplitOn: []Header{
            {Level: 1, Name: "#"},
            {Level: 2, Name: "##"},
            {Level: 3, Name: "###"},
            {Level: 4, Name: "####"},
        },
        chunkSize:    chunkSize,
        chunkOverlap: chunkOverlap,
    }
}

// Split documents into chunks while preserving header context.
func (s *MarkdownHeaderSplitter) Split(text string) ([]schema.Document, error) {
    // 1. Split by headers
    sections := s.splitByHeaders(text)

    // 2. For each section, create document with hierarchy metadata
    var docs []schema.Document
    for _, section := range sections {
        // Build metadata with parent headers
        metadata := map[string]any{
            "header_level": section.Level,
            "header_text":  section.Header,
            "parent_path":  section.ParentPath(), // e.g., "Introduction > Setup"
        }

        // If section is too large, apply recursive splitting
        if len(section.Content) > s.chunkSize {
            subDocs := s.recursiveSplit(section.Content)
            for _, subDoc := range subDocs {
                subDoc.Metadata = mergeMetadata(subDoc.Metadata, metadata)
                docs = append(docs, subDoc)
            }
        } else {
            docs = append(docs, schema.Document{
                PageContent: section.Content,
                Metadata:    metadata,
            })
        }
    }

    return docs, nil
}
```

### 3.3 Wiki Context Builder

**Location**: New application (`wiki-warden`), not goframe

```go
// wiki-warden/internal/context/builder.go
package context

// WikiContextBuilder builds context for wiki Q&A.
type WikiContextBuilder struct {
    store       vectorstores.VectorStore
    reranker    schema.Reranker
    tokenBudget int
}

// BuildContext retrieves relevant wiki pages for a question.
func (b *WikiContextBuilder) BuildContext(ctx context.Context, question string, spaceFilter string) (string, error) {
    // 1. Hybrid search (dense + BM25)
    docs, err := b.hybridSearch(ctx, question, spaceFilter)

    // 2. Rerank with cross-encoder
    if b.reranker != nil {
        docs = b.reranker.Rerank(docs, question)
    }

    // 3. Pack into token budget
    context := b.packer.Pack(ctx, docs, b.tokenBudget)

    return context, nil
}

// GetPageHierarchy retrieves parent and child pages for context.
func (b *WikiContextBuilder) GetPageHierarchy(ctx context.Context, pageID string) ([]schema.Document, error) {
    // Fetch parent page and direct children for context
}
```

---

## 4. Implementation Phases

### Phase 1: Foundation (Week 1-2)

**Goal**: Basic wiki ingestion and retrieval

```bash
# In goframe
goframe/
├── documentloaders/
│   └── confluence.go      # NEW - Confluence API loader
├── textsplitters/
│   └── markdown.go        # NEW - Markdown header splitter
└── chains/
    └── mapreduce.go        # NEW - Extract from code-warden
```

**Tasks**:
1. Implement `ConfluenceLoader` with pagination and rate limiting
2. Implement `MarkdownHeaderSplitter` with hierarchy preservation
3. Extract `MapReduceChain` from code-warden
4. Write unit tests with mock Confluence API

### Phase 2: Indexing Pipeline (Week 3-4)

**Goal**: Full ingestion pipeline with smart indexing

```bash
# New application
wiki-warden/
├── cmd/
│   └── wiki-server/
│       └── main.go
├── internal/
│   ├── indexer/
│   │   └── indexer.go     # Smart indexing for wiki
│   ├── loader/
│   │   └── loader.go      # Confluence-specific logic
│   └── config/
│       └── config.go
└── go.mod                  # depends on goframe
```

**Tasks**:
1. Create `wiki-warden` application skeleton
2. Implement ingestion pipeline using goframe components
3. Add hash-based change detection for incremental updates
4. Configure Qdrant collections with proper schema

### Phase 3: Query Pipeline (Week 5-6)

**Goal**: Answer questions with citations

**Tasks**:
1. Implement `WikiContextBuilder` with hybrid search
2. Create Q&A prompts with citation formatting
3. Add reranking for better relevance
4. Implement fallback when no relevant context

### Phase 4: Production Ready (Week 7-8)

**Goal**: Deployable service

**Tasks**:
1. HTTP API endpoints (`/chat`, `/ingest`, `/health`)
2. Slack/Teams bot integration
3. Scheduled ingestion (cron)
4. Metrics and monitoring
5. Rate limiting and caching

---

## 5. Metadata Schema

### 5.1 Wiki Document Metadata

```go
// Metadata for wiki pages in vector store
type WikiMetadata struct {
    // Identity
    Source      string   `json:"source"`       // Page ID
    SourceURL   string   `json:"source_url"`   // Confluence URL
    Title       string   `json:"title"`        // Page title

    // Hierarchy
    SpaceKey    string   `json:"space_key"`    // e.g., "ENG", "PRODUCT"
    SpaceName   string   `json:"space_name"`   // "Engineering Team"
    ParentID    string   `json:"parent_id"`    // Parent page ID
    Ancestors   []string `json:"ancestors"`    // [parent, grandparent, ...]

    // Content
    HeaderPath  string   `json:"header_path"`  // "Setup > Installation > Docker"
    ContentType string   `json:"content_type"`// "text", "code", "table"

    // Authorship
    Author      string   `json:"author"`       // Last editor
    UpdatedAt   string   `json:"updated_at"`   // ISO timestamp
    Version     int      `json:"version"`      // Page version

    // Classification
    Labels      []string `json:"labels"`       // Confluence labels
    ChunkType   string   `json:"chunk_type"`   // "content", "summary"
    ContentHash string   `json:"content_hash"` // For incremental updates
}
```

### 5.2 Qdrant Collection Schema

```json
{
  "collection_name": "wiki",
  "vectors": {
    "size": 768,
    "distance": "Cosine"
  },
  "payload_schema": {
    "source": "keyword",
    "space_key": "keyword",
    "parent_id": "keyword",
    "labels": "keyword[]",
    "header_path": "text",
    "updated_at": "integer",
    "content_hash": "keyword",
    "chunk_type": "keyword"
  }
}
```

---

## 6. Prompt Templates

### 6.1 Wiki Q&A Prompt

```go
// wiki-warden/prompts/wiki_qa.prompt
var WikiQAPrompt = `You are a helpful assistant answering questions based on the company wiki.

Below are relevant wiki pages. Use ONLY this information to answer the question.

{{.Context}}

## Question
{{.Question}}

## Instructions
1. Answer the question using ONLY the provided context
2. If the context doesn't contain the answer, say "I couldn't find relevant information in the wiki"
3. Always cite your sources using [Page Title](URL) format
4. If multiple pages have conflicting information, mention all perspectives

## Answer
Provide a clear, concise answer with citations:`
```

### 6.2 Page Summary Prompt (for MapReduce)

```go
// wiki-warden/prompts/page_summary.prompt
var PageSummaryPrompt = `Summarize the following wiki page section for a knowledge base.

Title: {{.Title}}
Section: {{.HeaderPath}}
Content:
{{.Content}}

Generate a concise summary that captures:
1. Key concepts and definitions
2. Important procedures or steps
3. Related topics and links

Keep the summary under 200 words. Focus on information that would help answer questions.`
```

---

## 7. Testing Strategy

### 7.1 Unit Tests

```go
// goframe/documentloaders/confluence_test.go
func TestConfluenceLoader_Load(t *testing.T) {
    // Mock HTTP server with Confluence API responses
    server := mockConfluenceServer()
    defer server.Close()

    loader, _ := NewConfluenceLoader(ConfluenceConfig{
        BaseURL:   server.URL,
        SpaceKeys: []string{"ENG"},
    })

    docs, err := collectDocs(loader.Load(context.Background()))
    require.NoError(t, err)
    assert.Len(t, docs, 10)
    assert.Equal(t, "ENG", docs[0].Metadata["space_key"])
}
```

### 7.2 Integration Tests

```go
// wiki-warden/integration/qa_test.go
func TestQA_EndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip()
    }

    // Requires running Qdrant and LLM
    qa := setupQA(t)

    answer, err := qa.Answer(context.Background(), "How do I deploy to staging?")
    require.NoError(t, err)
    assert.Contains(t, answer, "staging")
    assert.NotEmpty(t, answer.Citations)
}
```

---

## 8. Migration Path

### 8.1 From code-warden

If you have existing code-warden infrastructure:

1. **Shared Qdrant Instance**: Use same Qdrant, different collection
2. **Shared LLM Config**: Reuse `ai.llm_provider`, `ai.generator_model` config
3. **New Collection**: Create `wiki` collection alongside `code` collection

### 8.2 Configuration

```yaml
# wiki-warden/config.yaml
confluence:
  base_url: "https://company.atlassian.net/wiki"
  username: "${CONFLUENCE_USERNAME}"
  api_token: "${CONFLUENCE_API_TOKEN}"
  spaces:
    - "ENG"
    - "PRODUCT"
    - "DESIGN"
  workers: 5

embedding:
  provider: "ollama"
  model: "nomic-embed-text"

llm:
  provider: "ollama"
  model: "llama3"

qdrant:
  host: "localhost"
  port: 6333
  collection: "wiki"

indexing:
  batch_size: 100
  hash_algorithm: "sha256"
  schedule: "0 */6 * * *"  # Every 6 hours
```

---

## 9. References

### 9.1 Related Files in code-warden

| File | Purpose | Extract To |
|------|---------|------------|
| `internal/rag/contextpkg/reduce.go` | MapReduce pattern | `goframe/chains/mapreduce.go` |
| `internal/rag/index/indexer.go` | Smart indexing | `goframe/documentloaders/smart_index.go` |
| `internal/rag/index/filter.go` | File filtering | `goframe/documentloaders/filter.go` |

### 9.2 Existing goframe Components

| Component | Location | Status |
|-----------|----------|--------|
| GitLoader | `goframe/documentloaders/git.go` | ✅ Ready |
| TextSplitters | `goframe/textsplitters/` | ✅ Ready |
| QdrantStore | `goframe/vectorstores/qdrant/` | ✅ Ready |
| RetrievalQA | `goframe/chains/retrieval_qa.go` | ✅ Ready |
| ContextPacker | `goframe/contextpacker/` | ✅ Ready |
| HyDE | `goframe/vectorstores/hyde.go` | ✅ Ready |

### 9.3 New Components Needed

| Component | Location | Priority |
|-----------|----------|----------|
| ConfluenceLoader | `goframe/documentloaders/` | HIGH |
| MarkdownHeaderSplitter | `goframe/textsplitters/` | HIGH |
| MapReduceChain | `goframe/chains/` | HIGH |
| SmartIndexer | `goframe/documentloaders/` | MEDIUM |
| WikiContextBuilder | `wiki-warden/internal/` | MEDIUM |

---

## 10. Success Metrics

- **Ingestion Speed**: 100+ pages/minute with 5 workers
- **Query Latency**: < 2s for typical questions
- **Relevance**: 80%+ of answers cite correct pages
- **Coverage**: 95%+ of indexed pages findable
- **Freshness**: Updates reflected within 6 hours