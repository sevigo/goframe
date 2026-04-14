package llms

import "context"

// CallOption configures LLM generation options.
type CallOption func(*CallOptions)

// CallOptions contains configurable options for LLM generation calls.
type CallOptions struct {
	// Model specifies the model to use (overrides default).
	Model string `json:"model"`
	// Temperature controls randomness in generation (0.0 to 2.0).
	Temperature float64 `json:"temperature"`
	// MaxTokens limits the maximum tokens in the response.
	MaxTokens int `json:"max_tokens"`
	// StopWords specifies sequences where generation should stop.
	StopWords []string `json:"stop_words"`
	// TopP controls diversity via nucleus sampling (0.0 to 1.0).
	TopP float64 `json:"top_p"`
	// TopK limits sampling to top K tokens.
	TopK int `json:"top_k"`
	// MinP sets minimum probability threshold for token selection.
	MinP float64 `json:"min_p"`
	// Seed sets a deterministic seed for reproducible outputs.
	Seed int `json:"seed"`
	// Metadata contains additional provider-specific options.
	Metadata map[string]any `json:"metadata,omitempty"`
	// StreamingFunc is called for each chunk when streaming is enabled.
	StreamingFunc func(ctx context.Context, chunk []byte) error `json:"-"`
	// JSONMode enables JSON output format.
	JSONMode bool `json:"json_mode"`
	// JSONSchema specifies a JSON schema for structured output.
	JSONSchema any `json:"json_schema,omitempty"`
	// Tools specifies function tools the model may call.
	Tools []ToolDefinition `json:"tools,omitempty"`
	// Think enables thinking/reasoning output for supported models.
	// Can be true/false or "high"/"medium"/"low" for some models.
	Think any `json:"think,omitempty"`
	// KeepAlive controls how long the model stays loaded in memory.
	KeepAlive string `json:"keep_alive,omitempty"`
	// ContextLength sets the context window size in tokens.
	ContextLength int `json:"context_length,omitempty"`

	// setFields tracks which numeric fields were explicitly set,
	// since zero values (Temperature=0, Seed=0) are valid but
	// the default zero would otherwise be indistinguishable from "not set".
	setFields callOptionFields
}

// callOptionFields is a bitmask tracking which CallOptions fields were
// explicitly set through a With* option function.
type callOptionFields uint

const (
	fieldTemperature callOptionFields = 1 << iota
	fieldSeed
	fieldTopP
	fieldTopK
	fieldMinP
)

// TemperatureSet reports whether Temperature was explicitly set.
func (c *CallOptions) TemperatureSet() bool { return c.setFields&fieldTemperature != 0 }

// SeedSet reports whether Seed was explicitly set.
func (c *CallOptions) SeedSet() bool { return c.setFields&fieldSeed != 0 }

// TopPSet reports whether TopP was explicitly set.
func (c *CallOptions) TopPSet() bool { return c.setFields&fieldTopP != 0 }

// TopKSet reports whether TopK was explicitly set.
func (c *CallOptions) TopKSet() bool { return c.setFields&fieldTopK != 0 }

// MinPSet reports whether MinP was explicitly set.
func (c *CallOptions) MinPSet() bool { return c.setFields&fieldMinP != 0 }

// WithStreamingFunc specifies the streaming function to use.
// The function is called for each chunk of the streamed response.
func WithStreamingFunc(streamingFunc func(ctx context.Context, chunk []byte) error) CallOption {
	return func(o *CallOptions) {
		o.StreamingFunc = streamingFunc
	}
}

// WithTemperature specifies the temperature to use.
// Higher values produce more random outputs.
func WithTemperature(temperature float64) CallOption {
	return func(o *CallOptions) {
		o.Temperature = temperature
		o.setFields |= fieldTemperature
	}
}

// WithMaxTokens specifies the maximum number of tokens to generate.
func WithMaxTokens(maxTokens int) CallOption {
	return func(o *CallOptions) {
		o.MaxTokens = maxTokens
	}
}

// WithStopWords specifies the stop words to use.
// Generation stops when any stop word is encountered.
func WithStopWords(stopWords []string) CallOption {
	return func(o *CallOptions) {
		o.StopWords = stopWords
	}
}

// WithTopP specifies the top-p value to use for nucleus sampling.
func WithTopP(topP float64) CallOption {
	return func(o *CallOptions) {
		o.TopP = topP
		o.setFields |= fieldTopP
	}
}

// WithTopK specifies the top-k value to use for sampling.
func WithTopK(topK int) CallOption {
	return func(o *CallOptions) {
		o.TopK = topK
		o.setFields |= fieldTopK
	}
}

// WithSeed specifies the seed for deterministic generation.
func WithSeed(seed int) CallOption {
	return func(o *CallOptions) {
		o.Seed = seed
		o.setFields |= fieldSeed
	}
}

// WithModel specifies the model to use for this call.
func WithModel(model string) CallOption {
	return func(o *CallOptions) {
		o.Model = model
	}
}

// WithMinP specifies the minimum probability threshold for token selection.
func WithMinP(minP float64) CallOption {
	return func(o *CallOptions) {
		o.MinP = minP
		o.setFields |= fieldMinP
	}
}

// WithJSONMode enables JSON output format.
func WithJSONMode(enabled bool) CallOption {
	return func(o *CallOptions) {
		o.JSONMode = enabled
	}
}

// WithJSONSchema specifies a JSON schema for structured output.
func WithJSONSchema(schema any) CallOption {
	return func(o *CallOptions) {
		o.JSONSchema = schema
	}
}

// WithTools specifies function tools the model may call.
func WithTools(tools []ToolDefinition) CallOption {
	return func(o *CallOptions) {
		o.Tools = tools
	}
}

// WithThink enables thinking/reasoning output for supported models.
// Pass true/false for standard models, or "high"/"medium"/"low" for GPT-OSS.
func WithThink(think any) CallOption {
	return func(o *CallOptions) {
		o.Think = think
	}
}

// WithKeepAlive controls how long the model stays loaded in memory.
// Examples: "5m", "10m", "0" to unload immediately.
func WithKeepAlive(keepAlive string) CallOption {
	return func(o *CallOptions) {
		o.KeepAlive = keepAlive
	}
}

// WithContextLength sets the context window size in tokens.
func WithContextLength(length int) CallOption {
	return func(o *CallOptions) {
		o.ContextLength = length
	}
}

// ToolDefinition defines a function tool the model may call.
type ToolDefinition struct {
	// Type is always "function".
	Type string `json:"type"`
	// Function contains the function definition.
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes a function that can be called by the model.
type FunctionDefinition struct {
	// Name is the function name.
	Name string `json:"name"`
	// Description explains what the function does.
	Description string `json:"description,omitempty"`
	// Parameters is a JSON Schema for the function parameters.
	Parameters any `json:"parameters"`
}

// ToolCall represents a tool call request from the model.
type ToolCall struct {
	// Function contains the function call details.
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the details of a function call.
type FunctionCall struct {
	// Name is the name of the function to call.
	Name string `json:"name"`
	// Arguments is the JSON object of arguments to pass.
	Arguments map[string]any `json:"arguments"`
}
