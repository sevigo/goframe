# Qdrant Vector Store Example with Ollama Embeddings

This example demonstrates how to use the Qdrant vector store with Ollama for semantic search. The program initializes the store, adds a sample dataset of cities, performs several types of queries, and then cleans up the created resources.

## Features Demonstrated

-   Initializing the Qdrant store with an Ollama embedder.
-   Creating a collection automatically.
-   Adding documents and generating embeddings with `nomic-embed-text`.
-   Performing similarity searches.
-   Applying metadata filters to narrow down search results.
-   Using a score threshold to filter by relevance.
-   Deleting documents from the collection.

## Prerequisites

Before running the example, ensure you have the following installed and running:

1.  **Go:** Version 1.21 or later.
2.  **Ollama:** The Ollama server must be running.
3.  **Qdrant:** The easiest way to run Qdrant is with Docker:
    ```bash
    docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
    ```

## Setup

1.  **Pull the Embedding Model:**
    The example uses the `nomic-embed-text` model. Pull it from Ollama:
    ```bash
    ollama pull nomic-embed-text
    ```

2.  **Install Go Dependencies:**
    Navigate to the root of the `goframe` project and run:
    ```bash
    go mod tidy
    ```

## How to Run

Execute the `main.go` file from the root of the project:

```bash
go run ./examples/ollama-qdrant-vectorstore-example/main.go
```

## Expected Output

You will see a series of logs indicating the progress of the demo:

1.  Initialization of Ollama and Qdrant components.
2.  Creation of a new collection (e.g., `cities_demo_...`).
3.  Successful addition of 7 documents.
4.  A series of search scenarios will be executed, each with its own header and results printed to the console (e.g., `=== Basic Capital Search ===`).
5.  Logs showing that the metadata filter for "Europe" worked correctly.
6.  A demonstration of document deletion.
7.  Finally, a cleanup message indicating the test collection has been deleted.
8.  The program will exit with the message: `Vector store demo completed successfully`.