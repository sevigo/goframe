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
		"Maya":  "af_bella",
		"Kenji": "am_adam",
		"Alex":  "af_sky",
	})

	dialogue := []voice.DialogueSegment{
		{Speaker: "Alex", Text: "Welcome back everyone to Cities That Never Sleep. I'm Alex."},
		{Speaker: "Maya", Text: "And I'm Maya. Alex, today we're talking about one of my favorite cities in the world: Tokyo."},
		{Speaker: "Alex", Text: "Maya, I remember when you first came back from Tokyo. You wouldn't stop talking about it for weeks."},
		{Speaker: "Maya", Text: "Because it completely blew my mind, Alex. One minute you're surrounded by neon lights and giant video screens, and the next you're walking into a quiet temple that feels hundreds of years old."},
		{Speaker: "Alex", Text: "That's what fascinates me about Tokyo too. It's like the future and the past decided to share the same city."},
		{Speaker: "Kenji", Text: "That's actually a really good way to describe it, Alex."},
		{Speaker: "Alex", Text: "Kenji, glad you said that, because everyone listening should know you're originally from Tokyo."},
		{Speaker: "Kenji", Text: "That's right. I grew up there, and even for me the city never stops surprising me."},
		{Speaker: "Maya", Text: "Kenji, I have to ask you about Shibuya Crossing. The first time I saw it, I just stood there staring like a tourist."},
		{Speaker: "Kenji", Text: "Honestly Maya, even locals still pause sometimes. When the lights change and thousands of people cross at once, it's like watching a perfectly organized wave."},
		{Speaker: "Alex", Text: "And somehow nobody bumps into each other."},
		{Speaker: "Kenji", Text: "Exactly, Alex. Tokyo has this quiet rhythm to it. Everyone understands the flow."},
		{Speaker: "Maya", Text: "But Kenji, what surprised me most was how peaceful parts of Tokyo are. I expected noise everywhere."},
		{Speaker: "Kenji", Text: "Most visitors do, Maya. But if you walk just ten minutes away from the busy areas, you'll find small neighborhoods with bakeries, bicycles, and tiny parks."},
		{Speaker: "Alex", Text: "Okay Kenji, important question. Best ramen in Tokyo."},
		{Speaker: "Kenji", Text: "Alex, that's a dangerous question. People spend their entire lives arguing about that."},
		{Speaker: "Maya", Text: "I knew it. Food debates are serious business in Tokyo."},
		{Speaker: "Kenji", Text: "They really are. But that's part of the culture. Even the smallest ramen shop might have been perfecting one recipe for thirty years."},
		{Speaker: "Alex", Text: "So Maya, after everything you experienced there, what moment stuck with you the most?"},
		{Speaker: "Maya", Text: "Honestly, Alex? Early morning in Tokyo. The city waking up, shop owners opening their doors, and the smell of fresh ramen broth starting to fill the streets."},
		{Speaker: "Kenji", Text: "That's when Tokyo feels the most real."},
		{Speaker: "Alex", Text: "I think you just convinced half our listeners to book a flight."},
		{Speaker: "Maya", Text: "If they do, Alex, tell them one thing."},
		{Speaker: "Alex", Text: "What's that, Maya?"},
		{Speaker: "Maya", Text: "Don't just visit Tokyo. Wander it."},
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
