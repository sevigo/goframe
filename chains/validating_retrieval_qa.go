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

// ValidatingRetrievalQA is a RAG chain that first validates the relevance
// of retrieved context before passing it to the generator LLM.
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

func WithVLogger(logger *slog.Logger) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.logger = logger
	}
}

func NewValidatingRetrievalQA(retriever schema.Retriever, generator llms.Model, opts ...ValidatingRetrievalQAOption) (*ValidatingRetrievalQA, error) {
	chain := &ValidatingRetrievalQA{
		Retriever:    retriever,
		GeneratorLLM: generator,
		logger:       slog.Default(),
	}
	for _, opt := range opts {
		opt(chain)
	}
	if chain.ValidatorLLM == nil {
		return nil, errors.New("validator LLM is required, use WithValidator() option")
	}
	return chain, nil
}

func (c *ValidatingRetrievalQA) Call(ctx context.Context, query string) (string, error) {
	c.logger.Debug("Retrieving documents for validation...")
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	if len(docs) == 0 {
		c.logger.Info("No documents found. Falling back to direct generation.")
		return c.GeneratorLLM.Call(ctx, query)
	}

	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.PageContent
	}
	contextStr := strings.Join(docContents, "\n\n---\n\n")

	c.logger.Debug("Validating context...")
	validationPrompt := prompts.DefaultValidationPrompt.Format(map[string]string{
		"context": contextStr,
		"query":   query,
	})
	validationResponse, err := c.ValidatorLLM.Call(ctx, validationPrompt)
	if err != nil {
		return "", fmt.Errorf("validation call failed: %w", err)
	}
	c.logger.Info("Validation response", "response", validationResponse)

	if strings.Contains(strings.ToLower(validationResponse), "yes") {
		c.logger.Info("Context is RELEVANT. Generating answer with context.")
		ragPrompt := prompts.DefaultRAGPrompt.Format(map[string]string{
			"context": contextStr,
			"query":   query,
		})
		return c.GeneratorLLM.Call(ctx, ragPrompt)
	}

	c.logger.Info("Context is NOT RELEVANT. Generating answer without context.")
	return c.GeneratorLLM.Call(ctx, query)
}
