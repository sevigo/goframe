# OpenAI TTS Example

This example demonstrates how to use OpenAI's Text-to-Speech API with the goframe voice package.

## Features Demonstrated

1. **Basic Text-to-Speech** - Simple audio generation
2. **Voice Selection** - All 6 available OpenAI voices
3. **HD Model** - Higher quality audio synthesis
4. **Streaming** - Reduced latency for longer content
5. **Audio Formats** - MP3, Opus, AAC, FLAC

## Prerequisites

1. **OpenAI API Key** - Get it from [OpenAI Platform](https://platform.openai.com/api-keys)
2. **API Credits** - TTS API costs $15 per 1M characters (tts-1) or $30 per 1M characters (tts-1-hd)

## Running the Example

```bash
# Set your OpenAI API key
export OPENAI_API_KEY="sk-..."

# Run the example
go run main.go
```

## Output

The example generates multiple audio files:
- `openai_tts_output.mp3` - Basic example
- `openai_voice_alloy.mp3` - Alloy voice (neutral, versatile)
- `openai_voice_echo.mp3` - Echo voice (warm, engaging)
- `openai_voice_fable.mp3` - Fable voice (expressive, dramatic)
- `openai_voice_onyx.mp3` - Onyx voice (deep, authoritative)
- `openai_voice_nova.mp3` - Nova voice (energetic, friendly)
- `openai_voice_shimmer.mp3` - Shimmer voice (soft, gentle)
- `openai_hd_output.mp3` - HD quality example
- `openai_streamed.mp3` - Streaming example
- `openai_format_*.mp3` - Different audio formats

## Voice Characteristics

| Voice   | Description | Best For |
|---------|-------------|----------|
| **alloy** | Neutral, versatile | General purpose, documentaries |
| **echo** | Warm, engaging | Conversations, podcasts |
| **fable** | Expressive, dramatic | Storytelling, audiobooks |
| **onyx** | Deep, authoritative | Serious topics, news |
| **nova** | Energetic, friendly | Marketing, tutorials |
| **shimmer** | Soft, gentle | Meditation, children's content |

## Models

### tts-1 (Standard)
- **Faster** generation
- **Lower cost** ($15 per 1M characters)
- Suitable for real-time applications
- Good quality for most use cases

### tts-1-hd (High Definition)
- **Higher quality** audio
- **Higher cost** ($30 per 1M characters)
- Better for pre-recorded content
- Clearer pronunciation and prosody

## Audio Formats

| Format | Use Case | Compression |
|--------|----------|-------------|
| **mp3** | Most compatible | Good |
| **opus** | Web streaming | Best |
| **aac** | Mobile apps | Excellent |
| **flac** | Lossless archival | None |
| **wav** | Audio editing | None |
| **pcm** | Raw audio | None |

## API Usage Examples

### Basic Usage

```go
synthesizer, _ := openai.NewSynthesizer(
    openai.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
    openai.WithModel("tts-1"),
    openai.WithVoice("alloy"),
)

audio, _ := synthesizer.Synthesize(ctx, "Hello world!")
os.WriteFile("output.mp3", audio.Data, 0600)
```

### With Options

```go
audio, _ := synthesizer.Synthesize(ctx, 
    "Speaking faster now",
    voice.WithVoice("nova"),
    voice.WithModel("tts-1-hd"),
    voice.WithSpeed(1.2),
)
```

### Streaming

```go
stream, _ := synthesizer.Stream(ctx, "Long text...")
defer stream.Close()

file, _ := os.Create("output.mp3")
file.ReadFrom(stream)
```

## Cost Considerations

- **tts-1**: $0.015 per 1K characters
- **tts-1-hd**: $0.030 per 1K characters
- Example: 10,000 characters (~2 min audio) = $0.15 (tts-1) or $0.30 (tts-1-hd)

## Limitations

- Maximum text length: 4,096 characters per request
- Rate limits apply (check OpenAI dashboard)
- No word-level timestamps (use Kokoro-FastAPI for that)
- No subtitle generation built-in

## Related Examples

- `../kokoro-tts/` - Local TTS with Kokoro-FastAPI
- `../kokoro-dialogue/` - Multi-speaker dialogue synthesis
- `../kokoro-captioned-dialogue/` - Dialogue with word-level timestamps
