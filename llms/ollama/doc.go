// Package ollama provides a client for interacting with Ollama's local LLM server.
//
// The package implements the llms.Model interface, allowing seamless integration
// with the goframe framework for text generation, embeddings, and multimodal content.
//
// # Basic Usage
//
// Create a new LLM client and generate text:
//
//	llm, err := ollama.New(ollama.WithModel("llama3"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	resp, err := llm.GenerateContent(ctx, []schema.MessageContent{
//	    schema.NewSystemMessage("You are a helpful assistant."),
//	    schema.NewHumanMessage("Hello!"),
//	})
//
// # Vision (Image Input)
//
// For vision-capable models (e.g., llava, gemma3, qwen3.5), send images using
// NewHumanMessageWithImage. The image data must be base64-encoded:
//
//	// Read and encode an image file
//	imageBytes, _ := os.ReadFile("image.png")
//	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)
//
//	// Create a message with text and image
//	messages := []schema.MessageContent{
//	    schema.NewSystemMessage("You are a helpful assistant that describes images."),
//	    schema.NewHumanMessageWithImage("What do you see in this image?", imageBase64, "image/png"),
//	}
//
//	resp, err := llm.GenerateContent(ctx, messages)
//
// # Tool Calls (Function Calling)
//
// Define tools and let the model call them:
//
//	tools := []llms.ToolDefinition{
//	    {
//	        Type: "function",
//	        Function: llms.FunctionDefinition{
//	            Name:        "get_weather",
//	            Description: "Get the current weather for a location",
//	            Parameters: map[string]any{
//	                "type": "object",
//	                "properties": map[string]any{
//	                    "location": map[string]any{
//	                        "type":        "string",
//	                        "description": "The city and state, e.g. San Francisco, CA",
//	                    },
//	                },
//	                "required": []string{"location"},
//	            },
//	        },
//	    },
//	}
//
//	opts := []llms.CallOption{
//	    llms.WithTools(tools),
//	}
//
//	resp, err := llm.GenerateContent(ctx, messages, opts...)
//
//	// Check for tool calls in the response
//	if toolCalls, ok := resp.Choices[0].GenerationInfo["ToolCalls"].([]llms.ToolCall); ok {
//	    for _, tc := range toolCalls {
//	        fmt.Printf("Tool: %s, Args: %v\n", tc.Function.Name, tc.Function.Arguments)
//	        // Execute the tool and add result to conversation...
//	    }
//	}
//
// # Embeddings
//
// Generate embeddings for text:
//
//	embeddings, err := llm.EmbedDocuments(ctx, []string{"hello", "world"})
//
// # Configuration Options
//
// Configure the client with various options:
//
//	llm, err := ollama.New(
//	    ollama.WithModel("llama3"),
//	    ollama.WithServerURL("http://localhost:11434"),
//	    ollama.WithAPIKey("your-api-key"),        // For Ollama cloud
//	    ollama.WithKeepAlive("10m"),              // Keep model loaded for 10 minutes
//	    ollama.WithRetryAttempts(3),              // Retry on transient errors
//	)
//
// # Supported Models
//
// For image support, use vision-capable models:
//   - llava
//   - gemma3
//   - qwen3.5 (cloud)
//   - qwen3-vl
//
// For tool calling, use models that support function calling:
//   - llama3.2+
//   - qwen2.5+
//   - gemma3
//
// # Error Handling
//
// The client automatically retries on transient errors (network timeouts,
// connection resets). Configure retry behavior with WithRetryAttempts and
// WithRetryDelay options.
package ollama
