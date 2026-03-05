# Voice Generation Package

The `voice` package provides a modular Text-to-Speech (TTS) interface for generating audio from text. It supports both buffered synthesis and streaming modes, compatible with OpenAI's API and local OpenAI-compatible servers like Kokoro-FastAPI.

## Features

- **Dual Mode Operation**: Buffered synthesis (`Synthesize`) for short texts, streaming (`Stream`) for efficient processing of longer content
- **OpenAI Compatible**: Works with OpenAI cloud API and local servers (Kokoro, etc.)
- **Dialogue Synthesis**: Multi-speaker dialogue generation with natural conversation flow
- **Context-Aware Pacing**: Intelligent pause calculation based on dialogue context (questions, exclamations, transitions)
- **Word-Level Timestamps**: Captioned synthesis interface for precise synchronization and subtitle generation (where supported)
- **Voice Mixing**: Combine multiple voices with weighted ratios for unique character voices
- **Audio Processing**: Crossfading, volume normalization (LUFS-style), and zero-crossing optimization
- **Functional Options**: Flexible configuration using the functional options pattern
- **Context Support**: Proper cancellation and timeout handling
- **Multiple Voices**: Support for various voice identifiers per provider

## Installation

```bash
go get github.com/sevigo/goframe/voice
go get github.com/sevigo/goframe/voice/openai
```

## Usage

### Cloud OpenAI

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/sevigo/goframe/voice/openai"
)

func main() {
    synthesizer, err := openai.NewSynthesizer(
        openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
        openai.WithModel("tts-1"),
        openai.WithVoice("alloy"),
        openai.WithFormat("mp3"),
    )
    if err != nil {
        log.Fatal(err)
    }

    audio, err := synthesizer.Synthesize(context.Background(), "Hello, world!")
    if err != nil {
        log.Fatal(err)
    }

    err = os.WriteFile("output.mp3", audio.Data, 0600)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Audio saved to output.mp3")
}
```

### Local Kokoro Container

```go
package main

import (
    "context"
    "fmt"
    "io"
    "log"
    "os"

    "github.com/sevigo/goframe/voice/openai"
)

func main() {
    // Kokoro runs locally - no API key required
    synthesizer, err := openai.NewSynthesizer(
        openai.WithBaseURL("http://localhost:8880/v1"),
        openai.WithModel("kokoro"),
        openai.WithVoice("af_bella"),
        openai.WithFormat("wav"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Use streaming for efficient processing
    stream, err := synthesizer.Stream(context.Background(), "Hello from local TTS!")
    if err != nil {
        log.Fatal(err)
    }
    defer stream.Close()

    file, err := os.Create("output.wav")
    if err != nil {
        log.Fatal(err)
    }
    defer file.Close()

    written, err := io.Copy(file, stream)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Saved %d bytes to output.wav\n", written)
}
```

### Per-Request Options

Override default settings for individual requests:

```go
audio, err := synthesizer.Synthesize(ctx, text,
    voice.WithVoice("echo"),
    voice.WithModel("tts-1-hd"),
    voice.WithSpeed(1.2),
)
```

## Configuration Options

### Synthesizer Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithAPIKey(key)` | OpenAI/compatible API key | None |
| `WithBaseURL(url)` | API base URL | `https://api.openai.com/v1` |
| `WithModel(model)` | TTS model name | `tts-1` |
| `WithVoice(voice)` | Voice identifier | `alloy` |
| `WithFormat(format)` | Output format (mp3, wav, etc.) | `mp3` |
| `WithSpeed(speed)` | Speech speed multiplier (0.25-4.0) | 1.0 |
| `WithHTTPClient(client)` | Custom HTTP client | Default shared client |
| `WithLogger(logger)` | Custom structured logger | `slog.Default()` |

### DialogueSynthesizer Settings

```go
type DialogueSynthesizer struct {
    Syn             Synthesizer          // Underlying TTS engine
    VoiceMap        map[string]string    // Speaker -> voice ID mapping
    SpeedMap        map[string]float64   // Speaker -> speed multiplier
    Format          string               // Output format ("wav" recommended)
    CrossfadeMs     int                  // Crossfade duration (default: 50)
    PauseMsMin      int                  // Minimum pause (default: 200)
    PauseMsMax      int                  // Maximum pause (default: 300)
    NormalizeVolume bool                 // Enable volume normalization (default: true)
}
```

#### SpeedMap Guidelines

Recommended speed multipliers for different character types:

| Character Type | Speed | Example |
|----------------|-------|---------|
| Energetic host | 1.05-1.10 | News anchor, podcast host |
| Normal speaker | 1.00 | Most conversational voices |
| Thoughtful guest | 0.90-0.95 | Expert, professor |
| Slow narrator | 0.85-0.90 | Storyteller, documentary |

## Supported Voices

### OpenAI
- `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`

### Kokoro (Kokoro-FastAPI)
- **American Female**: `af_bella`, `af_sarah`, `af_sky`, `af_heart`, `af_nicole`
- **American Male**: `am_adam`, `am_michael`
- **British Female**: `bf_emma`, `bf_isabella`
- **British Male**: `bm_george`, `bm_lewis`

### Voice Combinations (Kokoro only)

Mix voices using `+` notation with optional weights:

```go
// Single voice
voice: "af_bella"

// Equal mix (50%/50%)
voice: "af_bella+af_sky"

// Weighted mix (67%/33%)
voice: "af_bella(2)+af_heart(1)"

// Complex mix (40%/30%/30%)
voice: "af_sky(4)+af_nicole(3)+af_heart(3)"
```

Combined voices are automatically cached for future use.

## Supported Formats

### Recommended for Dialogue Synthesis
- **`wav`** (recommended for dialogue) - Lossless, supports crossfading and normalization
- `mp3` - Not recommended for dialogue (compression artifacts at transitions)
- `opus` - Good for streaming
- `flac` - Lossless compression
- `pcm` - Raw audio, useful for streaming

### Why WAV for Dialogue?
WAV format preserves audio quality through multiple processing steps:
1. Crossfading between speakers
2. Volume normalization across different voices
3. Pause insertion with silence padding
4. Zero-crossing optimization

Other formats (MP3, Opus) apply lossy compression, which can introduce artifacts when processing the audio multiple times.

## Examples

### Multi-Speaker Dialogue with Natural Pacing

Generate dialogue with multiple speakers using different voices, with context-aware pause calculation for natural conversation flow:

```go
import "github.com/sevigo/goframe/voice"

synthesizer, _ := openai.NewSynthesizer(
    openai.WithBaseURL("http://localhost:8880/v1"),
    openai.WithModel("kokoro"),
)

dialogueSyn := voice.NewDialogueSynthesizer(synthesizer, map[string]string{
    "Alice": "af_bella",
    "Bob":   "am_adam",
})

// Customize speaker pacing
dialogueSyn.SpeedMap = map[string]float64{
    "Alice": 1.0,  // normal pace
    "Bob":   0.95, // slightly slower, thoughtful
}

dialogue := []voice.DialogueSegment{
    {Speaker: "Alice", Text: "What do you think about Tokyo?"},      // Longer pause (question)
    {Speaker: "Bob", Text: "I think it's amazing!"},                 // Medium pause (exclamation)
    {Speaker: "Alice", Text: "Yeah."},                                // Short pause (brief response)
    {Speaker: "Bob", Text: "And then we went to the station,"},       // Very short pause (continuing thought)
    {Speaker: "Alice", Text: "and the train was already there."},    // Flows naturally
}

stream, _ := dialogueSyn.StreamDialogue(ctx, dialogue)
defer stream.Close()
io.Copy(outputFile, stream)
```

#### Context-Aware Pause Calculation

The dialogue synthesizer automatically adjusts pauses based on conversational context:

| Context | Pause Multiplier | Reason |
|---------|------------------|--------|
| Questions (`?`) | 1.3x | Listener needs time to process |
| Exclamations (`!`) | 1.2x | Emotional impact |
| Ellipsis (`...`) | 1.4x | Thoughtful/pensive |
| Em dashes (`—`) | 1.5x | Dramatic interruption |
| Commas (`,`) | 0.7x | Continuing same thought |
| Short responses (≤3 words) | 0.6x | Quick back-and-forth |
| Long sentences (>20 words) | 1.2x | Complex idea, needs processing |
| Transition words ("so", "well") | 1.1x | Slight pause for transition |

**Benefits**:
- ✅ Podcast-quality natural flow
- ✅ No robotic fixed-pause timing
- ✅ Questions sound like questions
- ✅ Emotional responses have appropriate pauses
- ✅ Quick back-and-forth feels conversational

### Voice Mixing for Unique Characters

Combine multiple voices with weighted ratios:

```go
dialogueSyn := voice.NewDialogueSynthesizer(synthesizer, map[string]string{
    "Narrator": "af_bella(3)+af_heart(1)",  // 75% bella, 25% heart
    "Host":     "af_sky(2)+af_nicole(1)",   // 67% sky, 33% nicole
    "Guest":    "am_adam",                  // pure voice
})
```

### Parallel Dialogue Synthesis with Concurrency Limiting

For faster generation, synthesize all segments in parallel with controlled concurrency:

```go
// Unlimited concurrency (may overwhelm API for large dialogues)
stream, err := dialogueSyn.StreamDialogueParallel(ctx, dialogue)

// Limited concurrency (recommended for large dialogues)
// Process 10 segments at a time to avoid overwhelming the API
stream, err := dialogueSyn.StreamDialogueParallelWithLimit(ctx, dialogue, 10)
```

**Concurrency Recommendations:**
- **Kokoro-FastAPI (local)**: 5-10 concurrent requests
- **OpenAI API**: 3-5 concurrent requests (rate limits)
- **Large dialogues (>50 segments)**: Always use limiting
- **Small dialogues (<10 segments)**: Unlimited is fine

### Audio Processing Features

The `DialogueSynthesizer` includes professional audio processing:

1. **Crossfading** (default: 50ms): Smooth transitions between speakers using equal-power curves
2. **Volume Normalization**: Peak normalization to 95% for consistent loudness across voices
3. **Zero-Crossing Optimization**: Minimizes clicks at splice points
4. **Configurable Pauses**: Set custom pause ranges:

```go
dialogueSyn.PauseMsMin = 150  // minimum pause between segments
dialogueSyn.PauseMsMax = 350  // maximum pause (randomized for naturalness)
dialogueSyn.CrossfadeMs = 50  // crossfade duration
```

### Streaming with Progress

See `examples/kokoro-streaming/main.go` for a streaming example with real-time progress:

```bash
go run ./examples/kokoro-streaming/main.go
```

### Multiple Voices

See `examples/kokoro-tts/main.go` for generating audio with multiple voices:

```bash
go run ./examples/kokoro-tts/main.go
```

### Dialogue Synthesis (Podcast-Quality)

See `examples/kokoro-dialogue/main.go` for multi-speaker dialogue generation with context-aware pacing:

```bash
# Start Kokoro container first
docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest

# Run the example
go run ./examples/kokoro-dialogue/main.go

# Output: dialogue.wav with natural conversation flow
ffplay dialogue.wav
```

The dialogue example demonstrates:
- 3 different speakers with unique voice profiles
- Context-aware pacing (questions, exclamations, transitions)
- Voice mixing for character variety
- Per-speaker speed customization
- Volume normalization across speakers
- Crossfading for smooth transitions

## Testing Without API Credits

For local testing without spending API credits, use Kokoro-FastAPI:

```bash
# Start Kokoro container
docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest

# Test with local server
synthesizer, _ := openai.NewSynthesizer(
    openai.WithBaseURL("http://localhost:8880/v1"),
)
```

## Errors

| Error | Description |
|-------|-------------|
| `ErrAPIKeyRequired` | API key required when using default OpenAI endpoint |
| `openai: text cannot be empty` | Empty input text |
| `openai: request failed with status N` | HTTP error from API |
| `voice: no segments provided` | Empty dialogue segment list |
| `voice: no voice mapping for speaker X` | Speaker not found in VoiceMap |
| `voice: data too short for WAV header` | Invalid WAV audio data |
| `voice: unsupported WAV format N` | Non-PCM WAV format |

## Architecture

### Package Structure

```
voice/
├── voice.go           # Core interfaces and types
├── dialogue.go        # Dialogue synthesis with context-aware pacing
├── dialogue_test.go   # Tests for pause calculation
└── openai/
    └── openai.go      # OpenAI-compatible TTS implementation
```

### Key Types

**`voice.Synthesizer`** - Single-voice TTS interface
- `Synthesize(ctx, text, opts)` - Buffer entire audio
- `Stream(ctx, text, opts)` - Stream audio chunks

**`voice.CaptionedSynthesizer`** - TTS with word-level timestamps (where supported)
- `SynthesizeCaptioned(ctx, text, opts)` - Audio + word timing information
- `StreamCaptioned(ctx, text, opts)` - Stream audio + timing incrementally

**Note**: `DialogueSynthesizerCaptioned.StreamDialogueCaptioned()` is not yet implemented because timestamp-based pause calculation requires buffering all segments. For streaming dialogue synthesis, use `DialogueSynthesizer.StreamDialogueParallel()` instead.

**`voice.DialogueSynthesizer`** - Multi-speaker dialogue synthesis
- `StreamDialogue(ctx, segments)` - Sequential synthesis
- `StreamDialogueParallel(ctx, segments)` - Parallel synthesis (faster)
- `SynthesizeDialogue(ctx, segments)` - Return individual segments

**`voice.DialogueSegment`** - Single speaker's line
```go
type DialogueSegment struct {
    Speaker string
    Text    string
}
```

**`voice.CaptionedAudio`** - Audio with word timestamps
```go
type CaptionedAudio struct {
    Data       []byte            // Raw audio bytes
    Format     string            // Audio format
    Timestamps []WordTimestamp   // Word-level timing
    DurationMs int               // Total duration
}
```

## Advanced Features

### Word-Level Timestamps (Kokoro-FastAPI)

For providers that support captioned synthesis (currently Kokoro-FastAPI), you can get precise word timing:

```go
// Check if synthesizer supports captions
if cs, ok := synthesizer.(voice.CaptionedSynthesizer); ok {
    audio, err := cs.SynthesizeCaptioned(ctx, "Hello world", opts...)
    if err != nil {
        log.Fatal(err)
    }

    // Use timestamps for precise timing
    for _, ts := range audio.Timestamps {
        fmt.Printf("%d-%dms: %s\n", ts.StartMs, ts.EndMs, ts.Word)
    }
}
```

**Benefits**:
- ✅ Generate subtitles (SRT, VTT) automatically
- ✅ Calculate exact pauses between dialogue turns
- ✅ Analyze speech rate per speaker
- ✅ Create chapter markers for podcasts
- ✅ Perfect synchronization for background audio

**Limitation**: Captioned dialogue synthesis (`DialogueSynthesizerCaptioned`) does not support streaming. This is because calculating perfect pauses requires buffering all segments to detect built-in silence. For streaming dialogue, use heuristic-based `DialogueSynthesizer` instead.

### Subtitle Generation

```go
// Convert captions to SRT format
func generateSRT(segments []voice.CaptionedAudio) string {
    var srt strings.Builder
    index := 1
    timeOffset := 0

    for _, seg := range segments {
        for _, ts := range seg.Timestamps {
            start := formatSRTTime(timeOffset + ts.StartMs)
            end := formatSRTTime(timeOffset + ts.EndMs)
            srt.WriteString(fmt.Sprintf("%d\n%s --> %s\n%s\n\n",
                index, start, end, ts.Word))
            index++
        }
        timeOffset += seg.DurationMs
    }
    return srt.String()
}
```

## Performance

### Dialogue Synthesis Performance

- **Sequential**: Processes segments one at a time (~300-600ms per segment)
- **Parallel**: Synthesizes all segments concurrently, then assembles in order
  - 3 segments: ~3x faster than sequential
  - 10 segments: ~10x faster than sequential
  - Overhead: ~50-100ms for assembly (crossfading, pauses)
- **Captioned Synthesis**: Adds ~10-20ms overhead for timestamp generation

### Memory Usage

- **WAV format**: Requires buffering entire segment for crossfading
- **Non-WAV formats**: Streams directly without buffering
- **Peak memory**: ~2MB per minute of audio (WAV)
- **Captioned mode**: Additional ~1KB per 100 words for timestamp storage

### Optimization Tips

1. **Use `StreamDialogueParallel`** for dialogue with >3 segments
2. **Use `wav` format** for best quality with crossfading
3. **Adjust `CrossfadeMs`** (20-50ms is sufficient for most cases)
4. **Set appropriate `SpeedMap`** values to avoid re-synthesis
5. **Use captioned synthesis** for subtitle generation instead of separate processing