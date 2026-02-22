package vectorstores

import (
	"context"

	"github.com/sevigo/goframe/embeddings/sparse"
	"github.com/sevigo/goframe/schema"
)

type DefinitionRetriever struct {
	store VectorStore
}

func NewDefinitionRetriever(store VectorStore) *DefinitionRetriever {
	return &DefinitionRetriever{store: store}
}

// GetDefinition performs an exact match lookup for a symbol name.
// Uses hybrid search (dense + sparse) for better symbol resolution.
func (r *DefinitionRetriever) GetDefinition(ctx context.Context, symbolName string) ([]schema.Document, error) {
	searchOpts := []Option{
		WithFilters(map[string]any{
			"identifier":    symbolName,
			"is_definition": true,
		}),
	}

	// Add sparse vector for better exact symbol matching
	sparseVec, err := sparse.GenerateSparseVector(ctx, symbolName)
	if err != nil {
		// Sparse generation failed, fall back to dense-only search
		// This is acceptable as the filter on "identifier" should still work
	} else {
		searchOpts = append(searchOpts, WithSparseQuery(sparseVec))
	}

	return r.store.SimilaritySearch(ctx, symbolName, 1, searchOpts...)
}
