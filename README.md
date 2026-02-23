# GoFrame

A modular Go framework for building production-ready LLM and RAG applications. GoFrame provides a clean, extensible architecture with a powerful set of tools for document processing, embedding, and vector storage.

## Overview

GoFrame is designed to simplify the development of applications that leverage Large Language Models, with a strong focus on Retrieval-Augmented Generation (RAG). It provides a set of decoupled components that can be composed to build sophisticated data pipelines.

The framework is built around a set of core interfaces for LLMs, Embedders, and Vector Stores, allowing you to easily swap implementations (e.g., switch from Ollama to another provider) without changing your core application logic.

## Core Features

-   **Scalable Agentic Infrastructure**: Built for high-performance agentic workflows.
    -   **Streaming Ingestion**: Process massive repositories with flat memory usage.
    -   **Binary Quantization**: 30x memory reduction for vector storage.
-   **Graph-Like Retrieval**: Go beyond simple similarity search.
    -   **Impact Analysis**: Find downstream dependents ("who uses this code?").
    -   **Dependency Verification**: Trace upstream dependencies ("what does this use?").
    -   **Multi-Language Support**: Automatic metadata extraction for **Go** and **TypeScript/TSX**.
-   **Pluggable Architecture**:
    -   **LLMs**: Clean interfaces for Ollama (local) and cloud providers.
    -   **Vector Stores**: Robust Qdrant implementation with metadata filtering.
    -   **Embeddings**: Decoupled embedding generation.
-   **Advanced Document Processing**:
    -   **GitLoader**: Smart loading with automatic metadata extraction (imports, packages).
    -   **Code-Aware Splitter**: Semantically chunks code while preserving context.
    -   **Parsers**: Plugins for Go, TypeScript, Markdown, JSON, YAML, PDF, and more.

## Architecture

GoFrame follows a modular pipeline:

```
[Source Code] -> [GitLoader] -> [Parser Plugin] -> [CodeAwareSplitter] -> [Embedder] -> [VectorStore]
(Go, TS, etc.)   (Extracts Metadata)  (AST Analysis)    (Propagates Metadata)    (Ollama)      (Qdrant)
```

1.  **Load & Analyze**: `GitLoader` reads files and uses language parsers to extract *file-level metadata* (imports, package name).
2.  **Split & Propagate**: `CodeAwareTextSplitter` chunks the code, propagating the file-level metadata to every chunk.
3.  **Embed & Index**: content is embedded and stored in Qdrant with enriched metadata.
4.  **Graph Retrieval**: The `DependencyRetriever` uses this metadata to traverse the dependency graph.

## Getting Started

### Prerequisites

-   Go 1.21 or later.
-   [Ollama](https://ollama.com/) (for embeddings & local LLMs).
-   [Docker](https://www.docker.com/) (for Qdrant).

### Installation

```bash
go get github.com/sevigo/goframe@latest
```

## Usage Examples

### 1. Basic RAG

```go
// Initialize components...
store, _ := qdrant.New(qdrant.WithCollectionName("my-docs"))

// Add documents
docs := []schema.Document{
    schema.NewDocument("Paris is the capital of France.", map[string]any{"continent": "Europe"}),
}
store.AddDocuments(ctx, docs)

// Search
results, _ := store.SimilaritySearch(ctx, "Europe capital", 1)
```

### 2. Graph / Dependency Analysis

Perform sophisticated code navigation using the `DependencyRetriever`.

```go
import "github.com/sevigo/goframe/vectorstores"

// Initialize retriever
retriever, err := vectorstores.NewDependencyRetriever(store)
if err != nil {
    log.Fatal(err)
}

// 1. Impact Analysis: Who imports "my/package"?
network, _ := retriever.GetContextNetwork(ctx, "github.com/my/project/pkg", nil)
for _, dependent := range network.Dependents {
    fmt.Printf("File identifying impact: %s\n", dependent.Metadata["source"])
}

// 2. Upstream Verification: What does "my/package" depend on?
// (Pass known imports to verify their existence in the graph)
network, _ = retriever.GetContextNetwork(ctx, "github.com/my/project/pkg", []string{"fmt", "os"})
for _, dep := range network.Dependencies {
    fmt.Printf("Verified dependency: %s\n", dep.Metadata["source"])
}
```
### 3. Hybrid Search (Dense + Sparse)
Combine semantic understanding with exact keyword matching using sparse vectors.

```go
import (
    "github.com/sevigo/goframe/embeddings/sparse"
    "github.com/sevigo/goframe/vectorstores/qdrant"
    "github.com/sevigo/goframe/vectorstores"
)

// 1. Configure Store with Named Sparse Vector
store, _ := qdrant.New(
    qdrant.WithCollectionName("hybrid-docs"),
    qdrant.WithSparseVector("bow_sparse"), // Enable sparse vector support
)

// 2. Add Document with Sparse Vector
docContent := "func CalculateTax(income float64) float64 { ... }"
sparseVec, _ := sparse.GenerateSparseVector(ctx, docContent)
doc := schema.NewDocument(docContent, nil)
doc.Sparse = sparseVec
store.AddDocuments(ctx, []schema.Document{doc})

// 3. Perform Hybrid Search
query := "CalculateTax"
sparseQuery, _ := sparse.GenerateSparseVector(ctx, query)

results, _ := store.SimilaritySearch(ctx, query, 5,
    vectorstores.WithSparseQuery(sparseQuery), // Pass sparse query for hybrid retrieval
)
```

## Running the Ultimate RAG Demo

The `examples/qdrant-ultimate-rag` is a production-grade demonstration featuring:
*   Full repository ingestion (Go & TypeScript).
*   Streaming processing pipeline.
*   Graph Retrieval verification.

```bash
# Set up environment
export OLLAMA_API_KEY=your_key_if_using_cloud

# Run the full integration test
go run ./examples/qdrant-ultimate-rag/main.go
```

---

## Core Components

-   **`/schema`**: Defines the core data structures used throughout the framework, such as `Document`, `ChatMessage`, and `ParserPlugin`.
-   **`/llms`**: Contains interfaces and implementations for LLM clients. The `ollama` package provides a full-featured client.
-   **`/embeddings`**: Provides the `Embedder` interface and a default implementation that wraps an LLM client to perform embedding tasks.
-   **`/vectorstores`**: Contains interfaces and implementations for vector stores. The `qdrant` package provides a robust client.
-   **`/parsers`**: Home to the language parser plugin system. Each sub-directory (`/golang`, `/markdown`, etc.) contains a plugin for a specific file type. See `Plugins.md` for more details.
-   **`/textsplitter`**: Provides the `CodeAwareTextSplitter`, which uses the parser plugins to perform intelligent, semantic chunking of documents.

## How to Contribute

Contributions are welcome! Whether it's a bug fix, a new feature, or documentation improvements, we appreciate your help.

1.  **Fork the repository.**
2.  **Create a new branch** for your feature (`git checkout -b feature/my-new-feature`).
3.  **Make your changes** and add/update tests.
4.  **Run tests** to ensure everything is working (`go test ./...`).
5.  **Submit a pull request** with a clear description of your changes.

### Areas for Contribution

-   **New LLM Clients**: Add support for providers like OpenAI, Anthropic, or Hugging Face.
-   **New Vector Stores**: Implement the `VectorStore` interface for ChromaDB, Pinecone, Weaviate, etc.
-   **New Parser Plugins**: Add support for more languages like Python, Java, C++, or Rust.
-   **Enhance RAG Components**: Implement advanced retrieval strategies like re-rankers or query transformers.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.