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

type Option func(*Agent) error

func WithClient(client *opencode.Client) Option {
	return func(a *Agent) error {
		if client == nil {
			return ErrNilClient
		}
		a.client = client
		return nil
	}
}

func WithBaseURL(baseURL string) Option {
	return func(a *Agent) error {
		client := opencode.NewClient(option.WithBaseURL(baseURL))
		a.client = client
		return nil
	}
}

func WithModel(model string) Option {
	return func(a *Agent) error {
		a.config.Model = model
		return nil
	}
}

func WithSmallModel(model string) Option {
	return func(a *Agent) error {
		a.config.SmallModel = model
		return nil
	}
}

func WithAgentType(agentType AgentType) Option {
	return func(a *Agent) error {
		a.config.AgentType = agentType
		return nil
	}
}

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

func WithMCPRegistry(registry *MCPRegistry) Option {
	return func(a *Agent) error {
		a.mcp = registry
		return nil
	}
}

func WithPermissions(perm *PermissionConfig) Option {
	return func(a *Agent) error {
		a.config.Permissions = perm
		return nil
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(a *Agent) error {
		if logger == nil {
			logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		}
		a.logger = logger
		return nil
	}
}

func WithPermissionHandler(handler PermissionHandler) Option {
	return func(a *Agent) error {
		a.permissionHandler = handler
		return nil
	}
}

type SessionOption func(*SessionConfig)

type SessionConfig struct {
	Title     string
	ParentID  string
	Directory string
	ProjectID string
}

func WithTitle(title string) SessionOption {
	return func(c *SessionConfig) {
		c.Title = title
	}
}

func WithParentID(parentID string) SessionOption {
	return func(c *SessionConfig) {
		c.ParentID = parentID
	}
}

func WithDirectory(dir string) SessionOption {
	return func(c *SessionConfig) {
		c.Directory = dir
	}
}

func WithProjectID(projectID string) SessionOption {
	return func(c *SessionConfig) {
		c.ProjectID = projectID
	}
}
