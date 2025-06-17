package markdown_test //nolint:cyclop //ok

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/parsers/markdown"
	logger "github.com/sevigo/goframe/parsers/testing"
	model "github.com/sevigo/goframe/schema"
)

func TestMarkdownPlugin(t *testing.T) {
	log, _ := logger.NewTestLogger(t)
	plugin := markdown.NewMarkdownPlugin(log)

	// Test Name and Extensions
	t.Run("BasicInfo", func(t *testing.T) {
		assert.Equal(t, "markdown", plugin.Name())
		assert.Contains(t, plugin.Extensions(), ".md")
		assert.Contains(t, plugin.Extensions(), ".markdown")
	})

	// Test hierarchical chunking with frontmatter
	t.Run("HierarchicalChunking", func(t *testing.T) {
		mdContent := `---
title: Test Document
author: Test User
date: 2025-05-20
---

# Introduction

This is a test markdown document with multiple sections.

## Section 1

Some content for section 1.

### Subsection 1.1

More detailed content with a code sample:

` + "```go" + `
func Hello() {
	fmt.Println("Hello, world!")
}
` + "```" + `

## Section 2

Another section with different content.

### Subsection 2.1

Content under section 2.

#### Deep Subsection

Very nested content.
`

		chunks, err := plugin.Chunk(mdContent, "test.md", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Should have hierarchical chunks
		assert.GreaterOrEqual(t, len(chunks), 3)

		// Verify that we have the main sections
		foundIntro := false
		foundSection1 := false
		foundSection2 := false
		foundSubsections := false

		for _, chunk := range chunks {
			t.Logf("Chunk: Type=%s, Identifier=%s, Lines=%d-%d",
				chunk.Type, chunk.Identifier, chunk.LineStart, chunk.LineEnd)

			switch chunk.Identifier {
			case "Introduction":
				foundIntro = true
				assert.Contains(t, chunk.Content, "# Introduction")
				assert.Contains(t, chunk.Content, "title: Test Document")
			case "Section 1":
				foundSection1 = true
				assert.Contains(t, chunk.Content, "## Section 1")
			case "Section 2":
				foundSection2 = true
				assert.Contains(t, chunk.Content, "## Section 2")
			case "Subsection 1.1", "Subsection 2.1", "Deep Subsection":
				foundSubsections = true
			}
		}

		assert.True(t, foundIntro, "Should have found Introduction section")
		assert.True(t, foundSection1, "Should have found Section 1")
		assert.True(t, foundSection2, "Should have found Section 2")
		assert.True(t, foundSubsections, "Should have found subsections")
	})

	// Test code block separation
	t.Run("CodeBlockSeparation", func(t *testing.T) {
		mdContent := `# API Documentation

This document describes our API.

## Authentication

Use the following code for authentication:

` + "```javascript" + `
// Large authentication example
const auth = {
  apiKey: process.env.API_KEY,
  secret: process.env.API_SECRET,
  
  async authenticate() {
    const response = await fetch('/auth', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        key: this.apiKey,
        secret: this.secret
      })
    });
    
    if (!response.ok) {
      throw new Error('Authentication failed');
    }
    
    const data = await response.json();
    this.token = data.token;
    return this.token;
  },
  
  getHeaders() {
    return {
      'Authorization': 'Bearer ' + this.token,
      'Content-Type': 'application/json'
    };
  }
};

module.exports = auth;
` + "```" + `

## Usage

Small code example:

` + "```javascript" + `
console.log('Hello');
` + "```" + `
`

		chunks, err := plugin.Chunk(mdContent, "api.md", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Should have separate chunks for large code blocks
		hasMainDoc := false
		hasAuthSection := false
		hasLargeCodeBlock := false
		hasUsageSection := false

		for _, chunk := range chunks {
			t.Logf("Code separation chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)

			switch {
			case chunk.Identifier == "API Documentation":
				hasMainDoc = true
			case chunk.Identifier == "Authentication":
				hasAuthSection = true
				// Large code block should be separated, so auth section shouldn't contain it
				assert.NotContains(t, chunk.Content, "module.exports = auth;")
			case chunk.Type == "code_block" && chunk.Identifier == "Authentication (javascript code)":
				hasLargeCodeBlock = true
				assert.Contains(t, chunk.Content, "module.exports = auth;")
				assert.Equal(t, "javascript", chunk.Annotations["language"])
			case chunk.Identifier == "Usage":
				hasUsageSection = true
				// Small code block should remain in the section
				assert.Contains(t, chunk.Content, "console.log")
			}
		}

		assert.True(t, hasMainDoc, "Should have main document")
		assert.True(t, hasAuthSection, "Should have Authentication section")
		assert.True(t, hasLargeCodeBlock, "Should have separate large code block")
		assert.True(t, hasUsageSection, "Should have Usage section")
	})

	// Test table and list handling
	t.Run("TableAndListHandling", func(t *testing.T) {
		mdContent := `# Data Reference

## User Fields

| Field | Type | Description |
|-------|------|-------------|
| id | number | Unique identifier |
| name | string | User's full name |
| email | string | Contact email |
| created_at | date | Account creation date |

## Status Codes

- 200: Success
- 400: Bad Request  
- 401: Unauthorized
- 404: Not Found
- 500: Internal Server Error

## Nested Lists

1. First level item
   - Nested bullet point
   - Another nested item
     - Deeply nested item
2. Second level item
   - More nesting
`

		chunks, err := plugin.Chunk(mdContent, "reference.md", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		hasDataRef := false
		hasUserFields := false
		hasStatusCodes := false
		hasNestedLists := false

		for _, chunk := range chunks {
			t.Logf("Table/List chunk: Type=%s, Identifier=%s", chunk.Type, chunk.Identifier)

			switch chunk.Identifier {
			case "Data Reference":
				hasDataRef = true
			case "User Fields":
				hasUserFields = true
				// Should contain the complete table
				assert.Contains(t, chunk.Content, "| Field | Type | Description |")
				assert.Contains(t, chunk.Content, "| created_at | date |")
			case "Status Codes":
				hasStatusCodes = true
				// Should contain the complete list
				assert.Contains(t, chunk.Content, "- 200: Success")
				assert.Contains(t, chunk.Content, "- 500: Internal Server Error")
			case "Nested Lists":
				hasNestedLists = true
				// Should preserve nested structure
				assert.Contains(t, chunk.Content, "1. First level item")
				assert.Contains(t, chunk.Content, "   - Nested bullet point")
				assert.Contains(t, chunk.Content, "     - Deeply nested item")
			}
		}

		assert.True(t, hasDataRef, "Should have Data Reference")
		assert.True(t, hasUserFields, "Should have User Fields with table")
		assert.True(t, hasStatusCodes, "Should have Status Codes with list")
		assert.True(t, hasNestedLists, "Should have Nested Lists")
	})

	// Test enhanced metadata extraction
	t.Run("EnhancedMetadataExtraction", func(t *testing.T) {
		mdContent := `---
title: Comprehensive Guide
author: Technical Writer
version: 2.1.0
tags: [documentation, api, guide]
---

# Overview

This is a comprehensive technical guide.

## Getting Started

### Prerequisites

You need the following:

- Node.js 18+
- Python 3.9+

### Installation

` + "```bash" + `
npm install my-package
` + "```" + `

## API Reference

### Endpoints

#### GET /users

` + "```python" + `
import requests
response = requests.get('/api/users')
` + "```" + `

#### POST /users

` + "```go" + `
func createUser(w http.ResponseWriter, r *http.Request) {
    // Implementation here
}
` + "```" + `

## Examples

| Scenario | Code | Result |
|----------|------|--------|
| Basic | simple() | works |
| Advanced | complex() | powerful |
`

		metadata, err := plugin.ExtractMetadata(mdContent, "comprehensive-guide.md")
		require.NoError(t, err)

		// Test basic metadata
		assert.Equal(t, "markdown", metadata.Language)
		assert.Equal(t, "Comprehensive Guide", metadata.Properties["title"])
		assert.Equal(t, "Technical Writer", metadata.Properties["author"])
		assert.Equal(t, "2.1.0", metadata.Properties["version"])

		// Test code languages detection
		assert.Contains(t, metadata.Properties["code_languages"], "bash")
		assert.Contains(t, metadata.Properties["code_languages"], "python")
		assert.Contains(t, metadata.Properties["code_languages"], "go")

		// Test document statistics
		assert.NotEmpty(t, metadata.Properties["heading_count"])
		assert.NotEmpty(t, metadata.Properties["code_block_count"])
		assert.NotEmpty(t, metadata.Properties["table_count"])

		// Test headings in symbols
		headingLevels := make(map[string]bool)
		for _, symbol := range metadata.Symbols {
			headingLevels[symbol.Type] = true
		}

		assert.True(t, headingLevels["h1"], "Should have h1 symbols")
		assert.True(t, headingLevels["h2"], "Should have h2 symbols")
		assert.True(t, headingLevels["h3"], "Should have h3 symbols")
		assert.True(t, headingLevels["h4"], "Should have h4 symbols")

		// Test definitions
		assert.GreaterOrEqual(t, len(metadata.Definitions), 7) // Multiple headings

		hasOverview := false
		hasGettingStarted := false
		for _, def := range metadata.Definitions {
			if def.Name == "Overview" {
				hasOverview = true
				assert.Equal(t, "heading", def.Type)
				assert.Equal(t, "h1", def.Signature)
			}
			if def.Name == "Getting Started" {
				hasGettingStarted = true
				assert.Equal(t, "heading", def.Type)
				assert.Equal(t, "h2", def.Signature)
			}
		}

		assert.True(t, hasOverview, "Should have Overview definition")
		assert.True(t, hasGettingStarted, "Should have Getting Started definition")
	})

	// Test title fallback behavior
	t.Run("TitleFallback", func(t *testing.T) {
		// Test with H1 heading
		h1Content := `# Main Title

Content here.`

		metadata, err := plugin.ExtractMetadata(h1Content, "something.md")
		require.NoError(t, err)
		assert.Equal(t, "Main Title", metadata.Properties["title"])

		// Test with filename fallback
		noTitleContent := `Some content without headings or frontmatter.`

		metadata, err = plugin.ExtractMetadata(noTitleContent, "readme-file.md")
		require.NoError(t, err)
		assert.Equal(t, "Readme File", metadata.Properties["title"])

		// Test with frontmatter priority
		frontmatterContent := `---
title: Frontmatter Title
---

# Heading Title

Content.`

		metadata, err = plugin.ExtractMetadata(frontmatterContent, "test.md")
		require.NoError(t, err)
		assert.Equal(t, "Frontmatter Title", metadata.Properties["title"])
	})

	// Test document without headings
	t.Run("DocumentWithoutHeadings", func(t *testing.T) {
		plainContent := `This is a plain markdown document without any headings.

It has multiple paragraphs with different content.

` + "```javascript" + `
console.log('Some code');
` + "```" + `

And some more text after the code block.`

		chunks, err := plugin.Chunk(plainContent, "plain.md", &model.CodeChunkingOptions{})
		require.NoError(t, err)

		// Should create a single document chunk
		assert.Len(t, chunks, 1)

		chunk := chunks[0]
		assert.Equal(t, "document", chunk.Type)
		assert.Equal(t, "Plain", chunk.Identifier) // Derived from filename
		assert.Contains(t, chunk.Content, "This is a plain markdown document")
		assert.Contains(t, chunk.Content, "console.log")
	})
}
