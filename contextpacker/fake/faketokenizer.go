package fake

import (
	"context"
	"strings"

	"github.com/sevigo/goframe/llms"
)

// Tokenizer is a fake tokenizer for testing.
// It counts tokens by splitting on whitespace.
type Tokenizer struct {
	// TokensPerWord is the number of tokens per word (default 1).
	TokensPerWord int
	// FixedCount, if set, returns this count instead of counting.
	FixedCount int
	// Err, if set, is returned by CountTokens.
	Err error
}

// Compile-time assertion that Tokenizer implements llms.Tokenizer.
var _ llms.Tokenizer = (*Tokenizer)(nil)

// NewTokenizer creates a new fake tokenizer with sensible defaults.
func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		TokensPerWord: 1,
	}
}

// CountTokens returns the token count for the given text.
func (f *Tokenizer) CountTokens(_ context.Context, text string) (int, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	if f.FixedCount > 0 {
		return f.FixedCount, nil
	}
	if text == "" {
		return 0, nil
	}

	words := strings.Fields(text)
	return len(words) * f.TokensPerWord, nil
}
