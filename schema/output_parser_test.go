package schema_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/schema"
)

func TestStringParser_Parse(t *testing.T) {
	parser := schema.StringParser{}

	t.Run("returns input unchanged", func(t *testing.T) {
		result, err := parser.Parse(context.Background(), "hello world")
		require.NoError(t, err)
		assert.Equal(t, "hello world", result)
	})

	t.Run("handles empty string", func(t *testing.T) {
		result, err := parser.Parse(context.Background(), "")
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("preserves whitespace and newlines", func(t *testing.T) {
		input := "  line1\n\tline2  \n"
		result, err := parser.Parse(context.Background(), input)
		require.NoError(t, err)
		assert.Equal(t, input, result)
	})
}

// Compile-time check: StringParser implements OutputParser[string]
var _ schema.OutputParser[string] = schema.StringParser{}
