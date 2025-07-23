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
	processedText := "query: " + e.preprocessText(text)
	return e.client.EmbedQuery(ctx, processedText)
}

func (e *EmbedderImpl) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	processedTexts := make([]string, len(texts))
	for i, text := range texts {
		processedTexts[i] = "query: " + e.preprocessText(text)
	}

	return e.EmbedDocuments(ctx, processedTexts)
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
		processedTexts[i] = "passage: " + e.preprocessText(text)
	}

	return e.client.EmbedDocuments(ctx, processedTexts)
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
