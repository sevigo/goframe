package documentloaders_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/documentloaders"
	"github.com/sevigo/goframe/parsers"
	"github.com/sevigo/goframe/schema"
)

// fakeGoParser simulates a language-specific parser for Go files.
type fakeGoParser struct{}

func (p *fakeGoParser) Name() string         { return "go" }
func (p *fakeGoParser) Extensions() []string { return []string{".go"} }
func (p *fakeGoParser) CanHandle(path string, info fs.FileInfo) bool {
	return filepath.Ext(path) == ".go"
}
func (p *fakeGoParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	// Simulate finding two functions in a Go file.
	return []schema.CodeChunk{
		{Content: "package main\n\nfunc main() {}", LineStart: 1, LineEnd: 3, Type: "function", Identifier: "main"},
		{Content: "func helper() {}", LineStart: 5, LineEnd: 5, Type: "function", Identifier: "helper"},
	}, nil
}
func (p *fakeGoParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	return schema.FileMetadata{}, nil // Not needed for this test
}

// fakeTextParser simulates a fallback parser for generic text files.
type fakeTextParser struct{}

func (p *fakeTextParser) Name() string         { return "text" }
func (p *fakeTextParser) Extensions() []string { return []string{".txt"} }
func (p *fakeTextParser) CanHandle(path string, info fs.FileInfo) bool {
	return true // Acts as a general fallback
}
func (p *fakeTextParser) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	// Simulate creating a single chunk for the whole text file.
	return []schema.CodeChunk{
		{Content: content, LineStart: 1, LineEnd: 1, Type: "text_document", Identifier: "README"},
	}, nil
}
func (p *fakeTextParser) ExtractMetadata(content string, path string) (schema.FileMetadata, error) {
	return schema.FileMetadata{}, nil // Not needed for this test
}

// fakeParserRegistry provides the correct fake parser based on file type.
type fakeParserRegistry struct {
	goParser   schema.ParserPlugin
	textParser schema.ParserPlugin
}

func newFakeRegistry() parsers.ParserRegistry {
	return &fakeParserRegistry{
		goParser:   &fakeGoParser{},
		textParser: &fakeTextParser{},
	}
}
func (r *fakeParserRegistry) RegisterParser(plugin schema.ParserPlugin) error { return nil }
func (r *fakeParserRegistry) GetParser(language string) (schema.ParserPlugin, error) {
	if language == "go" {
		return r.goParser, nil
	}
	return r.textParser, nil
}
func (r *fakeParserRegistry) GetParserForFile(path string, info fs.FileInfo) (schema.ParserPlugin, error) {
	if r.goParser.CanHandle(path, info) {
		return r.goParser, nil
	}
	return r.textParser, nil
}
func (r *fakeParserRegistry) GetParserForExtension(ext string) (schema.ParserPlugin, error) {
	return nil, nil
}
func (r *fakeParserRegistry) GetAllParsers() []schema.ParserPlugin { return nil }

// --- Main Test Function ---

func TestGitLoader_Load(t *testing.T) {
	// 1. Arrange: Set up the in-memory file system and fake components.
	mockFS := fstest.MapFS{
		// A Go file that should be chunked by fakeGoParser
		"src/main.go": {Data: []byte("package main\n\nfunc main() {}\n\nfunc helper() {}")},
		// A text file that should be chunked by fakeTextParser
		"README.txt": {Data: []byte("This is a test README.")},
		// A file that should be skipped by shouldSkipFile
		"assets/logo.png": {Data: []byte("binary data")},
		// A directory that should be skipped by shouldSkipDir
		".git/config": {Data: []byte("some config")},
		// An empty directory to ensure it's handled correctly
		"empty_dir": {Mode: fs.ModeDir},
	}

	// Use a wrapper to make the in-memory FS compatible with filepath.WalkDir
	// by creating a temporary directory that mirrors the in-memory structure.
	tempDir := t.TempDir()
	err := fs.WalkDir(mockFS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		targetPath := filepath.Join(tempDir, path)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		data, readErr := mockFS.ReadFile(path)
		require.NoError(t, readErr)
		return os.WriteFile(targetPath, data, 0644)
	})
	require.NoError(t, err)

	registry := newFakeRegistry()
	loader := documentloaders.NewGit(tempDir, registry)

	docs, err := loader.Load(context.Background())

	require.NoError(t, err, "Load should not return an error")
	require.NotNil(t, docs, "Load should return documents")

	assert.Len(t, docs, 3, "Expected 3 documents to be loaded and chunked")

	// Create maps to check for presence and correctness, as order is not guaranteed.
	foundMain := false
	foundHelper := false
	foundReadme := false

	for _, doc := range docs {
		source, ok := doc.Metadata["source"].(string)
		require.True(t, ok, "Document metadata must have a 'source' key")

		identifier, ok := doc.Metadata["identifier"].(string)
		require.True(t, ok, "Document metadata must have an 'identifier' key")

		switch source {
		case "src/main.go":
			switch identifier {
			case "main":
				assert.Equal(t, "package main\n\nfunc main() {}", doc.PageContent)
				assert.Equal(t, "function", doc.Metadata["chunk_type"])
				foundMain = true
			case "helper":
				assert.Equal(t, "func helper() {}", doc.PageContent)
				assert.Equal(t, "function", doc.Metadata["chunk_type"])
				foundHelper = true
			}
		case "README.txt":
			assert.Equal(t, "This is a test README.", doc.PageContent)
			assert.Equal(t, "text_document", doc.Metadata["chunk_type"])
			foundReadme = true
		default:
			t.Errorf("Unexpected document source found: %s", source)
		}
	}

	assert.True(t, foundMain, "Did not find the 'main' function chunk")
	assert.True(t, foundHelper, "Did not find the 'helper' function chunk")
	assert.True(t, foundReadme, "Did not find the 'README' chunk")
}
