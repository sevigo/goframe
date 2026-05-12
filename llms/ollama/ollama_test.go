package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

func TestDefaultHTTPClientConfiguration(t *testing.T) {
	assert.NotNil(t, defaultHTTPClient)
	assert.Equal(t, DefaultTimeout, defaultHTTPClient.Timeout)

	transport, ok := defaultHTTPClient.Transport.(*http.Transport)
	assert.True(t, ok)
	assert.Equal(t, DefaultMaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, DefaultMaxIdleConnsHost, transport.MaxIdleConnsPerHost)
	assert.Equal(t, DefaultIdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, DefaultTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
}

func TestNewDoesNotMutateDefaultHTTPClient(t *testing.T) {
	origTransport := defaultHTTPClient.Transport

	u, _ := url.Parse("http://localhost:1")
	_, err := New(
		WithModel("test-model"),
		WithServerURL(u.String()),
	)
	assert.NoError(t, err)
	assert.Equal(t, origTransport, defaultHTTPClient.Transport, "defaultHTTPClient should not be mutated")

	_, err = New(
		WithModel("test-model"),
		WithServerURL(u.String()),
	)
	assert.NoError(t, err)
	assert.Equal(t, origTransport, defaultHTTPClient.Transport, "second New() should not mutate defaultHTTPClient")
}

func TestApplyOptionsDefaults(t *testing.T) {
	opts := applyOptions()

	assert.Equal(t, DefaultRetryAttempts, opts.retryAttempts)
	assert.Equal(t, DefaultRetryDelay, opts.retryDelay)
	assert.Equal(t, DefaultMaxRetryDelay, opts.maxRetryDelay)
	assert.Equal(t, DefaultRetryJitter, opts.retryJitter)
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
		{"http2 GOAWAY", errors.New("http2: server sent GOAWAY"), true},
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

func TestDoWithRetryContextCanceledNotRetryable(t *testing.T) {
	opts := applyOptions(WithRetryAttempts(3), WithRetryDelay(10*time.Millisecond))
	llm := &LLM{options: opts, logger: opts.logger}

	callCount := 0
	err := llm.doWithRetry(context.Background(), func() error {
		callCount++
		return context.Canceled
	})

	assert.Error(t, err)
	assert.Equal(t, 1, callCount, "context.Canceled should not be retried")
}

func TestWithServerURL(t *testing.T) {
	tests := []struct {
		name        string
		inputURL    string
		expectedURL string
		shouldParse bool
	}{
		{"valid URL", "http://localhost:8080", "http://localhost:8080", true},
		{"valid URL with path", "http://localhost:8080/api", "http://localhost:8080/api", true},
		{"invalid URL", "://invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := applyOptions(WithServerURL(tt.inputURL))
			if tt.shouldParse {
				assert.NotNil(t, opts.ollamaServerURL)
				assert.Equal(t, tt.expectedURL, opts.ollamaServerURL.String())
			} else {
				assert.Nil(t, opts.ollamaServerURL)
			}
		})
	}
}

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 30 * time.Second}
	opts := applyOptions(WithHTTPClient(customClient))
	assert.Equal(t, customClient, opts.httpClient)

	opts = applyOptions(WithHTTPClient(nil))
	assert.Nil(t, opts.httpClient)
}

func TestWithModel(t *testing.T) {
	opts := applyOptions(WithModel("llama2"))
	assert.Equal(t, "llama2", opts.model)
}

func TestWithAPIKey(t *testing.T) {
	opts := applyOptions(WithAPIKey("test-api-key"))
	assert.Equal(t, "test-api-key", opts.apiKey)
}

func TestNewUsesDefaultHTTPClient(t *testing.T) {
	u, _ := url.Parse("http://localhost:1")

	llm, err := New(
		WithModel("test-model"),
		WithServerURL(u.String()),
	)
	assert.NoError(t, err)
	assert.NotNil(t, llm)
}

func TestNewWithCustomHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 30 * time.Second}
	u, _ := url.Parse("http://localhost:1")

	llm, err := New(
		WithModel("test-model"),
		WithServerURL(u.String()),
		WithHTTPClient(customClient),
	)
	assert.NoError(t, err)
	assert.NotNil(t, llm)
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

func TestCallOptionsPresenceTracking(t *testing.T) {
	opts := llms.CallOptions{}
	assert.False(t, opts.TemperatureSet(), "default should not have Temperature set")
	assert.False(t, opts.SeedSet(), "default should not have Seed set")

	llms.WithTemperature(0.0)(&opts)
	assert.True(t, opts.TemperatureSet(), "WithTemperature(0.0) should mark Temperature as set")
	assert.Equal(t, 0.0, opts.Temperature)

	llms.WithSeed(0)(&opts)
	assert.True(t, opts.SeedSet(), "WithSeed(0) should mark Seed as set")
	assert.Equal(t, 0, opts.Seed)

	opts2 := llms.CallOptions{}
	llms.WithTemperature(0.7)(&opts2)
	assert.True(t, opts2.TemperatureSet())
	assert.Equal(t, 0.7, opts2.Temperature)
}

func TestBuildOllamaOptionsZeroTemperature(t *testing.T) {
	opts := llms.CallOptions{}
	llms.WithTemperature(0.0)(&opts)

	result := buildOllamaOptions(opts)
	assert.NotNil(t, result, "should allocate map when Temperature is explicitly set to 0")
	assert.Equal(t, float32(0), result["temperature"])
}

func TestBuildOllamaOptionsZeroSeed(t *testing.T) {
	opts := llms.CallOptions{}
	llms.WithSeed(0)(&opts)

	result := buildOllamaOptions(opts)
	assert.NotNil(t, result, "should allocate map when Seed is explicitly set to 0")
	assert.Equal(t, 0, result["seed"])
}

func TestBuildOllamaOptionsEmptyReturnsNil(t *testing.T) {
	opts := llms.CallOptions{}
	result := buildOllamaOptions(opts)
	assert.Nil(t, result, "should return nil when no options are set")
}

func TestExtractEmbeddingLength(t *testing.T) {
	tests := []struct {
		name      string
		modelInfo map[string]any
		expected  int64
	}{
		{
			"gemma3 embedding_length",
			map[string]any{"gemma3.embedding_length": int64(2560), "gemma3.context_length": int64(131072)},
			2560,
		},
		{
			"llama embedding_length as float64",
			map[string]any{"llama.embedding_length": float64(4096)},
			4096,
		},
		{
			"no embedding_length key",
			map[string]any{"general.architecture": "gemma3"},
			0,
		},
		{
			"nil model_info",
			nil,
			0,
		},
		{
			"embedding_length as json.Number",
			map[string]any{"gemma3.embedding_length": json.Number("2560")},
			2560,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEmbeddingLength(tt.modelInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int64
		ok       bool
	}{
		{"int", int(42), 42, true},
		{"int64", int64(42), 42, true},
		{"float64", float64(42.0), 42, true},
		{"json.Number", json.Number("42"), 42, true},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toInt64(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestInvalidateModelDetailsCache(t *testing.T) {
	llm := &LLM{
		options: applyOptions(),
		logger:  slog.Default(),
	}

	llm.details = &schema.ModelDetails{Family: "test"}
	llm.detailsErr = errors.New("cached error")

	llm.InvalidateModelDetailsCache()

	llm.detailsMu.RLock()
	assert.Nil(t, llm.details, "details should be nil after invalidation")
	assert.Nil(t, llm.detailsErr, "detailsErr should be nil after invalidation")
	llm.detailsMu.RUnlock()
}

func TestBuildChatRequest_WithLogprobs(t *testing.T) {
	llm := &LLM{
		options: applyOptions(),
		logger:  slog.Default(),
	}

	opts := llms.CallOptions{}
	llms.WithLogprobs(true)(&opts)
	llms.WithTopLogprobs(5)(&opts)

	req := llm.buildChatRequest("test-model", nil, opts)
	assert.True(t, req.Logprobs)
	assert.Equal(t, 5, req.TopLogprobs)
}

func TestBuildGenerationInfo_WithLogprobs(t *testing.T) {
	llm := &LLM{
		options: applyOptions(),
		logger:  slog.Default(),
	}

	handler := &chatResponseHandler{
		logprobs: []api.Logprob{
			{
				TokenLogprob: api.TokenLogprob{
					Token:   "hello",
					Logprob: -0.0123,
				},
			},
		},
	}

	var finalResp api.ChatResponse
	genInfo := llm.buildGenerationInfo(handler, finalResp, "test-model", 100*time.Millisecond)

	assert.NotNil(t, genInfo["Logprobs"])
	lps, ok := genInfo["Logprobs"].([]api.Logprob)
	assert.True(t, ok)
	assert.Len(t, lps, 1)
	assert.Equal(t, "hello", lps[0].Token)
	assert.Equal(t, -0.0123, lps[0].Logprob)
}

func TestParseKeepAlive_Indefinite(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"zero value", "0", 0},
		{"indefinite duration", "-1", -1},
		{"standard duration", "10m", 10 * time.Minute},
		{"invalid fallback", "invalid", 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKeepAlive(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyFormatOption_JSONSchemaTypes(t *testing.T) {
	llm := &LLM{
		options: applyOptions(),
		logger:  slog.Default(),
	}

	t.Run("raw json string", func(t *testing.T) {
		req := &api.ChatRequest{}
		opts := llms.CallOptions{
			JSONSchema: `{"type": "object"}`,
		}
		llm.applyFormatOption(req, opts)
		assert.Equal(t, []byte(`{"type": "object"}`), []byte(req.Format))
	})

	t.Run("raw json bytes", func(t *testing.T) {
		req := &api.ChatRequest{}
		opts := llms.CallOptions{
			JSONSchema: []byte(`{"type": "array"}`),
		}
		llm.applyFormatOption(req, opts)
		assert.Equal(t, []byte(`{"type": "array"}`), []byte(req.Format))
	})

	t.Run("structured map", func(t *testing.T) {
		req := &api.ChatRequest{}
		opts := llms.CallOptions{
			JSONSchema: map[string]any{"type": "string"},
		}
		llm.applyFormatOption(req, opts)
		assert.Equal(t, []byte(`{"type":"string"}`), []byte(req.Format))
	})
}

func TestBuildChatMessages_RobustImageDecoding(t *testing.T) {
	llm := &LLM{
		options: applyOptions(),
		logger:  slog.Default(),
	}

	t.Run("with data url prefix", func(t *testing.T) {
		messages := []schema.MessageContent{
			{
				Role: schema.ChatMessageTypeHuman,
				Parts: []schema.ContentPart{
					schema.ImageContent{
						Data: "data:image/png;base64,SGVsbG8=", // "Hello" base64
					},
				},
			},
		}
		msgs := llm.buildChatMessages(messages)
		assert.Len(t, msgs, 1)
		assert.Len(t, msgs[0].Images, 1)
		assert.Equal(t, []byte("Hello"), []byte(msgs[0].Images[0]))
	})

	t.Run("unpadded base64", func(t *testing.T) {
		messages := []schema.MessageContent{
			{
				Role: schema.ChatMessageTypeHuman,
				Parts: []schema.ContentPart{
					schema.ImageContent{
						Data: "SGVsbG8", // unpadded "Hello"
					},
				},
			},
		}
		msgs := llm.buildChatMessages(messages)
		assert.Len(t, msgs, 1)
		assert.Len(t, msgs[0].Images, 1)
		assert.Equal(t, []byte("Hello"), []byte(msgs[0].Images[0]))
	})
}
