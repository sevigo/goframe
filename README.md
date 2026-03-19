# GoFrame

[![Go Reference](https://pkg.go.dev/badge/github.com/sevigo/goframe.svg)](https://pkg.go.dev/github.com/sevigo/goframe)
[![Go Report Card](https://goreportcard.com/badge/github.com/sevigo/goframe)](https://goreportcard.com/report/github.com/sevigo/goframe)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**A Go RAG library built for code understanding.** GoFrame handles the plumbing — document loading, AST-based chunking, embedding, hybrid vector search, and dependency graph traversal — so you can focus on building applications on top of it.

It is the library underlying [Code-Warden](https://github.com/sevigo/code-warden), a self-hosted GitHub App that performs context-aware code reviews using a 6-stage RAG pipeline.

---

## What Makes It Different

Most RAG libraries treat code as plain text. GoFrame understands it:

- **AST-aware chunking** — splits at function and type boundaries, not arbitrary character counts; file-level metadata (package name, imports) propagates to every chunk
- **Multi-language parsing** — Go, TypeScript/TSX, Markdown, JSON, YAML, Python, Terraform, Protobuf, PDF, RSS; each parser extracts language-specific metadata
- **Dependency graph traversal** — `DependencyRetriever` answers "who imports this package?" and "what does this file depend on?" using metadata stored at index time
- **Code-aware sparse tokenization** — splits camelCase (`processPayment` → `process`, `payment`) and acronyms (`XMLParser` → `xml`, `parser`) before hashing into a sparse vector; hybrid search combines this with dense embeddings for better identifier recall
- **Test linkage** — indexes test files with `tested_symbols` metadata so tests can be retrieved by the symbols they exercise, not just by text similarity

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/sevigo/goframe/chains"
    "github.com/sevigo/goframe/embeddings"
    "github.com/sevigo/goframe/llms/ollama"
    "github.com/sevigo/goframe/schema"
    "github.com/sevigo/goframe/vectorstores"
    "github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
    ctx := context.Background()

    llm, _ := ollama.New(ollama.WithModel("qwen2.5-coder:7b"))
    embedder, _ := embeddings.NewEmbedder(llm)
    store, _ := qdrant.New(
        qdrant.WithCollectionName("my-repo"),
        qdrant.WithEmbedder(embedder),
    )

    docs := []schema.Document{
        schema.NewDocument("func getUserByID(id string) (*User, error) { ... }", map[string]any{
            "source":       "internal/users/service.go",
            "chunk_type":   "definition",
            "identifier":   "getUserByID",
            "package_name": "users",
        }),
    }
    store.AddDocuments(ctx, docs)

    retriever := vectorstores.ToRetriever(store, 5)
    chain, _ := chains.NewRetrievalQA(retriever, llm)
    answer, _ := chain.Call(ctx, "How does user lookup work?")
    fmt.Println(answer)
}
```

---

## Installation

```bash
go get github.com/sevigo/goframe@latest
```

Requires Go 1.21+, [Ollama](https://ollama.com/) for local LLMs and embeddings, and [Qdrant](https://qdrant.tech/) for vector storage.

---

## Core Pipeline

```
[GitLoader] → [ParserRegistry] → [CodeAwareSplitter] → [Embedder + SparseProvider] → [Qdrant]
   (load)      (AST metadata)       (chunk at          (dense + code sparse          (store with
               (imports, pkg)        boundaries)        vectors per chunk)             metadata)
```

At query time:
```
[Query] → [SparseProvider] → [SimilaritySearch with sparse+dense] → [Reranker] → [LLM Chain]
```

---

## Key Packages

| Package | What it does |
|---|---|
| `schema/` | Core types: `Document`, `SparseVector`, `Retriever`, `Reranker` |
| `llms/ollama` | Ollama LLM client — chat, completion, streaming |
| `embeddings/` | `Embedder` interface + batch embedding with retry |
| `embeddings/sparse/` | Sparse vector generation — default BoW provider, pluggable |
| `embeddings/sparse/code` | Code-aware sparse tokenizer (camelCase/snake_case splitting + FNV32a) |
| `vectorstores/qdrant` | Qdrant store — hybrid search, metadata filtering, binary quantization |
| `vectorstores/` | `DependencyRetriever`, `DefinitionRetriever`, `ToRetriever` helpers |
| `parsers/` | Language parser plugins — Go, TypeScript, Markdown, JSON, YAML, Python, etc. |
| `textsplitter/` | `CodeAwareTextSplitter` — AST-boundary splitting with metadata propagation |
| `documentloaders/` | `GitLoader` — streaming file ingestion from git repos with metadata |
| `chains/` | `LLMChain[T]`, `RetrievalQA`, `MapReduceChain` |
| `agent/` | OpenCode agent SDK — session management, MCP server config, streaming |

---

## Examples

### Hybrid Search (Dense + Sparse)

```go
import (
    "github.com/sevigo/goframe/embeddings/sparse"
    sparsecode "github.com/sevigo/goframe/embeddings/sparse/code"
    "github.com/sevigo/goframe/vectorstores/qdrant"
    "github.com/sevigo/goframe/vectorstores"
)

// Register code-aware sparse tokenizer (once at startup)
sparse.RegisterProvider(sparsecode.NewCodeSparseProvider())

// Create store with sparse vector support
store, _ := qdrant.New(
    qdrant.WithCollectionName("code"),
    qdrant.WithEmbedder(embedder),
    qdrant.WithSparseVector("code_sparse"),
)

// Index with sparse vectors
doc := schema.NewDocument("func getUserByID(id string) (*User, error)", nil)
doc.Sparse, _ = sparse.GenerateSparseVector(ctx, doc.PageContent)
store.AddDocuments(ctx, []schema.Document{doc})

// Hybrid search
sparseQuery, _ := sparse.GenerateSparseVector(ctx, "getUserByID")
results, _ := store.SimilaritySearch(ctx, "getUserByID", 5,
    vectorstores.WithSparseQuery(sparseQuery),
)
```

### Dependency Graph Traversal

```go
retriever, _ := vectorstores.NewDependencyRetriever(store)

// Who imports this package? (impact analysis)
network, _ := retriever.GetContextNetwork(ctx, "github.com/my/project/pkg/users", nil)
for _, dependent := range network.Dependents {
    fmt.Println("Affected file:", dependent.Metadata["source"])
}

// What does this file depend on?
network, _ = retriever.GetContextNetwork(ctx, "github.com/my/project/pkg/users",
    []string{"context", "database/sql"})
for _, dep := range network.Dependencies {
    fmt.Println("Dependency:", dep.Metadata["source"])
}
```

### Git Repository Ingestion

```go
import (
    "github.com/sevigo/goframe/documentloaders"
    "github.com/sevigo/goframe/parsers"
    "github.com/sevigo/goframe/textsplitter"
)

registry := parsers.NewRegistry(logger)
splitter := textsplitter.NewCodeAwareTextSplitter(registry,
    textsplitter.WithChunkSize(800),
    textsplitter.WithChunkOverlap(100),
)

loader, _ := documentloaders.NewGit(repoPath, registry,
    documentloaders.WithSplitter(splitter),
    documentloaders.WithBatchSize(50),
)

// Stream directly into vector store
loader.LoadAndProcessStream(ctx, func(ctx context.Context, batch []schema.Document) error {
    for i := range batch {
        batch[i].Sparse, _ = sparse.GenerateSparseVector(ctx, batch[i].PageContent)
    }
    _, err := store.AddDocuments(ctx, batch)
    return err
})
```

### Multi-Model Consensus (MapReduceChain)

```go
import "github.com/sevigo/goframe/chains"

models := []llms.Model{model1, model2, model3}
chain, _ := chains.NewMapReduceChain(models, reducerModel, prompt,
    chains.WithMaxParallel(3),
    chains.WithQuorum(0.66), // Proceed when 66% of models finish
)
result, _ := chain.Call(ctx, map[string]any{"context": ctx, "diff": diff})
```

### Exact Definition Lookup

```go
defRetriever, _ := vectorstores.NewDefinitionRetriever(store)

// Fast path: exact filter on identifier + chunk_type
exactDocs, err := store.SimilaritySearch(ctx, symbol, 1,
    vectorstores.WithFilters(map[string]any{
        "chunk_type": "definition",
        "identifier": symbol,
    }),
)

// Semantic fallback
if err != nil || len(exactDocs) == 0 {
    exactDocs, _ = defRetriever.GetDefinition(ctx, symbol)
}
```

---

## Running the Example

```bash
go run ./examples/qdrant-ultimate-rag/main.go
```

The example demonstrates full repository ingestion (Go + TypeScript), hybrid search, and dependency graph verification against a real Qdrant instance.

---

## Sparse Vector Provider

The default sparse provider uses a pretrained BoW tokenizer. For source code, register the code-aware provider instead:

```go
import (
    "github.com/sevigo/goframe/embeddings/sparse"
    sparsecode "github.com/sevigo/goframe/embeddings/sparse/code"
)

// Call once at application startup
sparse.RegisterProvider(sparsecode.NewCodeSparseProvider())
```

The code provider:
- Splits camelCase: `processPayment` → `["process", "payment"]`
- Splits acronyms: `XMLParser` → `["xml", "parser"]`, `HTTPClient` → `["http", "client"]`
- Handles mixed: `get_HTTPClient` → `["get", "http", "client"]`
- Filters Go/JS/Python/Rust language keywords
- Hashes via FNV32a into 50,000-dimension sparse space with L2 normalization

---

## How to Contribute

```bash
make lint      # Run linters
make test      # Run all tests
make pre-push  # lint + test combined
```

For a single package:
```bash
go test ./vectorstores/qdrant/... -v
go test -run TestStoreSimilaritySearch ./vectorstores/qdrant/...
```

See [TODO.md](TODO.md) for what's next.

## License

MIT — see [LICENSE](LICENSE).
