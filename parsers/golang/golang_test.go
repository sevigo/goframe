package golang_test

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/sevigo/goframe/parsers/golang"
	logger "github.com/sevigo/goframe/parsers/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testGoFileContent = `package main

import (
	"fmt"
)

const AppVersion = "1.0.0"
var Debug = false

// UserService defines operations for a user.
type UserService interface {
	GetUser(id int) string
}

// Person represents a person with a name and age.
type Person struct {
	Name string
	Age  int
}

// SayHello is a method on the Person struct.
func (p *Person) SayHello() {
	fmt.Printf("Hello, my name is %s\n", p.Name)
}

// GetAge returns the person's age.
func (p *Person) GetAge() int {
	return p.Age
}

// CreatePerson is a factory function.
func CreatePerson(name string, age int) *Person {
	return &Person{Name: name, Age: age}
}

func main() {
	p := CreatePerson("Alice", 30)
	p.SayHello()
}
`

func TestGoPlugin_Chunking_WithGrouping(t *testing.T) {
	logger, _ := logger.NewTestLogger(t)
	plugin := golang.NewGoPlugin(logger)

	t.Run("should create a single chunk for a small file", func(t *testing.T) {
		chunks, err := plugin.Chunk(testGoFileContent, "test.go", nil)
		require.NoError(t, err)

		// VERIFY NEW BEHAVIOR: Only one chunk is created.
		require.Len(t, chunks, 1, "Expected a single chunk because the content is smaller than targetChunkSize")

		chunk := chunks[0]
		assert.Equal(t, "code_group", chunk.Type)
		assert.Contains(t, chunk.Content, "package main", "Chunk should start with package declaration")
		assert.Contains(t, chunk.Content, `import (
	"fmt"
)`, "Chunk should contain the import block")
		assert.Contains(t, chunk.Content, "const AppVersion = \"1.0.0\"", "Chunk should contain const declarations")
		assert.Contains(t, chunk.Content, "type Person struct", "Chunk should contain type declarations")
		assert.Contains(t, chunk.Content, "func (p *Person) SayHello()", "Chunk should contain method declarations")
		assert.Contains(t, chunk.Content, "func main()", "Chunk should contain function declarations")
	})

	t.Run("should create multiple chunks for a large file", func(t *testing.T) {
		var largeContent strings.Builder
		largeContent.WriteString("package main\n\nimport \"fmt\"\n\n")

		// Create content large enough to force multiple chunks (targetChunkSize is 3000)
		for i := range 15 {
			largeContent.WriteString(fmt.Sprintf(`
// Function number %d is a long function.
func function%d() {
    fmt.Println("This is a function to make the content larger than the threshold.")
    fmt.Println("It has multiple lines to ensure it takes up significant space in the file.")
    fmt.Println("Line 1 of content to simulate real code.")
    fmt.Println("Line 2 of content to simulate real code.")
    fmt.Println("Line 3 of content to simulate real code.")
    fmt.Println("Line 4 of content to simulate real code.")
    fmt.Println("Line 5 of content to simulate real code.")
    fmt.Println("Line 6 of content to simulate real code.")
    fmt.Println("Line 7 of content to simulate real code.")
    fmt.Println("Line 8 of content to simulate real code.")
    fmt.Println("Line 9 of content to simulate real code.")
}

`, i, i))
		}

		chunks, err := plugin.Chunk(largeContent.String(), "large.go", nil)
		require.NoError(t, err)

		// VERIFY NEW BEHAVIOR: More than one chunk is created.
		assert.Greater(t, len(chunks), 1, "Expected multiple chunks for a file larger than targetChunkSize")

		// Verify all chunks are valid and contain the necessary header.
		for _, chunk := range chunks {
			assert.NotEmpty(t, chunk.Content)
			assert.Equal(t, "code_group", chunk.Type)
			assert.Contains(t, chunk.Content, "package main")
			assert.Contains(t, chunk.Content, "import \"fmt\"")
		}
	})
}

func TestGoPlugin_MetadataAndHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	plugin := golang.NewGoPlugin(logger)

	t.Run("should extract correct metadata", func(t *testing.T) {
		metadata, err := plugin.ExtractMetadata(testGoFileContent, "test.go")
		require.NoError(t, err)

		assert.Equal(t, "go", metadata.Language)
		assert.Equal(t, "main", metadata.Properties["package"])
		assert.Contains(t, metadata.Imports, "fmt")
		assert.GreaterOrEqual(t, len(metadata.Definitions), 8) // Check that definitions are still found

		// Verify a specific definition is correctly identified
		var found bool
		for _, def := range metadata.Definitions {
			if def.Name == "SayHello" && def.Type == "method" {
				assert.Equal(t, "public", def.Visibility)
				found = true
				break
			}
		}
		assert.True(t, found, "Method 'SayHello' should be in definitions")
	})

	t.Run("should handle file types correctly", func(t *testing.T) {
		assert.True(t, plugin.CanHandle("file.go", nil))
		assert.False(t, plugin.CanHandle("file_test.go", nil))
		assert.False(t, plugin.CanHandle("file.txt", nil))
	})

	t.Run("should handle empty or invalid content", func(t *testing.T) {
		// Empty content should produce no chunks and no error
		chunks, err := plugin.Chunk("", "empty.go", nil)
		require.NoError(t, err)
		assert.Empty(t, chunks)

		// Invalid Go code should produce an error
		invalidContent := "package main\nfunc broken{"
		_, err = plugin.Chunk(invalidContent, "broken.go", nil)
		assert.Error(t, err)
	})
}
