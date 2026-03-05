package voice

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand/v2"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/wav"
)

type AudioProcessor struct {
	format beep.Format
}

func NewAudioProcessor() *AudioProcessor {
	return &AudioProcessor{}
}

func (ap *AudioProcessor) DecodeWAV(data []byte) (beep.Streamer, beep.Format, error) {
	reader := bytes.NewReader(data)
	streamer, format, err := wav.Decode(reader)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("failed to decode WAV: %w", err)
	}
	ap.format = format
	return streamer, format, nil
}

// EncodeWAV is currently not implemented due to Beep's io.WriteSeeker requirement.
// For WAV encoding, use the manual WAV processing in dialogue.go instead.
// Beep works well for decoding and real-time streaming, but encoding to memory
// requires a seekable buffer (temp file or custom implementation).
//
// FUTURE: Use Beep for MP3/OGG encoding, or when writing directly to files.
// For now, manual WAV byte manipulation (in dialogue.go) is simpler and works well.
func (ap *AudioProcessor) EncodeWAV(streamer beep.Streamer, format beep.Format) ([]byte, error) {
	return nil, fmt.Errorf("beep WAV encoding to memory not implemented - use manual WAV processing")
}

func (ap *AudioProcessor) NormalizeVolumeStreamer(streamer beep.Streamer, targetVolume float64) beep.Streamer {
	return beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n, ok := streamer.Stream(samples)
		if !ok {
			return n, false
		}

		var maxSample float64
		for i := 0; i < n; i++ {
			if math.Abs(samples[i][0]) > maxSample {
				maxSample = math.Abs(samples[i][0])
			}
			if math.Abs(samples[i][1]) > maxSample {
				maxSample = math.Abs(samples[i][1])
			}
		}

		if maxSample < 0.001 {
			return n, true
		}

		gain := targetVolume / maxSample
		for i := 0; i < n; i++ {
			samples[i][0] *= gain
			samples[i][1] *= gain
		}

		return n, true
	})
}

func (ap *AudioProcessor) CrossfadeStreamers(prev, next beep.Streamer, durationMs int, format beep.Format) beep.Streamer {
	samples := int(float64(durationMs) / 1000.0 * float64(format.SampleRate))

	prevFade := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n, ok := prev.Stream(samples)
		if !ok {
			return n, false
		}
		for i := 0; i < n; i++ {
			t := float64(i) / float64(n)
			samples[i][0] *= (1.0 - t)
			samples[i][1] *= (1.0 - t)
		}
		return n, true
	})

	nextFade := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n, ok := next.Stream(samples)
		if !ok {
			return n, false
		}
		for i := 0; i < n; i++ {
			t := float64(i) / float64(n)
			samples[i][0] *= t
			samples[i][1] *= t
		}
		return n, true
	})

	return beep.Seq(
		beep.Take(samples, prevFade),
		beep.Mix(beep.Take(samples, prevFade), beep.Take(samples, nextFade)),
		beep.Take(samples, nextFade),
	)
}

func (ap *AudioProcessor) GenerateRoomToneStreamer(durationMs int, amplitude float64, format beep.Format) beep.Streamer {
	return beep.StreamerFunc(func(s [][2]float64) (int, bool) {
		for i := range s {
			val := (rand.Float64()*2 - 1) * amplitude
			s[i][0] = val
			s[i][1] = val
		}
		return len(s), true
	})
}

func (ap *AudioProcessor) GetWAVInfo(data []byte) (wavInfo, error) {
	info := wavInfo{}
	if len(data) < 44 {
		return info, fmt.Errorf("data too short for WAV header")
	}

	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return info, fmt.Errorf("invalid WAV magic numbers")
	}

	pos := 12
	for pos < len(data)-8 {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8

		switch chunkID {
		case "fmt ":
			if pos+chunkSize > len(data) {
				return info, fmt.Errorf("fmt chunk truncated")
			}
			audioFormat := binary.LittleEndian.Uint16(data[pos : pos+2])
			if audioFormat != 1 {
				return info, fmt.Errorf("unsupported WAV format %d (only PCM supported)", audioFormat)
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

	return info, fmt.Errorf("data chunk not found")
}

func (ap *AudioProcessor) ConcatenateAudioStreamers(segments []beep.Streamer, pauses []int, format beep.Format) beep.Streamer {
	var allStreamers []beep.Streamer

	for i, segment := range segments {
		if i > 0 && i-1 < len(pauses) {
			pauseMs := pauses[i-1]
			if pauseMs > 0 {
				pauseStreamer := ap.GenerateRoomToneStreamer(pauseMs, RoomToneAmplitude, format)
				allStreamers = append(allStreamers, pauseStreamer)
			}
		}

		allStreamers = append(allStreamers, segment)
	}

	return beep.Seq(allStreamers...)
}
