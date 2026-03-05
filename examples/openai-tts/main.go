// Package main demonstrates OpenAI Text-to-Speech API usage.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║         OpenAI Text-to-Speech API Demonstration              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("❌ OPENAI_API_KEY environment variable is required.\n" +
			"   Get your API key from: https://platform.openai.com/api-keys")
	}

	// Create OpenAI TTS synthesizer
	synthesizer, err := openai.NewSynthesizer(
		openai.WithAPIKey(apiKey),
		openai.WithModel("tts-1"),
		openai.WithVoice("alloy"),
		openai.WithFormat("mp3"),
	)
	if err != nil {
		log.Fatalf("Failed to create synthesizer: %v", err)
	}

	fmt.Println("✓ OpenAI TTS synthesizer created")
	fmt.Println("  Model: tts-1 (standard quality, faster)")
	fmt.Println("  Voice: alloy (neutral, versatile)")
	fmt.Println("  Format: mp3")
	fmt.Println()

	// Example 1: Simple synthesis
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Example 1: Basic Text-to-Speech")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()

	text := "Hello! This is a test of OpenAI's text-to-speech API. It produces high-quality, natural-sounding audio in real-time."
	fmt.Printf("Text: %q\n\n", text)

	audio, err := synthesizer.Synthesize(ctx1, text)
	if err != nil {
		log.Fatalf("Failed to synthesize: %v", err)
	}

	fmt.Printf("✓ Audio generated successfully!\n")
	fmt.Printf("  Size: %d bytes (%.2f KB)\n", len(audio.Data), float64(len(audio.Data))/1024)
	fmt.Printf("  Format: %s\n", audio.Format)

	// Save to file
	filename := "openai_tts_output.mp3"
	if err := os.WriteFile(filename, audio.Data, 0600); err != nil {
		log.Fatalf("Failed to save audio: %v", err)
	}
	fmt.Printf("  Saved to: %s\n", filename)
	fmt.Println()

	// Example 2: Different voices
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Example 2: Testing All Available Voices")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("OpenAI provides 6 voices with different characteristics:")
	fmt.Println()
	
	voices := []struct {
		name        string
		description string
	}{
		{"alloy", "neutral, versatile"},
		{"echo", "warm, engaging"},
		{"fable", "expressive, dramatic"},
		{"onyx", "deep, authoritative"},
		{"nova", "energetic, friendly"},
		{"shimmer", "soft, gentle"},
	}

	for _, v := range voices {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
		
		fmt.Printf("➤ %s (%s): ", v.name, v.description)
		
		audio, err := synthesizer.Synthesize(ctx2, 
			fmt.Sprintf("Hello, I am %s. I have a %s tone.", v.name, v.description),
			voice.WithVoice(v.name),
		)
		cancel2()
		
		if err != nil {
			fmt.Printf("✗ error - %v\n", err)
			continue
		}
		
		filename := fmt.Sprintf("openai_voice_%s.mp3", v.name)
		os.WriteFile(filename, audio.Data, 0600)
		fmt.Printf("✓ %d bytes -> %s\n", len(audio.Data), filename)
	}
	fmt.Println()

	// Example 3: HD model for better quality
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Example 3: HD Model for Higher Quality")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("The tts-1-hd model provides higher quality audio at a higher cost.")
	fmt.Println()

	ctx3, cancel3 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel3()

	hdSynthesizer, err := openai.NewSynthesizer(
		openai.WithAPIKey(apiKey),
		openai.WithModel("tts-1-hd"),
		openai.WithVoice("nova"),
		openai.WithFormat("mp3"),
	)
	if err != nil {
		log.Fatalf("Failed to create HD synthesizer: %v", err)
	}

	audioHD, err := hdSynthesizer.Synthesize(ctx3, "This is the HD model with even better audio quality. You'll notice clearer pronunciation and more natural prosody.")
	if err != nil {
		log.Printf("HD synthesis failed: %v", err)
	} else {
		filename := "openai_hd_output.mp3"
		os.WriteFile(filename, audioHD.Data, 0600)
		fmt.Printf("✓ HD model: %d bytes -> %s\n", len(audioHD.Data), filename)
		fmt.Println("  Model: tts-1-hd (high quality, slower)")
		fmt.Println("  Voice: nova (energetic, friendly)")
	}
	fmt.Println()

	// Example 4: Streaming synthesis
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Example 4: Streaming Synthesis")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Streaming reduces latency for longer content by sending audio as it's generated.")
	fmt.Println()

	ctx4, cancel4 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel4()

	streamText := "This example demonstrates streaming synthesis. Instead of waiting for the entire audio to be generated, the API streams chunks as they become available. This is particularly useful for longer texts where you want to start playing audio sooner, reducing perceived latency for the end user."
	fmt.Printf("Text: %q\n\n", streamText)

	stream, err := synthesizer.Stream(ctx4, streamText)
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}
	defer stream.Close()

	data, err := os.Create("openai_streamed.mp3")
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer data.Close()

	written, err := data.ReadFrom(stream)
	if err != nil {
		log.Fatalf("Failed to read stream: %v", err)
	}

	fmt.Printf("✓ Streamed: %.2f KB -> openai_streamed.mp3\n", float64(written)/1024)
	fmt.Println()

	// Example 5: Different audio formats
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Example 5: Audio Format Options")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("OpenAI supports multiple audio formats for different use cases:")
	fmt.Println()

	formats := []struct {
		name        string
		description string
	}{
		{"mp3", "most compatible, good compression"},
		{"opus", "best for streaming, smallest size"},
		{"aac", "good for mobile, efficient"},
		{"flac", "lossless, largest size"},
	}

	for _, f := range formats {
		ctx5, cancel5 := context.WithTimeout(context.Background(), 30*time.Second)
		
		formatSynth, err := openai.NewSynthesizer(
			openai.WithAPIKey(apiKey),
			openai.WithModel("tts-1"),
			openai.WithVoice("alloy"),
			openai.WithFormat(f.name),
		)
		if err != nil {
			cancel5()
			continue
		}
		
		fmt.Printf("➤ %s (%s): ", f.name, f.description)
		
		audio, err := formatSynth.Synthesize(ctx5, "Testing different audio formats.")
		cancel5()
		
		if err != nil {
			fmt.Printf("✗ error - %v\n", err)
			continue
		}
		
		filename := fmt.Sprintf("openai_format_%s.%s", f.name, f.name)
		os.WriteFile(filename, audio.Data, 0600)
		fmt.Printf("✓ %.2f KB -> %s\n", float64(len(audio.Data))/1024, filename)
	}
	fmt.Println()

	// Summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Summary")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("OpenAI TTS API Features:")
	fmt.Println("  ✓ 6 high-quality voices (alloy, echo, fable, onyx, nova, shimmer)")
	fmt.Println("  ✓ 2 models (tts-1 for speed, tts-1-hd for quality)")
	fmt.Println("  ✓ Multiple formats (mp3, opus, aac, flac, wav, pcm)")
	fmt.Println("  ✓ Streaming support for reduced latency")
	fmt.Println("  ✓ Speed control (0.25x to 4.0x)")
	fmt.Println()
	fmt.Println("Generated Files:")
	fmt.Println("  • openai_tts_output.mp3 - Basic example")
	fmt.Println("  • openai_voice_*.mp3 - All 6 voice variations")
	fmt.Println("  • openai_hd_output.mp3 - HD quality example")
	fmt.Println("  • openai_streamed.mp3 - Streaming example")
	fmt.Println("  • openai_format_*.mp3 - Different audio formats")
	fmt.Println()
	fmt.Println("✓ All examples completed successfully!")
	fmt.Println()
	fmt.Println("💡 Tips:")
	fmt.Println("  • Use tts-1 for real-time applications (faster)")
	fmt.Println("  • Use tts-1-hd for pre-recorded content (better quality)")
	fmt.Println("  • Use streaming for texts longer than 1000 characters")
	fmt.Println("  • Use opus format for web streaming (best compression)")
	fmt.Println()
}
