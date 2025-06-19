package main

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/llms/gemini"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

// llama3.2:latest, gemma3:latest
//
// Interesting idea:
// Keep fast model for most cases: Use gemma3:12b with improved prompts
// Add confidence scoring: If the fast model seems uncertain, fall back to thinking model
// Use thinking model selectively: For complex queries or when validation confidence is low
const (
	maxRandomQuestion = 10
	generatorModel    = "gemini-2.5-flash"
	validatorModel    = "gemma3:latest"
	embedderModel     = "nomic-embed-text"
	datasetPath       = "testdata/rag_dataset/msmarco/ms_marco_1000.csv"
)

// EvaluationCase represents a single test case from the MS-MARCO dataset
type EvaluationCase struct {
	QueryID           int
	QueryText         string
	GroundTruthAnswer string
	RelevantPassage   string
}

// ValidatingRetrievalChain implements a RAG pipeline with a validation step.
type ValidatingRetrievalChain struct {
	Retriever    schema.Retriever
	ValidatorLLM llms.Model
	GeneratorLLM llms.Model
	Logger       *slog.Logger
}

// Call executes the retrieve-validate-generate pipeline.
func (c *ValidatingRetrievalChain) Call(ctx context.Context, query string) (string, error) {
	c.Logger.Info("Retrieving documents...")
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	if len(docs) == 0 {
		c.Logger.Info("No documents found. Falling back to direct generation.")
		return c.GeneratorLLM.Call(ctx, query)
	}

	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.PageContent
	}
	contextStr := strings.Join(docContents, "\n\n---\n\n")
	c.Logger.Info("Retrieved context", "content", truncate(contextStr, 350))

	c.Logger.Info("Validating context with Ollama...")
	validationPrompt := buildValidationPrompt(contextStr, query)
	validationResponse, err := c.ValidatorLLM.Call(ctx, validationPrompt)
	if err != nil {
		return "", fmt.Errorf("validation call failed: %w", err)
	}
	validationResponse = strings.TrimSuffix(validationResponse, "\n")
	c.Logger.Info("Ollama validation response", "response", validationResponse)

	isRelevant := strings.Contains(strings.ToLower(validationResponse), "yes")
	if isRelevant {
		ragPrompt := buildRAGPrompt(contextStr, query)
		return c.GeneratorLLM.Call(ctx, ragPrompt)
	}
	c.Logger.Info("Context is NOT RELEVANT. Generating answer with Gemini without context.")
	return c.GeneratorLLM.Call(ctx, query)
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if os.Getenv("GEMINI_API_KEY") == "" {
		logger.Error("Please set the GEMINI_API_KEY environment variable.")
		return
	}

	// Load dataset
	allPassages, evaluationSet, err := loadMSMARCODataset(datasetPath, logger)
	if err != nil {
		logger.Error("Failed to load MS MARCO dataset", "error", err)
		return
	}

	// Setup the RAG system
	ragChain, vectorStore, cleanup, err := setupRAGSystem(ctx, logger)
	if err != nil {
		logger.Error("Failed to setup RAG system", "error", err)
		return
	}
	defer cleanup()

	exists, err := checkCollectionExists(ctx, vectorStore, logger)
	if err != nil {
		logger.Error("Failed to check collection status", "error", err)
		return
	}

	if !exists {
		logger.Info("Collection doesn't exist or is empty, loading knowledge base...")
		if err := loadKnowledgeBase(ctx, vectorStore, allPassages, logger); err != nil {
			logger.Error("Failed to load knowledge base", "error", err)
			return
		}
	} else {
		logger.Info("Using existing knowledge base")
	}

	// Run interactive Q&A session with random questions from the dataset
	runQASession(ctx, ragChain, evaluationSet, logger)
}

// checkCollectionExists checks if the collection exists and has documents
func checkCollectionExists(ctx context.Context, store vectorstores.VectorStore, logger *slog.Logger) (bool, error) {
	testQuery := "test"
	docs, err := store.SimilaritySearch(ctx, testQuery, 1)
	if errors.Is(err, vectorstores.ErrCollectionNotFound) {
		logger.Info("Collection appears to be empty or doesn't exist")
		return false, nil
	}

	exists := len(docs) > 0
	if exists {
		logger.Info("Found existing collection with documents", "sample_docs", len(docs))
	} else {
		logger.Info("Collection exists but appears empty")
	}
	return exists, nil
}

// setupRAGSystem initializes all components needed for the validated RAG pipeline.
func setupRAGSystem(ctx context.Context, logger *slog.Logger) (*ValidatingRetrievalChain, vectorstores.VectorStore, func(), error) {
	logger.Info("Setting up RAG system components")

	// Embedder (Ollama)
	embedderLLM, err := ollama.New(ollama.WithModel(embedderModel), ollama.WithLogger(logger))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create embedder LLM: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(embedderLLM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// Vector Store (Qdrant)
	collectionName := "validated_rag_msmarco"
	vectorStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create vector store: %w", err)
	}
	retriever := vectorstores.ToRetriever(
		vectorStore,
		1, // number of documents
		vectorstores.WithScoreThreshold(0.5),
	)

	validatorLLM, err := ollama.New(ollama.WithModel(validatorModel), ollama.WithLogger(logger))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create validator LLM: %w", err)
	}

	// Generator LLM (Gemini - powerful)
	generatorLLM, err := gemini.New(ctx, gemini.WithModel(generatorModel), gemini.WithLogger(logger))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create generator LLM: %w", err)
	}

	// Create the chain
	chain := &ValidatingRetrievalChain{
		Retriever:    retriever,
		ValidatorLLM: validatorLLM,
		GeneratorLLM: generatorLLM,
		Logger:       logger,
	}

	cleanup := func() {
		logger.Info("Cleaning up RAG system", "collection", collectionName)
	}

	logger.Info("RAG system setup completed", "collection", collectionName)
	return chain, vectorStore, cleanup, nil
}

// loadKnowledgeBase populates the vector store with passages from the dataset.
func loadKnowledgeBase(ctx context.Context, store vectorstores.VectorStore, passages []string, logger *slog.Logger) error {
	logger.Info("Loading knowledge base documents", "passage_count", len(passages))
	start := time.Now()

	documents := make([]schema.Document, len(passages))
	for i, passage := range passages {
		documents[i] = schema.NewDocument(passage, map[string]any{"source": "msmarco"})
	}

	_, err := store.AddDocuments(ctx, documents)
	if err != nil {
		return fmt.Errorf("failed to add documents to vector store: %w", err)
	}

	duration := time.Since(start)
	logger.Info("Knowledge base loaded successfully", "duration", duration)
	return nil
}

// runQASession demonstrates various Q&A scenarios using random questions from the dataset.
func runQASession(ctx context.Context, ragChain *ValidatingRetrievalChain, evaluationSet []EvaluationCase, logger *slog.Logger) {
	if len(evaluationSet) == 0 {
		logger.Error("Evaluation set is empty, cannot run Q&A session.")
		return
	}
	logger.Info("Starting Q&A session with 10 random questions from MS-MARCO...")

	for i := range maxRandomQuestion {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(evaluationSet))))
		if err != nil {
			logger.Error("Failed to generate random number", "error", err)
			continue
		}
		evalCase := evaluationSet[randomIndex.Int64()]

		fmt.Printf("\n%s\n", strings.Repeat("=", 80))
		fmt.Printf("Random Question %d/%d: %s\n", i+1, maxRandomQuestion, evalCase.QueryText)
		fmt.Printf("Ground Truth Answer Hint: %s\n", truncate(evalCase.GroundTruthAnswer, 350))
		fmt.Printf("%s\n", strings.Repeat("-", 80))

		answer, err := ragChain.Call(ctx, evalCase.QueryText)
		if err != nil {
			logger.Error("Failed to get answer", "question", evalCase.QueryText, "error", err)
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}
		if answer != "" {
			fmt.Printf("✅ Final Answer: %s\n", answer)
		}
	}
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	logger.Info("Q&A session completed")
}

// loadMSMARCODataset loads and processes the MS MARCO CSV dataset.
func loadMSMARCODataset(path string, logger *slog.Logger) ([]string, []EvaluationCase, error) {
	logger.Info("Loading MS MARCO dataset", "path", path)
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open dataset file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	if _, err := reader.Read(); err != nil { // Skip header
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	tempEvalCases := make(map[int]EvaluationCase)
	passageSet := make(map[string]struct{})

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(record) < 8 {
			continue
		}

		queryID, _ := strconv.Atoi(strings.TrimSpace(record[0]))
		queryText := strings.TrimSpace(record[1])
		answers := strings.TrimSpace(record[3])
		isSelected, _ := strconv.Atoi(strings.TrimSpace(record[6]))
		passageText := strings.TrimSpace(record[7])

		if passageText != "" {
			passageSet[passageText] = struct{}{}
		}

		if isSelected == 1 && queryText != "" && answers != "" {
			tempEvalCases[queryID] = EvaluationCase{
				QueryID:           queryID,
				QueryText:         queryText,
				GroundTruthAnswer: answers,
				RelevantPassage:   passageText,
			}
		}
	}

	allPassages := make([]string, 0, len(passageSet))
	for passage := range passageSet {
		allPassages = append(allPassages, passage)
	}
	evalSet := make([]EvaluationCase, 0, len(tempEvalCases))
	for _, ec := range tempEvalCases {
		evalSet = append(evalSet, ec)
	}

	logger.Info("Dataset loaded", "total_unique_passages", len(allPassages), "eval_queries", len(evalSet))
	return allPassages, evalSet, nil
}

func buildValidationPrompt(context, query string) string {
	return fmt.Sprintf(`You are evaluating whether the provided context contains information that helps answer the question.

Context:
---
%s
---

Question: %s

Does the context contain information that helps answer this question? Even if the context only partially addresses the question or provides related information, answer "yes" if it's helpful.

Answer only "yes" or "no":`, context, query)
}

func buildRAGPrompt(context, query string) string {
	return fmt.Sprintf(`Use the following context to answer the question at the end.
If you don't know the answer or the context isn't relevant, just say that you don't know, don't try to make up an answer.

Context:
%s

Question: %s

Helpful Answer:`, context, query)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
