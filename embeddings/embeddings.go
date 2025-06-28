package embeddings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

var ErrEmptyText = errors.New("text cannot be empty")

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

	if _, ok := client.(*EmbedderImpl); ok {
		return nil, errors.New("cannot wrap an already-wrapped EmbedderImpl")
	}

	return &EmbedderImpl{
		client: client,
		opts:   embedderOpts,
	}, nil
}

func (e *EmbedderImpl) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmptyText
	}

	processedText := e.preprocessText(text)

	return e.client.EmbedQuery(ctx, processedText)
}

func (e *EmbedderImpl) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	processedTexts := make([]string, len(texts))
	for i, text := range texts {
		processedTexts[i] = e.preprocessText(text)
	}

	batchedTexts := batchTexts(processedTexts, e.opts.BatchSize)
	batchResults := make([][][]float32, len(batchedTexts))
	errCh := make(chan error, len(batchedTexts))

	const maxConcurrent = 8
	semaphore := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for i, batch := range batchedTexts {
		wg.Add(1)
		go func(i int, batch []string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if ctx.Err() != nil {
				return
			}

			embeddings, err := e.client.EmbedDocuments(ctx, batch)
			if err != nil {
				errCh <- fmt.Errorf("error embedding batch %d: %w", i, err)
				return
			}
			batchResults[i] = embeddings
		}(i, batch)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	allEmbeddings := make([][]float32, 0, len(texts))
	for _, batch := range batchResults {
		allEmbeddings = append(allEmbeddings, batch...)
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
	if batchSize <= 0 {
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
