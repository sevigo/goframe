package voice

import (
	"context"
	"io"
	"testing"
)

func TestApplyContextualPauseMultiplier(t *testing.T) {
	tests := []struct {
		name     string
		prevText string
		currText string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "question before response",
			prevText: "What do you think?",
			currText: "I think it's great.",
			wantMin:  PauseMultQuestion * PauseMultMin / PauseMultMin, // 1.3
			wantMax:  1.3,
		},
		{
			name:     "exclamation",
			prevText: "That's amazing!",
			currText: "Right?",
			wantMin:  PauseMultExclamation * PauseMultShortResp,
			wantMax:  1.2 * 0.6,
		},
		{
			name:     "continuing with comma",
			prevText: "And then,",
			currText: "we went to the station.",
			wantMin:  PauseMultComma,
			wantMax:  0.7,
		},
		{
			name:     "short response",
			prevText: "That was fun.",
			currText: "Yeah.",
			wantMin:  PauseMultShortResp,
			wantMax:  0.6,
		},
		{
			name:     "long sentence after",
			prevText: "Good point.",
			currText: "So when you go to Tokyo and you see all these amazing places and you try the food and you meet the people it's just overwhelming and incredible.",
			wantMin:  PauseMultLongSentence,
			wantMax:  1.2,
		},
		{
			name:     "wait response",
			prevText: "I think so.",
			currText: "Wait, really?",
			wantMin:  PauseMultWait * PauseMultShortResp,
			wantMax:  1.3 * 0.6,
		},
		{
			name:     "emotional reaction",
			prevText: "It was incredible.",
			currText: "Wow, that's amazing!",
			wantMin:  PauseMultEmotional * PauseMultShortResp,
			wantMax:  1.2 * 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multiplier := applyContextualPauseMultiplier(tt.prevText, tt.currText)

			if multiplier < tt.wantMin*0.9 || multiplier > tt.wantMax*1.1 {
				t.Errorf("multiplier = %.2f, want range [%.2f, %.2f] for %q -> %q",
					multiplier, tt.wantMin, tt.wantMax, tt.prevText, tt.currText)
			}
		})
	}
}

func TestCalculatePerfectPause(t *testing.T) {
	ds := &DialogueSynthesizerCaptioned{
		TargetPauseMs: 250,
	}

	tests := []struct {
		name              string
		prev              *CaptionedSegment
		curr              *CaptionedSegment
		wantPauseMin      int
		wantPauseMax      int
		wantNoDoublePause bool
	}{
		{
			name: "question without built-in silence",
			prev: &CaptionedSegment{
				Text:             "What do you think?",
				DurationMs:       1500,
				SpeechDurationMs: 1400,
				Timestamps: []WordTimestamp{
					{Word: "What", StartMs: 0, EndMs: 300},
					{Word: "do", StartMs: 320, EndMs: 450},
					{Word: "you", StartMs: 470, EndMs: 620},
					{Word: "think", StartMs: 640, EndMs: 1400},
				},
				TrailingSilenceMs: 100,
			},
			curr: &CaptionedSegment{
				Text:             "I think it's great.",
				DurationMs:       1200,
				SpeechDurationMs: 1100,
				Timestamps: []WordTimestamp{
					{Word: "I", StartMs: 0, EndMs: 100},
					{Word: "think", StartMs: 120, EndMs: 450},
					{Word: "it's", StartMs: 470, EndMs: 650},
					{Word: "great", StartMs: 670, EndMs: 1100},
				},
				LeadingSilenceMs: 100,
			},
			wantPauseMin:      200, // target 250 * 1.3 (question) - 200 (built-in)
			wantPauseMax:      400,
			wantNoDoublePause: true,
		},
		{
			name: "enough built-in silence",
			prev: &CaptionedSegment{
				Text:             "Hello.",
				DurationMs:       1000,
				SpeechDurationMs: 500,
				Timestamps: []WordTimestamp{
					{Word: "Hello", StartMs: 0, EndMs: 500},
				},
				TrailingSilenceMs: 500, // Already 500ms silence
			},
			curr: &CaptionedSegment{
				Text:             "Hi.",
				DurationMs:       800,
				SpeechDurationMs: 300,
				Timestamps: []WordTimestamp{
					{Word: "Hi", StartMs: 0, EndMs: 300},
				},
				LeadingSilenceMs: 500, // Another 500ms
			},
			wantPauseMin:      0, // Already has 1000ms, more than enough
			wantPauseMax:      0,
			wantNoDoublePause: true,
		},
		{
			name: "short utterance after",
			prev: &CaptionedSegment{
				Text:             "And?",
				DurationMs:       600,
				SpeechDurationMs: 500, // Short
				Timestamps: []WordTimestamp{
					{Word: "And", StartMs: 0, EndMs: 500},
				},
				TrailingSilenceMs: 100,
			},
			curr: &CaptionedSegment{
				Text:             "Yeah.",
				DurationMs:       400,
				SpeechDurationMs: 300,
				Timestamps: []WordTimestamp{
					{Word: "Yeah", StartMs: 0, EndMs: 300},
				},
				LeadingSilenceMs: 100,
			},
			wantPauseMin:      50,  // Reduced pause for short response
			wantPauseMax:      200, // Short response + short prev
			wantNoDoublePause: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pause := ds.CalculatePerfectPause(tt.prev, tt.curr)

			if pause < tt.wantPauseMin || pause > tt.wantPauseMax {
				t.Errorf("pause = %dms, want range [%d, %d]ms", pause, tt.wantPauseMin, tt.wantPauseMax)
			}

			if tt.wantNoDoublePause {
				builtIn := tt.prev.TrailingSilenceMs + tt.curr.LeadingSilenceMs
				targetWithMultiplier := int(float64(ds.TargetPauseMs) * PauseMultQuestion)
				if targetWithMultiplier > 0 && pause > 0 {
					totalPause := builtIn + pause
					if totalPause > targetWithMultiplier*2 {
						t.Errorf("possible double-pause: built-in=%dms + added=%dms = %dms, target~%dms",
							builtIn, pause, totalPause, targetWithMultiplier)
					}
				}
			}
		})
	}
}

func TestGenerateSRT(t *testing.T) {
	segments := []CaptionedSegment{
		{
			Speaker: "Alice",
			Text:    "Hello",
			Timestamps: []WordTimestamp{
				{Word: "Hello", StartMs: 0, EndMs: 500},
			},
			DurationMs: 800,
		},
		{
			Speaker: "Bob",
			Text:    "Hi there",
			Timestamps: []WordTimestamp{
				{Word: "Hi", StartMs: 0, EndMs: 200},
				{Word: "there", StartMs: 250, EndMs: 600},
			},
			DurationMs: 1000,
		},
	}

	srt := generateSRT(segments)

	expected := `1
00:00:00,000 --> 00:00:00,500
Hello

2
00:00:00,800 --> 00:00:01,000
Hi

3
00:00:01,050 --> 00:00:01,400
there

`

	if srt != expected {
		t.Errorf("SRT mismatch:\nGot:\n%s\nWant:\n%s", srt, expected)
	}
}

func TestFormatSRTTime(t *testing.T) {
	tests := []struct {
		ms       int
		expected string
	}{
		{0, "00:00:00,000"},
		{500, "00:00:00,500"},
		{1500, "00:00:01,500"},
		{65000, "00:01:05,000"},
		{3661500, "01:01:01,500"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSRTTime(tt.ms)
			if result != tt.expected {
				t.Errorf("formatSRTTime(%d) = %s, want %s", tt.ms, result, tt.expected)
			}
		})
	}
}

func TestAnalyzeSpeechRate(t *testing.T) {
	tests := []struct {
		name      string
		segments  []CaptionedSegment
		wantWPM   float64
		tolerance float64
	}{
		{
			name: "normal speech rate",
			segments: []CaptionedSegment{
				{
					Text: "Hello world this is a test",
					Timestamps: []WordTimestamp{
						{Word: "Hello", StartMs: 0, EndMs: 300},
						{Word: "world", StartMs: 350, EndMs: 650},
						{Word: "this", StartMs: 700, EndMs: 900},
						{Word: "is", StartMs: 950, EndMs: 1100},
						{Word: "a", StartMs: 1150, EndMs: 1200},
						{Word: "test", StartMs: 1250, EndMs: 1600},
					},
					SpeechDurationMs: 1600,
				},
			},
			wantWPM:   225.0, // 6 words in 1.6 seconds = 225 WPM
			tolerance: 10.0,
		},
		{
			name: "fast speech",
			segments: []CaptionedSegment{
				{
					Text: "Quick brown fox jumps",
					Timestamps: []WordTimestamp{
						{Word: "Quick", StartMs: 0, EndMs: 150},
						{Word: "brown", StartMs: 160, EndMs: 300},
						{Word: "fox", StartMs: 310, EndMs: 420},
						{Word: "jumps", StartMs: 430, EndMs: 600},
					},
					SpeechDurationMs: 600,
				},
			},
			wantWPM:   400.0, // 4 words in 0.6 seconds = 400 WPM
			tolerance: 20.0,
		},
		{
			name:      "empty segments",
			segments:  []CaptionedSegment{},
			wantWPM:   0,
			tolerance: 0,
		},
		{
			name: "multiple segments",
			segments: []CaptionedSegment{
				{
					Text: "First segment with four words",
					Timestamps: []WordTimestamp{
						{Word: "First", StartMs: 0, EndMs: 400},
						{Word: "segment", StartMs: 450, EndMs: 900},
						{Word: "with", StartMs: 950, EndMs: 1100},
						{Word: "four", StartMs: 1150, EndMs: 1350},
						{Word: "words", StartMs: 1400, EndMs: 1800},
					},
					SpeechDurationMs: 1800,
				},
				{
					Text: "Second segment with three words",
					Timestamps: []WordTimestamp{
						{Word: "Second", StartMs: 0, EndMs: 350},
						{Word: "segment", StartMs: 400, EndMs: 850},
						{Word: "with", StartMs: 900, EndMs: 1050},
						{Word: "three", StartMs: 1100, EndMs: 1350},
						{Word: "words", StartMs: 1400, EndMs: 1700},
					},
					SpeechDurationMs: 1700,
				},
			},
			wantWPM:   163.0, // 10 words in 3.5 seconds = ~171 WPM
			tolerance: 15.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wpm := AnalyzeSpeechRate(tt.segments)

			if tt.wantWPM == 0 {
				if wpm != 0 {
					t.Errorf("AnalyzeSpeechRate() = %.2f, want 0", wpm)
				}
				return
			}

			diff := wpm - tt.wantWPM
			if diff < -tt.tolerance || diff > tt.tolerance {
				t.Errorf("AnalyzeSpeechRate() = %.2f, want %.2f (±%.2f)", wpm, tt.wantWPM, tt.tolerance)
			}
		})
	}
}

func TestNewDialogueSynthesizerCaptionedValidation(t *testing.T) {
	synth := &mockCaptionedSynthesizer{}

	tests := []struct {
		name      string
		synth     CaptionedSynthesizer
		voiceMap  map[string]string
		wantError bool
	}{
		{
			name:      "nil synthesizer",
			synth:     nil,
			voiceMap:  map[string]string{"Alice": "af_bella"},
			wantError: true,
		},
		{
			name:      "empty voice map",
			synth:     synth,
			voiceMap:  map[string]string{},
			wantError: true,
		},
		{
			name:      "valid parameters",
			synth:     synth,
			voiceMap:  map[string]string{"Alice": "af_bella", "Bob": "am_adam"},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, err := NewDialogueSynthesizerCaptioned(tt.synth, tt.voiceMap)

			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if ds != nil {
					t.Error("expected nil DialogueSynthesizerCaptioned on error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if ds == nil {
					t.Error("expected non-nil DialogueSynthesizerCaptioned")
				}
			}
		})
	}
}

type mockCaptionedSynthesizer struct{}

func (m *mockCaptionedSynthesizer) Synthesize(ctx context.Context, text string, opts ...Option) (*Audio, error) {
	return &Audio{Data: []byte{}, Format: "wav"}, nil
}

func (m *mockCaptionedSynthesizer) Stream(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockCaptionedSynthesizer) SynthesizeCaptioned(ctx context.Context, text string, opts ...Option) (*CaptionedAudio, error) {
	return &CaptionedAudio{
		Data:       []byte{},
		Format:     "wav",
		DurationMs: 1000,
		Timestamps: []WordTimestamp{
			{Word: "test", StartMs: 0, EndMs: 500},
		},
	}, nil
}

func (m *mockCaptionedSynthesizer) StreamCaptioned(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error) {
	return nil, nil
}
