package elevenlabs

import (
	"errors"
	"testing"

	"github.com/sevigo/goframe/voice"
)

func TestConvertAlignmentsToWords(t *testing.T) {
	tests := []struct {
		name       string
		alignments []*alignment
		text       string
		wantWords  []string
		wantCount  int
	}{
		{
			name: "simple sentence",
			alignments: []*alignment{
				{
					Chars:            []string{"H", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d"},
					CharStartTimesMs: []int{0, 50, 100, 150, 200, 250, 300, 350, 400, 450, 500},
					CharDurationsMs:  []int{50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50},
				},
			},
			text:      "Hello world",
			wantWords: []string{"Hello", "world"},
			wantCount: 2,
		},
		{
			name: "punctuation",
			alignments: []*alignment{
				{
					Chars:            []string{"H", "i", "!", " ", "B", "y", "e", "."},
					CharStartTimesMs: []int{0, 50, 100, 150, 200, 250, 300, 350},
					CharDurationsMs:  []int{50, 50, 50, 50, 50, 50, 50, 50},
				},
			},
			text:      "Hi! Bye.",
			wantWords: []string{"Hi", "Bye"},
			wantCount: 2,
		},
		{
			name: "empty alignment",
			alignments: []*alignment{
				{
					Chars:            []string{},
					CharStartTimesMs: []int{},
					CharDurationsMs:  []int{},
				},
			},
			text:      "",
			wantWords: nil,
			wantCount: 0,
		},
		{
			name:       "nil alignment",
			alignments: nil,
			text:       "test",
			wantWords:  nil,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamps := convertAlignmentsToWords(tt.alignments, tt.text)

			if len(timestamps) != tt.wantCount {
				t.Errorf("got %d timestamps, want %d", len(timestamps), tt.wantCount)
				return
			}

			for i, ts := range timestamps {
				if i < len(tt.wantWords) && ts.Word != tt.wantWords[i] {
					t.Errorf("timestamp[%d].Word = %q, want %q", i, ts.Word, tt.wantWords[i])
				}
				if ts.StartMs >= ts.EndMs {
					t.Errorf("timestamp[%d]: start (%d) >= end (%d)", i, ts.StartMs, ts.EndMs)
				}
			}
		})
	}
}

func TestIsWordCharacter(t *testing.T) {
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
			if got := isWordCharacter(tt.char); got != tt.want {
				t.Errorf("isWordCharacter(%q) = %v, want %v", tt.char, got, tt.want)
			}
		})
	}
}

func TestTimestampOrdering(t *testing.T) {
	alignments := []*alignment{
		{
			Chars:            []string{"T", "h", "e", " ", "q", "u", "i", "c", "k", " ", "f", "o", "x"},
			CharStartTimesMs: []int{0, 30, 60, 90, 120, 150, 180, 210, 240, 270, 300, 330, 360},
			CharDurationsMs:  []int{30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
		},
	}

	timestamps := convertAlignmentsToWords(alignments, "The quick fox")

	for i := 1; i < len(timestamps); i++ {
		if timestamps[i].StartMs < timestamps[i-1].EndMs {
			t.Errorf("word %d overlaps previous: prev end %d, curr start %d",
				i, timestamps[i-1].EndMs, timestamps[i].StartMs)
		}
	}

	for i, ts := range timestamps {
		if ts.StartMs >= ts.EndMs {
			t.Errorf("timestamp[%d]: start (%d) >= end (%d)", i, ts.StartMs, ts.EndMs)
		}
	}
}

func TestNewSynthesizerValidation(t *testing.T) {
	_, err := NewSynthesizer()
	if !errors.Is(err, ErrAPIKeyRequired) {
		t.Errorf("NewSynthesizer() without API key: got %v, want %v", err, ErrAPIKeyRequired)
	}

	_, err = NewSynthesizer(WithAPIKey("test-key"))
	if !errors.Is(err, ErrVoiceIDRequired) {
		t.Errorf("NewSynthesizer() without voice ID: got %v, want %v", err, ErrVoiceIDRequired)
	}

	syn, err := NewSynthesizer(WithAPIKey("test-key"), WithVoiceID("voice-123"))
	if err != nil {
		t.Errorf("NewSynthesizer() with valid params: got error %v", err)
	}
	if syn == nil {
		t.Error("NewSynthesizer() returned nil")
	}
}

func TestWithModelID(t *testing.T) {
	syn, _ := NewSynthesizer(WithAPIKey("test"), WithVoiceID("voice-123"), WithModelID("eleven_monolingual_v1"))
	if syn.modelID != "eleven_monolingual_v1" {
		t.Errorf("modelID = %q, want 'eleven_monolingual_v1'", syn.modelID)
	}
}

func TestWithFormat(t *testing.T) {
	syn, _ := NewSynthesizer(WithAPIKey("test"), WithVoiceID("voice-123"), WithFormat("wav"))
	if syn.format != "wav" {
		t.Errorf("format = %q, want 'wav'", syn.format)
	}
}

func TestVoiceSettings(t *testing.T) {
	syn, _ := NewSynthesizer(
		WithAPIKey("test"),
		WithVoiceID("voice-123"),
		WithStability(0.8),
		WithSimilarityBoost(0.9),
	)
	if syn.stability != 0.8 {
		t.Errorf("stability = %f, want 0.8", syn.stability)
	}
	if syn.similarityBoost != 0.9 {
		t.Errorf("similarityBoost = %f, want 0.9", syn.similarityBoost)
	}
}

func TestBuildOptions(t *testing.T) {
	syn, _ := NewSynthesizer(
		WithAPIKey("test"),
		WithVoiceID("voice-123"),
		WithModelID("custom-model"),
	)

	opts := syn.buildOptions([]voice.Option{voice.WithFormat("mp3_44100_192")})

	if opts.Model != "custom-model" {
		t.Errorf("Model = %q, want 'custom-model'", opts.Model)
	}
	if opts.Format != "mp3_44100_192" {
		t.Errorf("Format = %q, want 'mp3_44100_192'", opts.Format)
	}
}
