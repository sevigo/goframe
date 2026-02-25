package output

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// tokenArtifactRegex safely fixes common LLM spacing artifacts in closing tags (e.g., "</ review>").
var tokenArtifactRegex = regexp.MustCompile(`</\s+([a-zA-Z])`)

// XMLParser implements schema.OutputParser[T] for any XML-tagged LLM output.
type XMLParser[T any] struct {
	RootTag string
}

func NewXMLParser[T any](rootTag string) *XMLParser[T] {
	return &XMLParser[T]{RootTag: rootTag}
}

func (p *XMLParser[T]) Parse(ctx context.Context, text string) (T, error) {
	var result T

	// 1. Basic Cleaning
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// Fix common LLM tokenization artifacts safely using regex
	text = tokenArtifactRegex.ReplaceAllString(text, "</$1")

	// 2. Strip Markdown Fences (Memory Safe & Preamble Safe)
	text = stripMarkdownFence(text)

	// 3. Truncation Recovery (Case-Insensitive Best Effort)
	lowerText := strings.ToLower(text)
	lowerRoot := strings.ToLower(p.RootTag)
	openTag := "<" + lowerRoot + ">"
	closeTag := "</" + lowerRoot + ">"

	// If we find the open tag but lack the close tag, append it
	if strings.Contains(lowerText, openTag) && !strings.Contains(lowerText, closeTag) {
		text += "</" + p.RootTag + ">"
	}

	// 4. Decoding
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose

	for {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		t, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return result, fmt.Errorf("xml tokenization error: %w", err)
		}

		if se, ok := t.(xml.StartElement); ok {
			// XML standard is case-sensitive, but LLMs aren't.
			if strings.EqualFold(se.Name.Local, p.RootTag) {
				err := decoder.DecodeElement(&result, &se)
				if err != nil {
					// Return partial result alongside the error (crucial for truncated LLM responses)
					return result, fmt.Errorf("xml unmarshal error (possible truncation): %w", err)
				}
				return result, nil
			}
		}
	}

	return result, fmt.Errorf("root XML tag '%s' not found in LLM output", p.RootTag)
}

// stripMarkdownFence extracts content from markdown fences without allocating a slice of lines.
// It is immune to LLM conversational preambles.
func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)

	// Find the FIRST markdown fence (ignoring preamble text before it)
	startIdx := strings.Index(s, "```")
	if startIdx == -1 {
		return s
	}

	// Find the end of the fence header line (e.g., after ```xml)
	newlineIdx := strings.Index(s[startIdx:], "\n")
	if newlineIdx == -1 {
		// It's a one-liner like ```xml<root/>```
		// Check if it ends with ```
		if strings.HasSuffix(s, "```") {
			return strings.TrimSpace(s[startIdx+3 : len(s)-3])
		}
		// Unclosed one-liner
		return strings.TrimSpace(s[startIdx+3:])
	}

	// Content starts after the newline following the opening backticks
	contentStart := startIdx + newlineIdx + 1

	// Find the NEXT closing fence in the remainder of the string
	remainder := s[contentStart:]
	closingFence := strings.Index(remainder, "```")

	if closingFence == -1 {
		// Unclosed fence, return everything after the header
		return strings.TrimSpace(remainder)
	}

	// Closed fence, return content strictly between the fences
	return strings.TrimSpace(remainder[:closingFence])
}
