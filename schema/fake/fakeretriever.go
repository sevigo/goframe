package fake

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

// Retriever is a mock retriever for testing purposes.
type Retriever struct {
	DocsToReturn []schema.Document
	ErrToReturn  error
}

// NewRetriever creates a new fake retriever.
func NewRetriever() *Retriever {
	return &Retriever{}
}

// GetRelevantDocuments returns the pre-configured documents and error.
func (r *Retriever) GetRelevantDocuments(_ context.Context, _ string) ([]schema.Document, error) {
	return r.DocsToReturn, r.ErrToReturn
}
