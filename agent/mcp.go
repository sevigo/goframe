package agent

import (
	"sync"

	opencode "github.com/sst/opencode-sdk-go"
)

// MCPServerType defines the type of MCP server connection
type MCPServerType string

const (
	// MCPServerTypeLocal represents a local MCP server using stdio transport
	MCPServerTypeLocal MCPServerType = "local"
	// MCPServerTypeRemote represents a remote MCP server using HTTP/SSE transport
	MCPServerTypeRemote MCPServerType = "remote"
)

// MCPServer defines an MCP (Model Context Protocol) server configuration
type MCPServer struct {
	// Name is the unique identifier for the server
	Name string `json:"name"`
	// Type specifies whether this is a local or remote server
	Type MCPServerType `json:"type"`
	// Command is the executable and arguments for local servers
	Command []string `json:"command,omitempty"`
	// Environment variables for local servers
	Environment map[string]string `json:"environment,omitempty"`
	// URL is the endpoint for remote servers
	URL string `json:"url,omitempty"`
	// Headers for remote server authentication
	Headers map[string]string `json:"headers,omitempty"`
	// Enabled determines if the server is active
	Enabled bool `json:"enabled"`
	// Disabled explicitly disables a server
	Disabled bool `json:"disabled,omitempty"`
}

// MCPOption configures an MCP server
type MCPOption func(*MCPServer)

// WithEnv sets environment variables for local MCP servers
func WithEnv(env map[string]string) MCPOption {
	return func(s *MCPServer) {
		if s.Environment == nil {
			s.Environment = make(map[string]string)
		}
		for k, v := range env {
			s.Environment[k] = v
		}
	}
}

// WithHeaders sets HTTP headers for remote MCP servers
func WithHeaders(headers map[string]string) MCPOption {
	return func(s *MCPServer) {
		if s.Headers == nil {
			s.Headers = make(map[string]string)
		}
		for k, v := range headers {
			s.Headers[k] = v
		}
	}
}

// WithEnabled sets whether the server is enabled
func WithEnabled(enabled bool) MCPOption {
	return func(s *MCPServer) {
		s.Enabled = enabled
	}
}

// WithDisabled marks the server as disabled
func WithDisabled() MCPOption {
	return func(s *MCPServer) {
		s.Disabled = true
		s.Enabled = false
	}
}

// LocalMCPServer creates a local MCP server that runs as a subprocess
// using stdio for communication.
//
// Example:
//
//	server := LocalMCPServer("filesystem",
//	    []string{"mcp-filesystem", "/path/to/repo"},
//	    WithEnv(map[string]string{"LOG_LEVEL": "debug"}),
//	    WithEnabled(true),
//	)
func LocalMCPServer(name string, command []string, opts ...MCPOption) *MCPServer {
	s := &MCPServer{
		Name:    name,
		Type:    MCPServerTypeLocal,
		Command: command,
		Enabled: true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RemoteMCPServer creates a remote MCP server that connects via HTTP/SSE.
//
// Example:
//
//	server := RemoteMCPServer("brave-search",
//	    "https://mcp.brave.com/search",
//	    WithHeaders(map[string]string{"Authorization": "Bearer token"}),
//	    WithEnabled(true),
//	)
func RemoteMCPServer(name string, url string, opts ...MCPOption) *MCPServer {
	s := &MCPServer{
		Name:    name,
		Type:    MCPServerTypeRemote,
		URL:     url,
		Enabled: true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Validate checks if the MCP server configuration is valid
func (s *MCPServer) Validate() error {
	if s.Name == "" {
		return newMCPError("", ErrInvalidMCPConfig)
	}

	switch s.Type {
	case MCPServerTypeLocal:
		if len(s.Command) == 0 {
			return newMCPError(s.Name, ErrInvalidMCPConfig)
		}
	case MCPServerTypeRemote:
		if s.URL == "" {
			return newMCPError(s.Name, ErrInvalidMCPConfig)
		}
	default:
		return newMCPError(s.Name, ErrInvalidMCPConfig)
	}

	return nil
}

// ToConfig converts the MCPServer to the OpenCode SDK configuration format
func (s *MCPServer) ToConfig() (opencode.ConfigMcp, error) {
	if err := s.Validate(); err != nil {
		return opencode.ConfigMcp{}, err
	}

	config := opencode.ConfigMcp{
		Type:    opencode.ConfigMcpType(s.Type),
		Enabled: s.Enabled,
	}

	switch s.Type {
	case MCPServerTypeLocal:
		var cmd []interface{}
		for _, c := range s.Command {
			cmd = append(cmd, c)
		}
		config.Command = cmd

		if len(s.Environment) > 0 {
			env := make(map[string]interface{})
			for k, v := range s.Environment {
				env[k] = v
			}
			config.Environment = env
		}

	case MCPServerTypeRemote:
		config.URL = s.URL

		if len(s.Headers) > 0 {
			headers := make(map[string]interface{})
			for k, v := range s.Headers {
				headers[k] = v
			}
			config.Headers = headers
		}
	}

	return config, nil
}

// MCPRegistry manages a collection of MCP servers
type MCPRegistry struct {
	servers map[string]*MCPServer
	mu      sync.RWMutex
}

// NewMCPRegistry creates a new registry with the given servers
func NewMCPRegistry(servers ...*MCPServer) *MCPRegistry {
	r := &MCPRegistry{
		servers: make(map[string]*MCPServer),
	}
	for _, s := range servers {
		if s != nil && s.Name != "" {
			r.servers[s.Name] = s
		}
	}
	return r
}

// Add adds an MCP server to the registry
func (r *MCPRegistry) Add(server *MCPServer) error {
	if server == nil {
		return ErrInvalidMCPConfig
	}

	if err := server.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.servers[server.Name]; exists {
		return newMCPError(server.Name, ErrMCPServerExists)
	}

	r.servers[server.Name] = server
	return nil
}

// Remove removes an MCP server from the registry
func (r *MCPRegistry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.servers, name)
}

// Get retrieves an MCP server by name
func (r *MCPRegistry) Get(name string) (*MCPServer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	server, ok := r.servers[name]
	return server, ok
}

// List returns all MCP servers in the registry
func (r *MCPRegistry) List() []*MCPServer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]*MCPServer, 0, len(r.servers))
	for _, s := range r.servers {
		servers = append(servers, s)
	}
	return servers
}

// ToConfigMap converts all servers to the OpenCode SDK configuration format
func (r *MCPRegistry) ToConfigMap() map[string]opencode.ConfigMcp {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]opencode.ConfigMcp, len(r.servers))
	for name, server := range r.servers {
		config, err := server.ToConfig()
		if err != nil {
			continue
		}
		result[name] = config
	}
	return result
}

// Count returns the number of servers in the registry
func (r *MCPRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.servers)
}

// Clear removes all servers from the registry
func (r *MCPRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers = make(map[string]*MCPServer)
}

// Merge combines another registry into this one.
// Both registries are locked during the operation to prevent data races.
func (r *MCPRegistry) Merge(other *MCPRegistry) error {
	if other == nil {
		return nil
	}

	// Lock r first (write), then other (read) — deterministic order prevents deadlock.
	r.mu.Lock()
	defer r.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	// Check for conflicts while both locks are held.
	for name := range other.servers {
		if _, exists := r.servers[name]; exists {
			return newMCPError(name, ErrMCPServerExists)
		}
	}

	for name, server := range other.servers {
		r.servers[name] = server
	}

	return nil
}
