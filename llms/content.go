package llms

type ContentResponse struct {
	Choices []*ContentChoice
}

type ContentChoice struct {
	Content          string
	StopReason       string
	GenerationInfo   map[string]any
	ReasoningContent string
}
