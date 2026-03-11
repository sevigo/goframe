package browser

import (
	"encoding/json"
	"fmt"
)

// ConfidenceLevel indicates how reliable the selector is.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// Coordinates represents element position.
type Coordinates struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ElementSelector contains the best selector for a UI element.
type ElementSelector struct {
	PrimarySelector  string          `json:"primary_selector"`
	FallbackSelector string          `json:"fallback_selector,omitempty"`
	Coordinates      *Coordinates    `json:"coordinates,omitempty"`
	Confidence       ConfidenceLevel `json:"confidence"`
	Reason           string          `json:"reason"`
}

// ToJSON returns the selector as formatted JSON.
func (e *ElementSelector) ToJSON() (string, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return "", fmt.Errorf("browser: failed to marshal selector: %w", err)
	}
	return string(data), nil
}

// ParseElementSelector parses JSON into ElementSelector.
func ParseElementSelector(data string) (*ElementSelector, error) {
	var selector ElementSelector
	if err := json.Unmarshal([]byte(data), &selector); err != nil {
		return nil, fmt.Errorf("browser: failed to parse selector: %w", err)
	}
	return &selector, nil
}

// ElementSelectionPrompt is the system prompt for the element selection task.
const ElementSelectionPrompt = `You are a UI automation assistant. You receive:
1. A screenshot of the current page
2. A simplified DOM tree with element positions

Your task: Find the BEST selector for requested elements.

## Selector Priority (use this order)
1. **data-testid** attributes (most reliable)
   Example: [data-testid="models-btn"]
   
2. **id** attributes (unique)
   Example: #models-menu-button
   
3. **aria-label** attributes
   Example: [aria-label="Open models menu"]
   
4. **Combination selectors** (tag + class + text)
   Example: button.nav-item:has-text("Models")
   
5. **text content** (for buttons/links)
   Example: text=Models

6. **Coordinates** (LAST RESORT only)
   Use only when no unique selector exists

## Rules
- ALWAYS cross-reference: What you see in screenshot <-> DOM elements with matching coordinates
- Prefer selectors that won't break if layout changes
- If multiple elements match, use the one with visible text matching the request
- Include fallback selectors when primary might be fragile
- Provide coordinates as absolute last resort
- Always set confidence to "high", "medium", or "low"

## Output Format
Return JSON with:
{
  "primary_selector": "Best CSS selector",
  "fallback_selector": "Alternative selector (optional)",
  "coordinates": {"x": 123, "y": 456},
  "confidence": "high/medium/low",
  "reason": "Brief explanation"
}`
