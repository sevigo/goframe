// Package embeddings provides interfaces and utilities for text and image embedding.
// Embeddings are vector representations that capture semantic meaning for use in
// similarity search and RAG applications.
//
// The Embedder interface covers text embeddings, while ImageEmbedder extends
// support to multimodal embedding models that can embed images into the same
// vector space as text (e.g., Google's Gemini Embedding models, or OpenRouter's
// multimodal embedding endpoints).
package embeddings

import (
	"context"
	"errors"
	"strings"
)

// Embedder is the interface for embedding providers.
// Implementations convert text into dense vector representations.
type Embedder interface {
	// EmbedDocuments generates embeddings for multiple documents.
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedQuery generates an embedding for a single query.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	// EmbedQueries generates embeddings for multiple queries.
	EmbedQueries(ctx context.Context, texts []string) ([][]float32, error)
	// GetDimension returns the dimension of the embeddings.
	GetDimension(ctx context.Context) (int, error)
}

// EmbedderWithOptions is an optional interface that embedders can implement
// to support additional embedding options like truncate and dimensions.
type EmbedderWithOptions interface {
	Embedder
	// EmbedDocumentsWithOpts generates embeddings with additional options.
	EmbedDocumentsWithOpts(ctx context.Context, texts []string, opts EmbeddingOptions) ([][]float32, error)
	// EmbedQueryWithOpts generates an embedding with additional options.
	EmbedQueryWithOpts(ctx context.Context, text string, opts EmbeddingOptions) ([]float32, error)
}

// ImageData represents an image for embedding, using raw bytes and a MIME type.
type ImageData struct {
	// Data is the raw image bytes (not base64-encoded).
	Data []byte
	// MimeType is the MIME type (e.g., "image/png", "image/jpeg").
	MimeType string
}

// ImageEmbedder is the interface for providers that can generate embeddings
// from images. This enables multimodal retrieval where images are embedded
// into the same vector space as text.
type ImageEmbedder interface {
	// EmbedImages generates embeddings for multiple images.
	EmbedImages(ctx context.Context, images []ImageData) ([][]float32, error)
	// EmbedImage generates an embedding for a single image.
	EmbedImage(ctx context.Context, image ImageData) ([]float32, error)
}

// EmbeddingOptions contains options for embedding requests.
type EmbeddingOptions struct {
	// Truncate truncates the input to fit the model's max sequence length.
	Truncate bool
	// Dimensions truncates the output embedding to the specified dimension.
	Dimensions int
}

// EmbedderImpl wraps an Embedder with preprocessing and batching capabilities.
// It adds support for query/document prefixes and batch processing.
type EmbedderImpl struct {
	client Embedder
	opts   options
}

// ErrEmptyText is returned when an empty text is provided for embedding.
var ErrEmptyText = errors.New("text cannot be empty")

// NewEmbedder creates a new EmbedderImpl that wraps the given client.
// It adds preprocessing (prefixes, newline stripping) and batching.
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

	if client == nil {
		return nil, errors.New("client cannot be nil")
	}

	if _, ok := client.(*EmbedderImpl); ok {
		return nil, errors.New("cannot wrap an already-wrapped EmbedderImpl")
	}

	return &EmbedderImpl{
		client: client,
		opts:   embedderOpts,
	}, nil
}

// EmbedQuery generates an embedding for a single query with preprocessing.
func (e *EmbedderImpl) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmptyText
	}
	processedText := e.opts.QueryPrefix + e.preprocessText(text)
	return e.client.EmbedQuery(ctx, processedText)
}

// EmbedQueries generates embeddings for multiple queries with batching.
func (e *EmbedderImpl) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	allEmbeddings := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += e.opts.BatchSize {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		end := i + e.opts.BatchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		processedBatch := make([]string, len(batch))
		for j, text := range batch {
			processedBatch[j] = e.opts.QueryPrefix + e.preprocessText(text)
		}

		batchEmbeddings, err := e.client.EmbedQueries(ctx, processedBatch)
		if err != nil {
			return nil, err
		}
		allEmbeddings = append(allEmbeddings, batchEmbeddings...)
	}

	return allEmbeddings, nil
}

// EmbedDocuments generates embeddings for multiple documents with batching.
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
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

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

// GetDimension returns the dimension of the embeddings.
func (e *EmbedderImpl) GetDimension(ctx context.Context) (int, error) {
	return e.client.GetDimension(ctx)
}

func (e *EmbedderImpl) preprocessText(text string) string {
	if e.opts.StripNewLines {
		return strings.ReplaceAll(text, "\n", " ")
	}
	return text
}
