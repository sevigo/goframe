package llms

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"

	"github.com/sevigo/goframe/schema"
)

const RerankPromptDefault = `You are an expert technical lead and code auditor.
Evaluate the relevance of the following code snippet to the user's query.

Query: 
{{.Query}}

Code Snippet (Source: {{.Source}}):
---
{{.Content}}
---

Task: 
Assign a relevance score from 0 to 10. 
- 10: The snippet contains exactly the logic, function, or type mentioned in the query.
- 5: The snippet is in the same package or related logic but doesn't answer the query directly.
- 0: The snippet is completely unrelated.

Respond ONLY with a JSON object in this format: 
{"score": <number>, "reason": "<1-sentence-explanation>"}`

type LLMReranker struct {
	model       Model
	concurrency int
	prompt      string
	template    *template.Template
}

type LLMRerankerOption func(*LLMReranker)

func WithConcurrency(c int) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.concurrency = c
	}
}

func WithPrompt(p string) LLMRerankerOption {
	return func(r *LLMReranker) {
		r.prompt = p
	}
}

func NewLLMReranker(model Model, opts ...LLMRerankerOption) *LLMReranker {
	r := &LLMReranker{
		model:       model,
		concurrency: 5,
		prompt:      RerankPromptDefault,
	}
	for _, opt := range opts {
		opt(r)
	}

	tmpl, err := template.New("rerank").Parse(r.prompt)
	if err != nil {
		// Fallback to simple replacement if template parsing fails (should not happen with default)
		r.template = nil
	} else {
		r.template = tmpl
	}

	return r
}

func (r *LLMReranker) Rerank(ctx context.Context, query string, docs []schema.Document) ([]schema.ScoredDocument, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	resultsChan := make(chan schema.ScoredDocument, len(docs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.concurrency)

	for _, doc := range docs {
		wg.Add(1)
		go func(d schema.Document) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			scoredDoc := r.scoreDocument(ctx, query, d)
			resultsChan <- scoredDoc
		}(doc)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	scoredDocs := make([]schema.ScoredDocument, 0, len(docs))
	for sd := range resultsChan {
		scoredDocs = append(scoredDocs, sd)
	}

	// Sort by score descending
	sort.SliceStable(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].Score > scoredDocs[j].Score
	})

	return scoredDocs, nil
}

func (r *LLMReranker) scoreDocument(ctx context.Context, query string, doc schema.Document) schema.ScoredDocument {
	var prompt string
	if r.template != nil {
		var buf strings.Builder
		data := map[string]any{
			"Query":   query,
			"Content": doc.PageContent,
		}
		for k, v := range doc.Metadata {
			data[k] = v
		}
		if _, ok := data["Source"]; !ok {
			data["Source"], _ = doc.Metadata["source"]
		}

		if err := r.template.Execute(&buf, data); err == nil {
			prompt = buf.String()
		}
	}

	if prompt == "" {
		// Fallback to basic replacement
		source, _ := doc.Metadata["source"].(string)
		prompt = strings.ReplaceAll(r.prompt, "{{.Query}}", query)
		prompt = strings.ReplaceAll(prompt, "{{.Source}}", source)
		prompt = strings.ReplaceAll(prompt, "{{.Content}}", doc.PageContent)
	}

	resp, err := GenerateFromSinglePrompt(ctx, r.model, prompt)
	if err != nil {
		return schema.ScoredDocument{Document: doc, Score: 0, Reason: fmt.Sprintf("error: %v", err)}
	}

	// Extract JSON from response (handling potential markdown fences)
	cleanResp := resp
	if idx := strings.Index(resp, "{"); idx != -1 {
		cleanResp = resp[idx:]
	}
	if idx := strings.LastIndex(cleanResp, "}"); idx != -1 {
		cleanResp = cleanResp[:idx+1]
	}

	var result struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(cleanResp), &result); err != nil {
		return schema.ScoredDocument{Document: doc, Score: 0, Reason: fmt.Sprintf("parse error: %v", err)}
	}

	return schema.ScoredDocument{
		Document: doc,
		Score:    result.Score,
		Reason:   result.Reason,
	}
}
