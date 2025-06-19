package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sevigo/goframe/chains"
	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
	"github.com/sevigo/goframe/vectorstores/qdrant"
)

const (
	genModel      = "gemma3:1b"
	embedderModel = "nomic-embed-text"
)

// EvaluationCase represents a single test case for RAG evaluation
type EvaluationCase struct {
	QueryID           int    `json:"query_id"`
	QueryText         string `json:"query_text"`
	GroundTruthAnswer string `json:"ground_truth_answer"`
	RelevantPassage   string `json:"relevant_passage"`
}

// EvaluationResult holds the results of a single evaluation
type EvaluationResult struct {
	Case             EvaluationCase `json:"case"`
	RetrievedPassage string         `json:"retrieved_passage"`
	GeneratedAnswer  string         `json:"generated_answer"`
	RetrievalHit     bool           `json:"retrieval_hit"`
	GenerationPass   bool           `json:"generation_pass"`
	ResponseTime     time.Duration  `json:"response_time"`
	Error            error          `json:"error,omitempty"`
}

// EvaluationSummary contains overall evaluation statistics
type EvaluationSummary struct {
	TotalCases          int           `json:"total_cases"`
	RetrievalHits       int           `json:"retrieval_hits"`
	GenerationPasses    int           `json:"generation_passes"`
	RetrievalAccuracy   float64       `json:"retrieval_accuracy"`
	GenerationAccuracy  float64       `json:"generation_accuracy"`
	OverallAccuracy     float64       `json:"overall_accuracy"`
	AverageResponseTime time.Duration `json:"average_response_time"`
	TotalEvaluationTime time.Duration `json:"total_evaluation_time"`
	ErrorCount          int           `json:"error_count"`
}

// Configuration constants
const (
	DefaultDatasetPath    = "testdata/rag_dataset/msmarco/ms_marco_1000.csv"
	DefaultTimeoutMinutes = 15
	DefaultBatchSize      = 128
	DefaultRetrievalCount = 1
	TruncateLength        = 80
)

func main() {
	logger := setupLogger()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeoutMinutes*time.Minute)
	defer cancel()

	// Load dataset
	allPassages, evaluationSet, err := loadMSMARCODataset(DefaultDatasetPath, logger)
	if err != nil {
		logger.Error("Failed to load MS MARCO dataset", "error", err)
		return
	}

	// Setup RAG system
	ragChain, vectorStore, cleanup, err := setupRAGSystem(ctx, logger)
	if err != nil {
		logger.Error("Failed to set up RAG system", "error", err)
		return
	}
	defer cleanup()

	// Ingest knowledge base
	if err := ingestKnowledgeBase(ctx, vectorStore, allPassages, logger); err != nil {
		logger.Error("Failed to ingest knowledge base", "error", err)
		return
	}

	// Run evaluation
	summary := runEvaluation(ctx, ragChain, evaluationSet, logger)
	printEvaluationSummary(summary)
}

// setupLogger creates a structured logger
func setupLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false,
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// loadMSMARCODataset loads and processes the MS MARCO CSV dataset
func loadMSMARCODataset(path string, logger *slog.Logger) ([]string, []EvaluationCase, error) {
	logger.Info("Loading MS MARCO dataset", "path", path)
	start := time.Now()

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open dataset file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	// Skip header row
	if _, err := reader.Read(); err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	tempEvalCases := make(map[int]EvaluationCase)
	passageSet := make(map[string]struct{})
	var allPassages []string

	rowCount := 0
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			logger.Warn("Skipping malformed CSV row", "row", rowCount, "error", err)
			continue
		}

		if len(record) < 8 {
			logger.Warn("Skipping incomplete CSV row", "row", rowCount, "fields", len(record))
			continue
		}

		queryID, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil {
			logger.Warn("Invalid query ID", "row", rowCount, "value", record[0])
			continue
		}

		queryText := strings.TrimSpace(record[1])
		answers := strings.TrimSpace(record[3])
		isSelected, _ := strconv.Atoi(strings.TrimSpace(record[6]))
		passageText := strings.TrimSpace(record[7])

		// Add unique passages to knowledge base
		if passageText != "" {
			if _, exists := passageSet[passageText]; !exists {
				allPassages = append(allPassages, passageText)
				passageSet[passageText] = struct{}{}
			}
		}

		// Create evaluation case for selected passages
		if isSelected == 1 && queryText != "" && answers != "" {
			tempEvalCases[queryID] = EvaluationCase{
				QueryID:           queryID,
				QueryText:         queryText,
				GroundTruthAnswer: answers,
				RelevantPassage:   passageText,
			}
		}
		rowCount++
	}

	// Convert to slice for consistent ordering
	evalSet := make([]EvaluationCase, 0, len(tempEvalCases))
	for _, ec := range tempEvalCases {
		evalSet = append(evalSet, ec)
	}

	duration := time.Since(start)
	logger.Info("Dataset loaded successfully",
		"total_passages", len(allPassages),
		"eval_queries", len(evalSet),
		"rows_processed", rowCount,
		"duration", duration)

	return allPassages, evalSet, nil
}

// setupRAGSystem initializes the complete RAG pipeline
func setupRAGSystem(ctx context.Context, logger *slog.Logger) (*chains.RetrievalQA, vectorstores.VectorStore, func(), error) {
	collectionName := fmt.Sprintf("rag-eval-msmarco-%d", time.Now().Unix())
	logger.Info("Setting up RAG system", "collection", collectionName)

	// Setup embedder
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

	// Setup vector store
	vectorStore, err := qdrant.New(
		qdrant.WithEmbedder(embedder),
		qdrant.WithCollectionName(collectionName),
		qdrant.WithBatchSize(DefaultBatchSize),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	// Setup generation LLM
	generationLLM, err := ollama.New(
		ollama.WithModel(genModel),
		ollama.WithLogger(logger),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create generation LLM: %w", err)
	}

	// Create RAG chain
	retriever := vectorstores.ToRetriever(vectorStore, DefaultRetrievalCount)
	ragChain := chains.NewRetrievalQA(retriever, generationLLM)

	// Cleanup function
	cleanup := func() {
		logger.Info("Cleaning up resources", "collection", collectionName)
		// if err := vectorStore.DeleteCollection(ctx, collectionName); err != nil {
		// 	logger.Warn("Failed to cleanup collection", "error", err)
		// }
	}

	logger.Info("RAG system setup complete")
	return &ragChain, vectorStore, cleanup, nil
}

// ingestKnowledgeBase adds all passages to the vector store
func ingestKnowledgeBase(ctx context.Context, store vectorstores.VectorStore, passages []string, logger *slog.Logger) error {
	logger.Info("Starting knowledge base ingestion", "passage_count", len(passages))
	start := time.Now()

	// Convert passages to documents
	docs := make([]schema.Document, len(passages))
	for i, passage := range passages {
		docs[i] = schema.NewDocument(passage, map[string]any{
			"source":     "msmarco_1000.csv",
			"doc_id":     i,
			"created_at": time.Now().Unix(),
		})
	}

	// Add documents to vector store
	ids, err := store.AddDocuments(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to add documents to vector store: %w", err)
	}

	// Brief wait for indexing
	time.Sleep(2 * time.Second)

	duration := time.Since(start)
	logger.Info("Knowledge base ingestion complete",
		"documents_added", len(ids),
		"duration", duration,
		"docs_per_second", float64(len(docs))/duration.Seconds())

	return nil
}

// runEvaluation performs comprehensive RAG evaluation
func runEvaluation(ctx context.Context, ragChain *chains.RetrievalQA, dataset []EvaluationCase, logger *slog.Logger) EvaluationSummary {
	start := time.Now()
	total := len(dataset)

	logger.Info("Starting RAG evaluation", "test_cases", total)
	printEvaluationHeader(total)

	var totalResponseTime time.Duration
	retrievalHits := 0
	generationPasses := 0
	errorCount := 0

	for i, evalCase := range dataset {
		result := evaluateSingleCase(ctx, ragChain, evalCase, i+1, total)

		// Update counters
		if result.Error != nil {
			errorCount++
		} else {
			if result.RetrievalHit {
				retrievalHits++
			}
			if result.GenerationPass {
				generationPasses++
			}
			totalResponseTime += result.ResponseTime
		}

		// Print progress
		if (i+1)%10 == 0 || i == total-1 {
			logger.Info("Evaluation progress",
				"completed", i+1,
				"total", total,
				"success_rate", fmt.Sprintf("%.1f%%", float64(retrievalHits+generationPasses)/float64((i+1)*2)*100))
		}
	}

	// Calculate summary statistics
	summary := EvaluationSummary{
		TotalCases:          total,
		RetrievalHits:       retrievalHits,
		GenerationPasses:    generationPasses,
		RetrievalAccuracy:   float64(retrievalHits) / float64(total) * 100.0,
		GenerationAccuracy:  float64(generationPasses) / float64(total) * 100.0,
		OverallAccuracy:     float64(retrievalHits+generationPasses) / float64(total*2) * 100.0,
		AverageResponseTime: totalResponseTime / time.Duration(total-errorCount),
		TotalEvaluationTime: time.Since(start),
		ErrorCount:          errorCount,
	}

	logger.Info("RAG evaluation completed", "duration", summary.TotalEvaluationTime)
	return summary
}

// evaluateSingleCase processes one evaluation case
func evaluateSingleCase(ctx context.Context, ragChain *chains.RetrievalQA, evalCase EvaluationCase, current, total int) EvaluationResult {
	result := EvaluationResult{Case: evalCase}

	fmt.Printf("\n┌─ Test Case %d/%d (Query ID: %d) ─%s┐\n",
		current, total, evalCase.QueryID, strings.Repeat("─", max(0, 60-len(fmt.Sprintf("Test Case %d/%d (Query ID: %d)", current, total, evalCase.QueryID)))))
	fmt.Printf("│ ❓ Query: %s\n", truncateWithPadding(evalCase.QueryText, 70))

	// Stage 1: Evaluate Retrieval
	fmt.Printf("│ 🔍 Evaluating retrieval...\n")
	retrievedDocs, err := ragChain.Retriever.GetRelevantDocuments(ctx, evalCase.QueryText)
	if err != nil {
		result.Error = err
		fmt.Printf("│ ❌ Retrieval FAILED: %v\n", err)
		fmt.Printf("└%s┘\n", strings.Repeat("─", 78))
		return result
	}

	if len(retrievedDocs) > 0 {
		result.RetrievedPassage = retrievedDocs[0].PageContent
	}

	// Check retrieval accuracy
	result.RetrievalHit = strings.TrimSpace(result.RetrievedPassage) == strings.TrimSpace(evalCase.RelevantPassage)
	if result.RetrievalHit {
		fmt.Printf("│ ✅ Retrieval HIT: Correct passage retrieved\n")
	} else {
		fmt.Printf("│ ❌ Retrieval MISS: Incorrect passage retrieved\n")
		fmt.Printf("│    Expected: %s\n", truncateWithPadding(evalCase.RelevantPassage, 65))
		fmt.Printf("│    Retrieved: %s\n", truncateWithPadding(result.RetrievedPassage, 65))
	}

	// Stage 2: Evaluate Generation
	fmt.Printf("│ 🤖 Evaluating generation...\n")
	start := time.Now()
	answer, err := ragChain.Call(ctx, evalCase.QueryText)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Error = err
		fmt.Printf("│ ❌ Generation FAILED: %v\n", err)
		fmt.Printf("└%s┘\n", strings.Repeat("─", 78))
		return result
	}

	result.GeneratedAnswer = answer
	result.GenerationPass = containsKeywords(answer, evalCase.GroundTruthAnswer)

	fmt.Printf("│ 🎯 Generated: %s\n", truncateWithPadding(answer, 65))
	fmt.Printf("│ 📋 Expected: %s\n", truncateWithPadding(evalCase.GroundTruthAnswer, 65))
	fmt.Printf("│ ⏱️  Response time: %v\n", result.ResponseTime)

	// Overall result
	overallPass := result.RetrievalHit && result.GenerationPass
	if overallPass {
		fmt.Printf("│ 🎉 Overall: ✅ PASS\n")
	} else {
		fmt.Printf("│ 💥 Overall: ❌ FAIL\n")
	}

	fmt.Printf("└%s┘\n", strings.Repeat("─", 78))
	return result
}

// printEvaluationHeader displays the evaluation start banner
func printEvaluationHeader(totalCases int) {
	fmt.Printf("\n╔%s╗\n", strings.Repeat("═", 78))
	fmt.Printf("║%s║\n", centerText("🚀 RAG EVALUATION - MS MARCO DATASET 🚀", 78))
	fmt.Printf("║%s║\n", centerText(fmt.Sprintf("Running %d test cases", totalCases), 78))
	fmt.Printf("╚%s╝\n", strings.Repeat("═", 78))
}

// printEvaluationSummary displays comprehensive evaluation results
func printEvaluationSummary(summary EvaluationSummary) {
	fmt.Printf("\n╔%s╗\n", strings.Repeat("═", 78))
	fmt.Printf("║%s║\n", centerText("📊 RAG EVALUATION SUMMARY 📊", 78))
	fmt.Printf("╠%s╣\n", strings.Repeat("═", 78))
	fmt.Printf("║                                                                              ║\n")
	fmt.Printf("║  📈 Retrieval Accuracy:     %6.2f%% (%3d/%3d) %s                     ║\n",
		summary.RetrievalAccuracy, summary.RetrievalHits, summary.TotalCases,
		getStatusEmoji(summary.RetrievalAccuracy))
	fmt.Printf("║  🎯 Generation Accuracy:    %6.2f%% (%3d/%3d) %s                     ║\n",
		summary.GenerationAccuracy, summary.GenerationPasses, summary.TotalCases,
		getStatusEmoji(summary.GenerationAccuracy))
	fmt.Printf("║  🏆 Overall Accuracy:       %6.2f%% %s                                  ║\n",
		summary.OverallAccuracy, getStatusEmoji(summary.OverallAccuracy))
	fmt.Printf("║                                                                              ║\n")
	fmt.Printf("║  ⏱️  Average Response Time:  %8s                                       ║\n", summary.AverageResponseTime.Truncate(time.Millisecond))
	fmt.Printf("║  🕐 Total Evaluation Time:  %8s                                       ║\n", summary.TotalEvaluationTime.Truncate(time.Second))
	fmt.Printf("║  ❌ Error Count:            %8d                                       ║\n", summary.ErrorCount)
	fmt.Printf("║                                                                              ║\n")
	fmt.Printf("║  Status: %-60s   ║\n", getOverallStatus(summary.OverallAccuracy))
	fmt.Printf("╚%s╝\n", strings.Repeat("═", 78))
	fmt.Printf("\n")
}

func truncateWithPadding(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	leftPad := strings.Repeat(" ", padding)
	rightPad := strings.Repeat(" ", width-len(text)-padding)
	return leftPad + text + rightPad
}

func getStatusEmoji(accuracy float64) string {
	switch {
	case accuracy >= 90:
		return "🟢"
	case accuracy >= 75:
		return "🟡"
	case accuracy >= 60:
		return "🟠"
	default:
		return "🔴"
	}
}

func getOverallStatus(accuracy float64) string {
	switch {
	case accuracy >= 90:
		return "🌟 Excellent - System performing optimally"
	case accuracy >= 75:
		return "✅ Good - Minor optimizations possible"
	case accuracy >= 60:
		return "⚠️  Fair - Consider tuning parameters"
	default:
		return "🚨 Poor - Significant improvements needed"
	}
}

// containsKeywords checks if generated answer aligns with ground truth
func containsKeywords(generated, groundTruth string) bool {
	if generated == "" || groundTruth == "" {
		return false
	}

	genLower := strings.ToLower(strings.TrimSpace(generated))
	gtLower := strings.ToLower(strings.TrimSpace(groundTruth))

	// Check for exact match or significant overlap
	return strings.Contains(genLower, gtLower) ||
		strings.Contains(gtLower, genLower) ||
		calculateSimilarity(genLower, gtLower) > 0.5
}

// calculateSimilarity provides a simple similarity score based on common words
func calculateSimilarity(s1, s2 string) float64 {
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	wordSet := make(map[string]bool)
	for _, word := range words1 {
		if len(word) > 2 { // Ignore very short words
			wordSet[word] = true
		}
	}

	commonCount := 0
	for _, word := range words2 {
		if len(word) > 2 && wordSet[word] {
			commonCount++
		}
	}

	return float64(commonCount) / float64(max(len(words1), len(words2)))
}
