package chains_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/chains"
	"github.com/sevigo/goframe/llms/fake"
	"github.com/sevigo/goframe/prompts"
	"github.com/sevigo/goframe/schema"
)

// testReview is a simple struct to test typed output parsing.
type testReview struct {
	Summary  string
	Severity string
}

// testReviewParser parses "SUMMARY|SEVERITY" format into testReview.
type testReviewParser struct{}

func (testReviewParser) Parse(_ context.Context, text string) (testReview, error) {
	parts := strings.SplitN(text, "|", 2)
	if len(parts) != 2 {
		return testReview{}, fmt.Errorf("expected SUMMARY|SEVERITY, got: %s", text)
	}
	return testReview{
		Summary:  strings.TrimSpace(parts[0]),
		Severity: strings.TrimSpace(parts[1]),
	}, nil
}

var _ schema.OutputParser[testReview] = testReviewParser{}

func TestLLMChain_Call(t *testing.T) {
	ctx := context.Background()
	tmpl := prompts.NewPromptTemplate("Review this: {{.code}}")

	t.Run("string output without parser", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"Looks good, no issues found."})
		chain, err := chains.NewLLMChain[string](fakeLLM, tmpl)
		require.NoError(t, err)

		result, err := chain.Call(ctx, map[string]string{"code": "fmt.Println()"})

		require.NoError(t, err)
		assert.Equal(t, "Looks good, no issues found.", result)

		lastPrompt, _ := fakeLLM.LastPrompt()
		assert.Equal(t, "Review this: fmt.Println()", lastPrompt)
	})

	t.Run("typed output with custom parser", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"No issues found|low"})
		chain, err := chains.NewLLMChain[testReview](
			fakeLLM, tmpl,
			chains.WithOutputParser[testReview](testReviewParser{}),
		)
		require.NoError(t, err)

		result, err := chain.Call(ctx, map[string]string{"code": "x := 1"})

		require.NoError(t, err)
		assert.Equal(t, "No issues found", result.Summary)
		assert.Equal(t, "low", result.Severity)
	})

	t.Run("LLM error is propagated", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{})
		chain, err := chains.NewLLMChain[string](fakeLLM, tmpl)
		require.NoError(t, err)

		_, err = chain.Call(ctx, map[string]string{"code": "x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm call failed")
	})

	t.Run("parser error is propagated", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"bad format without pipe"})
		chain, err := chains.NewLLMChain[testReview](
			fakeLLM, tmpl,
			chains.WithOutputParser[testReview](testReviewParser{}),
		)
		require.NoError(t, err)

		_, err = chain.Call(ctx, map[string]string{"code": "x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "output parsing failed")
	})

	t.Run("non-string type without parser returns error", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"some output"})
		chain, err := chains.NewLLMChain[testReview](fakeLLM, tmpl)
		require.NoError(t, err)

		_, err = chain.Call(ctx, map[string]string{"code": "x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no output parser configured")
	})
}

func TestLLMChain_GetPrompt(t *testing.T) {
	tmpl := prompts.NewPromptTemplate("Analyze: {{.input}}")
	
	// Create a chain correctly so we can test GetPrompt
	fakeLLM := fake.NewFakeLLM([]string{"some output"})
	chain, err := chains.NewLLMChain[string](fakeLLM, tmpl)
	require.NoError(t, err)

	result := chain.GetPrompt(map[string]string{"input": "test data"})
	assert.Equal(t, "Analyze: test data", result)
}

func TestNewLLMChain_Validation(t *testing.T) {
	tmpl := prompts.NewPromptTemplate("Analyze: {{.input}}")
	fakeLLM := fake.NewFakeLLM([]string{"some output"})

	t.Run("nil LLM returns error", func(t *testing.T) {
		chain, err := chains.NewLLMChain[string](nil, tmpl)
		require.Error(t, err)
		assert.Nil(t, chain)
		assert.Contains(t, err.Error(), "llm cannot be nil")
	})

	t.Run("empty prompt template returns error", func(t *testing.T) {
		emptyTmpl := prompts.NewPromptTemplate("")
		chain, err := chains.NewLLMChain[string](fakeLLM, emptyTmpl)
		require.Error(t, err)
		assert.Nil(t, chain)
		assert.Contains(t, err.Error(), "prompt template cannot be empty")
	})
}

// failingParser always returns an error, for testing error paths.
type failingParser struct{}

func (failingParser) Parse(_ context.Context, _ string) (string, error) {
	return "", errors.New("parse failed intentionally")
}

var _ schema.OutputParser[string] = failingParser{}

func TestLLMChain_WithStringParser(t *testing.T) {
	ctx := context.Background()
	fakeLLM := fake.NewFakeLLM([]string{"raw output"})
	tmpl := prompts.NewPromptTemplate("{{.q}}")

	chain, err := chains.NewLLMChain[string](
		fakeLLM, tmpl,
		chains.WithOutputParser[string](schema.StringParser{}),
	)
	require.NoError(t, err)

	result, err := chain.Call(ctx, map[string]string{"q": "test"})
	require.NoError(t, err)
	assert.Equal(t, "raw output", result)
}
