package chains

import (
	"context"
	"fmt"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/prompts"
	"github.com/sevigo/goframe/schema"
)

type RetrievalQA struct {
	Retriever schema.Retriever
	LLM       llms.Model
}

func NewRetrievalQA(retriever schema.Retriever, llm llms.Model) RetrievalQA {
	return RetrievalQA{
		Retriever: retriever,
		LLM:       llm,
	}
}

func (c RetrievalQA) Call(ctx context.Context, query string) (string, error) {
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	if len(docs) == 0 {
		return c.LLM.Call(ctx, query)
	}

	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.PageContent
	}
	context := strings.Join(docContents, "\n\n---\n\n")

	prompt := prompts.DefaultRAGPrompt.Format(map[string]string{
		"context": context,
		"query":   query,
	})

	return c.LLM.Call(ctx, prompt)
}
