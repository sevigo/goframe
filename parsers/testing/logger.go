package testing

import (
	"bytes"
	"log/slog"
	"testing"
)

// NewTestLogger creates a slog.Logger that captures output to a buffer for test assertions.
func NewTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler)
	return logger, &buf
}
