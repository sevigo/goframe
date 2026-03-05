package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	synth, _ := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithFormat("wav"),
	)

	ds, _ := voice.NewDialogueSynthesizerCaptioned(synth, map[string]string{
		"Alex": "af_sky",
		"Bob":  "am_adam",
	})

	// Short test with just 3 segments
	dialogue := []voice.DialogueSegment{
		{Speaker: "Alex", Text: "Hello!"},
		{Speaker: "Bob", Text: "Hi there!"},
		{Speaker: "Alex", Text: "How are you?"},
	}

	ctx := context.Background()
	start := time.Now()
	
	result, err := ds.SynthesizeDialogueCaptioned(ctx, dialogue)
	if err != nil {
		log.Fatal("Error:", err)
	}

	fmt.Printf("✓ Success in %v\n", time.Since(start))
	fmt.Printf("Segments: %d, Duration: %dms\n", len(result.Segments), result.TotalDurationMs)
	for i, seg := range result.Segments {
		fmt.Printf("  [%d] %s: %dms (%d words)\n", i+1, seg.Speaker, seg.DurationMs, len(seg.Timestamps))
	}
}
