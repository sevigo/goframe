package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

func (p *GoPlugin) Chunk(content string, path string, opts *model.CodeChunkingOptions) ([]model.CodeChunk, error) {
	var chunks []model.CodeChunk

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	chunks = append(chunks, p.extractFunctionChunks(content, fset, file)...)
	chunks = append(chunks, p.extractTypeChunks(content, fset, file)...)
	chunks = append(chunks, p.extractVarConstChunks(content, fset, file)...)

	if len(chunks) == 0 {
		return nil, errors.New("no semantic chunks found in Go file")
	}

	p.logger.Debug("Created chunks for Go file", "count", len(chunks), "path", path)
	for i, chunk := range chunks {
		p.logger.Debug("Chunk info",
			"index", i,
			"type", chunk.Type,
			"identifier", chunk.Identifier,
			"lines", fmt.Sprintf("%d-%d", chunk.LineStart, chunk.LineEnd),
		)
	}

	return chunks, nil
}

func (p *GoPlugin) extractFunctionChunks(content string, fset *token.FileSet, file *ast.File) []model.CodeChunk {
	var chunks []model.CodeChunk
	lines := strings.Split(content, "\n")

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		startPos := fset.Position(fn.Pos())
		endPos := fset.Position(fn.End())

		var identifier string
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			receiverText := p.getExactReceiverText(content, fset, fn.Recv)
			if receiverText != "" {
				identifier = fmt.Sprintf("%s %s", receiverText, fn.Name.Name)
			} else {
				recv := p.getReceiverType(fn.Recv.List[0].Type)
				identifier = fmt.Sprintf("(%s) %s", recv, fn.Name.Name)
			}
		} else {
			identifier = fn.Name.Name
		}

		// Extract content between the start and end positions
		startLine := startPos.Line - 1 // 0-based index for slicing
		endLine := min(endPos.Line, len(lines))

		// Include documentation comment if present
		chunkContent := ""
		docComment := p.extractDocComment(fn.Doc)
		if docComment != "" {
			chunkContent = docComment + "\n"
		}
		chunkContent += strings.Join(lines[startLine:endLine], "\n")

		// Create annotations
		annotations := map[string]string{
			"type": "function",
			"name": fn.Name.Name,
		}

		// Add receiver type if method
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recv := p.getReceiverType(fn.Recv.List[0].Type)
			annotations["receiver"] = recv
			annotations["is_method"] = "true"
		} else {
			annotations["is_method"] = "false"
		}

		// Add function signature
		annotations["signature"] = p.getFunctionSignature(fn)

		// Add visibility
		annotations["visibility"] = p.getVisibility(fn.Name.Name)

		// Add documentation if present
		if docComment != "" {
			annotations["has_doc"] = "true"
		}

		// Log the chunk being created
		p.logger.Debug("Creating function chunk",
			"identifier", identifier,
			"start", startPos.Line,
			"end", endPos.Line,
			"hasReceiver", fn.Recv != nil,
		)

		// Create chunk
		chunk := model.CodeChunk{
			Content:       chunkContent,
			LineStart:     startPos.Line,
			LineEnd:       endPos.Line,
			Type:          "function",
			Identifier:    identifier,
			Annotations:   annotations,
			ParentContext: p.buildParentContext(file, fn),
			ContextLevel:  2, // function level
		}

		chunks = append(chunks, chunk)
	}

	return chunks
}

// Update buildParentContext to remove import redundancy
func (p *GoPlugin) buildParentContext(file *ast.File, fn *ast.FuncDecl) string {
	var context strings.Builder

	// Add package context
	if file.Name != nil {
		context.WriteString(fmt.Sprintf("// Package: %s\n", file.Name.Name))
	}

	// Add receiver context for methods (this is the key structural info)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := p.getReceiverType(fn.Recv.List[0].Type)
		context.WriteString(fmt.Sprintf("// Method of: %s\n", recv))

		// Add any related type methods context if useful
		context.WriteString(fmt.Sprintf("// Function: %s\n", fn.Name.Name))
	} else {
		// For standalone functions, add function signature context
		context.WriteString(fmt.Sprintf("// Function: %s\n", fn.Name.Name))
	}

	return context.String()
}

func (p *GoPlugin) extractTypeChunks(content string, fset *token.FileSet, file *ast.File) []model.CodeChunk {
	var chunks []model.CodeChunk
	lines := strings.Split(content, "\n")

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, typeSpecOK := spec.(*ast.TypeSpec)
			if !typeSpecOK {
				continue
			}

			startPos := fset.Position(genDecl.Pos())
			endPos := fset.Position(genDecl.End())

			startLine := startPos.Line - 1
			endLine := min(endPos.Line, len(lines))

			chunkContent := ""
			docComment := p.extractDocComment(genDecl.Doc)
			if docComment != "" {
				chunkContent = docComment + "\n"
			}
			chunkContent += strings.Join(lines[startLine:endLine], "\n")

			typeName := typeSpec.Name.Name
			annotations := map[string]string{
				"type":       "type_declaration",
				"name":       typeName,
				"visibility": p.getVisibility(typeName),
			}

			if docComment != "" {
				annotations["has_doc"] = "true"
			}

			if structType, structTypeOK := typeSpec.Type.(*ast.StructType); structTypeOK {
				annotations["structure_type"] = "struct"

				if structType.Fields != nil && len(structType.Fields.List) > 0 {
					fieldCount := len(structType.Fields.List)
					annotations["field_count"] = strconv.Itoa(fieldCount)

					var fieldNames []string
					for _, field := range structType.Fields.List {
						for _, name := range field.Names {
							fieldNames = append(fieldNames, name.Name)
						}
					}
					if len(fieldNames) > 0 {
						annotations["field_names"] = strings.Join(fieldNames, ",")
					}
				}
			}

			// Add interface methods if it's an interface
			if interfaceType, interfaceTypeOK := typeSpec.Type.(*ast.InterfaceType); interfaceTypeOK {
				annotations["structure_type"] = "interface"

				if interfaceType.Methods != nil && len(interfaceType.Methods.List) > 0 {
					methodCount := len(interfaceType.Methods.List)
					annotations["method_count"] = strconv.Itoa(methodCount)

					// Extract method names for better context
					var methodNames []string
					for _, method := range interfaceType.Methods.List {
						for _, name := range method.Names {
							methodNames = append(methodNames, name.Name)
						}
					}
					if len(methodNames) > 0 {
						annotations["method_names"] = strings.Join(methodNames, ",")
					}
				}
			}

			// Handle other type kinds
			if _, isAlias := typeSpec.Type.(*ast.Ident); isAlias {
				annotations["structure_type"] = "alias"
			}

			// Log the chunk being created
			p.logger.Debug("Creating type declaration chunk",
				"identifier", typeName,
				"start", startPos.Line,
				"end", endPos.Line,
			)

			// Create chunk
			chunk := model.CodeChunk{
				Content:     chunkContent,
				LineStart:   startPos.Line,
				LineEnd:     endPos.Line,
				Type:        "type_declaration",
				Identifier:  typeName,
				Annotations: annotations,
			}

			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

// extractVarConstChunks extracts top-level variable and constant declaration chunks
func (p *GoPlugin) extractVarConstChunks(
	content string,
	fset *token.FileSet,
	file *ast.File,
) []model.CodeChunk {
	var chunks []model.CodeChunk
	lines := strings.Split(content, "\n")

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.VAR && genDecl.Tok != token.CONST) {
			continue
		}

		if !p.shouldCreateVarConstChunk(genDecl) {
			continue
		}

		if len(genDecl.Specs) == 1 {
			if chunk := p.createIndividualVarConstChunk(genDecl, fset, lines); chunk != nil {
				chunks = append(chunks, *chunk)
			}
		} else {
			chunk := p.createGroupedVarConstChunk(genDecl, fset, lines)
			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

// shouldCreateVarConstChunk determines if a variable/constant declaration should become a chunk
func (p *GoPlugin) shouldCreateVarConstChunk(genDecl *ast.GenDecl) bool {
	hasExported := false
	hasDoc := genDecl.Doc != nil && len(genDecl.Doc.List) > 0

	// Check if any of the declarations are exported
	for _, spec := range genDecl.Specs {
		if valSpec, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range valSpec.Names {
				if ast.IsExported(name.Name) {
					hasExported = true
					break
				}
			}
			if hasExported {
				break
			}
		}
	}

	return hasExported || hasDoc
}

// createIndividualVarConstChunk creates a chunk for a single variable/constant declaration
func (p *GoPlugin) createIndividualVarConstChunk(
	genDecl *ast.GenDecl,
	fset *token.FileSet,
	lines []string,
) *model.CodeChunk {
	spec := genDecl.Specs[0]
	valSpec, ok := spec.(*ast.ValueSpec)
	if !ok || len(valSpec.Names) != 1 {
		return nil
	}

	name := valSpec.Names[0]
	startPos := fset.Position(genDecl.Pos())
	endPos := fset.Position(genDecl.End())

	chunkContent := p.buildChunkContent(genDecl, startPos.Line-1, min(endPos.Line, len(lines)), lines)
	declType := p.getDeclType(genDecl.Tok)

	annotations := map[string]string{
		"type":       declType,
		"names":      name.Name,
		"count":      "1",
		"visibility": p.getVisibility(name.Name),
	}

	if genDecl.Doc != nil && len(genDecl.Doc.List) > 0 {
		annotations["has_doc"] = "true"
	}

	p.logger.Debug("Creating individual var/const chunk",
		"identifier", name.Name,
		"type", declType,
		"start", startPos.Line,
		"end", endPos.Line,
	)

	return &model.CodeChunk{
		Content:     chunkContent,
		LineStart:   startPos.Line,
		LineEnd:     endPos.Line,
		Type:        declType,
		Identifier:  name.Name,
		Annotations: annotations,
	}
}

// createGroupedVarConstChunk creates a chunk for grouped variable/constant declarations
func (p *GoPlugin) createGroupedVarConstChunk(
	genDecl *ast.GenDecl,
	fset *token.FileSet,
	lines []string,
) model.CodeChunk {
	startPos := fset.Position(genDecl.Pos())
	endPos := fset.Position(genDecl.End())

	chunkContent := p.buildChunkContent(genDecl, startPos.Line-1, min(endPos.Line, len(lines)), lines)
	declType := p.getDeclType(genDecl.Tok)
	identifiers := p.extractIdentifiers(genDecl)
	identifier := strings.Join(identifiers, ", ")

	annotations := map[string]string{
		"type":       declType,
		"names":      identifier,
		"count":      strconv.Itoa(len(identifiers)),
		"visibility": p.determineGroupVisibility(identifiers),
	}

	if genDecl.Doc != nil && len(genDecl.Doc.List) > 0 {
		annotations["has_doc"] = "true"
	}

	p.logger.Debug("Creating grouped var/const chunk",
		"identifier", identifier,
		"type", declType,
		"start", startPos.Line,
		"end", endPos.Line,
	)

	return model.CodeChunk{
		Content:     chunkContent,
		LineStart:   startPos.Line,
		LineEnd:     endPos.Line,
		Type:        declType,
		Identifier:  identifier,
		Annotations: annotations,
	}
}

// buildChunkContent constructs the content for a chunk including documentation
func (p *GoPlugin) buildChunkContent(genDecl *ast.GenDecl, startLine, endLine int, lines []string) string {
	chunkContent := ""
	if genDecl.Doc != nil && len(genDecl.Doc.List) > 0 {
		chunkContent = p.extractRawDocComment(genDecl.Doc) + "\n\n"
	}
	chunkContent += strings.Join(lines[startLine:endLine], "\n")
	return chunkContent
}

// getDeclType returns the string representation of the declaration type
func (p *GoPlugin) getDeclType(tok token.Token) string {
	if tok == token.CONST {
		return "constant"
	}
	return "variable"
}

// extractIdentifiers extracts all identifiers from a GenDecl
func (p *GoPlugin) extractIdentifiers(genDecl *ast.GenDecl) []string {
	var identifiers []string
	for _, spec := range genDecl.Specs {
		if valSpec, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range valSpec.Names {
				identifiers = append(identifiers, name.Name)
			}
		}
	}
	return identifiers
}

// determineGroupVisibility determines the visibility for a group of identifiers
func (p *GoPlugin) determineGroupVisibility(identifiers []string) string {
	allPublic := true
	allPrivate := true

	for _, name := range identifiers {
		if ast.IsExported(name) {
			allPrivate = false
		} else {
			allPublic = false
		}
	}

	if allPublic {
		return "public"
	} else if allPrivate {
		return "private"
	}
	return "mixed"
}
