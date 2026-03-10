// Package agent provides an abstraction layer for managing communication with
// OpenCode in agent mode, with a focus on MCP (Model Context Protocol) server
// configuration.
//
// The agent package enables programmatic control of AI agents through the OpenCode SDK,
// providing structured configuration for MCP servers, permissions, sessions, and prompts.
//
// # Overview
//
// The package is built around several core concepts:
//
//   - Agent: The main entry point for interacting with OpenCode
//   - MCP (Model Context Protocol): Server configuration for tool access
//   - Session: Conversation context management
//   - Permission: Fine-grained control over agent capabilities
//   - Prompt: Structured message construction
//   - Event: Asynchronous response handling
//
// # Native Agentic Framework
//
// The package also provides a standalone, LLM-agnostic agentic framework for building
// autonomous agents that can run locally using standard goframe components.
//
// The native framework includes three core primitives:
//
// # 1. Tool Registry (agent.Tool)
//
// The Registry manages available tools with automatic schema generation:
//
//	registry := agent.NewRegistry()
//
//	// Create tool from function
//	searchTool, _ := agent.NewToolFromFunc(
//	    "search",
//	    "Search for documents matching a query",
//	    func(ctx context.Context, params SearchParams) (SearchResult, error) {
//	        // implementation
//	    },
//	)
//	registry.MustRegisterTool(searchTool)
//
// # 2. Governance (agent.IntegrityCheck)
//
// Integrity checks validate tool executions before they run:
//
//	governance := agent.NewGovernance(
//	    agent.NewPermissionCheck().Allow("read", "write").Deny("delete"),
//	    agent.NewParameterCheck().Require("write", "path"),
//	)
//
// # 3. Autonomous Loop (agent.AgentLoop)
//
// The AgentLoop manages the think-act-observe lifecycle:
//
//	loop, _ := agent.NewAgentLoop(model, registry,
//	    agent.WithLoopMaxIterations(20),
//	    agent.WithLoopGovernance(governance),
//	    agent.WithLoopSystemPrompt("You are a helpful assistant"),
//	)
//	result, _ := loop.Run(ctx, task, nil)
//
// # MCP Server Configuration
//
// MCP servers are the primary focus of this package. They enable agents to access
// external tools and resources. Two transport types are supported:
//
// Local (stdio) servers run as subprocesses:
//
//	server := agent.LocalMCPServer("filesystem",
//	    []string{"mcp-filesystem", "/path/to/repo"},
//	    agent.WithEnv(map[string]string{"LOG_LEVEL": "debug"}),
//	)
//
// Remote (HTTP/SSE) servers connect over HTTP:
//
//	server := agent.RemoteMCPServer("brave-search",
//	    "https://mcp.brave.com/search",
//	    agent.WithHeaders(map[string]string{"Authorization": "Bearer token"}),
//	)
//
// Servers are managed through an MCPRegistry:
//
//	registry := agent.NewMCPRegistry(server1, server2)
//	agent, _ := agent.New(
//	    agent.WithMCPRegistry(registry),
//	    agent.WithModel("anthropic/claude-3-5-sonnet"),
//	)
//
// # Permissions
//
// Permissions control what actions agents can take. Use the PermissionBuilder
// for fluent configuration:
//
//	perms := agent.NewPermissions().
//	    AllowBash("go test", "go build").
//	    AskBash("rm *").
//	    AllowEdit().
//	    DenyWebfetch().
//	    Build()
//
//	agent, _ := agent.New(agent.WithPermissions(perms))
//
// # Sessions and Prompts
//
// Agents operate within sessions that maintain conversation context:
//
//	session, _ := agent.NewSession(ctx, agent.WithTitle("Code Review"))
//	defer agent.DeleteSession(ctx, session.ID)
//
//	response, _ := session.Prompt(ctx, "Explain this code")
//
// Prompts can include files and other content:
//
//	response, _ := session.Prompt(ctx,
//	    "Compare these files",
//	    agent.WithFiles("main.go", "utils.go"),
//	    agent.WithTemperature(0.7),
//	)
//
// # Event Streaming
//
// For async responses, use streaming:
//
//	events, _ := agent.Stream(ctx, "Write a haiku")
//	for event := range events {
//	    switch event.Type {
//	    case agent.EventTypeComplete:
//	        // Handle completion
//	    case agent.EventTypeError:
//	        // Handle error
//	    }
//	}
//
// # Configuration Files
//
// Load configuration from opencode.json files:
//
//	config, err := agent.LoadConfigFromDir(".")
//	agent, _ := agent.New(
//	    agent.WithModel(config.Model),
//	    agent.WithMCPRegistry(agent.NewMCPRegistry(config.GetMCPServers()...)),
//	)
//
// # Thread Safety
//
// The Agent type is safe for concurrent use. All exported methods handle their
// own synchronization internally.
package agent
