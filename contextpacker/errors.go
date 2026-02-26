package contextpacker

import "errors"

var (
	// ErrNilTokenizer is returned when a nil tokenizer is provided.
	ErrNilTokenizer = errors.New("tokenizer cannot be nil")
	// ErrInvalidMaxTokens is returned when maxTokens is less than or equal to zero.
	ErrInvalidMaxTokens = errors.New("maxTokens must be greater than zero")
	// ErrTokenCountFailed is returned when token counting fails.
	ErrTokenCountFailed = errors.New("failed to count tokens")
	// ErrTemplateParse is returned when template parsing fails.
	ErrTemplateParse = errors.New("failed to parse template")
	// ErrTemplateExecute is returned when template execution fails.
	ErrTemplateExecute = errors.New("failed to execute template")
)
