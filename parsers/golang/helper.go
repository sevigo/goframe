package golang

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// getReceiverType extracts the receiver type name with improved handling
func (p *GoPlugin) getReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return "*" + ident.Name
		}
		// Handle more complex pointer types
		return "*" + p.exprToString(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return p.exprToString(t)
	default:
		p.logger.Warn("Unknown receiver type encountered", "type", fmt.Sprintf("%T", t))
		return "unknown"
	}
}

// exprToString converts an ast.Expr to a string representation with enhanced support
func (p *GoPlugin) exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + p.exprToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + p.exprToString(t.Elt)
		}
		return "[" + p.exprToString(t.Len) + "]" + p.exprToString(t.Elt)
	case *ast.SelectorExpr:
		return p.exprToString(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + p.exprToString(t.Key) + "]" + p.exprToString(t.Value)
	case *ast.InterfaceType:
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}" // Non-empty interface
	case *ast.StructType:
		return "struct{...}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + p.exprToString(t.Value)
		case ast.RECV:
			return "<-chan " + p.exprToString(t.Value)
		default:
			return "chan " + p.exprToString(t.Value)
		}
	case *ast.Ellipsis:
		return "..." + p.exprToString(t.Elt)
	case *ast.BasicLit:
		return t.Value
	case *ast.ParenExpr:
		return "(" + p.exprToString(t.X) + ")"
	default:
		p.logger.Debug("Unhandled expression type in exprToString", "type", fmt.Sprintf("%T", expr))
		return "unknown"
	}
}

// getFunctionSignature returns a string representation of the function signature
func (p *GoPlugin) getFunctionSignature(fn *ast.FuncDecl) string {
	var sb strings.Builder

	// Add receiver if it's a method
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := p.getReceiverType(fn.Recv.List[0].Type)
		sb.WriteString("func (" + recv + ") ")
	} else {
		sb.WriteString("func ")
	}

	sb.WriteString(fn.Name.Name + "(")

	// Add parameters
	if fn.Type.Params != nil && fn.Type.Params.List != nil {
		for i, param := range fn.Type.Params.List {
			if i > 0 {
				sb.WriteString(", ")
			}

			// Parameter names
			for j, name := range param.Names {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(name.Name)
			}

			// Parameter type
			if len(param.Names) > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p.exprToString(param.Type))
		}
	}

	sb.WriteString(")")

	// Add return values
	if fn.Type.Results != nil && fn.Type.Results.List != nil {
		if len(fn.Type.Results.List) == 1 && len(fn.Type.Results.List[0].Names) == 0 {
			// Single unnamed return value
			sb.WriteString(" ")
			sb.WriteString(p.exprToString(fn.Type.Results.List[0].Type))
		} else {
			// Multiple or named return values
			sb.WriteString(" (")
			for i, result := range fn.Type.Results.List {
				if i > 0 {
					sb.WriteString(", ")
				}

				// Names if any
				for j, name := range result.Names {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(name.Name)
				}

				// Type
				if len(result.Names) > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(p.exprToString(result.Type))
			}
			sb.WriteString(")")
		}
	}

	return sb.String()
}

// extractDocComment extracts and formats documentation comments
func (p *GoPlugin) extractDocComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	return doc.Text()
}

// extractRawDocComment extracts raw documentation comments preserving original format
func (p *GoPlugin) extractRawDocComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}

	var lines []string
	for _, comment := range doc.List {
		// ast.Comment.Text contains the raw comment including // or /* */
		lines = append(lines, comment.Text)
	}
	return strings.Join(lines, "\n")
}

// getExactReceiverText extracts the exact receiver text from source
func (p *GoPlugin) getExactReceiverText(content string, fset *token.FileSet, recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	receiverStart := fset.Position(recv.Pos())
	receiverEnd := fset.Position(recv.End())

	lines := strings.Split(content, "\n")
	if receiverStart.Line-1 >= len(lines) {
		return ""
	}

	receiverLine := lines[receiverStart.Line-1]

	// Extract the exact receiver text
	if receiverEnd.Column > len(receiverLine) || receiverStart.Column-1 < 0 {
		return ""
	}

	receiverText := receiverLine[receiverStart.Column-1 : receiverEnd.Column-1]
	receiverText = strings.TrimSpace(receiverText)

	// Ensure proper parentheses
	if !strings.HasPrefix(receiverText, "(") {
		receiverText = "(" + receiverText + ")"
	}

	return receiverText
}
