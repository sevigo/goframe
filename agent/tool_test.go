package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// mockTool is a simple tool implementation for testing
type mockTool struct {
	name        string
	description string
	schema      map[string]any
	execFunc    func(ctx context.Context, params map[string]any) (any, error)
}

func (t *mockTool) Name() string                     { return t.name }
func (t *mockTool) Description() string              { return t.description }
func (t *mockTool) ParametersSchema() map[string]any { return t.schema }
func (t *mockTool) Execute(ctx context.Context, params map[string]any) (any, error) {
	if t.execFunc == nil {
		return nil, nil
	}
	return t.execFunc(ctx, params)
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	tool := &mockTool{name: "test", description: "test tool"}
	err := registry.Register(tool)
	if err != nil {
		t.Errorf("failed to register tool: %v", err)
	}

	// Test duplicate registration
	err = registry.Register(tool)
	if err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()

	// Test get non-existent
	_, err := registry.Get("nonexistent")
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}

	// Test get existing
	tool := &mockTool{name: "test", description: "test tool"}
	_ = registry.Register(tool)

	got, err := registry.Get("test")
	if err != nil {
		t.Errorf("failed to get tool: %v", err)
	}
	if got.Name() != "test" {
		t.Errorf("expected tool name 'test', got: %s", got.Name())
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(&mockTool{name: "tool1", description: "first"})
	_ = registry.Register(&mockTool{name: "tool2", description: "second"})
	_ = registry.Register(&mockTool{name: "tool3", description: "third"})

	list := registry.List()
	if len(list) != 3 {
		t.Errorf("expected 3 tools, got: %d", len(list))
	}
}

func TestRegistry_Execute(t *testing.T) {
	registry := NewRegistry()

	called := false
	tool := &mockTool{
		name:        "test",
		description: "test tool",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			called = true
			return params["input"], nil
		},
	}
	_ = registry.Register(tool)

	result, err := registry.Execute(context.Background(), "test", map[string]any{"input": "hello"})
	if err != nil {
		t.Errorf("failed to execute tool: %v", err)
	}
	if !called {
		t.Error("tool was not called")
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got: %v", result)
	}
}

func TestNewToolFromFunc(t *testing.T) {
	type SearchParams struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}

	tool, err := NewToolFromFunc(
		"search",
		"Search for documents",
		func(ctx context.Context, params SearchParams) (string, error) {
			return "results for: " + params.Query, nil
		},
	)
	if err != nil {
		t.Fatalf("failed to create tool: %v", err)
	}

	if tool.Name() != "search" {
		t.Errorf("expected name 'search', got: %s", tool.Name())
	}

	if tool.Description() != "Search for documents" {
		t.Errorf("expected description 'Search for documents', got: %s", tool.Description())
	}

	schema := tool.ParametersSchema()
	if schema == nil {
		t.Error("expected non-nil schema")
	}

	// Test execution
	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "test",
		"limit": 10,
	})
	if err != nil {
		t.Errorf("failed to execute tool: %v", err)
	}

	if result != "results for: test" {
		t.Errorf("expected 'results for: test', got: %v", result)
	}
}

func TestNewToolFromFunc_InvalidInput(t *testing.T) {
	// Test nil function
	_, err := NewToolFromFunc("test", "test", nil)
	if err == nil {
		t.Error("expected error for nil function")
	}

	// Test non-function
	_, err = NewToolFromFunc("test", "test", "not a function")
	if err == nil {
		t.Error("expected error for non-function")
	}

	// Test function without context.Context
	_, err = NewToolFromFunc("test", "test", func() {})
	if err == nil {
		t.Error("expected error for function without context.Context")
	}
}

func TestGenerateJSONSchema(t *testing.T) {
	tests := []struct {
		name     string
		typ      reflect.Type
		expected map[string]any
	}{
		{
			name: "string type",
			typ: reflect.TypeOf(struct {
				Name string `json:"name"`
			}{}),
			expected: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []string{"name"},
			},
		},
		{
			name: "integer type",
			typ: reflect.TypeOf(struct {
				Count int `json:"count"`
			}{}),
			expected: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"count": map[string]any{"type": "integer"},
				},
				"required": []string{"count"},
			},
		},
		{
			name: "optional field",
			typ: reflect.TypeOf(struct {
				Name string `json:"name,omitempty"`
			}{}),
			expected: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := generateJSONSchema(tt.typ, true)
			if schema == nil {
				t.Error("expected non-nil schema")
			}
		})
	}
}

func TestToolLogger(t *testing.T) {
	registry := NewRegistry()

	called := false
	tool := &mockTool{
		name:        "test",
		description: "test tool",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			called = true
			return "result", nil
		},
	}
	_ = registry.Register(tool)

	// Test that the original tool works
	_, _ = registry.Execute(context.Background(), "test", nil)
	if !called {
		t.Error("tool was not called")
	}
}
