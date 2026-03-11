package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/sevigo/goframe/llms"
	"github.com/sevigo/goframe/schema"
)

// VisualSelector uses vision-capable LLMs to find UI element selectors.
type VisualSelector struct {
	llm     llms.Model
	browser *Browser
}

// NewVisualSelector creates a visual element selector.
func NewVisualSelector(llm llms.Model, browser *Browser) *VisualSelector {
	return &VisualSelector{
		llm:     llm,
		browser: browser,
	}
}

// FindElement finds the best selector for a described element.
func (v *VisualSelector) FindElement(ctx context.Context, description string) (*ElementSelector, error) {
	screenshot, dom, err := v.browser.GetPageState(ctx)
	if err != nil {
		return nil, fmt.Errorf("visual selector: failed to get page state: %w", err)
	}

	return v.FindElementWithState(ctx, description, screenshot, dom)
}

// FindElementWithState finds an element using provided screenshot and DOM.
func (v *VisualSelector) FindElementWithState(ctx context.Context, description string, screenshot []byte, dom string) (*ElementSelector, error) {
	screenshotBase64 := base64.StdEncoding.EncodeToString(screenshot)

	prompt := fmt.Sprintf(`Find the selector for: %s

DOM structure:
%s

Return JSON with: primary_selector, fallback_selector (optional), coordinates (last resort), confidence, reason.`,
		description, dom)

	messages := []schema.MessageContent{
		schema.NewSystemMessage(ElementSelectionPrompt),
		schema.NewHumanMessageWithImage(prompt, screenshotBase64, "image/png"),
	}

	resp, err := v.llm.GenerateContent(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("visual selector: LLM failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("visual selector: no response from LLM")
	}

	content := resp.Choices[0].Content
	selector, err := parseSelectorResponse(content)
	if err != nil {
		return nil, fmt.Errorf("visual selector: failed to parse response: %w", err)
	}

	return selector, nil
}

func parseSelectorResponse(content string) (*ElementSelector, error) {
	jsonStart := indexOfJSON(content)
	if jsonStart == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonContent := extractJSON(content[jsonStart:])

	var selector ElementSelector
	if err := json.Unmarshal([]byte(jsonContent), &selector); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &selector, nil
}

func indexOfJSON(s string) int {
	for i := range len(s) {
		if s[i] == '{' {
			return i
		}
	}
	return -1
}

func extractJSON(s string) string {
	depth := 0
	for i := range len(s) {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}
