package agent

import (
	"errors"
	"fmt"
)

// Error types for the agent package
var (
	// ErrNoModel is returned when no model is specified.
	ErrNoModel = errors.New("agent: no model specified")
	// ErrInvalidModel is returned when the model format is invalid.
	ErrInvalidModel = errors.New("agent: invalid model format, expected provider/model")
	// ErrToolNotFound is returned when a tool cannot be found in the registry.
	ErrToolNotFound = errors.New("agent: tool not found")
	// ErrMaxRetries is returned when the maximum retry count is exceeded.
	ErrMaxRetries = errors.New("agent: max retries exceeded")
	// ErrExecutionFailed is returned when an execution operation fails.
	ErrExecutionFailed = errors.New("agent: execution failed")
)

// AgentError represents an error from agent operations
type AgentError struct {
	// Op is the operation that failed
	Op string
	// Err is the underlying error
	Err error
	// Details contains additional error context
	Details map[string]any
}

// Error implements the error interface
func (e *AgentError) Error() string {
	return fmt.Sprintf("agent %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *AgentError) Unwrap() error {
	return e.Err
}
