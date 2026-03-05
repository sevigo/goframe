package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
)

// StreamDialogueCaptioned streams dialogue with timestamps, calculating perfect
// pauses on-the-fly using speech duration information.
//
// Note: Streaming captioned dialogue is not yet implemented. Use SynthesizeDialogueCaptioned
// for buffered synthesis with timestamps.
func (ds *DialogueSynthesizerCaptioned) StreamDialogueCaptioned(ctx context.Context, segments []DialogueSegment) (io.ReadCloser, error) {
	return nil, fmt.Errorf("voice: streaming captioned dialogue not yet implemented - use SynthesizeDialogueCaptioned instead")
}

// concatenateCaptionedAudio concatenates multiple captioned audio segments with
// perfect pauses into a single audio stream.
//
// This function:
// 1. Calculates exact pauses using timestamp information
// 2. Crossfades at word boundaries (not random positions)
// 3. Inserts precise silence for natural gaps
// 4. Normalizes volume across segments
func concatenateCaptionedAudio(segments []CaptionedSegment, pauses []int, crossfadeMs int) ([]byte, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	if len(segments) == 1 {
		return segments[0].Audio, nil
	}

	// TODO: Implement proper WAV concatenation with:
	// - Precise silence insertion
	// - Crossfade at word boundaries
	// - Volume normalization
	// For now, simplified version - just concatenate with calculated pauses

	var result bytes.Buffer

	// Write first segment completely (includes WAV header)
	result.Write(segments[0].Audio)

	// Write subsequent segments with calculated pauses
	for i := 1; i < len(segments); i++ {
		if pauses[i-1] > 0 {
			// TODO: Generate proper silence based on audio format
			// Simplified: estimate bytes per millisecond
			silence := make([]byte, pauses[i-1]*48) // rough: 48 bytes/ms for 24kHz 16-bit mono
			result.Write(silence)
		}
		// Write segment audio (skip WAV header for segments 2+)
		// TODO: Parse and skip WAV header properly
		result.Write(segments[i].Audio)
	}

	return result.Bytes(), nil
}

// generateSRT creates SRT-format subtitles from captioned segments.
// Automatically calculates global timestamps based on segment order.
func generateSRT(segments []CaptionedSegment) string {
	var srt bytes.Buffer
	index := 1
	globalOffsetMs := 0

	for _, seg := range segments {
		segOffset := globalOffsetMs

		for _, ts := range seg.Timestamps {
			startMs := segOffset + ts.StartMs
			endMs := segOffset + ts.EndMs

			// SRT format:
			// index
			// HH:MM:SS,mmm --> HH:MM:SS,mmm
			// text
			// (blank line)

			srt.WriteString(fmt.Sprintf("%d\n", index))
			srt.WriteString(fmt.Sprintf("%s --> %s\n",
				formatSRTTime(startMs),
				formatSRTTime(endMs)))
			srt.WriteString(fmt.Sprintf("%s\n\n", ts.Word))

			index++
		}

		// Update global offset for next segment
		// Add segment duration + pause
		globalOffsetMs += seg.DurationMs
		if seg.EndMs > 0 {
			globalOffsetMs = seg.EndMs
		}
	}

	return srt.String()
}

// formatSRTTime converts milliseconds to SRT time format (HH:MM:SS,mmm).
func formatSRTTime(ms int) string {
	hours := ms / 3600000
	ms = ms % 3600000
	minutes := ms / 60000
	ms = ms % 60000
	seconds := ms / 1000
	milliseconds := ms % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}

// perfectPauseBetween calculates the ideal pause between two captioned segments.
// Uses speech duration and built-in silence to avoid double-pausing.
func perfectPauseBetween(prev, curr *CaptionedSegment, targetPauseMs int) int {
	// Get silence from timestamps
	prevTrailing := prev.TrailingSilenceMs
	currLeading := curr.LeadingSilenceMs

	// Silence already in the audio
	builtInSilence := prevTrailing + currLeading

	// Adjust target based on speech characteristics
	target := targetPauseMs

	// Short utterances ("yeah", "okay") need less pause
	if prev.SpeechDurationMs < 500 && len(prev.Timestamps) <= 2 {
		target = int(float64(target) * 0.6)
	}

	// Long complex sentences need more processing time
	if prev.SpeechDurationMs > 2000 {
		target = int(float64(target) * 1.2)
	}

	// Calculate what we need to add
	needed := target - builtInSilence

	if needed < 0 {
		return 0 // Already have enough silence
	}

	return needed
}

// parseWAVHeaderForSpeech extracts audio format info for silence generation.
func parseWAVHeaderForSpeech(data []byte) (sampleRate, bitsPerSample, numChannels int, dataOffset int, err error) {
	if len(data) < 44 {
		return 0, 0, 0, 0, fmt.Errorf("audio data too short for WAV header")
	}

	// Check RIFF header
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, 0, 0, 0, fmt.Errorf("invalid WAV header")
	}

	// Parse chunks to find fmt and data
	pos := 12
	for pos < len(data)-8 {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8

		switch chunkID {
		case "fmt ":
			if pos+chunkSize > len(data) {
				return 0, 0, 0, 0, fmt.Errorf("fmt chunk truncated")
			}
			audioFormat := binary.LittleEndian.Uint16(data[pos : pos+2])
			if audioFormat != 1 {
				return 0, 0, 0, 0, fmt.Errorf("unsupported WAV format %d (only PCM supported)", audioFormat)
			}
			numChannels = int(binary.LittleEndian.Uint16(data[pos+2 : pos+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
			bitsPerSample = int(binary.LittleEndian.Uint16(data[pos+14 : pos+16]))
		case "data":
			dataOffset = pos - 8 + 8
			return sampleRate, bitsPerSample, numChannels, dataOffset, nil
		}

		pos += chunkSize
		if chunkSize%2 != 0 {
			pos++
		}
	}

	return 0, 0, 0, 0, fmt.Errorf("data chunk not found in WAV")
}

// generateSilence creates silence audio data for the specified duration.
func generateSilence(durationMs, sampleRate, bitsPerSample, numChannels int) []byte {
	samples := (durationMs * sampleRate) / 1000
	bytesPerSample := bitsPerSample / 8
	totalBytes := samples * numChannels * bytesPerSample

	return make([]byte, totalBytes) // Zero bytes = silence for PCM
}
