package llms

import "context"

type CallOption func(*CallOptions)

type CallOptions struct {
	Model         string                                        `json:"model"`
	Temperature   float64                                       `json:"temperature"`
	Metadata      map[string]any                                `json:"metadata,omitempty"`
	StreamingFunc func(ctx context.Context, chunk []byte) error `json:"-"`
}

// WithStreamingFunc specifies the streaming function to use.
func WithStreamingFunc(streamingFunc func(ctx context.Context, chunk []byte) error) CallOption {
	return func(o *CallOptions) {
		o.StreamingFunc = streamingFunc
	}
}
