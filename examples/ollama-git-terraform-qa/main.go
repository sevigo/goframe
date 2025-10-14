package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/sevigo/goframe/chains"
	"github.com/sevigo/goframe/documentloaders"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/gitutil"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

const (
	repoURL       = "https://github.com/terraform-aws-modules/terraform-aws-s3-bucket.git"
	question      = "How is the bucket name for the 'analytics_configuration_bucket' module constructed?"
	groundTruth   = "random_pet"
	genModel      = "gemma3:latest"
	embedderModel = "nomic-embed-text:latest"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	ragChain, vectorStore, parserRegistry, err := setupRAGSystem(logger)
	if err != nil {
		logger.Error("Failed to setup RAG system", "error", err)
		return
	}

	logger.Info("Cloning remote repository", "url", repoURL)
	cloner := gitutil.NewCloner(logger)
	tempRepoPath, cleanup, err := cloner.Clone(ctx, repoURL)
	if err != nil {
		logger.Error("Failed to clone repository", "error", err)
		return
	}
	defer cleanup()
	logger.Info("Repository cloned successfully", "path", tempRepoPath)

	err = indexTerraformRepo(ctx, tempRepoPath, vectorStore, parserRegistry, logger)
	if err != nil {
		logger.Error("Failed to index Terraform repository", "error", err)
		return
	}

	askAndCheck(ctx, ragChain, question, groundTruth, logger)
}

func setupRAGSystem(logger *slog.Logger) (*chains.RetrievalQA, vectorstores.VectorStore, parsers.ParserRegistry, error) {
	logger.Info("Setting up RAG system components...")

	embedderLLM, err := ollama.New(
		ollama.WithModel(embedderModel),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create embedder LLM: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(embedderLLM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	collectionName := fmt.Sprintf("terraform-rag-demo-%d", time.Now().Unix())
	vectorStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create vector store: %w", err)
	}
	logger.Info("Vector store initialized", "collection", collectionName)

	generatorLLM, err := ollama.New(
		ollama.WithModel(genModel),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create generator LLM: %w", err)
	}

	parserRegistry, err := parsers.RegisterLanguagePlugins(logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to register parsers: %w", err)
	}

	retriever := vectorstores.ToRetriever(vectorStore, 4)
	ragChain := chains.NewRetrievalQA(retriever, generatorLLM)

	logger.Info("RAG system setup complete.")
	return &ragChain, vectorStore, parserRegistry, nil
}

func indexTerraformRepo(ctx context.Context, repoPath string, store vectorstores.VectorStore, registry parsers.ParserRegistry, logger *slog.Logger) error {
	logger.Info("Starting to index Terraform files from repository...")

	gitLoader, err := documentloaders.NewGit(
		repoPath,
		registry,
		documentloaders.WithIncludeExts([]string{"tf"}),
		documentloaders.WithLogger(logger),
	)
	if err != nil {
		return fmt.Errorf("failed to create git loader: %w", err)
	}

	docs, err := gitLoader.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load documents from git repo: %w", err)
	}
	logger.Info("Documents loaded and parsed", "initial_count", len(docs))

	validDocs := make([]schema.Document, 0, len(docs))
	for _, doc := range docs {
		if strings.TrimSpace(doc.PageContent) != "" {
			validDocs = append(validDocs, doc)
		} else {
			source := doc.Metadata["source"]
			logger.Warn("Filtering out empty document chunk", "source", source)
		}
	}

	if len(validDocs) == 0 {
		return errors.New("no valid, non-empty .tf documents found to index")
	}
	logger.Info("Filtered documents", "valid_count", len(validDocs))

	logger.Info("Adding documents to vector store...")
	start := time.Now()
	_, err = store.AddDocuments(ctx, validDocs)
	if err != nil {
		return fmt.Errorf("failed to add documents to vector store: %w", err)
	}

	logger.Info("Indexing complete", "duration", time.Since(start))
	return nil
}

func askAndCheck(ctx context.Context, ragChain *chains.RetrievalQA, q, truth string, logger *slog.Logger) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	logger.Info("Asking question to the RAG system")
	fmt.Printf("❓ Question: %s\n", q)
	fmt.Printf("%s\n", strings.Repeat("-", 80))

	start := time.Now()
	answer, err := ragChain.Call(ctx, q)
	duration := time.Since(start)

	if err != nil {
		logger.Error("Failed to get answer from RAG chain", "error", err)
		return
	}

	fmt.Printf("🤖 Answer: %s\n", answer)
	logger.Info("Answer received", "duration", duration)

	fmt.Printf("%s\n", strings.Repeat("-", 80))
	logger.Info("Verifying the answer...")

	if checkAnswer(answer, truth) {
		fmt.Printf("✅ Verification PASSED: The answer contains the correct bucket name ('%s').\n", truth)
	} else {
		fmt.Printf("❌ Verification FAILED: The answer does NOT contain the correct bucket name ('%s').\n", truth)
	}
	fmt.Printf("%s\n", strings.Repeat("=", 80))
}

func checkAnswer(answer, groundTruth string) bool {
	return strings.Contains(answer, groundTruth)
}
