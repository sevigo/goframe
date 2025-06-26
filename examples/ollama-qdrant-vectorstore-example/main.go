package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	store, err := setupVectorStore(logger)
	if err != nil {
		logger.Error("Failed to setup vector store", "error", err)
		return
	}

	documents := createSampleDocuments()

	if err := demonstrateVectorStore(ctx, store, documents, logger); err != nil {
		logger.Error("Demo failed", "error", err)
		return
	}

	logger.Info("Vector store demo completed successfully")
}

func setupVectorStore(logger *slog.Logger) (vectorstores.VectorStore, error) {
	logger.Info("Initializing vector store components")

	ollamaEmbedder, err := ollama.New(
		ollama.WithModel("nomic-embed-text"),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama embedder: %w", err)
	}

	embedder, err := embeddings.NewEmbedder(ollamaEmbedder)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	collectionName := fmt.Sprintf("cities_demo_%d", time.Now().Unix())
	store, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
		qdrant.WithBatchSize(50),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant store: %w", err)
	}

	logger.Info("Vector store initialized successfully", "collection", collectionName)
	return store, nil
}

func createSampleDocuments() []schema.Document {
	return []schema.Document{
		{
			PageContent: "Tokyo is the capital of Japan and one of the world's most populous metropolitan areas.",
			Metadata: map[string]any{
				"country":    "Japan",
				"continent":  "Asia",
				"population": 37_400_000,
				"category":   "capital",
			},
		},
		{
			PageContent: "Kyoto was the former imperial capital of Japan and is famous for its temples and traditional architecture.",
			Metadata: map[string]any{
				"country":   "Japan",
				"continent": "Asia",
				"category":  "historical",
				"temples":   true,
			},
		},
		{
			PageContent: "Paris is the capital and most populous city of France, known for the Eiffel Tower and rich cultural heritage.",
			Metadata: map[string]any{
				"country":    "France",
				"continent":  "Europe",
				"population": 2_165_000,
				"category":   "capital",
				"landmarks":  []string{"Eiffel Tower", "Louvre", "Notre-Dame"},
			},
		},
		{
			PageContent: "London is the capital and largest city of England and the United Kingdom, a global financial center.",
			Metadata: map[string]any{
				"country":    "United Kingdom",
				"continent":  "Europe",
				"population": 9_540_000,
				"category":   "capital",
				"financial":  true,
			},
		},
		{
			PageContent: "Santiago is the capital and largest city of Chile, located in the country's central valley.",
			Metadata: map[string]any{
				"country":    "Chile",
				"continent":  "South America",
				"population": 6_310_000,
				"category":   "capital",
			},
		},
		{
			PageContent: "Buenos Aires is the capital and largest city of Argentina, known for its European-style architecture and tango.",
			Metadata: map[string]any{
				"country":    "Argentina",
				"continent":  "South America",
				"population": 15_150_000,
				"category":   "capital",
				"culture":    "tango",
			},
		},
		{
			PageContent: "New York City is the most populous city in the United States and a major global economic hub.",
			Metadata: map[string]any{
				"country":    "United States",
				"continent":  "North America",
				"population": 8_336_000,
				"category":   "major_city",
				"economic":   true,
			},
		},
	}
}

// demonstrateVectorStore shows various vector store operations
func demonstrateVectorStore(
	ctx context.Context,
	store vectorstores.VectorStore,
	documents []schema.Document,
	logger *slog.Logger,
) error {
	// Add documents to the vector store
	logger.Info("Adding documents to vector store", "count", len(documents))

	start := time.Now()
	documentIDs, err := store.AddDocuments(ctx, documents)
	if err != nil {
		return fmt.Errorf("failed to add documents: %w", err)
	}

	addDuration := time.Since(start)
	logger.Info("Documents added successfully",
		"count", len(documentIDs),
		"duration", addDuration,
		"docs_per_second", float64(len(documents))/addDuration.Seconds())

	time.Sleep(1 * time.Second)

	searchScenarios := []struct {
		name        string
		query       string
		numResults  int
		options     []vectorstores.Option
		expectCount int
	}{
		{
			name:        "Basic Capital Search",
			query:       "Which city is the capital of France?",
			numResults:  1,
			expectCount: 1,
		},
		{
			name:        "UK Cities",
			query:       "What is the primary city in the United Kingdom?",
			options:     []vectorstores.Option{vectorstores.WithScoreThreshold(0.7)},
			numResults:  2,
			expectCount: 1,
		},
		{
			name:  "South American Cities",
			query: "Tell me about cities in South America",
			options: []vectorstores.Option{
				vectorstores.WithFilters(map[string]any{"continent": "South America"}),
			},
			numResults:  3,
			expectCount: 2,
		},
		{
			name:       "High Score Threshold",
			query:      "European financial centers",
			numResults: 5,
			options:    []vectorstores.Option{vectorstores.WithScoreThreshold(0.7)},
		},
		{
			name:        "Cultural Cities",
			query:       "Cities known for traditional culture and temples",
			numResults:  3,
			expectCount: 1,
		},
		{
			name:        "Filter by Continent (Europe)",
			query:       "What are some major cities?",
			numResults:  3,
			options:     []vectorstores.Option{vectorstores.WithFilter("continent", "Europe")},
			expectCount: 2,
		},
	}

	for _, scenario := range searchScenarios {
		if err := runSearchScenario(ctx, store, scenario, logger); err != nil {
			return fmt.Errorf("search scenario '%s' failed: %w", scenario.name, err)
		}
	}

	return nil
}

type searchScenario struct {
	name        string
	query       string
	numResults  int
	options     []vectorstores.Option
	expectCount int
}

func runSearchScenario(ctx context.Context, store vectorstores.VectorStore, scenario searchScenario, logger *slog.Logger) error {
	logger.Info("Running search scenario",
		"name", scenario.name,
		"query", scenario.query)

	start := time.Now()

	results, err := store.SimilaritySearchWithScores(ctx, scenario.query, scenario.numResults, scenario.options...)
	if err != nil {
		return fmt.Errorf("similarity search failed: %w", err)
	}

	duration := time.Since(start)

	logger.Info("Search completed",
		"scenario", scenario.name,
		"results_found", len(results),
		"duration", duration)

	fmt.Printf("\n=== %s ===\n", scenario.name)
	fmt.Printf("Query: %q\n", scenario.query)
	fmt.Printf("Results (%d found):\n", len(results))

	for i, result := range results {
		fmt.Printf("  %d. %s (Score: %.4f)\n",
			i+1, result.Document.PageContent, result.Score)

		if country, ok := result.Document.Metadata["country"]; ok {
			fmt.Printf("     Country: %v\n", country)
		}
		if continent, ok := result.Document.Metadata["continent"]; ok {
			fmt.Printf("     Continent: %v\n", continent)
		}
	}

	if len(results) == 0 {
		fmt.Println("  No results found.")
	}

	if scenario.expectCount > 0 && len(results) != scenario.expectCount {
		logger.Warn("Unexpected result count",
			"scenario", scenario.name,
			"expected", scenario.expectCount,
			"actual", len(results))
	}

	return nil
}
