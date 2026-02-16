package vectorstores

import (
	"context"
	"strings"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

type MultiQueryRetriever struct {
	Retriever schema.Retriever
	LLM       llms.Model
	Count     int
}

func (r MultiQueryRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	// 1. Ask LLM to generate variations
	prompt := `Generate {{.count}} different versions of the following user query to retrieve relevant code snippets from a vector database. 
Original query: {{.query}}
Provide only the queries, one per line, without numbers or bullets.`

	formattedPrompt := strings.ReplaceAll(prompt, "{{.count}}", "3") // Default to 3
	formattedPrompt = strings.ReplaceAll(formattedPrompt, "{{.query}}", query)

	resp, err := r.LLM.Call(ctx, formattedPrompt)
	if err != nil {
		return r.Retriever.GetRelevantDocuments(ctx, query) // Fallback to original
	}

	queries := append(strings.Split(resp, "\n"), query)

	// 2. We need to check if our store supports Batch Search.
	// Since you implemented SimilaritySearchBatch in Qdrant, we can leverage it.
	// This is where your framework's interface shines.
	uniqueDocs := make(map[string]schema.Document)

	// Implementation Note: In a production version, you'd cast Retriever
	// to an interface that supports Batch search for efficiency.
	for _, q := range queries {
		trimmed := strings.TrimSpace(q)
		if trimmed == "" {
			continue
		}
		docs, _ := r.Retriever.GetRelevantDocuments(ctx, trimmed)
		for _, doc := range docs {
			// Use the source/line as a unique key to deduplicate
			source, _ := doc.Metadata["source"].(string)
			key := source + doc.PageContent
			uniqueDocs[key] = doc
		}
	}

	var finalDocs []schema.Document
	for _, doc := range uniqueDocs {
		finalDocs = append(finalDocs, doc)
	}

	return finalDocs, nil
}
