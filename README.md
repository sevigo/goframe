# GoFrame

A modular Go framework for building production-ready LLM and RAG applications. GoFrame provides a clean, extensible architecture with a powerful set of tools for document processing, embedding, and vector storage.

## Overview

GoFrame is designed to simplify the development of applications that leverage Large Language Models, with a strong focus on Retrieval-Augmented Generation (RAG). It provides a set of decoupled components that can be composed to build sophisticated data pipelines.

The framework is built around a set of core interfaces for LLMs, Embedders, and Vector Stores, allowing you to easily swap implementations (e.g., switch from Ollama to another provider) without changing your core application logic.

## Core Features

-   **Pluggable LLM Clients**: A clean interface for interacting with different LLMs.
    - **Ollama**: Full-featured client for local LLMs, including automatic model pulling.
-   **Vector Store Abstraction**: A unified interface for vector databases.
    - **Qdrant**: A robust implementation with support for collection management, metadata filtering, and similarity search.
-   **Unified Embedding Interface**: Decouples embedding generation from its usage.
-   **Advanced Document Processing**:
    - **Document Loaders**: Load documents from various sources (e.g., `GitLoader` for local repositories).
    - **Code-Aware Text Splitting**: Intelligently chunks files based on their semantic structure.
-   **Language-Specific Parsers**: A powerful plugin system for understanding different file types.
    - **Go**: Chunks by functions, types, and top-level declarations.
    - **Markdown**: Chunks by hierarchical heading structure.
    - **YAML & JSON**: Chunks by top-level keys and large structures.
    - **PDF**: Chunks by pages and paragraphs.
    - **CSV & Text**: Semantic chunking for structured and plain text.
-   **Standardized Schema**: Common data structures (`Document`, `CodeChunk`, etc.) for consistent data flow.

## Architecture

GoFrame follows a modular, pipeline-oriented architecture ideal for RAG workflows.

```
[Source Data] -> [Document Loader] -> [Parser Plugin] -> [Code-Aware Splitter] -> [Embedding Model] -> [Vector Store]
 (e.g., Git Repo)     (git.go)        (e.g., golang)         (code_aware.go)        (ollama.go)          (qdrant.go)
```

1.  **Load**: A `DocumentLoader` reads content from a source.
2.  **Split**: The `CodeAwareTextSplitter` uses a `ParserPlugin` to break the content into meaningful `CodeChunk`s.
3.  **Embed**: An `Embedder` (wrapping an LLM like Ollama's `nomic-embed-text`) converts each chunk's content into a vector.
4.  **Store**: A `VectorStore` (like Qdrant) stores the chunks and their vector embeddings.
5.  **Retrieve**: The application can then perform a `SimilaritySearch` against the `VectorStore` to find relevant documents for a given query.

## Getting Started

### Prerequisites

-   Go 1.21 or later.
-   (Optional) [Ollama](https://ollama.com/) running locally for the example.
-   (Optional) [Docker](https://www.docker.com/) for running Qdrant.

### Installation

Add GoFrame to your project's dependencies:
```bash
go get github.com/sevigo/goframe@latest
```

## Quick Usage

Here is a simple example of how to use GoFrame's components to embed and search documents.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
	ctx := context.Background()
	logger := slog.Default()

	// 1. Initialize an embedder using Ollama
	// Note: The client will automatically pull the model if not present.
	ollamaEmbedder, _ := ollama.New(ollama.WithModel("nomic-embed-text"))
	embedder, _ := embeddings.NewEmbedder(ollamaEmbedder)

	// 2. Initialize the Qdrant vector store
	store, _ := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName("my-docs"),
		qdrant.WithLogger(logger),
	)

	// 3. Add documents to the store
	docs := []schema.Document{
		schema.NewDocument("Paris is the capital of France.", map[string]any{"continent": "Europe"}),
		schema.NewDocument("London is the capital of the UK.", map[string]any{"continent": "Europe"}),
		schema.NewDocument("Tokyo is the capital of Japan.", map[string]any{"continent": "Asia"}),
	}
	_, err := store.AddDocuments(ctx, docs)
	if err != nil {
		panic(err)
	}
	fmt.Println("Documents added successfully.")

	// 4. Perform a similarity search with a metadata filter
	query := "Which city is in Europe?"
	results, err := store.SimilaritySearch(ctx, query, 2, vectorstores.WithFilter("continent", "Europe"))
	if err != nil {
		panic(err)
	}

	fmt.Printf("\nFound %d results for query: %q\n", len(results), query)
	for _, doc := range results {
		fmt.Printf("- %s\n", doc.PageContent)
	}
}
```

## Running the Full Example

A more comprehensive example demonstrating advanced features is available in the `examples/` directory.

### 1. Start Services

First, ensure Qdrant and Ollama are running.

**Qdrant:**
```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

**Ollama:**
Make sure the Ollama application is running on your machine.

### 2. Pull the Embedding Model

While the GoFrame client will pull the model automatically, it's good practice to do it manually for a smoother first run.

```bash
ollama pull nomic-embed-text
```

### 3. Run the Example Code

Execute the `main.go` file from the project root:
```bash
go run ./examples/ollama-qdrant-vectorstore-example/main.go
```
The program will create a collection, add documents, run several search scenarios (including a filtered search), demonstrate deletion, and clean up after itself.

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