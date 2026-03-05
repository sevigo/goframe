package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          Kokoro TTS - Dialogue Synthesis Demo                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Make sure Kokoro is running:")
	fmt.Println("  docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest")
	fmt.Println()

	if err := run(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	synthesizer, err := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithFormat("wav"),
	)
	if err != nil {
		return fmt.Errorf("failed to create synthesizer: %w", err)
	}

	dialogueSyn := voice.NewDialogueSynthesizer(synthesizer, map[string]string{
		"Sarah":    "af_bella",
		"James":    "am_adam",
		"Narrator": "af_sky",
	})

	dialogue := []voice.DialogueSegment{
		{Speaker: "Narrator", Text: "In a quiet coffee shop on a rainy afternoon, two old friends reunited after years apart."},
		{Speaker: "Sarah", Text: "James! I can't believe it's really you. It's been what, five years?"},
		{Speaker: "James", Text: "Six, actually. You look wonderful, Sarah. Time has been kind to you."},
		{Speaker: "Sarah", Text: "Flatterer. But tell me, how have you been? The last I heard, you were off to Tokyo."},
		{Speaker: "James", Text: "That expedition changed everything. The things I discovered there, the people I met. I learned that home isn't a place, it's the people you share it with."},
		{Speaker: "Sarah", Text: "That's beautiful, James. You always had a way with words. So, what brings you back?"},
		{Speaker: "James", Text: "I realized something important while I was away. There was unfinished business here. A conversation I never got to finish."},
		{Speaker: "Sarah", Text: "You mean that day at the airport? James, I thought you'd forgotten."},
		{Speaker: "James", Text: "I never forgot. Sarah, I came back to tell you what I should have said six years ago."},
		{Speaker: "Narrator", Text: "As the rain continued to fall outside, the coffee shop grew warmer, and two hearts found their way back to each other."},
	}

	ctx := context.Background()
	start := time.Now()

	fmt.Println("Synthesizing dialogue with 3 voices:")
	fmt.Println("  - Narrator: af_sky (American Female)")
	fmt.Println("  - Sarah: af_bella (American Female)")
	fmt.Println("  - James: am_adam (American Male)")
	fmt.Println()

	stream, err := dialogueSyn.StreamDialogue(ctx, dialogue)
	if err != nil {
		return fmt.Errorf("failed to stream dialogue: %w", err)
	}
	defer stream.Close()

	file, err := os.Create("dialogue.wav")
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, stream)
	if err != nil {
		return fmt.Errorf("failed to write dialogue: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("✓ Dialogue synthesized in %v\n", elapsed)
	fmt.Printf("✓ Total bytes: %d\n", written)
	fmt.Printf("✓ Saved to: dialogue.wav\n")
	fmt.Println()
	fmt.Println("Play with: ffplay dialogue.wav")

	return nil
}
