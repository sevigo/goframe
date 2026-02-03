package llms_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/llms/fake"
	"github.com/sevigo/goframe/schema"
)

type scorerLLM struct {
	fake.LLM
}

func (s *scorerLLM) GenerateContent(ctx context.Context, messages []schema.MessageContent, options ...llms.CallOption) (*schema.ContentResponse, error) {
	prompt := messages[0].GetTextContent()
	score := 0
	if strings.Contains(prompt, "doc 1") {
		score = 8
	} else if strings.Contains(prompt, "doc 2") {
		score = 2
	} else if strings.Contains(prompt, "doc 3") {
		score = 9
	}

	resp := fmt.Sprintf(`{"score": %d, "reason": "logic"}`, score)
	return &schema.ContentResponse{
		Choices: []*schema.ContentChoice{{Content: resp}},
	}, nil
}

func (s *scorerLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, _ := s.GenerateContent(ctx, []schema.MessageContent{
		{
			Role:  schema.ChatMessageTypeHuman,
			Parts: []schema.ContentPart{schema.TextContent{Text: prompt}},
		},
	}, options...)
	return resp.Choices[0].Content, nil
}

func TestLLMReranker(t *testing.T) {
	fakeLLM := &scorerLLM{}

	reranker := llms.NewLLMReranker(fakeLLM, llms.WithConcurrency(5))
	query := "how to parse JSON"
	docs := []schema.Document{
		{PageContent: "doc 1", Metadata: map[string]any{"source": "file1.go"}},
		{PageContent: "doc 2", Metadata: map[string]any{"source": "file2.go"}},
		{PageContent: "doc 3", Metadata: map[string]any{"source": "file3.go"}},
	}

	scoredDocs, err := reranker.Rerank(context.Background(), query, docs)
	if err != nil {
		t.Fatalf("Rerank failed: %v", err)
	}

	if len(scoredDocs) != 3 {
		t.Fatalf("expected 3 scored docs, got %d", len(scoredDocs))
	}

	if scoredDocs[0].Score != 9 || scoredDocs[0].Document.PageContent != "doc 3" {
		t.Errorf("expected top doc to be doc 3 with score 9, got %s with score %v", scoredDocs[0].Document.PageContent, scoredDocs[0].Score)
	}
	if scoredDocs[1].Score != 8 || scoredDocs[1].Document.PageContent != "doc 1" {
		t.Errorf("expected second doc to be doc 1 with score 8, got %s with score %v", scoredDocs[1].Document.PageContent, scoredDocs[1].Score)
	}
	if scoredDocs[2].Score != 2 || scoredDocs[2].Document.PageContent != "doc 2" {
		t.Errorf("expected bottom doc to be doc 2 with score 2, got %s with score %v", scoredDocs[2].Document.PageContent, scoredDocs[2].Score)
	}
}

func TestLLMReranker_Empty(t *testing.T) {
	reranker := llms.NewLLMReranker(nil, llms.WithConcurrency(1))
	docs, err := reranker.Rerank(context.Background(), "query", nil)
	if err != nil {
		t.Errorf("expected no error for nil docs, got %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}
