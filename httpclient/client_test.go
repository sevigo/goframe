package httpclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	assert.Equal(t, DefaultTimeout, cfg.Timeout)
	assert.Equal(t, DefaultMaxIdleConns, cfg.MaxIdleConns)
	assert.Equal(t, DefaultMaxIdleConnsPerHost, cfg.MaxIdleConnsPerHost)
	assert.Equal(t, DefaultIdleConnTimeout, cfg.IdleConnTimeout)
	assert.Equal(t, DefaultTLSHandshakeTimeout, cfg.TLSHandshakeTimeout)
	assert.Equal(t, DefaultRetryAttempts, cfg.RetryAttempts)
	assert.Equal(t, DefaultRetryDelay, cfg.RetryDelay)
	assert.Equal(t, DefaultMaxRetryDelay, cfg.MaxRetryDelay)
	assert.Equal(t, DefaultRetryJitter, cfg.RetryJitter)
}

func TestConfigWithOptions(t *testing.T) {
	cfg := NewConfig(
		WithTimeout(60*time.Second),
		WithMaxIdleConns(50),
		WithMaxIdleConnsPerHost(10),
		WithIdleConnTimeout(20*time.Second),
		WithTLSHandshakeTimeout(5*time.Second),
		WithRetryAttempts(5),
		WithRetryDelay(1*time.Second),
		WithMaxRetryDelay(60*time.Second),
		WithRetryJitter(500*time.Millisecond),
	)

	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Equal(t, 50, cfg.MaxIdleConns)
	assert.Equal(t, 10, cfg.MaxIdleConnsPerHost)
	assert.Equal(t, 20*time.Second, cfg.IdleConnTimeout)
	assert.Equal(t, 5*time.Second, cfg.TLSHandshakeTimeout)
	assert.Equal(t, 5, cfg.RetryAttempts)
	assert.Equal(t, 1*time.Second, cfg.RetryDelay)
	assert.Equal(t, 60*time.Second, cfg.MaxRetryDelay)
	assert.Equal(t, 500*time.Millisecond, cfg.RetryJitter)
}

func TestNewClient(t *testing.T) {
	cfg := NewConfig(WithTimeout(30 * time.Second))
	client := NewClient(cfg)

	assert.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)
	assert.NotNil(t, client.Transport)

	transport, ok := client.Transport.(*http.Transport)
	assert.True(t, ok)
	assert.Equal(t, DefaultMaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, DefaultMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
}

func TestNewDefaultClient(t *testing.T) {
	client := NewDefaultClient()

	assert.NotNil(t, client)
	assert.Equal(t, DefaultTimeout, client.Timeout)
}

func TestDefaultClient(t *testing.T) {
	assert.NotNil(t, DefaultClient)
	assert.Equal(t, DefaultTimeout, DefaultClient.Timeout)
}

func TestDownloadClient(t *testing.T) {
	client := DownloadClient()

	assert.NotNil(t, client)
	assert.Equal(t, 10*time.Minute, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	assert.True(t, ok)
	assert.Equal(t, 10, transport.MaxIdleConns)
	assert.Equal(t, 5, transport.MaxIdleConnsPerHost)
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"unexpected EOF", errors.New("unexpected EOF"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"http2 GOAWAY", errors.New("http2: server sent GOAWAY"), true},
		{"error 500", errors.New("Error 500: internal server error"), true},
		{"error 503", errors.New("Error 503: service unavailable"), true},
		{"error 429", errors.New("Error 429: rate limit"), true},
		{"nil error", nil, false},
		{"non-retryable error", errors.New("invalid model name"), false},
		{"bad request", errors.New("bad request: invalid parameter"), false},
		{"not found", errors.New("collection not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDoWithRetryNoRetry(t *testing.T) {
	cfg := &RetryConfig{Attempts: 0}
	callCount := 0

	err := DoWithRetry(context.Background(), cfg, "test_op", func() error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDoWithRetrySuccess(t *testing.T) {
	cfg := &RetryConfig{
		Attempts: 3,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
		Jitter:   5 * time.Millisecond,
	}
	callCount := 0

	err := DoWithRetry(context.Background(), cfg, "test_op", func() error {
		callCount++
		if callCount < 2 {
			return errors.New("connection refused")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestDoWithRetryNonRetryableError(t *testing.T) {
	cfg := &RetryConfig{
		Attempts: 3,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
	}
	callCount := 0

	err := DoWithRetry(context.Background(), cfg, "test_op", func() error {
		callCount++
		return errors.New("invalid parameter")
	})

	require.Error(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDoWithRetryExhausted(t *testing.T) {
	cfg := &RetryConfig{
		Attempts: 2,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
	}
	callCount := 0

	err := DoWithRetry(context.Background(), cfg, "test_op", func() error {
		callCount++
		return errors.New("connection refused")
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "test_op failed after 3 attempts")
	assert.Equal(t, 3, callCount) // initial + 2 retries
}

func TestDoWithRetryContextCancellation(t *testing.T) {
	cfg := &RetryConfig{
		Attempts: 10,
		Delay:    5 * time.Second,
		MaxDelay: 30 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	callCount := 0
	err := DoWithRetry(ctx, cfg, "test_op", func() error {
		callCount++
		return errors.New("connection refused")
	})

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, callCount)
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, DefaultRetryAttempts, cfg.Attempts)
	assert.Equal(t, DefaultRetryDelay, cfg.Delay)
	assert.Equal(t, DefaultMaxRetryDelay, cfg.MaxDelay)
	assert.Equal(t, DefaultRetryJitter, cfg.Jitter)
}
