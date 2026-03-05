package voice

import (
	"testing"
)

func BenchmarkCalculateContextualPause(b *testing.B) {
	ds := &DialogueSynthesizer{
		PauseMsMin: 200,
		PauseMsMax: 300,
	}

	b.ResetTimer()
	for range b.N {
		ds.calculateContextualPause("Speaker", "What do you think?", "Speaker", "I think it's great.", 200, 300)
	}
}

func BenchmarkCalculatePerfectPause(b *testing.B) {
	ds := &DialogueSynthesizerCaptioned{
		TargetPauseMs: 250,
	}

	prev := &CaptionedSegment{
		Text:              "What do you think?",
		DurationMs:        1500,
		SpeechDurationMs:  1400,
		Timestamps:        []WordTimestamp{{Word: "test", StartMs: 0, EndMs: 100}},
		TrailingSilenceMs: 100,
	}

	curr := &CaptionedSegment{
		Text:             "I think it's great.",
		DurationMs:       1200,
		SpeechDurationMs: 1100,
		LeadingSilenceMs: 100,
	}

	b.ResetTimer()
	for range b.N {
		ds.CalculatePerfectPause(prev, curr)
	}
}

func BenchmarkGenerateSRT(b *testing.B) {
	segments := make([]CaptionedSegment, 100)
	for i := range segments {
		segments[i] = CaptionedSegment{
			Speaker:    "Speaker",
			Text:       "Test text",
			DurationMs: 1000 + i*10,
			Timestamps: []WordTimestamp{
				{Word: "Test", StartMs: 0, EndMs: 400},
				{Word: "text", StartMs: 450, EndMs: 900},
			},
		}
	}

	b.ResetTimer()
	for range b.N {
		generateSRT(segments)
	}
}

func BenchmarkApplyContextualPauseMultiplier(b *testing.B) {
	b.ResetTimer()
	for range b.N {
		applyContextualPauseMultiplier("What do you think?", "I think it's great.", "Speaker", "Speaker")
	}
}

func BenchmarkAnalyzeSpeechRate(b *testing.B) {
	segments := make([]CaptionedSegment, 10)
	for i := range segments {
		segments[i] = CaptionedSegment{
			Text: "Test text with multiple words for speech rate analysis",
			Timestamps: []WordTimestamp{
				{Word: "Test", StartMs: 0, EndMs: 200},
				{Word: "text", StartMs: 250, EndMs: 500},
				{Word: "with", StartMs: 550, EndMs: 700},
				{Word: "multiple", StartMs: 750, EndMs: 1100},
				{Word: "words", StartMs: 1150, EndMs: 1500},
			},
			SpeechDurationMs: 1500,
		}
	}

	b.ResetTimer()
	for range b.N {
		AnalyzeSpeechRate(segments)
	}
}
