// Example demonstrates basic usage of the agent package with MCP servers
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

	// Create agent with MCP servers and permissions
	ag, session := createAgent(ctx)
	if ag == nil {
		return
	}

	// Ensure cleanup
	defer func() { _ = ag.DeleteSession(ctx, session.ID) }()

	// Run examples
	displayMCPServers(ctx, ag)
	exampleSimplePrompt(ctx, session)
	examplePromptWithFiles(ctx, session)
	exampleStreaming(ctx, ag)
	examplePromptBuilder(ctx, session)
	exampleSessionManagement(ctx, ag)
	exampleConfiguration()
	exampleLoadConfiguration()
}

func createAgent(ctx context.Context) (*agent.Agent, *agent.Session) {
	mcpRegistry := agent.NewMCPRegistry(
		agent.LocalMCPServer("filesystem",
			[]string{"mcp-filesystem", "/tmp/workspace"},
			agent.WithEnv(map[string]string{"LOG_LEVEL": "debug"}),
			agent.WithEnabled(true),
		),
	)

	permissions := agent.NewPermissions().
		AllowBash("go test", "go build", "go run").
		AskBash("rm *", "git push").
		AllowEdit().
		DenyWebfetch().
		Build()

	// Get OpenCode URL from environment or use default
	baseURL := os.Getenv("OPENCODE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}

	// Get model from environment or use default
	model := os.Getenv("OPENCODE_MODEL")
	if model == "" {
		model = "ollama/llama3"
	}

	ag, err := agent.New(
		agent.WithBaseURL(baseURL),
		agent.WithModel(model),
		agent.WithMCPRegistry(mcpRegistry),
		agent.WithPermissions(permissions),
		agent.WithWorkingDir("/tmp/workspace"),
		agent.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
		agent.WithPermissionHandler(agent.AllowAllPermissionHandler()),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error creating agent: %v\n", err)
		return nil, nil
	}

	session, err := ag.NewSession(ctx, agent.WithTitle("Example Session"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		return nil, nil
	}

	return ag, session
}

func displayMCPServers(ctx context.Context, ag *agent.Agent) {
	fmt.Println("=== MCP Servers ===")
	for _, server := range ag.ListMCPServers(ctx) {
		fmt.Printf("  - %s (%s): enabled=%v\n", server.Name, server.Type, server.Enabled)
	}
}

func exampleSimplePrompt(ctx context.Context, session *agent.Session) {
	fmt.Println("\n=== Example 1: Simple Prompt ===")
	response, err := session.Prompt(ctx, "Explain what MCP servers are in one paragraph")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	fmt.Printf("Response: %s\n", truncate(response.Content, 200))
	fmt.Printf("Tokens: input=%.0f, output=%.0f\n", response.Tokens.Input, response.Tokens.Output)
}

func examplePromptWithFiles(ctx context.Context, session *agent.Session) {
	fmt.Println("\n=== Example 2: Prompt with Files ===")
	content := []byte(`module example.com/test

go 1.21

require (
	github.com/some/package v1.0.0
)`)
	response, err := session.Prompt(ctx,
		"Analyze this code structure",
		agent.WithParts(agent.FileFromContent("go.mod", content, "text/x-go")),
		agent.WithContext("Focus on dependencies"),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return
	}
	fmt.Printf("Response: %s\n", truncate(response.Content, 200))
}

func exampleStreaming(ctx context.Context, ag *agent.Agent) {
	fmt.Println("\n=== Example 3: Streaming Response ===")
	stream, err := ag.Stream(ctx, "Count from 1 to 5, one number per line")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	for event := range stream {
		switch event.Type {
		case agent.EventTypeComplete:
			if resp, ok := event.Data.(agent.Response); ok {
				fmt.Printf("Stream completed: %s\n", truncate(resp.Content, 100))
			}
		case agent.EventTypeError:
			fmt.Printf("Error: %v\n", event.Error)
		}
	}
}

func examplePromptBuilder(ctx context.Context, session *agent.Session) {
	fmt.Println("\n=== Example 4: Prompt Builder ===")
	goModContent := []byte(`module example.com/test

go 1.21

require github.com/some/package v1.0.0`)
	goSumContent := []byte(`github.com/some/package v1.0.0 h1:abc123
github.com/some/package v1.0.0/go.mod h1:def456`)
	builder := agent.NewPromptBuilder().
		AddText("Compare these two files:\n").
		AddFileFromContent("go.mod", goModContent, "text/x-go").
		AddText("\nand\n").
		AddFileFromContent("go.sum", goSumContent, "text/plain").
		AddText("\nWhat are the main differences?")

	config := builder.Build()
	response, err := session.Prompt(ctx, "", agent.WithParts(config.Parts...))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return
	}
	fmt.Printf("Response: %s\n", truncate(response.Content, 200))
}

func exampleSessionManagement(ctx context.Context, ag *agent.Agent) {
	fmt.Println("\n=== Example 5: Session Management ===")
	sessions, err := ag.ListSessions(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	fmt.Printf("Found %d sessions\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("  - %s: %s\n", s.ID, s.Title)
	}
}

func exampleConfiguration() {
	fmt.Println("\n=== Example 6: Configuration ===")
	config := agent.NewConfigBuilder().
		WithModel("anthropic/claude-3-5-sonnet").
		WithSmallModel("anthropic/claude-3-haiku").
		AddLocalMCP("tools", []string{"mcp-tools"}, nil).
		WithPermissions(agent.NewPermissions().AllowEdit().Build()).
		Build()

	fmt.Printf("Model: %s\n", config.Model)
	fmt.Printf("Small Model: %s\n", config.SmallModel)
	fmt.Printf("MCP Servers: %d\n", len(config.GetMCPServers()))

	tmpDir := os.TempDir()
	configPath := tmpDir + "/opencode.json"
	err := agent.SaveConfig(config, configPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning saving config: %v\n", err)
	} else {
		fmt.Printf("Configuration saved to %s\n", configPath)
	}
}

func exampleLoadConfiguration() {
	fmt.Println("\n=== Example 7: Loading Configuration ===")
	tmpDir := os.TempDir()
	config, err := agent.LoadConfigFromDir(tmpDir)
	if err != nil {
		fmt.Printf("No config found: %v\n", err)
		return
	}
	fmt.Printf("Loaded model: %s\n", config.Model)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
