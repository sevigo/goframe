package comfyui

import "errors"

var (
	ErrNotConnected    = errors.New("comfyui: not connected to server")
	ErrPromptFailed    = errors.New("comfyui: prompt execution failed")
	ErrInvalidWorkflow = errors.New("comfyui: invalid workflow")
	ErrImageNotFound   = errors.New("comfyui: image not found")
	ErrQueueFull       = errors.New("comfyui: queue is full")
)
