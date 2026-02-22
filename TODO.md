# GoFrame Development Roadmap

A comprehensive development roadmap for the GoFrame RAG/Chain framework, organized by priority and category.

---

## 🎯 Code-Warden Priority Items

The following items are particularly valuable for the Code-Warden project:

### Immediate Priorities (Direct Impact)

| Section | Item | Why Important |
|---------|------|---------------|
| §11.2 | **PR Overlay System** | Core feature - PR changes without corrupting main index |
| §11.4 | **Smart Incremental Indexing** | Performance - git diff tracking with PostgreSQL |
| §11.5 | **Token-Aware Context Packing** | "Zero-Hallucination" goal - strict token budgets |
| §11.10 | **Structured Output Parsing** | JSON/XML parsing for review comments |
| §2.2 | **Count API** | Progress tracking during indexing |
| §2.3 | **Groups API** | Group results by file (avoid over-representation) |

### High Value Features

| Section | Item | Why Important |
|---------|------|---------------|
| §11.1 | **Multi-Stage Retrieval** | 5-stage pipeline support |
| §11.3 | **Consensus Review** | Multi-model review synthesis |
| §11.6 | **Chunk Splicing** | Continuous logic flows |
| §11.7 | **Reverse HyDE** | Better query-code alignment |
| §10.2 | **Call Graph Integration** | Impact analysis |
| §10.3 | **Code-Aware Reranking** | Exact match boosting |

### Supporting Features

| Section | Item | Why Important |
|---------|------|---------------|
| §11.8 | **GitHub Integration** | PR diff parsing, API helpers |
| §11.9 | **Hallucination Detection** | Zero-hallucination verification |
| §10.4 | **Query Understanding** | Parse code-related queries |
| §10.7 | **Code Review Pipeline** | Automated review workflow |
| §3.1 | **List Models/Model Info** | Model selection for consensus |
| §3.2 | **JSON Mode** | Reliable structured output |

---

## Table of Contents

1. [Architecture Improvements](#1-architecture-improvements)
2. [Qdrant Vector Store Features](#2-qdrant-vector-store-features)
3. [Ollama LLM Features](#3-ollama-llm-features)
4. [Testing & Quality](#4-testing--quality)
5. [Observability & Monitoring](#5-observability--monitoring)
6. [Performance Optimizations](#6-performance-optimizations)
7. [Developer Experience](#7-developer-experience)
8. [Documentation](#8-documentation)
9. [Future Considerations](#9-future-considerations)

---

## 1. Architecture Improvements

### 1.1 Connection Lifecycle Management

**Priority:** High | **Effort:** Low

Currently, only Qdrant has a `Close()` method. Standardize resource cleanup across all clients.

```go
// Proposed interface
type Closer interface {
    Close() error
}

// Implement for:
// - qdrant.Store (DONE)
// - ollama.LLM (TODO)
// - gemini.LLM (TODO)
// - fastapi.Embedder (TODO)
```

**Tasks:**
- [ ] Add `Close()` to `ollama.LLM` - cleanup HTTP connections
- [ ] Add `Close()` to `gemini.LLM` - cleanup gRPC connection
- [ ] Add `Close()` to `fastapi.Embedder` - cleanup HTTP client
- [ ] Document lifecycle management in README

### 1.2 Context Timeout Configuration

**Priority:** High | **Effort:** Medium

Add configurable default timeouts to prevent hanging operations.

```go
// Proposed configuration
type TimeoutConfig struct {
    DefaultTimeout   time.Duration // General operations
    EmbedTimeout     time.Duration // Embedding generation
    SearchTimeout    time.Duration // Vector search
    LLMTimeout       time.Duration // LLM generation
    StreamingTimeout time.Duration // Streaming operations
}

// Usage
store, err := qdrant.New(
    qdrant.WithTimeouts(qdrant.TimeoutConfig{
        SearchTimeout: 30 * time.Second,
    }),
)
```

**Tasks:**
- [ ] Define `TimeoutConfig` struct in each package
- [ ] Add `WithTimeouts()` option to Qdrant Store
- [ ] Add timeout options to Ollama client
- [ ] Add timeout options to Gemini client
- [ ] Ensure all context-aware operations respect timeouts

### 1.3 Retry/Resilience Layer

**Priority:** Medium | **Effort:** Medium

Extract and standardize retry logic from Qdrant into a reusable component.

```go
// Proposed retry package
package retry

type Config struct {
    MaxAttempts  int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Jitter       time.Duration
    Retryable    func(error) bool
}

func Do(ctx context.Context, cfg Config, fn func() error) error

// Usage in Qdrant
err := retry.Do(ctx, s.retryConfig, func() error {
    return s.client.Upsert(ctx, points)
})
```

**Tasks:**
- [ ] Create `pkg/retry` package with configurable retry logic
- [ ] Refactor Qdrant to use the new retry package
- [ ] Add retry support to Ollama client (for transient HTTP errors)
- [ ] Add retry support to FastAPI embedder
- [ ] Document retry configuration options

### 1.4 Structured Error Types

**Priority:** Medium | **Effort:** Low

Add error codes and structured errors for better error handling.

```go
// Proposed error types
package errors

type ErrorCode string

const (
    ErrCodeNotFound      ErrorCode = "not_found"
    ErrCodeTimeout       ErrorCode = "timeout"
    ErrCodeRateLimit     ErrorCode = "rate_limit"
    ErrCodeInvalidInput  ErrorCode = "invalid_input"
    ErrCodeUnavailable   ErrorCode = "unavailable"
    ErrCodeUnauthorized  ErrorCode = "unauthorized"
)

type FrameworkError struct {
    Code    ErrorCode
    Message string
    Cause   error
    Details map[string]any
}

func (e *FrameworkError) Error() string
func (e *FrameworkError) Unwrap() error
func IsCode(err error, code ErrorCode) bool
```

**Tasks:**
- [ ] Create `errors` package with structured error types
- [ ] Refactor Qdrant errors to use structured types
- [ ] Refactor Ollama errors to use structured types
- [ ] Add error wrapping throughout the codebase
- [ ] Document error handling patterns

### 1.5 Interface Abstraction for Testing

**Priority:** Medium | **Effort:** Medium

Create mock implementations for easier testing.

```go
// Already have:
// - fake.FakeLLM (llms/fake)
// - fake.FakeStore (vectorstores/fake)

// Need to add:
// - Mock implementations for Retriever interface
// - Mock implementations for Reranker interface
// - Mock implementations for ParserPlugin interface
// - Mock implementations for Tokenizer interface
```

**Tasks:**
- [ ] Add `MockRetriever` with configurable behavior
- [ ] Add `MockReranker` with configurable scores
- [ ] Add `MockParser` for testing chunking
- [ ] Add `MockTokenizer` for testing token counting
- [ ] Create test utilities package `testing/testutil`

---

## 2. Qdrant Vector Store Features

### 2.1 Scroll API (Pagination)

**Priority:** High | **Effort:** Low

Enable pagination through all points in a collection.

```go
// Proposed API
type ScrollResult struct {
    Points   []schema.Document
    NextPage []byte // Opaque cursor for next page
}

func (s *Store) Scroll(ctx context.Context, opts ...Option) (*ScrollResult, error)
func (s *Store) ScrollWithFilter(ctx context.Context, filters map[string]any, opts ...Option) (*ScrollResult, error)

// Usage
result, err := store.Scroll(ctx,
    qdrant.WithLimit(100),
    qdrant.WithOffset(0),
)
// Iterate through all documents
for result.NextPage != nil {
    result, err = store.Scroll(ctx, qdrant.WithPageToken(result.NextPage))
}
```

**Use Cases:**
- Re-indexing documents
- Bulk operations
- Exporting data
- Debugging

**Tasks:**
- [ ] Implement `Scroll()` method in Qdrant Store
- [ ] Add `WithLimit()`, `WithOffset()`, `WithPageToken()` options
- [ ] Add filter support for scroll
- [ ] Add tests for pagination
- [ ] Document scroll usage patterns

### 2.2 Count API

**Priority:** High | **Effort:** Low

Count documents matching a filter.

```go
// Proposed API
func (s *Store) Count(ctx context.Context, opts ...Option) (int64, error)
func (s *Store) CountWithFilter(ctx context.Context, filters map[string]any) (int64, error)

// Usage
total, err := store.Count(ctx)
goCount, err := store.CountWithFilter(ctx, map[string]any{"language": "go"})
```

**Use Cases:**
- Progress tracking during indexing
- Statistics dashboards
- Validation

**Tasks:**
- [ ] Implement `Count()` method in Qdrant Store
- [ ] Add filter support
- [ ] Add tests
- [ ] Document usage

### 2.3 Groups API (Grouped Search)

**Priority:** High | **Effort:** Medium

Group search results by a metadata field.

```go
// Proposed API
type GroupedResult struct {
    GroupKey string              // e.g., "main.go"
    Hits     []schema.Document   // Documents in this group
    Score    float32             // Best score in group
}

func (s *Store) SearchGroups(ctx context.Context, query string, groupBy string, opts ...Option) ([]GroupedResult, error)

// Usage
// Get 3 chunks per file, max 10 files
results, err := store.SearchGroups(ctx, query, "source",
    qdrant.WithLimit(10),
    qdrant.WithGroupSize(3),
)
```

**Use Cases:**
- "Show me implementations across different files"
- Avoid over-representation from single files
- Better code search UX

**Tasks:**
- [ ] Implement `SearchGroups()` method
- [ ] Add `WithGroupBy()`, `WithGroupSize()` options
- [ ] Add tests for grouping
- [ ] Document grouped search patterns

### 2.4 Lookup by ID

**Priority:** Medium | **Effort:** Low

Retrieve documents by their ID directly.

```go
// Proposed API
func (s *Store) GetByID(ctx context.Context, id string, opts ...Option) (*schema.Document, error)
func (s *Store) GetByIDs(ctx context.Context, ids []string, opts ...Option) ([]schema.Document, error)

// Usage
doc, err := store.GetByID(ctx, "doc-uuid-123")
docs, err := store.GetByIDs(ctx, []string{"id1", "id2", "id3"})
```

**Use Cases:**
- Document preview
- Updating single documents
- Reference resolution

**Tasks:**
- [ ] Implement `GetByID()` method
- [ ] Implement `GetByIDs()` batch method
- [ ] Add tests
- [ ] Document usage

### 2.5 Recommendation API

**Priority:** Medium | **Effort:** Medium

Find similar points using reference point IDs.

```go
// Proposed API
func (s *Store) Recommend(ctx context.Context, positiveIDs []string, negativeIDs []string, numDocuments int, opts ...Option) ([]schema.Document, error)

// Usage
// "More like this document"
similar, err := store.Recommend(ctx, []string{"doc-123"}, nil, 10)

// "More like this but NOT like that"
refined, err := store.Recommend(ctx,
    []string{"good-doc-1", "good-doc-2"},  // Positive examples
    []string{"bad-doc-1"},                  // Negative examples
    10,
)
```

**Use Cases:**
- "Find similar code" feature
- Feedback-based refinement
- Code clone detection

**Tasks:**
- [ ] Implement `Recommend()` method
- [ ] Support positive/negative examples
- [ ] Add filter support
- [ ] Add tests
- [ ] Document recommendation patterns

### 2.6 Payload Schema & Indexing

**Priority:** Medium | **Effort:** Low

Better payload index management.

```go
// Proposed API
func (s *Store) CreatePayloadIndex(ctx context.Context, fieldName string, schema PayloadSchema) error
func (s *Store) ListPayloadIndexes(ctx context.Context) ([]PayloadIndexInfo, error)
func (s *Store) DeletePayloadIndex(ctx context.Context, fieldName string) error

type PayloadSchema struct {
    Type      FieldType // Keyword, Integer, Float, Text, Geo
    OnDisk    bool
    Optimizer string
}
```

**Tasks:**
- [ ] Implement payload index management
- [ ] Support all field types (Keyword, Integer, Float, Text, Geo)
- [ ] Add tests
- [ ] Document indexing best practices

### 2.7 Quantization Options

**Priority:** Low | **Effort:** Low

Support for different quantization methods.

```go
// Current: Only binary quantization
// Proposed: Add scalar and product quantization

type QuantizationConfig struct {
    Type       QuantizationType // Binary, Scalar, Product
    CompressionRatio float64    // For product quantization
}

func WithQuantization(cfg QuantizationConfig) Option
```

**Tasks:**
- [ ] Add scalar quantization support
- [ ] Add product quantization support
- [ ] Document trade-offs between quantization types

### 2.8 Snapshot/Backup

**Priority:** Low | **Effort:** Medium

Disaster recovery support.

```go
// Proposed API
func (s *Store) CreateSnapshot(ctx context.Context, collectionName string) (*SnapshotInfo, error)
func (s *Store) ListSnapshots(ctx context.Context, collectionName string) ([]SnapshotInfo, error)
func (s *Store) RestoreSnapshot(ctx context.Context, snapshotID string) error
func (s *Store) DeleteSnapshot(ctx context.Context, snapshotID string) error
```

**Tasks:**
- [ ] Implement snapshot creation
- [ ] Implement snapshot listing
- [ ] Implement snapshot restoration
- [ ] Implement snapshot deletion
- [ ] Add tests
- [ ] Document backup/restore procedures

---

## 3. Ollama LLM Features

### 3.1 List Models & Model Info

**Priority:** High | **Effort:** Low

Query available models and their details.

```go
// Proposed API in llms/ollama

type ModelInfo struct {
    Name          string
    Size          int64
    Digest        string
    ModifiedAt    time.Time
    Family        string
    ParameterSize string
    Quantization  string
    Format        string
}

func (l *LLM) ListModels(ctx context.Context) ([]ModelInfo, error)
func (l *LLM) ShowModel(ctx context.Context, name string) (*ModelInfo, error)
func (l *LLM) DeleteModel(ctx context.Context, name string) error

// Usage
models, err := llm.ListModels(ctx)
// [{Name: "llama3:70b", Size: 42GB, Family: "llama", ...}]

info, err := llm.ShowModel(ctx, "llama3:70b")
// {Family: "llama", Parameters: "70B", Quantization: "Q4_0", ...}
```

**Use Cases:**
- Model selection UI
- Validation before use
- Display model capabilities

**Tasks:**
- [ ] Implement `ListModels()` using `GET /api/tags`
- [ ] Implement `ShowModel()` using `GET /api/show`
- [ ] Implement `DeleteModel()` using `DELETE /api/delete`
- [ ] Add tests with mock server
- [ ] Document model management

### 3.2 JSON Mode

**Priority:** High | **Effort:** Low**

Guaranteed valid JSON output.

```go
// Proposed API
func WithJSONMode(enabled bool) CallOption

// Usage
var result struct {
    Summary string   `json:"summary"`
    Issues  []string `json:"issues"`
}
resp, err := llm.Call(ctx, prompt, llms.WithJSONMode(true))
json.Unmarshal([]byte(resp), &result)
```

**Use Cases:**
- Structured output parsing
- Function calling preparation
- Reliable data extraction

**Tasks:**
- [ ] Add `WithJSONMode()` option
- [ ] Add `format: "json"` to request body
- [ ] Add tests
- [ ] Document JSON mode usage

### 3.3 Vision/Multimodal Support

**Priority:** High | **Effort:** Medium**

Support image inputs for vision models.

```go
// Proposed schema additions
type ImageContent struct {
    Data        []byte // Raw image bytes
    URL         string // Or URL to image
    MediaType   string // "image/jpeg", "image/png", etc.
}

// Usage
resp, err := llm.GenerateContent(ctx, []schema.MessageContent{
    {
        Role: schema.ChatMessageTypeHuman,
        Parts: []schema.ContentPart{
            schema.ImageContent{Data: imageData, MediaType: "image/png"},
            schema.TextContent{Text: "Explain this architecture diagram"},
        },
    },
})
```

**Use Cases:**
- Analyze code screenshots
- Read architecture diagrams
- Process UI mockups

**Tasks:**
- [ ] Add `ImageContent` type to schema package
- [ ] Update Ollama to encode images in base64
- [ ] Update Gemini to handle images
- [ ] Add tests with sample images
- [ ] Document vision capabilities

### 3.4 Keep Alive Configuration

**Priority:** Medium | **Effort:** Low**

Control model memory retention.

```go
// Proposed API
func WithKeepAlive(duration time.Duration) Option

// Usage
// Keep model loaded for 30 minutes after last request
resp, err := llm.Call(ctx, prompt, ollama.WithKeepAlive(30*time.Minute))

// Unload immediately after request
resp, err := llm.Call(ctx, prompt, ollama.WithKeepAlive(0))
```

**Use Cases:**
- Reduce latency for frequent requests
- Free GPU memory when done
- Cost optimization

**Tasks:**
- [ ] Add `WithKeepAlive()` option
- [ ] Add `keep_alive` to request body
- [ ] Add tests
- [ ] Document keep-alive tuning

### 3.5 Running Models Management

**Priority:** Medium | **Effort:** Low**

See and manage loaded models.

```go
// Proposed API
type RunningModel struct {
    Name       string
    Model      string
    Size       int64
    Digest     string
    ExpiresAt  time.Time
}

func (l *LLM) ListRunningModels(ctx context.Context) ([]RunningModel, error)

// Usage
running, err := llm.ListRunningModels(ctx)
// [{Name: "llama3:70b", Size: 42GB, ExpiresAt: ...}]
```

**Use Cases:**
- Check GPU memory usage
- Debug model loading issues
- Monitor system resources

**Tasks:**
- [ ] Implement `ListRunningModels()` using `GET /api/ps`
- [ ] Add tests
- [ ] Document running model management

### 3.6 Native Function Calling (Tools)

**Priority:** Medium | **Effort:** Medium**

Support for Ollama's native tool calling.

```go
// Proposed API
type Tool struct {
    Type     string       `json:"type"`
    Function FunctionSpec `json:"function"`
}

type FunctionSpec struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]any         `json:"parameters"`
}

func WithTools(tools []Tool) CallOption
func WithToolChoice(choice string) CallOption // "auto", "none", or specific tool

// Usage
tools := []Tool{{
    Type: "function",
    Function: FunctionSpec{
        Name:        "search_code",
        Description: "Search for code in the repository",
        Parameters:  map[string]any{...},
    },
}}

resp, err := llm.Call(ctx, prompt, ollama.WithTools(tools))
```

**Use Cases:**
- Agent workflows
- Code execution
- External API integration

**Tasks:**
- [ ] Define `Tool` and `FunctionSpec` types
- [ ] Add `WithTools()` and `WithToolChoice()` options
- [ ] Parse tool call responses
- [ ] Add tests
- [ ] Document tool calling

### 3.7 Raw Mode

**Priority:** Low | **Effort:** Low**

Bypass template formatting.

```go
// Proposed API
func WithRawMode(enabled bool) Option

// Usage - send raw prompt without template processing
resp, err := llm.Call(ctx, rawPrompt, ollama.WithRawMode(true))
```

**Use Cases:**
- Custom prompt templates
- Fine-grained control
- Advanced users

**Tasks:**
- [ ] Add `WithRawMode()` option
- [ ] Add tests
- [ ] Document usage

---

## 4. Testing & Quality

### 4.1 Unit Tests for Error Paths

**Priority:** High | **Effort:** Low**

Add tests for newly added error validation.

```
Files needing tests:
- chains/llm_chain_test.go: Test nil LLM, empty prompt
- chains/retrieval_qa_test.go: Test nil retriever, nil LLM
- vectorstores/dependency_retriever_test.go: Test nil store
- vectorstores/definition_retriever_test.go: Test nil store
```

**Tasks:**
- [ ] Add tests for `NewLLMChain` with nil parameters
- [ ] Add tests for `NewRetrievalQA` with nil parameters
- [ ] Add tests for `NewDependencyRetriever` with nil store
- [ ] Add tests for `NewDefinitionRetriever` with nil store
- [ ] Add tests for `WithConcurrency` with invalid values

### 4.2 Integration Tests with Docker

**Priority:** High | **Effort:** Medium**

Add integration tests that run against real services.

```yaml
# docker-compose.test.yml
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
```

```go
// Integration test pattern
func TestQdrantIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    // Real Qdrant test
}
```

**Tasks:**
- [ ] Create `docker-compose.test.yml`
- [ ] Add Qdrant integration tests
- [ ] Add Ollama integration tests (with mock model)
- [ ] Add GitHub Actions workflow for integration tests
- [ ] Document how to run integration tests

### 4.3 Benchmark Tests

**Priority:** Medium | **Effort:** Low**

Add performance benchmarks.

```go
// Proposed benchmarks
func BenchmarkEmbedding(b *testing.B)
func BenchmarkSearch(b *testing.B)
func BenchmarkChunking(b *testing.B)
func BenchmarkReranking(b *testing.B)
```

**Tasks:**
- [ ] Add embedding benchmarks
- [ ] Add search benchmarks
- [ ] Add chunking benchmarks
- [ ] Add end-to-end RAG benchmarks
- [ ] Add to CI with trend tracking

### 4.4 Fuzzing Tests

**Priority:** Low | **Effort:** Medium**

Add fuzzing for input validation.

```go
// Fuzz tests
func FuzzChunking(f *testing.F)
func FuzzEmbedding(f *testing.F)
func FuzzFilterBuilding(f *testing.F)
```

**Tasks:**
- [ ] Add fuzz test for code chunking
- [ ] Add fuzz test for filter building
- [ ] Add fuzz test for prompt formatting

---

## 5. Observability & Monitoring

### 5.1 Metrics Collection

**Priority:** High | **Effort:** Medium**

Add Prometheus metrics.

```go
// Proposed metrics
var (
    // Latency histograms
    embeddingLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name: "goframe_embedding_latency_seconds",
        Help: "Embedding generation latency",
    }, []string{"model"})

    searchLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name: "goframe_search_latency_seconds",
        Help: "Vector search latency",
    }, []string{"collection"})

    llmLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name: "goframe_llm_latency_seconds",
        Help: "LLM generation latency",
    }, []string{"model", "streaming"})

    // Counters
    documentsIndexed = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "goframe_documents_indexed_total",
        Help: "Total documents indexed",
    }, []string{"collection"})

    searchRequests = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "goframe_search_requests_total",
        Help: "Total search requests",
    }, []string{"collection"})

    // Error counters
    errors = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "goframe_errors_total",
        Help: "Total errors",
    }, []string{"component", "operation"})
)
```

**Tasks:**
- [ ] Create `metrics` package with Prometheus integration
- [ ] Add metrics to Qdrant Store
- [ ] Add metrics to Ollama client
- [ ] Add metrics to Gemini client
- [ ] Add metrics to embeddings
- [ ] Document metrics exposure

### 5.2 Structured Logging

**Priority:** Medium | **Effort:** Low**

Standardize structured logging.

```go
// Current: Already using slog
// Enhance with consistent fields

// Proposed log fields
const (
    FieldComponent   = "component"   // qdrant, ollama, embedder
    FieldOperation   = "operation"   // search, embed, index
    FieldDuration    = "duration_ms"
    FieldCollection  = "collection"
    FieldModel       = "model"
    FieldBatchSize   = "batch_size"
    FieldError       = "error"
)
```

**Tasks:**
- [ ] Define standard log field constants
- [ ] Review and standardize all log messages
- [ ] Add request ID tracking
- [ ] Document logging conventions

### 5.3 Tracing Support

**Priority:** Low | **Effort:** Medium**

Add OpenTelemetry tracing.

```go
// Proposed tracing
func (s *Store) SimilaritySearch(ctx context.Context, query string, numDocuments int, opts ...Option) ([]schema.Document, error) {
    ctx, span := tracer.Start(ctx, "qdrant.SimilaritySearch")
    defer span.End()

    span.SetAttributes(
        attribute.String("collection", collectionName),
        attribute.Int("num_documents", numDocuments),
    )

    // ... implementation
}
```

**Tasks:**
- [ ] Add OpenTelemetry dependency
- [ ] Add tracing to Qdrant operations
- [ ] Add tracing to LLM calls
- [ ] Add tracing to embedding generation
- [ ] Document tracing setup

---

## 6. Performance Optimizations

### 6.1 Regex Pattern Pre-compilation

**Priority:** High | **Effort:** Low**

Move regex patterns from functions to package-level variables.

```
Files affected:
- parsers/markdown/parser.go (lines 436-438, 469)
- parsers/markdown/extractor.go (lines 246-247)
- parsers/text/chunker.go (lines 143-146, 175-177)
```

**Tasks:**
- [ ] Pre-compile regex in `parsers/markdown/parser.go`
- [ ] Pre-compile regex in `parsers/markdown/extractor.go`
- [ ] Pre-compile regex in `parsers/text/chunker.go`
- [ ] Add benchmarks to verify improvement

### 6.2 Slice Pre-allocation

**Priority:** Medium | **Effort:** Low**

Pre-allocate slices in hot paths.

```
Files affected:
- schema/message.go:53-65 - MessageContent.String()
- vectorstores/qdrant/qdrant.go:1241-1258 - payloadToDocument()
- embeddings/sparse/sparse.go:85-88 - model loading
```

**Tasks:**
- [ ] Pre-allocate in `MessageContent.String()`
- [ ] Pre-allocate in `payloadToDocument()`
- [ ] Review and fix other hot paths

### 6.3 Binary Extensions Map Optimization

**Priority:** Medium | **Effort:** Low**

Move `binaryExts` map to package level in `documentloaders/git.go`.

**Tasks:**
- [ ] Move `binaryExts` map to package-level variable
- [ ] Benchmark improvement

### 6.4 TypeScript Parser JavaScript Runtime

**Priority:** Low | **Effort:** Low**

Use `sync.Once` for JavaScript runtime initialization.

**Tasks:**
- [ ] Add `sync.Once` for JS runtime in TypeScript parser
- [ ] Add test for concurrent initialization

### 6.5 Embedding Cache

**Priority:** Medium | **Effort:** Medium**

Add optional caching layer for embeddings.

```go
// Proposed API
type CachedEmbedder struct {
    embedder Embedder
    cache    Cache
}

func NewCachedEmbedder(embedder Embedder, cache Cache) *CachedEmbedder

// Cache interface
type Cache interface {
    Get(ctx context.Context, key string) ([]float32, bool)
    Set(ctx context.Context, key string, value []float32, ttl time.Duration)
}
```

**Tasks:**
- [ ] Define `Cache` interface
- [ ] Implement `CachedEmbedder`
- [ ] Add in-memory LRU cache implementation
- [ ] Add Redis cache implementation (optional)
- [ ] Add tests
- [ ] Document caching options

---

## 7. Developer Experience

### 7.1 Builder Pattern for Complex Objects

**Priority:** Medium | **Effort:** Medium**

Add fluent builders for complex configuration.

```go
// Proposed builder pattern
store := qdrant.NewStoreBuilder().
    WithURL("http://localhost:6333").
    WithCollection("my-collection").
    WithEmbedder(embedder).
    WithBatchConfig(qdrant.BatchConfig{
        BatchSize: 100,
        MaxConcurrency: 8,
    }).
    WithTimeouts(qdrant.TimeoutConfig{
        SearchTimeout: 30 * time.Second,
    }).
    Build()
```

**Tasks:**
- [ ] Add `StoreBuilder` for Qdrant
- [ ] Add `ChainBuilder` for RAG chains
- [ ] Document builder pattern usage

### 7.2 Configuration from Environment

**Priority:** Medium | **Effort:** Low**

Support environment variable configuration.

```go
// Proposed API
store, err := qdrant.NewFromEnv("QDRANT_")
// Reads: QDRANT_URL, QDRANT_COLLECTION, QDRANT_API_KEY, etc.

llm, err := ollama.NewFromEnv("OLLAMA_")
// Reads: OLLAMA_URL, OLLAMA_MODEL, OLLAMA_API_KEY, etc.
```

**Tasks:**
- [ ] Add `NewFromEnv()` to Qdrant
- [ ] Add `NewFromEnv()` to Ollama
- [ ] Add `NewFromEnv()` to Gemini
- [ ] Document environment variables

### 7.3 Example Applications

**Priority:** Medium | **Effort:** Low**

Expand example applications.

```
examples/
├── basic-rag/              # Simple RAG example
├── hybrid-search/          # Dense + sparse search
├── code-search/            # Code-specific RAG
├── multi-model/            # Multiple LLM providers
├── streaming/              # Streaming responses
├── agents/                 # Tool-calling agents
├── evaluation/             # RAG evaluation
└── production/             # Production-ready setup
```

**Tasks:**
- [ ] Create `examples/basic-rag/`
- [ ] Create `examples/agents/`
- [ ] Create `examples/production/`
- [ ] Ensure all examples are runnable
- [ ] Add README to each example

### 7.4 Error Messages Improvement

**Priority:** Medium | **Effort:** Low**

Improve error messages with actionable guidance.

```go
// Current
return nil, fmt.Errorf("failed to connect: %w", err)

// Proposed
return nil, &FrameworkError{
    Code:    ErrCodeUnavailable,
    Message: "Qdrant server is not responding",
    Cause:   err,
    Details: map[string]any{
        "url":        url,
        "suggestion": "Ensure Qdrant is running: docker run -p 6333:6333 qdrant/qdrant",
    },
}
```

**Tasks:**
- [ ] Review all error messages
- [ ] Add actionable suggestions to errors
- [ ] Document common errors and solutions

---

## 8. Documentation

### 8.1 API Reference

**Priority:** High | **Effort:** Ongoing**

Complete godoc for all packages.

```
Packages needing documentation:
- parsers/golang/ (partial)
- parsers/typescript/ (partial)
- parsers/markdown/ (partial)
- parsers/text/ (partial)
- parsers/json/ (partial)
- parsers/yaml/ (partial)
- parsers/csv/ (partial)
- parsers/terraform/ (partial)
- parsers/protobuf/ (partial)
- parsers/pdf/ (partial)
- documentloaders/ (partial)
- gitutil/ (partial)
- embeddings/sparse/ (partial)
- embeddings/fastapi/ (partial)
- llms/ollama/ (partial)
- llms/gemini/ (partial)
- llms/fake/ (partial)
```

**Tasks:**
- [ ] Complete godoc for all parser packages
- [ ] Complete godoc for documentloaders
- [ ] Complete godoc for embeddings packages
- [ ] Complete godoc for LLM packages
- [ ] Ensure pkg.go.dev renders correctly

### 8.2 README Enhancement

**Priority:** High | **Effort:** Low**

Improve main README.

```
Add:
- Badges (godoc, go report, license, CI status)
- Quick start section with minimal example
- Configuration options table
- Error handling patterns
- Thread safety notes
- Links to examples
- Link to RAG_PATTERNS.md
- Contributing section
```

**Tasks:**
- [ ] Add badges to README
- [ ] Add quick start example
- [ ] Add configuration tables
- [ ] Add error handling section
- [ ] Add thread safety documentation

### 8.3 Architecture Decision Records

**Priority:** Medium | **Effort:** Medium**

Document key architectural decisions.

```
docs/adr/
├── 001-vector-store-interface.md
├── 002-embedding-abstraction.md
├── 003-parser-plugin-system.md
├── 004-hybrid-search.md
├── 005-error-handling.md
└── 006-configuration-patterns.md
```

**Tasks:**
- [ ] Create ADR directory
- [ ] Write ADR for vector store interface
- [ ] Write ADR for parser plugin system
- [ ] Write ADR for hybrid search design

### 8.4 Performance Guide

**Priority:** Medium | **Effort:** Medium**

Document performance tuning.

```
docs/performance.md:
- Batch size recommendations
- Concurrency tuning
- Memory management
- Qdrant optimization
- Embedding caching
- Quantization trade-offs
```

**Tasks:**
- [ ] Create performance documentation
- [ ] Add benchmark results
- [ ] Add tuning recommendations

---

## 9. Future Considerations

### 9.1 Streaming RAG

**Priority:** Medium | **Effort:** Medium**

Stream LLM responses as they're generated.

```go
// Proposed API
func (c *RetrievalQA) CallStream(ctx context.Context, query string, callback func(chunk string) error) error
```

**Tasks:**
- [ ] Implement streaming in RetrievalQA
- [ ] Implement streaming in LLMChain
- [ ] Add tests
- [ ] Document streaming usage

### 9.2 Multi-Vector Store Support

**Priority:** Low | **Effort:** High**

Support multiple vector stores simultaneously.

```go
// Proposed API
type MultiStore struct {
    stores []VectorStore
    strategy RoutingStrategy
}

func (m *MultiStore) SimilaritySearch(ctx context.Context, query string, numDocuments int, opts ...Option) ([]schema.Document, error)
```

**Tasks:**
- [ ] Design multi-store interface
- [ ] Implement routing strategies (round-robin, latency-based, etc.)
- [ ] Add tests
- [ ] Document usage

### 9.3 Reranking Improvements

**Priority:** Medium | **Effort:** Medium**

Add more reranking options.

```go
// Proposed
type CrossEncoderReranker struct { ... }  // Use cross-encoder models
type ColbertReranker struct { ... }        // Use ColBERT
type HybridReranker struct { ... }         // Combine multiple rerankers
```

**Tasks:**
- [ ] Implement CrossEncoderReranker
- [ ] Implement HybridReranker
- [ ] Add tests
- [ ] Document reranking options

### 9.4 Evaluation Framework

**Priority:** Medium | **Effort:** High**

Built-in RAG evaluation.

```go
// Proposed API
type RAGEvaluator struct { ... }

func (e *RAGEvaluator) Evaluate(ctx context.Context, testCases []TestCase) EvaluationReport

type EvaluationReport struct {
    RetrievalMetrics  RetrievalMetrics  // Precision, Recall, MRR
    GenerationMetrics GenerationMetrics  // Faithfulness, Relevance
    LatencyMetrics    LatencyMetrics    // P50, P95, P99
}
```

**Tasks:**
- [ ] Design evaluation framework
- [ ] Implement retrieval metrics
- [ ] Implement generation metrics
- [ ] Add tests
- [ ] Document evaluation process

### 9.5 Vector Store Abstraction

**Priority:** Low | **Effort:** High**

Add support for more vector databases.

```
Potential additions:
- Weaviate
- Pinecone
- Milvus
- Chroma
- pgvector
```

**Tasks:**
- [ ] Design store interface extensions if needed
- [ ] Implement additional stores as needed

---

## 10. Advanced RAG Techniques for Code

This section covers advanced techniques specifically designed for code understanding, code reviews, and code FAQ systems.

### 10.1 Multi-Granularity Code Indexing

**Priority:** High | **Effort:** Medium**

Index code at multiple levels of granularity for better retrieval.

```go
// Proposed API
type CodeIndexConfig struct {
    // Index levels
    IndexFunctions    bool // Function/method level
    IndexClasses      bool // Class/struct level
    IndexFiles        bool // File level summaries
    IndexPackages     bool // Package/module level
    IndexCrossRefs    bool // Cross-references between entities
}

// Store multiple representations
type CodeDocument struct {
    ID              string
    Content         string
    Granularity     GranularityLevel // function, class, file, package
    ParentID        string            // ID of containing entity
    ChildrenIDs     []string          // IDs of contained entities
    Dependencies    []string          // Import dependencies
    CallGraph       []string          // Function calls
    Signature       string            // Function signature
    DocComment      string            // Documentation
    CodeEmbedding   []float32         // Dense embedding
    SparseVector    *SparseVector     // Sparse embedding (keywords)
}
```

**Use Cases:**
- Retrieve function implementations + class context
- Find related functions across files
- Understand code structure at multiple levels

**Tasks:**
- [ ] Design multi-level document schema
- [ ] Implement hierarchical chunking
- [ ] Add parent-child relationship storage
- [ ] Update search to use hierarchy
- [ ] Add tests

### 10.2 Call Graph and Dependency Graph Integration

**Priority:** High | **Effort:** High**

Build and query code relationship graphs.

```go
// Proposed API
type CodeGraph struct {
    nodes map[string]*CodeNode
    edges []CodeEdge
}

type CodeNode struct {
    ID           string
    Type         NodeType // function, class, file, package
    Name         string
    File         string
    StartLine    int
    EndLine      int
    Embedding    []float32
}

type CodeEdge struct {
    From         string
    To           string
    Type         EdgeType // calls, imports, extends, implements
    Weight       float32
}

// Graph queries
func (g *CodeGraph) GetCallers(functionID string) []*CodeNode
func (g *CodeGraph) GetCallees(functionID string) []*CodeNode
func (g *CodeGraph) GetDependencies(fileID string) []*CodeNode
func (g *CodeGraph) GetDependents(fileID string) []*CodeNode
func (g *CodeGraph) GetImplementations(interfaceID string) []*CodeNode
func (g *CodeGraph) FindPath(from, to string) []*CodeEdge
func (g *CodeGraph) GetContextWindow(nodeID string, depth int) []*CodeNode
```

**Use Cases:**
- "What functions call this function?" (impact analysis)
- "What does this function depend on?" (dependency analysis)
- "Show me the call chain that leads here" (trace analysis)
- "What files would be affected by this change?" (change impact)

**Tasks:**
- [ ] Design graph schema
- [ ] Implement call graph extraction (Go, TypeScript)
- [ ] Implement dependency graph extraction
- [ ] Add graph storage layer
- [ ] Implement graph queries
- [ ] Integrate with vector search for hybrid retrieval
- [ ] Add tests

### 10.3 Code-Aware Reranking

**Priority:** High | **Effort:** Medium**

Specialized reranking for code search results.

```go
// Proposed API
type CodeReranker struct {
    // Scoring factors
    SemanticWeight    float32 // Semantic similarity
    ExactMatchWeight  float32 // Exact identifier matches
    ContextWeight     float32 // Contextual relevance
    RecencyWeight     float32 // Git recency
    AuthorWeight      float32 // Author expertise
}

type CodeRerankOptions struct {
    QueryLanguage     string   // Filter by language
    QueryType         QueryType // function, class, usage
    PreferExactMatch  bool     // Boost exact matches
    IncludeContext    bool     // Include surrounding code
    MaxContextLines   int      // Lines of context
}

func (r *CodeReranker) Rerank(ctx context.Context, query string, docs []CodeDocument, opts CodeRerankOptions) ([]ScoredDocument, error)

// Scoring methods
func (r *CodeReranker) scoreExactMatch(query string, doc CodeDocument) float32
func (r *CodeReranker) scoreContextRelevance(query string, doc CodeDocument) float32
func (r *CodeReranker) scoreGitRecency(doc CodeDocument) float32
```

**Use Cases:**
- Exact identifier matches should rank higher
- Recent code changes might be more relevant
- Code in the same package has higher context relevance

**Tasks:**
- [ ] Implement exact match scoring
- [ ] Implement contextual relevance scoring
- [ ] Implement git recency scoring
- [ ] Add language-specific boosting
- [ ] Add tests

### 10.4 Query Understanding for Code

**Priority:** Medium | **Effort:** Medium**

Parse and understand code-related queries.

```go
// Proposed API
type CodeQueryParser struct {
    llm llms.Model
}

type ParsedCodeQuery struct {
    OriginalQuery     string
    QueryType         QueryType      // definition, usage, example, explanation
    Entities          []CodeEntity   // Mentioned functions, classes, packages
    Languages         []string       // Mentioned or inferred languages
    Intent            QueryIntent    // find, understand, fix, improve
    Constraints       []Constraint   // "in Go", "in tests", "recent"
    ExpandedQueries   []string       // Query expansions for better recall
}

type CodeEntity struct {
    Name         string
    Type         EntityType // function, class, variable, package
    Context      string     // Package/class context
    Language     string
}

type QueryType string
const (
    QueryTypeDefinition   QueryType = "definition"   // "What is function X?"
    QueryTypeUsage        QueryType = "usage"        // "How do I use X?"
    QueryTypeExample      QueryType = "example"      // "Show me an example of X"
    QueryTypeExplanation  QueryType = "explanation"  // "Explain how X works"
    QueryTypeFix          QueryType = "fix"          // "How do I fix X?"
    QueryTypeCompare      QueryType = "compare"      // "What's the difference between X and Y?"
    QueryTypeImpact       QueryType = "impact"       // "What does X affect?"
)

func (p *CodeQueryParser) Parse(ctx context.Context, query string) (*ParsedCodeQuery, error)
```

**Use Cases:**
- "What does `SimilaritySearch` do?" → definition query
- "How do I use the Qdrant client?" → usage query
- "Show me examples of RAG chains" → example query
- "What files import `parser.go`?" → impact query

**Tasks:**
- [ ] Design query parsing schema
- [ ] Implement entity extraction
- [ ] Implement intent classification
- [ ] Add query expansion
- [ ] Integrate with retrieval pipeline
- [ ] Add tests

### 10.5 Code Context Expansion

**Priority:** Medium | **Effort:** Medium**

Automatically expand retrieved code with relevant context.

```go
// Proposed API
type ContextExpander struct {
    store       VectorStore
    graph       *CodeGraph
    maxTokens   int
}

type ExpansionConfig struct {
    IncludeImports     bool    // Include import statements
    IncludeSignature   bool    // Include function signatures
    IncludeDocComments bool    // Include doc comments
    IncludeCallers     bool    // Include calling functions
    IncludeCallees     bool    // Include called functions
    IncludeClass       bool    // Include containing class
    IncludeTests       bool    // Include related tests
    MaxExpansionTokens int     // Token budget for expansion
}

func (e *ContextExpander) Expand(ctx context.Context, doc CodeDocument, config ExpansionConfig) (*ExpandedContext, error)

type ExpandedContext struct {
    Primary         string     // Original code
    Imports         string     // Import statements
    Definitions     []string   // Referenced definitions
    Context         string     // Surrounding context
    Tests           []string   // Related tests
    RelatedFiles    []string   // Related files
    TotalTokens     int        // Total token count
}
```

**Use Cases:**
- When showing a function, also show its imports
- When showing a class method, show the class definition
- When showing code, include related test code
- Provide complete context for LLM understanding

**Tasks:**
- [ ] Implement import extraction
- [ ] Implement signature extraction
- [ ] Implement caller/callee expansion
- [ ] Implement test association
- [ ] Add token budget management
- [ ] Add tests

### 10.6 Self-Querying RAG for Code

**Priority:** Medium | **Effort:** High**

Let the system generate its own search queries.

```go
// Proposed API
type SelfQueryingRetriever struct {
    llm         llms.Model
    store       VectorStore
    maxQueries  int
}

type QueryPlan struct {
    OriginalQuery    string
    SubQueries       []SubQuery
    SynthesisPlan    SynthesisPlan
}

type SubQuery struct {
    Query          string
    Filters        map[string]any
    QueryType      QueryType
    Priority       int
}

type SynthesisPlan struct {
    Strategy       string    // "merge", "compare", "synthesize"
    Template       string    // Synthesis prompt template
}

func (r *SelfQueryingRetriever) Plan(ctx context.Context, query string) (*QueryPlan, error)
func (r *SelfQueryingRetriever) Execute(ctx context.Context, plan *QueryPlan) ([]schema.Document, error)

// Example flow:
// User: "Compare the error handling in Qdrant and Ollama clients"
// System generates:
//   1. Search: "error handling" + filters:{package: "qdrant"}
//   2. Search: "error handling" + filters:{package: "ollama"}
//   3. Synthesis: Compare the two result sets
```

**Use Cases:**
- Complex multi-part questions
- Comparative queries
- Queries requiring multiple search strategies

**Tasks:**
- [ ] Design query planning schema
- [ ] Implement query decomposition
- [ ] Implement filter generation
- [ ] Implement result synthesis
- [ ] Add tests

### 10.7 Code Review Assistant

**Priority:** High | **Effort:** High**

Specialized pipeline for code reviews.

```go
// Proposed API
type CodeReviewPipeline struct {
    retriever    Retriever
    llm          llms.Model
    gitClient    GitClient
}

type ReviewContext struct {
    PR              PullRequest
    ChangedFiles    []ChangedFile
    HistoricalPRs   []PullRequest      // Similar past PRs
    RelatedCode     []CodeDocument     // Related code in codebase
    AuthorHistory   []Commit           // Author's past changes
    BlameInfo       []BlameLine        // Git blame for context
}

type ReviewComment struct {
    File          string
    Line          int
    Severity      Severity    // info, warning, error
    Category      string      // style, bug, performance, security
    Message       string
    Suggestion    string      // Code suggestion
    Confidence    float32
    References    []string    // Supporting references
}

type ReviewResult struct {
    Summary       string
    Comments      []ReviewComment
    OverallScore  float32
    Checklist     []ChecklistItem
}

func (p *CodeReviewPipeline) Review(ctx context.Context, pr PullRequest) (*ReviewResult, error)

// Review stages
func (p *CodeReviewPipeline) gatherContext(ctx context.Context, pr PullRequest) (*ReviewContext, error)
func (p *CodeReviewPipeline) detectIssues(ctx context.Context, diff string, context *ReviewContext) ([]ReviewComment, error)
func (p *CodeReviewPipeline) findSimilarPRs(ctx context.Context, pr PullRequest) ([]PullRequest, error)
func (p *CodeReviewPipeline) generateSuggestions(ctx context.Context, comments []ReviewComment) error
```

**Use Cases:**
- Automated code review comments
- Finding similar PRs that introduced bugs
- Style consistency checking
- Security vulnerability detection
- Performance regression detection

**Tasks:**
- [ ] Design review pipeline architecture
- [ ] Implement git integration
- [ ] Implement diff parsing
- [ ] Implement context gathering
- [ ] Implement issue detection
- [ ] Implement suggestion generation
- [ ] Add tests with real PRs

### 10.8 Semantic Code Search with Filters

**Priority:** Medium | **Effort:** Low**

Enhanced search with semantic code filters.

```go
// Proposed API
type SemanticCodeFilter struct {
    Languages       []string   // Go, TypeScript, Python
    FilePatterns    []string   // Glob patterns
    Authors         []string   // Git authors
    DateRange       DateRange  // Commit date range
    CodeTypes       []string   // function, class, interface, test
    Visibility      []string   // public, private
    MinComplexity   int        // Cyclomatic complexity
    HasTests        *bool      // Has associated tests
    HasComments     *bool      // Has doc comments
    ModifiedIn      []string   // Branch names
}

func (s *Store) SemanticCodeSearch(ctx context.Context, query string, filters SemanticCodeFilter, numResults int) ([]CodeDocument, error)

// Example usage:
results, err := store.SemanticCodeSearch(ctx,
    "error handling pattern",
    SemanticCodeFilter{
        Languages:   []string{"go"},
        CodeTypes:   []string{"function"},
        Visibility:  []string{"public"},
        HasComments: boolPtr(true),
    },
    10,
)
```

**Use Cases:**
- "Show me public Go functions with error handling"
- "Find all tests that test authentication"
- "Find code modified in the last month"

**Tasks:**
- [ ] Design filter schema
- [ ] Implement language filter
- [ ] Implement code type filter
- [ ] Implement complexity filter
- [ ] Implement git-based filters
- [ ] Add tests

### 10.9 Code FAQ System

**Priority:** High | **Effort:** Medium**

Build a self-learning FAQ system from codebase.

```go
// Proposed API
type CodeFAQSystem struct {
    store       VectorStore
    llm         llms.Model
    faqStore    FAQStore
}

type FAQEntry struct {
    ID              string
    Question        string
    Answer          string
    CodeReferences  []CodeReference
    Tags            []string
    Confidence      float32
    Source          string      // "user", "generated", "curated"
    LastUpdated     time.Time
    AccessCount     int
    HelpfulVotes    int
}

type CodeReference struct {
    File        string
    LineStart   int
    LineEnd     int
    Code        string
    Description string
}

func (f *CodeFAQSystem) Ask(ctx context.Context, question string) (*FAQAnswer, error)
func (f *CodeFAQSystem) Learn(ctx context.Context, question, answer string, refs []CodeReference) error
func (f *CodeFAQSystem) GenerateFAQs(ctx context.Context, codebase []CodeDocument) ([]FAQEntry, error)
func (f *CodeFAQSystem) FindSimilarFAQs(ctx context.Context, question string) ([]FAQEntry, error)

type FAQAnswer struct {
    Answer          string
    CodeReferences  []CodeReference
    RelatedFAQs     []FAQEntry
    Confidence      float32
    WasFromCache    bool
}
```

**Features:**
- Learn from user Q&A pairs
- Auto-generate FAQs from code comments
- Cache frequent questions
- Track question patterns

**Tasks:**
- [ ] Design FAQ schema
- [ ] Implement FAQ storage
- [ ] Implement question similarity
- [ ] Implement FAQ generation from code
- [ ] Implement learning from feedback
- [ ] Add tests

### 10.10 Incremental Indexing

**Priority:** Medium | **Effort:** Medium**

Efficiently update the index as code changes.

```go
// Proposed API
type IncrementalIndexer struct {
    store       VectorStore
    gitClient   GitClient
    parser      ParserPlugin
    stateStore  StateStore
}

type IndexState struct {
    LastCommitHash   string
    IndexedFiles     map[string]FileState
    LastIndexed      time.Time
}

type FileState struct {
    Path         string
    Hash         string    // Content hash
    LastModified time.Time
    ChunkIDs     []string  // Stored chunk IDs
}

type IndexDiff struct {
    Added    []string
    Modified []string
    Deleted  []string
}

func (i *IncrementalIndexer) Sync(ctx context.Context, repoPath string) (*IndexStats, error)
func (i *IncrementalIndexer) GetDiff(ctx context.Context, repoPath string) (*IndexDiff, error)
func (i *IncrementalIndexer) IndexCommit(ctx context.Context, commitHash string) error
func (i *IncrementalIndexer) IndexFiles(ctx context.Context, files []string) error
func (i *IncrementalIndexer) RemoveFiles(ctx context.Context, files []string) error

type IndexStats struct {
    FilesProcessed   int
    ChunksAdded      int
    ChunksUpdated    int
    ChunksDeleted    int
    TimeElapsed      time.Duration
}
```

**Use Cases:**
- CI/CD integration for automatic indexing
- Large codebases where full reindex is expensive
- Real-time code search during development

**Tasks:**
- [ ] Design state storage schema
- [ ] Implement git diff detection
- [ ] Implement incremental updates
- [ ] Implement deletion handling
- [ ] Add tests

### 10.11 Code Embedding Strategies

**Priority:** Medium | **Effort:** Medium**

Multiple embedding strategies optimized for code.

```go
// Proposed API
type CodeEmbeddingStrategy interface {
    Embed(ctx context.Context, code CodeDocument) (*EmbeddingResult, error)
    EmbedBatch(ctx context.Context, codes []CodeDocument) ([]EmbeddingResult, error)
}

type EmbeddingResult struct {
    Dense          []float32       // Dense embedding
    Sparse         *SparseVector   // Sparse embedding (keywords)
    Structural     []float32       // Structural embedding (AST)
    Semantic       []float32       // Semantic embedding
}

// Strategy 1: Code-specific models
type CodeBERTEmbedder struct { ... }  // CodeBERT, GraphCodeBERT
type StarCoderEmbedder struct { ... } // StarCoder embeddings

// Strategy 2: Structure-aware embedding
type StructuralEmbedder struct {
    embedder   Embedder
    options    StructuralOptions
}
type StructuralOptions struct {
    IncludeImports     bool
    IncludeSignature   bool
    IncludeDocComments bool
    IncludeContext     int  // Lines of context
}

// Strategy 3: Multi-vector embedding
type MultiVectorEmbedder struct {
    strategies []CodeEmbeddingStrategy
}
// Creates multiple embeddings per code chunk for different aspects

// Strategy 4: Contrastive embedding
type ContrastiveEmbedder struct {
    embedder   Embedder
    augmenter  CodeAugmenter
}
// Uses code augmentation for contrastive learning
```

**Use Cases:**
- Code-specific models understand syntax better
- Structure-aware embeddings capture code structure
- Multi-vector embeddings improve recall

**Tasks:**
- [ ] Research best code embedding models
- [ ] Implement structure-aware embedding
- [ ] Implement multi-vector strategy
- [ ] Add embedding evaluation
- [ ] Add tests

### 10.12 Code Generation from Context

**Priority:** Low | **Effort:** High**

Generate code suggestions from retrieved context.

```go
// Proposed API
type CodeGenerator struct {
    llm         llms.Model
    retriever   Retriever
}

type GenerationContext struct {
    Query           string
    Language        string
    StyleGuide      string
    RelatedCode     []CodeDocument
    Imports         []string
    TypeContext     []TypeDefinition
    TestExamples    []string
}

type CodeSuggestion struct {
    Code            string
    Explanation     string
    Confidence      float32
    References      []CodeReference
    AlternativeCode []string    // Alternative implementations
}

func (g *CodeGenerator) GenerateFunction(ctx context.Context, spec FunctionSpec) (*CodeSuggestion, error)
func (g *CodeGenerator) GenerateTest(ctx context.Context, functionCode string) (*CodeSuggestion, error)
func (g *CodeGenerator) GenerateDocs(ctx context.Context, code string) (string, error)
func (g *CodeGenerator) RefactorCode(ctx context.Context, code string, goal string) (*CodeSuggestion, error)
func (g *CodeGenerator) FixCode(ctx context.Context, code string, error string) (*CodeSuggestion, error)

// Context gathering for generation
func (g *CodeGenerator) gatherGenerationContext(ctx context.Context, query string, lang string) (*GenerationContext, error)
```

**Use Cases:**
- Generate function implementations from specs
- Generate tests for existing code
- Generate documentation
- Suggest refactoring

**Tasks:**
- [ ] Design generation pipeline
- [ ] Implement context gathering
- [ ] Implement function generation
- [ ] Implement test generation
- [ ] Implement doc generation
- [ ] Add tests

---

## 11. Code-Warden Specific Enhancements

Features specifically designed for the Code-Warden GitHub review agent and RAG-based code assistant.

### 11.1 Multi-Stage Retrieval Pipeline Support

**Priority:** High | **Effort:** Medium**

Built-in support for Code-Warden's 5-stage retrieval pipeline.

```go
// Proposed API
type MultiStageRetriever struct {
    store       VectorStore
    stages      []RetrievalStage
    merger      ResultMerger
}

type RetrievalStage struct {
    Name        string
    Retriever   Retriever
    Weight      float32
    MaxResults  int
}

// Pre-built stages for Code-Warden
func NewArchitecturalContextStage(store VectorStore) RetrievalStage
func NewHyDERetrieverStage(llm llms.Model, store VectorStore) RetrievalStage
func NewImpactAnalysisStage(graph *CodeGraph, store VectorStore) RetrievalStage
func NewMultiQueryStage(llm llms.Model, store VectorStore) RetrievalStage
func NewSymbolDefinitionStage(graph *CodeGraph, store VectorStore, depth int) RetrievalStage

func (r *MultiStageRetriever) Retrieve(ctx context.Context, query string) (*MultiStageResult, error)

type MultiStageResult struct {
    Documents   []ScoredDocument
    StageResults map[string][]ScoredDocument  // Results per stage
    Metadata    map[string]any
}
```

**Tasks:**
- [ ] Implement `MultiStageRetriever`
- [ ] Implement HyDE retriever stage
- [ ] Implement Impact Analysis stage
- [ ] Implement MultiQuery retriever stage
- [ ] Implement Symbol Definition stage (Graph-RAG Lite)
- [ ] Add result fusion/merging strategies
- [ ] Add tests

### 11.2 PR Overlay System

**Priority:** High | **Effort:** High**

Ephemeral overlay system for PR changes without corrupting main branch index.

```go
// Proposed API
type PROverlayStore struct {
    baseStore   VectorStore    // Main branch store (immutable)
    overlays    map[string]*OverlayLayer
    mu          sync.RWMutex
}

type OverlayLayer struct {
    PRNumber    int
    BaseSHA     string
    Added       []schema.Document
    Modified    []schema.Document    // New versions
    Deleted     []string             // IDs to exclude
    CreatedAt   time.Time
}

type PROptions struct {
    PRNumber    int
    BaseSHA     string
    Changes     []FileChange
}

type FileChange struct {
    Path        string
    OldContent  string    // Empty for new files
    NewContent  string    // Empty for deleted files
    Status      string    // "added", "modified", "deleted"
}

func NewPROverlayStore(base VectorStore) *PROverlayStore
func (s *PROverlayStore) CreateOverlay(ctx context.Context, opts PROptions) error
func (s *PROverlayStore) SearchWithOverlay(ctx context.Context, prNumber int, query string, numDocs int, opts ...Option) ([]schema.Document, error)
func (s *PROverlayStore) RemoveOverlay(prNumber int) error
func (s *PROverlayStore) RefreshOverlay(ctx context.Context, prNumber int, opts PROptions) error
```

**Key Features:**
- Base store remains unmodified
- PR changes are overlaid in memory
- Deleted files are excluded from results
- Modified files show new versions
- Automatic cleanup of old overlays

**Tasks:**
- [ ] Implement `PROverlayStore` wrapper
- [ ] Implement overlay creation from PR diff
- [ ] Implement search with overlay merging
- [ ] Implement overlay cleanup
- [ ] Add concurrent access safety
- [ ] Add tests

### 11.3 Consensus Review Pipeline

**Priority:** High | **Effort:** Medium**

Built-in support for multi-model consensus reviews.

```go
// Proposed API
type ConsensusReviewer struct {
    models      []llms.Model
    prompt      string
    reducer     ConsensusReducer
    maxParallel int
}

type ConsensusReducer interface {
    Reduce(ctx context.Context, reviews []Review) (*ConsensusReview, error)
}

type Review struct {
    Model       string
    Comments    []ReviewComment
    Confidence  float32
    RawOutput   string
}

type ConsensusReview struct {
    AgreedComments   []ReviewComment    // Issues found by multiple models
    UniqueComments   []ReviewComment    // Issues found by single model
    Confidence       float32
    Agreement        float32            // Agreement ratio
    Synthesis        string             // LLM-generated summary
}

func NewConsensusReviewer(models []llms.Model, reducer ConsensusReducer) *ConsensusReviewer
func (c *ConsensusReviewer) Review(ctx context.Context, diff string, context []schema.Document) (*ConsensusReview, error)

// Built-in reducers
type VotingReducer struct{}          // Vote on each issue
type LLMReducer struct { llm Model } // Use LLM to synthesize
type HybridReducer struct{}          // Vote + LLM synthesis
```

**Tasks:**
- [ ] Implement `ConsensusReviewer`
- [ ] Implement `VotingReducer`
- [ ] Implement `LLMReducer`
- [ ] Implement `HybridReducer`
- [ ] Add parallel model execution
- [ ] Add tests

### 11.4 Smart Incremental Indexing

**Priority:** High | **Effort:** Medium**

Git-aware incremental indexing with PostgreSQL state tracking.

```go
// Proposed API
type IncrementalGitIndexer struct {
    store       VectorStore
    git         GitClient
    stateStore  IndexStateStore
    parser      ParserPlugin
    options     IndexerOptions
}

type IndexStateStore interface {
    GetLastIndexedSHA(ctx context.Context, repoID string) (string, error)
    SetLastIndexedSHA(ctx context.Context, repoID, sha string) error
    GetIndexedFiles(ctx context.Context, repoID string) (map[string]FileHash, error)
    MarkFileIndexed(ctx context.Context, repoID, path, hash string) error
    RemoveFile(ctx context.Context, repoID, path string) error
}

type PostgreSQLStateStore struct {
    db *sql.DB
}

type IndexerOptions struct {
    BatchSize       int
    ExcludePatterns []string
    IncludePatterns []string
    OnProgress      func(processed, total int)
}

func NewIncrementalGitIndexer(store VectorStore, git GitClient, state IndexStateStore, parser ParserPlugin, opts IndexerOptions) *IncrementalGitIndexer
func (i *IncrementalGitIndexer) Sync(ctx context.Context, repoPath, repoID string) (*SyncResult, error)

type SyncResult struct {
    Added       int
    Modified    int
    Deleted     int
    Skipped     int
    Duration    time.Duration
    Errors      []error
}
```

**Tasks:**
- [ ] Implement `IncrementalGitIndexer`
- [ ] Implement `PostgreSQLStateStore`
- [ ] Implement git diff detection
- [ ] Implement file hash tracking
- [ ] Add tests with real git repos

### 11.5 Token-Aware Context Packing

**Priority:** High | **Effort:** Medium**

Strict token budget enforcement for RAG context.

```go
// Proposed API
type TokenAwarePacker struct {
    tokenizer   Tokenizer
    maxTokens   int
    strategy    PackingStrategy
}

type PackingStrategy string
const (
    StrategyImportance PackingStrategy = "importance"  // Highest scores first
    StrategyRecency    PackingStrategy = "recency"     // Most recent first
    StrategyDiversity  PackingStrategy = "diversity"   // Diverse content
    StrategyBalanced   PackingStrategy = "balanced"    // Balance all factors
)

type PackedContext struct {
    Documents   []schema.Document
    Tokens      int
    Budget      int
    Utilization float32
    Skipped     int
}

func NewTokenAwarePacker(tokenizer Tokenizer, maxTokens int, strategy PackingStrategy) *TokenAwarePacker
func (p *TokenAwarePacker) Pack(ctx context.Context, docs []ScoredDocument) (*PackedContext, error)
func (p *TokenAwarePacker) PackWithTemplate(ctx context.Context, docs []ScoredDocument, template string) (*PackedContext, error)

// Template-aware packing
type TemplatePacker struct {
    packer      *TokenAwarePacker
    template    string
    reserved    int    // Tokens reserved for template + response
}

func (p *TemplatePacker) EstimateAvailableTokens() int
func (p *TemplatePacker) PackForPrompt(ctx context.Context, docs []ScoredDocument) (string, *PackedContext, error)
```

**Tasks:**
- [ ] Implement `TokenAwarePacker`
- [ ] Implement importance-based strategy
- [ ] Implement diversity-based strategy
- [ ] Implement template-aware packing
- [ ] Add token counting utilities
- [ ] Add tests

### 11.6 Contiguous Chunk Splicing

**Priority:** Medium | **Effort:** Medium**

Merge overlapping code chunks for continuous logic flows.

```go
// Proposed API
type ChunkSplicer struct {
    overlapThreshold float32    // Minimum overlap ratio to splice
    maxGap           int        // Maximum line gap to splice
}

type SplicedChunk struct {
    Documents    []schema.Document  // Original chunks
    Spliced      string             // Merged content
    LineRanges   [][2]int           // Line ranges in original files
    Files        []string           // Source files
}

func NewChunkSplicer(overlapThreshold float32, maxGap int) *ChunkSplicer
func (s *ChunkSplicer) Splice(docs []schema.Document) []SplicedChunk
func (s *ChunkSplicer) CanSplice(doc1, doc2 schema.Document) bool

// Detect overlap
func detectOverlap(content1, content2 string) (overlap string, ratio float32)
// Merge chunks
func mergeChunks(doc1, doc2 schema.Document, overlap string) schema.Document
```

**Tasks:**
- [ ] Implement overlap detection
- [ ] Implement chunk merging
- [ ] Implement line range tracking
- [ ] Handle multi-file chunks
- [ ] Add tests

### 11.7 Reverse HyDE (Synthetic Questions)

**Priority:** Medium | **Effort:** Medium**

Generate synthetic questions during ingestion for better retrieval.

```go
// Proposed API
type SyntheticQuestionGenerator struct {
    llm         llms.Model
    questionsPerDoc int
    promptTemplate  string
}

type SyntheticDocument struct {
    Original        schema.Document
    Questions       []string
    QuestionVectors [][]float32
}

func NewSyntheticQuestionGenerator(llm llms.Model, questionsPerDoc int) *SyntheticQuestionGenerator
func (g *SyntheticQuestionGenerator) Generate(ctx context.Context, doc schema.Document) (*SyntheticDocument, error)
func (g *SyntheticQuestionGenerator) GenerateBatch(ctx context.Context, docs []schema.Document) ([]SyntheticDocument, error)

// During indexing
func (s *SyntheticDocument) ToDocuments() []schema.Document {
    // Returns original + synthetic question documents
    // Questions embed more naturally with user queries
}

// During search
type ReverseHyDERetriever struct {
    store       VectorStore
    generator   *SyntheticQuestionGenerator
}

func (r *ReverseHyDERetriever) Retrieve(ctx context.Context, query string, numDocs int) ([]schema.Document, error)
```

**Tasks:**
- [ ] Implement question generation prompts
- [ ] Implement `SyntheticQuestionGenerator`
- [ ] Implement storage format
- [ ] Implement retrieval logic
- [ ] Add tests

### 11.8 GitHub Integration Helpers

**Priority:** Medium | **Effort:** Low**

Utilities for GitHub PR handling.

```go
// Proposed API
package githubutil

type PRDiff struct {
    Number      int
    Title       string
    Description string
    Files       []FileDiff
    BaseSHA     string
    HeadSHA     string
}

type FileDiff struct {
    Path        string
    Status      string    // "added", "modified", "deleted", "renamed"
    OldContent  string
    NewContent  string
    Hunks       []Hunk
}

type Hunk struct {
    OldStart    int
    OldLines    int
    NewStart    int
    NewLines    int
    Content     string
}

func ParsePRDiff(diffText string) (*PRDiff, error)
func ExtractChangedLines(file *FileDiff) []LineRange
func ToDocuments(diff *PRDiff) []schema.Document

// GitHub API helpers
type GitHubClient struct {
    client *github.Client
}

func (c *GitHubClient) GetPR(ctx context.Context, owner, repo string, number int) (*PRDiff, error)
func (c *GitHubClient) PostReviewComments(ctx context.Context, owner, repo string, number int, comments []ReviewComment) error
func (c *GitHubClient) ResolveComment(ctx context.Context, owner, repo string, commentID int64) error
```

**Tasks:**
- [ ] Implement PR diff parser
- [ ] Implement hunk extraction
- [ ] Implement document conversion
- [ ] Implement GitHub API client
- [ ] Add tests

### 11.9 Hallucination Detection

**Priority:** Low | **Effort:** Medium**

Detect and flag potential LLM hallucinations.

```go
// Proposed API
type HallucinationDetector struct {
    store       VectorStore
    llm         llms.Model
    threshold   float32
}

type HallucinationCheck struct {
    Statement   string
    Confidence  float32
    Verified    bool
    Sources     []schema.Document
    Reason      string
}

func NewHallucinationDetector(store VectorStore, llm llms.Model, threshold float32) *HallucinationDetector
func (d *HallucinationDetector) Check(ctx context.Context, statement string, context []schema.Document) (*HallucinationCheck, error)
func (d *HallucinationDetector) CheckBatch(ctx context.Context, statements []string, context []schema.Document) ([]HallucinationCheck, error)

// Verification strategies
func (d *HallucinationDetector) verifyAgainstSources(statement string, sources []schema.Document) bool
func (d *HallucinationDetector) verifyWithRetrieval(ctx context.Context, statement string) ([]schema.Document, bool)
```

**Tasks:**
- [ ] Implement statement extraction
- [ ] Implement source verification
- [ ] Implement retrieval-based verification
- [ ] Add confidence scoring
- [ ] Add tests

### 11.10 Structured Output Parsing

**Priority:** High | **Effort:** Low**

Enhanced typed output parsing with validation.

```go
// Proposed API
package output

type StructuredParser[T any] struct {
    schema      string
    validator   func(T) error
    maxRetries  int
}

func NewStructuredParser[T any](schema string, validator func(T) error) *StructuredParser[T]
func (p *StructuredParser[T]) Parse(ctx context.Context, raw string) (T, error)
func (p *StructuredParser[T]) ParseWithRetry(ctx context.Context, llm llms.Model, prompt string) (T, error)

// Pre-built parsers for code review
type ReviewComment struct {
    File        string `json:"file"`
    Line        int    `json:"line"`
    Severity    string `json:"severity"`
    Message     string `json:"message"`
    Suggestion  string `json:"suggestion,omitempty"`
}

type ReviewOutput struct {
    Summary     string           `json:"summary"`
    Comments    []ReviewComment  `json:"comments"`
    Score       float32          `json:"score"`
}

// JSON schema enforcement
func WithJSONSchema(schema string) ParseOption
func WithXMLFormat() ParseOption
func WithStrictValidation() ParseOption
```

**Tasks:**
- [ ] Implement `StructuredParser`
- [ ] Implement retry logic
- [ ] Add JSON schema validation
- [ ] Add XML parsing support
- [ ] Add pre-built review parsers
- [ ] Add tests

---

## Quick Start Guide for Contributors

### High Priority, Low Effort (Start Here!)
1. Add tests for error paths (#4.1)
2. Pre-compile regex patterns (#6.1)
3. Implement Qdrant Count API (#2.2)
4. Implement Qdrant Scroll API (#2.1)
5. Implement Ollama List Models (#3.1)

### Medium Priority, Medium Effort
6. Add metrics collection (#5.1)
7. Implement Qdrant Groups API (#2.3)
8. Add Ollama JSON Mode (#3.2)
9. Create integration tests (#4.2)

### Documentation Tasks (Always Welcome)
10. Complete godoc for remaining packages (#8.1)
11. Improve README (#8.2)
12. Add more examples (#7.3)

---

*Last updated: 2026-02-23*