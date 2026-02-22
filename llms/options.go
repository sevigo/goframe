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
	// Seed sets a deterministic seed for reproducible outputs.
	Seed int `json:"seed"`
	// Metadata contains additional provider-specific options.
	Metadata map[string]any `json:"metadata,omitempty"`
	// StreamingFunc is called for each chunk when streaming is enabled.
	StreamingFunc func(ctx context.Context, chunk []byte) error `json:"-"`
}

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
	}
}

// WithTopK specifies the top-k value to use for sampling.
func WithTopK(topK int) CallOption {
	return func(o *CallOptions) {
		o.TopK = topK
	}
}

// WithSeed specifies the seed for deterministic generation.
func WithSeed(seed int) CallOption {
	return func(o *CallOptions) {
		o.Seed = seed
	}
}

// WithModel specifies the model to use for this call.
func WithModel(model string) CallOption {
	return func(o *CallOptions) {
		o.Model = model
	}
}
