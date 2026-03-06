package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/sevigo/goframe/documentloaders"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()

	feedURLs := []string{
		"https://news.ycombinator.com/rss",
		"https://feeds.bbci.co.uk/news/technology/rss.xml",
	}

	registry := parsers.NewRegistry(logger)
	if err := registry.RegisterParser(parsers.NewRSSParser()); err != nil {
		logger.Error("Failed to register RSS parser", "error", err)
		return
	}

	embedder, err := initializeEmbedder(ctx, logger)
	if err != nil {
		logger.Warn("Failed to initialize embedder, skipping vector store", "error", err)
	}

	loader, err := createRSSLoader(feedURLs, registry, logger)
	if err != nil {
		logger.Error("Failed to create RSS loader", "error", err)
		return
	}

	fmt.Println("\n=== Loading RSS feeds ===")
	docs, err := loader.Load(ctx)
	if err != nil {
		logger.Error("Failed to load RSS feeds", "error", err)
		return
	}

	logger.Info("Loaded documents", "count", len(docs))
	displayDocuments(docs)

	if embedder != nil {
		vectorStore, err := initializeVectorStore(ctx, logger, embedder)
		if err != nil {
			logger.Error("Failed to create vector store", "error", err)
			return
		}
		ingestToVectorStore(ctx, feedURLs, registry, vectorStore, logger)
		performSearch(ctx, vectorStore, logger)
	}

	fmt.Println("\n=== RSS ingestion complete ===")
}

func createRSSLoader(feedURLs []string, registry parsers.ParserRegistry, logger *slog.Logger) (*documentloaders.RSSLoader, error) {
	return documentloaders.NewRSS(
		feedURLs,
		registry,
		documentloaders.WithRSSLogger(logger),
		documentloaders.WithRSSBatchSize(20),
		documentloaders.WithRSSWorkerCount(5),
		documentloaders.WithRSSMaxItems(50),
		documentloaders.WithRSSTimeout(30*time.Second),
		documentloaders.WithRSSMaxRetries(3),
		documentloaders.WithRSSSkipDuplicates(true),
		documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
			StripHTML:        true,
			RemoveTracking:   true,
			MaxContentLength: 5000,
			MinContentLength: 100,
			NormalizeURLs:    true,
			MinTitleLength:   5,
			FallbackToURL:    true,
			NormalizeAuthors: true,
		}),
	)
}

func displayDocuments(docs []schema.Document) {
	for i, doc := range docs {
		if i >= 5 {
			break
		}
		fmt.Printf("\n--- Document %d ---\n", i+1)
		fmt.Printf("Title: %s\n", doc.Metadata["title"])
		fmt.Printf("Link: %s\n", doc.Metadata["link"])
		fmt.Printf("Author: %s\n", doc.Metadata["author"])
		fmt.Printf("Published: %s\n", doc.Metadata["pub_date"])
		fmt.Printf("Categories: %v\n", doc.Metadata["categories"])
		fmt.Printf("Content: %s...\n", truncate(doc.PageContent, 100))
	}
}

func ingestToVectorStore(ctx context.Context, feedURLs []string, registry parsers.ParserRegistry, vectorStore vectorstores.VectorStore, logger *slog.Logger) {
	fmt.Println("\n=== Ingesting into vector store ===")

	loader, err := documentloaders.NewRSS(
		feedURLs,
		registry,
		documentloaders.WithRSSLogger(logger),
		documentloaders.WithRSSBatchSize(50),
		documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
			StripHTML:        true,
			RemoveTracking:   true,
			MaxContentLength: 5000,
			MinContentLength: 100,
		}),
	)
	if err != nil {
		logger.Error("Failed to create RSS loader for ingestion", "error", err)
		return
	}

	batchCount := 0
	totalIngested := 0

	err = loader.LoadAndProcessStream(ctx, func(ctx context.Context, batch []schema.Document) error {
		batchCount++
		totalIngested += len(batch)

		logger.Info("Processing batch",
			"batch_num", batchCount,
			"batch_size", len(batch),
			"total", totalIngested)

		ids, addErr := vectorStore.AddDocuments(ctx, batch)
		if addErr != nil {
			return fmt.Errorf("failed to add documents: %w", addErr)
		}

		logger.Info("Ingested batch into vector store", "document_ids", len(ids))
		return nil
	})

	if err != nil {
		logger.Error("Streaming ingestion failed", "error", err)
		return
	}

	logger.Info("Streaming completed", "total_documents", totalIngested)
}

func performSearch(ctx context.Context, vectorStore vectorstores.VectorStore, logger *slog.Logger) {
	fmt.Println("\n=== Searching vector store ===")
	query := "artificial intelligence machine learning"
	results, err := vectorStore.SimilaritySearch(ctx, query, 5)
	if err != nil {
		logger.Error("Search failed", "error", err)
		return
	}

	fmt.Printf("\nSearch query: '%s'\n", query)
	fmt.Printf("Found %d results:\n", len(results))
	for i, result := range results {
		fmt.Printf("\n%d. Title: %s\n", i+1, result.Metadata["title"])
		fmt.Printf("   Link: %s\n", result.Metadata["link"])
		fmt.Printf("   Score: %.4f\n", result.Metadata["score"])
		fmt.Printf("   Content: %s...\n", truncate(result.PageContent, 150))
	}
}

func initializeEmbedder(_ context.Context, logger *slog.Logger) (embeddings.Embedder, error) {
	llm, err := ollama.New(
		ollama.WithModel("qwen3-embedding:0.6b"),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}

	embedder, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	return embedder, nil
}

func initializeVectorStore(ctx context.Context, logger *slog.Logger, embedder embeddings.Embedder) (vectorstores.VectorStore, error) {
	collectionName := "rss_feeds"

	store, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
		qdrant.WithBatchSize(50),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant store: %w", err)
	}

	_ = store.DeleteCollection(ctx, collectionName)

	dim, err := embedder.GetDimension(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dimension: %w", err)
	}

	qStore, ok := store.(*qdrant.Store)
	if !ok {
		return nil, fmt.Errorf("failed to cast store to qdrant.Store")
	}
	if err := qStore.CreateCollection(ctx, collectionName, dim); err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	logger.Info("Vector store initialized", "collection", collectionName, "dimension", dim)
	return store, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
