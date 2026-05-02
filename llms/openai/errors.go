package openai

import "errors"

var (
	// ErrNoAPIKey indicates that an API key was not provided during initialization.
	ErrNoAPIKey = errors.New("openai: API key is required")
	// ErrNoChoices indicates that the API response contained no completion choices.
	ErrNoChoices = errors.New("openai: no choices in response")
	// ErrEmbeddings indicates a failure during embedding generation.
	ErrEmbeddings = errors.New("openai: failed to generate embeddings")
)
