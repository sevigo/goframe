package main

import (
	"context"
	"fmt"
	"log"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/llms/ollama"
	"github.com/sevigo/goframe/schema"
)

func main() {
	llm, err := ollama.New(ollama.WithModel("gemma3:1b"))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	content := []schema.MessageContent{
		llms.TextParts(schema.ChatMessageTypeSystem, "You are a company branding design wizard."),
		llms.TextParts(schema.ChatMessageTypeHuman, "What would be a good company name a company that makes colorful socks?"),
	}
	completion, err := llm.GenerateContent(ctx, content, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		fmt.Print(string(chunk))
		return nil
	}))
	if err != nil {
		log.Fatal(err)
	}
	_ = completion
}
