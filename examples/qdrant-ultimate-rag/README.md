# Ultimate Qdrant RAG Integration Test

This example demonstrates the "ultimate" usage of the `goframe` library, integrating several core components:

1.  **Repository Ingestion**: Uses `GitLoader` to load the entire source code of this repo.
2.  **Code-Aware Splitting**: Uses `CodeAwareTextSplitter` to split files into logical chunks (functions, classes) while maintaining metadata.
3.  **Modern Qdrant Features**: Uses the refactored `Qdrant` store with **Streaming Integration**, **Binary Quantization** (RAM saving), and **Payload Indexing** (fast metadata filtering).
4.  **Local AI Infrastructure**: Uses Docker to run **Qdrant v1.16.0** and **Ollama** with **Qwen3** models (`qwen3-embedding:0.6b` and `qwen3-coder`).

## Prerequisites

-   Docker and Docker Compose
-   Go 1.25.6+

## How to Run

1.  **Start Infrastructure**:
    ```bash
    docker compose up -d
    ```
    *This will start Qdrant and Ollama, and automatically pull the required models (may take a few minutes depending on your internet connection).*

2.  **Run the Integration Test**:
    ```bash
    go run examples/qdrant-ultimate-rag/main.go
    ```

## What This Test Validates

-   **Memory Safety**: The streaming pipeline in `AddDocumentsBatch` ensures that even a large repository won't cause OOM errors.
-   **Quantization**: Verifies that the library correctly configures Qdrant's binary quantization.
-   **RAG Accuracy**: Demonstrates that the code-aware splitting and retrieval work in harmony to provide high-quality context to the LLM.
-   **Ollama Integration**: Validates the end-to-end flow with state-of-the-art Qwen3 models.

## Clean Up

To stop and remove the containers:
```bash
docker compose down
```
