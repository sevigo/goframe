package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

var (
	ErrMaxIterations = errors.New("agent: maximum iterations exceeded")
	ErrNoLLM         = errors.New("agent: no LLM model provided")
	ErrNoRegistry    = errors.New("agent: no tool registry provided")
	ErrLoopCancelled = errors.New("agent: loop cancelled by context")
)

// LoopState represents the current state of the agent loop.
type LoopState string

const (
	StateThinking  LoopState = "thinking"
	StateActing    LoopState = "acting"
	StateObserving LoopState = "observing"
	StateComplete  LoopState = "complete"
	StateError     LoopState = "error"
)

// Task represents a unit of work for the agent.
type Task struct {
	// ID is a unique identifier for the task.
	ID string
	// Description is the human-readable task description.
	Description string
	// Context provides additional context for the task.
	Context string
	// Priority indicates task priority (higher = more important).
	Priority int
}

// LoopResult represents the final result of an agent loop execution.
type LoopResult struct {
	// Response is the final answer from the LLM.
	Response string
	// ToolCalls is the list of tool calls made during execution.
	ToolCalls []ToolCallRecord
	// Iterations is the number of think-act-observe cycles completed.
	Iterations int
	// Tokens is the total token usage across all LLM calls.
	Tokens TokenUsage
	// State is the final loop state.
	State LoopState
	// TraceID is a unique identifier for tracing loop execution.
	TraceID string
}

// ToolCallRecord records a single tool execution.
type ToolCallRecord struct {
	// Name is the tool that was called.
	Name string
	// Params are the parameters passed to the tool.
	Params map[string]any
	// Result is the tool's return value.
	Result any
	// Error is any error that occurred during execution.
	Error error
}

// NativeLoopOption configures the agent loop.
type NativeLoopOption func(*AgentLoop)

// AgentLoop manages the "Think-Act-Observe" lifecycle for autonomous agents.
type AgentLoop struct {
	// Model is the LLM used for reasoning.
	model llms.Model
	// Registry provides tools for the agent to use.
	registry *Registry
	// Governance validates tool executions before they run.
	governance *Governance
	// MaxIterations limits the number of think-act-observe cycles.
	maxIterations int
	// SystemPrompt is prepended to all conversations.
	systemPrompt string
	// Temperature controls LLM randomness.
	temperature float64
	// Logger records loop execution details.
	logger *slog.Logger
	// GenerateTraceID generates a unique trace ID for the loop.
	// If nil, a default UUID-based ID is generated.
	GenerateTraceID func() string
}

// NewAgentLoop creates a new agent loop with the given configuration.
func NewAgentLoop(model llms.Model, registry *Registry, opts ...NativeLoopOption) (*AgentLoop, error) {
	if model == nil {
		return nil, ErrNoLLM
	}
	if registry == nil {
		return nil, ErrNoRegistry
	}

	loop := &AgentLoop{
		model:         model,
		registry:      registry,
		maxIterations: 10,
		logger:        slog.Default(),
	}

	for _, opt := range opts {
		opt(loop)
	}

	return loop, nil
}

// WithLoopMaxIterations sets the maximum number of iterations.
func WithLoopMaxIterations(n int) NativeLoopOption {
	return func(l *AgentLoop) {
		if n > 0 {
			l.maxIterations = n
		}
	}
}

// WithLoopGovernance sets the governance checks.
func WithLoopGovernance(g *Governance) NativeLoopOption {
	return func(l *AgentLoop) {
		l.governance = g
	}
}

// WithLoopSystemPrompt sets the system prompt for the LLM.
func WithLoopSystemPrompt(prompt string) NativeLoopOption {
	return func(l *AgentLoop) {
		l.systemPrompt = prompt
	}
}

// WithLoopTemperature sets the LLM temperature.
func WithLoopTemperature(temp float64) NativeLoopOption {
	return func(l *AgentLoop) {
		if temp >= 0 && temp <= 2 {
			l.temperature = temp
		}
	}
}

// WithLoopLogger sets the logger for the loop.
func WithLoopLogger(logger *slog.Logger) NativeLoopOption {
	return func(l *AgentLoop) {
		if logger != nil {
			l.logger = logger
		}
	}
}

// WithLoopTraceID sets a custom trace ID generator.
// The trace ID is useful for correlating logs across a multi-step agent execution.
func WithLoopTraceID(gen func() string) NativeLoopOption {
	return func(l *AgentLoop) {
		l.GenerateTraceID = gen
	}
}

// generateTraceID creates a default trace ID if none is provided.
func generateDefaultTraceID() string {
	// Simple UUID-like ID without external dependencies
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Run executes the autonomous loop for a given task and session.
// The session maintains the conversation history across iterations.
func (l *AgentLoop) Run(ctx context.Context, task Task, history []schema.MessageContent) (*LoopResult, error) {
	// Generate or use provided trace ID
	var traceID string
	if l.GenerateTraceID != nil {
		traceID = l.GenerateTraceID()
	} else {
		traceID = generateDefaultTraceID()
	}

	// Create logger with trace ID for consistent log correlation
	logger := l.logger.With("trace_id", traceID)

	result := &LoopResult{
		State:   StateThinking,
		TraceID: traceID,
	}

	// Build the initial message history with system prompt
	messages := l.buildInitialHistory(task)

	// Merge with provided history
	messages = append(messages, history...)

	// Run the think-act-observe loop
	for i := range l.maxIterations {
		select {
		case <-ctx.Done():
			result.State = StateError
			return result, ErrLoopCancelled
		default:
		}

		logger.Debug("starting iteration",
			"iteration", i+1,
			"state", "thinking",
		)

		// THINK: Call LLM with available tools
		response, toolCalls, err := l.think(ctx, messages)
		if err != nil {
			result.State = StateError
			return result, fmt.Errorf("think phase failed: %w", err)
		}

		// Add AI response to history
		messages = append(messages, schema.NewAIMessage(response))

		// If no tool calls, we have a final answer
		if len(toolCalls) == 0 {
			result.Response = response
			result.State = StateComplete
			result.Iterations = i + 1
			logger.Info("loop completed",
				"iterations", result.Iterations,
				"response_length", len(response),
			)
			return result, nil
		}

		// ACT & OBSERVE: Execute tools and collect observations
		observations, toolRecords := l.actAndObserve(ctx, toolCalls)

		// Add observations to history
		messages = append(messages, observations...)
		result.ToolCalls = append(result.ToolCalls, toolRecords...)
		result.Iterations = i + 1
	}

	result.State = StateError
	return result, fmt.Errorf("%w (max: %d)", ErrMaxIterations, l.maxIterations)
}

// think calls the LLM with the current context and available tools.
func (l *AgentLoop) think(ctx context.Context, messages []schema.MessageContent) (string, []llms.ToolCall, error) {
	// Build tool definitions from registry
	toolDefs := l.registry.Definitions()
	tools := make([]llms.ToolDefinition, len(toolDefs))
	for i, def := range toolDefs {
		fn, _ := def["function"].(map[string]any)
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		tools[i] = llms.ToolDefinition{
			Type: "function",
			Function: llms.FunctionDefinition{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		}
	}

	opts := []llms.CallOption{
		llms.WithTools(tools),
	}

	if l.temperature > 0 {
		opts = append(opts, llms.WithTemperature(l.temperature))
	}

	response, err := l.model.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", nil, fmt.Errorf("LLM call failed: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", nil, errors.New("empty response from LLM")
	}

	choice := response.Choices[0]

	// Check for tool calls in generation info
	var toolCalls []llms.ToolCall
	if genInfo := choice.GenerationInfo; genInfo != nil {
		// Try both keys for compatibility
		if tc, ok := genInfo["ToolCalls"].([]llms.ToolCall); ok {
			toolCalls = tc
		} else if tc, ok := genInfo["tool_calls"].([]llms.ToolCall); ok {
			toolCalls = tc
		}
	}

	return choice.Content, toolCalls, nil
}

// actAndObserve executes tools and returns observations for the LLM.
func (l *AgentLoop) actAndObserve(ctx context.Context, toolCalls []llms.ToolCall) ([]schema.MessageContent, []ToolCallRecord) {
	toolRecords := make([]ToolCallRecord, 0, len(toolCalls))
	observations := make([]schema.MessageContent, 0, len(toolCalls))

	for _, tc := range toolCalls {
		toolName := tc.Function.Name
		params := tc.Function.Arguments

		// Normalize params if wrapped in "properties" key
		if props, ok := params["properties"]; ok {
			if propsMap, ok := props.(map[string]any); ok {
				params = propsMap
			}
		}

		l.logger.Debug("executing tool",
			"tool", toolName,
			"params", params,
		)

		record := ToolCallRecord{
			Name:   toolName,
			Params: params,
		}

		// Run governance checks
		if l.governance != nil {
			if err := l.governance.Validate(ctx, toolName, params); err != nil {
				l.logger.Warn("governance blocked tool execution",
					"tool", toolName,
					"error", err,
				)

				record.Error = err
				toolRecords = append(toolRecords, record)

				// Add observation with error message
				obsContent := fmt.Sprintf("Tool '%s' was blocked: %s", toolName, err.Error())
				observations = append(observations, schema.NewToolResultMessage(toolName, obsContent))
				continue
			}
		}

		// Execute the tool
		result, err := l.registry.Execute(ctx, toolName, params)

		record.Result = result
		record.Error = err
		toolRecords = append(toolRecords, record)

		// Create observation message - check for images first (vision support)
		if err != nil {
			l.logger.Error("tool execution failed",
				"tool", toolName,
				"error", err,
			)
			obsContent := fmt.Sprintf("Tool '%s' failed: %s", toolName, err.Error())
			observations = append(observations, schema.NewToolResultMessage(toolName, obsContent))
		} else {
			l.logger.Debug("tool execution succeeded", "tool", toolName)

			// Check if result contains image for vision models
			if resultMap, ok := result.(map[string]any); ok {
				var imageData string
				var found bool

				// Check various image field names
				if img, ok := resultMap["imageBase64"].(string); ok && img != "" {
					imageData = img
					found = true
				} else if img, ok := resultMap["image"].(string); ok && img != "" {
					imageData = img
					found = true
				}

				// If image found, create multimodal message for vision models
				if found && len(imageData) > 100 {
					textPart := schema.TextContent{Text: fmt.Sprintf("Tool '%s' returned (see image):", toolName)}
					imagePart := schema.ImageContent{Data: imageData, MimeType: "image/png"}

					obsMsg := schema.MessageContent{
						Role:  schema.ChatMessageTypeTool,
						Parts: []schema.ContentPart{textPart, imagePart},
					}
					observations = append(observations, obsMsg)
					continue
				}
			}

			// Default: serialize result to JSON text
			jsonBytes, jsonErr := json.Marshal(result)
			var obsContent string
			if jsonErr != nil {
				obsContent = fmt.Sprintf("Tool '%s' returned: %v", toolName, result)
			} else {
				obsContent = fmt.Sprintf("Tool '%s' returned: %s", toolName, string(jsonBytes))
			}
			observations = append(observations, schema.NewToolResultMessage(toolName, obsContent))
		}
	}

	return observations, toolRecords
}

// buildInitialHistory creates the initial message history with system prompt and task.
func (l *AgentLoop) buildInitialHistory(task Task) []schema.MessageContent {
	messages := make([]schema.MessageContent, 0)

	// Add system prompt
	if l.systemPrompt != "" {
		messages = append(messages, schema.NewSystemMessage(l.systemPrompt))
	}

	// Add task as user message
	taskText := task.Description
	if task.Context != "" {
		taskText = fmt.Sprintf("%s\n\nContext:\n%s", task.Description, task.Context)
	}
	messages = append(messages, schema.NewHumanMessage(taskText))

	return messages
}

// StreamResult represents a partial result during streaming execution.
type StreamResult struct {
	// State indicates the current loop state.
	State LoopState
	// Text is the partial text content from the LLM.
	Text string
	// ToolCall is a tool call being executed.
	ToolCall *ToolCallRecord
	// Error is any error that occurred.
	Error error
	// Done indicates if the loop has completed.
	Done bool
}

// RunStream executes the loop and streams results.
// This allows real-time monitoring of the agent's progress.
func (l *AgentLoop) RunStream(ctx context.Context, task Task, history []schema.MessageContent) (<-chan StreamResult, error) {
	results := make(chan StreamResult, 100)

	go func() {
		defer close(results)

		result, err := l.Run(ctx, task, history)

		if err != nil {
			results <- StreamResult{
				State: StateError,
				Error: err,
				Done:  true,
			}
			return
		}

		results <- StreamResult{
			State: result.State,
			Text:  result.Response,
			Done:  true,
		}
	}()

	return results, nil
}
