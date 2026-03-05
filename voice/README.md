# Voice Generation Package

The `voice` package provides a modular Text-to-Speech (TTS) interface for generating audio from text.

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

    err = os.WriteFile("output.mp3", audio.Data, 0644)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Audio saved to output.mp3")
}
```

### Local Kokoro Container

To use a local OpenAI-compatible API like Kokoro-FastAPI-CPU:

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
    // Kokoro runs locally on port 8880 by default
    synthesizer, err := openai.NewSynthesizer(
        openai.WithBaseURL("http://localhost:8880/v1"),
        openai.WithModel("kokoro"),
        openai.WithVoice("af_bella"),
        openai.WithFormat("wav"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // No API key needed for local server
    audio, err := synthesizer.Synthesize(context.Background(), "Hello from local TTS!")
    if err != nil {
        log.Fatal(err)
    }

    err = os.WriteFile("output.wav", audio.Data, 0644)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Audio saved to output.wav")
}
```

### Per-Request Options

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
| `WithHTTPClient(client)` | Custom HTTP client | Default shared client |
| `WithLogger(logger)` | Custom logger | `slog.Default()` |

## Supported Voices (OpenAI)

- `alloy`
- `echo`
- `fable`
- `onyx`
- `nova`
- `shimmer`

## Supported Formats (OpenAI)

- `mp3` (default)
- `opus`
- `aac`
- `flac`
- `wav`
- `pcm`