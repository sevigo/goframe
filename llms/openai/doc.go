// Package openai provides LLM and embedding support for OpenAI models.
//
// OpenAI is a family of AI models supporting chat completions, function calling,
// streaming, structured output, and embeddings.
//
// Basic usage:
//
//	llm, err := openai.New(openai.WithModel("gpt-4o"), openai.WithAPIKey("sk-..."))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	response, err := llm.GenerateContent(ctx, []schema.MessageContent{
//	    schema.NewHumanMessage("Hello!"),
//	})
//
// Streaming usage:
//
//	response, err := llm.GenerateContent(ctx, messages,
//	    llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
//	        fmt.Print(string(chunk))
//	        return nil
//	    }),
//	)
//
// Embedding usage:
//
//	embeddings, err := llm.EmbedQuery(ctx, "Hello world")
package openai
