package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/embeddings/runpod"
)

func main() {
	// It's best practice to load credentials from environment variables.
	endpointID := os.Getenv("RUNPOD_ENDPOINT_ID")
	apiKey := os.Getenv("RUNPOD_API_KEY")

	if endpointID == "" || apiKey == "" {
		fmt.Println("Error: Please set RUNPOD_ENDPOINT_ID and RUNPOD_API_KEY environment variables.")
		os.Exit(1)
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("Starting Runpod embedder example...")

	// --- 2. Initialize the Embedder ---
	// First, create the specific Runpod client with your credentials.
	runpodClient, err := runpod.New(endpointID, apiKey, runpod.WithLogger(logger))
	if err != nil {
		logger.Error("Failed to create Runpod client", "error", err)
		os.Exit(1)
	}

	// Then, wrap it in the standard Embedder. This adds helpful preprocessing,
	// like adding "passage: " and "query: " prefixes to your text.
	embedder, err := embeddings.NewEmbedder(runpodClient)
	if err != nil {
		logger.Error("Failed to create the wrapping embedder", "error", err)
		os.Exit(1)
	}
	logger.Info("Runpod embedder initialized successfully.")

	// --- 3. Embed a Batch of Documents ---
	fmt.Println("\n--- Embedding Documents ---")
	docsToEmbed := []string{
		"The sky is blue.",
		"Grass is green.",
		"The sun is a bright star.",
	}
	fmt.Printf("Sending %d documents to the Runpod endpoint...\n", len(docsToEmbed))

	docEmbeddings, err := embedder.EmbedDocuments(ctx, docsToEmbed)
	if err != nil {
		logger.Error("Failed to embed documents", "error", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully embedded %d documents.\n", len(docEmbeddings))
	if len(docEmbeddings) > 0 {
		fmt.Printf("   Dimension of the first document vector: %d\n", len(docEmbeddings[0]))
	}

	// --- 4. Embed a Single Query ---
	fmt.Println("\n--- Embedding a Single Query ---")
	query := "What color is the sky?"
	fmt.Printf("Sending query to the Runpod endpoint: \"%s\"\n", query)

	queryEmbedding, err := embedder.EmbedQuery(ctx, query)
	if err != nil {
		logger.Error("Failed to embed query", "error", err)
		os.Exit(1)
	}

	fmt.Println("✅ Successfully embedded the query.")
	if len(queryEmbedding) > 5 {
		fmt.Printf("   Query vector (first 5 elements): %v...\n", queryEmbedding[:5])
	} else {
		fmt.Printf("   Query vector: %v\n", queryEmbedding)
	}

	fmt.Println("\nExample finished successfully.")
}
