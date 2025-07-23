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

	"github.com/sevigo/goframe/chains"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/gemini"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

const (
	maxRandomQuestion = 10
	generatorModel    = "gemini-2.5-flash"
	validatorModel    = "gemma3:latest"
	embedderModel     = "nomic-embed-text"
	datasetPath       = "testdata/rag_dataset/msmarco/ms_marco_1000.csv"
	collectionName    = "validated_rag_msmarco_v2"
)

// EvaluationCase represents a single test case from the MS-MARCO dataset.
type EvaluationCase struct {
	QueryID           int
	QueryText         string
	GroundTruthAnswer string
	RelevantPassage   string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if os.Getenv("GEMINI_API_KEY") == "" {
		logger.Error("Please set the GEMINI_API_KEY environment variable.")
		return
	}

	// Create all the components needed for the RAG system.
	vectorStore, generatorLLM, validatorLLM, err := setupRAGComponents(ctx, logger)
	if err != nil {
		logger.Error("Failed to setup RAG components", "error", err)
		return
	}

	// Load the dataset and ingest it into the vector store.
	allPassages, evaluationSet, err := loadMSMARCODataset(datasetPath, logger)
	if err != nil {
		logger.Error("Failed to load MS MARCO dataset", "error", err)
		return
	}
	if errLoad := loadKnowledgeBase(ctx, vectorStore, allPassages, logger); errLoad != nil {
		logger.Error("Failed to load knowledge base", "error", errLoad)
		return
	}

	// Now that the store is populated, create the retriever and the final chain.
	retriever := vectorstores.ToRetriever(vectorStore, 1, vectorstores.WithScoreThreshold(0.5))
	ragChain, err := chains.NewValidatingRetrievalQA(
		retriever,
		generatorLLM,
		chains.WithValidator(validatorLLM),
		chains.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create validating RAG chain", "error", err)
		return
	}

	// Run the interactive Q&A session.
	runQASession(ctx, ragChain, evaluationSet, logger)
}

// setupRAGComponents creates and returns all the building blocks for the chain.
func setupRAGComponents(ctx context.Context, logger *slog.Logger) (vectorstores.VectorStore, *gemini.LLM, *ollama.LLM, error) {
	logger.Info("Setting up RAG components...")

	// Create Embedder
	embedderLLM, err := ollama.New(ollama.WithModel(embedderModel), ollama.WithLogger(logger))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create embedder LLM: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(embedderLLM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// Create Vector Store
	vectorStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	// Create Validator LLM
	validatorLLM, err := ollama.New(ollama.WithModel(validatorModel), ollama.WithLogger(logger))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create validator LLM: %w", err)
	}

	// Create Generator LLM
	generatorLLM, err := gemini.New(ctx, gemini.WithModel(generatorModel))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create generator LLM: %w", err)
	}

	logger.Info("All RAG components initialized successfully.")
	return vectorStore, generatorLLM, validatorLLM, nil
}

func loadKnowledgeBase(ctx context.Context, store vectorstores.VectorStore, passages []string, logger *slog.Logger) error {
	logger.Info("Loading knowledge base documents", "passage_count", len(passages))
	start := time.Now()

	testDocs, _ := store.SimilaritySearch(ctx, "test query to check for existing documents", 1)
	if len(testDocs) > 0 {
		logger.Info("Knowledge base appears to be already loaded. Skipping ingestion.")
		return nil
	}
	logger.Info("No existing documents found. Proceeding with ingestion...")

	documents := make([]schema.Document, len(passages))
	for i, passage := range passages {
		documents[i] = schema.NewDocument(passage, map[string]any{"source": "msmarco"})
	}

	_, err := store.AddDocuments(ctx, documents, nil)
	if err != nil {
		return fmt.Errorf("failed to add documents to vector store: %w", err)
	}

	duration := time.Since(start)
	logger.Info("Knowledge base loaded successfully", "duration", duration)
	return nil
}

func runQASession(ctx context.Context, ragChain chains.ValidatingRetrievalQA, evaluationSet []EvaluationCase, logger *slog.Logger) {
	if len(evaluationSet) == 0 {
		logger.Error("Evaluation set is empty, cannot run Q&A session.")
		return
	}
	logger.Info("Starting Q&A session with random questions from MS-MARCO...", "count", maxRandomQuestion)

	for i := range maxRandomQuestion {
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(evaluationSet))))
		evalCase := evaluationSet[randomIndex.Int64()]

		fmt.Printf("\n%s\n", strings.Repeat("=", 80))
		fmt.Printf("Random Question %d/%d: %s\n", i+1, maxRandomQuestion, evalCase.QueryText)
		fmt.Printf("Ground Truth Answer Hint: %s\n", truncate(evalCase.GroundTruthAnswer, 120))
		fmt.Printf("%s\n", strings.Repeat("-", 80))

		answer, err := ragChain.Call(ctx, evalCase.QueryText)
		if err != nil {
			logger.Error("Failed to get answer", "question", evalCase.QueryText, "error", err)
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("✅ Final Answer: %s\n", answer)
	}
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	logger.Info("Q&A session completed")
}

// loadMSMARCODataset remains unchanged as it's data loading logic.
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

	if _, err := reader.Read(); err != nil {
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
