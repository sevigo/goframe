package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sync"
)

// DialogueSegment represents a single speaker's line in a dialogue.
type DialogueSegment struct {
	// Speaker identifies who is speaking (e.g., "Alice", "Bob").
	Speaker string
	// Text is what the speaker says.
	Text string
}

// DialogueSynthesizer generates audio for multi-speaker dialogues.
// It maps speakers to voice IDs and synthesizes each segment with the
// appropriate voice, concatenating the results into a single audio stream.
type DialogueSynthesizer struct {
	// Synthesizer is the underlying TTS engine.
	Syn Synthesizer
	// VoiceMap maps speaker names to voice IDs.
	// For OpenAI: alloy, echo, fable, onyx, nova, shimmer.
	// For Kokoro: af_bella, af_sky, am_adam, etc.
	VoiceMap map[string]string
	// Format specifies the output audio format (e.g., "wav", "mp3").
	// Default is "wav" for better concatenation support.
	Format string
	// CrossfadeMs specifies crossfade duration in milliseconds (default: 50ms).
	// Set to 0 to disable crossfading.
	CrossfadeMs int
}

// NewDialogueSynthesizer creates a new dialogue synthesizer.
// The format defaults to "wav" which supports reliable concatenation.
// Crossfading is enabled by default (50ms) to smooth transitions between segments.
func NewDialogueSynthesizer(syn Synthesizer, voiceMap map[string]string, format ...string) *DialogueSynthesizer {
	f := "wav"
	if len(format) > 0 && format[0] != "" {
		f = format[0]
	}
	return &DialogueSynthesizer{
		Syn:         syn,
		VoiceMap:    voiceMap,
		Format:      f,
		CrossfadeMs: 50,
	}
}

// SynthesizeDialogue generates audio for all segments and returns individual audio files.
// This is useful when you want to process each speaker's audio separately.
func (ds *DialogueSynthesizer) SynthesizeDialogue(ctx context.Context, segments []DialogueSegment) ([]*Audio, error) {
	if len(segments) == 0 {
		return nil, errors.New("voice: no segments provided")
	}

	results := make([]*Audio, 0, len(segments))
	for i, seg := range segments {
		voiceID, ok := ds.VoiceMap[seg.Speaker]
		if !ok {
			return nil, fmt.Errorf("voice: no voice mapping for speaker %q", seg.Speaker)
		}

		audio, err := ds.Syn.Synthesize(ctx, seg.Text, WithVoice(voiceID), WithFormat(ds.Format))
		if err != nil {
			return nil, fmt.Errorf("voice: failed to synthesize segment %d (speaker %q): %w", i, seg.Speaker, err)
		}

		results = append(results, audio)
	}

	return results, nil
}

// StreamDialogue generates audio for all segments and streams them as a single concatenated audio stream.
// For WAV format, it properly handles headers and applies crossfading to eliminate clicks.
// For MP3 format, it concatenates raw audio data which may not work with all players.
//
// The caller must close the returned ReadCloser when done.
func (ds *DialogueSynthesizer) StreamDialogue(ctx context.Context, segments []DialogueSegment) (io.ReadCloser, error) {
	if len(segments) == 0 {
		return nil, errors.New("voice: no segments provided")
	}

	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()

		var previousData []byte
		writtenHeader := false

		for i, seg := range segments {
			voiceID, ok := ds.VoiceMap[seg.Speaker]
			if !ok {
				slog.Error("No voice mapping for speaker", "speaker", seg.Speaker, "segment", i)
				writer.CloseWithError(fmt.Errorf("voice: no voice mapping for speaker %q", seg.Speaker))
				return
			}

			stream, err := ds.Syn.Stream(ctx, seg.Text, WithVoice(voiceID), WithFormat(ds.Format))
			if err != nil {
				slog.Error("Failed to synthesize segment", "speaker", seg.Speaker, "segment", i, "error", err)
				writer.CloseWithError(fmt.Errorf("voice: failed to synthesize segment %d: %w", i, err))
				return
			}

			data, err := io.ReadAll(stream)
			if closeErr := stream.Close(); closeErr != nil {
				slog.Warn("Failed to close stream", "error", closeErr)
			}
			if err != nil {
				writer.CloseWithError(fmt.Errorf("voice: failed to read segment %d: %w", i, err))
				return
			}

			// For WAV, strip headers and apply crossfade
			if ds.Format == "wav" {
				audioData := data
				if !writtenHeader {
					// First segment: keep the full WAV header
					writtenHeader = true
				} else {
					// Subsequent segments: strip the header
					audioData = stripWAVHeader(data)
				}

				// Apply crossfade if enabled and we have previous data
				if ds.CrossfadeMs > 0 && len(previousData) > 0 && len(audioData) > 0 {
					crossfaded := crossfadeWAV(previousData, audioData, ds.CrossfadeMs)
					_, err = writer.Write(crossfaded)
					previousData = audioData
				} else {
					_, err = writer.Write(audioData)
					previousData = audioData
				}

				if err != nil {
					writer.CloseWithError(err)
					return
				}
			} else {
				// For MP3, just concatenate
				_, err = writer.Write(data)
				if err != nil {
					writer.CloseWithError(err)
					return
				}
			}
		}
	}()

	return reader, nil
}

// StreamDialogueParallel generates audio for segments in parallel and streams them in order.
// This is faster than StreamDialogue but uses more memory due to parallel processing.
// The segments are synthesized concurrently but streamed in their original order.
func (ds *DialogueSynthesizer) StreamDialogueParallel(ctx context.Context, segments []DialogueSegment) (io.ReadCloser, error) {
	if len(segments) == 0 {
		return nil, errors.New("voice: no segments provided")
	}

	type result struct {
		index int
		data  []byte
		err   error
	}

	results := make(chan result, len(segments))
	var wg sync.WaitGroup

	for i, seg := range segments {
		wg.Add(1)
		go func(idx int, s DialogueSegment) {
			defer wg.Done()

			voiceID, ok := ds.VoiceMap[s.Speaker]
			if !ok {
				results <- result{index: idx, err: fmt.Errorf("voice: no voice mapping for speaker %q", s.Speaker)}
				return
			}

			stream, err := ds.Syn.Stream(ctx, s.Text, WithVoice(voiceID), WithFormat(ds.Format))
			if err != nil {
				results <- result{index: idx, err: fmt.Errorf("voice: failed to synthesize segment %d: %w", idx, err)}
				return
			}

			data, err := io.ReadAll(stream)
			if closeErr := stream.Close(); closeErr != nil {
				slog.Warn("Failed to close stream", "error", closeErr)
			}
			if err != nil {
				results <- result{index: idx, err: fmt.Errorf("voice: failed to read segment %d: %w", idx, err)}
				return
			}

			results <- result{index: idx, data: data}
		}(i, seg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	orderedData := make([][]byte, len(segments))
	for res := range results {
		if res.err != nil {
			return nil, res.err
		}
		orderedData[res.index] = res.data
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()

		writtenHeader := false
		for i, data := range orderedData {
			var toWrite []byte
			if ds.Format == "wav" {
				if !writtenHeader {
					toWrite = data
					writtenHeader = true
				} else {
					toWrite = stripWAVHeader(data)
				}
			} else {
				toWrite = data
			}

			_, err := writer.Write(toWrite)
			if err != nil {
				writer.CloseWithError(err)
				return
			}
			slog.Debug("Wrote segment", "index", i, "bytes", len(toWrite))
		}
	}()

	return reader, nil
}

const wavHeaderSize = 44

// stripWAVHeader removes the 44-byte WAV header from audio data.
// This is necessary when concatenating multiple WAV files.
func stripWAVHeader(data []byte) []byte {
	if len(data) <= wavHeaderSize {
		return data
	}
	return data[wavHeaderSize:]
}

// crossfadeWAV applies a crossfade between the end of prev and start of next.
// Both prev and next should be raw audio data (without WAV headers).
// durationMs is the crossfade duration in milliseconds.
func crossfadeWAV(prev, next []byte, durationMs int) []byte {
	if len(prev) == 0 || len(next) == 0 || durationMs <= 0 {
		return append(prev, next...)
	}

	// Assume 16-bit PCM audio (2 bytes per sample)
	sampleRate := 24000 // Common TTS sample rate
	bytesPerSample := 2
	channels := 1

	// Calculate samples for crossfade
	crossfadeSamples := (durationMs * sampleRate * channels) / 1000
	crossfadeBytes := crossfadeSamples * bytesPerSample

	// Don't crossfade more than available data
	if crossfadeBytes > len(prev)/2 || crossfadeBytes > len(next)/2 {
		crossfadeBytes = min(len(prev)/4, len(next)/4)
		crossfadeBytes = (crossfadeBytes / 2) * 2 // Align to sample boundary
	}

	if crossfadeBytes < 4 {
		return append(prev, next...)
	}

	// Ensure alignment
	crossfadeSamples = crossfadeBytes / bytesPerSample

	// Create output buffer
	output := make([]byte, len(prev)+len(next)-crossfadeBytes)
	copy(output, prev)

	// Apply crossfade: fade out end of prev, fade in start of next
	for i := range crossfadeSamples {
		// Position in prev (from the end)
		prevPos := len(prev) - crossfadeBytes + i*bytesPerSample
		// Position in next (from the start)
		nextPos := i * bytesPerSample
		// Position in output
		outPos := prevPos

		if prevPos+1 < len(prev) && nextPos+1 < len(next) && outPos+1 < len(output) {
			// Read 16-bit samples
			prevSample := int16(binary.LittleEndian.Uint16(prev[prevPos : prevPos+2]))
			nextSample := int16(binary.LittleEndian.Uint16(next[nextPos : nextPos+2]))

			// Calculate fade weights
			fadeOut := 1.0 - float64(i)/float64(crossfadeSamples)
			fadeIn := float64(i) / float64(crossfadeSamples)

			// Mix samples
			mixed := float64(prevSample)*fadeOut + float64(nextSample)*fadeIn
			if mixed > math.MaxInt16 {
				mixed = math.MaxInt16
			} else if mixed < math.MinInt16 {
				mixed = math.MinInt16
			}

			binary.LittleEndian.PutUint16(output[outPos:outPos+2], uint16(int16(mixed)))
		}
	}

	// Copy rest of next after crossfade
	copy(output[len(prev)-crossfadeBytes+crossfadeBytes:], next[crossfadeBytes:])

	return output
}
