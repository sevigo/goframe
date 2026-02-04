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

	// 1. Initialize Ollama for Dense Embeddings
	// Ensure you have ollama running locally or set OLLAMA_HOST
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	modelName := "nomic-embed-text" // A good default for embeddings

	client, err := ollama.New(
		ollama.WithServerURL(ollamaHost),
		ollama.WithModel(modelName),
	)
	if err != nil {
		log.Fatalf("Failed to create ollama client: %v", err)
	}

	embedder, err := embeddings.NewEmbedder(client)
	if err != nil {
		log.Fatalf("Failed to create embedder: %v", err)
	}

	// 2. Initialize Qdrant with Hybrid Support
	collectionName := "test-hybrid-search"

	qdrantURL, err := url.Parse("http://localhost:6334")
	if err != nil {
		log.Fatalf("Failed to parse Qdrant URL: %v", err)
	}

	store, err := qdrant.New(
		qdrant.WithCollectionName(collectionName),
		qdrant.WithURL(*qdrantURL), // Pass dereferenced URL struct
		qdrant.WithEmbedder(embedder),
		qdrant.WithSparseVector("bow_sparse"),
	)
	if err != nil {
		log.Fatalf("Failed to create Qdrant store: %v", err)
	}

	// Clean up previous runs
	// In a real app you might not want to check this way, but for a test it's fine.
	// We'll just define a unique collection name or assume we can recreate.
	// The qdrant.New call typically ensures the collection exists.
	// Let's verify connection first by trying to index something.

	// 3. Prepare Documents
	// We'll create documents where dense search might be ambiguous but sparse (keyword) is precise.
	texts := []string{
		"func CalculateTax(income float64) float64 { return income * 0.2 }",
		"func CalculateRebate(income float64) float64 { return income * 0.05 }",
		"taxation is a theft, said the libertarian",
		"rebates are good for the economy",
	}

	var docs []schema.Document
	for _, text := range texts {
		// Generate Sparse Vector
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

	// 4. Index Documents
	fmt.Println("Indexing documents...")
	ids, err := store.AddDocuments(ctx, docs)
	if err != nil {
		log.Fatalf("Failed to add documents: %v", err)
	}
	fmt.Printf("Successfully indexed %d documents\n", len(ids))

	// Allow some time for Qdrant to index (usually near-instant for small batches but good practice)
	time.Sleep(1 * time.Second)

	// 5. Perform Hybrid Search
	// Query: "CalculateTax" - should strongly match the first document via sparse vector (exact keyword)
	query := "CalculateTax"
	fmt.Printf("\nPerforming Hybrid Search for query: '%s'\n", query)

	// Generate sparse vector for the query
	querySparseVec, err := sparse.GenerateSparseVector(ctx, query)
	if err != nil {
		log.Fatalf("Failed to generate sparse query: %v", err)
	}

	// Search
	results, err := store.SimilaritySearch(ctx, query, 3,
		vectorstores.WithSparseQuery(querySparseVec),
	)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	// 6. Verify Results
	fmt.Println("Results:")
	for i, res := range results {
		fmt.Printf("%d. %s\n", i+1, res.PageContent)
	}

	// Basic assertion
	if len(results) > 0 && results[0].PageContent == texts[0] {
		fmt.Println("\n✅ SUCCESS: Top result matches expected document.")
	} else {
		fmt.Println("\n❌ FAILURE: Top result did not match expected document.")
	}
}
