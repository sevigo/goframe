package vectorstores

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sevigo/goframe/embeddings/sparse"
	"github.com/sevigo/goframe/schema"
)

// DefinitionRetriever looks up symbol definitions using hybrid search.
type DefinitionRetriever struct {
	store VectorStore
}

// NewDefinitionRetriever creates a retriever for looking up symbol definitions.
func NewDefinitionRetriever(store VectorStore) (*DefinitionRetriever, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	return &DefinitionRetriever{store: store}, nil
}

// GetDefinition looks up a symbol definition using hybrid search (dense + sparse).
// Filters by identifier and is_definition metadata for precise results.
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
		slog.Warn("sparse vector generation failed, falling back to dense-only search",
			"error", err, "symbol", symbolName)
	} else {
		searchOpts = append(searchOpts, WithSparseQuery(sparseVec))
	}

	return r.store.SimilaritySearch(ctx, symbolName, 1, searchOpts...)
}
