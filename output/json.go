package output

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// JSONParser implements schema.OutputParser[T] for JSON-formatted LLM output.
// It is designed to be robust against common LLM artifacts like markdown fences
// and conversational preambles/postambles.
type JSONParser[T any] struct{}

// NewJSONParser creates a new JSON parser for the specified type.
func NewJSONParser[T any]() *JSONParser[T] {
	return &JSONParser[T]{}
}

// Parse extracts JSON from the raw LLM output and unmarshals it into type T.
func (p *JSONParser[T]) Parse(ctx context.Context, text string) (T, error) {
	var result T

	// Respect context cancellation
	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	// Clean the LLM output to extract just the JSON
	cleanedText := extractJSON(text)
	if cleanedText == "" {
		return result, errors.New("no valid JSON object or array found in LLM output")
	}

	// Unmarshal into the generic type
	if err := json.Unmarshal([]byte(cleanedText), &result); err != nil {
		return result, err
	}

	return result, nil
}

// extractJSON attempts to robustly isolate the JSON payload within an LLM response.
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// Strategy 1: Markdown Fences
	// First, try to find a specifically tagged ```json block
	jsonFenceIdx := strings.Index(text, "```json")
	if jsonFenceIdx == -1 {
		// Fallback to any code block
		jsonFenceIdx = strings.Index(text, "```")
	}

	if jsonFenceIdx != -1 {
		// Find the newline after the opening backticks
		newlineIdx := strings.Index(text[jsonFenceIdx:], "\n")
		if newlineIdx != -1 {
			contentStart := jsonFenceIdx + newlineIdx + 1

			// Look for the NEXT closing fence relative to contentStart
			remainder := text[contentStart:]
			if endIdx := strings.Index(remainder, "```"); endIdx != -1 {
				return strings.TrimSpace(remainder[:endIdx])
			}
		}
	}

	// Strategy 2: Balanced Brackets (Lexical Scanner)
	// Scan for the first { or[ and track balance to find the end.
	var firstOpenIdx = -1
	var closeChar byte
	stack := 0
	inString := false
	escape := false

	for i, char := range text {
		// Handle string literals and escapes
		if inString {
			if escape {
				escape = false
				continue
			}
			if char == '\\' {
				escape = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}

		switch char {
		case '"':
			inString = true
		case '{', '[':
			if stack == 0 {
				// Found the start of the JSON
				firstOpenIdx = i
				if char == '{' {
					closeChar = '}'
				} else {
					closeChar = ']'
				}
			}
			stack++
		case '}', ']':
			if stack > 0 {
				stack--
				// Only return if we've fully unwound the stack AND the closing
				// character matches the type of the opening character.
				if stack == 0 && char == rune(closeChar) {
					return text[firstOpenIdx : i+1]
				}
			}
		}
	}

	// Fallback: return raw text and let json.Unmarshal try its best
	return text
}
