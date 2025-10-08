package textsplitter

import (
	"context"
	"fmt"
	"strings"
)

// RecursiveCharacter is a text splitter that recursively tries to split text
// using a list of separators. It aims to keep semantically related parts of
// the text together as long as possible.
type RecursiveCharacter struct {
	opts options
}

// NewRecursiveCharacter creates a new RecursiveCharacter text splitter.
func NewRecursiveCharacter(opts ...Option) *RecursiveCharacter {
	o := options{
		chunkSize:    1000, // Default chunk size in characters
		chunkOverlap: 200,  // Default overlap in characters
	}

	for _, opt := range opts {
		opt(&o)
	}

	return &RecursiveCharacter{
		opts: o,
	}
}

// SplitText splits a single text document into multiple chunks.
func (s *RecursiveCharacter) SplitText(_ context.Context, text string) ([]string, error) {
	separators := []string{"\n\n", "\n", " ", ""} // Default separators, from largest to smallest
	return s.splitTextRecursive(text, separators)
}

// splitTextRecursive is the core logic that recursively splits text.
func (s *RecursiveCharacter) splitTextRecursive(text string, separators []string) ([]string, error) {
	var finalChunks []string

	// If the text is already small enough, just return it.
	if len(text) <= s.opts.chunkSize {
		return []string{text}, nil
	}

	// Base case for the recursion: If we've run out of separators,
	// we must add the oversized text as-is and stop.
	if len(separators) == 0 {
		return []string{text}, nil
	}

	// Get the first separator to try
	separator := separators[0]
	remainingSeparators := separators[1:]

	// Split the text by the current separator
	splits := strings.Split(text, separator)
	var goodSplits []string
	currentSplit := ""

	for _, split := range splits {
		if len(split) == 0 {
			continue
		}

		// If adding the next split doesn't exceed the chunk size, merge it.
		if len(currentSplit) > 0 && len(currentSplit)+len(separator)+len(split) <= s.opts.chunkSize {
			currentSplit += separator + split
		} else {
			// Otherwise, if the current split is not empty, add it to the list.
			if len(currentSplit) > 0 {
				goodSplits = append(goodSplits, currentSplit)
			}
			// Start a new split.
			currentSplit = split
		}
	}
	// Add the last remaining split.
	if currentSplit != "" {
		goodSplits = append(goodSplits, currentSplit)
	}

	// Now, process the splits. If a split is still too large,
	// recursively split it with the remaining separators.
	for _, split := range goodSplits {
		if len(split) <= s.opts.chunkSize {
			finalChunks = append(finalChunks, split)
		} else {
			// This recursive call is now safe because of the base case at the top.
			recursiveChunks, err := s.splitTextRecursive(split, remainingSeparators)
			if err != nil {
				return nil, err
			}
			finalChunks = append(finalChunks, recursiveChunks...)
		}
	}

	// If overlap is configured, merge the chunks back together with overlap.
	if s.opts.chunkOverlap > 0 && len(finalChunks) > 1 {
		return s.mergeWithOverlap(finalChunks)
	}

	return finalChunks, nil
}

// mergeWithOverlap combines chunks, adding the specified overlap between them.
func (s *RecursiveCharacter) mergeWithOverlap(chunks []string) ([]string, error) {
	if s.opts.chunkOverlap >= s.opts.chunkSize {
		return nil, fmt.Errorf("chunk overlap (%d) must be smaller than chunk size (%d)", s.opts.chunkOverlap, s.opts.chunkSize)
	}

	var mergedChunks []string
	currentChunk := ""
	separator := "\n" // Use a newline as the separator when merging

	for i, chunk := range chunks {
		// If the current chunk is empty, start with the new chunk.
		if currentChunk == "" {
			currentChunk = chunk
			continue
		}

		// Calculate the overlap from the current chunk.
		var overlap string
		if len(currentChunk) > s.opts.chunkOverlap {
			overlap = currentChunk[len(currentChunk)-s.opts.chunkOverlap:]
		} else {
			overlap = currentChunk
		}

		// If adding the overlap and the next chunk doesn't exceed the size, merge them.
		if len(currentChunk)+len(separator)+len(chunk) <= s.opts.chunkSize {
			currentChunk += separator + chunk
		} else {
			// Finalize the current chunk and start a new one with the overlap.
			mergedChunks = append(mergedChunks, currentChunk)
			currentChunk = overlap + separator + chunk
		}

		// If this is the last chunk, add the current one.
		if i == len(chunks)-1 {
			mergedChunks = append(mergedChunks, currentChunk)
		}
	}

	return mergedChunks, nil
}
