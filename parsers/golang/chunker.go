package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/sevigo/goframe/schema"
)

const (
	// Target size for a chunk in characters. Adjust as needed.
	// 2000-4000 characters is a good range for balancing context and size.
	targetChunkSize = 3000
)

// Chunk implements the new grouping strategy for Go files. It iterates through
// top-level declarations (functions, types, vars) and groups them into larger,
// more context-rich chunks that do not exceed a target size.
func (p *GoPlugin) Chunk(content string, path string, opts *schema.CodeChunkingOptions) ([]schema.CodeChunk, error) {
	if strings.TrimSpace(content) == "" {
		return []schema.CodeChunk{}, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file for chunking: %w", err)
	}

	lines := strings.Split(content, "\n")
	var chunks []schema.CodeChunk

	var currentChunkContent strings.Builder
	var currentChunkStartLine = -1
	var lastDeclEndLine int

	// Pre-calculate the package and import block to prepend to every chunk.
	// This gives the LLM crucial context about the file's purpose and dependencies.
	packageAndImports := p.extractPackageAndImports(file, lines, fset)

	for _, decl := range file.Decls {
		startPos := fset.Position(decl.Pos())
		endPos := fset.Position(decl.End())

		// Extract the raw text of the entire declaration block, including its doc comment.
		declContent := p.extractDeclarationContent(lines, startPos.Line, endPos.Line)

		// If the current chunk has content and adding the next declaration would
		// exceed our target size, finalize the current chunk.
		if currentChunkContent.Len() > 0 && (currentChunkContent.Len()+len(declContent) > targetChunkSize) {
			chunkIdentifier := fmt.Sprintf("%s:%d-%d", path, currentChunkStartLine, lastDeclEndLine)
			chunk := schema.CodeChunk{
				Content:    packageAndImports + currentChunkContent.String(),
				LineStart:  currentChunkStartLine,
				LineEnd:    lastDeclEndLine,
				Type:       "code_group",
				Identifier: chunkIdentifier,
				Annotations: map[string]string{
					"type": "code_group",
				},
			}
			chunks = append(chunks, chunk)

			// Reset for the next chunk.
			currentChunkContent.Reset()
			currentChunkStartLine = -1
		}

		// If this is the start of a new chunk, record its starting line number.
		if currentChunkStartLine == -1 {
			currentChunkStartLine = startPos.Line
		}

		currentChunkContent.WriteString(declContent)
		currentChunkContent.WriteString("\n\n") // Add vertical space between declarations for clarity.
		lastDeclEndLine = endPos.Line
	}

	// After the loop, add the final remaining chunk if it has any content.
	if currentChunkContent.Len() > 0 {
		chunkIdentifier := fmt.Sprintf("%s:%d-%d", path, currentChunkStartLine, lastDeclEndLine)
		chunk := schema.CodeChunk{
			Content:    packageAndImports + currentChunkContent.String(),
			LineStart:  currentChunkStartLine,
			LineEnd:    lastDeclEndLine,
			Type:       "code_group",
			Identifier: chunkIdentifier,
			Annotations: map[string]string{
				"type": "code_group",
			},
		}
		chunks = append(chunks, chunk)
	}

	p.logger.Debug("Created grouped chunks for Go file", "count", len(chunks), "path", path)
	return chunks, nil
}

// extractPackageAndImports gets the package and import declarations as a formatted string.
func (p *GoPlugin) extractPackageAndImports(file *ast.File, lines []string, fset *token.FileSet) string {
	var header strings.Builder
	if file.Name != nil {
		header.WriteString(fmt.Sprintf("package %s\n\n", file.Name.Name))
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if ok && genDecl.Tok == token.IMPORT {
			start := fset.Position(genDecl.Pos()).Line
			end := fset.Position(genDecl.End()).Line
			// Ensure start and end are within bounds before slicing.
			if start > 0 && end <= len(lines) {
				header.WriteString(strings.Join(lines[start-1:end], "\n"))
				header.WriteString("\n\n")
			}
			break // Assume only one import block per file for simplicity.
		}
	}
	return header.String()
}

// extractDeclarationContent gets the full source text of a declaration using its line numbers.
func (p *GoPlugin) extractDeclarationContent(lines []string, startLine, endLine int) string {
	// The start and end lines are 1-based, so adjust for 0-based slice indexing.
	startIdx := startLine - 1
	endIdx := endLine

	// Add bounds checking for safety.
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	if startIdx >= endIdx {
		return ""
	}

	return strings.Join(lines[startIdx:endIdx], "\n")
}
