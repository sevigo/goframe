# Agent Package

The `agent` package provides an abstraction layer for managing communication with OpenCode in agent mode, with a focus on MCP (Model Context Protocol) server configuration.

## Overview

This package enables programmatic control of AI agents through the OpenCode SDK, providing structured configuration for:

- **MCP Servers** - Local (stdio) and remote (HTTP/SSE) server configuration
- **Permissions** - Fine-grained control over agent capabilities
- **Sessions** - Conversation context management
- **Events** - Asynchronous response handling
- **Prompts** - Structured message construction

## Installation

```bash
go get github.com/sevigo/goframe/agent
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/sevigo/goframe/agent"
)

func main() {
    ctx := context.Background()

    // Configure MCP servers
    mcpRegistry := agent.NewMCPRegistry(
        agent.LocalMCPServer("filesystem",
            []string{"mcp-filesystem", "/path/to/repo"},
            agent.WithEnv(map[string]string{"LOG_LEVEL": "debug"}),
        ),
        agent.RemoteMCPServer("brave-search",
            "https://mcp.brave.com/search",
            agent.WithHeaders(map[string]string{"Authorization": "Bearer token"}),
        ),
    )

    // Configure permissions
    permissions := agent.NewPermissions().
        AllowBash("go test", "go build").
        AllowEdit().
        DenyWebfetch().
        Build()

    // Create agent
    ag, err := agent.New(
        agent.WithModel("ollama/llama3"),
        agent.WithMCPRegistry(mcpRegistry),
        agent.WithPermissions(permissions),
        agent.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
    )
    if err != nil {
        panic(err)
    }

    // Create session
    session, err := ag.NewSession(ctx, agent.WithTitle("Code Review"))
    if err != nil {
        panic(err)
    }
    defer ag.DeleteSession(ctx, session.ID)

    // Send prompt
    response, err := session.Prompt(ctx, "Explain this code")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Response: %s\n", response.Content)
}
```

## Testing Locally

### Prerequisites

- Docker and Docker Compose
- Go 1.21+
- (Optional) API key for cloud models

### Step 1: Start Docker Services

```bash
# Start all services (Qdrant, Ollama, OpenCode)
docker-compose up -d

# Check services are running
docker-compose ps
```

Expected output:
```
NAME        IMAGE                              STATUS
qdrant      qdrant/qdrant:v1.16.0              running
ollama      ollama/ollama:latest               running
opencode    ghcr.io/anomalyco/opencode:latest  running
```

### Step 2: Pull Ollama Models

For local inference, pull a model:

```bash
# Pull Qwen 3.5 or qwen2.5-coder:3b
docker-compose exec ollama ollama pull qwen3.5:2b
# docker-compose exec ollama ollama pull qwen2.5-coder:3b

# Pull cloud version of GLM-5
docker-compose exec ollama ollama pull glm-5:cloud

# List available models
docker-compose exec ollama ollama list
```

### Step 3: Set Environment Variables

```bash
# OpenCode server URL (default: http://localhost:3000)
export OPENCODE_BASE_URL=http://localhost:3000

# For cloud models (optional)
export OPENCODE_API_KEY=your-api-key

# Model to use
export OPENCODE_MODEL=ollama/qwen3.5:2b
# export OPENCODE_MODEL=ollama/qwen2.5-coder:3b
# export OPENCODE_MODEL=ollama/glm-5:cloud
```

### Step 4: Run Tests

```bash
# Run all tests
make test

# Run only agent package tests
go test ./agent/... -v

# Run integration tests (requires OpenCode running)
go test ./agent/... -run Integration -v

# Run with race detector
make test-race
```

### Step 5: Run the Example

```bash
# Navigate to examples
cd agent/examples/basic

# Run the example
go run main.go
```

### Step 6: Run Linter

```bash
make lint
```

### Manual API Testing

Check OpenCode API directly:

```bash
# Health check
curl http://localhost:3000/health

# List agents
curl http://localhost:3000/agents

# Create a session
curl -X POST http://localhost:3000/session \
  -H "Content-Type: application/json" \
  -d '{"directory": "."}'

# Send a prompt (replace SESSION_ID from previous response)
curl -X POST http://localhost:3000/session/SESSION_ID/message \
  -H "Content-Type: application/json" \
  -d '{"parts": [{"type": "text", "text": "Hello"}]}'
```

### Quick Test Script

Create `test-agent.sh`:

```bash
#!/bin/bash
set -e

echo "=== Starting services ==="
docker-compose up -d

echo "=== Waiting for Ollama ==="
sleep 5

echo "=== Pulling model ==="
docker-compose exec -T ollama ollama pull llama3 || true

echo "=== Running tests ==="
go test ./agent/... -v

echo "=== Running linter ==="
make lint

echo "=== Running example ==="
cd agent/examples/basic && go run main.go

echo "=== Done ==="
```

Make it executable:
```bash
chmod +x test-agent.sh
./test-agent.sh
```

## Core Components

### MCP Server Management

```go
// Local MCP server (stdio transport)
localServer := agent.LocalMCPServer("filesystem",
    []string{"mcp-filesystem", "/path/to/repo"},
    agent.WithEnv(map[string]string{"LOG_LEVEL": "debug"}),
    agent.WithEnabled(true),
)

// Remote MCP server (HTTP/SSE transport)
remoteServer := agent.RemoteMCPServer("brave-search",
    "https://mcp.brave.com/search",
    agent.WithHeaders(map[string]string{"Authorization": "Bearer token"}),
    agent.WithEnabled(true),
)

// Create registry
registry := agent.NewMCPRegistry(localServer, remoteServer)

// Add to registry
registry.Add(agent.LocalMCPServer("tools", []string{"mcp-tools"}))

// List servers
for _, s := range registry.List() {
    fmt.Printf("%s (%s): enabled=%v\n", s.Name, s.Type, s.Enabled)
}
```

### Permissions

```go
// Build permissions fluently
perms := agent.NewPermissions().
    AllowBash("go test", "go build", "go run").
    AskBash("rm *", "git push").
    DenyBash("sudo *").
    AllowEdit().
    DenyWebfetch().
    Build()

// Use with agent
ag, _ := agent.New(agent.WithPermissions(perms))
```

### Session Management

```go
// Create new session
session, _ := ag.NewSession(ctx,
    agent.WithTitle("Code Review"),
    agent.WithDirectory("/path/to/project"),
)

// Send prompt
response, _ := session.Prompt(ctx, "Review main.go")

// Stream response
events, _ := ag.Stream(ctx, "Explain the architecture")
for event := range events {
    switch event.Type {
    case agent.EventTypeComplete:
        resp := event.Data.(agent.Response)
        fmt.Println(resp.Content)
    case agent.EventTypeError:
        log.Printf("Error: %v", event.Error)
    }
}

// List sessions
sessions, _ := ag.ListSessions(ctx)

// Delete session
ag.DeleteSession(ctx, session.ID)
```

### Prompt Building

```go
// Using prompt builder
builder := agent.NewPromptBuilder().
    AddText("Review this file:\n").
    AddFile("main.go").
    AddText("\nFocus on error handling.")

config := builder.Build(
    agent.WithContext("Code review context"),
    agent.WithTemperature(0.7),
)

response, _ := session.Prompt(ctx, "", agent.WithParts(config.Parts...))

// Using prompt options directly
response, _ := session.Prompt(ctx,
    "Analyze these files",
    agent.WithFiles("main.go", "utils.go"),
    agent.WithContext("Focus on performance"),
)
```

### Event Handling

```go
// Create event handler
handler := agent.NewEventHandler()
handler.OnTextPart(func(ctx context.Context, text string) error {
    fmt.Print(text)
    return nil
})
handler.OnError(func(ctx context.Context, err error) error {
    log.Printf("Error: %v", err)
    return nil
})
handler.OnComplete(func(ctx context.Context, resp agent.Response) error {
    fmt.Printf("\nTokens: input=%.0f, output=%.0f\n",
        resp.Tokens.Input, resp.Tokens.Output)
    return nil
})

// Attach to agent
ag, _ := agent.New(
    agent.WithModel("ollama/llama3"),
    agent.WithEventHandlers(handler.Handle),
)
```

### Configuration Files

```go
// Load from opencode.json
config, _ := agent.LoadConfigFromDir(".")

// Create config programmatically
config := agent.NewConfigBuilder().
    WithModel("anthropic/claude-3-5-sonnet").
    WithSmallModel("anthropic/claude-3-haiku").
    AddLocalMCP("filesystem", []string{"mcp-filesystem"}, nil).
    WithPermissions(agent.NewPermissions().AllowEdit().Build()).
    Build()

// Save to opencode.json
agent.SaveConfig(config, "./opencode.json")
```

## Docker Compose Services

The `docker-compose.yml` includes:

| Service | Image | Port | Purpose |
|---------|-------|------|---------|
| qdrant | qdrant/qdrant:v1.16.0 | 6333, 6334 | Vector database |
| ollama | ollama/ollama:latest | 11434 | Local LLM inference |
| opencode | ghcr.io/anomalyco/opencode:latest | 3000 | Agent API server |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENCODE_API_KEY` | (empty) | API key for cloud providers |
| `OPENCODE_MODEL` | `ollama/glm-5:cloud` | Default model to use |
| `OPENCODE_LOG_LEVEL` | `info` | Logging level |
| `OLLAMA_HOST` | `ollama:11434` | Ollama connection (internal) |

## API Reference

Full API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/sevigo/goframe/agent).

## Troubleshooting

```bash
# View logs
docker-compose logs -f opencode
docker-compose logs -f ollama

# Restart services
docker-compose restart

# Clean up everything
docker-compose down -v

# Check service health
docker-compose ps
docker-compose exec ollama ollama list
curl http://localhost:3000/health
```

## Examples

See `examples/basic/main.go` for a complete working example.

## License

MIT License - see [LICENSE](../LICENSE) for details.