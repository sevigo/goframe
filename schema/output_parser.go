package schema

import "context"

// OutputParser converts raw LLM text output into a structured type.
type OutputParser[T any] interface {
	Parse(ctx context.Context, text string) (T, error)
}

// StringParser is an identity parser that returns the raw LLM output as-is.
type StringParser struct{}

func (StringParser) Parse(_ context.Context, text string) (string, error) {
	return text, nil
}
