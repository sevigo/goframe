package contextpacker

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// DefaultTemplate is the default format for packing documents.
const DefaultTemplate = `{{.Content}}

---
Metadata: {{.Metadata}}
---`

// CompactTemplate is a minimal format without metadata.
const CompactTemplate = `{{.Content}}`

// templateData holds data for template execution.
type templateData struct {
	Content  string
	Metadata string
}

// formatDocument formats a single document using the template.
func formatDocument(tmpl *template.Template, doc documentWithTokens) (string, error) {
	data := templateData{
		Content:  doc.doc.PageContent,
		Metadata: formatMetadata(doc.doc.Metadata),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// formatMetadata converts metadata map to a string representation.
// Keys are sorted alphabetically for deterministic output.
func formatMetadata(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(formatValue(m[k]))
	}
	sb.WriteString("}")
	return sb.String()
}

func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
