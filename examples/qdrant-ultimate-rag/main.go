package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sevigo/goframe/documentloaders"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/textsplitter"
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

	// 1. Initialize Components
	logger.Info("Step 1: Initializing Ollama components")

	// Embedding model
	embLLM, err := ollama.New(
		ollama.WithModel("qwen3-embedding:0.6b"),
		ollama.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create embedding LLM", "error", err)
		return
	}

	// Coder model (Cloud class or local)
	coderLLM, err := ollama.New(
		ollama.WithModel("qwen3-coder"),
		ollama.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create coder LLM", "error", err)
		return
	}

	embedder, _ := embeddings.NewEmbedder(embLLM)
	tokenizer := &OllamaTokenizerAdapter{llm: coderLLM}

	// 2. Load Repository Code
	logger.Info("Step 2: Loading repository code from current directory")
	repoPath, _ := filepath.Abs(".")
	registry := parsers.NewRegistry(logger)
	loader, err := documentloaders.NewGit(repoPath, registry,
		documentloaders.WithBatchSize(50),
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

	// Type assertion to access AddDocumentsBatch with progress callback
	qStore, ok := vStore.(*qdrant.Store)
	if !ok {
		logger.Error("Failed to cast to Qdrant store")
		return
	}

	// 2, 3, 5. Load, Split, and Ingest in a stream
	logger.Info("Step 2-5: Loading, Splitting, and Ingesting in a streaming pipeline")
	start := time.Now()

	totalProcessed := 0
	err = loader.LoadAndProcessStream(ctx, func(streamCtx context.Context, batchDocs []schema.Document) error {
		// Split the documents in this batch
		splitDocs, err := splitter.SplitDocuments(streamCtx, batchDocs)
		if err != nil {
			return fmt.Errorf("failed to split batch: %w", err)
		}

		// Ingest into Qdrant (AddDocumentsBatch handles internal concurrency and retry)
		// We use a simplified AddDocuments here or simply AddDocumentsBatch
		// Since we already have a batch, AddDocuments is sufficient,
		// but AddDocumentsBatch is safe too.
		processedIDs, err := qStore.AddDocumentsBatch(streamCtx, splitDocs, nil)
		if err != nil {
			return fmt.Errorf("failed to ingest batch: %w", err)
		}

		totalProcessed += len(processedIDs)
		fmt.Printf("\rIngested: %d documents (Elapsed: %v)", totalProcessed, time.Since(start).Round(time.Second))
		return nil
	})

	if err != nil {
		logger.Error("Streaming pipeline failed", "error", err)
		return
	}
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
	for _, doc := range retrievedDocs {
		contextBuilder.WriteString(fmt.Sprintf("\n--- Source: %s ---\n%s\n", doc.Metadata["source"], doc.PageContent))
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
}
