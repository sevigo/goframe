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

var (
	// strayLtRegex matches '<' followed by a character that is NOT a valid tag start.
	// Valid tag starts: letters, _, /, !, or ? (for processing instructions like <?xml)
	strayLtRegex = regexp.MustCompile(`(<)([^a-zA-Z_/!?])`)

	// entityRegex matches existing XML entities to avoid double-escaping.
	entityRegex = regexp.MustCompile(`&(?:amp|lt|gt|quot|apos|#\d+|#x[a-fA-F0-9]+);`)

	// tokenArtifactRegex safely fixes common LLM spacing artifacts in closing tags
	tokenArtifactRegex = regexp.MustCompile(`</\s+([a-zA-Z])`)
)

type XMLParser[T any] struct {
	RootTag string
}

func NewXMLParser[T any](rootTag string) *XMLParser[T] {
	return &XMLParser[T]{RootTag: rootTag}
}

func (p *XMLParser[T]) Parse(ctx context.Context, text string) (T, error) {
	var result T

	// basic cleaning
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// fix common LLM tokenization artifacts safely
	text = tokenArtifactRegex.ReplaceAllString(text, "</$1")

	// strip markdown fences (memory safe & preamble safe)
	// only strip if the fence appears BEFORE the root tag.
	// this ensures we strip wrapping fences, but not fences inside content strings.
	startFenceIdx := strings.Index(text, "```")

	// look for the specific root tag to be safer against preambles containing math (x < y)
	// We use EqualFold-style logic by lowercase comparison for the check
	rootTagLower := strings.ToLower(p.RootTag)
	textLower := strings.ToLower(text)
	rootTagIdx := strings.Index(textLower, "<"+rootTagLower)

	// If we found a fence, and (we didn't find a root tag OR the fence is before the root tag)
	if startFenceIdx != -1 && (rootTagIdx == -1 || startFenceIdx < rootTagIdx) {
		text = stripMarkdownFence(text)
	}

	// LLM-specific XML sanitization
	// fix unescaped characters like "if a < b" -> "if a &lt; b"
	text = sanitizeLLMXML(text)

	// truncation recovery
	// Re-calculate lowercase text after sanitization changes
	textLower = strings.ToLower(text)
	openTag := "<" + rootTagLower + ">"
	closeTag := "</" + rootTagLower + ">"

	if strings.Contains(textLower, openTag) && !strings.Contains(textLower, closeTag) {
		text += "</" + p.RootTag + ">"
	}

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
			if strings.EqualFold(se.Name.Local, p.RootTag) {
				err := decoder.DecodeElement(&result, &se)
				if err != nil {
					// Return partial result on truncation/error
					return result, fmt.Errorf("xml unmarshal error (possible truncation): %w", err)
				}
				return result, nil
			}
		}
	}

	return result, fmt.Errorf("root XML tag '%s' not found in LLM output", p.RootTag)
}

// sanitizeLLMXML fixes common unescaped characters in LLM-generated XML.
func sanitizeLLMXML(text string) string {
	// fix stray '<' (not followed by a valid tag start character)
	text = strayLtRegex.ReplaceAllString(text, "&lt;$2")

	// fix stray ampersands while preserving existing entities.
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = entityRegex.ReplaceAllStringFunc(text, func(entity string) string {
		// "Un-escape" valid entities that got double-escaped
		// e.g. "&amp;lt;" -> "&lt;"
		return strings.Replace(entity, "&amp;", "&", 1)
	})

	return text
}

// stripMarkdownFence extracts content from markdown fences without allocating a slice of lines.
func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}

	newlineIdx := strings.Index(s, "\n")
	if newlineIdx == -1 {
		if strings.HasSuffix(s, "```") {
			return strings.TrimSpace(s[3 : len(s)-3])
		}
		return strings.TrimSpace(s[3:])
	}

	contentStart := newlineIdx + 1
	remainder := s[contentStart:]
	closingFence := strings.Index(remainder, "```")

	if closingFence == -1 {
		return strings.TrimSpace(remainder)
	}

	return strings.TrimSpace(remainder[:closingFence])
}
