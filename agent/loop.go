package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// Compactions is the number of times the conversation history was compacted.
	Compactions int
}

// TokenUsage tracks the token consumption of the LLM.
type TokenUsage struct {
	Input      float64
	Output     float64
	Reasoning  float64
	CacheRead  float64
	CacheWrite float64
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

// ActionHandler executes a tool with the given parameters and context.
type ActionHandler func(ctx context.Context, toolName string, params map[string]any) (any, error)

// ActionMiddleware intercepts tool execution in the AgentLoop.
type ActionMiddleware func(next ActionHandler) ActionHandler

// AgentObserver allows tracking the lifecycle of an AgentLoop execution for telemetry.
type AgentObserver interface {
	OnIterationStart(ctx context.Context, iteration int)
	OnThinkComplete(ctx context.Context, response string, toolCalls []llms.ToolCall, tokens TokenUsage, err error)
	OnToolCall(ctx context.Context, toolName string, params map[string]any)
	OnToolResult(ctx context.Context, toolName string, params map[string]any, result any, duration time.Duration, err error)
	OnLoopComplete(ctx context.Context, result *LoopResult, err error)
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
	// maxImagesInContext limits the number of images kept in conversation history.
	// Set to 0 (default) to keep all images, or a positive integer to limit.
	// This helps prevent context overflow when many screenshots are taken.
	maxImagesInContext int
	// compactionHook is called after each iteration with the current messages and
	// cumulative token usage. If it returns a non-nil slice, that slice replaces
	// the conversation history (minus the system prompt, which is preserved).
	// Use this to implement context summarization when approaching token limits.
	compactionHook func(ctx context.Context, msgs []schema.MessageContent, tokens TokenUsage) []schema.MessageContent

	// middlewares wrap the core tool execution registry
	middlewares []ActionMiddleware
	// observer records metrics and telemetry
	observer AgentObserver
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

// WithLoopMaxImagesInContext limits the number of images kept in conversation history.
// Set to 0 (default) to keep all images, or a positive integer to limit.
// This helps prevent context overflow when many screenshots are taken.
// Recommended value: 2-4 for most use cases.
func WithLoopMaxImagesInContext(n int) NativeLoopOption {
	return func(l *AgentLoop) {
		l.maxImagesInContext = n
	}
}

// WithLoopCompactionHook sets a callback invoked after every think-act-observe
// iteration. The hook receives the full conversation history (including the system
// prompt) and the cumulative token usage so far. If it returns a non-nil slice,
// that slice replaces the conversation history for subsequent iterations.
//
// Typical use: summarize and compact the history when token usage approaches
// the model's context limit, preserving the system prompt and recent tool results.
//
// The hook must be safe to call concurrently with the loop (it runs in-loop,
// not in a goroutine, so no extra synchronization is needed).
func WithLoopCompactionHook(fn func(ctx context.Context, msgs []schema.MessageContent, tokens TokenUsage) []schema.MessageContent) NativeLoopOption {
	return func(l *AgentLoop) {
		l.compactionHook = fn
	}
}

// WithLoopMiddleware adds a tool execution middleware to the loop.
func WithLoopMiddleware(mw ActionMiddleware) NativeLoopOption {
	return func(l *AgentLoop) {
		l.middlewares = append(l.middlewares, mw)
	}
}

// WithLoopObserver sets an observer for loop telemetry.
func WithLoopObserver(obs AgentObserver) NativeLoopOption {
	return func(l *AgentLoop) {
		l.observer = obs
	}
}

// generateDefaultTraceID creates a random trace ID without external dependencies.
func generateDefaultTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails (extremely unlikely).
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
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
			err := ErrLoopCancelled
			if l.observer != nil {
				l.observer.OnLoopComplete(ctx, result, err)
			}
			return result, err
		default:
		}

		if l.observer != nil {
			l.observer.OnIterationStart(ctx, i+1)
		}

		logger.Debug("starting iteration",
			"iteration", i+1,
			"state", "thinking",
		)

		// THINK: Call LLM with available tools
		response, toolCalls, tokens, err := l.think(ctx, messages)

		if l.observer != nil {
			l.observer.OnThinkComplete(ctx, response, toolCalls, tokens, err)
		}

		if err != nil {
			result.State = StateError
			err = fmt.Errorf("think phase failed: %w", err)
			if l.observer != nil {
				l.observer.OnLoopComplete(ctx, result, err)
			}
			return result, err
		}

		// Accumulate token usage.
		result.Tokens.Input += tokens.Input
		result.Tokens.Output += tokens.Output
		result.Tokens.Reasoning += tokens.Reasoning
		result.Tokens.CacheRead += tokens.CacheRead
		result.Tokens.CacheWrite += tokens.CacheWrite

		// Add AI response to history
		if len(toolCalls) > 0 {
			tcParts := make([]schema.ToolCallContent, len(toolCalls))
			for i, tc := range toolCalls {
				tcParts[i] = schema.ToolCallContent{
					ID:           tc.ID,
					FunctionName: tc.Function.Name,
					Arguments:    tc.Function.Arguments,
				}
			}
			messages = append(messages, schema.NewAIMessageWithToolCalls(response, tcParts))
		} else {
			messages = append(messages, schema.NewAIMessage(response))
		}

		// If no tool calls, we have a final answer
		if len(toolCalls) == 0 {
			result.Response = response
			result.State = StateComplete
			result.Iterations = i + 1
			logger.Info("loop completed",
				"iterations", result.Iterations,
				"response_length", len(response),
			)
			if l.observer != nil {
				l.observer.OnLoopComplete(ctx, result, nil)
			}
			return result, nil
		}

		// ACT & OBSERVE: Execute tools and collect observations
		observations, toolRecords := l.actAndObserve(ctx, toolCalls)

		// Add observations to history
		messages = append(messages, observations...)

		// Limit images in context if configured (helps prevent context overflow)
		if l.maxImagesInContext > 0 {
			messages = trimImageMessages(messages, l.maxImagesInContext)
		}

		result.ToolCalls = append(result.ToolCalls, toolRecords...)
		result.Iterations = i + 1

		// COMPACT: invoke the compaction hook if set. If it returns a non-nil
		// slice the conversation history is replaced, which keeps token usage
		// in check for long-running loops.
		if l.compactionHook != nil {
			if compacted := l.compactionHook(ctx, messages, result.Tokens); compacted != nil {
				logger.Info("context compacted",
					"iteration", i+1,
					"before", len(messages),
					"after", len(compacted),
				)
				messages = compacted
				result.Compactions++
			}
		}
	}

	result.State = StateError
	err := fmt.Errorf("%w (max: %d)", ErrMaxIterations, l.maxIterations)
	if l.observer != nil {
		l.observer.OnLoopComplete(ctx, result, err)
	}
	return result, err
}

// trimImageMessages keeps only the most recent N images in the message history.
// This prevents context overflow when many screenshots are taken during execution.
// Images in the most recent messages are preserved, older ones are removed.
// The returned slice is a new allocation; original messages are never mutated.
func trimImageMessages(messages []schema.MessageContent, maxImages int) []schema.MessageContent {
	imageCount := 0
	trimmed := make([]schema.MessageContent, 0, len(messages))

	// Iterate in reverse to find most recent images
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		hasImage := false

		// Check if this message contains images
		for _, part := range msg.Parts {
			if _, ok := part.(schema.ImageContent); ok {
				hasImage = true
				break
			}
		}

		if hasImage {
			imageCount++
			if imageCount > maxImages {
				// Build a copy with image parts removed — never mutate the original.
				newParts := make([]schema.ContentPart, 0, len(msg.Parts))
				for _, part := range msg.Parts {
					if _, isImage := part.(schema.ImageContent); !isImage {
						newParts = append(newParts, part)
					}
				}
				if len(newParts) == 0 {
					newParts = []schema.ContentPart{
						schema.TextContent{Text: "[Image trimmed to save context space]"},
					}
				}
				// Create a new MessageContent; do NOT assign back to msg.Parts.
				msg = schema.MessageContent{
					Role:  msg.Role,
					Parts: newParts,
				}
			}
		}

		trimmed = append([]schema.MessageContent{msg}, trimmed...)
	}

	return trimmed
}

// think calls the LLM with the current context and available tools.
// It returns the text response, any tool calls, the token usage for this call, and an error.
func (l *AgentLoop) think(ctx context.Context, messages []schema.MessageContent) (string, []llms.ToolCall, TokenUsage, error) {
	var tokens TokenUsage

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
		return "", nil, tokens, fmt.Errorf("LLM call failed: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", nil, tokens, errors.New("empty response from LLM")
	}

	choice := response.Choices[0]

	// Extract token usage and tool calls from generation info.
	// Providers use different key names and types; check all variants.
	var toolCalls []llms.ToolCall
	if genInfo := choice.GenerationInfo; genInfo != nil {
		tokens.Input = toFloat64(genInfo["PromptTokens"], genInfo["InputTokens"])
		tokens.Output = toFloat64(genInfo["CompletionTokens"], genInfo["OutputTokens"])

		// Reasoning tokens: prefer explicit key, fallback to Total - Input - Output
		if v := toFloat64(genInfo["ReasoningTokens"]); v > 0 {
			tokens.Reasoning = v
		} else if total := toFloat64(genInfo["TotalTokens"]); total > 0 && tokens.Input > 0 && tokens.Output > 0 {
			tokens.Reasoning = total - tokens.Input - tokens.Output
		}

		tokens.CacheRead = toFloat64(genInfo["CacheRead"])
		tokens.CacheWrite = toFloat64(genInfo["CacheWrite"])

		if tc, ok := genInfo["ToolCalls"].([]llms.ToolCall); ok {
			toolCalls = tc
		} else if tc, ok := genInfo["tool_calls"].([]llms.ToolCall); ok {
			toolCalls = tc
		}
	}

	return choice.Content, toolCalls, tokens, nil
}

// actAndObserve executes tools and returns observations for the LLM.
func (l *AgentLoop) actAndObserve(ctx context.Context, toolCalls []llms.ToolCall) ([]schema.MessageContent, []ToolCallRecord) {
	toolRecords := make([]ToolCallRecord, 0, len(toolCalls))
	observations := make([]schema.MessageContent, 0, len(toolCalls))

	// Pre-build middleware chain
	handler := func(ctx context.Context, toolName string, params map[string]any) (any, error) {
		return l.registry.Execute(ctx, toolName, params)
	}
	for i := len(l.middlewares) - 1; i >= 0; i-- {
		handler = l.middlewares[i](handler)
	}

	for _, tc := range toolCalls {
		toolName := tc.Function.Name
		toolCallID := tc.ID
		params := tc.Function.Arguments

		// Normalize params if wrapped in "properties" key
		if props, ok := params["properties"]; ok {
			if propsMap, ok := props.(map[string]any); ok {
				params = propsMap
			}
		}

		if l.observer != nil {
			l.observer.OnToolCall(ctx, toolName, params)
		}

		l.logger.DebugContext(ctx, "executing tool",
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
				observations = append(observations, schema.NewToolResultMessageWithID(toolName, toolCallID, obsContent))
				continue
			}
		}

		// Execute the tool through middleware chain
		startTime := time.Now()
		result, err := handler(ctx, toolName, params)
		duration := time.Since(startTime)

		if l.observer != nil {
			l.observer.OnToolResult(ctx, toolName, params, result, duration, err)
		}

		record.Result = result
		record.Error = err
		toolRecords = append(toolRecords, record)

		// Create observation message
		if err != nil {
			l.logger.Error("tool execution failed",
				"tool", toolName,
				"error", err,
				"duration_ms", duration.Milliseconds(),
			)
			obsContent := fmt.Sprintf("Tool '%s' failed: %s", toolName, err.Error())
			observations = append(observations, schema.NewToolResultMessageWithID(toolName, toolCallID, obsContent))
		} else {
			// Extract base64 image if present (for vision models)
			// Store it so we can send as a follow-up user message (Ollama only supports images in user role)
			var imageData string
			if resultMap, ok := result.(map[string]any); ok {
				if img, ok := resultMap["imageBase64"].(string); ok && img != "" && len(img) > 100 {
					imageData = img
				} else if img, ok := resultMap["image_base64"].(string); ok && img != "" && len(img) > 100 {
					imageData = img
				} else if img, ok := resultMap["image"].(string); ok && img != "" && len(img) > 100 {
					imageData = img
				}
			}

			// Serialize result to JSON (without the image data to reduce token usage)
			resultForJSON := result
			if imageData != "" {
				// Create a copy without the image for the JSON representation
				if resultMap, ok := result.(map[string]any); ok {
					resultForJSON = make(map[string]any)
					for k, v := range resultMap {
						if k != "imageBase64" && k != "image_base64" && k != "image" {
							if m, ok := resultForJSON.(map[string]any); ok {
								m[k] = v
							}
						}
					}
				}
			}

			jsonBytes, jsonErr := json.Marshal(resultForJSON)
			var obsContent string
			if jsonErr != nil {
				obsContent = fmt.Sprintf("Tool '%s' returned: %v", toolName, result)
			} else {
				obsContent = fmt.Sprintf("Tool '%s' returned: %s", toolName, string(jsonBytes))
			}
			observations = append(observations, schema.NewToolResultMessageWithID(toolName, toolCallID, obsContent))

			// If image present, add a user message with the image for vision models
			// Ollama only supports images in user role messages
			if imageData != "" {
				imagePart := schema.ImageContent{Data: imageData, MimeType: "image/png"}
				userMsg := schema.MessageContent{
					Role: schema.ChatMessageTypeHuman,
					Parts: []schema.ContentPart{
						schema.TextContent{Text: fmt.Sprintf("Here is the screenshot from tool '%s':", toolName)},
						imagePart,
					},
				}
				observations = append(observations, userMsg)
			}
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

// toFloat64 extracts a float64 value from a generation info map entry.
// It accepts float64 (typical json unmarshal), int, and int64. Multiple
// candidate values can be provided; the first non-zero match is returned.
func toFloat64(candidates ...any) float64 {
	for _, c := range candidates {
		if c == nil {
			continue
		}
		switch v := c.(type) {
		case float64:
			if v != 0 {
				return v
			}
		case int:
			if v != 0 {
				return float64(v)
			}
		case int64:
			if v != 0 {
				return float64(v)
			}
		}
	}
	return 0
}
