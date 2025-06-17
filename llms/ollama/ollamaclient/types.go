package ollamaclient

import (
	"fmt"
	"time"
)

// StatusError represents an error response from the Ollama API.
type StatusError struct {
	Status       string `json:"status,omitempty"`
	ErrorMessage string `json:"error"`
	StatusCode   int    `json:"code,omitempty"`
}

// Error implements the error interface for StatusError.
func (e StatusError) Error() string {
	switch {
	case e.Status != "" && e.ErrorMessage != "":
		return fmt.Sprintf("%s: %s", e.Status, e.ErrorMessage)
	case e.Status != "":
		return e.Status
	case e.ErrorMessage != "":
		return e.ErrorMessage
	default:
		return "something went wrong, please see the ollama server logs for details"
	}
}

// GenerateRequest represents a request to the /api/generate endpoint.
type GenerateRequest struct {
	Model     string  `json:"model"`
	Prompt    string  `json:"prompt"`
	System    string  `json:"system,omitempty"`
	Template  string  `json:"template,omitempty"`
	Context   []int   `json:"context,omitempty"`
	Stream    *bool   `json:"stream,omitempty"`
	KeepAlive string  `json:"keep_alive,omitempty"`
	Options   Options `json:"options,omitempty"`
}

// GenerateResponse represents a response from the /api/generate endpoint.
type GenerateResponse struct {
	CreatedAt          time.Time     `json:"created_at"`
	Model              string        `json:"model"`
	Response           string        `json:"response"`
	Context            []int         `json:"context,omitempty"`
	TotalDuration      time.Duration `json:"total_duration,omitempty"`
	LoadDuration       time.Duration `json:"load_duration,omitempty"`
	PromptEvalCount    int           `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration time.Duration `json:"prompt_eval_duration,omitempty"`
	EvalCount          int           `json:"eval_count,omitempty"`
	EvalDuration       time.Duration `json:"eval_duration,omitempty"`
	Done               bool          `json:"done"`
}

// EmbeddingRequest represents a request to the /api/embeddings endpoint.
type EmbeddingRequest struct {
	Model     string  `json:"model"`
	Prompt    string  `json:"prompt"`
	Options   Options `json:"options,omitempty"`
	KeepAlive string  `json:"keep_alive,omitempty"`
}

// EmbeddingResponse represents a response from the /api/embeddings endpoint.
type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

// PullRequest represents a request to the /api/pull endpoint.
type PullRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream,omitempty"`
}

// Options contains configuration parameters for generation requests.
type Options struct {
	Stop             []string `json:"stop,omitempty"`
	RepeatLastN      int      `json:"repeat_last_n,omitempty"`
	Seed             int      `json:"seed,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
	NumKeep          int      `json:"num_keep,omitempty"`
	Mirostat         int      `json:"mirostat,omitempty"`
	NumPredict       int      `json:"num_predict,omitempty"`
	Temperature      float32  `json:"temperature,omitempty"`
	TypicalP         float32  `json:"typical_p,omitempty"`
	RepeatPenalty    float32  `json:"repeat_penalty,omitempty"`
	PresencePenalty  float32  `json:"presence_penalty,omitempty"`
	FrequencyPenalty float32  `json:"frequency_penalty,omitempty"`
	TFSZ             float32  `json:"tfs_z,omitempty"`
	MirostatTau      float32  `json:"mirostat_tau,omitempty"`
	MirostatEta      float32  `json:"mirostat_eta,omitempty"`
	TopP             float32  `json:"top_p,omitempty"`
	PenalizeNewline  bool     `json:"penalize_newline,omitempty"`
}
