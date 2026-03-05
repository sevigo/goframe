package voice

import (
	"testing"
)

func TestSameSpeakerOptimization(t *testing.T) {
	tests := []struct {
		name        string
		prevSpeaker string
		prevText    string
		currSpeaker string
		currText    string
		wantMultMin float64
		wantMultMax float64
	}{
		{
			name:        "same_speaker_continuation",
			prevSpeaker: "Alice",
			prevText:    "And then we went,",
			currSpeaker: "Alice",
			currText:    "to the station.",
			wantMultMin: PauseMultSameSpeaker * 0.9, // Allow 10% variance
			wantMultMax: PauseMultSameSpeaker * 1.1,
		},
		{
			name:        "different_speakers_normal_pause",
			prevSpeaker: "Alice",
			prevText:    "What do you think?",
			currSpeaker: "Bob",
			currText:    "I think it's great.",
			wantMultMin: PauseMultQuestion * 0.9,
			wantMultMax: PauseMultQuestion * 1.1,
		},
		{
			name:        "same_speaker_no_punctuation",
			prevSpeaker: "Alice",
			prevText:    "So what happened",
			currSpeaker: "Alice",
			currText:    "well let me explain",
			wantMultMin: PauseMultSameSpeaker * 0.9,
			wantMultMax: PauseMultSameSpeaker * 1.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mult := applyContextualPauseMultiplier(tt.prevText, tt.currText, tt.prevSpeaker, tt.currSpeaker)

			if mult < tt.wantMultMin || mult > tt.wantMultMax {
				t.Errorf("same-speaker optimization failed: got mult=%.2f, want range [%.2f, %.2f]",
					mult, tt.wantMultMin, tt.wantMultMax)
			}
		})
	}
}

func TestNegativePauseForInterruptions(t *testing.T) {
	ds := &DialogueSynthesizer{
		PauseMsMin: 200,
		PauseMsMax: 300,
	}

	// Test same speaker (should produce very short pause ~50ms with 0.25x multiplier)
	pause := ds.calculateContextualPause("Alice", "I was thinking", "Alice", "about that", 200, 300)

	// Same speaker should have very short pause (minPause * 0.25 + random)
	if pause > 150 {
		t.Errorf("same speaker pause too long: got %dms, expected <= 150ms (min * 0.25 multiplier)", pause)
	}
}
