//go:build ignore

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	llm, err := ollama.New(
		ollama.WithModel("qwen3.5:cloud"),
	)
	if err != nil {
		slog.Error("Failed to create Ollama LLM", "error", err)
		os.Exit(1)
	}

	imageBytes, err := os.ReadFile("image.png")
	if err != nil {
		slog.Error("Failed to read test image", "error", err)
		os.Exit(1)
	}
	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)
	slog.Info("Loaded image", "bytes", len(imageBytes), "base64_len", len(imageBase64))

	messages := []schema.MessageContent{
		schema.NewSystemMessage("You are a helpful assistant that describes images."),
		schema.NewHumanMessageWithImage("What do you see in this image? Describe it in detail.", imageBase64, "image/png"),
	}

	slog.Info("Sending image to model...")
	resp, err := llm.GenerateContent(ctx, messages)
	if err != nil {
		slog.Error("Failed to generate content", "error", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Model Response ===")
	fmt.Println(resp.Choices[0].Content)
}
