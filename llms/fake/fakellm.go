package fake

import (
	"context"
	"errors"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// LLM represents a fake LLM implementation for testing purposes.
// It cycles through predefined responses in order.
type LLM struct {
	responses []string
	index     int
}

// NewFakeLLM creates a new fake LLM instance with the provided responses.
func NewFakeLLM(responses []string) *LLM {
	return &LLM{
		responses: responses,
		index:     0,
	}
}

// GenerateContent returns the next predefined response in the cycle.
// It implements the llms interface for generating content responses.
func (f *LLM) GenerateContent(
	_ context.Context,
	_ []schema.MessageContent,
	_ ...llms.CallOption,
) (*llms.ContentResponse, error) {
	if len(f.responses) == 0 {
		return nil, errors.New("no responses configured")
	}

	// Cycle back to the beginning if we've reached the end
	if f.index >= len(f.responses) {
		f.index = 0
	}

	response := f.responses[f.index]
	f.index++

	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: response},
		},
	}, nil
}

// Call provides a simplified interface for generating responses from a string prompt.
// It converts the prompt to the expected message format and returns the response content.
func (f *LLM) Call(
	ctx context.Context,
	prompt string,
	options ...llms.CallOption,
) (string, error) {
	messageContent := []schema.MessageContent{
		{
			Role: schema.ChatMessageTypeHuman,
			Parts: []schema.ContentPart{
				schema.TextContent{Text: prompt},
			},
		},
	}

	resp, err := f.GenerateContent(ctx, messageContent, options...)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("empty response from model")
	}

	return resp.Choices[0].Content, nil
}

// Reset resets the response index back to 0, allowing the response cycle to start over.
func (f *LLM) Reset() {
	f.index = 0
}

// AddResponse appends a new response to the list of available responses.
func (f *LLM) AddResponse(response string) {
	f.responses = append(f.responses, response)
}
