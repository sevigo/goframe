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

// ValidatingRetrievalQA implements a RAG chain that validates the relevance of retrieved
// context before generation to reduce hallucination.
type ValidatingRetrievalQA struct {
	Retriever    schema.Retriever
	GeneratorLLM llms.Model
	ValidatorLLM llms.Model
	logger       *slog.Logger
}

type ValidatingRetrievalQAOption func(*ValidatingRetrievalQA)

func WithValidator(llm llms.Model) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.ValidatorLLM = llm
	}
}

func WithLogger(logger *slog.Logger) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.logger = logger
	}
}

// NewValidatingRetrievalQA creates a new ValidatingRetrievalQA chain. It requires a retriever,
// a generator LLM, and the WithValidator() option.
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

func (c *ValidatingRetrievalQA) Call(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", errors.New("query cannot be empty")
	}

	c.logger.Debug("Starting document retrieval", "query", query)
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		c.logger.Error("Document retrieval failed", "error", err)
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	if len(docs) == 0 {
		c.logger.Info("No documents retrieved, using direct generation")
		return c.generateDirectAnswer(ctx, query)
	}

	contextStr := c.buildContextString(docs)
	c.logger.Debug("Built context string", "doc_count", len(docs), "context_length", len(contextStr))

	isRelevant, err := c.validateContext(ctx, query, contextStr)
	if err != nil {
		c.logger.Error("Context validation failed", "error", err)
		return "", fmt.Errorf("context validation failed: %w", err)
	}

	if isRelevant {
		c.logger.Info("Context validated as relevant, generating RAG answer")
		return c.generateRAGAnswer(ctx, query, contextStr)
	}

	c.logger.Info("Context validated as irrelevant, using direct generation")
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

	c.logger.Debug("Validation completed", "response", response)

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
