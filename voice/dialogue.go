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
	// Note: Crossfading requires buffering segments in memory.
	CrossfadeMs int
	// PauseMs specifies pause duration between segments in milliseconds (default: 100ms).
	// Set to 0 to disable pauses between segments.
	PauseMs int
}

// NewDialogueSynthesizer creates a new dialogue synthesizer.
// The format defaults to "wav" which supports reliable concatenation.
// Crossfading is enabled by default (50ms) to smooth transitions between segments
// using equal-power curves for constant perceived loudness.
// Pause between segments defaults to 150ms.
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
		PauseMs:     150,
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

// wavInfo holds parsed WAV metadata.
type wavInfo struct {
	sampleRate     int
	bitsPerSample  int
	numChannels    int
	dataOffset     int
	bytesPerSample int
}

// parseWAVHeader extracts audio format info from WAV header.
// Handles variable header sizes by parsing RIFF chunks.
func parseWAVHeader(data []byte) (wavInfo, error) {
	info := wavInfo{}
	if len(data) < 44 {
		return info, errors.New("voice: data too short for WAV header")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return info, errors.New("voice: invalid WAV magic numbers")
	}

	pos := 12
	for pos < len(data)-8 {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8

		switch chunkID {
		case "fmt ":
			if pos+chunkSize > len(data) {
				return info, errors.New("voice: fmt chunk truncated")
			}
			audioFormat := binary.LittleEndian.Uint16(data[pos : pos+2])
			if audioFormat != 1 {
				return info, fmt.Errorf("voice: unsupported WAV format %d (only PCM supported)", audioFormat)
			}
			info.numChannels = int(binary.LittleEndian.Uint16(data[pos+2 : pos+4]))
			info.sampleRate = int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
			info.bitsPerSample = int(binary.LittleEndian.Uint16(data[pos+14 : pos+16]))
			info.bytesPerSample = info.bitsPerSample / 8
		case "data":
			info.dataOffset = pos - 8 + 8
			return info, nil
		}

		pos += chunkSize
		if chunkSize%2 != 0 {
			pos++
		}
	}

	return info, errors.New("voice: data chunk not found")
}

// StreamDialogue generates audio for all segments and streams them as a single concatenated audio stream.
func (ds *DialogueSynthesizer) StreamDialogue(ctx context.Context, segments []DialogueSegment) (io.ReadCloser, error) {
	if len(segments) == 0 {
		return nil, errors.New("voice: no segments provided")
	}

	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()

		var (
			segmentBuffer [][]byte
			wavFormat     wavInfo
			err           error
		)

		// Buffer all segments first to apply crossfades correctly
		for i, seg := range segments {
			voiceID, ok := ds.VoiceMap[seg.Speaker]
			if !ok {
				writer.CloseWithError(fmt.Errorf("voice: no voice mapping for speaker %q", seg.Speaker))
				return
			}

			stream, synErr := ds.Syn.Stream(ctx, seg.Text, WithVoice(voiceID), WithFormat(ds.Format))
			if synErr != nil {
				writer.CloseWithError(fmt.Errorf("voice: failed to stream segment %d: %w", i, synErr))
				return
			}

			data, readErr := io.ReadAll(stream)
			if closeErr := stream.Close(); closeErr != nil {
				slog.Warn("Failed to close stream", "error", closeErr)
			}
			if readErr != nil {
				writer.CloseWithError(fmt.Errorf("voice: failed to read segment %d: %w", i, readErr))
				return
			}

			if i == 0 {
				var parseErr error
				wavFormat, parseErr = parseWAVHeader(data)
				if parseErr != nil {
					writer.CloseWithError(fmt.Errorf("voice: %w", parseErr))
					return
				}
			}

			segmentBuffer = append(segmentBuffer, data)
		}

		// Write first segment completely (includes WAV header)
		if _, err = writer.Write(segmentBuffer[0]); err != nil {
			writer.CloseWithError(err)
			return
		}

		// Write subsequent segments with pause and crossfade
		for i := 1; i < len(segmentBuffer); i++ {
			if ds.Format == "wav" {
				prevRaw := segmentBuffer[i-1][wavFormat.dataOffset:]
				currentRaw := segmentBuffer[i][wavFormat.dataOffset:]

				toWrite := ds.streamWAVSegment(prevRaw, currentRaw, wavFormat)

				if _, err = writer.Write(toWrite); err != nil {
					writer.CloseWithError(err)
					return
				}
			} else {
				if _, err = writer.Write(segmentBuffer[i]); err != nil {
					writer.CloseWithError(err)
					return
				}
			}
		}
	}()

	return reader, nil
}

// streamWAVSegment processes a single WAV segment with pause and crossfade.
func (ds *DialogueSynthesizer) streamWAVSegment(prevRaw, currentRaw []byte, wavFormat wavInfo) []byte {
	pauseMs := ds.PauseMs
	pauseSamples := (pauseMs * wavFormat.sampleRate * wavFormat.numChannels) / 1000
	pauseBytes := pauseSamples * wavFormat.bytesPerSample
	silence := make([]byte, pauseBytes)

	var toWrite []byte
	if ds.CrossfadeMs > 0 && len(prevRaw) > 0 && len(currentRaw) > 0 {
		crossfadeBytes := min(
			(ds.CrossfadeMs*wavFormat.sampleRate*wavFormat.bytesPerSample*wavFormat.numChannels)/1000,
			len(prevRaw)/4,
			len(currentRaw)/4,
		)
		crossfadeRegion := crossfadeWAVEqualPower(prevRaw[len(prevRaw)-crossfadeBytes:], currentRaw[:crossfadeBytes], wavFormat, ds.CrossfadeMs)
		toWrite = make([]byte, len(silence)+len(crossfadeRegion)+len(currentRaw)-crossfadeBytes/2)
		copy(toWrite, silence)
		copy(toWrite[len(silence):], crossfadeRegion)
		copy(toWrite[len(silence)+len(crossfadeRegion):], currentRaw[crossfadeBytes/2:])
	} else {
		toWrite = make([]byte, len(silence)+len(currentRaw))
		copy(toWrite, silence)
		copy(toWrite[len(silence):], currentRaw)
	}
	return toWrite
}

// StreamDialogueParallel generates audio for segments in parallel and streams them in order.
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
				results <- result{index: idx, err: fmt.Errorf("voice: failed to stream segment %d: %w", idx, err)}
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

		var (
			prevRawAudio []byte
			wavFormat    wavInfo
			err          error
		)

		for i, data := range orderedData {
			if ds.Format == "wav" {
				if i == 0 {
					wavFormat, err = parseWAVHeader(data)
					if err != nil {
						writer.CloseWithError(fmt.Errorf("voice: %w", err))
						return
					}
					if _, err = writer.Write(data); err != nil {
						writer.CloseWithError(err)
						return
					}
					prevRawAudio = data[wavFormat.dataOffset:]
					continue
				}

				currentRawAudio := data[wavFormat.dataOffset:]

				toWrite := ds.streamWAVSegment(prevRawAudio, currentRawAudio, wavFormat)

				if _, err = writer.Write(toWrite); err != nil {
					writer.CloseWithError(err)
					return
				}
				prevRawAudio = currentRawAudio
			} else {
				if _, err = writer.Write(data); err != nil {
					writer.CloseWithError(err)
					return
				}
			}
		}
	}()

	return reader, nil
}

// crossfadeWAVEqualPower applies equal-power crossfade with zero-crossing optimization.
// Uses sin/cos curves for constant perceived loudness during transitions.
// Searches for zero-crossings near the splice point to minimize phase cancellation clicks.
func crossfadeWAVEqualPower(prev, next []byte, info wavInfo, durationMs int) []byte {
	if len(prev) == 0 || len(next) == 0 || durationMs <= 0 {
		return append(prev, next...)
	}

	bytesPerFrame := info.bytesPerSample * info.numChannels

	crossfadeBytes := (durationMs * info.sampleRate * bytesPerFrame) / 1000
	if crossfadeBytes > len(prev)/2 {
		crossfadeBytes = len(prev) / 2
	}
	if crossfadeBytes > len(next)/2 {
		crossfadeBytes = len(next) / 2
	}
	crossfadeBytes = (crossfadeBytes / bytesPerFrame) * bytesPerFrame

	if crossfadeBytes < bytesPerFrame {
		return append(prev, next...)
	}

	searchWindow := min((5*info.sampleRate*bytesPerFrame)/1000, crossfadeBytes)
	searchWindow = (searchWindow / bytesPerFrame) * bytesPerFrame

	bestOffset := crossfadeBytes
	if info.bitsPerSample == 16 {
		minAmplitude := math.MaxFloat64
		for offset := range searchWindow / bytesPerFrame {
			for ch := range info.numChannels {
				pos := len(prev) - crossfadeBytes + offset*bytesPerFrame + ch*info.bytesPerSample
				if pos+1 < len(prev) {
					val := float64(int16(binary.LittleEndian.Uint16(prev[pos : pos+2])))
					amp := math.Abs(val)
					if amp < minAmplitude {
						minAmplitude = amp
						bestOffset = crossfadeBytes - offset*bytesPerFrame
					}
				}
			}
		}
	}

	crossfadeBytes = (bestOffset / bytesPerFrame) * bytesPerFrame
	if crossfadeBytes < bytesPerFrame {
		crossfadeBytes = bytesPerFrame
	}

	output := make([]byte, len(prev)+len(next)-crossfadeBytes)
	copy(output, prev[:len(prev)-crossfadeBytes])

	crossfadeStart := len(prev) - crossfadeBytes
	for i := range crossfadeBytes / bytesPerFrame {
		t := float64(i*bytesPerFrame) / float64(crossfadeBytes)
		gainA := math.Cos((math.Pi / 2) * t)
		gainB := math.Sin((math.Pi / 2) * t)

		for ch := range info.numChannels {
			bytePos := i*bytesPerFrame + ch*info.bytesPerSample

			var mixed float64
			if info.bitsPerSample == 16 {
				prevSample := float64(int16(binary.LittleEndian.Uint16(prev[len(prev)-crossfadeBytes+bytePos:])))
				nextSample := float64(int16(binary.LittleEndian.Uint16(next[bytePos:])))
				mixed = (prevSample/32768.0)*gainA + (nextSample/32768.0)*gainB
				if mixed > 1.0 {
					mixed = 1.0
				} else if mixed < -1.0 {
					mixed = -1.0
				}
				binary.LittleEndian.PutUint16(output[crossfadeStart+bytePos:], uint16(int16(mixed*32767)))
			}
		}
	}

	copy(output[crossfadeStart+crossfadeBytes:], next[crossfadeBytes:])

	return output
}
