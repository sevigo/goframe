package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// mockLLM is a mock implementation for testing
type mockLLM struct {
	responses []string
	callCount int
	toolCalls [][]llms.ToolCall
	index     int
}

func (m *mockLLM) GenerateContent(ctx context.Context, messages []schema.MessageContent, options ...llms.CallOption) (*schema.ContentResponse, error) {
	if len(m.responses) == 0 {
		return nil, errors.New("no responses configured")
	}

	resp := &schema.ContentResponse{
		Choices: []*schema.ContentChoice{
			{Content: m.responses[m.index]},
		},
	}

	// Add tool calls if configured
	if m.index < len(m.toolCalls) && len(m.toolCalls[m.index]) > 0 {
		resp.Choices[0].GenerationInfo = map[string]any{
			"tool_calls": m.toolCalls[m.index],
		}
	}

	m.index = (m.index + 1) % len(m.responses)
	m.callCount++

	return resp, nil
}

func (m *mockLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	resp, err := m.GenerateContent(ctx, []schema.MessageContent{
		schema.NewHumanMessage(prompt),
	}, options...)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Content, nil
}

func TestNewAgentLoop(t *testing.T) {
	registry := NewRegistry()

	// Test missing LLM
	_, err := NewAgentLoop(nil, registry)
	if !errors.Is(err, ErrNoLLM) {
		t.Errorf("expected ErrNoLLM, got: %v", err)
	}

	// Test missing registry
	model := &mockLLM{}
	_, err = NewAgentLoop(model, nil)
	if !errors.Is(err, ErrNoRegistry) {
		t.Errorf("expected ErrNoRegistry, got: %v", err)
	}

	// Test successful creation
	loop, err := NewAgentLoop(model, registry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if loop == nil {
		t.Error("expected loop, got nil")
	}
}

func TestAgentLoop_Run_NoToolCalls(t *testing.T) {
	registry := NewRegistry()
	model := &mockLLM{
		responses: []string{"The answer is 42."},
	}

	loop, err := NewAgentLoop(model, registry, WithLoopMaxIterations(5))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "What is the answer to life, the universe, and everything?",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.State != StateComplete {
		t.Errorf("expected StateComplete, got: %s", result.State)
	}

	if result.Response != "The answer is 42." {
		t.Errorf("unexpected response: %s", result.Response)
	}

	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got: %d", result.Iterations)
	}
}

func TestAgentLoop_Run_WithToolCalls(t *testing.T) {
	registry := NewRegistry()

	// Register a mock tool
	called := false
	_ = registry.Register(&mockTool{
		name:        "search",
		description: "Search for information",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			called = true
			return map[string]any{"result": "found"}, nil
		},
	})

	model := &mockLLM{
		responses: []string{
			"", // First call triggers tool
			"Based on the search results, the answer is found.",
		},
		toolCalls: [][]llms.ToolCall{
			{
				{
					Function: llms.FunctionCall{
						Name:      "search",
						Arguments: map[string]any{"query": "test"},
					},
				},
			},
			{}, // Second call has no tool calls
		},
	}

	loop, err := NewAgentLoop(model, registry, WithLoopMaxIterations(5))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Search for information",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called {
		t.Error("tool was not called")
	}

	if result.State != StateComplete {
		t.Errorf("expected StateComplete, got: %s", result.State)
	}

	if len(result.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got: %d", len(result.ToolCalls))
	}

	if result.ToolCalls[0].Name != "search" {
		t.Errorf("expected tool name 'search', got: %s", result.ToolCalls[0].Name)
	}
}

func TestAgentLoop_Run_GovernanceDenied(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(&mockTool{
		name:        "delete",
		description: "Delete files",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			return "deleted", nil
		},
	})

	model := &mockLLM{
		responses: []string{
			"", // First call triggers tool
			"I cannot delete files as it's not allowed.",
		},
		toolCalls: [][]llms.ToolCall{
			{
				{
					Function: llms.FunctionCall{
						Name:      "delete",
						Arguments: map[string]any{"path": "/important"},
					},
				},
			},
			{},
		},
	}

	governance := NewGovernance(
		NewPermissionCheck().Deny("delete"),
	)

	loop, err := NewAgentLoop(model, registry,
		WithLoopMaxIterations(5),
		WithLoopGovernance(governance),
	)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Delete some files",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Tool should have error from governance
	if len(result.ToolCalls) == 0 {
		t.Error("expected tool call record")
	} else if result.ToolCalls[0].Error == nil {
		t.Error("expected governance error")
	}
}

func TestAgentLoop_Run_MaxIterations(t *testing.T) {
	registry := NewRegistry()

	model := &mockLLM{
		responses: []string{""},
		toolCalls: [][]llms.ToolCall{
			{
				{
					Function: llms.FunctionCall{
						Name:      "loop",
						Arguments: map[string]any{},
					},
				},
			},
		},
	}

	// Register loop tool
	_ = registry.Register(&mockTool{
		name:        "loop",
		description: "Loop forever",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			return "looping", nil
		},
	})

	loop, err := NewAgentLoop(model, registry, WithLoopMaxIterations(2))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Run forever",
	}

	_, err = loop.Run(context.Background(), task, nil)
	if !errors.Is(err, ErrMaxIterations) {
		t.Errorf("expected ErrMaxIterations, got: %v", err)
	}
}

func TestAgentLoop_Run_ContextCancellation(t *testing.T) {
	registry := NewRegistry()

	model := &mockLLM{
		responses: []string{""},
	}

	loop, err := NewAgentLoop(model, registry, WithLoopMaxIterations(100))
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	task := Task{
		Description: "Test cancellation",
	}

	_, err = loop.Run(ctx, task, nil)
	if !errors.Is(err, ErrLoopCancelled) {
		t.Errorf("expected ErrLoopCancelled, got: %v", err)
	}
}

func TestAgentLoop_Run_SystemPrompt(t *testing.T) {
	registry := NewRegistry()

	model := &mockLLM{
		responses: []string{"Done."},
	}

	loop, err := NewAgentLoop(model, registry,
		WithLoopSystemPrompt("You are a helpful assistant."),
	)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Say hello",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.State != StateComplete {
		t.Errorf("expected StateComplete, got: %s", result.State)
	}
}

func TestAgentLoop_Run_TaskWithContext(t *testing.T) {
	registry := NewRegistry()

	model := &mockLLM{
		responses: []string{"Done."},
	}

	loop, err := NewAgentLoop(model, registry)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Write code",
		Context:     "Use Go programming language",
	}

	result, err := loop.Run(context.Background(), task, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.State != StateComplete {
		t.Errorf("expected StateComplete, got: %s", result.State)
	}
}

func TestAgentLoop_Definitions(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(&mockTool{
		name:        "test",
		description: "Test tool",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	})

	model := &mockLLM{
		responses: []string{"Done."},
	}

	loop, err := NewAgentLoop(model, registry)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	// Test that registry definitions are properly converted
	defs := loop.registry.Definitions()
	if len(defs) != 1 {
		t.Errorf("expected 1 definition, got: %d", len(defs))
	}
}

func TestAgentLoop_Stream(t *testing.T) {
	registry := NewRegistry()

	model := &mockLLM{
		responses: []string{"Streamed result."},
	}

	loop, err := NewAgentLoop(model, registry)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	task := Task{
		Description: "Stream test",
	}

	results, err := loop.RunStream(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []StreamResult
	for result := range results {
		received = append(received, result)
	}

	if len(received) == 0 {
		t.Error("expected at least one result")
	}

	lastResult := received[len(received)-1]
	if !lastResult.Done {
		t.Error("expected final result to be done")
	}

	if lastResult.State != StateComplete {
		t.Errorf("expected StateComplete, got: %s", lastResult.State)
	}
}

func TestAgentLoop_CompactionHook(t *testing.T) {
	registry := NewRegistry()

	// Tool that forces multiple iterations before finishing.
	callCount := 0
	tool := &mockTool{
		name:        "step",
		description: "advance one step",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			callCount++
			return map[string]any{"step": callCount}, nil
		},
	}
	_ = registry.Register(tool)

	// LLM: call tool twice, then produce final answer.
	model := &mockLLM{
		responses: []string{"thinking", "thinking", "Done."},
		toolCalls: [][]llms.ToolCall{
			{{Function: llms.FunctionCall{Name: "step", Arguments: map[string]any{}}}},
			{{Function: llms.FunctionCall{Name: "step", Arguments: map[string]any{}}}},
		},
	}

	compactionCount := 0
	hook := func(ctx context.Context, msgs []schema.MessageContent, tokens TokenUsage) []schema.MessageContent {
		// Compact after first tool observation (history length > 3: system + task + ai + tool-result).
		if len(msgs) > 3 {
			compactionCount++
			// Return a compacted history: keep system prompt + summary.
			return []schema.MessageContent{
				msgs[0], // system prompt
				schema.NewHumanMessage("[compacted] previous steps summarized"),
			}
		}
		return nil // no compaction needed yet
	}

	loop, err := NewAgentLoop(model, registry,
		WithLoopMaxIterations(10),
		WithLoopCompactionHook(hook),
	)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	result, err := loop.Run(context.Background(), Task{Description: "test compaction"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.State != StateComplete {
		t.Errorf("expected StateComplete, got %s", result.State)
	}
	if compactionCount == 0 {
		t.Error("expected at least one compaction")
	}
	if result.Compactions != compactionCount {
		t.Errorf("Compactions field: want %d, got %d", compactionCount, result.Compactions)
	}
}

func TestAgentLoop_CompactionHookNilReturn(t *testing.T) {
	registry := NewRegistry()

	tool := &mockTool{
		name:        "noop",
		description: "does nothing",
		execFunc: func(ctx context.Context, params map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}
	_ = registry.Register(tool)

	// LLM: one tool call, then final answer — hook returns nil (no compaction).
	model := &mockLLM{
		responses: []string{"thinking", "Done."},
		toolCalls: [][]llms.ToolCall{
			{{Function: llms.FunctionCall{Name: "noop", Arguments: map[string]any{}}}},
		},
	}

	hookCalled := false
	hook := func(ctx context.Context, msgs []schema.MessageContent, tokens TokenUsage) []schema.MessageContent {
		hookCalled = true
		return nil // signal: no compaction needed
	}

	loop, err := NewAgentLoop(model, registry,
		WithLoopCompactionHook(hook),
	)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	result, err := loop.Run(context.Background(), Task{Description: "no compaction"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hookCalled {
		t.Error("expected compaction hook to be called after tool iteration")
	}
	if result.Compactions != 0 {
		t.Errorf("expected 0 compactions (hook returned nil), got %d", result.Compactions)
	}
}
