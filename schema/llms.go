package schema

// ContentResponse represents the response from an LLM generation call.
type ContentResponse struct {
	// Choices contains the generated content choices.
	Choices []*ContentChoice
}

// ContentChoice represents a single generated response choice.
type ContentChoice struct {
	// Content is the generated text content.
	Content string
	// StopReason is the reason generation stopped (e.g., "stop", "length").
	StopReason string
	// GenerationInfo contains metadata about the generation.
	GenerationInfo map[string]any
	// ReasoningContent contains chain-of-thought reasoning (for models that support it).
	ReasoningContent string
}