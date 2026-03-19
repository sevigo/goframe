# GoFrame TODO

Focused roadmap for the library. Items here are genuine library improvements — not application-level features (those belong in the applications built on top of GoFrame).

---

## High Priority

### Qdrant: Scroll API

Paginate through all points in a collection — needed for re-indexing, bulk operations, and debugging.

```go
type ScrollResult struct {
    Points   []schema.Document
    NextPage []byte // cursor for next page
}

func (s *Store) Scroll(ctx context.Context, opts ...Option) (*ScrollResult, error)
```

- [ ] Implement `Scroll()` in Qdrant store
- [ ] Support filter options
- [ ] Add tests

### Qdrant: Count API

Count documents matching a filter — needed for progress tracking and index validation.

```go
func (s *Store) Count(ctx context.Context, filter map[string]any) (int64, error)
```

- [ ] Implement `Count()` in Qdrant store
- [ ] Add tests

### Qdrant: Groups API (Grouped Search)

Group results by a metadata field to avoid over-representation from a single file.

```go
type GroupedResult struct {
    GroupKey string
    Hits     []schema.Document
    Score    float32
}

func (s *Store) SearchGroups(ctx context.Context, query string, groupBy string, opts ...Option) ([]GroupedResult, error)
```

- [ ] Implement `SearchGroups()`
- [ ] Add `WithGroupSize()` option (max hits per group)
- [ ] Add tests

### Qdrant: Lookup by ID

Retrieve documents directly by Qdrant point ID — needed for updating and dereferencing stored chunks.

```go
func (s *Store) GetByID(ctx context.Context, id string) (*schema.Document, error)
func (s *Store) GetByIDs(ctx context.Context, ids []string) ([]schema.Document, error)
```

- [ ] Implement `GetByID()` and `GetByIDs()`
- [ ] Add tests

### Connection Lifecycle

Standardize `Close()` across all clients. Currently only `qdrant.Store` has it.

- [ ] Add `Close()` to `ollama.LLM` (cleanup HTTP connections)
- [ ] Add `Close()` to `gemini.LLM` (cleanup gRPC)
- [ ] Document lifecycle management in README

---

## Medium Priority

### Embedding Cache

Avoid re-embedding identical content during incremental indexing or repeated queries.

```go
type CachedEmbedder struct {
    embedder Embedder
    cache    Cache
}

type Cache interface {
    Get(ctx context.Context, key string) ([]float32, bool)
    Set(ctx context.Context, key string, value []float32, ttl time.Duration)
}

func NewCachedEmbedder(embedder Embedder, cache Cache) *CachedEmbedder
```

- [ ] Define `Cache` interface
- [ ] Implement `CachedEmbedder`
- [ ] Ship an in-memory LRU cache implementation
- [ ] Add tests

### More Language Parsers

Current parser coverage: Go, TypeScript/TSX, Markdown, JSON, YAML, Python (partial), Terraform, Protobuf, PDF, RSS.

- [ ] Python: improve definition extraction (classes, methods, decorators)
- [ ] Rust: struct, impl, trait extraction
- [ ] Java/Kotlin: class and method extraction
- [ ] Ruby: class and method extraction

Each parser should extract at minimum: `identifier`, `kind` (function/struct/interface/class), `package_name`, `is_exported`, `imports`.

### Integration Tests

Tests that run against real Qdrant and Ollama instances.

```go
func TestQdrantIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    // ...
}
```

- [ ] Create `docker-compose.test.yml` with Qdrant
- [ ] Add Qdrant integration tests covering similarity search, filters, hybrid search, groups
- [ ] Add to CI (skipped unless `INTEGRATION=1`)

### Performance: Regex Precompilation

Several parsers compile regexes inside functions on every call. Move to package-level vars.

Affected:
- `parsers/markdown/parser.go`
- `parsers/markdown/extractor.go`
- `parsers/text/chunker.go`

- [ ] Audit all parser packages
- [ ] Move regex compilation to `var` block
- [ ] Add benchmarks to confirm improvement

### Qdrant: Payload Index Management

Expose payload index creation so consumers can ensure fast filtering without manual Qdrant setup.

```go
func (s *Store) EnsurePayloadIndex(ctx context.Context, field string, fieldType PayloadFieldType) error
```

- [ ] Implement `EnsurePayloadIndex()`
- [ ] Call automatically for standard fields (`chunk_type`, `identifier`, `source`, `is_test`) on collection creation
- [ ] Add tests

---

## Lower Priority

### Structured Output Parsing

Generic typed parser for LLM outputs — validates, retries on parse failure, supports JSON and XML.

```go
type StructuredParser[T any] struct {
    validator  func(T) error
    maxRetries int
}

func NewStructuredParser[T any](validator func(T) error) *StructuredParser[T]
func (p *StructuredParser[T]) Parse(ctx context.Context, raw string) (T, error)
func (p *StructuredParser[T]) ParseWithRetry(ctx context.Context, llm llms.Model, prompt string) (T, error)
```

Currently each application implements its own XML/JSON parsing + retry. Extract the common pattern.

- [ ] Implement generic `StructuredParser[T]`
- [ ] Add retry with re-prompt on parse failure
- [ ] Add tests

### Metrics (Prometheus)

Optional metrics hook — latency histograms and counters for embedding, search, and LLM calls. Should be opt-in, no hard Prometheus dependency.

```go
type Metrics interface {
    ObserveEmbeddingLatency(model string, d time.Duration)
    ObserveSearchLatency(collection string, d time.Duration)
    ObserveLLMLatency(model string, d time.Duration)
    IncDocumentsIndexed(collection string, n int)
}
```

- [ ] Define `Metrics` interface in `schema/`
- [ ] Wire optional metrics into Qdrant store and Ollama client
- [ ] Ship a Prometheus implementation as a separate file (no import if unused)

### Qdrant: Recommendation API

Find similar documents using point IDs as positive/negative examples — useful for feedback-based retrieval.

```go
func (s *Store) Recommend(ctx context.Context, positiveIDs []string, negativeIDs []string, n int, opts ...Option) ([]schema.Document, error)
```

- [ ] Implement `Recommend()`
- [ ] Add tests

### Godoc Coverage

Complete package-level documentation for all public packages so pkg.go.dev renders usefully.

Priority order (least documented first):
- `embeddings/sparse/code`
- `vectorstores/` (retrievers, options)
- `parsers/golang`, `parsers/typescript`
- `textsplitter/`
- `documentloaders/`

---

## Out of Scope

The following belong in applications built on top of GoFrame, not in the library itself:

- PR overlay / ephemeral index layering — application-level state management
- Consensus review pipeline — application-level orchestration
- Incremental git indexer with PostgreSQL state — application-level persistence
- Token-aware context packing — application-level prompt assembly
- GitHub API client / PR diff parser — application-specific
- Hallucination detection pipeline — application-level validation
- Code FAQ system — application-level feature
- Multi-vector-store routing — speculative, no concrete use case yet
- Alternative vector store backends (Weaviate, Pinecone, Chroma) — not planned; Qdrant covers the use case well
