package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextPart(t *testing.T) {
	part := Text("Hello, world!")
	assert.NotNil(t, part)

	input, err := part.ToInput()
	assert.NoError(t, err)
	assert.NotNil(t, input)
}

func TestFilePart(t *testing.T) {
	t.Run("file from path", func(t *testing.T) {
		part := File("/path/to/file.go")
		assert.NotNil(t, part)
	})

	t.Run("file from content", func(t *testing.T) {
		content := []byte("package main\n\nfunc main() {}")
		part := FileFromContent("main.go", content, "text/plain")
		assert.NotNil(t, part)
	})
}

func TestSymbolPart(t *testing.T) {
	part := Symbol("main.go", "main", 12)
	assert.NotNil(t, part)

	input, err := part.ToInput()
	assert.NoError(t, err)
	assert.NotNil(t, input)
}

func TestAgentRefPart(t *testing.T) {
	part := AgentRef("build-agent")
	assert.NotNil(t, part)

	input, err := part.ToInput()
	assert.NoError(t, err)
	assert.NotNil(t, input)
}

func TestPromptBuilder(t *testing.T) {
	t.Run("basic builder", func(t *testing.T) {
		builder := NewPromptBuilder()
		assert.NotNil(t, builder)
		assert.Len(t, builder.parts, 0)
	})

	t.Run("add text", func(t *testing.T) {
		builder := NewPromptBuilder().
			AddText("Hello")

		assert.Len(t, builder.parts, 1)
	})

	t.Run("add multiple parts", func(t *testing.T) {
		builder := NewPromptBuilder().
			AddText("Review this code:").
			AddFile("main.go").
			AddFile("utils.go").
			AddSymbol("main.go", "main", 12).
			AddAgentRef("reviewer")

		assert.Len(t, builder.parts, 5)
	})

	t.Run("build with options", func(t *testing.T) {
		temp := 0.7
		config := NewPromptBuilder().
			AddText("Test").
			Build(
				WithContext("Additional context"),
				WithTemperature(temp),
				WithAgent("custom-agent"),
			)

		assert.Len(t, config.Parts, 1)
		assert.Equal(t, "Additional context", config.Context)
		assert.Equal(t, temp, *config.Temperature)
		assert.Equal(t, "custom-agent", config.Agent)
	})
}

func TestPromptOptions(t *testing.T) {
	t.Run("with files", func(t *testing.T) {
		config := &PromptConfig{}
		WithFiles("main.go", "utils.go")(config)
		assert.Len(t, config.Parts, 2)
	})

	t.Run("with parts", func(t *testing.T) {
		config := &PromptConfig{}
		WithParts(Text("Hello"), File("test.go"))(config)
		assert.Len(t, config.Parts, 2)
	})

	t.Run("with context", func(t *testing.T) {
		config := &PromptConfig{}
		WithContext("context text")(config)
		assert.Equal(t, "context text", config.Context)
	})

	t.Run("with system prompt", func(t *testing.T) {
		config := &PromptConfig{}
		WithSystemPrompt("system instructions")(config)
		assert.Equal(t, "system instructions", config.System)
	})

	t.Run("with temperature", func(t *testing.T) {
		config := &PromptConfig{}
		WithTemperature(0.5)(config)
		assert.NotNil(t, config.Temperature)
		assert.Equal(t, 0.5, *config.Temperature)
	})

	t.Run("with model", func(t *testing.T) {
		config := &PromptConfig{}
		WithPromptModel("claude-3")(config)
		assert.Equal(t, "claude-3", config.Model)
	})

	t.Run("with agent", func(t *testing.T) {
		config := &PromptConfig{}
		WithAgent("reviewer")(config)
		assert.Equal(t, "reviewer", config.Agent)
	})
}
