# Ollama Chat Example

A simple demonstration of conversational AI using Ollama with the lightweight Gemma 3:1B model via the Goframe framework.

## Features

💬 Interactive Chat - Direct conversation with local LLM
🚀 Lightweight Model - Fast responses with Gemma 3:1B (1 billion parameters)
📊 Performance Metrics - Response time logging and model info
🔧 Simple Setup - Minimal configuration required

## Prerequisites

* Go 1.21+
* Ollama running locally with gemma3:1b model

## Quick Setup
```bash
# Install Ollama and pull the model
ollama pull gemma3:1b
# Run the chat example
go run examples/ollama-chat-example/main.go
```

## What It Demonstrates

1. Basic LLM Integration
```go
// Initialize Ollama with lightweight model
llm, err := ollama.New(
    ollama.WithModel("gemma3:1b"),
    ollama.WithLogger(logger),
)

// Simple chat interaction
response, err := llm.Call(ctx, "Your question here")
```
