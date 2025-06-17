package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/sevigo/goframe/chains"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

// RAGExample demonstrates a complete Retrieval-Augmented Generation pipeline
// using technical documentation as the knowledge base.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Setup the RAG system
	ragChain, cleanup, err := setupRAGSystem(ctx, logger)
	if err != nil {
		logger.Error("Failed to setup RAG system", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	// Load knowledge base
	if err := loadKnowledgeBase(ctx, ragChain.Retriever.(*VectorStoreRetriever), logger); err != nil {
		logger.Error("Failed to load knowledge base", "error", err)
		os.Exit(1)
	}

	// Run interactive Q&A session
	runQASession(ctx, ragChain, logger)
}

// VectorStoreRetriever wraps a vector store to implement the Retriever interface
type VectorStoreRetriever struct {
	store vectorstores.VectorStore
}

// GetRelevantDocuments retrieves documents relevant to the query
func (r *VectorStoreRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	return r.store.SimilaritySearch(ctx, query, 3, vectorstores.WithScoreThreshold(0.6))
}

// setupRAGSystem initializes all components needed for RAG
func setupRAGSystem(ctx context.Context, logger *slog.Logger) (*chains.RetrievalQA, func(), error) {
	logger.Info("Setting up RAG system components")

	// Create embedder for document retrieval
	embedderLLM, err := ollama.New(
		ollama.WithModel("nomic-embed-text"),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create embedder LLM: %w", err)
	}

	embedder, err := embeddings.NewEmbedder(embedderLLM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// Create vector store for document storage
	collectionName := fmt.Sprintf("rag_docs_%d", time.Now().Unix())
	vectorStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	// Create LLM for answer generation
	generationLLM, err := ollama.New(
		ollama.WithModel("gemma3:1b"),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create generation LLM: %w", err)
	}

	// Create retriever and RAG chain
	retriever := &VectorStoreRetriever{store: vectorStore}
	ragChain := chains.NewRetrievalQA(retriever, generationLLM)

	// Cleanup function
	cleanup := func() {
		logger.Info("Cleaning up RAG system", "collection", collectionName)
		if err := vectorStore.DeleteCollection(ctx, collectionName); err != nil {
			logger.Warn("Failed to cleanup collection", "error", err)
		}
	}

	logger.Info("RAG system setup completed", "collection", collectionName)
	return &ragChain, cleanup, nil
}

// loadKnowledgeBase populates the vector store with sample technical documentation
func loadKnowledgeBase(ctx context.Context, retriever *VectorStoreRetriever, logger *slog.Logger) error {
	logger.Info("Loading knowledge base documents")

	// Sample technical documentation
	documents := []schema.Document{
		{
			PageContent: `Go is a programming language developed by Google. It's statically typed, compiled, and designed for simplicity and efficiency. Go features garbage collection, memory safety, and built-in concurrency support through goroutines and channels.`,
			Metadata: map[string]any{
				"topic":    "golang",
				"category": "programming",
				"source":   "go_basics.md",
			},
		},
		{
			PageContent: `Goroutines are lightweight threads in Go that are managed by the Go runtime. They are much cheaper than OS threads and allow for concurrent execution. You can start a goroutine by using the 'go' keyword before a function call.`,
			Metadata: map[string]any{
				"topic":    "concurrency",
				"category": "golang",
				"source":   "concurrency.md",
			},
		},
		{
			PageContent: `Channels in Go are used for communication between goroutines. They provide a way to pass data safely between concurrent operations. Channels can be buffered or unbuffered, and support select statements for non-blocking operations.`,
			Metadata: map[string]any{
				"topic":    "channels",
				"category": "golang",
				"source":   "channels.md",
			},
		},
		{
			PageContent: `Docker is a containerization platform that allows you to package applications and their dependencies into lightweight, portable containers. Containers share the host OS kernel and provide process isolation.`,
			Metadata: map[string]any{
				"topic":    "docker",
				"category": "devops",
				"source":   "docker_intro.md",
			},
		},
		{
			PageContent: `Kubernetes is an orchestration platform for managing containerized applications at scale. It provides features like service discovery, load balancing, rolling updates, and horizontal scaling across clusters.`,
			Metadata: map[string]any{
				"topic":    "kubernetes",
				"category": "devops",
				"source":   "k8s_overview.md",
			},
		},
		{
			PageContent: `Vector databases are specialized databases designed to store and search high-dimensional vectors efficiently. They're commonly used for AI applications like semantic search, recommendation systems, and RAG (Retrieval-Augmented Generation).`,
			Metadata: map[string]any{
				"topic":    "vector_db",
				"category": "ai",
				"source":   "vector_databases.md",
			},
		},
		{
			PageContent: `RAG (Retrieval-Augmented Generation) is a technique that combines information retrieval with language model generation. It allows LLMs to access external knowledge by retrieving relevant documents and using them as context for generating responses.`,
			Metadata: map[string]any{
				"topic":    "rag",
				"category": "ai",
				"source":   "rag_explained.md",
			},
		},
	}

	// Add documents to vector store
	start := time.Now()
	_, err := retriever.store.AddDocuments(ctx, documents)
	if err != nil {
		return fmt.Errorf("failed to add documents to vector store: %w", err)
	}

	duration := time.Since(start)
	logger.Info("Knowledge base loaded successfully",
		"document_count", len(documents),
		"duration", duration)

	return nil
}

// runQASession demonstrates various Q&A scenarios
func runQASession(ctx context.Context, ragChain *chains.RetrievalQA, logger *slog.Logger) {
	logger.Info("Starting Q&A session")

	// Sample questions covering different topics
	questions := []struct {
		category string
		question string
	}{
		{
			category: "Go Programming",
			question: "What are goroutines and how do they work?",
		},
		{
			category: "Go Concurrency",
			question: "How do channels work in Go for communication between goroutines?",
		},
		{
			category: "DevOps",
			question: "What's the difference between Docker and Kubernetes?",
		},
		{
			category: "AI/ML",
			question: "How does RAG work and why is it useful?",
		},
		{
			category: "Databases",
			question: "What are vector databases used for?",
		},
		{
			category: "Outside Knowledge",
			question: "What is the capital of Mars?", // Should admit it doesn't know
		},
	}

	for i, q := range questions {
		fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
		fmt.Printf("Question %d (%s): %s\n", i+1, q.category, q.question)
		fmt.Printf(strings.Repeat("-", 80) + "\n")

		start := time.Now()
		answer, err := ragChain.Call(ctx, q.question)
		duration := time.Since(start)

		if err != nil {
			logger.Error("Failed to get answer", "question", q.question, "error", err)
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("🤖 Answer: %s\n", answer)
		logger.Info("Question answered",
			"category", q.category,
			"duration", duration,
			"answer_length", len(answer))
	}

	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	logger.Info("Q&A session completed")
}
