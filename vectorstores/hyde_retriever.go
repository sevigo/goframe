package vectorstores

import (
	"context"
	"sync"

	"github.com/sevigo/goframe/schema"
)

// HyDERetriever implements the Hypothetical Document Embedding pattern.
// It asks an LLM to generate a hypothetical answer to the query, then
// uses that hypothetical answer as the search query against the base retriever.
// This often finds better matches because the hypothetical answer is closer
// in embedding space to the actual stored documents.
type HyDERetriever struct {
	// BaseRetriever performs the actual similarity search
	BaseRetriever schema.Retriever
	// Generator produces a hypothetical document from a query
	Generator func(ctx context.Context, query string) (string, error)
	// NumGenerations controls how many hypothetical docs to generate (default 1).
	// When > 1, results from all generated docs are deduplicated.
	NumGenerations int
}

// HyDEOption configures a HyDERetriever.
type HyDEOption func(*HyDERetriever)

// WithNumGenerations sets how many hypothetical documents to generate.
func WithNumGenerations(n int) HyDEOption {
	return func(r *HyDERetriever) {
		r.NumGenerations = n
	}
}

// NewHyDERetriever creates a HyDE retriever using the given base retriever and generator.
func NewHyDERetriever(baseRetriever schema.Retriever, generator func(ctx context.Context, query string) (string, error), opts ...HyDEOption) *HyDERetriever {
	r := &HyDERetriever{
		BaseRetriever:  baseRetriever,
		Generator:      generator,
		NumGenerations: 1,
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.NumGenerations < 1 {
		r.NumGenerations = 1
	}
	return r
}

// GetRelevantDocuments retrieves documents using hypothetical document embeddings.
func (r *HyDERetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	if r.NumGenerations == 1 {
		return r.singleGeneration(ctx, query)
	}
	return r.multiGeneration(ctx, query)
}

func (r *HyDERetriever) singleGeneration(ctx context.Context, query string) ([]schema.Document, error) {
	hypothetical, err := r.Generator(ctx, query)
	if err != nil {
		// Fall back to searching with the original query
		return r.BaseRetriever.GetRelevantDocuments(ctx, query)
	}
	return r.BaseRetriever.GetRelevantDocuments(ctx, hypothetical)
}

func (r *HyDERetriever) multiGeneration(ctx context.Context, query string) ([]schema.Document, error) {
	type result struct {
		docs []schema.Document
		err  error
	}

	results := make(chan result, r.NumGenerations)
	var wg sync.WaitGroup

	for range r.NumGenerations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hypothetical, err := r.Generator(ctx, query)
			if err != nil {
				results <- result{err: err}
				return
			}
			docs, err := r.BaseRetriever.GetRelevantDocuments(ctx, hypothetical)
			results <- result{docs: docs, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Deduplicate by source + content (same approach as MultiQueryRetriever)
	uniqueDocs := make(map[string]schema.Document)
	var lastErr error
	successCount := 0

	for res := range results {
		if res.err != nil {
			lastErr = res.err
			continue
		}
		successCount++
		for _, doc := range res.docs {
			source, _ := doc.Metadata["source"].(string)
			key := source + doc.PageContent
			uniqueDocs[key] = doc
		}
	}

	// If all generations failed, fall back to original query
	if successCount == 0 {
		docs, err := r.BaseRetriever.GetRelevantDocuments(ctx, query)
		if err != nil && lastErr != nil {
			return nil, lastErr
		}
		return docs, err
	}

	finalDocs := make([]schema.Document, 0, len(uniqueDocs))
	for _, doc := range uniqueDocs {
		finalDocs = append(finalDocs, doc)
	}
	return finalDocs, nil
}
