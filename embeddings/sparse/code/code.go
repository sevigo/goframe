package code

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

// Provider is the interface for code-aware sparse vector generation.
type Provider interface {
	GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error)
}

// NewProvider returns a new code-aware sparse vector provider.
func NewProvider() Provider {
	return NewCodeSparseProvider()
}
