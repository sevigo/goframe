package golang_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/parsers/golang"
	logger "github.com/sevigo/goframe/parsers/testing"
	"github.com/sevigo/goframe/schema"
)

func TestGoPlugin(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	assert.Equal(t, "go", plugin.Name())
	assert.Contains(t, plugin.Extensions(), ".go")

	goContent := `package main

import (
	"fmt"
	"strings"
)

// AppVersion holds the application version
const AppVersion = "1.0.0"

// Debug flag for development
var Debug = false

// UserService provides user-related operations
type UserService interface {
	GetUser(id string) (*User, error)
	CreateUser(user *User) error
}

// Person represents a person with basic information
type Person struct {
	Name string
	Age  int
}

// SayHello prints a greeting message
func (p *Person) SayHello() {
	fmt.Printf("Hello, my name is %s\n", p.Name)
}

// GetAge returns the person's age
func (p *Person) GetAge() int {
	return p.Age
}

// CreatePerson creates a new person instance
func CreatePerson(name string, age int) *Person {
	return &Person{Name: name, Age: age}
}

func main() {
	p := CreatePerson("Alice", 30)
	p.SayHello()
	fmt.Printf("Age: %d\n", p.GetAge())
}
`

	// Test chunking
	chunks, err := plugin.Chunk(goContent, "test.go", &schema.CodeChunkingOptions{})
	require.NoError(t, err)

	// Should have detected chunks for various elements
	assert.GreaterOrEqual(t, len(chunks), 6) // const, interface, struct, 3 functions, main

	// Verify chunk types and content
	chunkTypes := make(map[string]int)
	identifiers := make(map[string]bool)

	for _, chunk := range chunks {
		t.Logf("Chunk: Type=%s, Identifier=%s, Lines=%d-%d",
			chunk.Type, chunk.Identifier, chunk.LineStart, chunk.LineEnd)
		chunkTypes[chunk.Type]++
		identifiers[chunk.Identifier] = true

		// Verify all chunks have required fields
		assert.NotEmpty(t, chunk.Content)
		assert.Greater(t, chunk.LineStart, 0)
		assert.GreaterOrEqual(t, chunk.LineEnd, chunk.LineStart)
		assert.NotEmpty(t, chunk.Type)
		assert.NotEmpty(t, chunk.Identifier)
		assert.NotNil(t, chunk.Annotations)
	}

	// Verify expected chunks exist
	expectedChunks := map[string]string{
		"AppVersion":           "constant",
		"Debug":                "variable",
		"UserService":          "type_declaration",
		"Person":               "type_declaration",
		"(p *Person) SayHello": "function",
		"(p *Person) GetAge":   "function",
		"CreatePerson":         "function",
		"main":                 "function",
	}

	for identifier := range expectedChunks {
		assert.True(t, identifiers[identifier], "Expected chunk with identifier '%s' not found", identifier)
	}

	// Verify chunk type counts
	assert.Greater(t, chunkTypes["function"], 2, "Should have multiple function chunks")
	assert.Greater(t, chunkTypes["type_declaration"], 0, "Should have type declaration chunks")

	// Test metadata extraction
	metadata, err := plugin.ExtractMetadata(goContent, "test.go")
	require.NoError(t, err)

	assert.Equal(t, "go", metadata.Language)
	assert.Contains(t, metadata.Imports, "fmt")
	assert.Contains(t, metadata.Imports, "strings")

	// Should have extracted definitions for various elements
	assert.GreaterOrEqual(t, len(metadata.Definitions), 7)

	// Verify definitions
	definitionTypes := make(map[string]int)
	definitionNames := make(map[string]bool)

	for _, def := range metadata.Definitions {
		t.Logf("Definition: Type=%s, Name=%s, Visibility=%s", def.Type, def.Name, def.Visibility)
		definitionTypes[def.Type]++
		definitionNames[def.Name] = true

		// Verify all definitions have required fields
		assert.NotEmpty(t, def.Type)
		assert.NotEmpty(t, def.Name)
		assert.Positive(t, def.LineStart)
		assert.GreaterOrEqual(t, def.LineEnd, def.LineStart)
		assert.Contains(t, []string{"public", "private"}, def.Visibility)
	}

	// Verify expected definitions exist
	expectedDefinitions := []string{
		"AppVersion", "Debug", "UserService", "Person",
		"SayHello", "GetAge", "CreatePerson", "main",
	}

	for _, name := range expectedDefinitions {
		assert.True(t, definitionNames[name], "Expected definition '%s' not found", name)
	}

	// Verify definition type counts
	assert.Greater(t, definitionTypes["function"]+definitionTypes["method"], 3)
	assert.Positive(t, definitionTypes["struct"]+definitionTypes["interface"])

	// Verify symbols
	assert.GreaterOrEqual(t, len(metadata.Symbols), 7)

	// Verify properties
	assert.Equal(t, "main", metadata.Properties["package"])
	assert.NotEmpty(t, metadata.Properties["total_functions"])
	assert.NotEmpty(t, metadata.Properties["total_types"])
	assert.NotEmpty(t, metadata.Properties["total_methods"])
}

func TestGoPluginGroupedDeclarations(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	// Test with grouped declarations
	groupedGoContent := `package example

const (
	MaxRetries = 3
	MaxTimeout = 30
)

var (
	Debug   = false
	Verbose = true
)
`

	chunks, err := plugin.Chunk(groupedGoContent, "grouped.go", &schema.CodeChunkingOptions{})
	require.NoError(t, err)

	// Should create grouped chunks
	foundGroupedConst := false
	foundGroupedVar := false

	for _, chunk := range chunks {
		t.Logf("Grouped chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)

		if chunk.Type == "constant" && strings.Contains(chunk.Identifier, "MaxRetries") && strings.Contains(chunk.Identifier, "MaxTimeout") {
			foundGroupedConst = true
			assert.Equal(t, "MaxRetries, MaxTimeout", chunk.Identifier)
		}

		if chunk.Type == "variable" && strings.Contains(chunk.Identifier, "Debug") && strings.Contains(chunk.Identifier, "Verbose") {
			foundGroupedVar = true
			assert.Equal(t, "Debug, Verbose", chunk.Identifier)
		}
	}

	assert.True(t, foundGroupedConst, "Should find grouped constant declaration")
	assert.True(t, foundGroupedVar, "Should find grouped variable declaration")
}

func TestGoPluginDocComments(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	goContentWithDocs := `package example

// MaxRetries defines the maximum number of retry attempts
const MaxRetries = 3

// Logger interface for structured logging
type Logger interface {
	// Info logs an informational message
	Info(msg string)
	// Error logs an error message  
	Error(msg string, err error)
}

// Calculator performs mathematical operations
type Calculator struct {
	precision int
}

// Add performs addition of two numbers
// It returns the sum as a float64
func (c *Calculator) Add(a, b float64) float64 {
	return a + b
}
`

	// Test chunking with documentation
	chunks, err := plugin.Chunk(goContentWithDocs, "example.go", &schema.CodeChunkingOptions{})
	require.NoError(t, err)

	// Verify chunks include documentation
	foundDocumentedChunks := 0
	for _, chunk := range chunks {
		if chunk.Annotations["has_doc"] == "true" {
			foundDocumentedChunks++
			t.Logf("Documented chunk content preview: %s", chunk.Content[:min(100, len(chunk.Content))])

			// Just verify that we have documentation content
			// Documentation may be in cleaned form (without // markers) or raw form
			lines := strings.Split(chunk.Content, "\n")
			hasDocContent := false

			// Look for documentation content in the first few lines
			for i, line := range lines[:min(3, len(lines))] {
				line = strings.TrimSpace(line)
				// Check if this line contains documentation keywords from our test
				if strings.Contains(line, "MaxRetries defines") ||
					strings.Contains(line, "Logger interface") ||
					strings.Contains(line, "Calculator performs") ||
					strings.Contains(line, "Add performs addition") {
					hasDocContent = true
					t.Logf("  Found doc content in line [%d]: %q", i, line)
					break
				}
			}

			assert.True(t, hasDocContent, "Chunk with has_doc=true should contain recognizable documentation content")
		}
	}

	assert.Greater(t, foundDocumentedChunks, 0, "Should find chunks with documentation")

	// Test metadata extraction with documentation
	metadata, err := plugin.ExtractMetadata(goContentWithDocs, "example.go")
	require.NoError(t, err)

	// Verify definitions include documentation
	foundDocumentedDefs := 0
	for _, def := range metadata.Definitions {
		if def.Documentation != "" {
			foundDocumentedDefs++
			t.Logf("Definition '%s' has documentation: %s", def.Name, def.Documentation[:min(50, len(def.Documentation))])
		}
	}

	assert.Greater(t, foundDocumentedDefs, 0, "Should find definitions with documentation")
}

func TestGoPluginComplexTypes(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	complexGoContent := `package advanced

import (
	"context"
	"sync"
)

// Repository defines data access operations
type Repository[T any] interface {
	Find(ctx context.Context, id string) (T, error)
	Save(ctx context.Context, entity T) error
}

// Cache provides caching functionality
type Cache struct {
	mu    sync.RWMutex
	data  map[string]interface{}
	ttl   map[string]int64
}

// Service handles business logic
type Service struct {
	repo  Repository[string]
	cache *Cache
}

// ProcessBatch handles batch processing
func (s *Service) ProcessBatch(ctx context.Context, items []string) error {
	// Implementation here
	return nil
}

// Helper function type
type HandlerFunc func(ctx context.Context, data []byte) error

// Channel operations
func processChannel(input <-chan string, output chan<- string) {
	// Implementation here
}
`

	// Test chunking with complex types
	chunks, err := plugin.Chunk(complexGoContent, "advanced.go", &schema.CodeChunkingOptions{})
	require.NoError(t, err)

	// Should handle various Go constructs
	assert.GreaterOrEqual(t, len(chunks), 4)

	foundTypes := make(map[string]bool)
	for _, chunk := range chunks {
		foundTypes[chunk.Type] = true
		t.Logf("Complex chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)
	}

	assert.True(t, foundTypes["type_declaration"], "Should find type declarations")
	assert.True(t, foundTypes["function"], "Should find functions")
}

func TestGoPluginErrorHandling(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	// Test with invalid Go code
	invalidGoContent := `package invalid
	func incomplete(`

	// Should return error for unparseable code
	chunks, err := plugin.Chunk(invalidGoContent, "invalid.go", &schema.CodeChunkingOptions{})
	assert.Error(t, err)
	assert.Nil(t, chunks)

	// Test metadata extraction with invalid code
	metadata, err := plugin.ExtractMetadata(invalidGoContent, "invalid.go")
	assert.Error(t, err)
	assert.Equal(t, "go", metadata.Language)
}

func TestGoPlugin_Chunk_InvalidCode(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	// Test with invalid Go code (incomplete function)
	invalidGoContent := `package main
	func IncompleteFunc(a int`

	chunks, err := plugin.Chunk(invalidGoContent, "invalid_incomplete_func.go", &schema.CodeChunkingOptions{})
	assert.Error(t, err, "Chunking invalid Go code should return an error")
	assert.Contains(t, err.Error(), "failed to parse Go file", "Error message should indicate parsing failure")
	assert.Nil(t, chunks, "Chunks should be nil when parsing fails")

	// Test with invalid Go code (syntax error)
	invalidGoContentSyntax := `package main
	func ValidFunc() {
		a := 1 +
	}`
	chunks, err = plugin.Chunk(invalidGoContentSyntax, "invalid_syntax.go", &schema.CodeChunkingOptions{})
	assert.Error(t, err, "Chunking Go code with syntax error should return an error")
	assert.Contains(t, err.Error(), "failed to parse Go file", "Error message should indicate parsing failure")
	assert.Nil(t, chunks, "Chunks should be nil when parsing fails")
}

func TestGoPlugin_ExtractMetadata_InvalidCode(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	// Test metadata extraction with invalid Go code (incomplete function)
	invalidGoContent := `package main
	func IncompleteFunc(a int`

	metadata, err := plugin.ExtractMetadata(invalidGoContent, "invalid_incomplete_func.go")
	assert.Error(t, err, "ExtractMetadata on invalid Go code should return an error")
	assert.Contains(t, err.Error(), "failed to parse Go file", "Error message should indicate parsing failure")
	// Even with an error, some basic metadata might be populated or default
	assert.Equal(t, "go", metadata.Language, "Language should still be 'go'")
	assert.Empty(t, metadata.Imports, "Imports should be empty on parsing failure")
	assert.Empty(t, metadata.Definitions, "Definitions should be empty on parsing failure")
	assert.Empty(t, metadata.Symbols, "Symbols should be empty on parsing failure")
	assert.NotNil(t, metadata.Properties, "Properties map should not be nil")

	// Test metadata extraction with invalid Go code (syntax error)
	invalidGoContentSyntax := `package main
	func ValidFunc() {
		a := 1 +
	}`
	metadata, err = plugin.ExtractMetadata(invalidGoContentSyntax, "invalid_syntax.go")
	assert.Error(t, err, "ExtractMetadata on Go code with syntax error should return an error")
	assert.Contains(t, err.Error(), "failed to parse Go file", "Error message should indicate parsing failure")
	assert.Equal(t, "go", metadata.Language)
}

func TestGoPlugin_Chunk_EmptyContent(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	// Test with empty content
	chunks, err := plugin.Chunk("", "empty.go", &schema.CodeChunkingOptions{})
	assert.Error(t, err, "Chunking empty content should return an error")
	// The go/parser might return "expected 'package', found 'EOF'" or similar
	assert.Contains(t, err.Error(), "failed to parse Go file", "Error message should indicate parsing failure for empty file")
	assert.Nil(t, chunks, "Chunks should be nil for empty content")
}

func TestGoPlugin_ExtractMetadata_EmptyContent(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	metadata, err := plugin.ExtractMetadata("", "empty.go")
	assert.Error(t, err, "ExtractMetadata on empty content should return an error")
	assert.Contains(t, err.Error(), "failed to parse Go file", "Error message should indicate parsing failure for empty file")
	assert.Equal(t, "go", metadata.Language)
}

func TestGoPlugin_Chunk_NoSemanticChunks(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	contentOnlyComments := `package main

// This is a file with only comments and an empty main
// func main() {}
// var x = 1 // unexported
`
	chunks, err := plugin.Chunk(contentOnlyComments, "only_comments.go", &schema.CodeChunkingOptions{})
	require.NoError(t, err)
	assert.Equal(t, chunks, []schema.CodeChunk{})
}

func TestGoPlugin_CanHandle(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(log)

	testCases := []struct {
		name        string
		filePathArg string
		isDir       bool
		isNilInfo   bool
		expected    bool
	}{
		{"Go file", "main.go", false, false, true},
		{"Go test file", "main_test.go", false, false, false},
		{"Non-Go file", "main.txt", false, false, false},
		{"Directory", "mydir", true, false, false},
		{"No extension", "main", false, false, false},
		{"Hidden Go file", ".main.go", false, false, true},
		{"Path with test", "test/main.go", false, false, true},
		{"Path with _test dir", "_test/main.go", false, false, true},
		{"Nil FileInfo (should rely on ext)", "some.go", false, true, true},
		{"Nil FileInfo, test file", "other_test.go", false, true, false},
		{"Empty path", "", false, false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fileInfo fs.FileInfo // Use fs.FileInfo which is what CanHandle expects

			if !tc.isNilInfo {
				var tempPath string
				var err error

				if tc.isDir {
					// Create a temporary directory
					// Pass a pattern that doesn't necessarily match tc.filePathArg's extension
					// as the name of the temp dir doesn't matter for this test, only its type.
					tempPath, err = os.MkdirTemp("", "testdir_*")
					require.NoError(t, err)
					defer os.RemoveAll(tempPath)
				} else {
					// Create a temporary file
					// Pass a pattern that doesn't necessarily match tc.filePathArg's extension
					tmpFile, errCreate := os.CreateTemp("", "testfile_*"+filepath.Ext(tc.filePathArg))
					require.NoError(t, errCreate)
					tempPath = tmpFile.Name()
					tmpFile.Close() // Close immediately, we only need Stat
					defer os.Remove(tempPath)
				}
				fileInfo, err = os.Stat(tempPath)
				require.NoError(t, err)
			}
			// If tc.isNilInfo is true, fileInfo remains nil

			// The first argument to CanHandle is the path string we want to test,
			// which uses the intended filename and extension from the test case.
			// The fileInfo is only used to check if it's a directory or for other attributes if needed.
			assert.Equal(t, tc.expected, plugin.CanHandle(tc.filePathArg, fileInfo))
		})
	}
}
