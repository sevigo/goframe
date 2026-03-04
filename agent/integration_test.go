package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/sevigo/goframe/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test - requires OpenCode server running
// Run with: go test ./agent -run Integration -v

func TestIntegration_AgentWithMCP(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create MCP registry with a simple server
	mcpRegistry := agent.NewMCPRegistry(
		agent.LocalMCPServer("filesystem",
			[]string{"mcp-filesystem", "."},
			agent.WithEnabled(true),
		),
	)

	// Create agent
	ag, err := agent.New(
		agent.WithModel("ollama/llama3"),
		agent.WithMCPRegistry(mcpRegistry),
		agent.WithPermissions(
			agent.NewPermissions().
				AllowBash("ls", "cat").
				AllowEdit().
				Build(),
		),
	)
	require.NoError(t, err)
	require.NotNil(t, ag)

	// List MCP servers
	servers := ag.ListMCPServers(ctx)
	assert.Len(t, servers, 1)
	assert.Equal(t, "filesystem", servers[0].Name)

	// Create session
	session, err := ag.NewSession(ctx, agent.WithTitle("Test Session"))
	require.NoError(t, err)
	require.NotEmpty(t, session.ID)

	// List sessions
	sessions, err := ag.ListSessions(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, sessions)

	// Delete session
	err = ag.DeleteSession(ctx, session.ID)
	require.NoError(t, err)

	// Verify deletion
	sessions, err = ag.ListSessions(ctx)
	require.NoError(t, err)
	for _, s := range sessions {
		assert.NotEqual(t, session.ID, s.ID)
	}
}

func TestIntegration_SessionPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create agent with minimal config
	ag, err := agent.New(agent.WithModel("ollama/llama3"))
	require.NoError(t, err)

	// Create session
	session, err := ag.NewSession(ctx, agent.WithTitle("Prompt Test"))
	require.NoError(t, err)
	defer ag.DeleteSession(ctx, session.ID)

	// Test streaming
	t.Run("stream", func(t *testing.T) {
		events, err := ag.Stream(ctx, "Say 'hello'")
		require.NoError(t, err)

		var received bool
		for event := range events {
			received = true
			switch event.Type {
			case agent.EventTypeComplete:
				resp := event.Data.(agent.Response)
				assert.NotEmpty(t, resp.Content)
			case agent.EventTypeError:
				t.Logf("Error: %v", event.Error)
			}
		}
		assert.True(t, received, "Should receive at least one event")
	})
}
