package fake

import (
	"context"
	"fmt"

	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
)

// Store is an in-memory vector store for testing purposes.
type Store struct {
	docs  map[string]schema.Document
	idSeq int
}

// New a new fake vector store.
func New() *Store {
	return &Store{
		docs: make(map[string]schema.Document),
	}
}

// AddDocuments adds documents to the in-memory store.
func (s *Store) AddDocuments(_ context.Context, docs []schema.Document, _ ...vectorstores.Option) ([]string, error) {
	ids := make([]string, len(docs))
	for i, doc := range docs {
		id := fmt.Sprintf("fake-id-%d", s.idSeq)
		s.docs[id] = doc
		ids[i] = id
		s.idSeq++
	}
	return ids, nil
}

// SimilaritySearch returns the first N documents from the store, simulating a search.
func (s *Store) SimilaritySearch(_ context.Context, _ string, numDocuments int, _ ...vectorstores.Option) ([]schema.Document, error) {
	var results []schema.Document
	count := 0
	for _, doc := range s.docs {
		if count >= numDocuments {
			break
		}
		results = append(results, doc)
		count++
	}
	return results, nil
}

// SimilaritySearchWithScores returns documents with a faked score of 1.0.
func (s *Store) SimilaritySearchWithScores(ctx context.Context, query string, numDocuments int, options ...vectorstores.Option) ([]vectorstores.DocumentWithScore, error) {
	docs, err := s.SimilaritySearch(ctx, query, numDocuments, options...)
	if err != nil {
		return nil, err
	}

	results := make([]vectorstores.DocumentWithScore, len(docs))
	for i, doc := range docs {
		results[i] = vectorstores.DocumentWithScore{
			Document: doc,
			Score:    1.0,
		}
	}
	return results, nil
}

// ListCollections returns a dummy collection name.
func (s *Store) ListCollections(_ context.Context) ([]string, error) {
	return []string{"fake-collection"}, nil
}

// Docs returns all documents currently in the fake store.
func (s *Store) Docs() []schema.Document {
	var docs []schema.Document
	for _, doc := range s.docs {
		docs = append(docs, doc)
	}
	return docs
}
