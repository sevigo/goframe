package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/llms/gemini"
	"github.com/sevigo/goframe/schema"
)

func main() {
	// The Gemini client will automatically look for the GEMINI_API_KEY environment variable.
	// You can also set it with gemini.WithAPIKey("your-key")
	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Fatal("Please set the GEMINI_API_KEY environment variable.")
	}

	ctx := context.Background()

	//  Simple Non-Streaming Chat
	fmt.Println("--- Simple Chat Example ---")
	llm, err := gemini.New(ctx, gemini.WithModel("gemini-2.5-flash"))
	if err != nil {
		log.Fatal(err)
	}

	simplePrompt := "What is the largest animal on Earth?"
	fmt.Printf("You: %s\n", simplePrompt)

	answer, err := llm.Call(ctx, simplePrompt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Gemini: %s\n\n", answer)

	// Streaming Chat with History
	fmt.Println("--- Streaming Chat Example ---")

	// Create a message history
	history := []schema.MessageContent{
		llms.TextParts(schema.ChatMessageTypeSystem, "You are a friendly and helpful assistant who loves to talk about space."),
		llms.TextParts(schema.ChatMessageTypeHuman, "What is the closest star to Earth?"),
		llms.TextParts(schema.ChatMessageTypeAI, "The closest star to Earth is the Sun!"),
		llms.TextParts(schema.ChatMessageTypeHuman, "Okay, besides that one, what is the next closest?"),
	}

	fmt.Printf("You: %s\n", history[len(history)-1].GetTextContent())
	fmt.Print("Gemini: ")

	// Generate a streaming response
	completion, err := llm.GenerateContent(ctx, history, llms.WithStreamingFunc(
		func(ctx context.Context, chunk []byte) error {
			fmt.Print(string(chunk))
			return nil
		},
	))
	if err != nil {
		log.Fatal(err)
	}

	// The final completion object contains metadata
	fmt.Println("\n---")
	fmt.Printf("Stream finished. Generation Info: %v\n", completion.Choices[0].GenerationInfo)
}
