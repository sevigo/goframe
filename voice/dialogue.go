package voice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
}

// NewDialogueSynthesizer creates a new dialogue synthesizer.
// The format defaults to "wav" which supports reliable concatenation.
func NewDialogueSynthesizer(syn Synthesizer, voiceMap map[string]string, format ...string) *DialogueSynthesizer {
	f := "wav"
	if len(format) > 0 && format[0] != "" {
		f = format[0]
	}
	return &DialogueSynthesizer{
		Syn:      syn,
		VoiceMap: voiceMap,
		Format:   f,
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
// For WAV format, it properly handles headers to produce a valid concatenated audio file.
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

			_, err = writer.Write(toWrite)
			if err != nil {
				writer.CloseWithError(err)
				return
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

// stripWAVHeader removes the 44-byte WAV header from audio data.
// This is necessary when concatenating multiple WAV files.
func stripWAVHeader(data []byte) []byte {
	const wavHeaderSize = 44
	if len(data) <= wavHeaderSize {
		return data
	}
	return data[wavHeaderSize:]
}
