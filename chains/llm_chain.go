package chains

import (
	"context"
	"fmt"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/prompts"
	"github.com/sevigo/goframe/schema"
)

// LLMChainOption configures an LLMChain.
type LLMChainOption[T any] func(*LLMChain[T])

// LLMChain combines a prompt template, an LLM, and an output parser
// into a single callable unit. It renders the prompt, calls the LLM,
// and parses the output into a typed result.
type LLMChain[T any] struct {
	LLM         llms.Model
	Prompt      prompts.PromptTemplate
	Parser      schema.OutputParser[T]
	CallOptions []llms.CallOption
}

// WithOutputParser sets a custom output parser for the chain.
func WithOutputParser[T any](parser schema.OutputParser[T]) LLMChainOption[T] {
	return func(c *LLMChain[T]) {
		c.Parser = parser
	}
}

// WithLLMCallOptions sets LLM call options (temperature, max tokens, etc.)
func WithLLMCallOptions[T any](opts ...llms.CallOption) LLMChainOption[T] {
	return func(c *LLMChain[T]) {
		c.CallOptions = opts
	}
}

// NewLLMChain creates a new LLMChain. If no parser is provided via options,
// a StringParser is used (only valid when T is string).
// Returns an error if llm or prompt is nil.
func NewLLMChain[T any](llm llms.Model, prompt prompts.PromptTemplate, opts ...LLMChainOption[T]) (*LLMChain[T], error) {
	if llm == nil {
		return nil, fmt.Errorf("llm cannot be nil")
	}
	if prompt.Template == "" {
		return nil, fmt.Errorf("prompt template cannot be empty")
	}

	chain := &LLMChain[T]{
		LLM:    llm,
		Prompt: prompt,
	}
	for _, opt := range opts {
		opt(chain)
	}
	return chain, nil
}

// Call renders the prompt with the provided variables, calls the LLM,
// and parses the response. Returns an error if no parser is configured
// and T is not string.
func (c *LLMChain[T]) Call(ctx context.Context, vars map[string]string) (T, error) {
	var zero T

	rendered := c.Prompt.Format(vars)

	raw, err := c.LLM.Call(ctx, rendered, c.CallOptions...)
	if err != nil {
		return zero, fmt.Errorf("llm call failed: %w", err)
	}

	if c.Parser != nil {
		parsed, parseErr := c.Parser.Parse(ctx, raw)
		if parseErr != nil {
			return zero, fmt.Errorf("output parsing failed: %w", parseErr)
		}
		return parsed, nil
	}

	// No parser — try to return raw string if T is string
	var result any = raw
	typed, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("no output parser configured and cannot convert LLM output to %T", zero)
	}
	return typed, nil
}

// GetPrompt renders the prompt without calling the LLM. Useful for debugging.
func (c *LLMChain[T]) GetPrompt(vars map[string]string) string {
	return c.Prompt.Format(vars)
}
