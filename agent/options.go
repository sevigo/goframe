package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	opencode "github.com/sst/opencode-sdk-go"
	"github.com/sst/opencode-sdk-go/option"
)

// Option configures an Agent during construction.
type Option func(*Agent) error

// WithClient sets a pre-configured OpenCode client.
func WithClient(client *opencode.Client) Option {
	return func(a *Agent) error {
		if client == nil {
			return ErrNilClient
		}
		a.client = client
		return nil
	}
}

// WithBaseURL creates a new OpenCode client configured for the given base URL.
func WithBaseURL(baseURL string) Option {
	return func(a *Agent) error {
		client := opencode.NewClient(option.WithBaseURL(baseURL))
		a.client = client
		return nil
	}
}

// WithModel sets the primary model identifier (e.g. "ollama/llama3").
func WithModel(model string) Option {
	return func(a *Agent) error {
		a.config.Model = model
		return nil
	}
}

// WithSmallModel sets the secondary / lighter model identifier.
func WithSmallModel(model string) Option {
	return func(a *Agent) error {
		a.config.SmallModel = model
		return nil
	}
}

// WithAgentType sets the agent type (build, plan, general).
func WithAgentType(agentType AgentType) Option {
	return func(a *Agent) error {
		a.config.AgentType = agentType
		return nil
	}
}

// WithWorkingDir sets the default working directory for new sessions.
func WithWorkingDir(dir string) Option {
	return func(a *Agent) error {
		a.config.WorkingDir = dir
		return nil
	}
}

// WithPathMapping configures path translation for Docker-based agents.
// It maps host paths to container paths so that when the agent creates
// a session, the directory is correctly translated for the container.
// Example: {"/home/user/agent-workspaces": "/agent-workspaces"}
func WithPathMapping(mapping map[string]string) Option {
	return func(a *Agent) error {
		for hostPath, containerPath := range mapping {
			if hostPath == "" || containerPath == "" {
				return errors.New("path mapping values cannot be empty")
			}
			if !filepath.IsAbs(hostPath) || !filepath.IsAbs(containerPath) {
				return fmt.Errorf("path mapping must use absolute paths: %s -> %s", hostPath, containerPath)
			}
		}
		a.config.PathMapping = mapping
		return nil
	}
}

// WithMCPServers registers one or more MCP servers with the agent.
func WithMCPServers(servers ...*MCPServer) Option {
	return func(a *Agent) error {
		if a.mcp == nil {
			a.mcp = NewMCPRegistry()
		}
		for _, server := range servers {
			if err := a.mcp.Add(server); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithMCPRegistry replaces the agent's MCP registry.
func WithMCPRegistry(registry *MCPRegistry) Option {
	return func(a *Agent) error {
		a.mcp = registry
		return nil
	}
}

// WithPermissions sets the agent's permission configuration.
func WithPermissions(perm *PermissionConfig) Option {
	return func(a *Agent) error {
		a.config.Permissions = perm
		return nil
	}
}

// WithLogger sets the agent's structured logger.
func WithLogger(logger *slog.Logger) Option {
	return func(a *Agent) error {
		if logger == nil {
			logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		}
		a.logger = logger
		return nil
	}
}

// WithPermissionHandler sets a custom handler for interactive permission requests.
func WithPermissionHandler(handler PermissionHandler) Option {
	return func(a *Agent) error {
		a.permissionHandler = handler
		return nil
	}
}

// SessionOption configures a new Session.
type SessionOption func(*SessionConfig)

// SessionConfig holds session-creation parameters.
type SessionConfig struct {
	Title     string
	ParentID  string
	Directory string
	ProjectID string
}

// WithTitle sets the session title.
func WithTitle(title string) SessionOption {
	return func(c *SessionConfig) {
		c.Title = title
	}
}

// WithParentID sets the parent session ID for branching.
func WithParentID(parentID string) SessionOption {
	return func(c *SessionConfig) {
		c.ParentID = parentID
	}
}

// WithDirectory sets the working directory for the session.
func WithDirectory(dir string) SessionOption {
	return func(c *SessionConfig) {
		c.Directory = dir
	}
}

// WithProjectID sets the project ID.
// NOTE: This field is stored locally but is not currently sent to the
// OpenCode API. It is reserved for future use.
func WithProjectID(projectID string) SessionOption {
	return func(c *SessionConfig) {
		c.ProjectID = projectID
	}
}
