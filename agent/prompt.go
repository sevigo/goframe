package agent

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

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
	content, err := p.GetContent()
	if err != nil {
		return nil, err
	}

	mime := p.mime
	if mime == "" {
		mime = detectMimeType(p.path)
	}

	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content)

	params := opencode.FilePartInputParam{
		Type: opencode.F(opencode.FilePartInputTypeFile),
		Mime: opencode.F(mime),
		URL:  opencode.F(dataURL),
	}

	if p.path != "" {
		params.Filename = opencode.F(p.path)
	}

	return params, nil
}

func detectMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "text/x-go"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".tsx":
		return "text/typescript-jsx"
	case ".jsx":
		return "text/javascript-jsx"
	case ".py":
		return "text/x-python"
	case ".rb":
		return "text/x-ruby"
	case ".java":
		return "text/x-java"
	case ".c", ".h":
		return "text/x-c"
	case ".cpp", ".hpp", ".cc":
		return "text/x-c++"
	case ".rs":
		return "text/x-rust"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".xml":
		return "text/xml"
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".md":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".sh":
		return "text/x-shellscript"
	case ".toml":
		return "text/x-toml"
	default:
		return "text/plain"
	}
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
	path    string
	name    string
	kind    int
	content []byte
}

func Symbol(path string, name string, kind int) PromptPart {
	return &SymbolPart{
		path: path,
		name: name,
		kind: kind,
	}
}

func SymbolFromContent(path string, name string, kind int, content []byte) PromptPart {
	return &SymbolPart{
		path:    path,
		name:    name,
		kind:    kind,
		content: content,
	}
}

func (p *SymbolPart) ToInput() (opencode.SessionPromptParamsPartUnion, error) {
	content := p.content
	var err error
	if content == nil && p.path != "" {
		content, err = os.ReadFile(p.path)
		if err != nil {
			return nil, err
		}
	}

	mime := detectMimeType(p.path)
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content)

	return opencode.FilePartInputParam{
		Type:     opencode.F(opencode.FilePartInputTypeFile),
		Filename: opencode.F(p.path),
		Mime:     opencode.F(mime),
		URL:      opencode.F(dataURL),
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
