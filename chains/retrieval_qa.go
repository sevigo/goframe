package chains

import (
	"context"
	"fmt"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/prompts"
	"github.com/sevigo/goframe/schema"
)

type RetrievalQAOption func(*RetrievalQA)

type RetrievalQA struct {
	Retriever     schema.Retriever
	LLM           llms.Model
	PromptBuilder func(query string, docs []schema.Document) (string, error)
}

// WithPromptBuilder allows passing a custom function to format the
// retrieved documents and query into a final string prompt.
func WithPromptBuilder(pb func(query string, docs []schema.Document) (string, error)) RetrievalQAOption {
	return func(c *RetrievalQA) {
		c.PromptBuilder = pb
	}
}

func NewRetrievalQA(retriever schema.Retriever, llm llms.Model, opts ...RetrievalQAOption) RetrievalQA {
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

	return chain
}

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
