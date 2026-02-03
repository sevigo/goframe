package vectorstores

import (
	"context"
	"sort"
	"testing"

	"github.com/sevigo/goframe/schema"
)

type fakeReranker struct {
	scores []float64
}

func (f *fakeReranker) Rerank(_ context.Context, _ string, docs []schema.Document) ([]schema.ScoredDocument, error) {
	results := make([]schema.ScoredDocument, len(docs))
	for i, doc := range docs {
		score := 0.0
		if i < len(f.scores) {
			score = f.scores[i]
		}
		results[i] = schema.ScoredDocument{
			Document: doc,
			Score:    score,
			Reason:   "fake",
		}
	}
	// Match production reranker behavior by sorting
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results, nil
}

type fakeRetriever struct {
	docs []schema.Document
}

func (f *fakeRetriever) GetRelevantDocuments(_ context.Context, _ string) ([]schema.Document, error) {
	return f.docs, nil
}

func TestRerankingRetriever(t *testing.T) {
	docs := []schema.Document{
		{PageContent: "doc 1"},
		{PageContent: "doc 2"},
		{PageContent: "doc 3"},
	}

	baseRetriever := &fakeRetriever{docs: docs}
	reranker := &fakeReranker{scores: []float64{0.1, 0.9, 0.5}} // doc 2 has highest score

	rr := RerankingRetriever{
		Retriever: baseRetriever,
		Reranker:  reranker,
		TopK:      1,
	}

	result, err := rr.GetRelevantDocuments(context.Background(), "query")
	if err != nil {
		t.Fatalf("failed to get docs: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(result))
	}

	if result[0].PageContent != "doc 2" {
		t.Errorf("expected doc 2 to be top, got %s", result[0].PageContent)
	}
}
