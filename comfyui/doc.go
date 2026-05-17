// Package comfyui provides a Go client for ComfyUI's HTTP and WebSocket APIs.
//
// It enables programmatic image generation via ComfyUI workflows,
// with real-time progress streaming, image upload/download, and
// workflow construction helpers.
//
// # Quick Start
//
//	client, err := comfyui.New(comfyui.WithHost("127.0.0.1:8188"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	workflow := comfyui.NewWorkflow()
//	prompt := workflow.AddNode("KSampler", 3).
//	    SetInput("prompt", "a cat in a garden").
//	    SetInput("width", 512).
//	    SetInput("height", 512)
//	workflow.AddNode("CheckpointLoaderSimple", 4).
//	    SetInput("ckpt_name", "model.safetensors").
//	    Connect("MODEL", prompt.ID, "model").
//	    Connect("CLIP", prompt.ID, "clip")
//
//	result, err := client.Generate(ctx, workflow)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for i, img := range result.Images {
//	    os.WriteFile(fmt.Sprintf("output_%d.png", i), img, 0o644)
//	}
//
// # Async Generation with Progress
//
//	progressCh, promptID, err := client.GenerateAsync(ctx, workflow)
//	for event := range progressCh {
//	    log.Printf("[%s] step %d/%d", event.Phase, event.Step, event.MaxSteps)
//	}
//
// # API Methods
//
// Queue a workflow and poll for results:
//
//	QueuePrompt, GetHistory, GetImage, UploadImage, GetQueue, Interrupt
//
// Stream progress via WebSocket:
//
//	StreamProgress returns a channel of ProgressEvent
package comfyui
