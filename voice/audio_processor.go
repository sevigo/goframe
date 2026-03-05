package voice

import (
	"fmt"

	"github.com/gopxl/beep/v2"
)

func (ap *AudioProcessor) ProcessSegment(data []byte, normalize bool, targetVolume float64) ([]byte, error) {
	streamer, format, err := ap.DecodeWAV(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode WAV: %w", err)
	}

	if normalize {
		streamer = ap.NormalizeVolumeStreamer(streamer, targetVolume)
	}

	return ap.EncodeWAV(streamer, format)
}

func (ap *AudioProcessor) ProcessSegmentsWithPauses(segments [][]byte, pauses []int, crossfadeMs int) ([]byte, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	var firstFormat beep.Format
	streamers := make([]beep.Streamer, 0, len(segments)*2)

	for i, segment := range segments {
		streamer, format, err := ap.DecodeWAV(segment)
		if err != nil {
			return nil, fmt.Errorf("failed to decode segment %d: %w", i, err)
		}

		if i == 0 {
			firstFormat = format
		}

		if format.SampleRate != firstFormat.SampleRate {
			streamer = beep.Resample(3, format.SampleRate, firstFormat.SampleRate, streamer)
		}

		if i > 0 && i-1 < len(pauses) {
			pauseMs := pauses[i-1]
			if pauseMs > 0 {
				pauseStreamer := ap.GenerateRoomToneStreamer(pauseMs, RoomToneAmplitude, firstFormat)
				streamers = append(streamers, pauseStreamer)
			}
		}

		streamers = append(streamers, streamer)
	}

	if crossfadeMs > 0 && len(streamers) > 1 {
		return ap.processWithCrossfade(streamers, crossfadeMs, firstFormat)
	}

	combined := beep.Seq(streamers...)
	return ap.EncodeWAV(combined, firstFormat)
}

func (ap *AudioProcessor) processWithCrossfade(streamers []beep.Streamer, crossfadeMs int, format beep.Format) ([]byte, error) {
	var result []beep.Streamer

	for i, streamer := range streamers {
		if i > 0 && i-1 < len(streamers)-1 {
			prevStreamer := streamers[i-1]
			nextStreamer := streamer

			crossfade := ap.CrossfadeStreamers(prevStreamer, nextStreamer, crossfadeMs, format)
			result = append(result, crossfade)
		} else {
			result = append(result, streamer)
		}
	}

	combined := beep.Seq(result...)
	return ap.EncodeWAV(combined, format)
}

func (ap *AudioProcessor) GetAudioDuration(data []byte) (int, error) {
	streamer, format, err := ap.DecodeWAV(data)
	if err != nil {
		return 0, fmt.Errorf("failed to decode WAV for duration: %w", err)
	}
	defer func() {
		if closer, ok := streamer.(beep.StreamCloser); ok {
			closer.Close()
		}
	}()

	buffer := beep.NewBuffer(format)
	buffer.Append(streamer)

	samples := buffer.Len()
	durationMs := int(float64(samples) / float64(format.SampleRate) * 1000)

	return durationMs, nil
}

func (ap *AudioProcessor) TrimAudioToTimestamp(data []byte, endMs int) ([]byte, error) {
	streamer, format, err := ap.DecodeWAV(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode WAV for trimming: %w", err)
	}

	samplesToKeep := int(float64(endMs)/1000.0) * int(format.SampleRate)
	trimmed := beep.Take(samplesToKeep, streamer)

	return ap.EncodeWAV(trimmed, format)
}
