package voice

import (
	"testing"
)

func TestCalculateContextualPause(t *testing.T) {
	ds := &DialogueSynthesizer{
		PauseMsMin: 200,
		PauseMsMax: 300,
	}

	tests := []struct {
		name     string
		prevText string
		currText string
		wantMin  int // minimum expected pause
		wantMax  int // maximum expected pause
	}{
		{
			name:     "question before response",
			prevText: "What do you think about Tokyo?",
			currText: "I think it's amazing.",
			wantMin:  250, // questions get 1.3x multiplier
			wantMax:  450,
		},
		{
			name:     "exclamation",
			prevText: "That's incredible!",
			currText: "I know right?",
			wantMin:  220, // exclamations get 1.2x
			wantMax:  400,
		},
		{
			name:     "continuing thought",
			prevText: "And then we went to the station,",
			currText: "and the train was already there.",
			wantMin:  100, // commas get 0.7x
			wantMax:  280, // allow for randomness
		},
		{
			name:     "short response",
			prevText: "That was amazing.",
			currText: "Yeah.", // 1 word
			wantMin:  100,     // short responses get 0.6x
			wantMax:  300,     // allow for randomness and combined factors
		},
		{
			name:     "emotional reaction",
			prevText: "The sushi was incredible.",
			currText: "Wow, really?",
			wantMin:  240, // "wow" adds 1.2x
			wantMax:  450,
		},
		{
			name:     "long sentence",
			prevText: "That's interesting.",
			currText: "So when you go to Tokyo and you see all these amazing places and you try the food and you meet the people it's just overwhelming.",
			wantMin:  250, // long sentences (>20 words) get 1.2x
			wantMax:  450,
		},
		{
			name:     "conversational transition",
			prevText: "So that's my story.",
			currText: "Well, let's move on to the next topic.",
			wantMin:  240, // "well" adds slight pause
			wantMax:  400,
		},
		{
			name:     "normal statement",
			prevText: "The train arrives at 5.",
			currText: "We should hurry then.",
			wantMin:  200, // baseline
			wantMax:  350,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pause := ds.calculateContextualPause("Speaker", tt.prevText, "Speaker", tt.currText, ds.PauseMsMin, ds.PauseMsMax)

			if pause < tt.wantMin {
				t.Errorf("pause too short: got %dms, want at least %dms for %q -> %q",
					pause, tt.wantMin, tt.prevText, tt.currText)
			}

			if pause > tt.wantMax {
				t.Errorf("pause too long: got %dms, want at most %dms for %q -> %q",
					pause, tt.wantMax, tt.prevText, tt.currText)
			}
		})
	}
}

func TestEndsWith(t *testing.T) {
	tests := []struct {
		text   string
		suffix string
		want   bool
	}{
		{"Hello!", "!", true},
		{"What?", "?", true},
		{"Wait...", "...", true},
		{"Hmm—", "—", true},
		{"Then,", ",", true},
		{"Hello! ", "!", true}, // trims whitespace
		{"Hello", "!", false},
		{"Hello.", "!", false},
	}

	for _, tt := range tests {
		got := endsWith(tt.text, tt.suffix)
		if got != tt.want {
			t.Errorf("endsWith(%q, %q) = %v, want %v", tt.text, tt.suffix, got, tt.want)
		}
	}
}

func TestStartsWith(t *testing.T) {
	tests := []struct {
		text   string
		prefix string
		want   bool
	}{
		{"Wait, what?", "wait", true},
		{"But I thought", "but", true},
		{"So, let me explain", "so,", true},
		{"Well, that's odd", "well,", true},
		{"Ha! That's funny", "ha", true},
		{"Oh my god", "oh", true},
		{"WOW!", "wow", true},
		{"  Wait  ", "wait", true}, // trims whitespace
		{"Hello", "wait", false},
		{"So...", "but", false},
	}

	for _, tt := range tests {
		got := startsWith(tt.text, tt.prefix)
		if got != tt.want {
			t.Errorf("startsWith(%q, %q) = %v, want %v", tt.text, tt.prefix, got, tt.want)
		}
	}
}
