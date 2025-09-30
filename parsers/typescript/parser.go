package typescript

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	_ "embed" // Required for go:embed

	"github.com/dop251/goja"
	"github.com/sevigo/goframe/schema" // Use your actual schema import path
)

//go:embed typescript.js
var tsCompilerSource string

//go:embed parser_logic.js
var tsParserLogic string

// Parser implements the schema.ParserPlugin interface for TypeScript.
type Parser struct {
	vm     *goja.Runtime
	mu     sync.Mutex
	logger *slog.Logger
}

// errorResponse is used to detect if the JS part returned an error.
type errorResponse struct {
	Error string `json:"error"`
}

// NewTypeScriptPlugin creates and initializes a new TypeScript parser that satisfies the ParserPlugin interface.
func NewTypeScriptPlugin(logger *slog.Logger) schema.ParserPlugin {
	vm := goja.New()

	// Add console polyfills that wire a slog.Logger to the JS runtime.
	_ = vm.Set("console", map[string]interface{}{
		"error": func(msg ...interface{}) {
			logger.Error("[JS]", "msg", fmt.Sprint(msg...))
		},
		"log": func(msg ...interface{}) {
			logger.Info("[JS]", "msg", fmt.Sprint(msg...))
		},
	})

	_, err := vm.RunString(tsCompilerSource)
	if err != nil {
		// Panic is appropriate here. If the embedded compiler is broken,
		// the application is in an unrecoverable state.
		panic(fmt.Sprintf("failed to load typescript.js: %v", err))
	}

	_, err = vm.RunString(tsParserLogic)
	if err != nil {
		panic(fmt.Sprintf("failed to load parser_logic.js: %v", err))
	}

	return &Parser{
		vm:     vm,
		logger: logger,
	}
}

// Name returns the name of the parser.
func (p *Parser) Name() string {
	return "typescript"
}

// Extensions returns the file extensions this parser handles.
func (p *Parser) Extensions() []string {
	return []string{".ts", ".tsx"}
}

// CanHandle determines if the parser can process a given file.
// It accepts .ts and .tsx files, excluding common test file patterns (*.test.*, *.spec.*).
func (p *Parser) CanHandle(path string, info fs.FileInfo) bool {
	if info != nil && info.IsDir() {
		return false
	}

	ext := filepath.Ext(path)
	if ext != ".ts" && ext != ".tsx" {
		return false
	}

	// Exclude test files by checking the base name.
	// e.g., "component.test.ts" -> "component.test"
	base := strings.TrimSuffix(path, ext)
	if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".spec") {
		return false
	}

	return true
}

// Chunk parses TypeScript code and divides it into logical code chunks.
// The 'opts' parameter is included to satisfy the ParserPlugin interface.
func (p *Parser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// opts is currently unused but available for future functionality.

	callable, ok := goja.AssertFunction(p.vm.Get("extractChunks"))
	if !ok {
		return nil, fmt.Errorf("extractChunks is not a function in JS runtime")
	}

	result, err := callable(goja.Undefined(), p.vm.ToValue(content), p.vm.ToValue(path))
	if err != nil {
		return nil, fmt.Errorf("failed to execute chunking script: %w", err)
	}

	resultStr := result.String()

	var errResp errorResponse
	if json.Unmarshal([]byte(resultStr), &errResp) == nil && errResp.Error != "" {
		return nil, fmt.Errorf("javascript parser error: %s", errResp.Error)
	}

	var chunks []schema.CodeChunk
	if err := json.Unmarshal([]byte(resultStr), &chunks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chunks from JS runtime: %w", err)
	}

	return chunks, nil
}

// ExtractMetadata parses TypeScript code to extract high-level metadata.
func (p *Parser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var metadata schema.FileMetadata

	callable, ok := goja.AssertFunction(p.vm.Get("extractMetadata"))
	if !ok {
		return metadata, fmt.Errorf("extractMetadata is not a function in JS runtime")
	}

	result, err := callable(goja.Undefined(), p.vm.ToValue(content), p.vm.ToValue(path))
	if err != nil {
		return metadata, fmt.Errorf("failed to execute metadata script: %w", err)
	}

	resultStr := result.String()

	var errResp errorResponse
	if json.Unmarshal([]byte(resultStr), &errResp) == nil && errResp.Error != "" {
		return metadata, fmt.Errorf("javascript parser error: %s", errResp.Error)
	}

	if err := json.Unmarshal([]byte(resultStr), &metadata); err != nil {
		return metadata, fmt.Errorf("failed to unmarshal metadata from JS runtime: %w", err)
	}

	metadata.Language = "typescript" // Set language in Go for consistency
	return metadata, nil
}
