package chains

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/prompts"
	"github.com/sevigo/goframe/schema"
)

// ValidatingRetrievalQA validates the relevance of retrieved context before generation.
// It uses a separate validator LLM to check if the retrieved documents are relevant
// to the query before using them for generation.
type ValidatingRetrievalQA struct {
	// Retriever fetches relevant documents for the query.
	Retriever schema.Retriever
	// GeneratorLLM generates the final answer.
	GeneratorLLM llms.Model
	// ValidatorLLM validates context relevance.
	ValidatorLLM llms.Model
	// logger is used for logging.
	logger *slog.Logger
}

// ValidatingRetrievalQAOption configures a ValidatingRetrievalQA chain.
type ValidatingRetrievalQAOption func(*ValidatingRetrievalQA)

// WithValidator sets the LLM used for context validation.
func WithValidator(llm llms.Model) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.ValidatorLLM = llm
	}
}

// WithLogger sets the logger for the chain.
func WithLogger(logger *slog.Logger) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.logger = logger
	}
}

// NewValidatingRetrievalQA creates a new ValidatingRetrievalQA chain.
// Returns an error if retriever, generator, or validator is nil.
func NewValidatingRetrievalQA(retriever schema.Retriever, generator llms.Model, opts ...ValidatingRetrievalQAOption) (ValidatingRetrievalQA, error) {
	if retriever == nil {
		return ValidatingRetrievalQA{}, errors.New("retriever cannot be nil")
	}
	if generator == nil {
		return ValidatingRetrievalQA{}, errors.New("generator LLM cannot be nil")
	}

	chain := ValidatingRetrievalQA{
		Retriever:    retriever,
		GeneratorLLM: generator,
		logger:       slog.Default(),
	}

	for _, opt := range opts {
		opt(&chain)
	}

	if chain.ValidatorLLM == nil {
		return ValidatingRetrievalQA{}, errors.New("validator LLM is required, use WithValidator() option")
	}

	return chain, nil
}

// Call retrieves documents, validates context relevance, and generates an answer.
// If the retrieved context is not relevant, it generates an answer without context.
func (c *ValidatingRetrievalQA) Call(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", errors.New("query cannot be empty")
	}

	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		c.logger.ErrorContext(ctx, "Document retrieval failed", "error", err)
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	if len(docs) == 0 {
		c.logger.InfoContext(ctx, "No documents retrieved, using direct generation")
		return c.generateDirectAnswer(ctx, query)
	}

	contextStr := c.buildContextString(docs)

	isRelevant, err := c.validateContext(ctx, query, contextStr)
	if err != nil {
		c.logger.ErrorContext(ctx, "Context validation failed", "error", err)
		return "", fmt.Errorf("context validation failed: %w", err)
	}

	if isRelevant {
		c.logger.InfoContext(ctx, "Context validated as relevant, generating RAG answer")
		return c.generateRAGAnswer(ctx, query, contextStr)
	}

	c.logger.InfoContext(ctx, "Context validated as irrelevant, using direct generation")
	return c.generateDirectAnswer(ctx, query)
}

func (c *ValidatingRetrievalQA) buildContextString(docs []schema.Document) string {
	if len(docs) == 0 {
		return ""
	}

	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.PageContent
	}
	return strings.Join(docContents, "\n\n---\n\n")
}

func (c *ValidatingRetrievalQA) validateContext(ctx context.Context, query, context string) (bool, error) {
	validationPrompt := prompts.DefaultValidationPrompt.Format(map[string]string{
		"context": context,
		"query":   query,
	})

	response, err := c.ValidatorLLM.Call(ctx, validationPrompt)
	if err != nil {
		return false, err
	}

	c.logger.DebugContext(ctx, "Validation completed", "response", response)

	// TODO: we should consider more sophisticated validation parsing
	return strings.Contains(strings.ToLower(response), "yes"), nil
}

func (c *ValidatingRetrievalQA) generateRAGAnswer(ctx context.Context, query, context string) (string, error) {
	ragPrompt := prompts.DefaultRAGPrompt.Format(map[string]string{
		"context": context,
		"query":   query,
	})
	return c.GeneratorLLM.Call(ctx, ragPrompt)
}

func (c *ValidatingRetrievalQA) generateDirectAnswer(ctx context.Context, query string) (string, error) {
	return c.GeneratorLLM.Call(ctx, query)
}
