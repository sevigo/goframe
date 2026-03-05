package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/sevigo/goframe/voice/openai"
)

type VoiceConfig struct {
	Name        string
	Description string
}

var kokoroVoices = []VoiceConfig{
	{Name: "af_bella", Description: "American Female - Bella"},
	{Name: "af_sarah", Description: "American Female - Sarah"},
	{Name: "af_sky", Description: "American Female - Sky"},
	{Name: "am_adam", Description: "American Male - Adam"},
	{Name: "am_michael", Description: "American Male - Michael"},
	{Name: "bf_emma", Description: "British Female - Emma"},
	{Name: "bf_isabella", Description: "British Female - Isabella"},
	{Name: "bm_george", Description: "British Male - George"},
	{Name: "bm_lewis", Description: "British Male - Lewis"},
}

var scifiPassage = `In the beginning the Universe was created. This has made a lot of people very angry 
and been widely regarded as a bad move. Many races believe that it was created by some sort of god, 
though the Jatravartid people of Viltvodle VI believe that the entire Universe was in fact sneezed 
out of the nose of a being called the Great Green Arkleseizure.

The Jatravartids, who live in perpetual fear of the time they call The Coming of the Great White 
Handkerchief, are small blue creatures with more than fifty arms each, who are therefore unique 
in being the only race in history to have invented the aerosol deodorant before the wheel.`

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          Kokoro TTS - Streaming Audio Generation             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Make sure Kokoro is running:")
	fmt.Println("  docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest")
	fmt.Println()

	outputDir := "audio_output"
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	for _, vc := range kokoroVoices {
		if err := generateVoice(context.Background(), vc, outputDir); err != nil {
			log.Printf("Failed to generate voice %s: %v", vc.Name, err)
			continue
		}
	}

	fmt.Println()
	fmt.Printf("✓ All audio files saved to: %s/\n", outputDir)
}

func generateVoice(ctx context.Context, vc VoiceConfig, outputDir string) error {
	fmt.Printf("Generating [%s] %s...\n", vc.Name, vc.Description)

	synthesizer, err := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithVoice(vc.Name),
		openai.WithFormat("wav"),
	)
	if err != nil {
		return fmt.Errorf("failed to create synthesizer: %w", err)
	}

	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.wav", vc.Name))

	stream, err := synthesizer.Stream(ctx, scifiPassage)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}
	defer stream.Close()

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, stream)
	if err != nil {
		return fmt.Errorf("failed to write audio: %w", err)
	}

	fmt.Printf("  ✓ Saved %s (%d bytes)\n", outputPath, written)
	return nil
}
