// Package llms provides interfaces and utilities for LLM providers.
package llms

import (
	"context"
	"errors"

	"github.com/sevigo/goframe/schema"
)

// Model is the interface for LLM providers.
// Implementations support both single-turn and multi-turn conversations
// with optional streaming support.
type Model interface {
	// GenerateContent generates a response from the LLM given a conversation history.
	// Use this for multi-turn conversations or when you need access to full response metadata.
	GenerateContent(ctx context.Context, messages []schema.MessageContent, options ...CallOption) (*schema.ContentResponse, error)
	// Call is a convenience method for single-turn prompts.
	// It returns the generated text directly.
	Call(ctx context.Context, prompt string, options ...CallOption) (string, error)
}

// Tokenizer is the interface for token counting.
// Implementations provide accurate token counts for a given model.
type Tokenizer interface {
	// CountTokens returns the number of tokens in the text.
	CountTokens(ctx context.Context, text string) (int, error)
}

// GenerateFromSinglePrompt generates a response from a single prompt.
// It wraps the prompt in a human message and returns the generated text.
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

// TextParts creates a MessageContent with multiple text parts.
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