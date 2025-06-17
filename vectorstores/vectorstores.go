package vectorstores

import (
	"context"
	"errors"
	"maps"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/schema"
)

var (
	ErrCollectionNotFound = errors.New("collection not found")
)

type VectorStore interface {
	AddDocuments(ctx context.Context, docs []schema.Document, options ...Option) ([]string, error)
	SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...Option) ([]schema.Document, error)
	SimilaritySearchWithScores(ctx context.Context, query string, numDocuments int, options ...Option) ([]DocumentWithScore, error)
	ListCollections(ctx context.Context) ([]string, error)
}

type CollectionManager interface {
	DeleteCollection(ctx context.Context, collectionName string) error
	ListCollections(ctx context.Context) ([]schema.CollectionInfo, error)
}

type DocumentWithScore struct {
	Document schema.Document
	Score    float32
}

type Option func(*Options)

type Options struct {
	Embedder       embeddings.Embedder
	NameSpace      string
	ScoreThreshold float32
	Filters        map[string]any
}

func WithEmbedder(embedder embeddings.Embedder) Option {
	return func(opts *Options) {
		opts.Embedder = embedder
	}
}

func WithNameSpace(namespace string) Option {
	return func(opts *Options) {
		opts.NameSpace = namespace
	}
}

func WithScoreThreshold(threshold float32) Option {
	return func(opts *Options) {
		opts.ScoreThreshold = threshold
	}
}

func WithFilters(filters map[string]any) Option {
	return func(opts *Options) {
		if opts.Filters == nil {
			opts.Filters = make(map[string]any)
		}
		maps.Copy(opts.Filters, filters)
	}
}

func WithFilter(key string, value any) Option {
	return func(opts *Options) {
		if opts.Filters == nil {
			opts.Filters = make(map[string]any)
		}
		opts.Filters[key] = value
	}
}

func ParseOptions(options ...Option) Options {
	opts := Options{
		Filters: make(map[string]any),
	}
	for _, option := range options {
		option(&opts)
	}
	return opts
}
