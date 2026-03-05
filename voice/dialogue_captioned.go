// Package voice provides interfaces and types for Text-to-Speech synthesis.
// It defines a modular interface that supports multiple TTS backends.
package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// DialogueSynthesizerCaptioned generates multi-speaker dialogue with perfect timing
// using word-level timestamps from_captioned synthesis.
//
// This synthesizer provides superior dialogue quality compared to DialogueSynthesizer
// by using actual speech duration and timing information instead of heuristics.
// It eliminates problems like:
//   - Double-pausing (built-in silence + added silence)
//   - Cutting words during crossfade
//   - Inconsistent speech rates between speakers
//   - Manual subtitle timing
//
// Requirements: The underlying synthesizer must implement CaptionedSynthesizer interface.
// Compatible providers: Kokoro-FastAPI (with /dev/captioned_speech endpoint).
type DialogueSynthesizerCaptioned struct {
	// Syn is the captioned synthesizer used to generate audio with timestamps.
	Syn CaptionedSynthesizer
	// VoiceMap maps speaker names to voice identifiers.
	VoiceMap map[string]string
	// SpeedMap maps speaker names to speech speed multipliers.
	// Values typically range from 0.8 to 1.2, where 1.0 is normal speed.
	SpeedMap map[string]float64
	// Format specifies the output audio format (e.g., "wav", "mp3").
	// Default is "wav" for best quality with crossfading.
	Format string
	// CrossfadeMs specifies crossfade duration in milliseconds (default: 50).
	// Set to 0 to disable crossfading.
	CrossfadeMs int
	// TargetPauseMs is the target pause between segments (default: 250).
	// This is the desired gap between the END of one speech and START of the next.
	TargetPauseMs int
	// NormalizeVolume enables peak volume normalization per segment (default: true).
	NormalizeVolume bool
	// GenerateSubtitles enables automatic subtitle generation (default: true).
	// When enabled, returns both audio and SRT-format subtitles.
	GenerateSubtitles bool
}

// NewDialogueSynthesizerCaptioned creates a new captioned dialogue synthesizer for multi-speaker audio generation.
// The synthesizer uses word-level timestamps for perfect pause calculation
// and optional subtitle generation.
//
// Prerequisites: The synthesizer parameter must implement CaptionedSynthesizer.
// This is supported by Kokoro-FastAPI and similar providers with timestamp capabilities.
//
// The format defaults to "wav" which preserves quality through multiple processing steps.
// For subtitle generation and timestamp analysis, WAV is strongly recommended.
//
// Returns an error if the synthesizer is nil or voiceMap is empty.
//
// Example:
//
//	syn, _ := openai.NewSynthesizer(openai.WithBaseURL("http://localhost:8880/v1"))
//	ds, err := voice.NewDialogueSynthesizerCaptioned(syn, map[string]string{
//	    "Alice": "af_bella",
//	    "Bob":   "am_adam",
//	})
func NewDialogueSynthesizerCaptioned(syn CaptionedSynthesizer, voiceMap map[string]string, format ...string) (*DialogueSynthesizerCaptioned, error) {
	if syn == nil {
		return nil, errors.New("voice: synthesizer cannot be nil")
	}
	if len(voiceMap) == 0 {
		return nil, errors.New("voice: voiceMap cannot be empty")
	}

	f := "wav"
	if len(format) > 0 && format[0] != "" {
		f = format[0]
	}
	return &DialogueSynthesizerCaptioned{
		Syn:               syn,
		VoiceMap:          voiceMap,
		Format:            f,
		CrossfadeMs:       50,
		TargetPauseMs:     250,
		NormalizeVolume:   true,
		GenerateSubtitles: true,
	}, nil
}

// speakerSpeed returns the speed multiplier for a speaker, defaulting to 1.0.
func (ds *DialogueSynthesizerCaptioned) speakerSpeed(speaker string) float64 {
	if ds.SpeedMap == nil {
		return 1.0
	}
	speed, ok := ds.SpeedMap[speaker]
	if !ok {
		return 1.0
	}
	if speed < 0.25 {
		return 0.25
	}
	if speed > 4.0 {
		return 4.0
	}
	return speed
}

// CaptionedDialogueResult contains the synthesis output with timing information.
type CaptionedDialogueResult struct {
	// Audio is the complete dialogue audio.
	Audio []byte
	// Format is the audio format (e.g., "wav").
	Format string
	// Segments contains timing information for each segment.
	Segments []CaptionedSegment
	// TotalDurationMs is the total dialogue duration in milliseconds.
	TotalDurationMs int
	// Subtitles is the SRT-format subtitle string, if enabled.
	Subtitles string
}

// CaptionedSegment represents one speaker's segment with timing details.
type CaptionedSegment struct {
	// Speaker is the segment speaker.
	Speaker string
	// Text is the spoken text.
	Text string
	// Audio is the segment audio data.
	Audio []byte
	// Timestamps contains word-level timing.
	Timestamps []WordTimestamp
	// StartMs is when this segment starts in the full dialogue.
	StartMs int
	// EndMs is when this segment ends in the full dialogue.
	EndMs int
	// DurationMs is the total segment duration including trailing silence.
	DurationMs int
	// SpeechDurationMs is the actual speech duration without trailing silence.
	SpeechDurationMs int
	// TrailingSilenceMs is the silence at the end of the audio.
	TrailingSilenceMs int
	// LeadingSilenceMs is the silence at the start of the audio.
	LeadingSilenceMs int
}

// CalculatePerfectPause calculates the exact pause needed between two segments.
// It uses word-level timestamps to avoid double-pausing and applies context-aware
// adjustments based on dialogue content.
func (ds *DialogueSynthesizerCaptioned) CalculatePerfectPause(prev, curr *CaptionedSegment) int {
	// Calculate built-in silence from timestamps
	prevTrailing := 0
	if len(prev.Timestamps) > 0 {
		lastWordEnd := prev.Timestamps[len(prev.Timestamps)-1].EndMs
		prevTrailing = prev.DurationMs - lastWordEnd
	}

	currLeading := 0
	if len(curr.Timestamps) > 0 {
		currLeading = curr.Timestamps[0].StartMs
	}

	// Total silence already in the audio
	builtInSilence := prevTrailing + currLeading

	// Start with target pause
	targetPause := ds.TargetPauseMs

	// Apply context-aware adjustments using shared logic
	multiplier := applyContextualPauseMultiplier(prev.Text, curr.Text, prev.Speaker, curr.Speaker)

	// Adjust for actual speech duration (timestamp-based enhancement)
	if prev.SpeechDurationMs < 600 {
		// Very short utterance, reduce pause further
		multiplier *= 0.9
	} else if prev.SpeechDurationMs > 2000 {
		// Long utterance, increase pause
		multiplier *= 1.1
	}

	// Calculate final target
	targetPause = int(float64(targetPause) * multiplier)

	// Add exactly what's needed
	additionalPause := targetPause - builtInSilence
	if additionalPause < 0 {
		// Already have enough silence, don't add more
		return 0
	}

	return additionalPause
}

// SynthesizeDialogueCaptioned generates dialogue with perfect timing using timestamps.
// This method provides superior audio quality by:
//   - Calculating exact pauses from actual speech duration
//   - Avoiding double-pausing (built-in silence + added silence)
//   - Crossfading at word boundaries instead of random positions
//   - Generating subtitles automatically (if enabled)
//
// Returns complete dialogue audio and detailed timing information for each segment.
func (ds *DialogueSynthesizerCaptioned) SynthesizeDialogueCaptioned(ctx context.Context, segments []DialogueSegment) (*CaptionedDialogueResult, error) {
	logger := slog.Default()

	if len(segments) == 0 {
		return nil, errors.New("voice: no segments provided")
	}

	logger.Info("starting captioned dialogue synthesis",
		"segment_count", len(segments),
		"generate_subtitles", ds.GenerateSubtitles,
		"crossfade_ms", ds.CrossfadeMs,
		"target_pause_ms", ds.TargetPauseMs,
	)

	// Synthesize all segments with timestamps
	captionedSegments := make([]CaptionedSegment, 0, len(segments))
	for i, seg := range segments {
		voiceID, ok := ds.VoiceMap[seg.Speaker]
		if !ok {
			return nil, fmt.Errorf("voice: no voice mapping for speaker %q (segment %d)", seg.Speaker, i)
		}

		logger.Debug("synthesizing segment",
			"index", i,
			"speaker", seg.Speaker,
			"voice_id", voiceID,
			"text_len", len(seg.Text),
			"text_preview", truncateText(seg.Text, 50),
		)

		audio, err := ds.Syn.SynthesizeCaptioned(ctx, seg.Text, WithVoice(voiceID), WithFormat(ds.Format), WithSpeed(ds.speakerSpeed(seg.Speaker)))
		if err != nil {
			return nil, fmt.Errorf("voice: failed to synthesize segment %d for speaker %q: %w", i, seg.Speaker, err)
		}

		logger.Debug("segment synthesized",
			"index", i,
			"audio_bytes", len(audio.Data),
			"duration_ms", audio.DurationMs,
			"word_count", len(audio.Timestamps),
		)

		// Calculate timing details
		trailingSilence := 0
		leadingSilence := 0
		speechDuration := audio.DurationMs

		if len(audio.Timestamps) > 0 {
			lastWordEnd := audio.Timestamps[len(audio.Timestamps)-1].EndMs
			trailingSilence = audio.DurationMs - lastWordEnd
			leadingSilence = audio.Timestamps[0].StartMs
			speechDuration = lastWordEnd - audio.Timestamps[0].StartMs

			logger.Debug("segment timing calculated",
				"index", i,
				"duration_ms", audio.DurationMs,
				"speech_duration_ms", speechDuration,
				"trailing_silence_ms", trailingSilence,
				"leading_silence_ms", leadingSilence,
				"first_word_start", audio.Timestamps[0].StartMs,
				"last_word_end", lastWordEnd,
			)
		}

		captionedSegments = append(captionedSegments, CaptionedSegment{
			Speaker:           seg.Speaker,
			Text:              seg.Text,
			Audio:             audio.Data,
			Timestamps:        audio.Timestamps,
			DurationMs:        audio.DurationMs,
			SpeechDurationMs:  speechDuration,
			TrailingSilenceMs: trailingSilence,
			LeadingSilenceMs:  leadingSilence,
		})
	}

	logger.Info("all segments synthesized, calculating pauses")

	// Calculate perfect pauses between segments
	pauses := make([]int, len(segments)-1)
	for i := 0; i < len(segments)-1; i++ {
		pause := ds.CalculatePerfectPause(&captionedSegments[i], &captionedSegments[i+1])
		pauses[i] = pause

		prev := &captionedSegments[i]
		curr := &captionedSegments[i+1]

		logger.Info("pause calculated",
			"between", fmt.Sprintf("segment %d (%s) -> %d (%s)", i, prev.Speaker, i+1, curr.Speaker),
			"pause_ms", pause,
			"target_pause_ms", ds.TargetPauseMs,
			"prev_duration_ms", prev.DurationMs,
			"prev_speech_ms", prev.SpeechDurationMs,
			"prev_trailing_ms", prev.TrailingSilenceMs,
			"curr_leading_ms", curr.LeadingSilenceMs,
			"built_in_silence_ms", prev.TrailingSilenceMs+curr.LeadingSilenceMs,
			"prev_text_preview", truncateText(prev.Text, 30),
			"curr_text_preview", truncateText(curr.Text, 30),
		)
	}

	logger.Info("concatenating audio",
		"segments", len(captionedSegments),
		"pauses", len(pauses),
		"crossfade_ms", ds.CrossfadeMs,
	)

	// Concatenate audio with perfect pauses
	audioData, err := concatenateCaptionedAudio(captionedSegments, pauses, ds.CrossfadeMs)
	if err != nil {
		return nil, fmt.Errorf("voice: failed to concatenate audio: %w", err)
	}

	logger.Info("audio concatenated successfully",
		"output_bytes", len(audioData),
	)

	// Generate subtitles if enabled
	subtitles := ""
	if ds.GenerateSubtitles {
		subtitles = generateSRT(captionedSegments)
		logger.Debug("subtitles generated",
			"subtitle_len", len(subtitles),
		)
	}

	// Calculate total duration
	totalDuration := 0
	for _, seg := range captionedSegments {
		totalDuration += seg.DurationMs
	}
	for _, pause := range pauses {
		totalDuration += pause
	}

	logger.Info("dialogue synthesis complete",
		"total_duration_ms", totalDuration,
		"total_duration_sec", totalDuration/1000,
		"total_words", countTotalWords(captionedSegments),
		"total_pauses", len(pauses),
		"avg_pause_ms", averagePause(pauses),
	)

	result := &CaptionedDialogueResult{
		Audio:           audioData,
		Format:          ds.Format,
		Segments:        captionedSegments,
		TotalDurationMs: totalDuration,
		Subtitles:       subtitles,
	}

	// Validate result
	if err := validateCaptionedResult(result); err != nil {
		return nil, fmt.Errorf("voice: validation failed: %w", err)
	}

	return result, nil
}

// validateCaptionedResult ensures the dialogue result is consistent.
func validateCaptionedResult(result *CaptionedDialogueResult) error {
	if len(result.Audio) == 0 {
		return fmt.Errorf("no audio data generated")
	}
	if result.Format == "" {
		return fmt.Errorf("audio format not specified")
	}
	if len(result.Segments) == 0 {
		return fmt.Errorf("no segments in result")
	}
	if result.TotalDurationMs <= 0 {
		return fmt.Errorf("invalid total duration: %dms", result.TotalDurationMs)
	}

	// Validate each segment
	for i, seg := range result.Segments {
		if seg.Speaker == "" {
			return fmt.Errorf("segment %d has empty speaker", i)
		}
		if seg.DurationMs <= 0 {
			return fmt.Errorf("segment %d has invalid duration: %dms", i, seg.DurationMs)
		}
		if len(seg.Audio) == 0 {
			return fmt.Errorf("segment %d has no audio data", i)
		}
		// Validate timestamp ordering
		for j, ts := range seg.Timestamps {
			if ts.StartMs < 0 || ts.EndMs < 0 {
				return fmt.Errorf("segment %d timestamp %d has negative time", i, j)
			}
			if ts.EndMs < ts.StartMs {
				return fmt.Errorf("segment %d timestamp %d has end time before start time", i, j)
			}
			if ts.Word == "" {
				return fmt.Errorf("segment %d timestamp %d has empty word", i, j)
			}
		}
	}

	return nil
}

// GenerateSRT creates SRT-format subtitles from captioned segments.
// This automatically generates perfectly timed subtitles without manual adjustment.
// This is a convenience method that wraps the internal generateSRT function.
func (ds *DialogueSynthesizerCaptioned) GenerateSRT(segments []CaptionedSegment) string {
	return generateSRT(segments)
}

// AnalyzeSpeechRate calculates words per minute for a speaker.
// This enables automatic speed adjustment for consistent pacing.
func AnalyzeSpeechRate(segments []CaptionedSegment) float64 {
	if len(segments) == 0 {
		return 0
	}

	totalWords := 0
	totalDurationMs := 0

	for _, seg := range segments {
		totalWords += len(seg.Timestamps)
		totalDurationMs += seg.SpeechDurationMs
	}

	if totalDurationMs == 0 {
		return 0
	}

	// Words per minute = (words / minutes)
	minutes := float64(totalDurationMs) / 60000.0
	return float64(totalWords) / minutes
}

// truncateText truncates text to maxLen characters for logging
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// countTotalWords counts total words across all segments
func countTotalWords(segments []CaptionedSegment) int {
	total := 0
	for _, seg := range segments {
		total += len(seg.Timestamps)
	}
	return total
}

// averagePause calculates average pause duration
func averagePause(pauses []int) int {
	if len(pauses) == 0 {
		return 0
	}
	sum := 0
	for _, p := range pauses {
		sum += p
	}
	return sum / len(pauses)
}
