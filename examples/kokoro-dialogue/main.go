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
		"Case":     "am_adam",
		"Molly":    "af_bella",
		"Narrator": "af_sky",
	})

	dialogue := []voice.DialogueSegment{
		{Speaker: "Narrator", Text: "The neon lights flickered against the rain-slicked streets of Night City."},
		{Speaker: "Case", Text: "I've seen things you people wouldn't believe. Attack ships on fire off the shoulder of Orion."},
		{Speaker: "Molly", Text: "Time to die. But not today. Not when we have a job to do."},
		{Speaker: "Case", Text: "The matrix was everywhere. It was all around us. It was the world that had been pulled over our eyes."},
		{Speaker: "Molly", Text: "We're not in Kansas anymore. But then again, Kansas doesn't exist anymore either."},
		{Speaker: "Narrator", Text: "And so they walked into the digital sunset, two shadows in a world of light."},
	}

	ctx := context.Background()
	start := time.Now()

	fmt.Println("Synthesizing dialogue with 3 voices:")
	fmt.Println("  - Narrator: af_sky (American Female)")
	fmt.Println("  - Case: am_adam (American Male)")
	fmt.Println("  - Molly: af_bella (American Female)")
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
