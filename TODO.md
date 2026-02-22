# GoFrame Development Roadmap

A comprehensive development roadmap for the GoFrame RAG/Chain framework, organized by priority and category.

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