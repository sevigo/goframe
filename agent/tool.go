package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
)

var (
	// ErrToolInvalidParams is returned when tool parameters are invalid.
	ErrToolInvalidParams = errors.New("agent: invalid tool parameters")
	// ErrToolExecution is returned when a tool execution fails.
	ErrToolExecution = errors.New("agent: tool execution failed")
)

// Tool defines the interface for agent tools.
// Tools are functions that an agent can call during execution.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// ParametersSchema returns a JSON Schema describing the tool's parameters.
	ParametersSchema() map[string]any

	// Execute runs the tool with the given parameters.
	// The params map contains the arguments parsed from the LLM's tool call.
	Execute(ctx context.Context, params map[string]any) (any, error)
}

// ToolFunc is an adapter to use a function as a Tool.
type ToolFunc struct {
	name        string
	description string
	schema      map[string]any
	execFunc    func(ctx context.Context, params map[string]any) (any, error)
}

// Name returns the tool's unique identifier.
func (t *ToolFunc) Name() string { return t.name }

// Description returns a human-readable description of what the tool does.
func (t *ToolFunc) Description() string { return t.description }

// ParametersSchema returns a JSON Schema describing the tool's parameters.
func (t *ToolFunc) ParametersSchema() map[string]any { return t.schema }

// Execute runs the tool with the given parameters.
func (t *ToolFunc) Execute(ctx context.Context, params map[string]any) (any, error) {
	return t.execFunc(ctx, params)
}

// Registry manages available tools for an agent.
// It provides lookup, validation, and execution capabilities.
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// NewRegistryWithTools creates a registry pre-populated with tools.
func NewRegistryWithTools(tools ...Tool) (*Registry, error) {
	r := NewRegistry()
	for _, tool := range tools {
		if err := r.Register(tool); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds a tool to the registry.
// Returns an error if a tool with the same name already exists.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("agent: cannot register nil tool")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("agent: tool %q already registered", name)
	}

	r.tools[name] = tool
	return nil
}

// Unregister removes a tool from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get retrieves a tool by name.
// Returns ErrToolNotFound if the tool doesn't exist.
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return tool, nil
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Definitions returns tool definitions in the format expected by LLMs.
// This returns a slice format suitable for most LLM APIs.
func (r *Registry) Definitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]map[string]any, 0, len(r.tools))
	for _, tool := range r.tools {
		defs = append(defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.ParametersSchema(),
			},
		})
	}
	return defs
}

// GetSchemaMap returns tool schemas as a map keyed by tool name.
// This format is useful for parallel tool-calling protocols and
// supports complex tool orchestration workflows.
//
// Example:
//
//	schemas := registry.GetSchemaMap()
//	searchSchema := schemas["search"]
//	// Use schema for validation or introspection
func (r *Registry) GetSchemaMap() map[string]map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make(map[string]map[string]any, len(r.tools))
	for _, tool := range r.tools {
		schemas[tool.Name()] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.ParametersSchema(),
			},
		}
	}
	return schemas
}

// Execute runs a tool by name with the given parameters.
// Returns ErrToolNotFound if the tool doesn't exist, or ErrToolExecution if execution fails.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]any) (any, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	result, err := tool.Execute(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrToolExecution, err)
	}

	return result, nil
}

// NewToolFromFunc creates a Tool from a function using reflection.
// The function must have a signature like: func(ctx context.Context, params T) (R, error)
// where T is a struct with JSON tags and R is the return type.
//
// Example:
//
//	tool, err := NewToolFromFunc(
//	    "search",
//	    "Search for documents matching a query",
//	    func(ctx context.Context, params SearchParams) (SearchResult, error) {
//	        // implementation
//	    },
//	)
func NewToolFromFunc(name, description string, fn any) (Tool, error) {
	if fn == nil {
		return nil, errors.New("agent: function cannot be nil")
	}

	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return nil, errors.New("agent: fn must be a function")
	}

	schema, paramIndex, err := extractSchemaFromFunc(fnType)
	if err != nil {
		return nil, fmt.Errorf("agent: failed to extract schema: %w", err)
	}

	execFunc := buildExecFunc(fn, fnType, paramIndex)

	return &ToolFunc{
		name:        name,
		description: description,
		schema:      schema,
		execFunc:    execFunc,
	}, nil
}

// extractSchemaFromFunc generates a JSON Schema from function parameters.
func extractSchemaFromFunc(fnType reflect.Type) (map[string]any, int, error) {
	if fnType.NumIn() < 1 {
		return nil, 0, errors.New("agent: function must have at least one parameter (context.Context)")
	}

	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if fnType.In(0) != ctxType {
		return nil, 0, errors.New("agent: first parameter must be context.Context")
	}

	if fnType.NumIn() < 2 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}, 1, nil
	}

	paramType := fnType.In(1)
	schema := generateJSONSchema(paramType, true)

	return schema, 1, nil
}

// generateJSONSchema creates a JSON Schema from a Go type using reflection.
func generateJSONSchema(t reflect.Type, isRoot bool) map[string]any {
	schema := make(map[string]any)

	switch t.Kind() {
	case reflect.Struct:
		schema["type"] = "object"
		properties := make(map[string]any)
		required := make([]string, 0)

		for i := range t.NumField() {
			field := t.Field(i)
			fieldName := getJSONFieldName(field)
			if fieldName == "-" || fieldName == "" {
				continue
			}

			fieldSchema := generateJSONSchema(field.Type, false)
			properties[fieldName] = fieldSchema

			if !hasOptionalTag(field) {
				required = append(required, fieldName)
			}
		}

		schema["properties"] = properties
		if len(required) > 0 {
			schema["required"] = required
		}

	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = generateJSONSchema(t.Elem(), false)

	case reflect.Map:
		schema["type"] = "object"
		if t.Key().Kind() == reflect.String {
			schema["additionalProperties"] = generateJSONSchema(t.Elem(), false)
		}

	case reflect.String:
		schema["type"] = "string"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"

	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"

	case reflect.Bool:
		schema["type"] = "boolean"

	case reflect.Ptr:
		return generateJSONSchema(t.Elem(), isRoot)

	case reflect.Interface:
		schema["type"] = "object"
		schema["additionalProperties"] = true
	}

	if isRoot {
		return schema
	}

	return schema
}

// getJSONFieldName extracts the JSON field name from struct tags.
func getJSONFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(field.Name)
	}

	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		return strings.ToLower(field.Name)
	}
	return name
}

// hasOptionalTag checks if the field is marked as optional.
func hasOptionalTag(field reflect.StructField) bool {
	tag := field.Tag.Get("json")
	if tag == "" {
		return false
	}
	parts := strings.Split(tag, ",")
	for _, part := range parts[1:] {
		if part == "omitempty" {
			return true
		}
	}
	return false
}

// buildExecFunc creates an execution function that handles parameter binding.
func buildExecFunc(fn any, fnType reflect.Type, paramIndex int) func(context.Context, map[string]any) (any, error) {
	return func(ctx context.Context, params map[string]any) (any, error) {
		args := make([]reflect.Value, fnType.NumIn())
		args[0] = reflect.ValueOf(ctx)

		if fnType.NumIn() > 1 {
			paramType := fnType.In(paramIndex)
			paramValue := reflect.New(paramType).Interface()

			jsonBytes, err := json.Marshal(params)
			if err != nil {
				return nil, fmt.Errorf("agent: failed to marshal params: %w", err)
			}

			if err := json.Unmarshal(jsonBytes, paramValue); err != nil {
				return nil, fmt.Errorf("agent: failed to unmarshal params: %w", err)
			}

			args[paramIndex] = reflect.ValueOf(paramValue).Elem()
		}

		results := reflect.ValueOf(fn).Call(args)

		if len(results) == 0 {
			return nil, nil
		}

		var err error
		if len(results) > 1 {
			if errVal, ok := results[1].Interface().(error); ok {
				err = errVal
			}
		}

		if err != nil {
			return nil, err
		}

		return results[0].Interface(), nil
	}
}

// MustRegisterTool registers a tool and panics on error.
// Use for static initialization of tools.
func (r *Registry) MustRegisterTool(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(fmt.Sprintf("agent: failed to register tool: %v", err))
	}
}

// ToolLogger wraps a tool with logging.
func ToolLogger(tool Tool, logger *slog.Logger) Tool {
	return &loggingTool{
		Tool:   tool,
		logger: logger,
	}
}

type loggingTool struct {
	Tool
	logger *slog.Logger
}

// Execute runs the wrapped tool and logs the call.
func (t *loggingTool) Execute(ctx context.Context, params map[string]any) (any, error) {
	t.logger.DebugContext(ctx, "executing tool",
		"name", t.Name(),
		"params", params,
	)

	result, err := t.Tool.Execute(ctx, params)
	if err != nil {
		t.logger.ErrorContext(ctx, "tool execution failed",
			"name", t.Name(),
			"error", err,
		)
		return nil, err
	}

	t.logger.DebugContext(ctx, "tool execution complete",
		"name", t.Name(),
	)
	return result, nil
}
