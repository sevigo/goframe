package schema

import "context"

// OutputParser converts raw LLM text output into a structured type.
// Implementations can parse JSON, CSV, or custom formats into typed values.
type OutputParser[T any] interface {
	// Parse converts the raw text output into type T.
	Parse(ctx context.Context, text string) (T, error)
}

// StringParser is an identity parser that returns the raw LLM output as-is.
// Use StringParser when no parsing is needed and the raw string is sufficient.
type StringParser struct{}

// Parse returns the input text unchanged.
func (StringParser) Parse(_ context.Context, text string) (string, error) {
	return text, nil
}
