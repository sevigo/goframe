package elevenlabs

import (
	"testing"

	"github.com/sevigo/goframe/voice"
)

func TestExtractWordTimestamps(t *testing.T) {
	tests := []struct {
		name      string
		alignment *characterAlignment
		startIdx  int
		endIdx    int
		wantCount int
		wantWords []string
	}{
		{
			name: "simple sentence",
			alignment: &characterAlignment{
				Characters:                 []string{"H", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d"},
				CharacterStartTimesSeconds: []float64{0.0, 0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35, 0.40, 0.45, 0.50},
				CharacterEndTimesSeconds:   []float64{0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35, 0.40, 0.45, 0.50, 0.55},
			},
			startIdx:  0,
			endIdx:    11,
			wantCount: 2,
			wantWords: []string{"Hello", "world"},
		},
		{
			name: "single word",
			alignment: &characterAlignment{
				Characters:                 []string{"H", "i"},
				CharacterStartTimesSeconds: []float64{0.0, 0.05},
				CharacterEndTimesSeconds:   []float64{0.05, 0.10},
			},
			startIdx:  0,
			endIdx:    2,
			wantCount: 1,
			wantWords: []string{"Hi"},
		},
		{
			name: "with punctuation",
			alignment: &characterAlignment{
				Characters:                 []string{"H", "i", "!", " ", "B", "y", "e", "."},
				CharacterStartTimesSeconds: []float64{0.0, 0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35},
				CharacterEndTimesSeconds:   []float64{0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.35, 0.40},
			},
			startIdx:  0,
			endIdx:    8,
			wantCount: 2,
			wantWords: []string{"Hi", "Bye"},
		},
		{
			name:      "nil alignment",
			alignment: nil,
			startIdx:  0,
			endIdx:    5,
			wantCount: 0,
		},
		{
			name: "empty range",
			alignment: &characterAlignment{
				Characters:                 []string{},
				CharacterStartTimesSeconds: []float64{},
				CharacterEndTimesSeconds:   []float64{},
			},
			startIdx:  0,
			endIdx:    0,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamps := extractWordTimestamps(tt.alignment, tt.startIdx, tt.endIdx)

			if len(timestamps) != tt.wantCount {
				t.Errorf("got %d timestamps, want %d", len(timestamps), tt.wantCount)
				return
			}

			for i, ts := range timestamps {
				if ts.StartMs >= ts.EndMs {
					t.Errorf("timestamp[%d]: start (%d) >= end (%d)", i, ts.StartMs, ts.EndMs)
				}
				if i < len(tt.wantWords) && ts.Word != tt.wantWords[i] {
					t.Errorf("timestamp[%d].Word = %q, want %q", i, ts.Word, tt.wantWords[i])
				}
			}
		})
	}
}

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		char string
		want bool
	}{
		{"a", true},
		{"Z", true},
		{"0", true},
		{"9", true},
		{"'", true},
		{"-", true},
		{" ", false},
		{"!", false},
		{".", false},
		{",", false},
	}

	for _, tt := range tests {
		t.Run(tt.char, func(t *testing.T) {
			if got := isWordChar(tt.char); got != tt.want {
				t.Errorf("isWordChar(%q) = %v, want %v", tt.char, got, tt.want)
			}
		})
	}
}

func TestGenerateSubtitles(t *testing.T) {
	syn, _ := NewSynthesizer(WithAPIKey("test"), WithVoiceID("voice-123"))

	segments := []DialogueSegmentResult{
		{
			Speaker: "Alice",
			Text:    "Hello",
			StartMs: 0,
			EndMs:   500,
			WordTimestamps: []voice.WordTimestamp{
				{Word: "Hello", StartMs: 0, EndMs: 500},
			},
		},
		{
			Speaker: "Bob",
			Text:    "Hi there",
			StartMs: 600,
			EndMs:   1200,
			WordTimestamps: []voice.WordTimestamp{
				{Word: "Hi", StartMs: 600, EndMs: 800},
				{Word: "there", StartMs: 810, EndMs: 1200},
			},
		},
	}

	subtitles := syn.generateSubtitles(segments)

	if subtitles == "" {
		t.Error("generateSubtitles returned empty string")
	}

	if len(subtitles) < 50 {
		t.Errorf("subtitles too short: %d bytes", len(subtitles))
	}
}

func TestGenerateSubtitlesEmpty(t *testing.T) {
	syn, _ := NewSynthesizer(WithAPIKey("test"), WithVoiceID("voice-123"))

	subtitles := syn.generateSubtitles(nil)
	if subtitles != "" {
		t.Errorf("generateSubtitles with nil should return empty, got %q", subtitles)
	}

	subtitles = syn.generateSubtitles([]DialogueSegmentResult{})
	if subtitles != "" {
		t.Errorf("generateSubtitles with empty slice should return empty, got %q", subtitles)
	}
}

func TestSynthesizeDialogueValidation(t *testing.T) {
	syn, _ := NewSynthesizer(WithAPIKey("test"), WithVoiceID("voice-123"))

	_, err := syn.SynthesizeDialogue(nil, nil)
	if err == nil {
		t.Error("SynthesizeDialogue with nil should return error")
	}

	_, err = syn.SynthesizeDialogue(nil, []voice.DialogueSegment{})
	if err == nil {
		t.Error("SynthesizeDialogue with empty segments should return error")
	}
}
