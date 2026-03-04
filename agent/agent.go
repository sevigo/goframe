package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	opencode "github.com/sst/opencode-sdk-go"
)

type AgentType string

const (
	AgentTypeBuild   AgentType = "build"
	AgentTypePlan    AgentType = "plan"
	AgentTypeGeneral AgentType = "general"
)

type HookConfig struct {
	OnSessionComplete []HookFunc
	OnFileEdited      []HookFunc
}

type HookFunc func(ctx context.Context, event HookEvent) error

type HookEvent struct {
	Type      string
	SessionID string
	Data      map[string]interface{}
}

type Config struct {
	Model       string
	SmallModel  string
	AgentType   AgentType
	Permissions *PermissionConfig
	Hooks       *HookConfig
	WorkingDir  string
	mcpRegistry *MCPRegistry
	agents      map[string]AgentConfig
	Tools       map[string]bool
}

type Agent struct {
	client            *opencode.Client
	config            *Config
	mcp               *MCPRegistry
	events            *EventHandler
	logger            *slog.Logger
	permissionHandler PermissionHandler
	mu                sync.RWMutex
}

func New(opts ...Option) (*Agent, error) {
	agent := &Agent{
		config: &Config{
			AgentType: AgentTypeGeneral,
		},
		mcp:               NewMCPRegistry(),
		events:            NewEventHandler(),
		logger:            slog.New(slog.NewTextHandler(os.Stderr, nil)),
		permissionHandler: DefaultPermissionHandler(),
	}

	for _, opt := range opts {
		if err := opt(agent); err != nil {
			return nil, err
		}
	}

	if agent.client == nil {
		agent.client = opencode.NewClient()
	}

	return agent, nil
}

func (a *Agent) NewSession(ctx context.Context, opts ...SessionOption) (*Session, error) {
	config := &SessionConfig{}
	for _, opt := range opts {
		opt(config)
	}

	if config.Directory == "" {
		a.mu.RLock()
		workingDir := a.config.WorkingDir
		a.mu.RUnlock()
		config.Directory = workingDir
		if config.Directory == "" {
			wd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getting working directory: %w", err)
			}
			config.Directory = wd
		}
	}

	params := opencode.SessionNewParams{
		Directory: opencode.F(config.Directory),
	}

	if config.Title != "" {
		params.Title = opencode.F(config.Title)
	}

	if config.ParentID != "" {
		params.ParentID = opencode.F(config.ParentID)
	}

	resp, err := a.client.Session.New(ctx, params)
	if err != nil {
		return nil, newError("new_session", err)
	}

	session := &Session{
		ID:        resp.ID,
		Title:     resp.Title,
		Directory: resp.Directory,
		ProjectID: resp.ProjectID,
		Created:   resp.Time.Created,
		Updated:   resp.Time.Updated,
		client:    a.client,
		logger:    a.logger,
	}

	return session, nil
}

func (a *Agent) GetSession(ctx context.Context, id string) (*Session, error) {
	resp, err := a.client.Session.Get(ctx, id, opencode.SessionGetParams{})
	if err != nil {
		return nil, newSessionError("get_session", id, err)
	}

	session := &Session{
		ID:        resp.ID,
		Title:     resp.Title,
		Directory: resp.Directory,
		ProjectID: resp.ProjectID,
		Created:   resp.Time.Created,
		Updated:   resp.Time.Updated,
		client:    a.client,
		logger:    a.logger,
	}

	return session, nil
}

func (a *Agent) ListSessions(ctx context.Context) ([]*Session, error) {
	resp, err := a.client.Session.List(ctx, opencode.SessionListParams{})
	if err != nil {
		return nil, newError("list_sessions", err)
	}

	sessions := make([]*Session, 0, len(*resp))
	for _, s := range *resp {
		sessions = append(sessions, &Session{
			ID:        s.ID,
			Title:     s.Title,
			Directory: s.Directory,
			ProjectID: s.ProjectID,
			Created:   s.Time.Created,
			Updated:   s.Time.Updated,
			client:    a.client,
			logger:    a.logger,
		})
	}

	return sessions, nil
}

func (a *Agent) DeleteSession(ctx context.Context, id string) error {
	_, err := a.client.Session.Delete(ctx, id, opencode.SessionDeleteParams{})
	if err != nil {
		return newSessionError("delete_session", id, err)
	}

	return nil
}

func (a *Agent) AddMCPServer(ctx context.Context, server *MCPServer) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.mcp.Add(server); err != nil {
		return err
	}

	a.logger.Info("MCP server added", "name", server.Name, "type", server.Type)
	return nil
}

func (a *Agent) RemoveMCPServer(ctx context.Context, name string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.mcp.Remove(name)
	a.logger.Info("MCP server removed", "name", name)
}

func (a *Agent) ListMCPServers(ctx context.Context) []*MCPServer {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.mcp.List()
}

func (a *Agent) GetConfig() *Config {
	a.mu.RLock()
	defer a.mu.RUnlock()

	config := *a.config
	if a.config.Permissions != nil {
		permCopy := *a.config.Permissions
		config.Permissions = &permCopy
	}

	return &config
}

func (a *Agent) GetEventHandler() *EventHandler {
	return a.events
}

// Ask sends a one-off prompt and returns the response.
// Note: This creates a new session for each call. For multiple prompts,
// use NewSession and session.Prompt instead to reuse the session.
func (a *Agent) Ask(ctx context.Context, prompt string, opts ...PromptOption) (*Response, error) {
	session, err := a.NewSession(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	return session.Prompt(ctx, prompt, opts...)
}

// Stream sends a one-off prompt and returns a channel for streaming responses.
// Note: This creates a new session for each call. For multiple prompts,
// use NewSession and session.PromptStream instead to reuse the session.
func (a *Agent) Stream(ctx context.Context, prompt string, opts ...PromptOption) (<-chan Event, error) {
	session, err := a.NewSession(ctx)
	if err != nil {
		return nil, err
	}

	// Note: Session is NOT closed here because the goroutine in PromptStream
	// may still be using it. The caller is responsible for cleanup if needed.
	// For one-off streaming, consider the session auto-cleaned after completion.

	return session.PromptStream(ctx, prompt, opts...)
}

func (a *Agent) GetClient() *opencode.Client {
	return a.client
}
