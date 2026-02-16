package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/embeddings/sparse"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
	ctx := context.Background()

	// 1. Initialize Embedder
	embedder, err := initEmbedder()
	if err != nil {
		log.Fatal(err)
	}

	// 2. Initialize Qdrant Store
	collectionName := "test-hybrid-search"
	store, err := initStore(embedder, collectionName)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Prepare and Index Documents
	if err := indexDocuments(ctx, store, collectionName); err != nil {
		log.Fatal(err)
	}

	// 4. Perform Hybrid Search
	if err := performSearch(ctx, store, "CalculateTax"); err != nil {
		log.Fatal(err)
	}
}

func initEmbedder() (embeddings.Embedder, error) {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	modelName := "nomic-embed-text"

	client, err := ollama.New(
		ollama.WithServerURL(ollamaHost),
		ollama.WithModel(modelName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	return embeddings.NewEmbedder(client)
}

func initStore(embedder embeddings.Embedder, collectionName string) (vectorstores.VectorStore, error) {
	qdrantURL, err := url.Parse("http://localhost:6334")
	if err != nil {
		return nil, fmt.Errorf("failed to parse Qdrant URL: %w", err)
	}

	return qdrant.New(
		qdrant.WithCollectionName(collectionName),
		qdrant.WithURL(*qdrantURL),
		qdrant.WithEmbedder(embedder),
		qdrant.WithSparseVector("bow_sparse"),
	)
}

func indexDocuments(ctx context.Context, store vectorstores.VectorStore, collectionName string) error {
	fmt.Printf("Cleaning up collection %s...\n", collectionName)
	if err := store.DeleteCollection(ctx, collectionName); err != nil {
		log.Printf("Warning: could not delete existing collection: %v", err)
	}

	texts := []string{
		"func CalculateTax(income float64) float64 { return income * 0.2 }",
		"func CalculateRebate(income float64) float64 { return income * 0.05 }",
		"taxation is a theft, said the libertarian",
		"rebates are good for the economy",
	}

	var docs []schema.Document
	for _, text := range texts {
		sparseVec, err := sparse.GenerateSparseVector(ctx, text)
		if err != nil {
			log.Printf("Warning: failed to generate sparse vector for '%s': %v", text, err)
			continue
		}

		doc := schema.NewDocument(text, map[string]any{
			"source": "example_test",
		})
		doc.Sparse = sparseVec
		docs = append(docs, doc)
	}

	fmt.Println("Indexing documents...")
	if _, err := store.AddDocuments(ctx, docs); err != nil {
		return fmt.Errorf("failed to add documents: %w", err)
	}

	time.Sleep(1 * time.Second)
	return nil
}

func performSearch(ctx context.Context, store vectorstores.VectorStore, query string) error {
	fmt.Printf("\nPerforming Hybrid Search for query: '%s'\n", query)

	querySparseVec, err := sparse.GenerateSparseVector(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to generate sparse query: %w", err)
	}

	results, err := store.SimilaritySearch(ctx, query, 3,
		vectorstores.WithSparseQuery(querySparseVec),
	)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	fmt.Println("Results:")
	for i, res := range results {
		fmt.Printf("%d. %s\n", i+1, res.PageContent)
	}

	return nil
}
