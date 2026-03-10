package voice

import (
	"math"
	"testing"
)

func TestTokenizeWords(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "simple sentence",
			text:     "Hello world!",
			expected: []string{"Hello", "world"},
		},
		{
			name:     "with punctuation",
			text:     "What's up? I'm fine.",
			expected: []string{"What's", "up", "I'm", "fine"},
		},
		{
			name:     "hyphenated word",
			text:     "state-of-the-art design",
			expected: []string{"state-of-the-art", "design"},
		},
		{
			name:     "numbers",
			text:     "The year 2024 is great",
			expected: []string{"The", "year", "2024", "is", "great"},
		},
		{
			name:     "empty string",
			text:     "",
			expected: []string(nil),
		},
		{
			name:     "only punctuation",
			text:     "!@#$%",
			expected: []string(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizeWords(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("tokenizeWords(%q) = %v, want %v", tt.text, result, tt.expected)
				return
			}
			for i, w := range result {
				if w != tt.expected[i] {
					t.Errorf("word[%d] = %q, want %q", i, w, tt.expected[i])
				}
			}
		})
	}
}

func TestCalculateWordWeight(t *testing.T) {
	tests := []struct {
		name     string
		word     string
		minScore int
	}{
		{"short word", "Hi", 2},
		{"medium word", "Hello", 5},
		{"long word", "extraordinary", 13},
		{"with exclamation", "Wow!", 6},
		{"capitalized", "Hello", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weight := calculateWordWeight(tt.word)
			if weight < tt.minScore {
				t.Errorf("calculateWordWeight(%q) = %d, want >= %d", tt.word, weight, tt.minScore)
			}
		})
	}
}

func TestCountVowels(t *testing.T) {
	tests := []struct {
		word     string
		expected int
	}{
		{"hello", 2},
		{"beautiful", 5},
		{"sky", 0},
		{"AEIOU", 5},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := countVowels(tt.word)
			if result != tt.expected {
				t.Errorf("countVowels(%q) = %d, want %d", tt.word, result, tt.expected)
			}
		})
	}
}

func TestEstimateWordTimestamps(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		durationMs       int
		leadingSilenceMs int
		wantWordCount    int
	}{
		{
			name:             "simple sentence",
			text:             "Hello world",
			durationMs:       2000,
			leadingSilenceMs: 100,
			wantWordCount:    2,
		},
		{
			name:             "longer text",
			text:             "The quick brown fox jumps over the lazy dog",
			durationMs:       5000,
			leadingSilenceMs: 200,
			wantWordCount:    9,
		},
		{
			name:             "empty text",
			text:             "",
			durationMs:       1000,
			leadingSilenceMs: 0,
			wantWordCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamps := EstimateWordTimestamps(tt.text, tt.durationMs, tt.leadingSilenceMs)

			if len(timestamps) != tt.wantWordCount {
				t.Errorf("got %d timestamps, want %d", len(timestamps), tt.wantWordCount)
				return
			}

			for i, ts := range timestamps {
				if ts.StartMs >= ts.EndMs {
					t.Errorf("timestamp[%d]: start (%d) >= end (%d)", i, ts.StartMs, ts.EndMs)
				}
				if ts.Word == "" {
					t.Errorf("timestamp[%d]: empty word", i)
				}
				if i > 0 && timestamps[i-1].EndMs > ts.StartMs {
					t.Errorf("timestamp[%d]: overlaps with previous", i)
				}
			}

			if len(timestamps) > 0 {
				lastTS := timestamps[len(timestamps)-1]
				totalDuration := tt.durationMs
				expectedEnd := lastTS.EndMs
				if math.Abs(float64(expectedEnd-totalDuration)) > 100 {
					t.Errorf("last word ends at %dms, total duration is %dms", expectedEnd, totalDuration)
				}
			}
		})
	}
}

func TestTimestampOrdering(t *testing.T) {
	text := "One two three four five"
	durationMs := 3000
	leadingSilenceMs := 100

	timestamps := EstimateWordTimestamps(text, durationMs, leadingSilenceMs)

	if len(timestamps) != 5 {
		t.Fatalf("got %d timestamps, want 5", len(timestamps))
	}

	for i := 1; i < len(timestamps); i++ {
		if timestamps[i].StartMs < timestamps[i-1].EndMs {
			t.Errorf("word %d starts at %dms, but word %d ends at %dms (overlap)",
				i, timestamps[i].StartMs, i-1, timestamps[i-1].EndMs)
		}
	}

	lastTS := timestamps[len(timestamps)-1]
	expectedEnd := leadingSilenceMs + (durationMs - leadingSilenceMs)
	if lastTS.EndMs != expectedEnd {
		t.Errorf("last word ends at %dms, expected %dms", lastTS.EndMs, expectedEnd)
	}
}

func TestWordDistribution(t *testing.T) {
	text := "A very long word compared to short ones"
	durationMs := 2000
	leadingSilenceMs := 0

	timestamps := EstimateWordTimestamps(text, durationMs, leadingSilenceMs)

	if len(timestamps) < 2 {
		t.Fatal("need at least 2 words to test distribution")
	}

	shortWordDuration := 0
	longWordDuration := 0
	for _, ts := range timestamps {
		dur := ts.EndMs - ts.StartMs
		if ts.Word == "A" {
			shortWordDuration = dur
		}
		if ts.Word == "compared" {
			longWordDuration = dur
		}
	}

	if shortWordDuration == 0 || longWordDuration == 0 {
		t.Skip("could not find test words")
	}

	if longWordDuration <= shortWordDuration {
		t.Errorf("longer word duration (%dms) should be greater than short word duration (%dms)",
			longWordDuration, shortWordDuration)
	}
}
