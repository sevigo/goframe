package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestToolContextCancellation demonstrates the pattern for implementing
// context-aware tools. Tools should check ctx.Done() and return early
// when cancelled to prevent resource leaks and enable graceful shutdown.
func TestToolContextCancellation(t *testing.T) {
	registry := NewRegistry()

	// Create a tool that properly handles context cancellation
	slowTool := &mockTool{
		name:        "slow_operation",
		description: "A tool that takes a long time",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			// Simulate a long-running operation
			select {
			case <-time.After(5 * time.Second):
				return "completed", nil
			case <-ctx.Done():
				// IMPORTANT: Tools must check context cancellation
				// and return early when cancelled
				return nil, ctx.Err()
			}
		},
	}
	_ = registry.Register(slowTool)

	// Create a context that we'll cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before execution

	result, err := registry.Execute(ctx, "slow_operation", nil)
	if err == nil {
		t.Error("expected context cancelled error")
	}

	if result != nil {
		t.Error("expected nil result on cancellation")
	}
}

// TestToolContextDeadline shows using context deadline for timeouts.
func TestToolContextDeadline(t *testing.T) {
	registry := NewRegistry()

	timeoutTool := &mockTool{
		name:        "timeout_operation",
		description: "A tool with a deadline",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			// Check deadline before starting work
			deadline, ok := ctx.Deadline()
			if !ok {
				return nil, errors.New("no deadline set")
			}
			return deadline.Format(time.RFC3339), nil
		},
	}
	_ = registry.Register(timeoutTool)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	result, err := registry.Execute(ctx, "timeout_operation", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result == nil {
		t.Error("expected deadline result")
	}
}

// TestAgentLoop_TraceID tests that trace IDs are generated and logged.
func TestAgentLoop_TraceID(t *testing.T) {
	registry := NewRegistry()
	model := &mockLLM{
		responses: []string{"Done."},
	}

	// Custom trace ID generator
	customID := "custom-trace-123"
	loop, err := NewAgentLoop(model, registry,
		WithLoopMaxIterations(5),
		WithLoopTraceID(func() string { return customID }),
	)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Test trace ID",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.TraceID != customID {
		t.Errorf("expected trace ID %q, got %q", customID, result.TraceID)
	}
}

// TestAgentLoop_DefaultTraceID tests that trace IDs are auto-generated.
func TestAgentLoop_DefaultTraceID(t *testing.T) {
	registry := NewRegistry()
	model := &mockLLM{
		responses: []string{"Done."},
	}

	loop, err := NewAgentLoop(model, registry)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Test default trace ID",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.TraceID == "" {
		t.Error("expected auto-generated trace ID")
	}
}
