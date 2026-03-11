package browser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseElementSelector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *ElementSelector
	}{
		{
			name: "full selector with all fields",
			input: `{
				"primary_selector": "#models-btn",
				"fallback_selector": "[aria-label='Open Models menu']",
				"coordinates": {"x": 360, "y": 166},
				"confidence": "high",
				"reason": "Unique ID matches visible button"
			}`,
			expected: &ElementSelector{
				PrimarySelector:  "#models-btn",
				FallbackSelector: "[aria-label='Open Models menu']",
				Coordinates:      &Coordinates{X: 360, Y: 166},
				Confidence:       ConfidenceHigh,
				Reason:           "Unique ID matches visible button",
			},
		},
		{
			name: "minimal selector",
			input: `{
				"primary_selector": "text=Submit",
				"confidence": "medium",
				"reason": "Button text visible in screenshot"
			}`,
			expected: &ElementSelector{
				PrimarySelector: "text=Submit",
				Confidence:      ConfidenceMedium,
				Reason:          "Button text visible in screenshot",
			},
		},
		{
			name: "coordinates only",
			input: `{
				"primary_selector": "",
				"coordinates": {"x": 100, "y": 200},
				"confidence": "low",
				"reason": "No unique selector found"
			}`,
			expected: &ElementSelector{
				Coordinates: &Coordinates{X: 100, Y: 200},
				Confidence:  ConfidenceLow,
				Reason:      "No unique selector found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseElementSelector(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.PrimarySelector, result.PrimarySelector)
			assert.Equal(t, tt.expected.FallbackSelector, result.FallbackSelector)
			assert.Equal(t, tt.expected.Confidence, result.Confidence)
			assert.Equal(t, tt.expected.Reason, result.Reason)
			if tt.expected.Coordinates != nil {
				require.NotNil(t, result.Coordinates)
				assert.Equal(t, tt.expected.Coordinates.X, result.Coordinates.X)
				assert.Equal(t, tt.expected.Coordinates.Y, result.Coordinates.Y)
			}
		})
	}
}

func TestElementSelector_ToJSON(t *testing.T) {
	selector := &ElementSelector{
		PrimarySelector:  "#models-btn",
		FallbackSelector: "[aria-label='Models']",
		Coordinates:      &Coordinates{X: 360, Y: 166},
		Confidence:       ConfidenceHigh,
		Reason:           "Unique ID",
	}

	json, err := selector.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, json, `"primary_selector": "#models-btn"`)
	assert.Contains(t, json, `"confidence": "high"`)
}

func TestParseSelectorResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{
			name:        "clean JSON",
			input:       `{"primary_selector": "#btn", "confidence": "high", "reason": "test"}`,
			shouldError: false,
		},
		{
			name: "JSON with text before",
			input: `Here is the result:
{"primary_selector": "#btn", "confidence": "high", "reason": "test"}`,
			shouldError: false,
		},
		{
			name: "JSON with text after",
			input: `{"primary_selector": "#btn", "confidence": "high", "reason": "test"}
This is my analysis.`,
			shouldError: false,
		},
		{
			name:        "no JSON",
			input:       "This is just text",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSelectorResponse(tt.input)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
