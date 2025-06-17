package yaml_test //nolint:cyclop //ok

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logger "github.com/sevigo/goframe/parsers/testing"
	"github.com/sevigo/goframe/parsers/yaml"
	model "github.com/sevigo/goframe/schema"
)

func TestYamlPlugin(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := yaml.NewYamlPlugin(log)

	// Test Name and Extensions
	t.Run("BasicInfo", func(t *testing.T) {
		assert.Equal(t, "yaml", plugin.Name())
		assert.Contains(t, plugin.Extensions(), ".yaml")
		assert.Contains(t, plugin.Extensions(), ".yml")
	})

	t.Run("CanHandle", func(t *testing.T) {
		assert.True(t, plugin.CanHandle("config.yaml", nil))
		assert.True(t, plugin.CanHandle("docker-compose.yml", nil))
		assert.False(t, plugin.CanHandle("config.json", nil))
		assert.False(t, plugin.CanHandle("readme.md", nil))
	})

	// Test basic YAML structure chunking
	t.Run("BasicStructureChunking", func(t *testing.T) {
		yamlContent := `---
name: test-service
version: "1.0.0"

database:
  host: localhost
  port: 5432
  name: myapp
  credentials:
    username: admin
    password: secret123

server:
  port: 8080
  host: 0.0.0.0
  ssl:
    enabled: true
    cert_path: /etc/ssl/cert.pem
    key_path: /etc/ssl/key.pem

logging:
  level: info
  format: json
  outputs:
    - console
    - file:/var/log/app.log
`

		chunks, err := plugin.Chunk(yamlContent, "config.yaml", &model.CodeChunkingOptions{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(chunks), 1)

		// Should have chunks for major sections or a single document chunk
		foundDatabase := false
		foundServer := false
		foundLogging := false
		foundConfig := false

		for _, chunk := range chunks {
			t.Logf("Chunk: Type=%s, Identifier=%s, Lines=%d-%d",
				chunk.Type, chunk.Identifier, chunk.LineStart, chunk.LineEnd)

			// Check if we have individual section chunks
			switch chunk.Identifier {
			case "database":
				foundDatabase = true
				assert.Contains(t, chunk.Content, "host: localhost")
				assert.Contains(t, chunk.Content, "credentials:")
			case "server":
				foundServer = true
				assert.Contains(t, chunk.Content, "port: 8080")
				assert.Contains(t, chunk.Content, "ssl:")
			case "logging":
				foundLogging = true
				assert.Contains(t, chunk.Content, "level: info")
				assert.Contains(t, chunk.Content, "outputs:")
			case "Config":
				// Single document chunk containing everything
				foundConfig = true
				assert.Contains(t, chunk.Content, "database:")
				assert.Contains(t, chunk.Content, "server:")
				assert.Contains(t, chunk.Content, "logging:")
			}
		}

		// Either we have section chunks OR a single config chunk
		sectionsFound := foundDatabase && foundServer && foundLogging
		singleDocFound := foundConfig

		assert.True(t, sectionsFound || singleDocFound,
			"Should have either individual section chunks or single document chunk")
	})

	// Test sequence chunking
	t.Run("SequenceChunking", func(t *testing.T) {
		yamlContent := `---
# Large list of items
items:
  - name: item1
    value: 100
    description: "First item"
  - name: item2
    value: 200
    description: "Second item"
  - name: item3
    value: 300
    description: "Third item"
  - name: item4
    value: 400
    description: "Fourth item"
  - name: item5
    value: 500
    description: "Fifth item"
  - name: item6
    value: 600
    description: "Sixth item"

# Simple list
tags:
  - production
  - database
  - critical
`

		chunks, err := plugin.Chunk(yamlContent, "items.yaml", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		foundItems := false
		foundTags := false

		for _, chunk := range chunks {
			t.Logf("Sequence chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)

			if chunk.Identifier == "items" || strings.Contains(chunk.Content, "items:") {
				foundItems = true
				assert.Contains(t, chunk.Content, "- name: item1")
				assert.Contains(t, chunk.Content, "description:")
			}
			if chunk.Identifier == "tags" || strings.Contains(chunk.Content, "tags:") {
				foundTags = true
				assert.Contains(t, chunk.Content, "- production")
			}
		}

		assert.True(t, foundItems, "Should have items section")
		assert.True(t, foundTags, "Should have tags section")
	})

	// Test Kubernetes manifest chunking
	t.Run("KubernetesManifestChunking", func(t *testing.T) {
		yamlContent := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
  namespace: production
  labels:
    app: web-app
    version: v1.2.0
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web-app
  template:
    metadata:
      labels:
        app: web-app
    spec:
      containers:
      - name: web-container
        image: nginx:1.21
        ports:
        - containerPort: 80
        env:
        - name: ENV
          value: production
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: web-service
spec:
  selector:
    app: web-app
  ports:
  - port: 80
    targetPort: 80
  type: LoadBalancer
`

		chunks, err := plugin.Chunk(yamlContent, "k8s-manifest.yaml", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		foundMetadata := false
		foundSpec := false

		for _, chunk := range chunks {
			t.Logf("K8s chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)
			t.Logf("Content: %s", chunk.Content[:min(200, len(chunk.Content))])

			if chunk.Identifier == "metadata" {
				foundMetadata = true
				// The metadata chunk should contain the metadata section
				assert.Contains(t, chunk.Content, "metadata:")
				// It might contain "name: web-app" but could be in nested template metadata
			}
			if chunk.Identifier == "spec" {
				foundSpec = true
				assert.Contains(t, chunk.Content, "replicas: 3")
				// Should contain container specs
				assert.Contains(t, chunk.Content, "containers:")
			}
		}

		assert.True(t, foundMetadata || foundSpec, "Should have meaningful chunks for K8s manifest")
	})

	// Test single document chunk for simple YAML
	t.Run("SimpleYamlSingleChunk", func(t *testing.T) {
		yamlContent := `---
title: Simple Config
debug: true
max_connections: 100
`

		chunks, err := plugin.Chunk(yamlContent, "simple.yml", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Should create single document chunk for simple YAML
		assert.Equal(t, 1, len(chunks))
		chunk := chunks[0]
		assert.Equal(t, "yaml_document", chunk.Type)
		assert.Equal(t, "Simple", chunk.Identifier)
		assert.Contains(t, chunk.Content, "title: Simple Config")
	})

	// Test invalid YAML handling
	t.Run("InvalidYamlHandling", func(t *testing.T) {
		invalidYaml := `---
name: test
  invalid: indentation
    more: problems
- list item without proper structure
`

		chunks, err := plugin.Chunk(invalidYaml, "invalid.yaml", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		assert.Equal(t, 1, len(chunks))
		chunk := chunks[0]
		assert.Equal(t, "yaml_invalid", chunk.Type)
		assert.Contains(t, chunk.Annotations["error"], "yaml") // Should contain YAML error
	})

	// Test comprehensive metadata extraction
	t.Run("MetadataExtraction", func(t *testing.T) {
		yamlContent := `---
application:
  name: MyApp
  version: "2.1.0"
  
database:
  host: db.example.com
  port: 5432
  ssl: true
  
servers:
  - name: web1
    ip: 192.168.1.10
  - name: web2
    ip: 192.168.1.11
    
features:
  auth: true
  logging: enabled
  cache: redis
  
environment: production
deployment:
  strategy: rolling
`

		metadata, err := plugin.ExtractMetadata(yamlContent, "app-config.yaml")
		require.NoError(t, err)

		// Test basic metadata
		assert.Equal(t, "yaml", metadata.Language)
		assert.Equal(t, "true", metadata.Properties["valid"])

		// Test configuration pattern detection
		assert.Equal(t, "true", metadata.Properties["has_database_config"])
		assert.Equal(t, "true", metadata.Properties["has_auth_config"])
		assert.Equal(t, "true", metadata.Properties["has_cache_config"])
		assert.Equal(t, "true", metadata.Properties["has_environment_config"])
		assert.Equal(t, "true", metadata.Properties["has_deployment_config"])

		// Test structure analysis - root should be mapping since we have top-level keys
		assert.Equal(t, "mapping", metadata.Properties["root_type"])
		assert.NotEmpty(t, metadata.Properties["mappings"])
		assert.NotEmpty(t, metadata.Properties["sequences"])
		assert.NotEmpty(t, metadata.Properties["scalars"])
		assert.NotEmpty(t, metadata.Properties["keys"])

		// Test symbols and definitions - should have at least some symbols
		t.Logf("Found %d symbols and %d definitions", len(metadata.Symbols), len(metadata.Definitions))
		for _, symbol := range metadata.Symbols {
			t.Logf("Symbol: Name=%s, Type=%s", symbol.Name, symbol.Type)
		}

		assert.GreaterOrEqual(t, len(metadata.Symbols), 3) // Should have multiple keys as symbols

		foundApplication := false
		foundDatabase := false
		foundServers := false

		for _, symbol := range metadata.Symbols {
			switch symbol.Name {
			case "application":
				foundApplication = true
				assert.Equal(t, "mapping", symbol.Type)
			case "database":
				foundDatabase = true
				assert.Equal(t, "mapping", symbol.Type)
			case "servers":
				foundServers = true
				assert.Equal(t, "sequence", symbol.Type)
			}
		}

		assert.True(t, foundApplication, "Should have application symbol")
		assert.True(t, foundDatabase, "Should have database symbol")
		assert.True(t, foundServers, "Should have servers symbol")

		// Test definitions for complex structures - should have at least some definitions
		assert.GreaterOrEqual(t, len(metadata.Definitions), 1)

		foundAppDef := false
		foundDBDef := false
		for _, def := range metadata.Definitions {
			t.Logf("Definition: Name=%s, Type=%s, Signature=%s", def.Name, def.Type, def.Signature)
			if def.Name == "application" {
				foundAppDef = true
				assert.Equal(t, "mapping", def.Type)
				assert.Contains(t, def.Signature, "mapping")
			}
			if def.Name == "database" {
				foundDBDef = true
				assert.Equal(t, "mapping", def.Type)
			}
		}

		// At least one of these should be found
		assert.True(t, foundAppDef || foundDBDef, "Should have at least one complex structure definition")
	})

	// Test Docker Compose detection
	t.Run("DockerComposeDetection", func(t *testing.T) {
		dockerComposeContent := `---
version: '3.8'

services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
    environment:
      - ENV=production
    
  database:
    image: postgres:13
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: user
      POSTGRES_PASSWORD: pass
    volumes:
      - db_data:/var/lib/postgresql/data

volumes:
  db_data:
`

		metadata, err := plugin.ExtractMetadata(dockerComposeContent, "docker-compose.yml")
		require.NoError(t, err)

		assert.Equal(t, "true", metadata.Properties["has_docker_config"])
		assert.Equal(t, "true", metadata.Properties["has_environment_config"])
	})

	// Test CI/CD pipeline detection
	t.Run("CIPipelineDetection", func(t *testing.T) {
		ciContent := `---
name: CI Pipeline

on:
  push:
    branches: [ main ]

jobs:
  build:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v2
    
    - name: Setup Node
      uses: actions/setup-node@v2
      with:
        node-version: '16'
    
    - name: Install dependencies
      run: npm ci
    
    - name: Run tests
      run: npm test
    
    - name: Deploy
      if: github.ref == 'refs/heads/main'
      run: npm run deploy
`

		metadata, err := plugin.ExtractMetadata(ciContent, ".github/workflows/ci.yml")
		require.NoError(t, err)

		assert.Equal(t, "true", metadata.Properties["has_ci_config"])
	})

	// Test empty and minimal YAML
	t.Run("EmptyAndMinimalYaml", func(t *testing.T) {
		// Test completely empty YAML
		emptyContent := ""
		chunks, err := plugin.Chunk(emptyContent, "empty.yaml", &model.CodeChunkingOptions{})
		require.NoError(t, err)
		assert.Len(t, chunks, 1)

		// Test minimal YAML
		minimalContent := `---
value: 42
`
		chunks, err = plugin.Chunk(minimalContent, "minimal.yml", &model.CodeChunkingOptions{})
		require.NoError(t, err)
		assert.Len(t, chunks, 1)
		assert.Equal(t, "yaml_document", chunks[0].Type)
	})

	// Test large sequence chunking
	t.Run("LargeSequenceChunking", func(t *testing.T) {
		// Create a YAML with a large sequence that should be chunked
		var yamlBuilder strings.Builder
		yamlBuilder.WriteString("---\nlarge_list:\n")

		for i := range 25 {
			yamlBuilder.WriteString(fmt.Sprintf("  - id: %d\n", i))
			yamlBuilder.WriteString(fmt.Sprintf("    name: item%d\n", i))
			yamlBuilder.WriteString(fmt.Sprintf("    value: %d\n", i*10))
		}

		chunks, err := plugin.Chunk(yamlBuilder.String(), "large-list.yaml", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Should create multiple chunks for large sequence
		for _, chunk := range chunks {
			if chunk.Type == "yaml_sequence_slice" {
				assert.Contains(t, chunk.Annotations, "items_count")
				assert.Contains(t, chunk.Annotations, "start_index")
			}
		}

		// Either single large_list chunk or sequence slices
		assert.GreaterOrEqual(t, len(chunks), 1, "Should have at least one chunk")
	})
}
