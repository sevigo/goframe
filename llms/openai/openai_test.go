package openai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
	"github.com/stretchr/testify/assert"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

func TestApplyOptionsDefaults(t *testing.T) {
	opts := applyOptions()

	assert.Equal(t, "gpt-4o", opts.model)
	assert.Equal(t, DefaultEmbeddingModel, opts.embeddingModel)
	assert.Equal(t, DefaultRetryAttempts, opts.retryAttempts)
	assert.Equal(t, DefaultRetryDelay, opts.retryDelay)
	assert.Equal(t, DefaultMaxRetryDelay, opts.maxRetryDelay)
	assert.Equal(t, DefaultRetryJitter, opts.retryJitter)
	assert.Equal(t, DefaultTimeout, opts.requestTimeout)
}

func TestWithModel(t *testing.T) {
	opts := applyOptions(WithModel("gpt-4o-mini"))
	assert.Equal(t, "gpt-4o-mini", opts.model)
}

func TestWithEmbeddingModel(t *testing.T) {
	opts := applyOptions(WithEmbeddingModel("text-embedding-3-large"))
	assert.Equal(t, "text-embedding-3-large", opts.embeddingModel)
}

func TestWithAPIKey(t *testing.T) {
	opts := applyOptions(WithAPIKey("sk-test"))
	assert.Equal(t, "sk-test", opts.apiKey)
}

func TestWithBaseURL(t *testing.T) {
	opts := applyOptions(WithBaseURL("https://custom.api.com/v1"))
	assert.Equal(t, "https://custom.api.com/v1", opts.baseURL)
}

func TestWithLogger(t *testing.T) {
	customLogger := slog.Default()
	opts := applyOptions(WithLogger(customLogger))
	assert.Equal(t, customLogger, opts.logger)

	opts = applyOptions(WithLogger(nil))
	assert.Equal(t, slog.Default(), opts.logger)
}

func TestWithRetryAttempts(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		expected int
	}{
		{"valid attempts", 5, 5},
		{"zero attempts", 0, 0},
		{"negative attempts should be ignored", -1, DefaultRetryAttempts},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := applyOptions(WithRetryAttempts(tt.attempts))
			assert.Equal(t, tt.expected, opts.retryAttempts)
		})
	}
}

func TestWithRetryDelay(t *testing.T) {
	opts := applyOptions(WithRetryDelay(5 * time.Second))
	assert.Equal(t, 5*time.Second, opts.retryDelay)

	opts = applyOptions(WithRetryDelay(0))
	assert.Equal(t, DefaultRetryDelay, opts.retryDelay)
}

func TestWithMaxRetryDelay(t *testing.T) {
	opts := applyOptions(WithMaxRetryDelay(60 * time.Second))
	assert.Equal(t, 60*time.Second, opts.maxRetryDelay)
}

func TestWithRetryJitter(t *testing.T) {
	opts := applyOptions(WithRetryJitter(2 * time.Second))
	assert.Equal(t, 2*time.Second, opts.retryJitter)
}

func TestWithRequestTimeout(t *testing.T) {
	opts := applyOptions(WithRequestTimeout(60 * time.Second))
	assert.Equal(t, 60*time.Second, opts.requestTimeout)

	opts = applyOptions(WithRequestTimeout(0))
	assert.Equal(t, DefaultTimeout, opts.requestTimeout)
}

func TestWithThinking(t *testing.T) {
	opts := applyOptions(WithThinking(true))
	assert.NotNil(t, opts.thinking)
	assert.True(t, *opts.thinking)
}

func TestWithReasoningEffort(t *testing.T) {
	opts := applyOptions(WithReasoningEffort("high"))
	assert.Equal(t, "high", opts.reasoningEffort)
}

func TestWithOrganization(t *testing.T) {
	opts := applyOptions(WithOrganization("org-123"))
	assert.Equal(t, "org-123", opts.organization)
}

func TestWithProject(t *testing.T) {
	opts := applyOptions(WithProject("proj-456"))
	assert.Equal(t, "proj-456", opts.project)
}

func TestNewRequiresAPIKey(t *testing.T) {
	_, err := New()
	assert.ErrorIs(t, err, ErrNoAPIKey)
}

func TestNewWithAPIKey(t *testing.T) {
	llm, err := New(WithAPIKey("sk-test-key"))
	assert.NoError(t, err)
	assert.NotNil(t, llm)
}

func TestNewWithCustomModel(t *testing.T) {
	llm, err := New(WithAPIKey("sk-test"), WithModel("gpt-4o-mini"))
	assert.NoError(t, err)
	assert.NotNil(t, llm)
	assert.Equal(t, "gpt-4o-mini", llm.options.model)
}

func TestNewWithBaseURL(t *testing.T) {
	llm, err := New(
		WithAPIKey("sk-test"),
		WithBaseURL("https://my-proxy.example.com/v1"),
	)
	assert.NoError(t, err)
	assert.NotNil(t, llm)
}

func TestNewWithRetryAttemptsZero(t *testing.T) {
	llm, err := New(WithAPIKey("sk-test"), WithRetryAttempts(0))
	assert.NoError(t, err)
	assert.NotNil(t, llm)
	assert.Equal(t, 0, llm.options.retryAttempts)
}

func TestIsRetryableError(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"unexpected EOF", errors.New("unexpected EOF"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"rate limit 429", errors.New("rate_limit exceeded"), true},
		{"rate limit text", errors.New("rate limit"), true},
		{"503", errors.New("503 service unavailable"), true},
		{"502", errors.New("502 bad gateway"), true},
		{"server_error", errors.New("server_error: internal"), true},
		{"nil error", nil, false},
		{"non-retryable error", errors.New("invalid model name"), false},
		{"bad request", errors.New("bad request: invalid parameter"), false},
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"wrapped context canceled", fmt.Errorf("wrapped: %w", context.Canceled), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := llm.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateNextDelay(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	tests := []struct {
		name     string
		delay    time.Duration
		expected time.Duration
	}{
		{"double the delay", 2 * time.Second, 4 * time.Second},
		{"double again", 4 * time.Second, 8 * time.Second},
		{"capped at max", 20 * time.Second, DefaultMaxRetryDelay},
		{"already at max", DefaultMaxRetryDelay, DefaultMaxRetryDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := llm.calculateNextDelay(tt.delay)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDoWithRetryNoRetry(t *testing.T) {
	opts := applyOptions(WithRetryAttempts(0))
	llm := &LLM{options: opts, logger: opts.logger}

	callCount := 0
	err := llm.doWithRetry(context.Background(), func() error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDoWithRetrySuccess(t *testing.T) {
	opts := applyOptions(WithRetryAttempts(3), WithRetryDelay(10*time.Millisecond))
	llm := &LLM{options: opts, logger: opts.logger}

	callCount := 0
	err := llm.doWithRetry(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return errors.New("connection refused")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestDoWithRetryNonRetryableError(t *testing.T) {
	opts := applyOptions(WithRetryAttempts(3), WithRetryDelay(10*time.Millisecond))
	llm := &LLM{options: opts, logger: opts.logger}

	callCount := 0
	err := llm.doWithRetry(context.Background(), func() error {
		callCount++
		return errors.New("invalid model name")
	})

	assert.Error(t, err)
	assert.Equal(t, 1, callCount, "should not retry non-retryable errors")
}

func TestDoWithRetryExhausted(t *testing.T) {
	opts := applyOptions(WithRetryAttempts(2), WithRetryDelay(10*time.Millisecond))
	llm := &LLM{options: opts, logger: opts.logger}

	callCount := 0
	err := llm.doWithRetry(context.Background(), func() error {
		callCount++
		return errors.New("connection refused")
	})

	assert.Error(t, err)
	assert.Equal(t, 3, callCount, "should try initial + 2 retries = 3 total attempts")
}

func TestDoWithRetryContextCancellation(t *testing.T) {
	opts := applyOptions(WithRetryAttempts(10), WithRetryDelay(5*time.Second))
	llm := &LLM{options: opts, logger: opts.logger}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	callCount := 0
	err := llm.doWithRetry(ctx, func() error {
		callCount++
		return errors.New("connection refused")
	})

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, callCount, "should stop after context cancellation")
}

func TestDetermineModel(t *testing.T) {
	llm := &LLM{options: applyOptions(WithModel("gpt-4o")), logger: slog.Default()}

	assert.Equal(t, "gpt-4o", llm.determineModel(llms.CallOptions{}))
	assert.Equal(t, "gpt-4o-mini", llm.determineModel(llms.CallOptions{Model: "gpt-4o-mini"}))
}

func TestConvertMessages(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewSystemMessage("You are a helpful assistant."),
		schema.NewHumanMessage("Hello!"),
		schema.NewAIMessage("Hi there!"),
	}

	result := llm.convertMessages(messages)
	assert.Len(t, result, 3)

	assert.NotNil(t, result[0].OfSystem)
	assert.NotNil(t, result[1].OfUser)
	assert.NotNil(t, result[2].OfAssistant)
}

func TestConvertHumanMessageWithImage(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	msg := schema.NewHumanMessageWithImage("What's in this image?", "dGVzdA==", "image/png")

	result := llm.convertHumanMessage(msg)
	assert.NotNil(t, result.OfUser)
}

func TestConvertToolMessage(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	msg := schema.NewToolResultMessage("get_weather", `{"temp": 72}`)

	result := llm.convertToolMessage(msg)
	assert.NotNil(t, result.OfTool)
}

func TestConvertTools(t *testing.T) {
	tools := []llms.ToolDefinition{
		{
			Type: "function",
			Function: llms.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	result := convertTools(tools)
	assert.Len(t, result, 1)
	assert.Equal(t, "function", string(result[0].Type))
	assert.Equal(t, "get_weather", result[0].Function.Name)
	assert.False(t, param.IsOmitted(result[0].Function.Description))
}

func TestConvertToolCalls(t *testing.T) {
	toolCalls := []openai.ChatCompletionMessageToolCall{
		{
			ID:   "call_abc123",
			Type: "function",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "get_weather",
				Arguments: `{"location": "NYC"}`,
			},
		},
	}

	result := convertToolCalls(slog.Default(), toolCalls)
	assert.Len(t, result, 1)
	assert.Equal(t, "call_abc123", result[0].ID)
	assert.Equal(t, "get_weather", result[0].Function.Name)
	assert.Equal(t, "NYC", result[0].Function.Arguments["location"])
}

func TestConvertToolCallsEmpty(t *testing.T) {
	result := convertToolCalls(slog.Default(), nil)
	assert.Nil(t, result)
}

func TestBuildChatParams(t *testing.T) {
	llm := &LLM{options: applyOptions(WithModel("gpt-4o")), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("Hello!"),
	}

	opts := llms.CallOptions{}
	llms.WithTemperature(0.7)(&opts)
	llms.WithMaxTokens(100)(&opts)
	llms.WithTopP(0.9)(&opts)
	llms.WithSeed(42)(&opts)

	params := llm.buildChatParams("gpt-4o", messages, opts)

	assert.Equal(t, "gpt-4o", params.Model)
	assert.False(t, param.IsOmitted(params.Temperature))
	assert.False(t, param.IsOmitted(params.MaxCompletionTokens))
	assert.False(t, param.IsOmitted(params.TopP))
	assert.False(t, param.IsOmitted(params.Seed))
	assert.NotNil(t, params.Messages)
}

func TestBuildChatParamsWithStopWords(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("Hello!"),
	}

	opts := llms.CallOptions{}
	llms.WithStopWords([]string{"STOP", "END"})(&opts)

	params := llm.buildChatParams("gpt-4o", messages, opts)

	assert.Equal(t, []string{"STOP", "END"}, params.Stop.OfStringArray)
}

func TestBuildChatParamsWithJSONMode(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("Hello!"),
	}

	opts := llms.CallOptions{}
	llms.WithJSONMode(true)(&opts)

	params := llm.buildChatParams("gpt-4o", messages, opts)

	assert.NotNil(t, params.ResponseFormat.OfJSONObject)
}

func TestBuildChatParamsWithJSONSchema(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("Hello!"),
	}

	opts := llms.CallOptions{}
	llms.WithJSONSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	})(&opts)

	params := llm.buildChatParams("gpt-4o", messages, opts)

	assert.NotNil(t, params.ResponseFormat.OfJSONSchema)
}

func TestBuildChatParamsWithTools(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("What's the weather?"),
	}

	opts := llms.CallOptions{}
	llms.WithTools([]llms.ToolDefinition{
		{
			Type: "function",
			Function: llms.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
	})(&opts)

	params := llm.buildChatParams("gpt-4o", messages, opts)

	assert.Len(t, params.Tools, 1)
	assert.Equal(t, "get_weather", params.Tools[0].Function.Name)
}

func TestInvalidateDimensionCache(t *testing.T) {
	llm := &LLM{
		options: applyOptions(),
		logger:  slog.Default(),
	}

	llm.dimension = 1536

	llm.InvalidateDimensionCache()

	llm.dimMu.Lock()
	assert.Equal(t, 0, llm.dimension)
	llm.dimMu.Unlock()
}

func TestBuildChatParamsWithThinking(t *testing.T) {
	llm := &LLM{options: applyOptions(WithThinking(true)), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("Think about this."),
	}

	opts := llms.CallOptions{}
	params := llm.buildChatParams("o3-mini", messages, opts)

	assert.Equal(t, shared.ReasoningEffort("medium"), params.ReasoningEffort)
}

func TestBuildChatParamsWithReasoningEffort(t *testing.T) {
	llm := &LLM{options: applyOptions(WithReasoningEffort("high")), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("Think carefully."),
	}

	opts := llms.CallOptions{}
	params := llm.buildChatParams("o3-mini", messages, opts)

	assert.Equal(t, shared.ReasoningEffort("high"), params.ReasoningEffort)
}

func TestConvertAIMessageWithToolCalls(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	msg := schema.NewAIMessageWithToolCalls("I'll check the weather.", []schema.ToolCallContent{
		{
			ID:           "call_abc123",
			FunctionName: "get_weather",
			Arguments:    map[string]any{"location": "NYC"},
		},
	})

	result := llm.convertAIMessage(msg)
	assert.NotNil(t, result.OfAssistant)
	assert.Len(t, result.OfAssistant.ToolCalls, 1)
	assert.Equal(t, "call_abc123", result.OfAssistant.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", result.OfAssistant.ToolCalls[0].Function.Name)
}

func TestConvertToolMessageWithCallID(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	msg := schema.NewToolResultMessageWithID("get_weather", "call_abc123", `{"temp": 72}`)

	result := llm.convertToolMessage(msg)
	assert.NotNil(t, result.OfTool)
}

func TestConvertMessagesWithToolCalls(t *testing.T) {
	llm := &LLM{options: applyOptions(), logger: slog.Default()}

	messages := []schema.MessageContent{
		schema.NewHumanMessage("What's the weather?"),
		schema.NewAIMessageWithToolCalls("", []schema.ToolCallContent{
			{
				ID:           "call_001",
				FunctionName: "get_weather",
				Arguments:    map[string]any{"location": "Paris"},
			},
		}),
		schema.NewToolResultMessageWithID("get_weather", "call_001", `{"temp": 18}`),
	}

	result := llm.convertMessages(messages)
	assert.Len(t, result, 3)
	assert.NotNil(t, result[1].OfAssistant)
	assert.Len(t, result[1].OfAssistant.ToolCalls, 1)
	assert.NotNil(t, result[2].OfTool)
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"normal key", "sk-1234567890abcdef", "sk-1****"},
		{"short key", "abc", "****"},
		{"empty key", "", "****"},
		{"exactly 4 chars", "abcd", "****"},
		{"5 chars", "abcde", "abcd****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskAPIKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewWithAllOptions(t *testing.T) {
	llm, err := New(
		WithAPIKey("sk-test-key"),
		WithModel("gpt-4o-mini"),
		WithBaseURL("https://proxy.example.com/v1"),
		WithEmbeddingModel("text-embedding-3-large"),
		WithOrganization("org-123"),
		WithProject("proj-456"),
		WithRetryAttempts(5),
		WithRetryDelay(3*time.Second),
		WithMaxRetryDelay(60*time.Second),
		WithRetryJitter(500*time.Millisecond),
		WithRequestTimeout(180*time.Second),
		WithLogger(slog.Default()),
	)
	assert.NoError(t, err)
	assert.NotNil(t, llm)
	assert.Equal(t, "gpt-4o-mini", llm.options.model)
	assert.Equal(t, "text-embedding-3-large", llm.options.embeddingModel)
	assert.Equal(t, 5, llm.options.retryAttempts)
	assert.Equal(t, 3*time.Second, llm.options.retryDelay)
	assert.Equal(t, 60*time.Second, llm.options.maxRetryDelay)
	assert.Equal(t, 500*time.Millisecond, llm.options.retryJitter)
	assert.Equal(t, 180*time.Second, llm.options.requestTimeout)
}
