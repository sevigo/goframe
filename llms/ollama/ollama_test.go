package ollama

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

	// Zero delay should be ignored
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
	llm := &LLM{options: applyOptions()}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"timeout error", errors.New("context deadline exceeded"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"unexpected EOF", errors.New("unexpected EOF"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"http2 GOAWAY", errors.New("http2: server sent GOAWAY"), true},
		{"nil error", nil, false},
		{"non-retryable error", errors.New("invalid model name"), false},
		{"bad request", errors.New("bad request: invalid parameter"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := llm.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateNextDelay(t *testing.T) {
	llm := &LLM{options: applyOptions()}

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
	cancel() // Cancel immediately

	callCount := 0
	err := llm.doWithRetry(ctx, func() error {
		callCount++
		return errors.New("connection refused")
	})

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, callCount, "should stop after context cancellation")
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

	// Nil client should be ignored
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
	// Create a mock server URL that won't actually connect
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
