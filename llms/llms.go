package llms

import (
	"context"
	"errors"

	"github.com/sevigo/goframe/schema"
)

type Model interface {
	GenerateContent(ctx context.Context, messages []schema.MessageContent, options ...CallOption) (*ContentResponse, error)
	Call(ctx context.Context, prompt string, options ...CallOption) (string, error)
}

type Tokenizer interface {
	CountTokens(ctx context.Context, text string) (int, error)
}

func GenerateFromSinglePrompt(ctx context.Context, llm Model, prompt string, options ...CallOption) (string, error) {
	msg := schema.MessageContent{
		Role:  schema.ChatMessageTypeHuman,
		Parts: []schema.ContentPart{schema.TextContent{Text: prompt}},
	}

	resp, err := llm.GenerateContent(ctx, []schema.MessageContent{msg}, options...)
	if err != nil {
		return "", err
	}

	choices := resp.Choices
	if len(choices) < 1 {
		return "", errors.New("empty response from model")
	}
	c1 := choices[0]
	return c1.Content, nil
}

func TextParts(role schema.ChatMessageType, parts ...string) schema.MessageContent {
	result := schema.MessageContent{
		Role:  role,
		Parts: make([]schema.ContentPart, 0, len(parts)),
	}
	for _, part := range parts {
		result.Parts = append(result.Parts, schema.TextContent{Text: part})
	}
	return result
}
