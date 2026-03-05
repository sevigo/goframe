package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("Initializing Kokoro TTS (local Docker container)...")
	fmt.Println("Make sure Kokoro is running: docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest")
	fmt.Println()

	synthesizer, err := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithVoice("af_bella"),
		openai.WithFormat("wav"),
	)
	if err != nil {
		log.Fatalf("Failed to create synthesizer: %v", err)
	}

	fmt.Println("Synthesizing speech...")
	fmt.Println()

	text := "Hello! This is a demonstration of the Kokoro text to speech engine running locally via Docker. " +
		"You can use this for voice generation without any cloud API costs."

	audio, err := synthesizer.Synthesize(context.Background(), text,
		voice.WithSpeed(1.0),
	)
	if err != nil {
		log.Fatalf("Failed to synthesize: %v", err)
	}

	outputFile := "output.wav"
	err = os.WriteFile(outputFile, audio.Data, 0600)
	if err != nil {
		log.Fatalf("Failed to write audio file: %v", err)
	}

	fmt.Printf("Audio saved to %s\n", outputFile)
	fmt.Printf("Format: %s\n", audio.Format)
	fmt.Printf("Size: %d bytes\n", len(audio.Data))
}
