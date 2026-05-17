package code

import (
	"context"
	"errors"
	"math"

	"github.com/sevigo/goframe/schema"
)

// CodeSparseProvider generates sparse vectors from source code.
type CodeSparseProvider struct {
	tokenizer *Tokenizer
}

// NewCodeSparseProvider creates a new code-aware sparse vector provider.
func NewCodeSparseProvider() *CodeSparseProvider {
	return &CodeSparseProvider{
		tokenizer: NewTokenizer(),
	}
}

// GenerateSparseVector produces a sparse vector from text using code-aware tokenization.
func (p *CodeSparseProvider) GenerateSparseVector(ctx context.Context, text string) (*schema.SparseVector, error) {
	if text == "" {
		return nil, errors.New("text cannot be empty")
	}

	tokens := p.tokenizer.Tokenize(text)
	if len(tokens) == 0 {
		return nil, errors.New("no valid tokens generated")
	}

	tokenCounts := make(map[uint32]float32)
	for _, token := range tokens {
		idx := hashToken(token)
		tokenCounts[idx] += 1.0
	}

	var normSq float64
	for _, count := range tokenCounts {
		normSq += float64(count) * float64(count)
	}
	norm := math.Sqrt(normSq)

	if norm <= 0 {
		return nil, errors.New("invalid normalization")
	}

	indices := make([]uint32, 0, len(tokenCounts))
	values := make([]float32, 0, len(tokenCounts))
	for id, count := range tokenCounts {
		indices = append(indices, id)
		values = append(values, float32(float64(count)/norm))
	}

	return &schema.SparseVector{
		Indices: indices,
		Values:  values,
	}, nil
}
