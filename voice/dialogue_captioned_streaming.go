package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// StreamDialogueCaptioned streams dialogue with timestamps, calculating perfect
// pauses on-the-fly using speech duration information.
//
// IMPLEMENTATION STATUS: This method is currently a stub and returns an error.
// Streaming captioned dialogue requires buffering segments anyway to calculate
// perfect pauses, so there's no significant benefit over SynthesizeDialogueCaptioned.
//
// FUTURE WORK: If streaming is needed for very long dialogues, consider:
// 1. Using a heuristic pause calculation instead of perfect pause
// 2. Buffering N segments ahead for pause calculation while streaming
// 3. Using a separate goroutine for synthesis and another for assembly
//
// For now, use SynthesizeDialogueCaptioned which provides the full feature set.
func (ds *DialogueSynthesizerCaptioned) StreamDialogueCaptioned(ctx context.Context, segments []DialogueSegment) (io.ReadCloser, error) {
	return nil, fmt.Errorf("voice: streaming captioned dialogue not yet implemented - use SynthesizeDialogueCaptioned instead")
}

// concatenateCaptionedAudio concatenates multiple captioned audio segments with
// perfect pauses into a single audio stream.
// Reuses WAV processing from dialogue.go for proper audio handling.
func concatenateCaptionedAudio(segments []CaptionedSegment, pauses []int, crossfadeMs int) ([]byte, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	if len(segments) == 1 {
		return segments[0].Audio, nil
	}

	// Parse first segment to get WAV format
	wavFormat, err := parseWAVHeader(segments[0].Audio)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WAV header: %w", err)
	}

	// Normalize all segments first (consistent volume before crossfade)
	for _, seg := range segments {
		if len(seg.Audio) > wavFormat.dataOffset {
			normalizeWAVVolume(seg.Audio, wavFormat)
		}
	}

	// Build result: start with first segment (includes WAV header)
	var result bytes.Buffer
	result.Write(segments[0].Audio)

	// Track previous audio for crossfading
	prevRaw := segments[0].Audio[wavFormat.dataOffset:]

	// Add subsequent segments with pauses and crossfade
	for i := 1; i < len(segments); i++ {
		currentRaw := segments[i].Audio[wavFormat.dataOffset:]

		// Insert pause silence
		if i-1 < len(pauses) && pauses[i-1] > 0 {
			pauseBytes := calculatePauseBytes(pauses[i-1], wavFormat)
			silence := make([]byte, pauseBytes)
			result.Write(silence)
		}

		// Apply crossfade if enabled and we have enough audio
		// Match the logic from streamWAVSegment in dialogue.go
		if crossfadeMs > 0 && len(prevRaw) > 0 && len(currentRaw) > 0 {
			bytesPerFrame := wavFormat.bytesPerSample * wavFormat.numChannels
			crossfadeBytes := (crossfadeMs * wavFormat.sampleRate * bytesPerFrame) / 1000
			crossfadeBytes = min(crossfadeBytes, len(prevRaw)/4, len(currentRaw)/4)
			crossfadeBytes = (crossfadeBytes / bytesPerFrame) * bytesPerFrame

			if crossfadeBytes >= bytesPerFrame {
				// Crossfade the end of previous with start of current
				crossfadeRegion := crossfadeWAVEqualPower(
					prevRaw[len(prevRaw)-crossfadeBytes:],
					currentRaw[:crossfadeBytes],
					wavFormat,
					crossfadeMs,
				)
				// Write crossfaded region (contains blended overlap)
				result.Write(crossfadeRegion)
				// Write the rest of current segment (after crossfade region)
				// Note: crossfadeRegion contains (prev_tail - crossfade) + crossfaded + part_of_next
				// So we need to continue from middle of current
				result.Write(currentRaw[crossfadeBytes/2:])
			} else {
				result.Write(currentRaw)
			}
		} else {
			result.Write(currentRaw)
		}

		prevRaw = currentRaw
	}

	return result.Bytes(), nil
}

// calculatePauseBytes converts milliseconds to bytes based on WAV format.
func calculatePauseBytes(pauseMs int, format wavInfo) int {
	// Use int64 for intermediate calculations to avoid overflow with large pauses
	samples := (int64(pauseMs) * int64(format.sampleRate)) / 1000
	bytes := samples * int64(format.numChannels) * int64(format.bytesPerSample)
	return int(bytes)
}

// generateSRT creates SRT-format subtitles from captioned segments.
func generateSRT(segments []CaptionedSegment) string {
	var srt bytes.Buffer
	index := 1
	globalOffsetMs := 0

	for _, seg := range segments {
		segOffset := globalOffsetMs

		for _, ts := range seg.Timestamps {
			startMs := segOffset + ts.StartMs
			endMs := segOffset + ts.EndMs

			srt.WriteString(fmt.Sprintf("%d\n", index))
			srt.WriteString(fmt.Sprintf("%s --> %s\n",
				formatSRTTime(startMs),
				formatSRTTime(endMs)))
			srt.WriteString(fmt.Sprintf("%s\n\n", ts.Word))

			index++
		}

		globalOffsetMs += seg.DurationMs
	}

	return srt.String()
}

// generateSRTWithSpeakers creates SRT-format subtitles with speaker labels.
// Each subtitle line includes "[Speaker]: word" format, useful for multi-speaker content.
func generateSRTWithSpeakers(segments []CaptionedSegment) string {
	var srt bytes.Buffer
	index := 1
	globalOffsetMs := 0

	for _, seg := range segments {
		segOffset := globalOffsetMs

		for _, ts := range seg.Timestamps {
			startMs := segOffset + ts.StartMs
			endMs := segOffset + ts.EndMs

			srt.WriteString(fmt.Sprintf("%d\n", index))
			srt.WriteString(fmt.Sprintf("%s --> %s\n",
				formatSRTTime(startMs),
				formatSRTTime(endMs)))
			srt.WriteString(fmt.Sprintf("[%s]: %s\n\n", seg.Speaker, ts.Word))

			index++
		}

		globalOffsetMs += seg.DurationMs
	}

	return srt.String()
}

// formatSRTTime converts milliseconds to SRT time format.
func formatSRTTime(ms int) string {
	hours := ms / 3600000
	ms %= 3600000
	minutes := ms / 60000
	ms %= 60000
	seconds := ms / 1000
	milliseconds := ms % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}
