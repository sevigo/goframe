package voice

import "strings"

// Pause multipliers for context-aware dialogue pacing.
// These values are based on natural speech patterns and provide
// natural-sounding conversation flow.
const (
	PauseMultQuestion     = 1.3
	PauseMultExclamation  = 1.2
	PauseMultEllipsis     = 1.4
	PauseMultDash         = 1.5
	PauseMultComma        = 0.7
	PauseMultShortResp    = 0.6
	PauseMultLongSentence = 1.2
	PauseMultWait         = 1.3
	PauseMultContinuation = 0.8
	PauseMultTransition   = 1.1
	PauseMultEmotional    = 1.2
	PauseMultSameSpeaker  = 0.25
	PauseMultMin          = 0.5
	PauseMultMax          = 1.8
	PauseMultInterruption = -0.3

	RoomToneAmplitude    = 30
	RoomToneMaxPauseMs   = 3000
	MaxTrailingSilenceMs = 500
	MaxLeadingSilenceMs  = 300
	MinPauseMs           = 50
)

func applyContextualPauseMultiplier(prevText, currText string, prevSpeaker, currSpeaker string) float64 {
	multiplier := 1.0

	prevEnd := strings.ToLower(strings.TrimSpace(prevText))
	currStart := strings.ToLower(strings.TrimSpace(currText))

	if prevSpeaker == currSpeaker && prevSpeaker != "" {
		multiplier = PauseMultSameSpeaker
		// Don't clamp same-speaker multipliers - they need to be very short
		return multiplier
	} else {
		switch {
		case endsWith(prevEnd, "?"):
			multiplier = PauseMultQuestion
		case endsWith(prevEnd, "!"):
			multiplier = PauseMultExclamation
		case endsWith(prevEnd, "..."):
			multiplier = PauseMultEllipsis
		case endsWith(prevEnd, "—") || endsWith(prevEnd, "--"):
			multiplier = PauseMultDash
		case endsWith(prevEnd, ","):
			multiplier = PauseMultComma
		}

		switch {
		case startsWith(currStart, "wait,") || startsWith(currStart, "wait "):
			multiplier *= PauseMultWait
		case startsWith(currStart, "but ") || startsWith(currStart, "and "):
			multiplier *= PauseMultContinuation
		case startsWith(currStart, "so,") || startsWith(currStart, "well,"):
			multiplier *= PauseMultTransition
		case startsWith(currStart, "ha") || startsWith(currStart, "oh") || startsWith(currStart, "wow"):
			multiplier *= PauseMultEmotional
		}

		wordCount := len(strings.Fields(currText))
		switch {
		case wordCount <= 3:
			multiplier *= PauseMultShortResp
		case wordCount > 20:
			multiplier *= PauseMultLongSentence
		}
	}

	if multiplier < PauseMultMin {
		multiplier = PauseMultMin
	} else if multiplier > PauseMultMax {
		multiplier = PauseMultMax
	}

	return multiplier
}
