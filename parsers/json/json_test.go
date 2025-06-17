package json_test //nolint:cyclop //ok

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jsonparser "github.com/sevigo/goframe/parsers/json"
	logger "github.com/sevigo/goframe/parsers/testing"
	model "github.com/sevigo/goframe/schema"
)

func TestJsonPlugin(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := jsonparser.NewJSONPlugin(log)

	// Test basic info
	t.Run("BasicInfo", func(t *testing.T) {
		assert.Equal(t, "json", plugin.Name())
		assert.Contains(t, plugin.Extensions(), ".json")
	})

	// Test simple object chunking
	t.Run("SimpleObject", func(t *testing.T) {
		jsonContent := `{
  "name": "John Doe",
  "age": 30,
  "email": "john@example.com"
}`

		chunks, err := plugin.Chunk(jsonContent, "simple.json", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Small object should create single chunk
		assert.Equal(t, 1, len(chunks))

		chunk := chunks[0]
		assert.Equal(t, "json_document", chunk.Type)
		assert.Equal(t, "Simple", chunk.Identifier)
		assert.Contains(t, chunk.Content, "John Doe")
		assert.Equal(t, "object", chunk.Annotations["json_type"])
	})

	// Test complex object chunking
	t.Run("ComplexObject", func(t *testing.T) {
		jsonContent := `{
  "users": [
    {
      "id": 1,
      "name": "Alice",
      "profile": {
        "email": "alice@example.com",
        "preferences": {
          "theme": "dark",
          "notifications": true
        }
      }
    },
    {
      "id": 2,
      "name": "Bob",
      "profile": {
        "email": "bob@example.com",
        "preferences": {
          "theme": "light",
          "notifications": false
        }
      }
    }
  ],
  "settings": {
    "version": "1.0",
    "features": ["auth", "api", "dashboard"],
    "database": {
      "host": "localhost",
      "port": 5432,
      "name": "myapp"
    }
  },
  "metadata": {
    "created": "2024-01-01",
    "updated": "2024-12-01"
  }
}`

		chunks, err := plugin.Chunk(jsonContent, "config.json", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Should create chunks for significant properties
		assert.GreaterOrEqual(t, len(chunks), 1)

		// Log chunks for debugging
		for _, chunk := range chunks {
			t.Logf("Chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)
		}

		// Should have meaningful identifiers
		foundUsers := false
		foundSettings := false

		for _, chunk := range chunks {
			switch chunk.Identifier {
			case "users":
				foundUsers = true
				assert.Equal(t, "json_property", chunk.Type)
				assert.Contains(t, chunk.Content, "Alice")
				assert.Contains(t, chunk.Content, "Bob")
			case "settings":
				foundSettings = true
				assert.Equal(t, "json_property", chunk.Type)
				assert.Contains(t, chunk.Content, "database")
			}
		}

		// Note: Depending on chunking strategy, might create single chunk or multiple
		if len(chunks) > 1 {
			assert.True(t, foundUsers || foundSettings, "Should find at least one major property")
		}
	})

	// Test array chunking
	t.Run("LargeArray", func(t *testing.T) {
		// Create large array
		items := make([]map[string]interface{}, 25)
		for i := range 25 {
			items[i] = map[string]interface{}{
				"id":   i + 1,
				"name": fmt.Sprintf("Item %d", i+1),
				"type": "test",
			}
		}

		jsonBytes, err := json.MarshalIndent(items, "", "  ")
		require.NoError(t, err)

		chunks, err := plugin.Chunk(string(jsonBytes), "items.json", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Large array should be chunked
		assert.GreaterOrEqual(t, len(chunks), 1)

		// Log chunks for debugging
		for _, chunk := range chunks {
			t.Logf("Array chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)
		}
	})

	// Test metadata extraction
	t.Run("MetadataExtraction", func(t *testing.T) {
		jsonContent := `{
  "api": {
    "version": "v1",
    "endpoints": [
      {"path": "/users", "method": "GET"},
      {"path": "/users", "method": "POST"},
      {"path": "/users/:id", "method": "PUT"}
    ]
  },
  "database": {
    "type": "postgresql",
    "host": "localhost",
    "port": 5432
  },
  "features": ["auth", "logging", "metrics"],
  "enabled": true,
  "timeout": 30000
}`

		metadata, err := plugin.ExtractMetadata(jsonContent, "api-config.json")
		require.NoError(t, err)

		// Basic metadata
		assert.Equal(t, "json", metadata.Language)
		assert.Equal(t, "true", metadata.Properties["valid"])
		assert.Equal(t, "object", metadata.Properties["root_type"])

		// Statistics
		assert.NotEmpty(t, metadata.Properties["objects"])
		assert.NotEmpty(t, metadata.Properties["arrays"])
		assert.NotEmpty(t, metadata.Properties["strings"])
		assert.NotEmpty(t, metadata.Properties["numbers"])
		assert.NotEmpty(t, metadata.Properties["booleans"])

		// Should have symbols for top-level properties
		symbolNames := make(map[string]bool)
		for _, symbol := range metadata.Symbols {
			symbolNames[symbol.Name] = true
		}

		assert.True(t, symbolNames["api"], "Should have 'api' symbol")
		assert.True(t, symbolNames["database"], "Should have 'database' symbol")
		assert.True(t, symbolNames["features"], "Should have 'features' symbol")

		// Should have definitions for complex objects
		definitionNames := make(map[string]bool)
		for _, def := range metadata.Definitions {
			definitionNames[def.Name] = true
		}

		assert.True(t, definitionNames["api"] || definitionNames["database"],
			"Should have definitions for complex objects")

		t.Logf("Found %d symbols and %d definitions",
			len(metadata.Symbols), len(metadata.Definitions))
	})

	// Test invalid JSON
	t.Run("InvalidJson", func(t *testing.T) {
		invalidContent := `{
  "name": "test"
  "missing": "comma"
}`

		chunks, err := plugin.Chunk(invalidContent, "invalid.json", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		assert.Equal(t, 1, len(chunks))
		chunk := chunks[0]
		assert.Equal(t, "json_invalid", chunk.Type)
		assert.Contains(t, chunk.Annotations, "error")
	})

	// Test primitive JSON
	t.Run("PrimitiveJson", func(t *testing.T) {
		primitiveContent := `"Hello World"`

		chunks, err := plugin.Chunk(primitiveContent, "message.json", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		assert.Equal(t, 1, len(chunks))
		chunk := chunks[0]
		assert.Equal(t, "json_document", chunk.Type)
		assert.Equal(t, "Message", chunk.Identifier)
		assert.Equal(t, "primitive", chunk.Annotations["json_type"])
	})

	// Test empty JSON
	t.Run("EmptyJson", func(t *testing.T) {
		emptyContent := `{}`

		chunks, err := plugin.Chunk(emptyContent, "empty.json", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		assert.Equal(t, 1, len(chunks))
		chunk := chunks[0]
		assert.Equal(t, "json_document", chunk.Type)
		assert.Equal(t, "Empty", chunk.Identifier)
	})
}
