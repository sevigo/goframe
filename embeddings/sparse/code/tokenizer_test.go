package code

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenizer_Tokenize(t *testing.T) {
	tok := NewTokenizer()

	tests := []struct {
		name  string
		input string
		check func(tokens []string)
	}{
		{
			name:  "camelCase splits correctly",
			input: "getUserByID",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "get")
				assert.Contains(t, tokens, "user")
				assert.Contains(t, tokens, "id")
			},
		},
		{
			name:  "snake_case splits correctly",
			input: "get_user_by_id",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "get")
				assert.Contains(t, tokens, "user")
				assert.Contains(t, tokens, "id")
			},
		},
		{
			name:  "operators are split",
			input: "abc + xyz",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "abc")
				assert.Contains(t, tokens, "xyz")
			},
		},
		{
			name:  "acronym prefix (XMLParser)",
			input: "XMLParser",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "xml")
				assert.Contains(t, tokens, "parser")
				assert.NotContains(t, tokens, "xmlparser", "should be split, not kept whole")
			},
		},
		{
			name:  "acronym standalone (HTTPClient)",
			input: "HTTPClient",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "http")
				assert.Contains(t, tokens, "client")
			},
		},
		{
			name:  "mixed camel and snake (get_HTTPClient)",
			input: "get_HTTPClient",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "get")
				assert.Contains(t, tokens, "http")
				assert.Contains(t, tokens, "client")
			},
		},
		{
			name:  "case-insensitive: processPayment and ProcessPayment same tokens",
			input: "ProcessPayment",
			check: func(tokens []string) {
				assert.Contains(t, tokens, "process")
				assert.Contains(t, tokens, "payment")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tok.Tokenize(tt.input)
			tt.check(tokens)
		})
	}
}

func TestCodeSparseProvider_GenerateSparseVector(t *testing.T) {
	provider := NewCodeSparseProvider()
	ctx := context.Background()

	vec, err := provider.GenerateSparseVector(ctx, "func getUserByID() string { return userID }")
	require.NoError(t, err)
	require.NotNil(t, vec)

	assert.NotEmpty(t, vec.Indices)
	assert.NotEmpty(t, vec.Values)
	assert.Len(t, vec.Values, len(vec.Indices))

	sum := float32(0)
	for _, v := range vec.Values {
		sum += v * v
	}
	assert.InDelta(t, 1.0, sum, 0.01, "sparse vector should be normalized")
}
