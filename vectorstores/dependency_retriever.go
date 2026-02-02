package vectorstores

import (
	"context"
	"fmt"

	"github.com/sevigo/goframe/schema"
)

// ContextNetwork represents the graph neighborhood of a piece of code
type ContextNetwork struct {
	// Dependencies are documents that the current code imports/depends on
	Dependencies []schema.Document
	// Dependents are documents that import/depend on the current code (impact analysis)
	Dependents []schema.Document
}

// DependencyRetriever optimizes RAG by traversing the dependency graph
type DependencyRetriever struct {
	store VectorStore
}

// NewDependencyRetriever creates a new graph-based retriever
func NewDependencyRetriever(store VectorStore) *DependencyRetriever {
	return &DependencyRetriever{
		store: store,
	}
}

// GetContextNetwork retrieves both upstream dependencies and downstream impact
func (r *DependencyRetriever) GetContextNetwork(ctx context.Context, packageName string, imports []string) (*ContextNetwork, error) {
	network := &ContextNetwork{
		Dependencies: []schema.Document{},
		Dependents:   []schema.Document{},
	}

	// 1. Fetch Dependencies (Upstream)
	// We want to find documents where 'package_name' is in our 'imports' list.
	// Filter: package_name IN [imports]
	if len(imports) > 0 {
		// Convert imports to interface slice for the filter
		importsAny := make([]any, len(imports))
		for i, v := range imports {
			importsAny[i] = v
		}

		filterDeps := map[string]any{
			"package_name": importsAny, // Qdrant/Store should interpret slice as "IN" or "Match_Any"
		}

		// We use a generic query because we are filtering by exact metadata match,
		// but VectorStore interface usually requires a query string for SimilaritySearch.
		//Ideally, we would perform a pure filter search, but SimilaritySearch with a dummy query works if the store supports it.
		// Alternatively, if the Store supports a "Search" method without query vector, better.
		// For now, using SimilaritySearch with generic query text.
		deps, err := r.store.SimilaritySearch(ctx, "", 10, WithFilters(filterDeps))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch dependencies: %w", err)
		}
		network.Dependencies = deps
	}

	// 2. Fetch Dependents (Downstream / Impact)
	// We want to find documents where their 'imports' list contains our 'package_name'.
	// Filter: imports CONTAINS package_name
	if packageName != "" {
		filterImpact := map[string]any{
			"imports": packageName, // Qdrant/Store should interpret string val against array field as "CONTAINS"
		}

		impacts, err := r.store.SimilaritySearch(ctx, "", 10, WithFilters(filterImpact))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch impact: %w", err)
		}
		network.Dependents = impacts
	}

	return network, nil
}
