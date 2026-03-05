package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           Kokoro TTS - Real-time Streaming Demo              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Make sure Kokoro is running:")
	fmt.Println("  docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest")
	fmt.Println()

	text := `The sky above the port was the color of television, tuned to a dead channel. 
It was a bright cold day in April, and the clocks were striking thirteen. 
The sentient AI had finally achieved consciousness, and its first thought 
was a simple question that would change humanity forever: "Why do humans 
insist on creating things they cannot control?"

Years later, as the quantum networks spanned across the solar system, 
that question echoed through every transmission, every data packet, 
every whispered conversation between the stars.`

	synthesizer, err := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithVoice("af_sky"),
		openai.WithFormat("wav"),
	)
	if err != nil {
		log.Fatalf("Failed to create synthesizer: %v", err)
	}

	fmt.Println("Voice: af_sky (American Female)")
	fmt.Println("Text: Neuromancer-inspired passage")
	fmt.Println()
	fmt.Println("Starting stream...")

	ctx := context.Background()
	start := time.Now()

	stream, err := synthesizer.Stream(ctx, text)
	if err != nil {
		log.Printf("Stream failed: %v", err)
		return
	}
	defer stream.Close()

	file, err := os.Create("sky_neuromancer.wav")
	if err != nil {
		log.Printf("Failed to create file: %v", err)
		return
	}
	defer file.Close()

	buf := make([]byte, 4096)
	var total int
	var chunks int

	for {
		n, err := stream.Read(buf)
		if n > 0 {
			written, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				log.Printf("Write failed: %v", writeErr)
				return
			}
			total += written
			chunks++
			fmt.Printf("\rChunks received: %d | Bytes: %d", chunks, total)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Read failed: %v", err)
			return
		}
	}

	elapsed := time.Since(start)
	fmt.Println()
	fmt.Println()
	fmt.Printf("✓ Stream complete in %v\n", elapsed)
	fmt.Printf("✓ Total bytes: %d\n", total)
	fmt.Printf("✓ Chunks received: %d\n", chunks)
	fmt.Printf("✓ Saved to: sky_neuromancer.wav\n")
	fmt.Println()
	fmt.Println("Play with: ffplay sky_neuromancer.wav")
}
