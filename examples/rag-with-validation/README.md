# Gemini and Ollama: A Validated RAG Example with MS-MARCO

This example demonstrates an advanced Retrieval-Augmented Generation (RAG) pattern that uses two different LLMs: a fast, local Ollama model for validation, and a powerful Google Gemini model for final answer generation. It uses the **MS-MARCO dataset** to provide a real-world knowledge base and a diverse set of questions for testing.

## The Concept: Validated RAG

Standard RAG pipelines retrieve documents and immediately use them as context. This can be inefficient if the retrieved documents are irrelevant. This example implements a "Validated RAG" pipeline:

1.  **Retrieve**: Fetch relevant passages from a vector store (Qdrant) populated with the MS-MARCO dataset.
2.  **Validate**: Use a fast, local LLM (Ollama with `gemma3:1b`) to check if the retrieved context is actually relevant to the user's question. This acts as a "gatekeeper."
3.  **Generate**:
    -   If the context is **valid**, send it along with the question to a powerful generator LLM (Google Gemini) for a high-quality, context-aware answer.
    -   If the context is **invalid**, discard it and send only the question to Gemini, allowing it to use its general knowledge.

This pattern improves accuracy by preventing "context stuffing" with irrelevant information and can reduce costs by avoiding unnecessary work by the larger model.

## Features Demonstrated

-   Using multiple, distinct LLMs for different tasks in a single pipeline.
-   Implementing a custom chain (`ValidatingRetrievalChain`) with conditional logic.
-   Ingesting a real-world dataset (MS-MARCO) into a vector store.
-   Testing the pipeline against a random sample of 10 questions from the dataset.

## Prerequisites

-   Go 1.21+
-   **Ollama**: Must be running locally.
-   **Qdrant**: Must be running locally (e.g., via Docker).
-   **Gemini API Key**: Your key must be set as an environment variable.
-   **MS-MARCO Dataset**: The example expects the dataset to be at `testdata/rag_dataset/msmarco/ms_marco_1000.csv`. Make sure this file exists.

## Setup

1.  **Start Services:**
    ```bash
    # Start Qdrant
    docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant

    # Ensure Ollama is running
    ```

2.  **Pull Ollama Models:**
    This example uses two models from Ollama.
    ```bash
    ollama pull nomic-embed-text
    ollama pull gemma3:1b
    ```

3.  **Set Gemini API Key:**
    ```bash
    export GEMINI_API_KEY="YOUR_API_KEY"
    ```

## How to Run

Execute the `main.go` file from the root of the project:

```bash
go run ./examples/rag-with-validation/main.go