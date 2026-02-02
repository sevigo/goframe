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
		// Filter: package_name IN [imports]
		// We pass []string directly. The store implementation should handle []string or convert []any containing strings.
		filterDeps := map[string]any{
			"package_name": imports,
		}

		// We use "*" as a dummy query to ensure the vector store processes the request.
		// Pure metadata filtering often requires a non-empty query in some implementations.
		deps, err := r.store.SimilaritySearch(ctx, "*", 10, WithFilters(filterDeps))
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

		impacts, err := r.store.SimilaritySearch(ctx, "*", 10, WithFilters(filterImpact))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch impact: %w", err)
		}
		network.Dependents = impacts
	}

	return network, nil
}
