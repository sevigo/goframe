package agent

import (
	"errors"
	"fmt"
)

// Error types for the agent package
var (
	// ErrNilClient is returned when a nil OpenCode client is provided.
	ErrNilClient = errors.New("agent: client cannot be nil")
	// ErrNoModel is returned when no model is specified.
	ErrNoModel = errors.New("agent: no model specified")
	// ErrInvalidModel is returned when the model format is invalid.
	ErrInvalidModel = errors.New("agent: invalid model format, expected provider/model")
	// ErrSessionNotFound is returned when a session cannot be found.
	ErrSessionNotFound = errors.New("agent: session not found")
	// ErrSessionAborted is returned when a session is aborted.
	ErrSessionAborted = errors.New("agent: session was aborted")
	// ErrNoMCPServers is returned when no MCP servers are configured.
	ErrNoMCPServers = errors.New("agent: no MCP servers configured")
	// ErrMCPServerExists is returned when trying to add a duplicate MCP server.
	ErrMCPServerExists = errors.New("agent: MCP server already exists")
	// ErrMCPServerNotFound is returned when an MCP server cannot be found.
	ErrMCPServerNotFound = errors.New("agent: MCP server not found")
	// ErrInvalidMCPConfig is returned when MCP server configuration is invalid.
	ErrInvalidMCPConfig = errors.New("agent: invalid MCP server configuration")
	// ErrPermissionDenied is returned when an action is denied by permissions.
	ErrPermissionDenied = errors.New("agent: permission denied")
	// ErrPromptFailed is returned when a prompt operation fails.
	ErrPromptFailed = errors.New("agent: prompt failed")
	// ErrStreamFailed is returned when a stream operation fails.
	ErrStreamFailed = errors.New("agent: stream failed")
	// ErrToolNotFound is returned when a tool cannot be found in the registry.
	ErrToolNotFound = errors.New("agent: tool not found")
	// ErrReviewRejected is returned when a review is rejected.
	ErrReviewRejected = errors.New("agent: review rejected")
	// ErrMaxRetries is returned when the maximum retry count is exceeded.
	ErrMaxRetries = errors.New("agent: max retries exceeded")
	// ErrReviewFailed is returned when a review operation fails.
	ErrReviewFailed = errors.New("agent: review failed")
	// ErrExecutionFailed is returned when an execution operation fails.
	ErrExecutionFailed = errors.New("agent: execution failed")
)

// AgentError represents an error from agent operations
type AgentError struct {
	// Op is the operation that failed
	Op string
	// Session is the session ID if applicable
	Session string
	// Err is the underlying error
	Err error
	// Details contains additional error context
	Details map[string]interface{}
}

// Error implements the error interface
func (e *AgentError) Error() string {
	if e.Session != "" {
		return fmt.Sprintf("agent %s [session=%s]: %v", e.Op, e.Session, e.Err)
	}
	return fmt.Sprintf("agent %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *AgentError) Unwrap() error {
	return e.Err
}

func newError(op string, err error) *AgentError {
	return &AgentError{Op: op, Err: err}
}

func newSessionError(op, session string, err error) *AgentError {
	return &AgentError{Op: op, Session: session, Err: err}
}

// MCPError represents an error from MCP server operations
type MCPError struct {
	// Server is the name of the MCP server
	Server string
	// Err is the underlying error
	Err error
	// Details contains additional error context
	Details map[string]interface{}
}

// Error implements the error interface
func (e *MCPError) Error() string {
	return fmt.Sprintf("MCP server %q: %v", e.Server, e.Err)
}

// Unwrap returns the underlying error
func (e *MCPError) Unwrap() error {
	return e.Err
}

func newMCPError(server string, err error) *MCPError {
	return &MCPError{Server: server, Err: err}
}
