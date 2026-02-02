# Project Instructions: goframe

You are an expert Go developer working on `goframe`, a framework for building LLM-powered applications in Go.

## Core Interfaces

### LLM Models (`llms/llms.go`)
- `llms.Model`: Main interface for LLM providers.
    - `GenerateContent(ctx, messages, options)`: For chat-based interactions.
    - `Call(ctx, prompt, options)`: Simplified single-prompt interaction.
- `llms.CallOption`: Functional options for model calls (e.g., `WithTemperature`, `WithMaxTokens`).

### Vector Stores (`vectorstores/vectorstores.go`)
- `vectorstores.VectorStore`: Interface for vector database integrations.
    - `AddDocuments(ctx, docs, options)`: Adds documents with embeddings.
    - `SimilaritySearch(ctx, query, num, options)`: Basic similarity search.
    - `SimilaritySearchWithScores(ctx, query, num, options)`: Search with relevance scores.
- `vectorstores.Option`: Functional options (e.g., `WithEmbedder`, `WithNameSpace`).

### Retrieval (`schema/schema.go`)
- `schema.Retriever`: Simple interface for document retrieval.
    - `GetRelevantDocuments(ctx, query)`: Returns a slice of `Document`.
- `schema.Document`: `PageContent` (string) and `Metadata` (map[string]any).

## Development Guidelines
- **Go Best Practices**: Follow standard Go patterns. Use `context.Context` for all IO-bound operations.
- **Functional Options**: Use the functional options pattern for complex configurations (see `vectorstores/vectorstores.go`).
- **Interfaces**: Code against interfaces, especially when working with models and retrievers.
- **Testing**: Use `go test ./...` and check `Makefile` for specific bench or integration tests.
- **Pre-Commit Validation**: Before committing changes, ALWAYS run the Ultimate RAG integration test to ensure no regressions: `go run ./examples/qdrant-ultimate-rag/main.go`.
- **Documentation**: Exported symbols MUST have comments in GoDoc format.

## AI Assistant Role
- Help implement new LLM providers in `llms/`.
- Extend retrieval capabilities in `chains/` and `documentloaders/`.
- Automate repetitive tasks using workflows in `.agent/workflows/`.
- Ensure all new features are accompanied by tests and examples.
