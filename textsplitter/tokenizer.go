package textsplitter

import "context"

// Tokenizer is the interface for token counting and text splitting.
// Implementations provide accurate token counts for specific LLM models.
type Tokenizer interface {
	// CountTokens returns the exact number of tokens in the text.
	CountTokens(ctx context.Context, modelName, text string) int
	// EstimateTokens returns an estimated token count (faster but less accurate).
	EstimateTokens(ctx context.Context, modelName, text string) int
	// SplitTextByTokens splits text into chunks that fit within maxTokens.
	SplitTextByTokens(ctx context.Context, modelName, text string, maxTokens int) ([]string, error)
	// GetRecommendedChunkSize returns the recommended chunk size for the model.
	GetRecommendedChunkSize(ctx context.Context, modelName string) int
	// GetOptimalOverlapTokens returns the optimal overlap for chunking.
	GetOptimalOverlapTokens(ctx context.Context, modelName string) int
	// GetMaxContextWindow returns the maximum context window for the model.
	GetMaxContextWindow(ctx context.Context, modelName string) int
}
