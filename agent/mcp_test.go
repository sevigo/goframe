package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalMCPServer(t *testing.T) {
	tests := []struct {
		name     string
		name_    string
		command  []string
		opts     []MCPOption
		expected *MCPServer
	}{
		{
			name:    "basic local server",
			name_:   "test-server",
			command: []string{"mcp-server", "--port", "8080"},
			opts:    nil,
			expected: &MCPServer{
				Name:    "test-server",
				Type:    MCPServerTypeLocal,
				Command: []string{"mcp-server", "--port", "8080"},
				Enabled: true,
			},
		},
		{
			name:    "local server with env",
			name_:   "env-server",
			command: []string{"mcp-server"},
			opts: []MCPOption{
				WithEnv(map[string]string{"LOG_LEVEL": "debug"}),
			},
			expected: &MCPServer{
				Name:        "env-server",
				Type:        MCPServerTypeLocal,
				Command:     []string{"mcp-server"},
				Enabled:     true,
				Environment: map[string]string{"LOG_LEVEL": "debug"},
			},
		},
		{
			name:    "local server disabled",
			name_:   "disabled-server",
			command: []string{"mcp-server"},
			opts:    []MCPOption{WithDisabled()},
			expected: &MCPServer{
				Name:     "disabled-server",
				Type:     MCPServerTypeLocal,
				Command:  []string{"mcp-server"},
				Enabled:  false,
				Disabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := LocalMCPServer(tt.name_, tt.command, tt.opts...)
			assert.Equal(t, tt.expected.Name, server.Name)
			assert.Equal(t, tt.expected.Type, server.Type)
			assert.Equal(t, tt.expected.Command, server.Command)
			assert.Equal(t, tt.expected.Enabled, server.Enabled)
			assert.Equal(t, tt.expected.Disabled, server.Disabled)
			if tt.expected.Environment != nil {
				assert.Equal(t, tt.expected.Environment, server.Environment)
			}
		})
	}
}

func TestRemoteMCPServer(t *testing.T) {
	tests := []struct {
		name     string
		name_    string
		url      string
		opts     []MCPOption
		expected *MCPServer
	}{
		{
			name:  "basic remote server",
			name_: "test-remote",
			url:   "https://mcp.example.com/api",
			opts:  nil,
			expected: &MCPServer{
				Name:    "test-remote",
				Type:    MCPServerTypeRemote,
				URL:     "https://mcp.example.com/api",
				Enabled: true,
			},
		},
		{
			name:  "remote server with headers",
			name_: "auth-server",
			url:   "https://mcp.example.com/api",
			opts:  []MCPOption{WithHeaders(map[string]string{"Authorization": "Bearer token"})},
			expected: &MCPServer{
				Name:    "auth-server",
				Type:    MCPServerTypeRemote,
				URL:     "https://mcp.example.com/api",
				Enabled: true,
				Headers: map[string]string{"Authorization": "Bearer token"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := RemoteMCPServer(tt.name_, tt.url, tt.opts...)
			assert.Equal(t, tt.expected.Name, server.Name)
			assert.Equal(t, tt.expected.Type, server.Type)
			assert.Equal(t, tt.expected.URL, server.URL)
			assert.Equal(t, tt.expected.Enabled, server.Enabled)
			if tt.expected.Headers != nil {
				assert.Equal(t, tt.expected.Headers, server.Headers)
			}
		})
	}
}

func TestMCPServerValidate(t *testing.T) {
	tests := []struct {
		name    string
		server  *MCPServer
		wantErr bool
	}{
		{
			name: "valid local server",
			server: &MCPServer{
				Name:    "test",
				Type:    MCPServerTypeLocal,
				Command: []string{"mcp-server"},
			},
			wantErr: false,
		},
		{
			name: "valid remote server",
			server: &MCPServer{
				Name: "test",
				Type: MCPServerTypeRemote,
				URL:  "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			server: &MCPServer{
				Type:    MCPServerTypeLocal,
				Command: []string{"mcp-server"},
			},
			wantErr: true,
		},
		{
			name: "local server missing command",
			server: &MCPServer{
				Name: "test",
				Type: MCPServerTypeLocal,
			},
			wantErr: true,
		},
		{
			name: "remote server missing url",
			server: &MCPServer{
				Name: "test",
				Type: MCPServerTypeRemote,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			server: &MCPServer{
				Name: "test",
				Type: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.server.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMCPRegistry(t *testing.T) {
	t.Run("add server", func(t *testing.T) {
		registry := NewMCPRegistry()
		server := LocalMCPServer("test", []string{"mcp-server"})
		err := registry.Add(server)
		assert.NoError(t, err)
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("add duplicate server", func(t *testing.T) {
		registry := NewMCPRegistry()
		server := LocalMCPServer("test", []string{"mcp-server"})
		_ = registry.Add(server)
		err := registry.Add(server)
		assert.Error(t, err)
		var mcpErr *MCPError
		assert.True(t, errors.As(err, &mcpErr))
		assert.Equal(t, ErrMCPServerExists, mcpErr.Err)
	})

	t.Run("remove server", func(t *testing.T) {
		registry := NewMCPRegistry()
		server := LocalMCPServer("test", []string{"mcp-server"})
		_ = registry.Add(server)
		registry.Remove("test")
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("get server", func(t *testing.T) {
		registry := NewMCPRegistry()
		server := LocalMCPServer("test", []string{"mcp-server"})
		_ = registry.Add(server)
		got, ok := registry.Get("test")
		assert.True(t, ok)
		assert.Equal(t, server, got)
	})

	t.Run("list servers", func(t *testing.T) {
		registry := NewMCPRegistry(
			LocalMCPServer("server1", []string{"mcp-server1"}),
			LocalMCPServer("server2", []string{"mcp-server2"}),
		)
		servers := registry.List()
		assert.Len(t, servers, 2)
	})

	t.Run("merge registries", func(t *testing.T) {
		registry1 := NewMCPRegistry(
			LocalMCPServer("server1", []string{"mcp-server1"}),
		)
		registry2 := NewMCPRegistry(
			LocalMCPServer("server2", []string{"mcp-server2"}),
		)
		err := registry1.Merge(registry2)
		assert.NoError(t, err)
		assert.Equal(t, 2, registry1.Count())
	})

	t.Run("merge with conflict", func(t *testing.T) {
		registry1 := NewMCPRegistry(
			LocalMCPServer("server", []string{"mcp-server"}),
		)
		registry2 := NewMCPRegistry(
			LocalMCPServer("server", []string{"mcp-server"}),
		)
		err := registry1.Merge(registry2)
		assert.Error(t, err)
	})
}
