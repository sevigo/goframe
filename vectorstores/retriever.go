package vectorstores

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

// Retriever is the interface for fetching relevant documents for a query.
type Retriever interface {
	GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error)
}

// retrieverImpl implements the schema.Retriever interface.
type retrieverImpl struct {
	vectorStore VectorStore
	numDocs     int
}

// GetRelevantDocuments retrieves documents from the vector store.
func (r retrieverImpl) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	return r.vectorStore.SimilaritySearch(ctx, query, r.numDocs)
}

// ToRetriever creates a retriever from a vector store.
func ToRetriever(vectorStore VectorStore, numDocs int) schema.Retriever {
	return retrieverImpl{
		vectorStore: vectorStore,
		numDocs:     numDocs,
	}
}
