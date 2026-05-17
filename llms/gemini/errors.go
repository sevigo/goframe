package gemini

import "errors"

var (
	ErrNoAPIKey      = errors.New("gemini: API key is required")
	ErrInvalidModel  = errors.New("gemini: invalid model specified")
	ErrNoContent     = errors.New("gemini: no content generated")
	ErrSystemMessage = errors.New("gemini: system message must be the first message in the conversation")
	ErrEmbeddings    = errors.New("gemini: failed to generate embeddings")
	ErrNoMessages    = errors.New("gemini: no messages to send")
)
