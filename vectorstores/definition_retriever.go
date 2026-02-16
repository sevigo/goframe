package vectorstores

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

type DefinitionRetriever struct {
	store VectorStore
}

func NewDefinitionRetriever(store VectorStore) *DefinitionRetriever {
	return &DefinitionRetriever{store: store}
}

// GetDefinition performs an exact match lookup for a symbol name
func (r *DefinitionRetriever) GetDefinition(ctx context.Context, symbolName string) ([]schema.Document, error) {
	return r.store.SimilaritySearch(ctx, symbolName, 1,
		WithFilters(map[string]any{
			"identifier":    symbolName,
			"is_definition": true,
		}),
	)
}
