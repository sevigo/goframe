// Package main demonstrates captioned dialogue synthesis with timestamp-based timing.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║    Captioned Dialogue Synthesis with Word-Level Timestamps  ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Make sure Kokoro is running:")
	fmt.Println("  docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest")
	fmt.Println()

	if err := run(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	// Create synthesizer - it implements CaptionedSynthesizer interface
	synthesizer, err := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithFormat("wav"),
	)
	if err != nil {
		return fmt.Errorf("failed to create synthesizer: %w", err)
	}

	// The Synthesizer pointer implements CaptionedSynthesizer
	captionedSyn := synthesizer

	fmt.Println("✓ Synthesizer supports word-level timestamps")
	fmt.Println()

	// Create captioned dialogue synthesizer
	dialogueSyn := voice.NewDialogueSynthesizerCaptioned(captionedSyn, map[string]string{
		"Alice": "af_bella(3)+af_heart(1)",
		"Bob":   "am_adam",
	})
	dialogueSyn.SpeedMap = map[string]float64{
		"Alice": 1.0,
		"Bob":   0.95,
	}
	dialogueSyn.TargetPauseMs = 250
	dialogueSyn.GenerateSubtitles = true

	fmt.Println("Synthesizing captioned dialogue:")
	fmt.Println("  Alice: af_bella(3)+af_heart(1) @ 1.00x")
	fmt.Println("  Bob:   am_adam                  @ 0.95x")
	fmt.Println()

	dialogue := []voice.DialogueSegment{
		{Speaker: "Alice", Text: "What do you think about Tokyo?"},
		{Speaker: "Bob", Text: "I think it's amazing!"},
		{Speaker: "Alice", Text: "Yeah."},
		{Speaker: "Bob", Text: "And then we went to the station,"},
		{Speaker: "Alice", Text: "and the train was already there."},
	}

	ctx := context.Background()
	start := time.Now()

	result, err := dialogueSyn.SynthesizeDialogueCaptioned(ctx, dialogue)
	if err != nil {
		return fmt.Errorf("failed to synthesize dialogue: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("✓ Dialogue synthesized in %v\n", elapsed)
	fmt.Printf("✓ Total duration: %dms\n", result.TotalDurationMs)
	fmt.Println()

	// Display timing analysis
	fmt.Println("Segment Analysis:")
	fmt.Println("────────────────────────────────────────────────────────")
	for i, seg := range result.Segments {
		fmt.Printf("\n[%d] %s: \"%s\"\n", i+1, seg.Speaker, seg.Text)
		fmt.Printf("  Duration: %dms (speech: %dms)\n", seg.DurationMs, seg.SpeechDurationMs)
		fmt.Printf("  Silence: %dms leading, %dms trailing\n", seg.LeadingSilenceMs, seg.TrailingSilenceMs)
		fmt.Printf("  Words: %d\n", len(seg.Timestamps))

		// Show first few word timestamps
		if len(seg.Timestamps) > 0 {
			fmt.Printf("  Timestamps:\n")
			max := 5
			if len(seg.Timestamps) < max {
				max = len(seg.Timestamps)
			}
			for j := 0; j < max; j++ {
				ts := seg.Timestamps[j]
				fmt.Printf("    [%d-%dms] %s\n", ts.StartMs, ts.EndMs, ts.Word)
			}
			if len(seg.Timestamps) > max {
				fmt.Printf("    ... and %d more words\n", len(seg.Timestamps)-max)
			}
		}
	}

	// Show pause calculations
	fmt.Println("\nPause Analysis:")
	fmt.Println("────────────────────────────────────────────────────────")
	for i := 0; i < len(result.Segments)-1; i++ {
		pause := result.Segments[i+1].StartMs - result.Segments[i].EndMs
		fmt.Printf("After \"%s\": %dms pause (target: %dms)\n",
			result.Segments[i].Text, pause, dialogueSyn.TargetPauseMs)
	}

	// Save subtitles
	if result.Subtitles != "" {
		subtitleFile := "dialogue.srt"
		if err := os.WriteFile(subtitleFile, []byte(result.Subtitles), 0644); err != nil {
			return fmt.Errorf("failed to save subtitles: %w", err)
		}
		fmt.Printf("\n✓ Subtitles saved to: %s\n", subtitleFile)
		fmt.Println("\nFirst few subtitle entries:")
		fmt.Println("────────────────────────────────")
		lines := splitLines(result.Subtitles, 20)
		for _, line := range lines {
			fmt.Println(line)
		}
	}

	// Save audio
	audioFile := "dialogue_captioned.wav"
	if err := os.WriteFile(audioFile, result.Audio, 0644); err != nil {
		return fmt.Errorf("failed to save audio: %w", err)
	}
	fmt.Printf("\n✓ Audio saved to: %s (%d bytes)\n", audioFile, len(result.Audio))

	// Speech rate analysis
	aliceWPM := voice.AnalyzeSpeechRate(filterSegments(result.Segments, "Alice"))
	bobWPM := voice.AnalyzeSpeechRate(filterSegments(result.Segments, "Bob"))
	fmt.Printf("\nSpeech Rate Analysis:\n")
	fmt.Printf("  Alice: %.0f words/min\n", aliceWPM)
	fmt.Printf("  Bob:   %.0f words/min\n", bobWPM)

	fmt.Println("\nPlay with: ffplay dialogue_captioned.wav")
	fmt.Println("Subtitles: ffplay dialogue_captioned.wav -vf subtitles=dialogue.srt")

	return nil
}

func filterSegments(segments []voice.CaptionedSegment, speaker string) []voice.CaptionedSegment {
	var filtered []voice.CaptionedSegment
	for _, seg := range segments {
		if seg.Speaker == speaker {
			filtered = append(filtered, seg)
		}
	}
	return filtered
}

func splitLines(s string, maxLines int) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s) && len(lines) < maxLines; i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) && len(lines) < maxLines {
		lines = append(lines, s[start:])
	}
	return lines
}
