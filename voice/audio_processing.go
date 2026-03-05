package voice

import (
	"encoding/binary"
	"math/rand/v2"
)

func generateRoomTone(numBytes, amplitude int) []byte {
	noise := make([]byte, numBytes)
	for i := 0; i < numBytes; i += 2 {
		//nolint:gosec // Room tone generation doesn't need cryptographic randomness
		val := int16(rand.IntN(amplitude*2) - amplitude)
		binary.LittleEndian.PutUint16(noise[i:i+2], uint16(val))
	}
	return noise
}
