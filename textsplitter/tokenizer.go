package textsplitter

import "context"

// Tokenizer is an interface for components that can count tokens.
// This allows the CodeAwareTextSplitter to remain decoupled from any specific
// tokenization implementation.
type Tokenizer interface {
	CountTokens(ctx context.Context, modelName, text string) int
	EstimateTokens(ctx context.Context, modelName, text string) int
	SplitTextByTokens(ctx context.Context, modelName, text string, maxTokens int) ([]string, error)
	GetRecommendedChunkSize(ctx context.Context, modelName string) int
	GetOptimalOverlapTokens(ctx context.Context, modelName string) int
	GetMaxContextWindow(ctx context.Context, modelName string) int
}
