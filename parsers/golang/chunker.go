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

	// Track definition info for the current chunk
	var chunkIsDefinition bool
	var chunkSymbolType string
	var chunkIdentifier string

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

		// Skip import declarations
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			continue
		}

		isDef, symType, id := p.findDefinitionInfo(decl)

		// Capture any gap content
		gapContent := p.extractGapContent(lines, lastDeclEndLine, startPos.Line)

		declContent := p.extractDeclarationContent(lines, startPos.Line, endPos.Line)
		fullNewContent := gapContent + declContent
		totalAddSize := len(fullNewContent)

		// If this is a definition and we have one already, flush it first
		if isDef && chunkIdentifier != "" && currentChunkContent.Len() > 0 {
			chunks = p.flushChunk(chunks, path, packageAndImports, &currentChunkContent, &currentChunkStartLine, &chunkIsDefinition, &chunkSymbolType, &chunkIdentifier, lastDeclEndLine)
		}

		if isDef {
			chunkIsDefinition = true
			chunkSymbolType = symType
			chunkIdentifier = id
		}

		// 1. Check if the new declaration ITSELF is too large
		if totalAddSize > targetChunkSize {
			chunks = p.handleLargeDeclaration(chunks, path, decl, fullNewContent, packageAndImports, &currentChunkContent, &currentChunkStartLine, &chunkIsDefinition, &chunkSymbolType, &chunkIdentifier, lastDeclEndLine, targetChunkSize)
			lastDeclEndLine = endPos.Line
			continue
		}

		// 2. Normal flow: Check if adding to current chunk exceeds limit
		if currentChunkContent.Len() > 0 && (currentChunkContent.Len()+totalAddSize > targetChunkSize) {
			chunks = p.flushChunk(chunks, path, packageAndImports, &currentChunkContent, &currentChunkStartLine, &chunkIsDefinition, &chunkSymbolType, &chunkIdentifier, lastDeclEndLine)

			if isDef {
				chunkIsDefinition = true
				chunkSymbolType = symType
				chunkIdentifier = id
			}
		}

		if currentChunkStartLine == -1 {
			currentChunkStartLine = startPos.Line
			if gapContent != "" && lastDeclEndLine > 0 {
				currentChunkStartLine = lastDeclEndLine + 1
			}
		}

		currentChunkContent.WriteString(fullNewContent)
		currentChunkContent.WriteString("\n\n")
		lastDeclEndLine = endPos.Line
	}

	if currentChunkContent.Len() > 0 {
		chunks = p.flushChunk(chunks, path, packageAndImports, &currentChunkContent, &currentChunkStartLine, &chunkIsDefinition, &chunkSymbolType, &chunkIdentifier, lastDeclEndLine)
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

func (p *GoPlugin) createChunk(id string, isDef bool, symType string, content string, startLine, endLine int) schema.CodeChunk {
	return schema.CodeChunk{
		Content:      content,
		LineStart:    startLine,
		LineEnd:      endLine,
		Type:         "code_group",
		Identifier:   id,
		IsDefinition: isDef,
		SymbolType:   symType,
		Annotations: map[string]string{
			"type": "code_group",
		},
	}
}

func (p *GoPlugin) findDefinitionInfo(decl ast.Decl) (bool, string, string) {
	if f, ok := decl.(*ast.FuncDecl); ok {
		return true, "function", f.Name.Name
	}

	if g, ok := decl.(*ast.GenDecl); ok && g.Tok == token.TYPE {
		for _, spec := range g.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				symType := "type"
				switch typeSpec.Type.(type) {
				case *ast.StructType:
					symType = "struct"
				case *ast.InterfaceType:
					symType = "interface"
				}
				return true, symType, typeSpec.Name.Name
			}
		}
	}

	return false, "", ""
}

func (p *GoPlugin) handleLargeDeclaration(
	chunks []schema.CodeChunk,
	path string,
	decl ast.Decl,
	fullNewContent string,
	packageAndImports string,
	currentChunkContent *strings.Builder,
	currentChunkStartLine *int,
	chunkIsDefinition *bool,
	chunkSymbolType *string,
	chunkIdentifier *string,
	lastDeclEndLine int,
	targetSize int,
) []schema.CodeChunk {
	if currentChunkContent.Len() > 0 {
		chunks = p.flushChunk(chunks, path, packageAndImports, currentChunkContent, currentChunkStartLine, chunkIsDefinition, chunkSymbolType, chunkIdentifier, lastDeclEndLine)
	}

	isDef, symType, id := p.findDefinitionInfo(decl)
	subSplits := p.recursiveSplit(fullNewContent, targetSize, 200)
	for i, sub := range subSplits {
		finalID := id
		if finalID == "" {
			finalID = fmt.Sprintf("%s:large:part%d", path, i)
		} else if i > 0 {
			finalID = fmt.Sprintf("%s:part%d", id, i)
		}

		chunks = append(chunks, p.createChunk(
			finalID,
			isDef && i == 0,
			symType,
			packageAndImports+sub,
			-1,
			-1,
		))
	}
	return chunks
}

func (p *GoPlugin) extractGapContent(lines []string, lastEndLine, currentStartLine int) string {
	if lastEndLine > 0 && currentStartLine > lastEndLine+1 {
		gap := p.extractDeclarationContent(lines, lastEndLine+1, currentStartLine-1)
		if strings.TrimSpace(gap) != "" {
			return gap + "\n\n"
		}
	}
	return ""
}

func (p *GoPlugin) flushChunk(
	chunks []schema.CodeChunk,
	path string,
	packageHeader string,
	content *strings.Builder,
	startLine *int,
	isDef *bool,
	symType *string,
	id *string,
	endLine int,
) []schema.CodeChunk {
	finalID := *id
	if finalID == "" {
		finalID = fmt.Sprintf("%s:chunk:%d-%d", path, *startLine, endLine)
	}
	chunks = append(chunks, p.createChunk(
		finalID,
		*isDef,
		*symType,
		packageHeader+content.String(),
		*startLine,
		endLine,
	))

	content.Reset()
	*startLine = -1
	*isDef = false
	*symType = ""
	*id = ""

	return chunks
}
