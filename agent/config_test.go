package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sevigo/goframe/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_SaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config
	config := agent.NewConfigBuilder().
		WithModel("anthropic/claude-3-5-sonnet").
		WithSmallModel("anthropic/claude-3-haiku").
		AddLocalMCP("filesystem", []string{"mcp-filesystem", "/tmp"}, map[string]string{"LOG_LEVEL": "debug"}).
		WithPermissions(
			agent.NewPermissions().
				AllowBash("go test", "go build").
				AllowEdit().
				Build(),
		).
		Build()

	// Save config
	configPath := filepath.Join(tmpDir, "opencode.json")
	err = agent.SaveConfig(config, configPath)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Load config
	loadedConfig, err := agent.LoadConfig(configPath)
	require.NoError(t, err)

	// Verify loaded config
	assert.Equal(t, config.Model, loadedConfig.Model)
	assert.Equal(t, config.SmallModel, loadedConfig.SmallModel)
	assert.Len(t, loadedConfig.GetMCPServers(), 1)
}

func TestConfig_LoadFromDir(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// No config file should return error
	_, err = agent.LoadConfigFromDir(tmpDir)
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// Create config file
	config := agent.NewConfigBuilder().
		WithModel("test-model").
		Build()
	configPath := filepath.Join(tmpDir, "opencode.json")
	err = agent.SaveConfig(config, configPath)
	require.NoError(t, err)

	// Now load should work
	loadedConfig, err := agent.LoadConfigFromDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "test-model", loadedConfig.Model)
}

func TestMCPRegistry_ToConfigMap(t *testing.T) {
	registry := agent.NewMCPRegistry(
		agent.LocalMCPServer("local-server", []string{"mcp-server", "--port", "8080"}),
		agent.RemoteMCPServer("remote-server", "https://mcp.example.com"),
	)

	configMap := registry.ToConfigMap()
	assert.Len(t, configMap, 2)

	// Check local server config
	localConfig, ok := configMap["local-server"]
	assert.True(t, ok)
	assert.Equal(t, "local", string(localConfig.Type))

	// Check remote server config
	remoteConfig, ok := configMap["remote-server"]
	assert.True(t, ok)
	assert.Equal(t, "remote", string(remoteConfig.Type))
}

func TestPromptBuilder(t *testing.T) {
	builder := agent.NewPromptBuilder().
		AddText("Hello ").
		AddFile("main.go").
		AddText(" Review this").
		AddSymbol("main.go", "main", 12)

	config := builder.Build()
	require.Len(t, config.Parts, 4)

	// Test with options
	configWithOptions := builder.Build(
		agent.WithContext("Additional context"),
		agent.WithTemperature(0.7),
		agent.WithAgent("reviewer"),
	)

	assert.Equal(t, "Additional context", configWithOptions.Context)
	assert.NotNil(t, configWithOptions.Temperature)
	assert.Equal(t, 0.7, *configWithOptions.Temperature)
	assert.Equal(t, "reviewer", configWithOptions.Agent)
}
