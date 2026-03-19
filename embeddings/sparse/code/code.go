package code

import (
	"context"

	"github.com/sevigo/goframe/schema"
)

type Provider interface {
	GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error)
}

func NewProvider() Provider {
	return NewCodeSparseProvider()
}
