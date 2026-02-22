# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make help       # Show all available make targets
make lint       # Run linters
make lint-fix   # Run linters and auto-fix
make test       # Run all tests
make test-race  # Run tests with race detector
make test-cover # Run tests and generate coverage report
make pre-push   # Run lint + test (pre-commit check)
```

To run a single test file or package:
```bash
go test ./vectorstores/qdrant/... -v
go test -run TestStoreSimilaritySearch ./vectorstores/qdrant/...
```

For the ultimate RAG integration test:
```bash
go run ./examples/qdrant-ultimate-rag/main.go
```

## Architecture

### Pipeline Flow
```
[Source Code] -> [GitLoader] -> [Parser Plugin] -> [CodeAwareSplitter] -> [Embedder] -> [VectorStore]
(Go, TS, etc.)   (Extracts Metadata)  (AST Analysis)    (Propagates Metadata)    (Ollama)      (Qdrant)
```

1. **GitLoader** (`documentloaders/git.go`): Loads files from a git repository with batch processing and streaming support
2. **Parser Registry** (`parsers/`): Language-specific plugins (Go, TypeScript, Markdown, etc.) extract metadata (package name, imports, definitions)
3. **CodeAwareSplitter** (`textsplitter/code_aware.go`): Chunks code while preserving structure and propagating file-level metadata
4. **Embedder** (`embeddings/embeddings.go`): Wraps LLM clients for embedding generation with batch processing
5. **VectorStore** (`vectorstores/qdrant/qdrant.go`): Qdrant integration with metadata filtering and hybrid search (dense + sparse vectors)

### Core Interfaces

| Package | Interface | Purpose |
|---------|-----------|---------|
| `llms` | `Model` | LLM provider abstraction (GenerateContent, Call) |
| `embeddings` | `Embedder` | Vector embedding (EmbedDocuments, EmbedQuery, GetDimension) |
| `schema` | `Retriever` | Document retrieval (GetRelevantDocuments) |
| `schema` | `Reranker` | Result reranking (Rerank) |
| `vectorstores` | `VectorStore` | Vector database operations (AddDocuments, SimilaritySearch) |
| `parsers` | `ParserPlugin` | Language parsing (Chunk, ExtractMetadata, CanHandle) |

### Key Patterns

- **Functional Options**: All constructors use functional options pattern (e.g., `qdrant.New(WithCollectionName("..."), WithEmbedder(...))`)
- **Context Propagation**: All IO-bound operations accept `context.Context` as first parameter
- **Batch Processing**: Large operations use configurable batch sizes with concurrency limits
- **Retry Logic**: Transient errors are automatically retried with exponential backoff
- **Sparse Vectors**: Hybrid search enabled via `WithSparseVector()` config and `WithSparseQuery()` option

### Package Layout

| Directory | Purpose |
|-----------|---------|
| `schema/` | Core data structures (Document, SparseVector, Retriever, Reranker) |
| `llms/` | LLM interface and common utilities |
| `embeddings/` | Embedder interface and EmbedderImpl wrapper |
| `vectorstores/` | Vector store interfaces and Qdrant implementation |
| `parsers/` | Language parser plugin system |
| `textsplitter/` | Code-aware text splitting logic |
| `chains/` | RAG chain implementations (LLMChain, RetrievalQA, MapReduceChain) |
| `documentloaders/` | Document loading strategies (GitLoader) |
| `examples/` | Usage examples and integration tests |

