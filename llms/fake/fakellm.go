package fake

import (
	"context"
	"errors"
	"sync"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// LLM represents a fake LLM implementation for testing purposes.
// It cycles through predefined responses and records calls for inspection.
type LLM struct {
	mu         sync.Mutex
	responses  []string
	index      int
	lastPrompt string
	callCount  int
}

// NewFakeLLM creates a new fake LLM instance with the provided responses.
func NewFakeLLM(responses []string) *LLM {
	return &LLM{
		responses: responses,
	}
}

// GenerateContent returns the next predefined response in the cycle.
// It implements the llms interface and records the call and prompt content.
func (f *LLM) GenerateContent(
	_ context.Context,
	messages []schema.MessageContent,
	_ ...llms.CallOption,
) (*llms.ContentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.responses) == 0 {
		return nil, errors.New("no responses configured")
	}

	// Record the prompt content and increment call count
	if len(messages) > 0 {
		f.lastPrompt = messages[0].GetTextContent()
	}
	f.callCount++

	// Get current response and advance index
	response := f.responses[f.index]
	f.index = (f.index + 1) % len(f.responses)

	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: response},
		},
	}, nil
}

// Call provides a simplified interface for generating responses from a string prompt.
func (f *LLM) Call(
	ctx context.Context,
	prompt string,
	options ...llms.CallOption,
) (string, error) {
	messageContent := []schema.MessageContent{
		{
			Role:  schema.ChatMessageTypeHuman,
			Parts: []schema.ContentPart{schema.TextContent{Text: prompt}},
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

// Reset resets the response index and call tracking.
func (f *LLM) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.index = 0
	f.callCount = 0
	f.lastPrompt = ""
}

// AddResponse appends a new response to the list of available responses.
func (f *LLM) AddResponse(response string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, response)
}

// LastPrompt returns the last prompt sent to the LLM and whether one exists.
func (f *LLM) LastPrompt() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPrompt, f.lastPrompt != ""
}

// GetCallCount returns the number of times the LLM has been called.
func (f *LLM) GetCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}
