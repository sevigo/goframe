package embeddings

import (
	"context"
	"errors"
	"strings"
)

type Embedder interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	EmbedQueries(ctx context.Context, texts []string) ([][]float32, error)
	GetDimension(ctx context.Context) (int, error)
}

type EmbedderImpl struct {
	client Embedder
	opts   options
}

var ErrEmptyText = errors.New("text cannot be empty")

func NewEmbedder(client Embedder, opts ...Option) (Embedder, error) {
	embedderOpts := options{
		StripNewLines:  true,
		BatchSize:      32,
		QueryPrefix:    "query: ",
		DocumentPrefix: "passage: ",
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
	processedText := e.opts.QueryPrefix + e.preprocessText(text)
	return e.client.EmbedQuery(ctx, processedText)
}

func (e *EmbedderImpl) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	allEmbeddings := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += e.opts.BatchSize {
		end := i + e.opts.BatchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		processedBatch := make([]string, len(batch))
		for j, text := range batch {
			processedBatch[j] = e.opts.QueryPrefix + e.preprocessText(text)
		}

		batchEmbeddings, err := e.client.EmbedDocuments(ctx, processedBatch)
		if err != nil {
			return nil, err
		}
		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
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

	allEmbeddings := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += e.opts.BatchSize {
		end := i + e.opts.BatchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		processedBatch := make([]string, len(batch))
		for j, text := range batch {
			processedBatch[j] = e.opts.DocumentPrefix + e.preprocessText(text)
		}

		batchEmbeddings, err := e.client.EmbedDocuments(ctx, processedBatch)
		if err != nil {
			return nil, err
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
