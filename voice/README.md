# Voice Generation Package

The `voice` package provides a modular Text-to-Speech (TTS) interface for generating audio from text. It supports both buffered synthesis and streaming modes, compatible with OpenAI's API and local OpenAI-compatible servers like Kokoro-FastAPI.

## Features

- **Dual Mode Operation**: Buffered synthesis (`Synthesize`) for short texts, streaming (`Stream`) for efficient processing of longer content
- **OpenAI Compatible**: Works with OpenAI cloud API and local servers (Kokoro, etc.)
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

## Supported Voices

### OpenAI
- `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`

### Kokoro
- **American Female**: `af_bella`, `af_sarah`, `af_sky`
- **American Male**: `am_adam`, `am_michael`
- **British Female**: `bf_emma`, `bf_isabella`
- **British Male**: `bm_george`, `bm_lewis`

## Supported Formats

- `mp3` (default)
- `opus`
- `aac`
- `flac`
- `wav`
- `pcm`

## Examples

### Multi-Speaker Dialogue

Generate dialogue with multiple speakers using different voices:

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

dialogue := []voice.DialogueSegment{
    {Speaker: "Alice", Text: "Hello, how are you?"},
    {Speaker: "Bob", Text: "I'm doing great, thanks for asking!"},
}

stream, _ := dialogueSyn.StreamDialogue(ctx, dialogue)
defer stream.Close()
io.Copy(outputFile, stream)
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

### Dialogue Synthesis

See `examples/kokoro-dialogue/main.go` for multi-speaker dialogue generation:

```bash
go run ./examples/kokoro-dialogue/main.go
```

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