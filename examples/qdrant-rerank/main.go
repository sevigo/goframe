package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()

	const (
		embModel   = "qwen3-embedding:0.6b"
		coderModel = "gemma2:2b"
	)

	// 1. Initialize Ollama Models
	embLLM, _ := ollama.New(
		ollama.WithModel(embModel),
		ollama.WithLogger(logger),
	)
	embedder, _ := embeddings.NewEmbedder(embLLM)

	rerankLLM, _ := ollama.New(
		ollama.WithModel(coderModel),
		ollama.WithLogger(logger),
	)

	// 2. Setup Vector Store (Qdrant)
	collectionName := "rerank_test"
	vStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create Qdrant store", "error", err)
		return
	}

	// 3. Ingest some test documents
	docs := []schema.Document{
		{
			PageContent: "The Apple iPhone 15 features a ceramic shield front and a titanium design.",
			Metadata:    map[string]any{"source": "mobile_tech.txt"},
		},
		{
			PageContent: "Apples are round, edible fruits produced by an apple tree (Malus domestica).",
			Metadata:    map[string]any{"source": "nature.txt"},
		},
		{
			PageContent: "A review of the latest Apple MacBook Pro with M3 Max chip.",
			Metadata:    map[string]any{"source": "laptop_tech.txt"},
		},
		{
			PageContent: "How to bake a delicious apple pie with cinnamon and sugar.",
			Metadata:    map[string]any{"source": "recipes.txt"},
		},
		{
			PageContent: "Apple Inc. is an American multinational technology company headquartered in Cupertino.",
			Metadata:    map[string]any{"source": "business.txt"},
		},
	}

	logger.Info("Ingesting documents...")
	vStore.DeleteCollection(ctx, collectionName)
	vStore.AddDocuments(ctx, docs)

	// 4. Setup Reranking Retriever
	logger.Info("Setting up RerankingRetriever")
	baseRetriever := vectorstores.ToRetriever(vStore, 4)                // Fetch 4 docs initially
	reranker := llms.NewLLMReranker(rerankLLM, llms.WithConcurrency(2)) // Pointwise reranker with 2 concurrent workers

	rr := vectorstores.RerankingRetriever{
		Retriever: baseRetriever,
		Reranker:  reranker,
		TopK:      2, // We want the best 2
	}

	// 5. Query
	query := "Tell me about Apple the tech company"
	logger.Info("Querying", "query", query)

	results, err := rr.GetRelevantScoredDocuments(ctx, query)
	if err != nil {
		logger.Error("Reranking retrieval failed", "error", err)
		return
	}

	fmt.Printf("\n=== RERANKING RESULTS for: %s ===\n", query)
	for i, sd := range results {
		fmt.Printf("[%d] Score: %.2f | Source: %s\n    Reason: %s\n    Content Snippet: %.50s...\n",
			i+1, sd.Score, sd.Metadata["source"], sd.Reason, strings.ReplaceAll(sd.PageContent, "\n", " "))
	}
}
