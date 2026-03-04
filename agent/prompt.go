package agent

import (
	"os"

	opencode "github.com/sst/opencode-sdk-go"
)

type PromptPart interface {
	ToInput() (opencode.SessionPromptParamsPartUnion, error)
}

type TextPart struct {
	content string
}

func Text(content string) PromptPart {
	return &TextPart{content: content}
}

func (p *TextPart) ToInput() (opencode.SessionPromptParamsPartUnion, error) {
	return opencode.TextPartInputParam{
		Type: opencode.F(opencode.TextPartInputTypeText),
		Text: opencode.F(p.content),
	}, nil
}

type FilePart struct {
	path    string
	content []byte
	mime    string
}

func File(path string) PromptPart {
	return &FilePart{path: path}
}

func FileFromContent(path string, content []byte, mime string) PromptPart {
	return &FilePart{
		path:    path,
		content: content,
		mime:    mime,
	}
}

func (p *FilePart) ToInput() (opencode.SessionPromptParamsPartUnion, error) {
	params := opencode.FilePartInputParam{
		Type: opencode.F(opencode.FilePartInputTypeFile),
	}

	if p.path != "" {
		params.Filename = opencode.F(p.path)
	}

	if p.mime != "" {
		params.Mime = opencode.F(p.mime)
	}

	return params, nil
}

func (p *FilePart) GetContent() ([]byte, error) {
	if p.content != nil {
		return p.content, nil
	}

	if p.path == "" {
		return nil, nil
	}

	return os.ReadFile(p.path)
}

type SymbolPart struct {
	path string
	name string
	kind int
}

func Symbol(path string, name string, kind int) PromptPart {
	return &SymbolPart{
		path: path,
		name: name,
		kind: kind,
	}
}

func (p *SymbolPart) ToInput() (opencode.SessionPromptParamsPartUnion, error) {
	return opencode.FilePartInputParam{
		Type:     opencode.F(opencode.FilePartInputTypeFile),
		Filename: opencode.F(p.path),
	}, nil
}

type AgentRefPart struct {
	name string
}

func AgentRef(name string) PromptPart {
	return &AgentRefPart{name: name}
}

func (p *AgentRefPart) ToInput() (opencode.SessionPromptParamsPartUnion, error) {
	return opencode.AgentPartInputParam{
		Type: opencode.F(opencode.AgentPartInputTypeAgent),
		Name: opencode.F(p.name),
	}, nil
}

type PromptConfig struct {
	Parts       []PromptPart
	Context     string
	System      string
	Temperature *float64
	Model       string
	Agent       string
}

type PromptOption func(*PromptConfig)

func WithFiles(paths ...string) PromptOption {
	return func(c *PromptConfig) {
		for _, path := range paths {
			c.Parts = append(c.Parts, File(path))
		}
	}
}

func WithParts(parts ...PromptPart) PromptOption {
	return func(c *PromptConfig) {
		c.Parts = append(c.Parts, parts...)
	}
}

func WithContext(ctx string) PromptOption {
	return func(c *PromptConfig) {
		c.Context = ctx
	}
}

func WithSystemPrompt(prompt string) PromptOption {
	return func(c *PromptConfig) {
		c.System = prompt
	}
}

func WithTemperature(temp float64) PromptOption {
	return func(c *PromptConfig) {
		c.Temperature = &temp
	}
}

func WithPromptModel(model string) PromptOption {
	return func(c *PromptConfig) {
		c.Model = model
	}
}

func WithAgent(name string) PromptOption {
	return func(c *PromptConfig) {
		c.Agent = name
	}
}

type PromptBuilder struct {
	parts []PromptPart
}

func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		parts: make([]PromptPart, 0),
	}
}

func (b *PromptBuilder) AddText(content string) *PromptBuilder {
	b.parts = append(b.parts, Text(content))
	return b
}

func (b *PromptBuilder) AddFile(path string) *PromptBuilder {
	b.parts = append(b.parts, File(path))
	return b
}

func (b *PromptBuilder) AddFileFromContent(path string, content []byte, mime string) *PromptBuilder {
	b.parts = append(b.parts, FileFromContent(path, content, mime))
	return b
}

func (b *PromptBuilder) AddSymbol(path string, name string, kind int) *PromptBuilder {
	b.parts = append(b.parts, Symbol(path, name, kind))
	return b
}

func (b *PromptBuilder) AddAgentRef(name string) *PromptBuilder {
	b.parts = append(b.parts, AgentRef(name))
	return b
}

func (b *PromptBuilder) Build(opts ...PromptOption) *PromptConfig {
	config := &PromptConfig{
		Parts: b.parts,
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}
