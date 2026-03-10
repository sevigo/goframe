# GoFrame Agent Development Guide

This document provides essential information for coding agents working on the GoFrame codebase.

## Build, Lint, and Test Commands

```bash
# Run all tests
make test

# Run tests with race detector
make test-race

# Run a single test
go test ./agent/... -v -run TestRegistry

# Run tests for specific package
go test ./vectorstores/qdrant/... -v

# Run linter
make lint

# Run linter with auto-fix
make lint-fix

# Pre-push checks (lint + test)
make pre-push
```

## Code Style Guidelines

### Import Organization

Imports are organized in three groups separated by blank lines:

```go
import (
	// Standard library
	"context"
	"fmt"
	
	// External packages
	"github.com/some/external/pkg"
	
	// Internal packages
	"github.com/sevigo/goframe/llms"
)
```

Use `goimports` for automatic formatting.

### Package Comments

Every package must have a doc comment:

```go
// Package agent provides an abstraction layer.
//
// The agent package enables programmatic control of AI agents.
package agent
```

### Naming Conventions

- **Types**: PascalCase (`AgentLoop`, `Registry`)
- **Interfaces**: noun or `-er` suffix (`Tool`, `RiskAssessor`)
- **Functions**: camelCase (`NewRegistry`, `WithLoopGovernance`)
- **Errors**: `ErrXxx` sentinel errors (`ErrToolNotFound`)
- **Options**: `WithXxx` prefix (`WithAPIKey`)

### Error Handling

Use `%w` for error wrapping (enforced by errorlint):

```go
if err != nil {
	return fmt.Errorf("operation failed: %w", err)
}
```

Define sentinel errors:

```go
var (
	ErrToolNotFound = errors.New("agent: tool not found")
	ErrToolExecution = errors.New("agent: tool execution failed")
)
```

### Context Cancellation

Always check context in long-running operations:

```go
func (t *Tool) Execute(ctx context.Context, params map[string]any) (any, error) {
	select {
	case <-time.After(5 * time.Second):
		return "completed", nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
```

### Thread Safety

Use `sync.RWMutex` for concurrent access:

```go
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name], nil
}
```

## Testing Guidelines

### Test Structure

```go
func TestRegistry_Execute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Arrange
		registry := NewRegistry()
		tool := &mockTool{name: "test"}
		_ = registry.Register(tool)
		
		// Act
		result, err := registry.Execute(context.Background(), "test", nil)
		
		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
```

### Table-Driven Tests

```go
func TestRiskLevel(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		wantRisk RiskLevel
	}{
		{"low", "read", RiskLow},
		{"high", "delete", RiskHigh},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// test implementation
		})
	}
}
```

## Linter Rules

Key rules enforced by golangci-lint:

- **Error wrapping**: Use `%w`, not `%v`
- **Named returns**: Not allowed
- **Function length**: Max 150 lines, 60 statements
- **Cyclomatic complexity**: Max 30
- **Unused code**: All unused code fails lint
- **Imports**: Use `goimports`

## Logging

Use `log/slog` for structured logging:

```go
logger := slog.Default()
logger.Debug("executing tool", "name", toolName, "params", params)
logger.Info("loop completed", "iterations", result.Iterations)
logger.Warn("governance blocked", "tool", toolName, "error", err)
logger.Error("tool execution failed", "tool", toolName, "error", err)
```

## Common Patterns

### Constructor with Options

```go
func NewAgentLoop(model llms.Model, registry *Registry, opts ...Option) (*AgentLoop, error) {
	loop := &AgentLoop{
		model:         model,
		registry:      registry,
		maxIterations: 10,
	}
	for _, opt := range opts {
		opt(loop)
	}
	return loop, nil
}
```

### Functional Options

```go
type Option func(*Config)

func WithAPIKey(apiKey string) Option {
	return func(c *Config) {
		c.apiKey = apiKey
	}
}
```

### Error Wrapping

```go
result, err := tool.Execute(ctx, params)
if err != nil {
	return nil, fmt.Errorf("%w: %w", ErrToolExecution, err)
}
```

## Pre-commit Checklist

1. Run `make lint-fix` to auto-fix formatting
2. Run `make test` to ensure all tests pass
3. Check exported types have godoc comments
4. Verify error wrapping uses `%w`
5. Ensure context cancellation in long-running operations
6. Add tests for new functionality
7. Update package documentation if adding new types

## Important Files

1. `doc.go` - Package documentation
2. `errors.go` - Sentinel error definitions  
3. Interface definitions in main files
4. Test files for expected behavior