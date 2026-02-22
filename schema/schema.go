// Package schema defines core data structures and interfaces used throughout the goframe library.
// It provides types for documents, sparse vectors, retrievers, and rerankers.
package schema

import (
	"context"
	"fmt"
)

// SparseVector represents a sparse vector with indices and values.
// Sparse vectors are used for hybrid search combining dense embeddings
// with exact term matching for improved retrieval accuracy.
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// Document represents a text document with its content and metadata.
// It is the primary data structure for RAG operations, containing
// the text content, associated metadata, and optional sparse vector.
type Document struct {
	// PageContent is the text content of the document.
	PageContent string
	// Metadata contains arbitrary key-value pairs associated with the document.
	Metadata map[string]any
	// Sparse is an optional sparse vector for hybrid search.
	Sparse *SparseVector
}

// ScoredDocument represents a document with an associated relevance score
// and optional reasoning for the score, typically produced by a reranker.
type ScoredDocument struct {
	Document
	// Score is the relevance score, typically between 0 and 10.
	Score float64
	// Reason contains an explanation for the score, if available.
	Reason string
}

// NewDocument creates a new Document with the given content and metadata.
// If metadata is nil, an empty map is created.
func NewDocument(content string, metadata map[string]any) Document {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return Document{
		PageContent: content,
		Metadata:    metadata,
	}
}

// String returns the page content of the document.
func (d Document) String() string {
	return d.PageContent
}

// ModelDetails contains information about an LLM model.
type ModelDetails struct {
	// Family is the model family (e.g., "llama", "gemma").
	Family string
	// ParameterSize is the parameter count (e.g., "7B", "13B").
	ParameterSize string
	// Quantization is the quantization level (e.g., "q4_0", "f16").
	Quantization string
	// Dimension is the embedding dimension of the model.
	Dimension int64
}

// String returns a human-readable representation of the model details.
func (md ModelDetails) String() string {
	return fmt.Sprintf("%s (%s, %s, dim: %d)",
		md.Family, md.ParameterSize, md.Quantization, md.Dimension)
}

// Retriever is the interface for document retrieval.
// Implementations return documents relevant to a query from a corpus.
type Retriever interface {
	// GetRelevantDocuments returns documents relevant to the query.
	GetRelevantDocuments(ctx context.Context, query string) ([]Document, error)
}

// Reranker is the interface for reranking documents.
// Implementations reorder documents by relevance to a query,
// typically using an LLM or cross-encoder model.
type Reranker interface {
	// Rerank reorders documents by relevance to the query,
	// returning scored documents with explanations.
	Rerank(ctx context.Context, query string, docs []Document) ([]ScoredDocument, error)
}

// NoOpReranker is a reranker that returns documents in their original order
// with a constant high score. Use this when reranking is not needed.
type NoOpReranker struct{}

// Rerank implements Reranker by returning documents with a constant score of 10.0.
func (n NoOpReranker) Rerank(_ context.Context, _ string, docs []Document) ([]ScoredDocument, error) {
	results := make([]ScoredDocument, len(docs))
	for i, d := range docs {
		results[i] = ScoredDocument{
			Document: d,
			Score:    10.0,
			Reason:   "reranking disabled",
		}
	}
	return results, nil
}