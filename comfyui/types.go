package comfyui

import "time"

type ProgressEvent struct {
	Step     int     `json:"step"`
	MaxSteps int     `json:"max_steps"`
	Value    float64 `json:"value"`
	Max      float64 `json:"max"`
	Phase    string  `json:"phase,omitempty"`
	PromptID string  `json:"prompt_id,omitempty"`
	NodeID   int     `json:"node_id,omitempty"`
}

type ExecutionStatus struct {
	PromptID string `json:"prompt_id"`
	Status   string `json:"status"` // "success" or "error"
}

type PromptRequest struct {
	Prompt   map[string]any `json:"prompt"`
	ClientID string         `json:"client_id"`
}

type PromptResponse struct {
	PromptID string        `json:"prompt_id"`
	Number   int           `json:"number"`
	Errors   []PromptError `json:"errors,omitempty"`
}

type PromptError struct {
	Message   string         `json:"message"`
	Exception map[string]any `json:"exception,omitempty"`
	NodeID    int            `json:"node_id,omitempty"`
	NodeType  string         `json:"node_type,omitempty"`
}

type QueuedPrompt struct {
	PromptID string `json:"prompt_id"`
	Number   int    `json:"number"`
}

type QueueInfo struct {
	Running []QueuedPrompt `json:"running"`
	Pending []QueuedPrompt `json:"pending"`
}

type HistoryEntry struct {
	Prompt    map[string]any `json:"prompt"`
	Outputs   map[string]any `json:"outputs"`
	Status    map[string]any `json:"status"`
	Timestamp int64          `json:"timestamp"`
}

type ImageResult struct {
	PromptID  string
	Filenames []string
	Images    [][]byte
	Duration  time.Duration
}

type UploadResponse struct {
	Name      string `json:"name"`
	SubFolder string `json:"subfolder"`
	Type      string `json:"type"`
}

type SystemStats struct {
	System struct {
		CPU  float64 `json:"cpu"`
		RAM  float64 `json:"ram"`
		Disk float64 `json:"disk"`
	} `json:"system"`
	Devices []struct {
		Name      string  `json:"name"`
		VRAMUsed  float64 `json:"vram_used"`
		VRAMTotal float64 `json:"vram_total"`
	} `json:"devices"`
}
