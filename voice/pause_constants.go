package voice

import "strings"

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

	PauseMultMin = 0.5
	PauseMultMax = 1.8
)

func applyContextualPauseMultiplier(prevText, currText string) float64 {
	multiplier := 1.0

	prevEnd := strings.ToLower(strings.TrimSpace(prevText))
	currStart := strings.ToLower(strings.TrimSpace(currText))

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
