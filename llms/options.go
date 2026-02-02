package llms

import "context"

type CallOption func(*CallOptions)

type CallOptions struct {
	Model         string                                        `json:"model"`
	Temperature   float64                                       `json:"temperature"`
	MaxTokens     int                                           `json:"max_tokens"`
	StopWords     []string                                      `json:"stop_words"`
	TopP          float64                                       `json:"top_p"`
	TopK          int                                           `json:"top_k"`
	Seed          int                                           `json:"seed"`
	Metadata      map[string]any                                `json:"metadata,omitempty"`
	StreamingFunc func(ctx context.Context, chunk []byte) error `json:"-"`
}

// WithStreamingFunc specifies the streaming function to use.
func WithStreamingFunc(streamingFunc func(ctx context.Context, chunk []byte) error) CallOption {
	return func(o *CallOptions) {
		o.StreamingFunc = streamingFunc
	}
}

// WithTemperature specifies the temperature to use.
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
func WithStopWords(stopWords []string) CallOption {
	return func(o *CallOptions) {
		o.StopWords = stopWords
	}
}

// WithTopP specifies the top-p value to use.
func WithTopP(topP float64) CallOption {
	return func(o *CallOptions) {
		o.TopP = topP
	}
}

// WithTopK specifies the top-k value to use.
func WithTopK(topK int) CallOption {
	return func(o *CallOptions) {
		o.TopK = topK
	}
}

// WithSeed specifies the seed to use.
func WithSeed(seed int) CallOption {
	return func(o *CallOptions) {
		o.Seed = seed
	}
}

// WithModel specifies the model to use.
func WithModel(model string) CallOption {
	return func(o *CallOptions) {
		o.Model = model
	}
}
