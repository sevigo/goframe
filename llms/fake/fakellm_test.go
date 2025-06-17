package fake_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/llms/fake"
)

func TestNewFakeLLM(t *testing.T) {
	responses := []string{"response1", "response2"}
	fakeLLM := fake.NewFakeLLM(responses)

	assert.NotNil(t, fakeLLM, "NewFakeLLM should not return nil")
}

func TestLLM_GenerateContent(t *testing.T) {
	ctx := context.Background()

	t.Run("successful response cycle", func(t *testing.T) {
		responses := []string{"first", "second", "third"}
		fakeLLM := fake.NewFakeLLM(responses)

		// Test first response
		resp, err := fakeLLM.GenerateContent(ctx, nil)
		assert.NoError(t, err, "should not return error")
		assert.Len(t, resp.Choices, 1, "should have exactly one choice")
		assert.Equal(t, "first", resp.Choices[0].Content, "should return first response")

		// Test second response
		resp, err = fakeLLM.GenerateContent(ctx, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "second", resp.Choices[0].Content, "should return second response")

		// Test third response
		resp, err = fakeLLM.GenerateContent(ctx, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "third", resp.Choices[0].Content, "should return third response")

		// Test cycling back to first
		resp, err = fakeLLM.GenerateContent(ctx, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "first", resp.Choices[0].Content, "should cycle back to first response")
	})

	t.Run("empty responses", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{})

		resp, err := fakeLLM.GenerateContent(ctx, nil)
		assert.Error(t, err, "should return error for empty responses")
		assert.EqualError(t, err, "no responses configured", "should return specific error message")
		assert.Nil(t, resp, "response should be nil on error")
	})

	t.Run("single response cycles correctly", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"only response"})

		// First call
		resp, err := fakeLLM.GenerateContent(ctx, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "only response", resp.Choices[0].Content, "should return the only response")

		// Second call should cycle back
		resp, err = fakeLLM.GenerateContent(ctx, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "only response", resp.Choices[0].Content, "should cycle back to the only response")
	})
}

func TestLLM_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("successful call", func(t *testing.T) {
		responses := []string{"hello world", "second response"}
		fakeLLM := fake.NewFakeLLM(responses)

		result, err := fakeLLM.Call(ctx, "test prompt")
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "hello world", result, "should return first response")

		// Test second call
		result, err = fakeLLM.Call(ctx, "another prompt")
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "second response", result, "should return second response")
	})

	t.Run("empty responses", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{})

		result, err := fakeLLM.Call(ctx, "test prompt")
		require.Error(t, err, "should return error for empty responses")
		assert.EqualError(t, err, "no responses configured", "should return specific error message")
		assert.Empty(t, result, "result should be empty on error")
	})

	t.Run("call with options", func(t *testing.T) {
		responses := []string{"response with options"}
		fakeLLM := fake.NewFakeLLM(responses)

		result, err := fakeLLM.Call(ctx, "test prompt")
		require.NoError(t, err, "should not return error")
		assert.Equal(t, "response with options", result, "should return response regardless of options")
	})
}

func TestLLM_Reset(t *testing.T) {
	responses := []string{"first", "second", "third"}
	fakeLLM := fake.NewFakeLLM(responses)

	// Advance through responses
	_, err := fakeLLM.GenerateContent(context.Background(), nil)
	require.NoError(t, err)
	_, err = fakeLLM.GenerateContent(context.Background(), nil)
	require.NoError(t, err)

	// Reset and verify it starts from the beginning
	fakeLLM.Reset()

	// Verify next response is first again
	resp, err := fakeLLM.GenerateContent(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "first", resp.Choices[0].Content, "should return first response after reset")
}

func TestLLM_AddResponse(t *testing.T) {
	fakeLLM := fake.NewFakeLLM([]string{"initial"})

	fakeLLM.AddResponse("added response")

	// Test that the new response is used in cycle
	_, err := fakeLLM.GenerateContent(context.Background(), nil) // "initial"
	assert.NoError(t, err)

	resp, err := fakeLLM.GenerateContent(context.Background(), nil) // "added response"
	assert.NoError(t, err)
	assert.Equal(t, "added response", resp.Choices[0].Content, "should return the added response")
}

func TestLLM_Integration(t *testing.T) {
	t.Run("full workflow", func(t *testing.T) {
		fakeLLM := fake.NewFakeLLM([]string{"response1", "response2"})

		// Test Call method
		result1, err := fakeLLM.Call(context.Background(), "prompt1")
		require.NoError(t, err)
		assert.Equal(t, "response1", result1)

		// Test GenerateContent method
		resp, err := fakeLLM.GenerateContent(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "response2", resp.Choices[0].Content)

		// Test cycling
		result2, err := fakeLLM.Call(context.Background(), "prompt2")
		require.NoError(t, err)
		assert.Equal(t, "response1", result2, "should cycle back to first response")

		// Test reset
		fakeLLM.Reset()
		result3, err := fakeLLM.Call(context.Background(), "prompt3")
		require.NoError(t, err)
		assert.Equal(t, "response1", result3, "should start from first response after reset")

		// Test add response
		fakeLLM.AddResponse("response3")
		_, err = fakeLLM.Call(context.Background(), "prompt4") // response2
		require.NoError(t, err)
		result4, err := fakeLLM.Call(context.Background(), "prompt5")
		require.NoError(t, err)
		assert.Equal(t, "response3", result4, "should return newly added response")
	})
}
