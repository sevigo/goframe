package vectorstores

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

// Retriever is the interface for fetching relevant documents for a query.
type Retriever interface {
	GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error)
}

// ScoredRetriever is a retriever that returns documents with relevance scores.
type ScoredRetriever interface {
	GetRelevantScoredDocuments(ctx context.Context, query string) ([]schema.ScoredDocument, error)
}

// retrieverImpl implements the schema.Retriever interface.
type retrieverImpl struct {
	vectorStore VectorStore
	numDocs     int
	options     []Option
}

// GetRelevantDocuments retrieves documents from the vector store.
func (r retrieverImpl) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	return r.vectorStore.SimilaritySearch(ctx, query, r.numDocs, r.options...)
}

// ToRetriever creates a retriever from a vector store.
func ToRetriever(vectorStore VectorStore, numDocs int, options ...Option) schema.Retriever {
	return retrieverImpl{
		vectorStore: vectorStore,
		numDocs:     numDocs,
		options:     options,
	}
}

// RerankingRetriever wraps a standard retriever and uses a reranker to refine results.
type RerankingRetriever struct {
	Retriever schema.Retriever
	Reranker  schema.Reranker
	TopK      int // Final number of documents to return after reranking
}

func (r RerankingRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	scored, err := r.GetRelevantScoredDocuments(ctx, query)
	if err != nil {
		return nil, err
	}
	docs := make([]schema.Document, len(scored))
	for i, sd := range scored {
		docs[i] = sd.Document
	}
	return docs, nil
}

func (r RerankingRetriever) GetRelevantScoredDocuments(ctx context.Context, query string) ([]schema.ScoredDocument, error) {
	// 1. Fetch wide net of documents
	// IMPORTANT: The provided r.Retriever should be configured to return a broad set of results
	// (e.g., if you want top 5 reranked, the base retriever should probably return 20).
	docs, err := r.Retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, nil
	}

	// 2. Rerank them
	scored, err := r.Reranker.Rerank(ctx, query, docs)
	if err != nil {
		return nil, err
	}

	// 3. Keep topK
	topK := r.TopK
	if topK <= 0 {
		topK = len(scored)
	}
	if topK > len(scored) {
		topK = len(scored)
	}

	return scored[:topK], nil
}
