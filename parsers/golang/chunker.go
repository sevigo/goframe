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
	packageAndImports := p.extractPackageAndImports(file, lines, fset)

	// Determine where to start scanning for declarations (after imports)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if ok && genDecl.Tok == token.IMPORT {
			lastDeclEndLine = fset.Position(decl.End()).Line
		}
	}
	// If no imports, start after package declaration
	if lastDeclEndLine == 0 && file.Name != nil {
		lastDeclEndLine = fset.Position(file.Name.End()).Line
	}

	for _, decl := range file.Decls {
		startPos := fset.Position(decl.Pos())
		endPos := fset.Position(decl.End())

		// Skip import declarations as they are handled in the header
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			continue
		}

		// Capture any gap content (comments, whitespace) between the last declaration and this one
		gapContent := ""
		if lastDeclEndLine > 0 && startPos.Line > lastDeclEndLine+1 {
			gapContent = p.extractDeclarationContent(lines, lastDeclEndLine+1, startPos.Line-1)
			if strings.TrimSpace(gapContent) != "" {
				gapContent += "\n\n"
			} else {
				gapContent = "" // Don't add just whitespace if not needed
			}
		}

		// Extract the raw text of the entire declaration block
		declContent := p.extractDeclarationContent(lines, startPos.Line, endPos.Line)

		// Combined content of gap + declaration
		fullNewContent := gapContent + declContent
		totalAddSize := len(fullNewContent)

		// 1. Check if the new declaration ITSELF is too large to fit in a standard chunk
		if totalAddSize > targetChunkSize {
			// Flush current chunk if any
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
				currentChunkContent.Reset()
				currentChunkStartLine = -1
			}

			// Sub-chunk the large declaration using internal recursive splitter
			subSplits := p.recursiveSplit(fullNewContent, targetChunkSize, 200)

			for i, sub := range subSplits {
				chunkIdentifier := fmt.Sprintf("%s:%d-%d:part%d", path, startPos.Line, endPos.Line, i)
				chunks = append(chunks, schema.CodeChunk{
					Content:    packageAndImports + sub,
					LineStart:  startPos.Line, // Approximate line tracking for sub-chunks
					LineEnd:    endPos.Line,
					Type:       "code_group_split",
					Identifier: chunkIdentifier,
				})
			}

			lastDeclEndLine = endPos.Line
			continue
		}

		// 2. Normal flow: Check if adding to current chunk exceeds limit
		if currentChunkContent.Len() > 0 && (currentChunkContent.Len()+totalAddSize > targetChunkSize) {
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
			// If we have gap content, strictly speaking the chunk starts at the gap.
			currentChunkStartLine = startPos.Line
			if gapContent != "" && lastDeclEndLine > 0 {
				currentChunkStartLine = lastDeclEndLine + 1
			}
		}

		currentChunkContent.WriteString(fullNewContent)
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

// recursiveSplit splits text using a list of separators
func (p *GoPlugin) recursiveSplit(text string, chunkSize, chunkOverlap int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}
	separators := []string{"\n\n", "\n", " ", ""}
	return p.splitTextRecursive(text, separators, chunkSize, chunkOverlap)
}

func (p *GoPlugin) splitTextRecursive(text string, separators []string, chunkSize, chunkOverlap int) []string {
	var finalChunks []string
	if len(text) <= chunkSize {
		return []string{text}
	}
	if len(separators) == 0 {
		return []string{text}
	}
	separator := separators[0]
	remainingSeparators := separators[1:]
	splits := strings.Split(text, separator)
	var goodSplits []string
	currentSplit := ""
	for _, split := range splits {
		if len(split) == 0 {
			continue
		}
		if len(currentSplit) > 0 && len(currentSplit)+len(separator)+len(split) <= chunkSize {
			currentSplit += separator + split
		} else {
			if len(currentSplit) > 0 {
				goodSplits = append(goodSplits, currentSplit)
			}
			currentSplit = split
		}
	}
	if currentSplit != "" {
		goodSplits = append(goodSplits, currentSplit)
	}

	for _, split := range goodSplits {
		if len(split) <= chunkSize {
			finalChunks = append(finalChunks, split)
		} else {
			recursiveChunks := p.splitTextRecursive(split, remainingSeparators, chunkSize, chunkOverlap)
			finalChunks = append(finalChunks, recursiveChunks...)
		}
	}

	// Merge with overlap if needed
	if chunkOverlap > 0 && len(finalChunks) > 1 {
		return p.mergeWithOverlap(finalChunks, chunkSize, chunkOverlap)
	}

	return finalChunks
}

func (p *GoPlugin) mergeWithOverlap(chunks []string, chunkSize, chunkOverlap int) []string {
	var mergedChunks []string
	currentChunk := ""
	separator := "\n"
	for i, chunk := range chunks {
		if currentChunk == "" {
			currentChunk = chunk
			continue
		}
		var overlap string
		if len(currentChunk) > chunkOverlap {
			overlap = currentChunk[len(currentChunk)-chunkOverlap:]
		} else {
			overlap = currentChunk
		}
		if len(currentChunk)+len(separator)+len(chunk) <= chunkSize {
			currentChunk += separator + chunk
		} else {
			mergedChunks = append(mergedChunks, currentChunk)
			currentChunk = overlap + separator + chunk
		}
		if i == len(chunks)-1 {
			mergedChunks = append(mergedChunks, currentChunk)
		}
	}
	return mergedChunks
}
