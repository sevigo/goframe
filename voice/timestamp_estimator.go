package voice

import (
	"strings"
	"unicode"
)

// EstimateWordTimestamps distributes word timestamps evenly across the speech duration.
func EstimateWordTimestamps(text string, totalDurationMs int, leadingSilenceMs int) []WordTimestamp {
	words := tokenizeWords(text)
	if len(words) == 0 {
		return nil
	}

	speechDuration := totalDurationMs - leadingSilenceMs
	if speechDuration <= 0 {
		speechDuration = totalDurationMs
		leadingSilenceMs = 0
	}

	weights := calculateWordWeights(words)
	totalWeight := 0
	for _, w := range weights {
		totalWeight += w
	}

	timestamps := make([]WordTimestamp, 0, len(words))
	currentTime := leadingSilenceMs
	msPerWeight := float64(speechDuration) / float64(totalWeight)

	minWordDuration := 20
	availableDuration := speechDuration
	maxMinWordDuration := availableDuration / len(words) / 2
	if maxMinWordDuration > 0 && maxMinWordDuration < minWordDuration {
		minWordDuration = maxMinWordDuration
		if minWordDuration < 5 {
			minWordDuration = 5
		}
	}

	for i, word := range words {
		wordDuration := int(float64(weights[i]) * msPerWeight)
		if wordDuration < minWordDuration {
			wordDuration = minWordDuration
		}

		proposedEnd := currentTime + wordDuration
		lastEndTarget := leadingSilenceMs + speechDuration

		if i == len(words)-1 {
			proposedEnd = lastEndTarget
		} else {
			remainingWords := len(words) - i - 1
			if remainingWords > 0 {
				remainingTime := lastEndTarget - proposedEnd
				minRemaining := remainingWords * minWordDuration
				if remainingTime < minRemaining {
					wordDuration = (lastEndTarget - currentTime - minRemaining) / 2
					if wordDuration < minWordDuration {
						wordDuration = minWordDuration
					}
					proposedEnd = currentTime + wordDuration
				}
			}
		}

		if proposedEnd <= currentTime {
			proposedEnd = currentTime + minWordDuration
		}

		timestamps = append(timestamps, WordTimestamp{
			Word:    word,
			StartMs: currentTime,
			EndMs:   proposedEnd,
		})

		currentTime = proposedEnd
	}

	return timestamps
}

func tokenizeWords(text string) []string {
	var words []string
	var currentWord strings.Builder
	inWord := false

	for _, r := range text {
		if isWordChar(r) {
			currentWord.WriteRune(r)
			inWord = true
		} else if inWord {
			words = append(words, currentWord.String())
			currentWord.Reset()
			inWord = false
		}
	}

	if inWord {
		words = append(words, currentWord.String())
	}

	return words
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-'
}

func calculateWordWeights(words []string) []int {
	weights := make([]int, len(words))
	for i, word := range words {
		weights[i] = calculateWordWeight(word)
	}
	return weights
}

func calculateWordWeight(word string) int {
	weight := len(word)

	weight += countVowels(word)

	switch {
	case strings.Contains(word, "!"):
		weight += 3
	case strings.Contains(word, "?"):
		weight += 2
	case strings.Contains(word, ","):
		weight++
	case strings.Contains(word, ".") || strings.Contains(word, "..."):
		weight += 2
	}

	if isCapitalized(word) || isAllCapitalizedOrPunct(word) {
		weight++
	}

	if weight < 2 {
		weight = 2
	}

	return weight
}

func countVowels(word string) int {
	count := 0
	for _, r := range strings.ToLower(word) {
		if r == 'a' || r == 'e' || r == 'i' || r == 'o' || r == 'u' {
			count++
		}
	}
	return count
}

func isAllCapitalizedOrPunct(word string) bool {
	if len(word) == 0 {
		return false
	}
	uppercaseCount := 0
	for _, r := range word {
		if unicode.IsUpper(r) {
			uppercaseCount++
		}
	}
	return uppercaseCount > 0
}

func isCapitalized(word string) bool {
	if len(word) == 0 {
		return false
	}
	r := []rune(word)[0]
	return unicode.IsUpper(r)
}
