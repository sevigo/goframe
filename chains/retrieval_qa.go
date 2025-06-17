package chains

import (
	"context"
	"fmt"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// RetrievalQA implements a Retrieval-Augmented Generation (RAG) chain that
// combines document retrieval with LLM generation for context-aware responses.
type RetrievalQA struct {
	Retriever schema.Retriever
	LLM       llms.Model
}

// NewRetrievalQA creates a new RetrievalQA chain with the specified retriever and LLM.
func NewRetrievalQA(retriever schema.Retriever, llm llms.Model) RetrievalQA {
	return RetrievalQA{
		Retriever: retriever,
		LLM:       llm,
	}
}

// Call executes the RAG pipeline by retrieving relevant documents and generating
// a context-aware response. Falls back to direct LLM query if no documents found.
func (c RetrievalQA) Call(ctx context.Context, query string) (string, error) {
	// Retrieve relevant documents for the query
	docs, err := c.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return "", fmt.Errorf("document retrieval failed: %w", err)
	}

	// Fall back to direct LLM query if no relevant documents found
	if len(docs) == 0 {
		return c.LLM.Call(ctx, query)
	}

	// Combine document contents into context
	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.PageContent
	}
	context := strings.Join(docContents, "\n\n---\n\n")

	// Create RAG prompt with retrieved context
	prompt := buildRAGPrompt(context, query)

	return c.LLM.Call(ctx, prompt)
}

// buildRAGPrompt constructs a prompt that instructs the LLM to answer based
// on the provided context and admit when it doesn't know the answer.
func buildRAGPrompt(context, query string) string {
	return fmt.Sprintf(`Use the following context to answer the question at the end.
If you don't know the answer, just say that you don't know, don't try to make up an answer.

Context:
%s

Question: %s

Helpful Answer:`, context, query)
}
