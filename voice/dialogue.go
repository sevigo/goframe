package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"strings"
	"sync"
)

// DialogueSegment represents a single speaker's line in a multi-speaker dialogue.
// Each segment specifies who is speaking (Speaker) and what they say (Text),
// which allows the synthesizer to select the appropriate voice and apply
// context-aware pacing based on the conversation flow.
type DialogueSegment struct {
	// Speaker identifies who is speaking (e.g., "Alice", "Bob", "Narrator").
	// Must match a key in the DialogueSynthesizer.VoiceMap.
	Speaker string
	// Text is the content spoken by this speaker.
	// Punctuation and sentence structure affect pause timing between segments.
	Text string
}

// DialogueSynthesizer generates audio for multi-speaker dialogues with natural conversation flow.
// It maps speakers to voice IDs and synthesizes each segment with the appropriate voice,
// then concatenates the results into a single audio stream with intelligent pacing.
//
// The synthesizer automatically adjusts pause durations based on conversational context:
//   - Questions and exclamations get longer pauses for processing time
//   - Comma-terminated segments get shorter pauses as thoughts continue
//   - Short responses get minimal pauses for quick back-and-forth
//   - Transition words ("so", "well") get appropriate pauses
//
// Audio is processed with crossfading between speakers and volume normalization
// to ensure consistent loudness across different voices.
//
// Example:
//
//	syn, _ := openai.NewSynthesizer(openai.WithBaseURL("http://localhost:8880/v1"))
//	ds := voice.NewDialogueSynthesizer(syn, map[string]string{
//	    "Alice": "af_bella",
//	    "Bob":   "am_adam",
//	})
//	ds.SpeedMap = map[string]float64{
//	    "Bob": 0.95, // Bob speaks slightly slower
//	}
//	stream, _ := ds.StreamDialogue(ctx, []voice.DialogueSegment{
//	    {Speaker: "Alice", Text: "What do you think?"},
//	    {Speaker: "Bob", Text: "I think it's great!"},
//	})
type DialogueSynthesizer struct {
	// Syn is the underlying TTS engine used to generate audio for each segment.
	Syn Synthesizer
	// VoiceMap maps speaker names to voice identifiers.
	// For OpenAI: alloy, echo, fable, onyx, nova, shimmer.
	// For Kokoro: af_bella, af_sky, am_adam, etc.
	// Use "+" for voice mixing: "af_bella(3)+af_heart(1)" for 75%/25% mix.
	VoiceMap map[string]string
	// SpeedMap maps speaker names to speech speed multipliers.
	// Values typically range from 0.8 to 1.2, where 1.0 is normal speed.
	// Speakers not in the map default to 1.0.
	// Use higher values for energetic speakers, lower for thoughtful speakers.
	SpeedMap map[string]float64
	// Format specifies the output audio format (e.g., "wav", "mp3").
	// Default is "wav" for better concatenation support and crossfade quality.
	// Note: Compressed formats (mp3, opus) may introduce artifacts during processing.
	Format string
	// CrossfadeMs specifies crossfade duration in milliseconds (default: 50).
	// Higher values (80-100ms) create smoother transitions but may reduce clarity.
	// Lower values (20-40ms) are faster but may sound abrupt on speaker changes.
	// Set to 0 to disable crossfading (useful for compressed formats).
	// Note: Crossfading requires buffering segments in memory.
	CrossfadeMs int
	// PauseMsMin specifies minimum pause duration between segments in milliseconds (default: 200).
	// This is the base pause duration before context-aware adjustments.
	// Set both PauseMsMin and PauseMsMax to 0 to disable pauses between segments.
	// Recommended: 150-250ms for natural conversation, 300-500ms for dramatic effect.
	PauseMsMin int
	// PauseMsMax specifies maximum pause duration between segments in milliseconds (default: 300).
	// A random value between PauseMsMin and PauseMsMax provides natural variation.
	// Context-aware adjustments (questions, exclamations, etc.) can extend beyond this maximum
	// by up to 2x to accommodate natural speech patterns.
	PauseMsMax int
	// NormalizeVolume enables peak volume normalization per segment (default: true).
	// Ensures consistent loudness across different speakers/voices, which is critical
	// for dialogue where different native volumes could be jarring.
	// Normalization targets 95% of maximum amplitude to avoid clipping while maintaining
	// consistent perceived volume across all speakers.
	NormalizeVolume bool
}

// NewDialogueSynthesizer creates a new dialogue synthesizer for multi-speaker audio generation.
// The synthesizer applies context-aware pacing, crossfading, and volume normalization
// to create natural-sounding dialogues.
//
// The format defaults to "wav" which supports reliable concatenation and processing.
// For dialogue synthesis, WAV is recommended over compressed formats like MP3 to avoid
// quality degradation through multiple processing steps.
//
// Default settings:
//   - CrossfadeMs: 50ms (smooth transitions between speakers)
//   - PauseMsMin: 200ms (minimum pause between segments)
//   - PauseMsMax: 300ms (maximum pause, randomized for naturalness)
//   - NormalizeVolume: true (consistent loudness across voices)
//
// Example:
//
//	ds := voice.NewDialogueSynthesizer(synthesizer, map[string]string{
//	    "Alice": "af_bella",
//	    "Bob":   "am_adam",
//	})
//	ds.SpeedMap = map[string]float64{
//	    "Alice": 1.05, // slightly faster
//	    "Bob":   0.95, // slightly slower
//	}
func NewDialogueSynthesizer(syn Synthesizer, voiceMap map[string]string, format ...string) *DialogueSynthesizer {
	f := "wav"
	if len(format) > 0 && format[0] != "" {
		f = format[0]
	}
	return &DialogueSynthesizer{
		Syn:             syn,
		VoiceMap:        voiceMap,
		Format:          f,
		CrossfadeMs:     50,
		PauseMsMin:      200,
		PauseMsMax:      300,
		NormalizeVolume: true,
	}
}

// speakerSpeed returns the speed multiplier for a speaker, defaulting to 1.0 if not specified.
// Values are clamped to the valid API range of 0.25 to 4.0.
func (ds *DialogueSynthesizer) speakerSpeed(speaker string) float64 {
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

// SynthesizeDialogue generates audio for all segments and returns individual audio files.
// This is useful when you want to process each speaker's audio separately,
// apply custom audio processing, or store segments individually.
//
// Returns a slice of Audio objects, one per segment, in the same order as the input.
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

		audio, err := ds.Syn.Synthesize(ctx, seg.Text, WithVoice(voiceID), WithFormat(ds.Format), WithSpeed(ds.speakerSpeed(seg.Speaker)))
		if err != nil {
			return nil, fmt.Errorf("voice: failed to synthesize segment %d (speaker %q): %w", i, seg.Speaker, err)
		}

		results = append(results, audio)
	}

	return results, nil
}

// wavInfo holds parsed WAV file metadata needed for audio processing.
// It is used internally to correctly apply crossfading, volume normalization,
// and pause insertion while maintaining proper audio format.
type wavInfo struct {
	sampleRate     int // samples per second (e.g., 24000, 44100)
	bitsPerSample  int // bits per sample (typically 16 for PCM)
	numChannels    int // number of audio channels (1 for mono, 2 for stereo)
	dataOffset     int // byte offset to the start of audio data
	bytesPerSample int // bytes per sample (bitsPerSample / 8)
}

// parseWAVHeader extracts audio format information from a WAV file header.
// It handles variable header sizes by parsing the RIFF chunk structure,
// supporting only PCM format (audioFormat = 1) which is the standard for
// uncompressed audio.
//
// Returns wavInfo with sample rate, bit depth, channels, and data offset,
// or an error if the WAV file is malformed or uses an unsupported format.
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
// Segments are synthesized sequentially with context-aware pauses and crossfading between speakers.
//
// The returned io.ReadCloser streams the complete dialogue audio. The caller must close
// the ReadCloser when done reading.
//
// Example:
//
//	stream, err := ds.StreamDialogue(ctx, []voice.DialogueSegment{
//	    {Speaker: "Alice", Text: "Hello?"},
//	    {Speaker: "Bob", Text: "Hi there!"},
//	})
//	defer stream.Close()
//	io.Copy(outputFile, stream)
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

			stream, synErr := ds.Syn.Stream(ctx, seg.Text, WithVoice(voiceID), WithFormat(ds.Format), WithSpeed(ds.speakerSpeed(seg.Speaker)))
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

			if ds.NormalizeVolume && ds.Format == "wav" {
				normalizeWAVVolume(data, wavFormat)
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

				// Normalize both segments before crossfade (fixes inconsistent normalization)
				normalizeWAVVolume(segmentBuffer[i-1], wavFormat)
				normalizeWAVVolume(segmentBuffer[i], wavFormat)

				prevText := segments[i-1].Text
				currText := segments[i].Text
				toWrite := ds.streamWAVSegment(prevRaw, currentRaw, wavFormat, prevText, currText)

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
// It calculates an appropriate pause duration based on the dialogue context,
// applies crossfading between segments, and returns the processed audio data.
func (ds *DialogueSynthesizer) streamWAVSegment(prevRaw, currentRaw []byte, wavFormat wavInfo, prevText, currText string) []byte {
	pauseMs := ds.calculateContextualPause(prevText, currText, ds.PauseMsMin, ds.PauseMsMax)
	pauseSamples := (pauseMs * wavFormat.sampleRate * wavFormat.numChannels) / 1000
	pauseBytes := pauseSamples * wavFormat.bytesPerSample
	silence := make([]byte, pauseBytes)

	var toWrite []byte
	if ds.CrossfadeMs > 0 && len(prevRaw) > 0 && len(currentRaw) > 0 {
		bytesPerFrame := wavFormat.bytesPerSample * wavFormat.numChannels
		crossfadeBytes := min(
			(ds.CrossfadeMs*wavFormat.sampleRate*bytesPerFrame)/1000,
			len(prevRaw)/4,
			len(currentRaw)/4,
		)
		// Align to sample boundary
		crossfadeBytes = (crossfadeBytes / bytesPerFrame) * bytesPerFrame

		if crossfadeBytes < bytesPerFrame {
			// Too small for crossfade, just concatenate
			toWrite = make([]byte, len(silence)+len(currentRaw))
			copy(toWrite, silence)
			copy(toWrite[len(silence):], currentRaw)
		} else {
			crossfadeRegion := crossfadeWAVEqualPower(prevRaw[len(prevRaw)-crossfadeBytes:], currentRaw[:crossfadeBytes], wavFormat, ds.CrossfadeMs)
			// crossfadeRegion contains: (prev_tail - crossfade) + crossfaded + (next_head starting from crossfadeBytes/2)
			// So we need: silence + crossfadeRegion + rest of currentRaw after crossfadeBytes/2
			alignedCrossfadeBytes := crossfadeBytes / 2
			alignedCrossfadeBytes = (alignedCrossfadeBytes / bytesPerFrame) * bytesPerFrame

			toWrite = make([]byte, len(silence)+len(crossfadeRegion)+len(currentRaw)-alignedCrossfadeBytes)
			copy(toWrite, silence)
			copy(toWrite[len(silence):], crossfadeRegion)
			copy(toWrite[len(silence)+len(crossfadeRegion):], currentRaw[alignedCrossfadeBytes:])
		}
	} else {
		toWrite = make([]byte, len(silence)+len(currentRaw))
		copy(toWrite, silence)
		copy(toWrite[len(silence):], currentRaw)
	}
	return toWrite
}

// parallelResult holds the output of a single parallel segment synthesis.
// It is used internally by StreamDialogueParallel to collect synthesized audio
// from concurrent goroutines and maintain segment ordering.
type parallelResult struct {
	index int    // segment index in the original dialogue
	data  []byte // synthesized audio data
	err   error  // synthesis error, if any
}

// synthesizeSegment synthesizes a single dialogue segment and sends the result to the channel.
// This is used for parallel synthesis where each segment is processed concurrently.
func (ds *DialogueSynthesizer) synthesizeSegment(ctx context.Context, idx int, s DialogueSegment, results chan<- parallelResult) {
	select {
	case <-ctx.Done():
		results <- parallelResult{index: idx, err: ctx.Err()}
		return
	default:
	}

	voiceID, ok := ds.VoiceMap[s.Speaker]
	if !ok {
		results <- parallelResult{index: idx, err: fmt.Errorf("voice: no voice mapping for speaker %q", s.Speaker)}
		return
	}

	stream, err := ds.Syn.Stream(ctx, s.Text, WithVoice(voiceID), WithFormat(ds.Format), WithSpeed(ds.speakerSpeed(s.Speaker)))
	if err != nil {
		results <- parallelResult{index: idx, err: fmt.Errorf("voice: failed to stream segment %d: %w", idx, err)}
		return
	}

	data, err := io.ReadAll(stream)
	if closeErr := stream.Close(); closeErr != nil {
		slog.Warn("Failed to close stream", "error", closeErr)
	}
	if err != nil {
		results <- parallelResult{index: idx, err: fmt.Errorf("voice: failed to read segment %d: %w", idx, err)}
		return
	}

	results <- parallelResult{index: idx, data: data}
}

// normalizeWAVVolume normalizes the volume of WAV PCM data to a target peak level.
// It scans all samples to find the peak amplitude, then applies gain to bring
// the peak to 95% of maximum (0.95 = leaving 5% headroom to avoid clipping).
//
// This ensures consistent loudness across different speakers/voices, which is
// critical for multi-speaker dialogues where native voice volumes may vary.
//
// The function modifies data in-place and only supports 16-bit PCM audio.
// For other bit depths, it returns without modification.
func normalizeWAVVolume(data []byte, info wavInfo) {
	if info.bitsPerSample != 16 || info.dataOffset >= len(data) {
		return
	}

	pcmData := data[info.dataOffset:]
	if len(pcmData) < 2 {
		return
	}

	// Find peak amplitude.
	var peak float64
	for i := 0; i+1 < len(pcmData); i += 2 {
		sample := math.Abs(float64(int16(binary.LittleEndian.Uint16(pcmData[i : i+2]))))
		if sample > peak {
			peak = sample
		}
	}

	if peak < 1.0 {
		return // silence, nothing to normalize
	}

	const targetPeak = 0.95
	gain := (targetPeak * 32767.0) / peak
	if gain >= 1.0 && gain < 1.01 {
		return // already near target, skip to avoid unnecessary processing
	}

	// Apply gain to all samples.
	for i := 0; i+1 < len(pcmData); i += 2 {
		sample := float64(int16(binary.LittleEndian.Uint16(pcmData[i : i+2])))
		normalized := sample * gain
		if normalized > 32767 {
			normalized = 32767
		} else if normalized < -32768 {
			normalized = -32768
		}
		binary.LittleEndian.PutUint16(pcmData[i:i+2], uint16(int16(normalized)))
	}
}

// writeOrderedSegments writes ordered audio segments to a pipe writer with crossfade and pause.
// This function is used by StreamDialogueParallel to write segments in the correct order
// after they have been synthesized concurrently.
func (ds *DialogueSynthesizer) writeOrderedSegments(writer *io.PipeWriter, orderedData [][]byte, segments []DialogueSegment) {
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
				if ds.NormalizeVolume {
					normalizeWAVVolume(data, wavFormat)
				}
				if _, err = writer.Write(data); err != nil {
					writer.CloseWithError(err)
					return
				}
				prevRawAudio = data[wavFormat.dataOffset:]
				continue
			}

			currentRawAudio := data[wavFormat.dataOffset:]
			if ds.NormalizeVolume {
				normalizeWAVVolume(data, wavFormat)
			}

			prevText := segments[i-1].Text
			currText := segments[i].Text
			toWrite := ds.streamWAVSegment(prevRawAudio, currentRawAudio, wavFormat, prevText, currText)

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
}

// StreamDialogueParallel generates audio for segments in parallel and streams them in order.
// This is significantly faster than sequential synthesis for dialogues with many segments,
// as all segments are synthesized concurrently, then assembled in order with proper
// crossfading and pause timing.
//
// The concurrencyLimit parameter controls how many segments are synthesized simultaneously.
// A value of 0 or negative means no limit (may overwhelm the API for large dialogues).
// Recommended: 5-10 for most APIs, adjust based on API rate limits.
// For Kokoro-FastAPI, 5-10 works well. For OpenAI, use lower values (3-5) due to rate limits.
//
// Returns a ReadCloser that streams the concatenated audio with natural transitions.
func (ds *DialogueSynthesizer) StreamDialogueParallel(ctx context.Context, segments []DialogueSegment) (io.ReadCloser, error) {
	return ds.streamDialogueParallelWithLimit(ctx, segments, 0) // 0 = no limit, use all goroutines
}

// StreamDialogueParallelWithLimit generates audio with controlled concurrency.
// Use this for large dialogues or when dealing with rate-limited APIs.
// ConcurrencyLimit of 5-10 is recommended for most use cases.
func (ds *DialogueSynthesizer) StreamDialogueParallelWithLimit(ctx context.Context, segments []DialogueSegment, concurrencyLimit int) (io.ReadCloser, error) {
	return ds.streamDialogueParallelWithLimit(ctx, segments, concurrencyLimit)
}

func (ds *DialogueSynthesizer) streamDialogueParallelWithLimit(ctx context.Context, segments []DialogueSegment, concurrencyLimit int) (io.ReadCloser, error) {
	if len(segments) == 0 {
		return nil, errors.New("voice: no segments provided")
	}

	results := make(chan parallelResult, len(segments))
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// If no limit specified or limit is invalid, process all at once
	if concurrencyLimit <= 0 {
		concurrencyLimit = len(segments)
	}

	// Create a semaphore to limit concurrency
	sem := make(chan struct{}, concurrencyLimit)

	for i, seg := range segments {
		wg.Add(1)
		go func(idx int, s DialogueSegment) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			ds.synthesizeSegment(ctx, idx, s, results)
		}(i, seg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	orderedData := make([][]byte, len(segments))
	var firstErr error
	for res := range results {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
				cancel()
			}
			continue
		}
		orderedData[res.index] = res.data
	}

	if firstErr != nil {
		return nil, firstErr
	}

	reader, writer := io.Pipe()
	go ds.writeOrderedSegments(writer, orderedData, segments)

	return reader, nil
}

// crossfadeWAVEqualPower applies equal-power crossfade between two audio segments.
// Equal-power crossfade uses sin/cos curves (instead of linear) to maintain
// constant perceived loudness during the transition, preventing the "dip"
// effect that occurs with simple linear crossfading.
//
// The function also searches for zero-crossings near the transition point
// to minimize clicks and phase cancellation artifacts.
//
// Parameters:
//   - prev: ending audio data from previous segment
//   - next: starting audio data from next segment
//   - info: WAV format information
//   - durationMs: desired crossfade duration in milliseconds
//
// Returns the crossfaded audio with smooth transition from prev to next.
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

// calculateContextualPause determines pause duration based on dialogue context.
// It analyzes the previous and current text segments to calculate an appropriate
// pause that creates natural conversation flow.
//
// Pause adjustments are based on:
//   - Punctuation: Questions (?), exclamations (!), ellipses (...), dashes (—), commas (,)
//   - Response patterns: Transition words, emotional reactions, continuation markers
//   - Speech length: Short responses get shorter pauses, long sentences get longer pauses
//
// The function applies multipliers to the base pause range and adds randomness for naturalness.
// Not security-sensitive - uses math/rand for variety in dialogue pacing.
func (ds *DialogueSynthesizer) calculateContextualPause(prevText, currText string, minMs, maxMs int) int {
	if maxMs <= minMs {
		return minMs
	}

	base := minMs
	variable := maxMs - minMs

	// Analyze previous segment's ending
	prevEnd := strings.ToLower(strings.TrimSpace(prevText))
	// Analyze current segment's beginning
	currStart := strings.ToLower(strings.TrimSpace(currText))

	// Factor 1: Punctuation-based pause adjustment
	// Questions and exclamations need longer pause for listener processing
	multiplier := 1.0
	switch {
	case endsWith(prevEnd, "?"):
		multiplier = 1.3 // Questions need 30% longer pause (processing time)
	case endsWith(prevEnd, "!"):
		multiplier = 1.2 // Exclamations: 20% longer (impact)
	case endsWith(prevEnd, "..."):
		multiplier = 1.4 // Trailing off: 40% longer (thoughtful pause)
	case endsWith(prevEnd, "—") || endsWith(prevEnd, "--"):
		multiplier = 1.5 // Interruption/continuation: 50% (dramatic)
	case endsWith(prevEnd, ","):
		multiplier = 0.7 // Comma: 30% shorter (continuing thought)
	}

	// Factor 2: Response type affects pause
	switch {
	case startsWith(currStart, "wait,") || startsWith(currStart, "wait "):
		multiplier *= 1.3 // "Wait..." needs more pause
	case startsWith(currStart, "but ") || startsWith(currStart, "and "):
		multiplier *= 0.8 // Continuing thought - shorter pause
	case startsWith(currStart, "so,") || startsWith(currStart, "well,"):
		multiplier *= 1.1 // Transition phrases - slightly longer
	case startsWith(currStart, "ha") || startsWith(currStart, "oh") || startsWith(currStart, "wow"):
		multiplier *= 1.2 // Emotional reactions - need processing time
	}

	// Factor 3: Very short responses need less pause
	wordCount := len(strings.Fields(currText))
	switch {
	case wordCount <= 3:
		multiplier *= 0.6 // "Yeah", "Right", "Okay" need quick back-and-forth
	case wordCount > 20:
		multiplier *= 1.2 // Long sentences need more pause before them (processing)
	}

	// Factor 4: Same-speaker continuations need much shorter pauses
	// This handles cases like Kenji's "Ten years." after his own long explanation
	// We don't know the speaker here, so this would need to be passed in
	// TODO: Add speaker parameter to enable same-speaker optimization

	// Clamp multiplier to reasonable range to prevent overshoot
	// Range [0.5, 1.8] ensures pauses stay proportional
	if multiplier < 0.5 {
		multiplier = 0.5
	} else if multiplier > 1.8 {
		multiplier = 1.8
	}

	// Calculate final pause with randomness for naturalness
	// Not security-sensitive - just adding variety to dialogue pacing
	pause := int(float64(base) + float64(variable)*multiplier*0.5 + float64(rand.IntN(variable/2))) //nolint:gosec // dialogue pacing doesn't need cryptographic randomness

	// Clamp to valid range
	if pause < minMs {
		pause = minMs
	}
	if pause > maxMs*2 { // Allow up to 2x max for dramatic moments
		pause = maxMs * 2
	}

	return pause
}

// endsWith checks if text ends with a specific punctuation mark.
// Whitespace is trimmed before checking. Useful for detecting punctuation patterns
// that affect dialogue pacing.
func endsWith(text, suffix string) bool {
	text = strings.TrimSpace(text)
	return strings.HasSuffix(text, suffix)
}

// startsWith checks if text starts with a specific word or prefix (case-insensitive).
// Useful for detecting speech patterns like "wait...", "but", "well," that affect timing.
func startsWith(text, prefix string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	return strings.HasPrefix(text, prefix)
}
