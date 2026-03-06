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
	"github.com/sevigo/goframe/parsers/html"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	fmt.Println("=== RSS + HTML Parser → Vector Store Pipeline ===")

	// STEP 1: Initialize Parsers
	fmt.Println("📦 Step 1: Initializing parsers...")

	registry := parsers.NewRegistry(logger)
	if err := registry.RegisterParser(parsers.NewRSSParser()); err != nil {
		logger.Error("Failed to register RSS parser", "error", err)
		return
	}
	if err := registry.RegisterParser(html.NewHTMLParser()); err != nil {
		logger.Error("Failed to register HTML parser", "error", err)
		return
	}

	// STEP 2: Create HTML Parser for Content Transformation
	fmt.Println("🔧 Step 2: Creating HTML parser...")

	htmlParser := html.NewHTMLParser(
		html.WithBaseURL(""), // Will be set per feed
		html.WithBoilerplateRemoval(true),
		html.WithMetadataExtraction(true),
		html.WithMarkdownConversion(true),
		html.WithStructurePreservation(true),
	)

	// STEP 3: Define RSS Feeds
	feedURLs := []string{
		"https://news.ycombinator.com/rss",
		"https://feeds.bbci.co.uk/news/technology/rss.xml",
		// Add more feeds here
	}

	// STEP 4: Create RSS Loader with HTML Parser
	fmt.Println("📡 Step 3: Creating RSS loader with HTML parser...")

	loader, err := documentloaders.NewRSS(
		feedURLs,
		registry,
		documentloaders.WithHTMLParser(htmlParser), // 🔥 This is the key integration!
		documentloaders.WithRSSLogger(logger),
		documentloaders.WithRSSBatchSize(10),
		documentloaders.WithRSSWorkerCount(3),
		documentloaders.WithRSSMaxItems(50),
		documentloaders.WithRSSTimeout(30*time.Second),
		documentloaders.WithRSSMaxRetries(2),
		documentloaders.WithRSSSkipDuplicates(true),
		documentloaders.WithRSSNormalization(documentloaders.NormalizationConfig{
			StripHTML:        false, // HTML parser handles this
			RemoveTracking:   true,  // But still remove tracking params
			MaxContentLength: 20000, // Allow longer content
			MinContentLength: 50,    // 🔥 KEY: Lower threshold for HTML-parsed content
		}),
	)
	if err != nil {
		logger.Error("Failed to create RSS loader", "error", err)
		return
	}

	// STEP 5: Initialize Embedder (Ollama)
	fmt.Println("🧠 Step 4: Initializing embedder...")

	embedder, err := initializeEmbedder(ctx, logger)
	if err != nil {
		logger.Warn("Failed to initialize embedder, will skip vector store", "error", err)
		fmt.Println("\n💡 Tip: Make sure Ollama is running with: ollama serve")
		fmt.Println("💡 And pull the embedding model: ollama pull qwen3-embedding:0.6b\n")

		// Continue without vector store - just show the content
		ingestWithoutVectorStore(ctx, loader, logger)
		return
	}

	// STEP 6: Initialize Vector Store (Qdrant)
	fmt.Println("💾 Step 5: Initializing vector store...")

	vectorStore, err := initializeVectorStore(ctx, logger, embedder)
	if err != nil {
		logger.Error("Failed to create vector store", "error", err)
		return
	}

	// STEP 7: Load and Process RSS Feeds
	fmt.Println("\n🚀 Step 6: Loading RSS feeds...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	batchCount := 0
	totalDocs := 0

	err = loader.LoadAndProcessStream(ctx, func(ctx context.Context, batch []schema.Document) error {
		batchCount++
		totalDocs += len(batch)

		logger.Info("Processing batch",
			"batch_num", batchCount,
			"batch_size", len(batch),
			"total_docs", totalDocs)

		// Add to vector store
		for _, doc := range batch {
			// Show transformation
			fmt.Printf("\n📄 Document %d:\n", totalDocs)
			fmt.Printf("   Title: %v\n", doc.Metadata["title"])
			fmt.Printf("   Author: %v\n", doc.Metadata["author"])
			fmt.Printf("   Published: %v\n", doc.Metadata["published_date"])
			fmt.Printf("   Link: %v\n", doc.Metadata["link"])
			fmt.Printf("   Keywords: %v\n", doc.Metadata["keywords"])
			fmt.Printf("   Content Preview:\n   %s\n", truncate(doc.PageContent, 150))
		}

		ids, err := vectorStore.AddDocuments(ctx, batch)
		if err != nil {
			logger.Error("Failed to add documents", "error", err)
			return err
		}

		logger.Info("Ingested into vector store", "document_ids", len(ids))
		return nil
	})

	if err != nil {
		logger.Error("Failed to process feeds", "error", err)
		return
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Ingestion complete", "total_documents", totalDocs)

	// STEP 8: Perform Search
	fmt.Println("\n🔍 Step 7: Searching vector store...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	query := "artificial intelligence machine learning"
	fmt.Printf("Query: '%s'\n\n", query)

	results, err := vectorStore.SimilaritySearch(ctx, query, 3)
	if err != nil {
		logger.Error("Search failed", "error", err)
		return
	}

	fmt.Printf("Found %d results:\n\n", len(results))
	for i, result := range results {
		fmt.Printf("━━━ Result %d ━━━\n", i+1)
		fmt.Printf("Title: %v\n", result.Metadata["title"])
		fmt.Printf("Author: %v\n", result.Metadata["author"])
		fmt.Printf("Link: %v\n", result.Metadata["link"])
		if score, ok := result.Metadata["score"].(float64); ok {
			fmt.Printf("Score: %.4f\n", score)
		}
		fmt.Printf("Content Snippet:\n%s\n\n", truncate(result.PageContent, 200))
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ Pipeline completed successfully!")
}

func ingestWithoutVectorStore(ctx context.Context, loader *documentloaders.RSSLoader, logger *slog.Logger) {
	fmt.Println("\n⚠️  Running without vector store (Ollama not available)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	docs, err := loader.Load(ctx)
	if err != nil {
		logger.Error("Failed to load RSS feeds", "error", err)
		return
	}

	fmt.Printf("Loaded %d documents:\n\n", len(docs))

	for i, doc := range docs {
		if i >= 5 {
			break
		}
		fmt.Printf("━━━ Document %d ━━━\n", i+1)
		fmt.Printf("Title: %v\n", doc.Metadata["title"])
		fmt.Printf("Author: %v\n", doc.Metadata["author"])
		fmt.Printf("Published: %v\n", doc.Metadata["published_date"])
		fmt.Printf("Source: %v\n", doc.Metadata["source"])
		fmt.Printf("Keywords: %v\n", doc.Metadata["keywords"])
		fmt.Printf("\nContent (Markdown):\n%s\n\n", truncate(doc.PageContent, 300))
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	}
}

func initializeEmbedder(ctx context.Context, logger *slog.Logger) (embeddings.Embedder, error) {
	llm, err := ollama.New(
		ollama.WithModel("nomic-embed-text"), // or "qwen3-embedding:0.6b"
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
	collectionName := "rss_news_with_html_parsing"

	store, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
		qdrant.WithBatchSize(10),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant store: %w", err)
	}

	// Delete existing collection for clean demo
	_ = store.DeleteCollection(ctx, collectionName)

	// Get embedding dimension
	dim, err := embedder.GetDimension(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dimension: %w", err)
	}

	// Create new collection
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
