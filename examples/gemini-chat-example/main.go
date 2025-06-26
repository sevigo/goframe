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
	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Fatal("Please set the GEMINI_API_KEY environment variable.")
	}

	ctx := context.Background()

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

	fmt.Println("--- Streaming Chat Example ---")

	history := []schema.MessageContent{
		llms.TextParts(schema.ChatMessageTypeSystem, "You are a friendly and helpful assistant who loves to talk about space."),
		llms.TextParts(schema.ChatMessageTypeHuman, "What is the closest star to Earth?"),
		llms.TextParts(schema.ChatMessageTypeAI, "The closest star to Earth is the Sun!"),
		llms.TextParts(schema.ChatMessageTypeHuman, "Okay, besides that one, what is the next closest?"),
	}

	fmt.Printf("You: %s\n", history[len(history)-1].GetTextContent())
	fmt.Print("Gemini: ")

	completion, err := llm.GenerateContent(ctx, history, llms.WithStreamingFunc(
		func(ctx context.Context, chunk []byte) error {
			fmt.Print(string(chunk))
			return nil
		},
	))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n---")
	fmt.Printf("Stream finished. Generation Info: %v\n", completion.Choices[0].GenerationInfo)
}
