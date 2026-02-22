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

	apiKey := loadAPIKey(logger)

	// 1. Initialize Components
	const (
		embModel   = "qwen3-embedding:0.6b"
		coderModel = "qwen3-coder:480b-cloud"
	)

	embLLM, coderLLM, err := initModels(ctx, logger, apiKey, embModel, coderModel)
	if err != nil {
		logger.Error("Initialization failed", "error", err)
		return
	}

	embedder, _ := embeddings.NewEmbedder(embLLM)
	tokenizer := &OllamaTokenizerAdapter{llm: coderLLM}
	registry, _ := parsers.RegisterLanguagePlugins(logger)

	// 2. Initialize Store
	collectionName := "goframe_ultimate_test"
	vStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithBinaryQuantization(true),
		qdrant.WithPayloadIndex("source", "chunk_type"),
		qdrant.WithLogger(logger),
		qdrant.WithBatchSize(100),
	)
	if err != nil {
		logger.Error("Failed to create store", "error", err)
		return
	}

	// 3. Ingest Repository
	if err := ingestRepository(ctx, logger, vStore, registry, tokenizer, collectionName); err != nil {
		logger.Error("Ingestion failed", "error", err)
		return
	}

	// 4. Perform RAG Query
	query := "How is the streaming pipeline implemented in AddDocumentsBatch in Qdrant store? Explain the concurrency and memory management."
	if err := runRAGQuery(ctx, logger, vStore, coderLLM, query); err != nil {
		logger.Error("RAG query failed", "error", err)
	}

	// 5. Impact Analysis
	targetPackage := "github.com/sevigo/goframe/vectorstores/qdrant"
	if err := runImpactAnalysis(ctx, logger, vStore, targetPackage); err != nil {
		logger.Error("Impact analysis failed", "error", err)
	}
}

func runRAGQuery(ctx context.Context, logger *slog.Logger, vStore vectorstores.VectorStore, coderLLM *ollama.LLM, query string) error {
	logger.Info("Step 4: Performing RAG Query")

	// Retrieve relevant snippets
	retrievedDocs, err := vStore.SimilaritySearch(ctx, query, 5)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Build context
	var contextBuilder strings.Builder
	for i, doc := range retrievedDocs {
		contextBuilder.WriteString(fmt.Sprintf("\n--- Source: %s ---\n%s\n", doc.Metadata["source"], doc.PageContent))

		logger.Debug("Retrieved Document Metadata",
			"rank", i,
			"source", doc.Metadata["source"],
			"package", doc.Metadata["package_name"],
			"imports", doc.Metadata["imports"],
			"chunk_type", doc.Metadata["chunk_type"],
		)
	}

	// Prompt Coder
	logger.Info("Step 5: Generating answer with Qwen3-Coder")
	prompt := fmt.Sprintf(`You are an expert Go developer. Use the following context from the 'goframe' repository to answer the question.

Context:
%s

Question: %s

Answer (be technical and concise):`, contextBuilder.String(), query)

	answer, err := coderLLM.Call(ctx, prompt)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	fmt.Printf("\n\n=== ULTIMATE TEST RESULT ===\n\n%s\n\n", answer)
	return nil
}

func runImpactAnalysis(ctx context.Context, logger *slog.Logger, vStore vectorstores.VectorStore, targetPackage string) error {
	logger.Info("Step 6: Impact Analysis (Graph-Like Retrieval)")

	depRetriever, err := vectorstores.NewDependencyRetriever(vStore)
	if err != nil {
		return fmt.Errorf("failed to create dependency retriever: %w", err)
	}
	network, err := depRetriever.GetContextNetwork(ctx, targetPackage, nil)
	if err != nil {
		return fmt.Errorf("impact analysis failed: %w", err)
	}

	fmt.Printf("\n=== IMPACT ANALYSIS: Files importing %s ===\n", targetPackage)
	seenFiles := make(map[string]bool)
	foundCount := 0
	foundMain := false

	for _, doc := range network.Dependents {
		src, _ := doc.Metadata["source"].(string)
		if !seenFiles[src] {
			fmt.Printf("- %s\n", src)
			seenFiles[src] = true
			foundCount++

			normSrc := strings.ReplaceAll(src, "\\", "/")
			if strings.Contains(normSrc, "examples/qdrant-ultimate-rag/main.go") {
				foundMain = true
			}
		}
	}
	fmt.Println("=================================================")

	switch {
	case foundCount == 0:
		logger.Error("VERIFICATION FAILED: No documents found importing the target package.")
	case !foundMain:
		logger.Warn("VERIFICATION WARNING: 'main.go' was not found in the impact list.")
	default:
		logger.Info("VERIFICATION PASSED: Graph traversal successfully identified 'main.go' as a dependent.")
	}
	return nil
}

func loadAPIKey(logger *slog.Logger) string {
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
	return apiKey
}

func initModels(ctx context.Context, logger *slog.Logger, apiKey, embModel, coderModel string) (*ollama.LLM, *ollama.LLM, error) {
	embLLM, err := ollama.New(
		ollama.WithModel(embModel),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create embedding LLM: %w", err)
	}

	coderLLM, err := ollama.New(
		ollama.WithModel(coderModel),
		ollama.WithAPIKey(apiKey),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create coder LLM: %w", err)
	}

	// Pre-flight Check
	for _, m := range []struct {
		name string
		llm  *ollama.LLM
	}{
		{embModel, embLLM},
		{coderModel, coderLLM},
	} {
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
				return nil, nil, fmt.Errorf("failed to pull model %s: %w", m.name, err)
			}
		}
	}

	return embLLM, coderLLM, nil
}

func ingestRepository(ctx context.Context, logger *slog.Logger, vStore vectorstores.VectorStore, registry parsers.ParserRegistry, tokenizer textsplitter.Tokenizer, collectionName string) error {
	repoPath, _ := filepath.Abs(".")
	loader, err := documentloaders.NewGit(repoPath, registry,
		documentloaders.WithBatchSize(50),
		documentloaders.WithIncludeExts([]string{".go"}),
	)
	if err != nil {
		return fmt.Errorf("failed to create git loader: %w", err)
	}

	splitter, err := textsplitter.NewCodeAware(registry, tokenizer, logger,
		textsplitter.WithChunkSize(1024),
		textsplitter.WithChunkOverlap(128),
	)
	if err != nil {
		return fmt.Errorf("failed to create splitter: %w", err)
	}

	if delErr := vStore.DeleteCollection(ctx, collectionName); delErr != nil {
		logger.Warn("Failed to delete collection (might not exist)", "error", delErr)
	}

	qStore, _ := vStore.(*qdrant.Store)
	embedder := qStore.GetEmbedder()
	dim, dimErr := embedder.GetDimension(ctx)
	if dimErr != nil {
		return fmt.Errorf("failed to get dimension: %w", dimErr)
	}

	if createErr := qStore.CreateCollection(ctx, collectionName, dim); createErr != nil {
		return fmt.Errorf("failed to create collection: %w", createErr)
	}

	ingestChan := make(chan []schema.Document, 10)
	var ingestWg sync.WaitGroup
	numIngestWorkers := 4

	for range numIngestWorkers {
		ingestWg.Add(1)
		go func() {
			defer ingestWg.Done()
			for batchDocs := range ingestChan {
				if _, ingestErr := qStore.AddDocumentsBatch(ctx, batchDocs, nil); ingestErr != nil {
					logger.Error("Failed to ingest batch", "error", ingestErr)
				}
			}
		}()
	}

	err = loader.LoadAndProcessStream(ctx, func(streamCtx context.Context, batchDocs []schema.Document) error {
		splitDocs, splitErr := splitter.SplitDocuments(streamCtx, batchDocs)
		if splitErr != nil {
			return fmt.Errorf("failed to split batch: %w", splitErr)
		}
		ingestChan <- splitDocs
		return nil
	})

	close(ingestChan)
	ingestWg.Wait()
	return err
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
