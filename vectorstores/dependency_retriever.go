package vectorstores

import (
	"context"
	"fmt"
	"strings"

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

// NewDependencyRetriever creates a new graph-based retriever.
// It returns an error if store is nil.
func NewDependencyRetriever(store VectorStore) (*DependencyRetriever, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	return &DependencyRetriever{
		store: store,
	}, nil
}

// GetContextNetwork retrieves both upstream dependencies and downstream impact.
//
// Imports are expected to be full module paths (e.g. "github.com/foo/bar/baz").
// The store indexes the short package name under "package_name" (e.g. "baz") and
// the list of short import names under "import_names". Both filters therefore use
// the last path segment of each import so they match correctly.
func (r *DependencyRetriever) GetContextNetwork(ctx context.Context, packageName string, imports []string) (*ContextNetwork, error) {
	network := &ContextNetwork{
		Dependencies: []schema.Document{},
		Dependents:   []schema.Document{},
	}

	// 1. Fetch Dependencies (Upstream)
	// Find documents whose package_name matches the short name of any package we import.
	// import paths like "github.com/foo/bar/schema" → short name "schema".
	if len(imports) > 0 {
		shortNames := importShortNames(imports)
		filterDeps := map[string]any{
			"package_name": shortNames,
		}
		deps, err := r.store.SimilaritySearch(ctx, "*", 10, WithFilters(filterDeps))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch dependencies: %w", err)
		}
		network.Dependencies = deps
	}

	// 2. Fetch Dependents (Downstream / Impact)
	// Find documents that import our package.  We match against the "import_names"
	// metadata field which stores the short names of every import in each file.
	// This lets us find callers without knowing the full module path of our package.
	if packageName != "" {
		filterImpact := map[string]any{
			"import_names": packageName,
		}
		impacts, err := r.store.SimilaritySearch(ctx, "*", 10, WithFilters(filterImpact))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch impact: %w", err)
		}
		network.Dependents = impacts
	}

	return network, nil
}

// importShortNames extracts the last path segment from each import path.
// e.g. "github.com/foo/bar/schema" → "schema".
func importShortNames(imports []string) []string {
	names := make([]string, 0, len(imports))
	seen := make(map[string]struct{}, len(imports))
	for _, imp := range imports {
		parts := strings.Split(imp, "/")
		name := parts[len(parts)-1]
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}
