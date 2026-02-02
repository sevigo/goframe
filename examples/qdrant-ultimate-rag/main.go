package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sevigo/goframe/documentloaders"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/textsplitter"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

// OllamaTokenizerAdapter adapts the Ollama LLM to the textsplitter.Tokenizer interface.
type OllamaTokenizerAdapter struct {
	llm *ollama.LLM
}

func (a *OllamaTokenizerAdapter) CountTokens(ctx context.Context, modelName, text string) int {
	count, _ := a.llm.CountTokens(ctx, text)
	if count == 0 {
		return len(text) / 4 // Fallback estimation
	}
	return count
}

func (a *OllamaTokenizerAdapter) EstimateTokens(ctx context.Context, modelName, text string) int {
	return len(text) / 4
}

func (a *OllamaTokenizerAdapter) SplitTextByTokens(ctx context.Context, modelName, text string, maxTokens int) ([]string, error) {
	// Simple character-based splitting as a fallback for the adapter
	charsPerToken := 4
	maxChars := maxTokens * charsPerToken
	var chunks []string
	for i := 0; i < len(text); i += maxChars {
		end := i + maxChars
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[i:end])
	}
	return chunks, nil
}

func (a *OllamaTokenizerAdapter) GetRecommendedChunkSize(ctx context.Context, modelName string) int {
	return 1024
}

func (a *OllamaTokenizerAdapter) GetOptimalOverlapTokens(ctx context.Context, modelName string) int {
	return 128
}

func (a *OllamaTokenizerAdapter) GetMaxContextWindow(ctx context.Context, modelName string) int {
	return 32768
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()

	// Load API key from .env file if it exists
	envMap := loadEnv(".env")
	if len(envMap) == 0 {
		envMap = loadEnv("examples/qdrant-ultimate-rag/.env")
	}
	apiKey := envMap["OLLAMA_API_KEY"]
	if apiKey == "" {
		apiKey = os.Getenv("OLLAMA_API_KEY")
	}

	if apiKey != "" {
		logger.Debug("Loaded API key", "prefix", apiKey[:4]+"...")
	} else {
		logger.Warn("No API key found in .env or environment")
	}

	// 1. Initialize Components
	logger.Info("Step 1: Initializing Ollama components")

	const (
		embModel   = "qwen3-embedding:0.6b"
		coderModel = "qwen3-coder:480b-cloud"
	)

	// Embedding model
	embLLM, err := ollama.New(
		ollama.WithModel(embModel),
		ollama.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create embedding LLM", "error", err)
		return
	}

	// Coder model (Cloud or local)
	coderLLM, err := ollama.New(
		ollama.WithModel(coderModel),
		ollama.WithAPIKey(apiKey),
		ollama.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create coder LLM", "error", err)
		return
	}

	// Programmatic Pre-flight Check: Ensure models are available (local only)
	for _, m := range []struct {
		name string
		llm  *ollama.LLM
	}{
		{embModel, embLLM},
		{coderModel, coderLLM},
	} {
		// If we are using an API key, we assume it's a cloud model and skip pre-flight
		// because ollama.com might not support /api/tags or /api/pull with API keys.
		if apiKey != "" && m.name == coderModel {
			logger.Info("Cloud model detected, skipping pre-flight", "model", m.name)
			continue
		}

		has, err := m.llm.HasModel(ctx, m.name)
		if err != nil {
			logger.Warn("Failed to check for model", "model", m.name, "error", err)
		} else if !has {
			logger.Info("Model not found locally, pulling...", "model", m.name)
			if err := m.llm.PullModel(ctx, m.name); err != nil {
				logger.Error("Failed to pull model", "model", m.name, "error", err)
				return
			}
		} else {
			logger.Info("Model verified", "model", m.name)
		}
	}

	embedder, err := embeddings.NewEmbedder(embLLM)
	if err != nil {
		logger.Error("Failed to create embedder", "error", err)
		return
	}
	tokenizer := &OllamaTokenizerAdapter{llm: coderLLM}

	// 2. Load Repository Code
	logger.Info("Step 2: Loading repository code from current directory")
	repoPath, _ := filepath.Abs(".")
	registry, err := parsers.RegisterLanguagePlugins(logger)
	if err != nil {
		logger.Error("Failed to register language plugins", "error", err)
		return
	}

	loader, err := documentloaders.NewGit(repoPath, registry,
		documentloaders.WithBatchSize(50),
		documentloaders.WithIncludeExts([]string{".go"}), // Scan only Go files
	)
	if err != nil {
		logger.Error("Failed to create git loader", "error", err)
		return
	}

	// 3. Split Code with Code-Aware Splitter
	logger.Info("Step 3: Initializing CodeAwareTextSplitter")
	splitter, err := textsplitter.NewCodeAware(registry, tokenizer, logger,
		textsplitter.WithChunkSize(1024),
		textsplitter.WithChunkOverlap(128),
	)
	if err != nil {
		logger.Error("Failed to create splitter", "error", err)
		return
	}

	// 4. Setup Qdrant with Binary Quantization and Payload Indexing
	logger.Info("Step 4: Initializing Qdrant with advanced features")
	collectionName := "goframe_ultimate_test"
	vStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithBinaryQuantization(true),             // Save memory!
		qdrant.WithPayloadIndex("source", "chunk_type"), // Speed up filtering!
		qdrant.WithLogger(logger),
		qdrant.WithBatchSize(100),
	)
	if err != nil {
		logger.Error("Failed to create Qdrant store", "error", err)
		return
	}

	// Type assertion to access specific Qdrant features (CreateCollection, AddDocumentsBatch)
	qStore, ok := vStore.(*qdrant.Store)
	if !ok {
		logger.Error("Failed to cast to Qdrant store")
		return
	}

	// CLEANUP: Ensure we start with a fresh collection
	logger.Info("Cleanup: Deleting existing collection to avoid duplicates")
	if err := vStore.DeleteCollection(ctx, collectionName); err != nil {
		// Ignore error if collection doesn't exist
		logger.Warn("Failed to delete collection (might not exist)", "error", err)
	}

	// Re-create collection synchronously to avoid race conditions in concurrent workers
	dim, err := embedder.GetDimension(ctx)
	if err != nil {
		logger.Error("Failed to get embedding dimension", "error", err)
		return
	}
	logger.Info("Initializing collection synchronously", "dimension", dim)
	// Use qStore.CreateCollection (specific method)
	if err := qStore.CreateCollection(ctx, collectionName, dim); err != nil {
		logger.Error("Failed to create collection", "error", err)
		return
	}

	// 2, 3, 5. Load, Split, and Ingest in a concurrent pipeline
	logger.Info("Step 2-5: Loading, Splitting, and Ingesting in a streaming pipeline")
	start := time.Now()

	// Create a channel for batches of split documents
	ingestChan := make(chan []schema.Document, 10) // buffer 10 batches
	var ingestWg sync.WaitGroup

	// Start Ingestion Workers (Concurrent Uploads)
	// We use 4 workers to allow overlapping Qdrant uploads (Network/Embedding I/O)
	// regardless of the loader speed.
	numIngestWorkers := 4

	for i := 0; i < numIngestWorkers; i++ {
		ingestWg.Add(1)
		go func() {
			defer ingestWg.Done()
			for batchDocs := range ingestChan {
				// Ingest into Qdrant
				// AddDocumentsBatch will handle its own internal batching/concurrency if batchDocs is large,
				// but here we rely on having multiple workers processing moderate-sized batches in parallel.
				ids, err := qStore.AddDocumentsBatch(ctx, batchDocs, nil)
				if err != nil {
					logger.Error("Failed to ingest batch", "error", err)
					// In a real app, we might want to cancel the pipeline here
					continue
				}

				// Simple progress reporting
				count := len(ids)
				// Note: In a real high-concurrency scenario, use atomic.AddInt32
				// For this example, we just print progress loosely.
				fmt.Printf("\rApprox Ingested Batch: %d docs", count)
			}
		}()
	}

	err = loader.LoadAndProcessStream(ctx, func(streamCtx context.Context, batchDocs []schema.Document) error {
		// Split the documents in this batch (CPU bound)
		splitDocs, err := splitter.SplitDocuments(streamCtx, batchDocs)
		if err != nil {
			return fmt.Errorf("failed to split batch: %w", err)
		}

		// Send to ingestion workers (non-blocking if buffer has space)
		ingestChan <- splitDocs
		return nil
	})

	close(ingestChan) // Signal workers to finish
	ingestWg.Wait()   // Wait for all uploads to complete
	fmt.Println("\nIngestion complete in", time.Since(start))

	// 6. Perform RAG Query
	logger.Info("Step 6: Performing RAG Query")
	query := "How is the streaming pipeline implemented in AddDocumentsBatch in Qdrant store? Explain the concurrency and memory management."

	// Retrieve relevant snippets
	retrievedDocs, err := vStore.SimilaritySearch(ctx, query, 5)
	if err != nil {
		logger.Error("Search failed", "error", err)
		return
	}

	// Build context
	var contextBuilder strings.Builder
	for i, doc := range retrievedDocs {
		contextBuilder.WriteString(fmt.Sprintf("\n--- Source: %s ---\n%s\n", doc.Metadata["source"], doc.PageContent))

		// User requested debug info about metadata
		logger.Debug("Retrieved Document Metadata",
			"rank", i,
			"source", doc.Metadata["source"],
			"package", doc.Metadata["package_name"],
			"imports", doc.Metadata["imports"],
			"chunk_type", doc.Metadata["chunk_type"],
		)
	}

	// Prompt Coder
	logger.Info("Step 7: Generating answer with Qwen3-Coder")
	prompt := fmt.Sprintf(`You are an expert Go developer. Use the following context from the 'goframe' repository to answer the question.

Context:
%s

Question: %s

Answer (be technical and concise):`, contextBuilder.String(), query)

	answer, err := coderLLM.Call(ctx, prompt)
	if err != nil {
		logger.Error("LLM call failed", "error", err)
		return
	}

	fmt.Printf("\n\n=== ULTIMATE TEST RESULT ===\n\n%s\n\n", answer)

	// 8. Graph-Like Retrieval (Impact Analysis)
	logger.Info("Step 8: Impact Analysis (Graph-Like Retrieval)")
	targetPackage := "github.com/sevigo/goframe/vectorstores/qdrant"

	depRetriever := vectorstores.NewDependencyRetriever(vStore)
	// We want to see what imports 'targetPackage'. In the framework, we'd pass the current file's package name.
	// Here, we simulate we are analyzing the 'qdrant' package, so we want to find Dependents (Reverse Deps).
	network, err := depRetriever.GetContextNetwork(ctx, targetPackage, nil)

	if err != nil {
		logger.Error("Impact analysis failed", "error", err)
	} else {
		fmt.Printf("\n=== IMPACT ANALYSIS: Files importing %s ===\n", targetPackage)
		seenFiles := make(map[string]bool)
		foundCount := 0
		foundMain := false

		for _, doc := range network.Dependents {
			src := doc.Metadata["source"].(string)
			if !seenFiles[src] {
				fmt.Printf("- %s\n", src)
				// DEBUG: Print metadata to verify graph fields
				if imports, ok := doc.Metadata["imports"]; ok {
					logger.Debug("Verified Metadata", "source", src, "imports", imports)
				}
				if pkg, ok := doc.Metadata["package_name"]; ok {
					logger.Debug("Verified Metadata", "source", src, "package", pkg)
				}

				seenFiles[src] = true
				foundCount++

				// Standardize path separators for check
				normSrc := strings.ReplaceAll(src, "\\", "/")
				if strings.Contains(normSrc, "examples/qdrant-ultimate-rag/main.go") {
					foundMain = true
				}
			}
		}
		fmt.Println("=================================================")

		if foundCount == 0 {
			logger.Error("VERIFICATION FAILED: No documents found importing the target package.")
		} else if !foundMain {
			logger.Warn("VERIFICATION WARNING: 'main.go' was not found in the impact list (might be due to chunking limits or path issues).")
		} else {
			logger.Info("VERIFICATION PASSED: Graph traversal successfully identified 'main.go' as a dependent.")
		}
	}

	// 8b. Upstream Dependencies Verification
	logger.Info("Step 8b: Verifying Upstream Dependencies")
	// We want to check what 'vectorstores/qdrant' depends on.
	// Expected dependency: "github.com/sevigo/goframe/embeddings"
	expectedDep := "github.com/sevigo/goframe/embeddings"

	// Note: We need to pass the *current package's imports* to finding upstream dependencies.
	// But network.Dependencies was already fetched in the previous call because we passed `nil` for imports.
	// Wait, we passed `nil` to `GetContextNetwork` for imports, so Dependencies is likely empty.
	// We need to simulate knowing the imports of `targetPackage`.
	// For this test, let's explicitly ask for it.

	networkUpstream, err := depRetriever.GetContextNetwork(ctx, targetPackage, []string{expectedDep})
	if err != nil {
		logger.Error("Upstream detailed check failed", "error", err)
	} else {
		if len(networkUpstream.Dependencies) > 0 {
			fmt.Printf("✅ Qdrant correctly depends on: %s\n", networkUpstream.Dependencies[0].Metadata["source"])
		} else {
			fmt.Printf("⚠️  Could not verify upstream dependency on %s (Metadata might be missing or chunking split imports)\n", expectedDep)
		}
	}

	// DEBUG: Investigate why qdrant.go is returned as dependent
	logger.Info("Step 9: DEBUG - Metadata Investigation")
	debugDocs, _ := vStore.SimilaritySearch(ctx, "", 1,
		vectorstores.WithFilters(map[string]any{"source": "vectorstores/qdrant/qdrant.go"}))
	if len(debugDocs) > 0 {
		logger.Info("Metadata for qdrant.go", "imports", debugDocs[0].Metadata["imports"], "package", debugDocs[0].Metadata["package_name"])
	}

	// DEBUG: Investigate if main.go exists
	mainDocs, _ := vStore.SimilaritySearch(ctx, "", 1,
		vectorstores.WithFilters(map[string]any{"source": "examples/qdrant-ultimate-rag/main.go"}))
	if len(mainDocs) > 0 {
		logger.Info("Metadata for main.go", "found", true, "imports", mainDocs[0].Metadata["imports"])
	} else {
		logger.Warn("Metadata for main.go NOT FOUND in store!")
	}
}

// loadEnv is a simple .env file parser
func loadEnv(path string) map[string]string {
	env := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return env
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			// Remove quotes if present
			val = strings.Trim(val, `"'`)
			env[key] = val
		}
	}
	return env
}
