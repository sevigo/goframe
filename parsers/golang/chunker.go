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

// Chunk breaks Go code into semantic chunks
func (p *GoPlugin) Chunk(content string, path string, opts *model.CodeChunkingOptions) ([]model.CodeChunk, error) {
	var chunks []model.CodeChunk

	// Parse the Go file
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		// Return error to let LanguageAwareChunker handle fallback
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	// Extract functions as chunks
	chunks = append(chunks, p.extractFunctionChunks(content, fset, file)...)

	// Extract type declarations as chunks
	chunks = append(chunks, p.extractTypeChunks(content, fset, file)...)

	// Extract top-level variable and constant declarations as chunks
	chunks = append(chunks, p.extractVarConstChunks(content, fset, file)...)

	// If no semantic chunks found, return error to trigger fallback
	if len(chunks) == 0 {
		return nil, errors.New("no semantic chunks found in Go file")
	}

	// Log the total chunks created
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

// extractFunctionChunks extracts function and method chunks
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

		// Get function name with receiver if it exists
		var identifier string
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			receiverText := p.getExactReceiverText(content, fset, fn.Recv)
			if receiverText != "" {
				identifier = fmt.Sprintf("%s %s", receiverText, fn.Name.Name)
			} else {
				// Fallback to constructed receiver text
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

// extractTypeChunks extracts type declaration chunks
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

			// Extract content
			startLine := startPos.Line - 1
			endLine := min(endPos.Line, len(lines))

			// Include documentation comment if present
			chunkContent := ""
			docComment := p.extractDocComment(genDecl.Doc)
			if docComment != "" {
				chunkContent = docComment + "\n"
			}
			chunkContent += strings.Join(lines[startLine:endLine], "\n")

			// Create annotations
			typeName := typeSpec.Name.Name
			annotations := map[string]string{
				"type":       "type_declaration",
				"name":       typeName,
				"visibility": p.getVisibility(typeName),
			}

			// Add documentation if present
			if docComment != "" {
				annotations["has_doc"] = "true"
			}

			// Add struct fields if it's a struct
			if structType, structTypeOK := typeSpec.Type.(*ast.StructType); structTypeOK {
				annotations["structure_type"] = "struct"

				if structType.Fields != nil && len(structType.Fields.List) > 0 {
					fieldCount := len(structType.Fields.List)
					annotations["field_count"] = strconv.Itoa(fieldCount)

					// Extract field names for better context
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
func (p *GoPlugin) extractVarConstChunks( //nolint:funlen //fixme
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

		// Only create chunks for exported variables/constants or those with documentation
		hasExported := false
		hasDoc := genDecl.Doc != nil && len(genDecl.Doc.List) > 0

		// Check if any of the declarations are exported
		for _, spec := range genDecl.Specs {
			if valSpec, valSpecOK := spec.(*ast.ValueSpec); valSpecOK {
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

		// Skip if not exported and no documentation
		if !hasExported && !hasDoc {
			continue
		}

		// For individual declarations (not grouped), create individual chunks
		if len(genDecl.Specs) == 1 {
			spec := genDecl.Specs[0]
			if valSpec, valSpecOK := spec.(*ast.ValueSpec); valSpecOK && len(valSpec.Names) == 1 {
				name := valSpec.Names[0]

				startPos := fset.Position(genDecl.Pos())
				endPos := fset.Position(genDecl.End())

				// Extract content
				startLine := startPos.Line - 1
				endLine := min(endPos.Line, len(lines))

				// Include documentation comment if present
				chunkContent := ""
				docComment := p.extractDocComment(genDecl.Doc)
				if docComment != "" {
					// Preserve original comment format for better recognition
					chunkContent = p.extractRawDocComment(genDecl.Doc) + "\n\n"
				}
				chunkContent += strings.Join(lines[startLine:endLine], "\n")

				declType := "variable"
				if genDecl.Tok == token.CONST {
					declType = "constant"
				}

				// Create annotations
				annotations := map[string]string{
					"type":       declType,
					"names":      name.Name,
					"count":      "1",
					"visibility": p.getVisibility(name.Name),
				}

				// Add documentation if present
				if docComment != "" {
					annotations["has_doc"] = "true"
				}

				// Log the chunk being created
				p.logger.Debug("Creating individual var/const chunk",
					"identifier", name.Name,
					"type", declType,
					"start", startPos.Line,
					"end", endPos.Line,
				)

				// Create chunk
				chunk := model.CodeChunk{
					Content:     chunkContent,
					LineStart:   startPos.Line,
					LineEnd:     endPos.Line,
					Type:        declType,
					Identifier:  name.Name,
					Annotations: annotations,
				}

				chunks = append(chunks, chunk)
				continue
			}
		}

		// For grouped declarations, create a single chunk
		startPos := fset.Position(genDecl.Pos())
		endPos := fset.Position(genDecl.End())

		// Extract content
		startLine := startPos.Line - 1
		endLine := min(endPos.Line, len(lines))

		// Include documentation comment if present
		chunkContent := ""
		docComment := p.extractDocComment(genDecl.Doc)
		if docComment != "" {
			// Preserve original comment format for better recognition
			chunkContent = p.extractRawDocComment(genDecl.Doc) + "\n\n"
		}
		chunkContent += strings.Join(lines[startLine:endLine], "\n")

		// Create identifier from all names in the declaration
		var identifiers []string
		declType := "variable"
		if genDecl.Tok == token.CONST {
			declType = "constant"
		}

		for _, spec := range genDecl.Specs {
			if valSpec, valSpecOK := spec.(*ast.ValueSpec); valSpecOK {
				for _, name := range valSpec.Names {
					identifiers = append(identifiers, name.Name)
				}
			}
		}

		if len(identifiers) == 0 {
			continue
		}

		identifier := strings.Join(identifiers, ", ")

		// Create annotations
		annotations := map[string]string{
			"type":       declType,
			"names":      identifier,
			"count":      strconv.Itoa(len(identifiers)),
			"visibility": "mixed", // Default, will be refined below
		}

		// Add documentation if present
		if docComment != "" {
			annotations["has_doc"] = "true"
		}

		// Determine overall visibility
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
			annotations["visibility"] = "public"
		} else if allPrivate {
			annotations["visibility"] = "private"
		}

		// Log the chunk being created
		p.logger.Debug("Creating grouped var/const chunk",
			"identifier", identifier,
			"type", declType,
			"start", startPos.Line,
			"end", endPos.Line,
		)

		// Create chunk
		chunk := model.CodeChunk{
			Content:     chunkContent,
			LineStart:   startPos.Line,
			LineEnd:     endPos.Line,
			Type:        declType,
			Identifier:  identifier,
			Annotations: annotations,
		}

		chunks = append(chunks, chunk)
	}

	return chunks
}
