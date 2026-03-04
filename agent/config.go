package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AgentMode string

const (
	AgentModeSubagent AgentMode = "subagent"
	AgentModePrimary  AgentMode = "primary"
	AgentModeAll      AgentMode = "all"
)

type AgentConfig struct {
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	Model       string            `json:"model"`
	Temperature *float64          `json:"temperature,omitempty"`
	TopP        *float64          `json:"top_p,omitempty"`
	Mode        AgentMode         `json:"mode"`
	Permissions *PermissionConfig `json:"permissions,omitempty"`
	Tools       map[string]bool   `json:"tools,omitempty"`
}

type ConfigBuilder struct {
	config *Config
}

func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: &Config{
			Permissions: NewPermissions().Build(),
		},
	}
}

func (b *ConfigBuilder) AddLocalMCP(name string, command []string, env map[string]string) *ConfigBuilder {
	if b.config.mcpRegistry == nil {
		b.config.mcpRegistry = NewMCPRegistry()
	}
	server := LocalMCPServer(name, command, WithEnv(env))
	_ = b.config.mcpRegistry.Add(server)
	return b
}

func (b *ConfigBuilder) AddRemoteMCP(name string, url string, headers map[string]string) *ConfigBuilder {
	if b.config.mcpRegistry == nil {
		b.config.mcpRegistry = NewMCPRegistry()
	}
	server := RemoteMCPServer(name, url, WithHeaders(headers))
	_ = b.config.mcpRegistry.Add(server)
	return b
}

func (b *ConfigBuilder) WithModel(model string) *ConfigBuilder {
	b.config.Model = model
	return b
}

func (b *ConfigBuilder) WithSmallModel(model string) *ConfigBuilder {
	b.config.SmallModel = model
	return b
}

func (b *ConfigBuilder) WithAgent(name string, config AgentConfig) *ConfigBuilder {
	if b.config.agents == nil {
		b.config.agents = make(map[string]AgentConfig)
	}
	b.config.agents[name] = config
	return b
}

func (b *ConfigBuilder) WithPermissions(perm *PermissionConfig) *ConfigBuilder {
	b.config.Permissions = perm
	return b
}

func (b *ConfigBuilder) Build() *Config {
	return b.config
}

type configFile struct {
	Schema     string                   `json:"$schema,omitempty"`
	Model      string                   `json:"model,omitempty"`
	SmallModel string                   `json:"small_model,omitempty"`
	Agent      map[string]AgentConfig   `json:"agent,omitempty"`
	Permission *PermissionConfig        `json:"permission,omitempty"`
	MCP        map[string]mcpConfigFile `json:"mcp,omitempty"`
	Tools      map[string]bool          `json:"tools,omitempty"`
}

type mcpConfigFile struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, err
	}

	config := &Config{
		Model:       cf.Model,
		SmallModel:  cf.SmallModel,
		Permissions: cf.Permission,
		agents:      cf.Agent,
		Tools:       cf.Tools,
	}

	if cf.MCP != nil {
		registry := NewMCPRegistry()
		for name, mcp := range cf.MCP {
			var server *MCPServer
			switch mcp.Type {
			case "local":
				server = LocalMCPServer(name, mcp.Command, WithEnv(mcp.Environment), WithEnabled(mcp.Enabled))
			case "remote":
				server = RemoteMCPServer(name, mcp.URL, WithHeaders(mcp.Headers), WithEnabled(mcp.Enabled))
			}
			if server != nil {
				_ = registry.Add(server)
			}
		}
		config.mcpRegistry = registry
	}

	return config, nil
}

func LoadConfigFromDir(dir string) (*Config, error) {
	configNames := []string{"opencode.json", ".opencode.json", "opencode.config.json"}

	for _, name := range configNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return LoadConfig(path)
		}
	}

	return nil, os.ErrNotExist
}

func SaveConfig(config *Config, path string) error {
	cf := configFile{
		Model:      config.Model,
		SmallModel: config.SmallModel,
		Permission: config.Permissions,
		Agent:      config.agents,
		Tools:      config.Tools,
	}

	if config.mcpRegistry != nil {
		cf.MCP = make(map[string]mcpConfigFile)
		for _, server := range config.mcpRegistry.List() {
			mcp := mcpConfigFile{
				Type:    string(server.Type),
				Enabled: server.Enabled,
			}
			if server.Type == MCPServerTypeLocal {
				mcp.Command = server.Command
				mcp.Environment = server.Environment
			} else {
				mcp.URL = server.URL
				mcp.Headers = server.Headers
			}
			cf.MCP[server.Name] = mcp
		}
	}

	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c *Config) GetMCPServers() []*MCPServer {
	if c.mcpRegistry == nil {
		return nil
	}
	return c.mcpRegistry.List()
}

func (c *Config) GetAgents() map[string]AgentConfig {
	if c.agents == nil {
		return make(map[string]AgentConfig)
	}
	return c.agents
}
