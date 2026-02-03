package schema

import (
	"context"
	"fmt"
)

type Document struct {
	PageContent string
	Metadata    map[string]any
}

type ScoredDocument struct {
	Document
	Score  float64
	Reason string
}

func NewDocument(content string, metadata map[string]any) Document {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return Document{
		PageContent: content,
		Metadata:    metadata,
	}
}

func (d Document) String() string {
	return d.PageContent
}

type ModelDetails struct {
	Family        string
	ParameterSize string
	Quantization  string
	Dimension     int64
}

func (md ModelDetails) String() string {
	return fmt.Sprintf("%s (%s, %s, dim: %d)",
		md.Family, md.ParameterSize, md.Quantization, md.Dimension)
}

type Retriever interface {
	GetRelevantDocuments(ctx context.Context, query string) ([]Document, error)
}

type Reranker interface {
	Rerank(ctx context.Context, query string, docs []Document) ([]ScoredDocument, error)
}

// NoOpReranker returns documents in their original order with a constant high score.
type NoOpReranker struct{}

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
