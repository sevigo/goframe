package chains

import (
	"context"
	"fmt"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/prompts"
	"github.com/sevigo/goframe/schema"
)

// RetrievalQAOption configures a RetrievalQA chain.
type RetrievalQAOption func(*RetrievalQA)

// RetrievalQA is a simple RAG chain that retrieves documents and generates an answer.
// It retrieves relevant documents using the Retriever, builds a prompt with the
// documents as context, and uses the LLM to generate an answer.
type RetrievalQA struct {
	// Retriever fetches relevant documents for the query.
	Retriever schema.Retriever
	// LLM generates the final answer.
	LLM llms.Model
	// PromptBuilder formats the query and documents into a prompt.
	// If nil, uses DefaultRAGPrompt.
	PromptBuilder func(query string, docs []schema.Document) (string, error)
}

// WithPromptBuilder sets a custom prompt builder function.
// The builder receives the query and retrieved documents and returns
// the formatted prompt string.
func WithPromptBuilder(pb func(query string, docs []schema.Document) (string, error)) RetrievalQAOption {
	return func(c *RetrievalQA) {
		c.PromptBuilder = pb
	}
}

// NewRetrievalQA creates a new RetrievalQA chain.
// Returns an error if retriever or llm is nil.
func NewRetrievalQA(retriever schema.Retriever, llm llms.Model, opts ...RetrievalQAOption) (RetrievalQA, error) {
	if retriever == nil {
		return RetrievalQA{}, fmt.Errorf("retriever cannot be nil")
	}
	if llm == nil {
		return RetrievalQA{}, fmt.Errorf("llm cannot be nil")
	}

	chain := RetrievalQA{
		Retriever: retriever,
		LLM:       llm,
	}
	for _, opt := range opts {
		opt(&chain)
	}

	// Fall back to the standard RAG prompt if no custom builder was provided
	if chain.PromptBuilder == nil {
		chain.PromptBuilder = func(query string, docs []schema.Document) (string, error) {
			docContents := make([]string, len(docs))
			for i, doc := range docs {
				docContents[i] = doc.PageContent
			}
			contextStr := strings.Join(docContents, "\n\n---\n\n")

			return prompts.DefaultRAGPrompt.Format(map[string]string{
				"context": contextStr,
				"query":   query,
			}), nil
		}
	}

	return chain, nil
}

// Call retrieves relevant documents and generates an answer for the query.
// If no documents are retrieved, it calls the LLM directly with the query.
func (c RetrievalQA) Call(ctx context.Context, query string) (string, error) {
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	if len(docs) == 0 {
		return c.LLM.Call(ctx, query)
	}

	prompt, err := c.PromptBuilder(query, docs)
	if err != nil {
		return "", fmt.Errorf("prompt building failed: %w", err)
	}

	return c.LLM.Call(ctx, prompt)
}
