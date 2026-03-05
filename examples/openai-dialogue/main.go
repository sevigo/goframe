package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║    OpenAI Dialogue - Realistic Conversation (5 Segments)     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("❌ OPENAI_API_KEY required")
	}

	synth, err := openai.NewSynthesizer(
		openai.WithAPIKey(apiKey),
		openai.WithModel("tts-1"),
		openai.WithVoice("alloy"),
		openai.WithFormat("wav"),
	)
	if err != nil {
		log.Fatalf("Failed: %v", err)
	}

	ds := voice.NewDialogueSynthesizer(synth, map[string]string{
		"Alice": "alloy", // Host
		"Bob":   "echo",  // Co-host
	})

	// Final settings (tested and working)
	ds.PauseMsMin = 200       // Long pauses for clear separation
	ds.PauseMsMax = 400       // Realistic conversation rhythm
	ds.CrossfadeMs = 0        // No blending to prevent overlap
	ds.NormalizeVolume = true // Consistent volume

	fmt.Println("✓ Optimized Settings (From Testing):")
	fmt.Println("  Crossfade: 0ms (clean cuts, no overlap)")
	fmt.Println("  Context-aware: Questions get 1.3x pause")
	fmt.Println()

	// REALISTIC DIALOGUE - Addressing each other naturally
	dialogue := []voice.DialogueSegment{
		{Speaker: "Alice", Text: "Hey Bob, welcome back to Tech Talk!"},
		{Speaker: "Bob", Text: "Thanks Alice! Great to be here again."},
		{Speaker: "Alice", Text: "So Bob, what have you been working on lately?"},
		{Speaker: "Bob", Text: "I've been exploring machine learning applications in healthcare, Alice."},
		{Speaker: "Alice", Text: "That sounds fascinating! Tell me more about it."},
	}

	fmt.Println()
	for i, seg := range dialogue {
		fmt.Printf("%d. [%s]: \"%s\"\n", i+1, seg.Speaker, seg.Text)
	}
	fmt.Println()
	fmt.Println("  • Alice (host) - asking questions, guiding conversation")
	fmt.Println("  • Bob (guest) - responding thoughtfully")
	fmt.Println()

	ctx := context.Background()
	filename := "dialogue_realistic.wav"

	stream, err := ds.StreamDialogue(ctx, dialogue)
	if err != nil {
		log.Fatalf("Failed: %v", err)
	}

	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}

	written, err := io.Copy(file, stream)
	file.Close()
	stream.Close()

	if err != nil {
		log.Fatalf("Failed to write: %v", err)
	}

	fmt.Printf("✓ Success! %.2f KB -> %s\n\n", float64(written)/1024, filename)
	fmt.Printf("🎵 afplay %s\n", filename)
}
