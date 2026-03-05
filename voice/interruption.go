package voice

func handleInterruption(prevRaw, currentRaw []byte, wavFormat wavInfo, overlapMs int) []byte {
	bytesPerFrame := wavFormat.bytesPerSample * wavFormat.numChannels
	overlapBytes := (overlapMs * wavFormat.sampleRate * bytesPerFrame) / 1000
	overlapBytes = min(overlapBytes, len(prevRaw)/4, len(currentRaw)/4)
	overlapBytes = (overlapBytes / bytesPerFrame) * bytesPerFrame

	if overlapBytes < bytesPerFrame {
		toWrite := make([]byte, len(currentRaw))
		copy(toWrite, currentRaw)
		return toWrite
	}

	crossfadeRegion := crossfadeWAVEqualPower(
		prevRaw[len(prevRaw)-overlapBytes:],
		currentRaw[:overlapBytes],
		wavFormat,
		overlapMs,
	)

	output := make([]byte, len(crossfadeRegion)+len(currentRaw)-overlapBytes/2)
	copy(output, crossfadeRegion)
	copy(output[len(crossfadeRegion):], currentRaw[overlapBytes/2:])

	return output
}
