// Package comfyui provides a Go client for ComfyUI's HTTP and WebSocket APIs.
//
// It enables programmatic image generation via ComfyUI workflows,
// with real-time progress streaming, image upload/download, and
// workflow construction helpers.
//
// # Quick Start
//
//	client, err := comfyui.New(comfyui.WithHost("127.0.0.1:8188"))
//
//	workflow := comfyui.NewWorkflow()
//	prompt := workflow.AddNode("KSampler", 3).
//		SetInput("prompt", "a cat in a garden").
//		SetInput("width", 512).
//		SetInput("height", 512)
//	workflow.AddNode("CheckpointLoaderSimple", 4).
//		SetInput("ckpt_name", "model.safetensors").
//		Connect("MODEL", prompt.ID, "model").
//		Connect("CLIP", prompt.ID, "clip")
//
//	result, err := client.Generate(ctx, workflow)
package comfyui
