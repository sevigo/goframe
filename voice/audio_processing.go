package voice

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
)

func generateRoomTone(numBytes, amplitude int) []byte {
	noise := make([]byte, numBytes)
	for i := 0; i < numBytes; i += 2 {
		val := int16(rand.IntN(amplitude*2) - amplitude)
		binary.LittleEndian.PutUint16(noise[i:i+2], uint16(val))
	}
	return noise
}

func normalizeWAVRMS(data []byte, info wavInfo, targetRMS float64) {
	if info.bitsPerSample != 16 || info.dataOffset >= len(data) {
		return
	}

	pcmData := data[info.dataOffset:]
	if len(pcmData) < 2 {
		return
	}

	var sumSquares float64
	sampleCount := 0
	for i := 0; i+1 < len(pcmData); i += 2 {
		sample := float64(int16(binary.LittleEndian.Uint16(pcmData[i : i+2])))
		sumSquares += sample * sample
		sampleCount++
	}

	if sampleCount == 0 {
		return
	}

	currentRMS := math.Sqrt(sumSquares / float64(sampleCount))
	if currentRMS < 1.0 {
		return // silence, nothing to normalize
	}

	gain := targetRMS / currentRMS
	if gain > 10.0 {
		gain = 10.0
	}

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

func trimAudioToTimestamp(audio []byte, info wavInfo, endMs int) []byte {
	if endMs <= 0 || len(audio) == 0 {
		return audio
	}

	samplesToKeep := (endMs * info.sampleRate) / 1000
	bytesToKeep := samplesToKeep * info.bytesPerSample * info.numChannels
	trimOffset := info.dataOffset + bytesToKeep

	if trimOffset >= len(audio) {
		return audio
	}

	trimmed := make([]byte, trimOffset)
	copy(trimmed, audio[:trimOffset])
	return trimmed
}

func calculateDynamicCrossfade(pauseMs, baseCrossfadeMs int) int {
	if pauseMs < 0 {
		return baseCrossfadeMs + (-pauseMs)
	}

	if pauseMs > 600 {
		return min(150, baseCrossfadeMs*2)
	} else if pauseMs > 300 {
		return min(100, int(float64(baseCrossfadeMs)*1.5))
	}

	return baseCrossfadeMs
}
