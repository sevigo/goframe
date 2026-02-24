package httpclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"strings"
	"time"
)

// RetryableErrorPatterns contains error patterns that indicate a transient failure.
var RetryableErrorPatterns = []string{
	"context deadline exceeded",
	"context canceled",
	"http2: server sent GOAWAY",
	"connection reset by peer",
	"connection refused",
	"unexpected EOF",
	"io: read/write on closed pipe",
	"network is unreachable",
	"no such host",
	"timeout",
	"ECONNRESET",
	"ECONNREFUSED",
	"ETIMEDOUT",
	"Error 500",
	"Error 503",
	"Error 429",
}

// IsRetryableError determines if an error is transient and should be retried.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	for _, pattern := range RetryableErrorPatterns {
		if strings.Contains(errStr, strings.ToLower(pattern)) {
			return true
		}
	}

	// Check for net.OpError with temporary errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	return false
}

// RetryConfig holds configuration for retry behavior.
type RetryConfig struct {
	// Attempts is the number of retry attempts (not including the initial attempt).
	Attempts int

	// Delay is the initial delay between retry attempts.
	Delay time.Duration

	// MaxDelay is the maximum delay between retry attempts.
	MaxDelay time.Duration

	// Jitter is the random jitter added to retry delays.
	Jitter time.Duration
}

// DefaultRetryConfig returns a RetryConfig with default values.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		Attempts: DefaultRetryAttempts,
		Delay:    DefaultRetryDelay,
		MaxDelay: DefaultMaxRetryDelay,
		Jitter:   DefaultRetryJitter,
	}
}

// DoWithRetry executes a function with retry logic for transient errors.
// The operation function should return an error if it fails.
// If the error is retryable, the function will be retried with exponential backoff.
func DoWithRetry(ctx context.Context, cfg *RetryConfig, operation string, fn func() error) error {
	if cfg == nil {
		cfg = DefaultRetryConfig()
	}

	if cfg.Attempts == 0 {
		return fn()
	}

	var lastErr error
	delay := cfg.Delay

	for attempt := 0; attempt <= cfg.Attempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we've exhausted our retries
		if attempt >= cfg.Attempts {
			break
		}

		// Only retry on transient errors
		if !IsRetryableError(err) {
			break
		}

		// Calculate delay with jitter
		jitter := time.Duration(0)
		if cfg.Jitter > 0 {
			//nolint:gosec // rand.IntN is sufficient for retry jitter calculation
			jitter = time.Duration(rand.IntN(int(cfg.Jitter.Milliseconds()))) * time.Millisecond
		}
		totalDelay := delay + jitter

		// Wait before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(totalDelay):
		}

		// Exponential backoff
		delay = calculateNextDelay(delay, cfg.MaxDelay)
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, cfg.Attempts+1, lastErr)
}

// calculateNextDelay calculates the next delay with exponential backoff.
func calculateNextDelay(delay, maxDelay time.Duration) time.Duration {
	delay *= 2
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
