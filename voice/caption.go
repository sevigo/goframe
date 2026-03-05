// Package voice provides interfaces and types for Text-to-Speech synthesis.
// It defines a modular interface that supports multiple TTS backends.
package voice

import (
	"context"
	"io"
)

// WordTimestamp represents a single word with its timing information.
// This enables precise synchronization of audio with text, useful for
// generating subtitles, chapter markers, and analyzing speech patterns.
type WordTimestamp struct {
	// Word is the text content of this segment.
	Word string
	// StartMs is the start time in milliseconds from the beginning of the audio.
	StartMs int
	// EndMs is the end time in milliseconds from the beginning of the audio.
	EndMs int
}

// CaptionedAudio represents synthesized audio with word-level timestamps.
// This provides precise timing information for each word, enabling advanced
// features like subtitle generation, speech analysis, and perfect synchronization.
type CaptionedAudio struct {
	// Data contains the raw audio bytes.
	Data []byte
	// Format specifies the audio format (e.g., "mp3", "wav", "opus").
	Format string
	// Timestamps contains word-level timing information.
	// Words are ordered chronologically as they appear in the audio.
	Timestamps []WordTimestamp
	// DurationMs is the total duration of the audio in milliseconds.
	// This is the end time of the last word plus any trailing silence.
	DurationMs int
}

// CaptionedSynthesizer extends the basic Synthesizer interface with
// timestamp-aware synthesis capabilities.
//
// Implementations that support word-level timestamps (like Kokoro-FastAPI)
// should implement this interface in addition to Synthesizer. This enables
// more sophisticated audio processing like:
//   - Exact pause calculation based on actual speech duration
//   - Automatic subtitle generation (SRT, VTT)
//   - Speech rate analysis and normalization
//   - Perfect synchronization for background music/ambience
//   - Quality control for podcast production
type CaptionedSynthesizer interface {
	Synthesizer

	// SynthesizeCaptioned generates audio from text with word-level timestamps.
	// This is similar to Synthesize but returns timing information for each word,
	// enabling precise synchronization and analysis.
	//
	// The returned CaptionedAudio contains both the audio data and timestamps.
	// Not all TTS providers support this feature.
	//
	// Example:
	//
	//	audio, err := synth.SynthesizeCaptioned(ctx, "Hello world", opts...)
	//	for _, ts := range audio.Timestamps {
	//	    fmt.Printf("%d-%dms: %s\n", ts.StartMs, ts.EndMs, ts.Word)
	//	}
	SynthesizeCaptioned(ctx context.Context, text string, opts ...Option) (*CaptionedAudio, error)

	// StreamCaptioned generates audio from text with timestamps streamed incrementally.
	// Each chunk contains a JSON object with "audio" (base64) and "timestamps" fields.
	// This is useful for long texts where you want to process audio and timing
	// information as it's generated, rather than waiting for complete synthesis.
	//
	// The returned ReadCloser streams JSON objects, one per chunk.
	//
	// Example:
	//
	//	stream, err := synth.StreamCaptioned(ctx, longText, opts...)
	//	defer stream.Close()
	//	decoder := json.NewDecoder(stream)
	//	for {
	//	    var chunk CaptionedChunk
	//	    if err := decoder.Decode(&chunk); err != nil {
	//	        if err == io.EOF { break }
	//	        return err
	//	    }
	//	    // Process chunk.Audio and chunk.Timestamps
	//	}
	StreamCaptioned(ctx context.Context, text string, opts ...Option) (io.ReadCloser, error)
}

// CaptionedChunk represents a single chunk from a captioned stream.
// It contains both audio data (base64 encoded) and word timestamps for
// incremental processing during streaming synthesis.
type CaptionedChunk struct {
	// Audio contains base64-encoded audio data for this chunk.
	Audio string `json:"audio"`
	// Timestamps contains word-level timing for words in this chunk.
	Timestamps []WordTimestamp `json:"timestamps"`
}
