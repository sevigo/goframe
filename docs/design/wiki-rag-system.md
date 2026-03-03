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

---

## 11. Demo Project: Employee Knowledge Navigator (Staffbase)

> This section describes a demo project aligned with Staffbase's Employee AI platform, suitable for job application demonstration.

### 11.1 Company Alignment

Staffbase ([staffbase.com](https://staffbase.com)) is building the **first AI-native Employee Experience Platform** with these key products:

| Product | Description | Demo Alignment |
|---------|-------------|----------------|
| **Native AI Assistant** (Q4 2025) | Conversational AI that answers questions AND completes tasks | ✅ Core demo feature |
| **Hyper-Personal Podcasts** (Q4 2025) | 100K unique podcasts for 100K employees | ⚪ Future extension |
| **Agentic Content Governance** (Q1 2026) | Auto-flags outdated/duplicate content | ✅ Multi-agent demo |
| **AI Writing Companion** | Helps editors draft content | ⚪ Content generation |

**Key Technical Requirements from JD:**
- Go backend expertise (microservices)
- RAG pipelines in production
- Agentic frameworks (multi-agent workflows)
- AIOps (Langfuse, observability)
- Azure cloud (deployment)

### 11.2 Demo Project Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     employee-knowledge-nav (Go)                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ cmd/server/                                                        │ │
│  │  - REST API: POST /chat, POST /ingest, GET /search                │ │
│  │  - WebSocket: Streaming responses                                 │ │
│  │  - Voice endpoint: Audio transcription + TTS                     │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ internal/                                                          │ │
│  │  ├── rag/                 # RAG pipeline (goframe)               │ │
│  │  │   ├── pipeline.go      # Query orchestration                   │ │
│  │  │   ├── reranker.go      # Cross-encoder reranking               │ │
│  │  │   └── citation.go      # Source citation formatting            │ │
│  │  ├── agent/               # Multi-agent workflows                 │ │
│  │  │   ├── governance.go    # Content freshness detection           │ │
│  │  │   ├── duplicate.go      # Duplicate content finder             │ │
│  │  │   └── suggester.go      # Update suggestion generator          │ │
│  │  ├── observability/       # Langfuse integration                  │ │
│  │  │   ├── tracer.go         # LLM call tracing                      │ │
│  │  │   └── metrics.go       # Token/cost/latency tracking            │ │
│  │  └── voice/               # Voice interface                       │ │
│  │      ├── whisper.go        # Speech-to-text                       │ │
│  │      └── tts.go            # Text-to-speech                       │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                              │                                          │
│                              ▼                                          │
└─────────────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                            goframe                                       │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐       │
│  │ documentloaders/ │  │ chains/          │  │ observability/   │       │
│  │  - Confluence ★  │  │  - MapReduce ★   │  │  - Langfuse ★    │       │
│  │  - Git (exists)  │  │  - RetrievalQA   │  └──────────────────┘       │
│  └──────────────────┘  └──────────────────┘                             │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐       │
│  │ textsplitters/   │  │ vectorstores/    │  │ agents/ ★        │       │
│  │  - Markdown ★    │  │  - Qdrant        │  │  - Governance    │       │
│  │  - Code (exists) │  │  - Reranking     │  │  - Supervisor    │       │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘       │
└─────────────────────────────────────────────────────────────────────────┘

★ = NEW components to build
```

---

### 11.3 What to Add to goframe

#### 11.3.1 ConfluenceLoader (HIGH PRIORITY)

**Location**: `goframe/documentloaders/confluence.go`

**Purpose**: Stream Confluence pages as documents for RAG ingestion.

**Implementation Details**:

```go
package documentloaders

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "github.com/sevigo/goframe/schema"
)

// ConfluenceConfig holds configuration for Confluence API access.
type ConfluenceConfig struct {
    // Required
    BaseURL   string // e.g., "https://company.atlassian.net/wiki"
    APIToken  string // Personal access token or PAT

    // Optional
    Username      string   // For basic auth (if not using PAT)
    SpaceKeys     []string // Spaces to index (empty = all accessible)
    PageIDs       []string // Specific pages only (optional)
    IncludeAttachments bool
    MaxPages      int      // Max pages to fetch (0 = unlimited)

    // Performance
    Workers       int           // Parallel workers (default: 5)
    BatchSize     int           // Pages per batch (default: 25)
    RateLimit     time.Duration // Delay between requests (default: 100ms)
    Timeout       time.Duration // HTTP timeout (default: 30s)

    // Content processing
    StripMacros   bool     // Remove Confluence macros (default: true)
    ConvertFormat string   // Output format: "markdown" or "html" (default: "markdown")
}

// ConfluenceLoader streams Confluence pages as documents.
type ConfluenceLoader struct {
    config     ConfluenceConfig
    client     *http.Client
    rateLimit  <-chan time.Time
}

// NewConfluenceLoader creates a new Confluence loader with validation.
func NewConfluenceLoader(config ConfluenceConfig) (*ConfluenceLoader, error) {
    if config.BaseURL == "" {
        return nil, fmt.Errorf("base URL is required")
    }
    if config.APIToken == "" && config.Username == "" {
        return nil, fmt.Errorf("API token or username is required")
    }

    // Set defaults
    if config.Workers == 0 {
        config.Workers = 5
    }
    if config.BatchSize == 0 {
        config.BatchSize = 25
    }
    if config.RateLimit == 0 {
        config.RateLimit = 100 * time.Millisecond
    }
    if config.Timeout == 0 {
        config.Timeout = 30 * time.Second
    }
    if config.ConvertFormat == "" {
        config.ConvertFormat = "markdown"
    }

    return &ConfluenceLoader{
        config: config,
        client: &http.Client{Timeout: config.Timeout},
        rateLimit: time.NewTicker(config.RateLimit).C,
    }, nil
}

// Load streams pages from Confluence as documents.
func (l *ConfluenceLoader) Load(ctx context.Context) (<-chan schema.Document, error) {
    out := make(chan schema.Document, l.config.Workers*10)

    go func() {
        defer close(out)

        // If specific pages requested, fetch those
        if len(l.config.PageIDs) > 0 {
            l.fetchPagesByID(ctx, l.config.PageIDs, out)
            return
        }

        // Otherwise, fetch by space
        spaces := l.config.SpaceKeys
        if len(spaces) == 0 {
            spaces = l.discoverSpaces(ctx)
        }

        for _, spaceKey := range spaces {
            l.fetchSpacePages(ctx, spaceKey, out)
        }
    }()

    return out, nil
}

// fetchSpacePages fetches all pages in a space with pagination.
func (l *ConfluenceLoader) fetchSpacePages(ctx context.Context, spaceKey string, out chan<- schema.Document) {
    start := 0
    for {
        l.waitForRateLimit()

        // GET /rest/api/space/{spaceKey}/content
        pages, hasMore, err := l.fetchPageBatch(ctx, spaceKey, start)
        if err != nil {
            // Log error and continue with next space
            continue
        }

        // Process pages in parallel
        l.processPageBatch(ctx, pages, out)

        if !hasMore {
            break
        }
        start += l.config.BatchSize

        // Check max pages limit
        if l.config.MaxPages > 0 && start >= l.config.MaxPages {
            break
        }
    }
}

// processPageBatch processes a batch of pages with workers.
func (l *ConfluenceLoader) processPageBatch(ctx context.Context, pages []Page, out chan<- schema.Document) {
    var wg sync.WaitGroup
    wg.Add(len(pages))

    for _, page := range pages {
        go func(p Page) {
            defer wg.Done()

            // Fetch full page content
            fullPage, err := l.fetchPageContent(ctx, p.ID)
            if err != nil {
                return
            }

            // Extract and convert content
            content := l.extractContent(fullPage)

            // Create document with metadata
            doc := schema.Document{
                PageContent: content,
                Metadata: map[string]any{
                    "source":        fullPage.ID,
                    "source_url":    fullPage.Links.WebUI,
                    "title":         fullPage.Title,
                    "space_key":     fullPage.Space.Key,
                    "space_name":    fullPage.Space.Name,
                    "page_id":       fullPage.ID,
                    "parent_id":     fullPage.ParentID,
                    "author":        fullPage.Version.Author,
                    "updated_at":    fullPage.Version.When,
                    "version":       fullPage.Version.Number,
                    "labels":         l.extractLabels(fullPage.Labels),
                    "ancestors":      l.extractAncestors(fullPage.Ancestors),
                    "content_type":  l.detectContentType(fullPage.Body),
                    "content_hash":  l.hashContent(content),
                },
            }

            select {
            case out <- doc:
            case <-ctx.Done():
                return
            }
        }(page)
    }

    wg.Wait()
}

// extractContent converts Confluence HTML to clean Markdown.
func (l *ConfluenceLoader) extractContent(page Page) string {
    content := page.Body.Storage.Value

    // Strip Confluence macros if configured
    if l.config.StripMacros {
        content = l.stripConfluenceMacros(content)
    }

    // Convert to Markdown
    if l.config.ConvertFormat == "markdown" {
        content = l.htmlToMarkdown(content)
    }

    return content
}
```

**API Endpoints Used**:

| Endpoint | Purpose | Pagination |
|----------|---------|------------|
| `GET /rest/api/space` | List all spaces | Yes |
| `GET /rest/api/space/{key}/content` | List pages in space | Yes (start, limit) |
| `GET /rest/api/content/{id}` | Get page with body | No |
| `GET /rest/api/content/{id}/child/attachment` | Get attachments | Yes |
| `GET /rest/api/content/search?cql=...` | Advanced search | Yes |

#### 11.3.2 MarkdownHeaderSplitter (HIGH PRIORITY)

**Location**: `goframe/textsplitters/markdown.go`

**Purpose**: Split markdown documents by headers while preserving hierarchy.

```go
package textsplitters

import (
    "regexp"
    "strings"

    "github.com/sevigo/goframe/schema"
)

// MarkdownHeaderSplitter splits markdown by headers while preserving hierarchy.
type MarkdownHeaderSplitter struct {
    headersToSplitOn []headerSplit
    chunkSize        int
    chunkOverlap     int
    stripHeaders     bool
}

type headerSplit struct {
    Level int
    Name  string // e.g., "#", "##"
}

// NewMarkdownHeaderSplitter creates a splitter that respects markdown structure.
func NewMarkdownHeaderSplitter(opts ...MarkdownSplitOption) *MarkdownHeaderSplitter {
    s := &MarkdownHeaderSplitter{
        headersToSplitOn: []headerSplit{
            {Level: 1, Name: "#"},
            {Level: 2, Name: "##"},
            {Level: 3, Name: "###"},
            {Level: 4, Name: "####"},
        },
        chunkSize:    1000,
        chunkOverlap: 200,
        stripHeaders: false,
    }

    for _, opt := range opts {
        opt(s)
    }

    return s
}

// Split splits markdown text into chunks while preserving header context.
func (s *MarkdownHeaderSplitter) Split(text string) ([]schema.Document, error) {
    // 1. Parse markdown into sections by headers
    sections := s.parseSections(text)

    // 2. Convert sections to documents with hierarchy metadata
    var docs []schema.Document
    for _, section := range sections {
        // Build metadata with parent headers
        metadata := map[string]any{
            "header_level":    section.Level,
            "header_text":     section.Header,
            "header_path":     strings.Join(section.Breadcrumb, " > "),
            "header_ancestors": section.Breadcrumb[:len(section.Breadcrumb)-1],
        }

        content := section.Content
        if !s.stripHeaders && section.Header != "" {
            content = section.Header + "\n" + content
        }

        // If section is too large, apply recursive splitting
        if len(content) > s.chunkSize {
            subDocs := s.recursiveSplit(content, metadata)
            docs = append(docs, subDocs...)
        } else {
            docs = append(docs, schema.Document{
                PageContent: content,
                Metadata:    metadata,
            })
        }
    }

    return docs, nil
}

// MarkdownSection represents a parsed markdown section.
type MarkdownSection struct {
    Level      int      // Header level (1-6)
    Header     string   // Header text without #
    Content    string   // Content under the header
    Breadcrumb []string // Full path: ["H1", "H2", "H3"]
}

// parseSections parses markdown into hierarchical sections.
func (s *MarkdownHeaderSplitter) parseSections(text string) []MarkdownSection {
    lines := strings.Split(text, "\n")
    var sections []MarkdownSection

    var currentSection *MarkdownSection
    breadcrumb := make([]string, 6) // Max 6 header levels

    headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

    for _, line := range lines {
        matches := headerRegex.FindStringSubmatch(line)
        if matches != nil {
            // Save previous section
            if currentSection != nil {
                sections = append(sections, *currentSection)
            }

            level := len(matches[1])
            header := matches[2]

            // Update breadcrumb
            breadcrumb[level-1] = header
            for i := level; i < 6; i++ {
                breadcrumb[i] = ""
            }

            currentSection = &MarkdownSection{
                Level:      level,
                Header:     header,
                Content:    "",
                Breadcrumb: make([]string, 0, level),
            }

            // Build breadcrumb from non-empty entries
            for i := 0; i < level; i++ {
                if breadcrumb[i] != "" {
                    currentSection.Breadcrumb = append(currentSection.Breadcrumb, breadcrumb[i])
                }
            }
        } else if currentSection != nil {
            currentSection.Content += line + "\n"
        }
    }

    // Don't forget last section
    if currentSection != nil {
        sections = append(sections, *currentSection)
    }

    return sections
}

// recursiveSplit applies character splitting for oversized sections.
func (s *MarkdownHeaderSplitter) recursiveSplit(content string, baseMetadata map[string]any) []schema.Document {
    // Use RecursiveCharacter splitter for oversized content
    recursive := NewRecursiveCharacter(s.chunkSize, s.chunkOverlap)

    chunks, err := recursive.Split(content)
    if err != nil {
        return nil
    }

    var docs []schema.Document
    for i, chunk := range chunks {
        metadata := make(map[string]any)
        for k, v := range baseMetadata {
            metadata[k] = v
        }
        metadata["chunk_index"] = i
        metadata["chunk_total"] = len(chunks)

        docs = append(docs, schema.Document{
            PageContent: chunk,
            Metadata:    metadata,
        })
    }

    return docs
}

// Functional options
type MarkdownSplitOption func(*MarkdownHeaderSplitter)

func WithChunkSize(size int) MarkdownSplitOption {
    return func(s *MarkdownHeaderSplitter) { s.chunkSize = size }
}

func WithChunkOverlap(overlap int) MarkdownSplitOption {
    return func(s *MarkdownHeaderSplitter) { s.chunkOverlap = overlap }
}

func WithStripHeaders(strip bool) MarkdownSplitOption {
    return func(s *MarkdownHeaderSplitter) { s.stripHeaders = strip }
}

func WithMaxHeaderLevel(level int) MarkdownSplitOption {
    return func(s *MarkdownHeaderSplitter) {
        s.headersToSplitOn = nil
        for i := 1; i <= level && i <= 6; i++ {
            s.headersToSplitOn = append(s.headersToSplitOn, headerSplit{
                Level: i,
                Name:  strings.Repeat("#", i),
            })
        }
    }
}
```

#### 11.3.3 MapReduceChain (HIGH PRIORITY)

**Location**: `goframe/chains/mapreduce.go`

**Purpose**: Generic MapReduce pattern for document processing (extract from code-warden).

**Source**: `code-warden/internal/rag/contextpkg/reduce.go`

```go
package chains

import (
    "context"
    "sync"

    "github.com/sevigo/goframe/llms"
    "github.com/sevigo/goframe/schema"
    "github.com/sevigo/goframe/vectorstores"
)

// MapReduceChain implements the MapReduce pattern for document processing.
// Use cases:
// - Summarizing multiple documents into one summary
// - Extracting key information from pages and synthesizing
// - Content governance: analyze pages → find issues → recommendations
type MapReduceChain struct {
    // Map phase
    mapPrompt    string           // Prompt template for mapping each document
    mapLLM       llms.Model       // LLM for map phase (can be smaller/faster)
    mapWorkers   int              // Parallel workers for map phase

    // Reduce phase
    reducePrompt string           // Prompt template for reducing all results
    reduceLLM    llms.Model        // LLM for reduce phase (usually larger)

    // Optional storage
    store        vectorstores.VectorStore
    storeFilter  map[string]any   // Filter for retrieving stored docs

    // Metadata
    chunkType    string           // Type tag for stored docs (e.g., "summary", "issue")
}

// MapReduceOption configures the chain.
type MapReduceOption func(*MapReduceChain)

func WithMapPrompt(prompt string) MapReduceOption {
    return func(c *MapReduceChain) { c.mapPrompt = prompt }
}

func WithMapLLM(llm llms.Model) MapReduceOption {
    return func(c *MapReduceChain) { c.mapLLM = llm }
}

func WithMapWorkers(workers int) MapReduceOption {
    return func(c *MapReduceChain) { c.mapWorkers = workers }
}

func WithReducePrompt(prompt string) MapReduceOption {
    return func(c *MapReduceChain) { c.reducePrompt = prompt }
}

func WithReduceLLM(llm llms.Model) MapReduceOption {
    return func(c *MapReduceChain) { c.reduceLLM = llm }
}

func WithStore(store vectorstores.VectorStore, filter map[string]any) MapReduceOption {
    return func(c *MapReduceChain) {
        c.store = store
        c.storeFilter = filter
    }
}

func WithChunkType(chunkType string) MapReduceOption {
    return func(c *MapReduceChain) { c.chunkType = chunkType }
}

// NewMapReduceChain creates a new MapReduce chain.
func NewMapReduceChain(opts ...MapReduceOption) *MapReduceChain {
    c := &MapReduceChain{
        mapWorkers: 5, // Default parallelism
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}

// Execute runs the full MapReduce pipeline.
func (c *MapReduceChain) Execute(ctx context.Context, input any) (string, error) {
    // 1. MAP phase - process each input in parallel
    mapResults, err := c.mapPhase(ctx, input)
    if err != nil {
        return "", err
    }

    // 2. STORE phase - optionally store intermediate results
    if c.store != nil {
        if err := c.storeResults(ctx, mapResults); err != nil {
            // Log warning but continue
        }
    }

    // 3. REDUCE phase - synthesize into final result
    return c.reducePhase(ctx, mapResults)
}

// mapPhase processes each input document in parallel.
func (c *MapReduceChain) mapPhase(ctx context.Context, input any) ([]mapResult, error) {
    // Convert input to documents
    docs, err := c.toDocuments(input)
    if err != nil {
        return nil, err
    }

    // Process in parallel with worker pool
    results := make([]mapResult, len(docs))
    var wg sync.WaitGroup
    sem := make(chan struct{}, c.mapWorkers)

    for i, doc := range docs {
        wg.Add(1)
        go func(idx int, d schema.Document) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()

            // Render prompt with document data
            prompt, err := c.renderMapPrompt(d)
            if err != nil {
                results[idx] = mapResult{err: err}
                return
            }

            // Call LLM
            response, err := c.mapLLM.Call(ctx, prompt)
            if err != nil {
                results[idx] = mapResult{err: err}
                return
            }

            results[idx] = mapResult{
                source:    d.Metadata["source"],
                content:   response,
                metadata: d.Metadata,
            }
        }(i, doc)
    }

    wg.Wait()
    return results, nil
}

// reducePhase synthesizes all map results into final output.
func (c *MapReduceChain) reducePhase(ctx context.Context, results []mapResult) (string, error) {
    // Combine all results into prompt
    combined := c.combineResults(results)

    // Render reduce prompt
    prompt, err := c.renderReducePrompt(combined)
    if err != nil {
        return "", err
    }

    // Call LLM for synthesis
    return c.reduceLLM.Call(ctx, prompt)
}

// Example usage for content governance:
func ExampleContentGovernance() {
    chain := NewMapReduceChain(
        WithMapPrompt(AnalyzeFreshnessPrompt),
        WithMapLLM(fastModel),
        WithMapWorkers(10),
        WithReducePrompt(SynthesizeGovernanceIssuesPrompt),
        WithReduceLLM(powerfulModel),
        WithStore(vectorStore, map[string]any{"type": "governance_issue"}),
    )

    // Analyze all wiki pages for governance issues
    issues, err := chain.Execute(ctx, wikiPages)
}
```

#### 11.3.4 Langfuse Integration (MEDIUM PRIORITY)

**Location**: `goframe/observability/langfuse.go`

**Purpose**: Production observability for LLM calls (JD requirement: "Langfuse, Arize Phoenix").

```go
package observability

import (
    "context"
    "time"

    "github.com/sevigo/goframe/llms"
)

// LangfuseConfig holds configuration for Langfuse integration.
type LangfuseConfig struct {
    PublicKey  string
    SecretKey  string
    BaseURL    string // Default: https://cloud.langfuse.com
    Enabled    bool
    SampleRate float64 // 0.0 to 1.0 (default: 1.0)
}

// LangfuseTracer provides LLM call tracing for observability.
type LangfuseTracer struct {
    config    LangfuseConfig
    client    *http.Client
    sessionID string
}

// NewLangfuseTracer creates a new tracer.
func NewLangfuseTracer(config LangfuseConfig) *LangfuseTracer {
    return &LangfuseTracer{
        config: config,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

// Span represents a traced operation.
type Span struct {
    ID        string
    Name      string
    StartTime time.Time
    EndTime   time.Time
    Metadata  map[string]any
    Input     string
    Output    string
    Model     string
    Tokens    TokenUsage
    parent    *Span
    client    *LangfuseTracer
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
    Prompt     int
    Completion int
    Total      int
}

// StartSpan begins a new traced operation.
func (t *LangfuseTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) *Span {
    span := &Span{
        ID:        generateID(),
        Name:      name,
        StartTime: time.Now(),
        Metadata:  make(map[string]any),
        client:    t,
    }

    for _, opt := range opts {
        opt(span)
    }

    return span
}

// End completes the span and sends to Langfuse.
func (s *Span) End() {
    s.EndTime = time.Now()

    if s.client.config.Enabled {
        go s.client.flushSpan(s)
    }
}

// WithModel sets the model used.
func (s *Span) WithModel(model string) *Span {
    s.Model = model
    return s
}

// WithInput sets the input prompt.
func (s *Span) WithInput(input string) *Span {
    s.Input = input
    return s
}

// WithOutput sets the output response.
func (s *Span) WithOutput(output string) *Span {
    s.Output = output
    return s
}

// WithTokens sets token usage.
func (s *Span) WithTokens(prompt, completion int) *Span {
    s.Tokens = TokenUsage{
        Prompt:     prompt,
        Completion: completion,
        Total:      prompt + completion,
    }
    return s
}

// WithMetadata adds metadata.
func (s *Span) WithMetadata(key string, value any) *Span {
    s.Metadata[key] = value
    return s
}

// Usage with RAG pipeline:
func ExampleRAGWithTracing() {
    tracer := NewLangfuseTracer(config)

    func (r *RAGService) Answer(ctx context.Context, query string) (*Answer, error) {
        span := tracer.StartSpan(ctx, "rag.answer",
            observability.WithModel(r.config.Model),
            observability.WithMetadata("query", query),
        )
        defer span.End()

        // Retrieve
        retrieveSpan := span.StartSpan("retrieve")
        docs := r.retrieve(ctx, query)
        retrieveSpan.End()

        // Generate
        genSpan := span.StartSpan("generate")
        answer, tokens := r.llm.Call(ctx, prompt)
        genSpan.WithTokens(tokens.Prompt, tokens.Completion).End()

        span.WithOutput(answer)
        return answer, nil
    }
}
```

#### 11.3.5 Multi-Agent Governance System (MEDIUM PRIORITY)

**Location**: `goframe/agents/governance.go`

**Purpose**: Agentic content governance for detecting outdated/duplicate content (Staffbase Q1 2026 feature).

```go
package agents

import (
    "context"
    "sync"

    "github.com/sevigo/goframe/chains"
    "github.com/sevigo/goframe/llms"
    "github.com/sevigo/goframe/schema"
)

// GovernanceAgent analyzes content for issues.
type GovernanceAgent struct {
    name      string
    llm       llms.Model
    prompt    string
    processor func(string) []GovernanceIssue
}

// GovernanceIssue represents a detected content issue.
type GovernanceIssue struct {
    Type        string  // "outdated", "duplicate", "conflict", "broken_link"
    Severity    string  // "high", "medium", "low"
    PageID      string
    PageTitle   string
    Description string
    Suggestion  string
    Confidence  float64
}

// GovernanceSystem coordinates multiple agents for content analysis.
type GovernanceSystem struct {
    agents     []*GovernanceAgent
    supervisor *SupervisorAgent
    store      VectorStore
}

// NewGovernanceSystem creates a multi-agent governance system.
func NewGovernanceSystem(config GovernanceConfig) *GovernanceSystem {
    return &GovernanceSystem{
        agents: []*GovernanceAgent{
            NewFreshnessAgent(config.FreshnessLLM, config.FreshnessThreshold),
            NewDuplicateAgent(config.DuplicateLLM, config.SimilarityThreshold),
            NewConflictAgent(config.ConflictLLM),
            NewBrokenLinkAgent(config.LinkChecker),
        },
        supervisor: NewSupervisorAgent(config.SupervisorLLM),
        store:      config.Store,
    }
}

// Analyze runs all agents in parallel and synthesizes results.
func (g *GovernanceSystem) Analyze(ctx context.Context, pages []schema.Document) (*GovernanceReport, error) {
    // MAP: Each agent analyzes pages in parallel
    results := make([][]GovernanceIssue, len(g.agents))
    var wg sync.WaitGroup

    for i, agent := range g.agents {
        wg.Add(1)
        go func(idx int, a *GovernanceAgent) {
            defer wg.Done()
            issues := a.Analyze(ctx, pages)
            results[idx] = issues
        }(i, agent)
    }

    wg.Wait()

    // Flatten issues
    allIssues := flattenIssues(results)

    // REDUCE: Supervisor synthesizes and prioritizes
    report, err := g.supervisor.Synthesize(ctx, allIssues, pages)
    if err != nil {
        return nil, err
    }

    return report, nil
}

// NewFreshnessAgent detects outdated content.
func NewFreshnessAgent(llm llms.Model, threshold time.Duration) *GovernanceAgent {
    return &GovernanceAgent{
        name: "freshness",
        llm:  llm,
        prompt: `Analyze the following wiki page for content freshness.

Title: {{.Title}}
Last Updated: {{.UpdatedAt}}
Content:
{{.Content}}

Check for:
1. Outdated information (e.g., "we will launch in 2023", deprecated processes)
2. Stale procedures that may no longer apply
3. References to tools/systems that have been replaced

Output JSON with:
- is_outdated: boolean
- confidence: float (0-1)
- issues: list of specific problems
- suggestions: list of recommended updates`,
        processor: func(content string) []GovernanceIssue {
            // Parse LLM response and create issues
        },
    }
}

// NewDuplicateAgent finds similar/duplicate content.
func NewDuplicateAgent(llm llms.Model, threshold float64) *GovernanceAgent {
    return &GovernanceAgent{
        name: "duplicate",
        llm:  llm,
        prompt: `Compare the following wiki pages for duplicate or highly similar content.

Page 1: {{.Page1Title}}
{{.Page1Content}}

Page 2: {{.Page2Title}}
{{.Page2Content}}

Determine if these pages contain:
1. Exact duplicates
2. Overlapping information that should be merged
3. Contradictory information that needs reconciliation`,
        processor: func(content string) []GovernanceIssue {
            // Parse and create duplicate issues
        },
    }
}

// SupervisorAgent synthesizes findings from all agents.
type SupervisorAgent struct {
    llm llms.Model
}

func (s *SupervisorAgent) Synthesize(ctx context.Context, issues []GovernanceIssue, pages []schema.Document) (*GovernanceReport, error) {
    prompt := `You are a content governance supervisor. Review the following issues detected by multiple agents:

{{.Issues}}

Pages analyzed: {{.Pages}}

Create a prioritized action plan:
1. Critical issues that need immediate attention
2. Recommended remediation steps
3. Estimated impact of each issue
4. Suggested owners based on page authors`

    // Call LLM and parse response
    return &GovernanceReport{
        Issues:        issues,
        Priorities:   priorities,
        Actions:      actions,
        GeneratedAt:  time.Now(),
    }, nil
}
```

---

### 11.4 What to Move from code-warden to goframe

#### 11.4.1 MapReduce Pattern (HIGH PRIORITY)

| File in code-warden | Move to goframe | Changes |
|---------------------|-----------------|---------|
| `internal/rag/contextpkg/reduce.go` | `chains/mapreduce.go` | Generalize for any document type |
| `internal/rag/contextpkg/arch.go:generateSummaryForDirectory` | Remove (domain-specific) | Keep pattern, not implementation |
| `internal/llm/prompts/arch_summary.prompt` | Keep in code-warden | Domain-specific |

**Extraction Steps**:

1. **Create generic MapReduce interface** in goframe
2. **Keep domain-specific prompts** in code-warden
3. **Wire up** in code-warden using the new goframe chain

```go
// code-warden: Use goframe's MapReduce
import "github.com/sevigo/goframe/chains"

func (r *RAGService) GenerateProjectContext(ctx context.Context, collection string) (string, error) {
    chain := chains.NewMapReduceChain(
        chains.WithMapPrompt(archSummaryPrompt),
        chains.WithMapLLM(r.config.FastModel),
        chains.WithMapWorkers(5),
        chains.WithReducePrompt(projectContextPrompt),
        chains.WithReduceLLM(r.config.GeneratorModel),
        chains.WithStore(r.vectorStore, map[string]any{"chunk_type": "arch"}),
    )

    // Fetch summaries from store
    summaries := r.fetchArchSummaries(ctx, collection)

    return chain.Execute(ctx, summaries)
}
```

#### 11.4.2 Smart Indexing Pattern (MEDIUM PRIORITY)

| File in code-warden | Move to goframe | Changes |
|---------------------|-----------------|---------|
| `internal/rag/index/indexer.go:smartScan` | `documentloaders/smart_index.go` | Generic interface |
| `internal/rag/index/indexer.go:processBatch` | `embeddings/batch.go` | Batch processing |
| `internal/rag/index/filter.go` | `documentloaders/filter.go` | Already generic |

**Extraction Steps**:

1. Create `SmartIndexer` interface in goframe
2. Implement for Git (code-warden) and Confluence (wiki-warden)
3. Add hash-based change detection

```go
// goframe/documentloaders/smart_index.go
type SmartIndexer interface {
    // Index performs incremental indexing with change detection
    Index(ctx context.Context, loader Loader, store VectorStore) (*IndexResult, error)

    // Reindex forces full reindex
    Reindex(ctx context.Context, loader Loader, store VectorStore) (*IndexResult, error)
}

// IndexResult contains indexing statistics
type IndexResult struct {
    Total       int
    New         int
    Updated     int
    Unchanged   int
    Failed      int
    Duration    time.Duration
    Errors      []error
}
```

#### 11.4.3 HyDE Pattern (ALREADY IN GOFRAME)

The HyDE (Hypothetical Document Embeddings) pattern is already in goframe:

```go
// code-warden already uses goframe's HyDE
retriever := vectorstores.NewHyDERetriever(
    baseRetriever,
    generateHypotheticalDoc,  // This function is code-warden specific
    vectorstores.WithNumGenerations(2),
)
```

For wiki-warden, just provide a different hypothetical document generator:

```go
// wiki-warden: Use goframe's HyDE with wiki-specific generator
func generateWikiHypotheticalDoc(ctx context.Context, question string) (string, error) {
    prompt := `Given the question: "{{.Question}}"

Generate a hypothetical wiki page that would answer this question.
Include relevant headings and key information that would be in such a page.`

    return llm.Call(ctx, prompt)
}

retriever := vectorstores.NewHyDERetriever(
    wikiRetriever,
    generateWikiHypotheticalDoc,
    vectorstores.WithNumGenerations(3),
)
```

---

### 11.5 Step-by-Step Implementation Plan

#### Week 1: goframe Foundation

**Day 1-2: MapReduceChain**

```bash
# Create the chain
goframe/chains/mapreduce.go
goframe/chains/mapreduce_test.go

# Test with existing code-warden
cd code-warden && go test ./internal/rag/contextpkg/...
```

**Day 3-4: MarkdownHeaderSplitter**

```bash
# Create the splitter
goframe/textsplitters/markdown.go
goframe/textsplitters/markdown_test.go

# Test with sample markdown
go test -v ./textsplitters/ -run TestMarkdown
```

**Day 5: ConfluenceLoader (Skeleton)**

```bash
# Create the loader
goframe/documentloaders/confluence.go
goframe/documentloaders/confluence_test.go
```

#### Week 2: Confluence Integration

**Day 1-2: Confluence API Client**

```bash
# Implement API methods
goframe/documentloaders/confluence.go
- fetchSpaces()
- fetchPages()
- fetchPageContent()
- extractContent()
- htmlToMarkdown()
```

**Day 3-4: Pagination & Rate Limiting**

```bash
# Add robust pagination
goframe/documentloaders/confluence.go
- fetchPageBatch() with pagination
- rate limiting
- error handling & retries
```

**Day 5: Integration Test**

```bash
# Test with real Confluence (or mock)
go test -v ./documentloaders/ -run TestConfluenceIntegration
```

#### Week 3: Demo Application

**Day 1-2: Application Skeleton**

```bash
# Create demo app
employee-knowledge-nav/
├── cmd/server/main.go
├── internal/
│   ├── config/config.go
│   ├── rag/pipeline.go
│   └── handler/chat.go
├── config.yaml
└── go.mod
```

**Day 3-4: RAG Pipeline**

```bash
# Implement core RAG
internal/rag/pipeline.go
- IngestCommand (index wiki)
- QueryCommand (answer questions)
- HybridSearch (dense + BM25)
```

**Day 5: Voice Interface**

```bash
# Add voice capability
internal/voice/whisper.go
internal/voice/tts.go
```

#### Week 4: Content Governance Agents

**Day 1-2: Freshness Agent**

```bash
# Detect outdated content
goframe/agents/freshness.go
goframe/agents/freshness_test.go
```

**Day 3-4: Duplicate Agent**

```bash
# Find duplicate content
goframe/agents/duplicate.go
goframe/agents/duplicate_test.go
```

**Day 5: Supervisor Synthesis**

```bash
# Coordinate agents
goframe/agents/supervisor.go
```

#### Week 5: Production Readiness

**Day 1-2: Langfuse Integration**

```bash
# Add observability
goframe/observability/langfuse.go
```

**Day 3-4: Deployment**

```bash
# Docker, K8s manifests
deploy/
├── Dockerfile
├── k8s/
│   ├── deployment.yaml
│   ├── service.yaml
│   └── configmap.yaml
```

**Day 5: Demo Polish**

```bash
# Final testing, documentation
README.md
docs/demo-guide.md
```

---

### 11.6 Demo Script (5 Minutes)

**Setup**: Have Confluence with sample wiki pages (or mock data).

**1. Problem Statement (30 seconds)**
> "Employees waste hours searching across wiki, handbook, Slack... Let me show how we can solve this with goframe."

**2. Live Demo (2 minutes)**
```
# Ingest wiki
curl -X POST http://localhost:8080/ingest \
  -d '{"space_keys": ["ENG", "HR", "PRODUCT"]}'

# Ask question (voice or text)
curl -X POST http://localhost:8080/chat \
  -d '{"question": "How do I request PTO?"}'

# Response with citations:
{
  "answer": "To request PTO, submit a request through Workday...",
  "citations": [
    {"title": "PTO Policy", "url": "https://wiki.company.com/pto"}
  ],
  "confidence": 0.92
}
```

**3. Content Governance (1 minute)**
```
# Run governance analysis
curl -X POST http://localhost:8080/governance/analyze

# Shows:
- 3 pages outdated (last updated > 6 months ago)
- 2 duplicate pages (85% similar content)
- 1 conflicting information (different procedures in ENG vs HR)
```

**4. Voice Interface (30 seconds)**
> *[Record audio]: "What's the process for onboarding new employees?"*
> *[Play response]*

**5. Code Walkthrough (1 minute)**
> "Here's how it works with goframe:"
```go
// MapReduce for content governance
chain := chains.NewMapReduceChain(
    chains.WithMapPrompt(analyzeFreshnessPrompt),
    chains.WithMapWorkers(10),
    chains.WithReducePrompt(synthesizeIssuesPrompt),
)

// Confluence loader
loader := documentloaders.NewConfluenceLoader(config)
docs, _ := loader.Load(ctx)

// Markdown splitting
splitter := textsplitters.NewMarkdownHeaderSplitter()
chunks := splitter.Split(docs)
```

---

### 11.7 Success Metrics for Demo

| Metric | Target | How to Measure |
|--------|--------|----------------|
| Ingestion speed | 50+ pages/min | Logging |
| Query latency | < 2s for 90% | Prometheus |
| Answer accuracy | 80%+ with citations | Manual testing |
| Governance recall | Find 90% of outdated pages | Ground truth |
| Code quality | 80%+ test coverage | `go test -cover` |
| Open source | 1 PR to goframe | GitHub |

---

### 11.8 Post-Demo Follow-up

After the interview, contribute the components back to goframe:

```bash
# Week 1-2: Polish ConfluenceLoader
cd goframe && git checkout -b feature/confluence-loader

# Week 3-4: Polish MarkdownHeaderSplitter
git checkout -b feature/markdown-splitter

# Week 5-6: Polish MapReduceChain
git checkout -b feature/mapreduce-chain

# Submit PRs with:
# - Comprehensive tests
# - Documentation
# - Examples
```

This demonstrates:
- Open source contribution
- Production-ready code
- Collaboration skills
- Long-term thinking