package vectorstores

import (
	"context"
	"fmt"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// MultiQueryRetriever generates multiple query variations and deduplicates results.
type MultiQueryRetriever struct {
	Store        VectorStore
	LLM          llms.Model
	NumDocuments int
	Count        int
	// Max results to return after deduplication across all query variations.
	// Defaults to NumDocuments when zero.
	MaxResults int
	// Hook to generate sparse vectors for the newly generated queries
	SparseGenFunc func(ctx context.Context, queries []string) ([]*schema.SparseVector, error)
}

// GetRelevantDocuments generates query variations and returns deduplicated results.
func (r MultiQueryRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	prompt := `Generate {{.count}} different versions of the following user query to retrieve relevant code snippets from a vector database. 
Original query: {{.query}}
Provide only the queries, one per line, without numbers or bullets.`

	count := "3"
	if r.Count > 0 {
		count = fmt.Sprint(r.Count)
	}
	formattedPrompt := strings.ReplaceAll(prompt, "{{.count}}", count)
	formattedPrompt = strings.ReplaceAll(formattedPrompt, "{{.query}}", query)

	resp, err := r.LLM.Call(ctx, formattedPrompt)

	// On LLM failure, we still search with the original query below
	var queries []string
	if err == nil {
		lines := strings.Split(resp, "\n")
		for _, l := range lines {
			if trimmed := strings.TrimSpace(l); trimmed != "" {
				queries = append(queries, trimmed)
			}
		}
	}
	queries = append(queries, query) // Always include original query

	var opts []Option
	if r.SparseGenFunc != nil {
		sparseVecs, sparseErr := r.SparseGenFunc(ctx, queries)
		if sparseErr == nil && len(sparseVecs) == len(queries) {
			opts = append(opts, WithSparseQueries(sparseVecs))
		}
	}

	batchResults, err := r.Store.SimilaritySearchBatch(ctx, queries, r.NumDocuments, opts...)
	if err != nil {
		return nil, err
	}

	// Deduplicate by source + content
	uniqueDocs := make(map[string]schema.Document)
	for _, docs := range batchResults {
		for _, doc := range docs {
			source, _ := doc.Metadata["source"].(string)
			key := source + doc.PageContent
			uniqueDocs[key] = doc
		}
	}

	finalDocs := make([]schema.Document, 0, len(uniqueDocs))
	for _, doc := range uniqueDocs {
		finalDocs = append(finalDocs, doc)
	}

	// Cap output to prevent overwhelming downstream consumers
	maxResults := r.MaxResults
	if maxResults <= 0 {
		maxResults = r.NumDocuments
	}
	if maxResults > 0 && len(finalDocs) > maxResults {
		finalDocs = finalDocs[:maxResults]
	}

	return finalDocs, nil
}
