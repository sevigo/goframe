package textsplitter

import (
	"fmt"
	"strings"
	"unicode"

	model "github.com/sevigo/goframe/schema"
)

// ValidateChunkingOptions validates the provided chunking options for correctness.
func (c *CodeAwareTextSplitter) ValidateChunkingOptions(opts *model.CodeChunkingOptions) error {
	if opts == nil {
		return nil
	}

	if err := c.validateChunkSize(opts.ChunkSize); err != nil {
		return err
	}

	return c.validateOverlapTokens(opts.OverlapTokens, opts.ChunkSize)
}

func (c *CodeAwareTextSplitter) validateChunkSize(chunkSize int) error {
	if chunkSize < 0 {
		return fmt.Errorf("%w: chunk size cannot be negative: %d", ErrInvalidChunkSize, chunkSize)
	}

	if chunkSize > c.maxChunkSize {
		return fmt.Errorf("%w: chunk size too large: %d (max: %d)", ErrInvalidChunkSize, chunkSize, c.maxChunkSize)
	}

	return nil
}

func (c *CodeAwareTextSplitter) validateOverlapTokens(overlapTokens, chunkSize int) error {
	if overlapTokens < 0 {
		return fmt.Errorf("%w: overlap tokens cannot be negative: %d", ErrInvalidChunkSize, overlapTokens)
	}

	if chunkSize > 0 && overlapTokens >= chunkSize {
		return fmt.Errorf("%w: overlap tokens (%d) must be less than chunk size (%d)",
			ErrInvalidChunkSize, overlapTokens, chunkSize)
	}

	return nil
}

// isValidChunk validates that a chunk meets basic requirements.
func (c *CodeAwareTextSplitter) isValidChunk(chunk model.CodeChunk) bool {
	trimmed := strings.TrimSpace(chunk.Content)

	if len(trimmed) < c.minChunkSize {
		return false
	}

	return c.hasSignificantContent(trimmed)
}

// hasSignificantContent checks if content has meaningful text.
func (c *CodeAwareTextSplitter) hasSignificantContent(content string) bool {
	if len(strings.TrimSpace(content)) < c.minChunkSize {
		return false
	}

	significantChars, totalNonWhitespaceChars := c.analyzeContentCharacters(content)

	if totalNonWhitespaceChars == 0 {
		return false
	}

	significanceRatio := float64(significantChars) / float64(totalNonWhitespaceChars)
	return significanceRatio >= minSignificanceRatio && significantChars >= minSignificantChars
}

func (c *CodeAwareTextSplitter) analyzeContentCharacters(content string) (int, int) {
	var totalNonWhitespaceChars, significantChars = 0, 0

	for _, r := range content {
		if !unicode.IsSpace(r) {
			totalNonWhitespaceChars++
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				significantChars++
			}
		}
	}

	return significantChars, totalNonWhitespaceChars
}
