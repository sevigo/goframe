package embeddings

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Embedder interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	GetDimension(ctx context.Context) (int, error)
}

type EmbedderImpl struct {
	client Embedder
	opts   options
}

func NewEmbedder(client Embedder, opts ...Option) (Embedder, error) {
	embedderOpts := options{
		StripNewLines: true,
		BatchSize:     32,
	}

	for _, opt := range opts {
		opt(&embedderOpts)
	}

	if embedderOpts.BatchSize <= 0 {
		embedderOpts.BatchSize = 32
	}

	return &EmbedderImpl{
		client: client,
		opts:   embedderOpts,
	}, nil
}

func (e *EmbedderImpl) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text cannot be empty")
	}

	processedText := e.preprocessText(text)

	embeddings, err := e.client.EmbedDocuments(ctx, []string{processedText})
	if err != nil {
		return nil, fmt.Errorf("error embedding query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, errors.New("embedding client returned no vectors for query")
	}

	return embeddings[0], nil
}

func (e *EmbedderImpl) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	processedTexts := make([]string, len(texts))
	for i, text := range texts {
		processedTexts[i] = e.preprocessText(text)
	}

	batchedTexts := batchTexts(processedTexts, e.opts.BatchSize)
	allEmbeddings := make([][]float32, 0, len(texts))

	for _, batch := range batchedTexts {
		batchEmbeddings, err := e.client.EmbedDocuments(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("error embedding document batch: %w", err)
		}
		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
}

func (e *EmbedderImpl) GetDimension(ctx context.Context) (int, error) {
	return e.client.GetDimension(ctx)
}

func (e *EmbedderImpl) preprocessText(text string) string {
	if e.opts.StripNewLines {
		return strings.ReplaceAll(text, "\n", " ")
	}
	return text
}

func batchTexts(texts []string, batchSize int) [][]string {
	if batchSize <= 0 || len(texts) <= batchSize {
		return [][]string{texts}
	}

	numBatches := (len(texts) + batchSize - 1) / batchSize
	batches := make([][]string, 0, numBatches)

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batches = append(batches, texts[i:end])
	}

	return batches
}
