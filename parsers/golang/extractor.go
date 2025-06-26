package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	model "github.com/sevigo/goframe/schema"
)

// ExtractMetadata extracts language-specific metadata from Go code
func (p *GoPlugin) ExtractMetadata(content string, path string) (model.FileMetadata, error) {
	metadata := model.FileMetadata{
		Language:    "go",
		Imports:     []string{},
		Definitions: []model.CodeEntityDefinition{},
		Symbols:     []model.CodeSymbol{},
		Properties:  map[string]string{},
	}

	// Parse the Go file
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return metadata, fmt.Errorf("failed to parse Go file: %w", err)
	}

	// Extract package name
	if file.Name != nil {
		metadata.Properties["package"] = file.Name.Name
	}

	// Extract imports
	metadata.Imports = p.extractImports(file)

	// Extract definitions and symbols
	p.extractFunctionMetadata(fset, file, &metadata)
	p.extractTypeMetadata(fset, file, &metadata)
	p.extractVarConstMetadata(fset, file, &metadata)

	// Add additional properties
	metadata.Properties["total_functions"] = strconv.Itoa(p.countFunctions(file))
	metadata.Properties["total_types"] = strconv.Itoa(p.countTypes(file))
	metadata.Properties["total_methods"] = strconv.Itoa(p.countMethods(file))

	return metadata, nil
}

// extractImports extracts import statements
func (p *GoPlugin) extractImports(file *ast.File) []string {
	var imports []string

	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		imports = append(imports, importPath)
	}

	return imports
}

// extractFunctionMetadata extracts function and method metadata
func (p *GoPlugin) extractFunctionMetadata(fset *token.FileSet, file *ast.File, metadata *model.FileMetadata) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		startPos := fset.Position(fn.Pos())
		endPos := fset.Position(fn.End())

		fnName := fn.Name.Name
		isMethod := fn.Recv != nil && len(fn.Recv.List) > 0

		def := model.CodeEntityDefinition{
			Type:       "function",
			Name:       fnName,
			LineStart:  startPos.Line,
			LineEnd:    endPos.Line,
			Visibility: p.getVisibility(fnName),
			Signature:  p.getFunctionSignature(fn),
		}

		if isMethod {
			def.Type = "method"
			recv := p.getReceiverType(fn.Recv.List[0].Type)
			def.Signature = fmt.Sprintf("(%s) %s", recv, def.Signature[5:]) // Remove "func " prefix
		}

		if fn.Doc != nil {
			def.Documentation = p.extractDocComment(fn.Doc)
		}

		metadata.Definitions = append(metadata.Definitions, def)

		symbol := model.CodeSymbol{
			Name:      fnName,
			Type:      def.Type,
			LineStart: startPos.Line,
			LineEnd:   endPos.Line,
			IsExport:  ast.IsExported(fnName),
		}

		metadata.Symbols = append(metadata.Symbols, symbol)
	}
}

// extractTypeMetadata extracts type declaration metadata
func (p *GoPlugin) extractTypeMetadata(fset *token.FileSet, file *ast.File, metadata *model.FileMetadata) {
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

			startPos := fset.Position(typeSpec.Pos())
			endPos := fset.Position(typeSpec.End())

			typeKind := "type"
			var additionalInfo string

			switch t := typeSpec.Type.(type) {
			case *ast.StructType:
				typeKind = "struct"
				if t.Fields != nil {
					additionalInfo = fmt.Sprintf("fields:%d", len(t.Fields.List))
				}
			case *ast.InterfaceType:
				typeKind = "interface"
				if t.Methods != nil {
					additionalInfo = fmt.Sprintf("methods:%d", len(t.Methods.List))
				}
			case *ast.Ident:
				typeKind = "alias"
				additionalInfo = fmt.Sprintf("alias_of:%s", t.Name)
			case *ast.ArrayType:
				typeKind = "array_type"
			case *ast.MapType:
				typeKind = "map_type"
			case *ast.ChanType:
				typeKind = "channel_type"
			case *ast.FuncType:
				typeKind = "function_type"
			}

			def := model.CodeEntityDefinition{
				Type:       typeKind,
				Name:       typeSpec.Name.Name,
				LineStart:  startPos.Line,
				LineEnd:    endPos.Line,
				Visibility: p.getVisibility(typeSpec.Name.Name),
			}

			// Add documentation if present
			if genDecl.Doc != nil {
				def.Documentation = p.extractDocComment(genDecl.Doc)
			}

			// Add additional type information
			if additionalInfo != "" {
				def.Signature = additionalInfo
			}

			metadata.Definitions = append(metadata.Definitions, def)

			// Add as symbol
			symbol := model.CodeSymbol{
				Name:      typeSpec.Name.Name,
				Type:      typeKind,
				LineStart: startPos.Line,
				LineEnd:   endPos.Line,
				IsExport:  ast.IsExported(typeSpec.Name.Name),
			}

			metadata.Symbols = append(metadata.Symbols, symbol)
		}
	}
}

// extractVarConstMetadata extracts variable and constant metadata
func (p *GoPlugin) extractVarConstMetadata(fset *token.FileSet, file *ast.File, metadata *model.FileMetadata) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || (genDecl.Tok != token.VAR && genDecl.Tok != token.CONST) {
			continue
		}

		for _, spec := range genDecl.Specs {
			valSpec, valSpecOK := spec.(*ast.ValueSpec)
			if !valSpecOK {
				continue
			}

			startPos := fset.Position(valSpec.Pos())
			endPos := fset.Position(valSpec.End())

			for _, name := range valSpec.Names {
				declType := "variable"
				if genDecl.Tok == token.CONST {
					declType = "constant"
				}

				// Create definition
				def := model.CodeEntityDefinition{
					Type:       declType,
					Name:       name.Name,
					LineStart:  startPos.Line,
					LineEnd:    endPos.Line,
					Visibility: p.getVisibility(name.Name),
				}

				// Add documentation if present
				if genDecl.Doc != nil {
					def.Documentation = p.extractDocComment(genDecl.Doc)
				}

				// Add type information if available
				if valSpec.Type != nil {
					def.Signature = p.exprToString(valSpec.Type)
				}

				metadata.Definitions = append(metadata.Definitions, def)

				// Add as symbol
				symbol := model.CodeSymbol{
					Name:      name.Name,
					Type:      declType,
					LineStart: startPos.Line,
					LineEnd:   endPos.Line,
					IsExport:  ast.IsExported(name.Name),
				}

				metadata.Symbols = append(metadata.Symbols, symbol)
			}
		}
	}
}

// countFunctions counts the total number of functions in the file
func (p *GoPlugin) countFunctions(file *ast.File) int {
	count := 0
	for _, decl := range file.Decls {
		if _, ok := decl.(*ast.FuncDecl); ok {
			count++
		}
	}
	return count
}

// countTypes counts the total number of type declarations in the file
func (p *GoPlugin) countTypes(file *ast.File) int {
	count := 0
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			count += len(genDecl.Specs)
		}
	}
	return count
}

// countMethods counts the total number of methods (functions with receivers) in the file
func (p *GoPlugin) countMethods(file *ast.File) int {
	count := 0
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				count++
			}
		}
	}
	return count
}
