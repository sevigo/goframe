package typescript_test

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/parsers/typescript"
	"github.com/sevigo/goframe/schema"
)

// setupParser is a test helper to initialize the parser.
func setupParser(t *testing.T) schema.ParserPlugin {
	t.Helper()
	p := typescript.NewTypeScriptPlugin(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	return p
}

func TestTypeScriptParser_EmptyContent(t *testing.T) {
	p := setupParser(t)
	content := ""
	path := "empty.ts"

	chunks, err := p.Chunk(content, path, nil)
	require.NoError(t, err, "Chunking empty content should not produce an error")
	assert.Empty(t, chunks, "Chunks slice should be empty for empty content")
}

func TestTypeScriptParser_InvalidContent(t *testing.T) {
	p := setupParser(t)
	content := `import { some } from 'module'; class MyClass { method(`
	path := "invalid.ts"

	_, err := p.Chunk(content, path, nil)
	require.Error(t, err, "Chunking invalid content should return an error")
	assert.Contains(t, err.Error(), "')' expected.", "Error should describe the specific TS syntax issue")
}

func TestTypeScriptParser_BasicFeatures(t *testing.T) {
	p := setupParser(t)
	content := `
import { Injectable } from '@angular/core';

/**
 * This is a description for the Greeter class.
 */
export class Greeter {
    private message: string; // A private message

    // Constructor for Greeter
    constructor(initialMessage: string) {
        this.message = initialMessage;
    }

    /**
     * Greets the user.
     * @returns A greeting string.
     */
    public greet(): string {
        return "Hello!";
    }

	private internalMethod(): void {}
}

export const APP_VERSION = "1.0.0";

// A non-exported function
function calculateSum(a: number, b: number): number {
    return a + b;
}
`
	path := "basic.ts"

	t.Run("Chunking Basic Features", func(t *testing.T) {
		chunks, err := p.Chunk(content, path, nil)
		require.NoError(t, err)

		chunkMap := make(map[string]schema.CodeChunk)
		for _, c := range chunks {
			chunkMap[c.Identifier] = c
		}

		// Class
		greeterChunk, ok := chunkMap["Greeter"]
		require.True(t, ok)
		assert.Equal(t, 7, greeterChunk.LineStart)
		assert.Equal(t, 24, greeterChunk.LineEnd)

		// Constructor
		constructorChunk, ok := chunkMap["Greeter.constructor"]
		require.True(t, ok)
		assert.Equal(t, "public", constructorChunk.Annotations["visibility"])
	})

	t.Run("Metadata Extraction Basic Features", func(t *testing.T) {
		metadata, err := p.ExtractMetadata(content, path)
		require.NoError(t, err)
		assert.Equal(t, "2", metadata.Properties["total_methods"])
	})
}

func TestTypeScriptParser_GroupedDeclarations(t *testing.T) {
	p := setupParser(t)
	content := `
export const API_URL = "/api", TIMEOUT = 5000;
let isDebug = false, logLevel = "info";
`
	path := "grouped.ts"

	t.Run("Chunking Grouped Declarations", func(t *testing.T) {
		chunks, err := p.Chunk(content, path, nil)
		require.NoError(t, err)
		chunkMap := make(map[string]schema.CodeChunk)
		for _, c := range chunks {
			chunkMap[c.Identifier] = c
		}
		require.Len(t, chunks, 4)

		apiChunk, ok := chunkMap["API_URL"]
		require.True(t, ok)
		assert.Equal(t, "public", apiChunk.Annotations["visibility"])

		timeoutChunk, ok := chunkMap["TIMEOUT"]
		require.True(t, ok)
		assert.Equal(t, "public", timeoutChunk.Annotations["visibility"])
	})
}

func TestTypeScriptParser_AdvancedTypes(t *testing.T) {
	p := setupParser(t)
	content := `
type UserID = string;

export interface Repository<T> {
    findById(id: UserID): Promise<T | undefined>;
    save(entity: T): Promise<void>;
}

const enum LogLevel { DEBUG, INFO, ERROR }
`
	path := "advanced.ts"

	t.Run("Chunking Advanced Types", func(t *testing.T) {
		chunks, err := p.Chunk(content, path, nil)
		require.NoError(t, err)
		require.Len(t, chunks, 5)

		chunkMap := make(map[string]schema.CodeChunk)
		for _, c := range chunks {
			chunkMap[c.Identifier] = c
		}

		_, ok := chunkMap["Repository.findById"]
		assert.True(t, ok, "findById method chunk not found in interface")

		_, ok = chunkMap["Repository.save"]
		assert.True(t, ok, "save method chunk not found in interface")
	})
}

func TestTypeScriptParser_Signatures(t *testing.T) {
	p := setupParser(t)
	content := `
export function complexFunc(
    param1: string,
    param2: number = 10,
    ...rest: any[]
): Promise<string> {
    return Promise.resolve("test");
}

export class TestClass {
    constructor(private readonly name: string) {}

    async asyncMethod(id: number): Promise<void> {
        // implementation
    }
}
`
	path := "signatures.ts"

	metadata, err := p.ExtractMetadata(content, path)
	require.NoError(t, err)

	var complexFuncDef *schema.CodeEntityDefinition
	for _, def := range metadata.Definitions {
		if def.Name == "complexFunc" {
			complexFuncDef = &def
			break
		}
	}

	require.NotNil(t, complexFuncDef)
	assert.Contains(t, complexFuncDef.Signature, "param1: string")
	assert.Contains(t, complexFuncDef.Signature, "param2: number")
	assert.Contains(t, complexFuncDef.Signature, "Promise<string>")
}

func TestTypeScriptParser_ArrowFunctions(t *testing.T) {
	p := setupParser(t)
	content := `
export const arrowFunc = (x: number): number => x * 2;

const complexArrow = async (data: string[]): Promise<number> => {
    return data.length;
};
`
	path := "arrow.ts"

	chunks, err := p.Chunk(content, path, nil)
	require.NoError(t, err)

	chunkMap := make(map[string]schema.CodeChunk)
	for _, c := range chunks {
		chunkMap[c.Identifier] = c
	}

	arrowChunk, ok := chunkMap["arrowFunc"]
	require.True(t, ok)
	assert.Equal(t, "constant", arrowChunk.Type)
	assert.Equal(t, "public", arrowChunk.Annotations["visibility"])

	complexChunk, ok := chunkMap["complexArrow"]
	require.True(t, ok)
	assert.Equal(t, "constant", complexChunk.Type)
	assert.Equal(t, "private", complexChunk.Annotations["visibility"])
}

func TestTypeScriptParser_LargeFile(t *testing.T) {
	p := setupParser(t)

	// Generate a large file with many classes
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf(`
export class Class%d {
    private field%d: number;
    
    constructor(value: number) {
        this.field%d = value;
    }
    
    public method%d(): number {
        return this.field%d;
    }
}
`, i, i, i, i, i))
	}

	content := sb.String()
	path := "large.ts"

	chunks, err := p.Chunk(content, path, nil)
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 300) // 100 classes * 3 members each

	metadata, err := p.ExtractMetadata(content, path)
	require.NoError(t, err)
	assert.Equal(t, "100", metadata.Properties["total_classes"])
	assert.Equal(t, "100", metadata.Properties["total_methods"])
}
