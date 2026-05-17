package gemini

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/httpclient"
)

func TestApplyOptionsDefaults(t *testing.T) {
	opts := applyOptions()

	assert.Equal(t, "gemini-2.5-flash", opts.model)
	assert.Equal(t, "gemini-embedding-001", opts.embeddingModel)
	assert.Equal(t, DefaultRetryAttempts, opts.retry.Attempts)
	assert.Equal(t, DefaultRetryDelay, opts.retry.Delay)
	assert.Equal(t, DefaultMaxRetryDelay, opts.retry.MaxDelay)
	assert.Equal(t, DefaultRetryJitter, opts.retry.Jitter)
	assert.Equal(t, DefaultTimeout, opts.requestTimeout)
}

func TestWithModel(t *testing.T) {
	opts := applyOptions(WithModel("gemini-2.5-pro"))
	assert.Equal(t, "gemini-2.5-pro", opts.model)
}

func TestWithEmbeddingModel(t *testing.T) {
	opts := applyOptions(WithEmbeddingModel("text-embedding-004"))
	assert.Equal(t, "text-embedding-004", opts.embeddingModel)
}

func TestWithAPIKey(t *testing.T) {
	opts := applyOptions(WithAPIKey("test-key"))
	assert.Equal(t, "test-key", opts.apiKey)
}

func TestWithLogger(t *testing.T) {
	opts := applyOptions(WithLogger(nil))
	assert.NotNil(t, opts.logger)
}

func TestWithHTTPClient(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	opts := applyOptions(WithHTTPClient(client))
	assert.Equal(t, client, opts.httpClient)

	opts = applyOptions(WithHTTPClient(nil))
	assert.Nil(t, opts.httpClient)
}

func TestWithRequestTimeout(t *testing.T) {
	opts := applyOptions(WithRequestTimeout(30 * time.Second))
	assert.Equal(t, 30*time.Second, opts.requestTimeout)

	opts = applyOptions(WithRequestTimeout(0))
	assert.Equal(t, DefaultTimeout, opts.requestTimeout)
}

func TestWithRetryAttempts(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		expected int
	}{
		{"valid attempts", 5, 5},
		{"zero attempts", 0, 0},
		{"negative should be ignored", -1, DefaultRetryAttempts},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := applyOptions(WithRetryAttempts(tt.attempts))
			assert.Equal(t, tt.expected, opts.retry.Attempts)
		})
	}
}

func TestWithRetryDelay(t *testing.T) {
	opts := applyOptions(WithRetryDelay(5 * time.Second))
	assert.Equal(t, 5*time.Second, opts.retry.Delay)

	opts = applyOptions(WithRetryDelay(0))
	assert.Equal(t, DefaultRetryDelay, opts.retry.Delay)
}

func TestWithMaxRetryDelay(t *testing.T) {
	opts := applyOptions(WithMaxRetryDelay(60 * time.Second))
	assert.Equal(t, 60*time.Second, opts.retry.MaxDelay)

	opts = applyOptions(WithMaxRetryDelay(0))
	assert.Equal(t, DefaultMaxRetryDelay, opts.retry.MaxDelay)
}

func TestWithRetryJitter(t *testing.T) {
	opts := applyOptions(WithRetryJitter(2 * time.Second))
	assert.Equal(t, 2*time.Second, opts.retry.Jitter)

	opts = applyOptions(WithRetryJitter(0))
	assert.Equal(t, time.Duration(0), opts.retry.Jitter)
}

func TestNewNoAPIKey(t *testing.T) {
	_, err := New(context.Background(), WithAPIKey(""))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoAPIKey)
}

func TestNewInvalidModel(t *testing.T) {
	_, err := New(context.Background(), WithAPIKey("test-key"), WithModel(""))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidModel)
}

func TestIsRetryableError(t *testing.T) {
	g := &LLM{options: applyOptions()}

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"API_KEY_INVALID", errors.New("API_KEY_INVALID: key is invalid"), false},
		{"PERMISSION_DENIED", errors.New("PERMISSION_DENIED: access denied"), false},
		{"INVALID_ARGUMENT", errors.New("INVALID_ARGUMENT: bad request"), false},
		{"QUOTA_EXCEEDED", errors.New("QUOTA_EXCEEDED: over limit"), false},
		{"RESOURCE_EXHAUSTED", errors.New("RESOURCE_EXHAUSTED: rate limited"), true},
		{"429 rate limit", errors.New("Error 429: rate limit exceeded"), true},
		{"500 server error", errors.New("Error 500: internal server error"), true},
		{"503 unavailable", errors.New("Error 503: service unavailable"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"timeout", errors.New("timeout waiting for response"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, g.isRetryableError(tt.err))
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"standard key", "AIzaSyB1234567890abcdef", "AIza****cdef"},
		{"short key", "short", "****"},
		{"8 char key", "12345678", "****"},
		{"9 char key", "123456789", "1234****6789"},
		{"empty key", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, maskAPIKey(tt.key))
		})
	}
}

func TestNewOptimizedHTTPClient(t *testing.T) {
	client := newOptimizedHTTPClient(30 * time.Second)
	require.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "expected *http.Transport")
	assert.Equal(t, httpclient.DefaultMaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, httpclient.DefaultMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
}

func TestClose(t *testing.T) {
	g := &LLM{
		httpClient: newOptimizedHTTPClient(DefaultTimeout),
		ownsClient: true,
	}
	assert.NoError(t, g.Close())

	g = &LLM{
		httpClient: &http.Client{},
		ownsClient: false,
	}
	assert.NoError(t, g.Close())
}

func TestSentinelErrors(t *testing.T) {
	sentinels := []error{ErrNoAPIKey, ErrInvalidModel, ErrNoContent, ErrSystemMessage, ErrEmbeddings, ErrNoMessages}
	for _, err := range sentinels {
		assert.Error(t, err)
	}
}
