// Package textsplitter provides text splitting utilities for chunking documents.
// It includes code-aware splitters that respect language syntax boundaries.
package textsplitter

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

// TextSplitter is the interface for splitting documents into smaller chunks.
// Implementations can use various strategies like character-based, token-based,
// or code-aware splitting.
type TextSplitter interface {
	// SplitDocuments splits the input documents into smaller chunks.
	SplitDocuments(ctx context.Context, docs []schema.Document) ([]schema.Document, error)
}