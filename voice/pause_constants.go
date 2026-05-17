package voice

import "strings"

// Pause multipliers for context-aware dialogue pacing.
// These values are based on natural speech patterns and provide
// natural-sounding conversation flow.
const (
	// PauseMultQuestion is the pause multiplier after a question.
	PauseMultQuestion = 1.3
	// PauseMultExclamation is the pause multiplier after an exclamation.
	PauseMultExclamation = 1.2
	// PauseMultEllipsis is the pause multiplier after an ellipsis.
	PauseMultEllipsis = 1.4
	// PauseMultDash is the pause multiplier after a dash.
	PauseMultDash = 1.5
	// PauseMultComma is the pause multiplier after a comma.
	PauseMultComma = 0.7
	// PauseMultShortResp is the pause multiplier for short responses.
	PauseMultShortResp = 0.6
	// PauseMultLongSentence is the pause multiplier for long sentences.
	PauseMultLongSentence = 1.2
	// PauseMultWait is the pause multiplier for "wait" prompts.
	PauseMultWait = 1.3
	// PauseMultContinuation is the pause multiplier for continuation words.
	PauseMultContinuation = 0.8
	// PauseMultTransition is the pause multiplier for transition words.
	PauseMultTransition = 1.1
	// PauseMultEmotional is the pause multiplier for emotional responses.
	PauseMultEmotional = 1.2
	// PauseMultSameSpeaker is the pause multiplier for same-speaker turns.
	PauseMultSameSpeaker = 0.25
	// PauseMultMin is the minimum pause multiplier.
	PauseMultMin = 0.5
	// PauseMultMax is the maximum pause multiplier.
	PauseMultMax = 1.8
	// PauseMultInterruption is the pause multiplier for interruptions (negative = overlap).
	PauseMultInterruption = -0.3

	// RoomToneAmplitude is the amplitude for room tone filler.
	RoomToneAmplitude = 30
	// RoomToneMaxPauseMs is the maximum duration of room tone filler.
	RoomToneMaxPauseMs = 3000
	// MaxTrailingSilenceMs is the maximum trailing silence to preserve.
	MaxTrailingSilenceMs = 500
	// MaxLeadingSilenceMs is the maximum leading silence to preserve.
	MaxLeadingSilenceMs = 300
	// MinPauseMs is the minimum pause duration.
	MinPauseMs = 50
)

func applyContextualPauseMultiplier(prevText, currText string, prevSpeaker, currSpeaker string) float64 {
	multiplier := 1.0

	prevEnd := strings.ToLower(strings.TrimSpace(prevText))
	currStart := strings.ToLower(strings.TrimSpace(currText))

	if prevSpeaker == currSpeaker && prevSpeaker != "" {
		multiplier = PauseMultSameSpeaker
		// Don't clamp same-speaker multipliers - they need to be very short
		return multiplier
	}

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

	if multiplier < PauseMultMin {
		multiplier = PauseMultMin
	} else if multiplier > PauseMultMax {
		multiplier = PauseMultMax
	}

	return multiplier
}
