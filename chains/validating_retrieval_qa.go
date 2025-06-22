// Package chains provides retrieval-augmented generation (RAG) implementations
// with various validation and processing strategies.
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

// ValidatingRetrievalQA implements a RAG chain that validates the relevance
// of retrieved context before generation. It uses a two-step process:
//  1. Retrieve documents based on the query
//  2. Validate if the retrieved context is relevant to answering the query
//  3. Generate answer with or without context based on validation result
//
// This approach helps reduce hallucination by filtering out irrelevant context
// that might confuse the generator LLM.
type ValidatingRetrievalQA struct {
	// Retriever fetches relevant documents based on the input query
	Retriever schema.Retriever

	// GeneratorLLM generates the final answer, either with validated context
	// or directly from the query if context is deemed irrelevant
	GeneratorLLM llms.Model

	// ValidatorLLM determines if retrieved context is relevant to the query.
	// This is required and must be set via WithValidator() option.
	ValidatorLLM llms.Model

	// logger handles structured logging for debugging and monitoring
	logger *slog.Logger
}

// ValidatingRetrievalQAOption defines functional options for configuring
// the ValidatingRetrievalQA chain during construction.
type ValidatingRetrievalQAOption func(*ValidatingRetrievalQA)

// WithValidator sets the LLM model used for context validation.
// This option is required - the constructor will return an error if not provided.
func WithValidator(llm llms.Model) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.ValidatorLLM = llm
	}
}

// WithLogger sets a custom logger for the chain. If not provided,
// slog.Default() will be used.
func WithLogger(logger *slog.Logger) ValidatingRetrievalQAOption {
	return func(c *ValidatingRetrievalQA) {
		c.logger = logger
	}
}

// NewValidatingRetrievalQA creates a new ValidatingRetrievalQA chain.
//
// Parameters:
//   - retriever: Document retriever for fetching relevant context
//   - generator: LLM for generating final answers
//   - opts: Functional options for configuration
//
// Required options:
//   - WithValidator(): Must provide a validator LLM
//
// Optional options:
//   - WithLogger(): Custom logger (defaults to slog.Default())
//
// Returns an error if required parameters are missing or invalid.
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

	// Apply functional options
	for _, opt := range opts {
		opt(&chain)
	}

	// Validate required configuration
	if chain.ValidatorLLM == nil {
		return ValidatingRetrievalQA{}, errors.New("validator LLM is required, use WithValidator() option")
	}

	return chain, nil
}

// Call executes the validating retrieval-augmented generation process.
//
// Process flow:
//  1. Retrieve relevant documents using the configured retriever
//  2. If no documents found, fall back to direct generation
//  3. Validate if retrieved context is relevant to the query
//  4. Generate answer with context (if relevant) or without context (if not)
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - query: User's question or prompt
//
// Returns the generated answer or an error if any step fails.
func (c *ValidatingRetrievalQA) Call(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", errors.New("query cannot be empty")
	}

	// Step 1: Retrieve relevant documents
	c.logger.Debug("Starting document retrieval", "query", query)
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		c.logger.Error("Document retrieval failed", "error", err)
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	// Step 2: Handle case where no documents are found
	if len(docs) == 0 {
		c.logger.Info("No documents retrieved, using direct generation")
		return c.generateDirectAnswer(ctx, query)
	}

	// Step 3: Prepare context from retrieved documents
	contextStr := c.buildContextString(docs)
	c.logger.Debug("Built context string", "doc_count", len(docs), "context_length", len(contextStr))

	// Step 4: Validate context relevance
	isRelevant, err := c.validateContext(ctx, query, contextStr)
	if err != nil {
		c.logger.Error("Context validation failed", "error", err)
		return "", fmt.Errorf("context validation failed: %w", err)
	}

	// Step 5: Generate answer based on validation result
	if isRelevant {
		c.logger.Info("Context validated as relevant, generating RAG answer")
		return c.generateRAGAnswer(ctx, query, contextStr)
	}

	c.logger.Info("Context validated as irrelevant, using direct generation")
	return c.generateDirectAnswer(ctx, query)
}

// buildContextString combines document contents into a single context string
// with clear delimiters between documents.
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

// validateContext determines if the retrieved context is relevant for answering the query.
// Returns true if context is relevant, false otherwise.
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

	// Simple heuristic: look for affirmative responses
	// TODO: we should consider more sophisticated validation parsing
	return strings.Contains(strings.ToLower(response), "yes"), nil
}

// generateRAGAnswer creates an answer using both the query and validated context.
func (c *ValidatingRetrievalQA) generateRAGAnswer(ctx context.Context, query, context string) (string, error) {
	ragPrompt := prompts.DefaultRAGPrompt.Format(map[string]string{
		"context": context,
		"query":   query,
	})
	return c.GeneratorLLM.Call(ctx, ragPrompt)
}

// generateDirectAnswer creates an answer using only the query, without context.
func (c *ValidatingRetrievalQA) generateDirectAnswer(ctx context.Context, query string) (string, error) {
	return c.GeneratorLLM.Call(ctx, query)
}
