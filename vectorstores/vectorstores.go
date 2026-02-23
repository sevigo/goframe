// Package vectorstores provides interfaces and implementations for vector databases.
// Vector stores are used to persist and search document embeddings for RAG applications.
package vectorstores

import (
	"context"
	"errors"
	"maps"
	"strings"

	"github.com/sevigo/goframe/embeddings"
	"github.com/sevigo/goframe/schema"
)

// ErrCollectionNotFound is returned when a collection does not exist.
var ErrCollectionNotFound = errors.New("collection not found")

// VectorStore is the interface for vector database operations.
// Implementations support document storage, similarity search, and collection management.
type VectorStore interface {
	// AddDocuments adds documents to the vector store and returns their IDs.
	AddDocuments(ctx context.Context, docs []schema.Document, options ...Option) ([]string, error)
	// SimilaritySearch returns documents similar to the query.
	SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...Option) ([]schema.Document, error)
	// SimilaritySearchBatch returns documents similar to multiple queries.
	SimilaritySearchBatch(ctx context.Context, queries []string, numDocuments int, options ...Option) ([][]schema.Document, error)
	// SimilaritySearchWithScores returns documents with similarity scores.
	SimilaritySearchWithScores(ctx context.Context, query string, numDocuments int, options ...Option) ([]DocumentWithScore, error)
	// ListCollections returns all collection names.
	ListCollections(ctx context.Context) ([]string, error)
	// DeleteCollection deletes a collection by name.
	DeleteCollection(ctx context.Context, collectionName string) error
	// DeleteDocumentsByFilter deletes documents matching the filter.
	DeleteDocumentsByFilter(ctx context.Context, filters map[string]any, options ...Option) error
}

// DocumentWithScore represents a document with its similarity score.
type DocumentWithScore struct {
	// Document is the retrieved document.
	Document schema.Document
	// Score is the similarity score (higher is more similar).
	Score float32
}

// Option configures vector store operations.
type Option func(*Options)

// Options contains configuration for vector store operations.
type Options struct {
	// Embedder overrides the default embedder for this operation.
	Embedder embeddings.Embedder
	// NameSpace is an optional namespace for the operation.
	NameSpace string
	// CollectionName specifies the collection to use.
	CollectionName string
	// ScoreThreshold filters results below this score.
	ScoreThreshold float32
	// Filters contains metadata filters for the search.
	Filters map[string]any
	// SparseQuery is the sparse vector for hybrid search.
	SparseQuery *schema.SparseVector
	// SparseQueries are sparse vectors for batch hybrid search.
	SparseQueries []*schema.SparseVector
}

// WithSparseQuery sets the sparse vector for hybrid search.
func WithSparseQuery(sparse *schema.SparseVector) Option {
	return func(opts *Options) {
		opts.SparseQuery = sparse
	}
}

// WithSparseQueries sets sparse vectors for batch hybrid search.
func WithSparseQueries(sparse []*schema.SparseVector) Option {
	return func(opts *Options) {
		opts.SparseQueries = sparse
	}
}

// WithEmbedder sets the embedder for the operation.
func WithEmbedder(embedder embeddings.Embedder) Option {
	return func(opts *Options) {
		opts.Embedder = embedder
	}
}

// WithNameSpace sets the namespace for the operation.
func WithNameSpace(namespace string) Option {
	return func(opts *Options) {
		opts.NameSpace = namespace
	}
}

// WithCollectionName sets the collection name for the operation.
func WithCollectionName(name string) Option {
	return func(opts *Options) {
		opts.CollectionName = strings.TrimSpace(name)
	}
}

// WithScoreThreshold sets the minimum score threshold for results.
func WithScoreThreshold(threshold float32) Option {
	return func(opts *Options) {
		opts.ScoreThreshold = threshold
	}
}

// WithFilters sets metadata filters for the search.
func WithFilters(filters map[string]any) Option {
	return func(opts *Options) {
		if opts.Filters == nil {
			opts.Filters = make(map[string]any)
		}
		maps.Copy(opts.Filters, filters)
	}
}

// WithFilter adds a single metadata filter for the search.
func WithFilter(key string, value any) Option {
	return func(opts *Options) {
		if opts.Filters == nil {
			opts.Filters = make(map[string]any)
		}
		opts.Filters[key] = value
	}
}

// ParseOptions creates Options from functional options.
func ParseOptions(options ...Option) Options {
	opts := Options{
		Filters: make(map[string]any),
	}
	for _, option := range options {
		option(&opts)
	}
	return opts
}
