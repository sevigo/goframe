package chains

import (
	"context"
	"fmt"

	"github.com/sevigo/goframe/schema"
	"github.com/sevigo/goframe/vectorstores"
)

// MapFunc transforms input into documents.
type MapFunc func(ctx context.Context, input any) ([]schema.Document, error)

// ReduceFunc synthesizes multiple documents into a single output.
type ReduceFunc func(ctx context.Context, docs []schema.Document) (string, error)

// DocumentMapReduceOption configures a DocumentMapReduceChain.
type DocumentMapReduceOption func(*DocumentMapReduceChain)

// DocumentMapReduceChain implements the MapReduce pattern for document processing.
// It transforms inputs into documents via a mapper, optionally stores them
// in a vector store for persistence/caching, and synthesizes them via a reducer.
//
// The chain is safe for concurrent use as each call to Execute receives its own
// input and produces its own output. The chain struct itself is read-only after
// construction.
//
// Use cases:
//   - Code-warden: Directory summaries → Project context
//   - Wiki-warden: Page summaries → Space overview
//   - Document summarization: Section summaries → Document summary
//
// # Design Notes
//
// The vector store is intended for write-only persistence/caching of mapped documents.
// The reducer operates on the raw documents returned by the mapper, not retrieved
// from the store. If retrieval-augmented reduction is needed, implement it in a
// custom MapFunc that performs the retrieval itself.
type DocumentMapReduceChain struct {
	mapper       MapFunc
	reducer      ReduceFunc
	store        vectorstores.VectorStore
	storeOptions []vectorstores.Option // Options applied to each document during storage
	batchSize    int
}

// WithStore sets the vector store for storing mapped documents.
// The store is used for write-only persistence. Documents are stored after
// the map phase, but the reducer operates on the original mapped documents,
// not retrieved ones.
func WithStore(store vectorstores.VectorStore) DocumentMapReduceOption {
	return func(c *DocumentMapReduceChain) {
		c.store = store
	}
}

// WithStoreOptions sets options applied to each document during storage.
// These are typically metadata filters or other storage-level options.
// Note: These are applied to ALL documents, not per-document metadata.
func WithStoreOptions(opts ...vectorstores.Option) DocumentMapReduceOption {
	return func(c *DocumentMapReduceChain) {
		c.storeOptions = opts
	}
}

// WithBatchSize sets the batch size for storing documents.
// Defaults to 100 if not set or set to a value <= 0.
func WithBatchSize(size int) DocumentMapReduceOption {
	return func(c *DocumentMapReduceChain) {
		c.batchSize = size
	}
}

// NewDocumentMapReduceChain creates a new DocumentMapReduce chain.
// Returns an error if mapper or reducer is nil.
//
// The chain is safe for concurrent use.
func NewDocumentMapReduceChain(mapper MapFunc, reducer ReduceFunc, opts ...DocumentMapReduceOption) (*DocumentMapReduceChain, error) {
	if mapper == nil {
		return nil, fmt.Errorf("mapper cannot be nil")
	}
	if reducer == nil {
		return nil, fmt.Errorf("reducer cannot be nil")
	}

	chain := DocumentMapReduceChain{
		mapper:    mapper,
		reducer:   reducer,
		batchSize: 100,
	}

	for _, opt := range opts {
		opt(&chain)
	}

	return &chain, nil
}

// Execute runs the full MapReduce pipeline:
//  1. MAP phase: Transform input into documents via mapper
//  2. STORE phase: Optionally store documents in vector store
//  3. REDUCE phase: Synthesize documents into output via reducer
//
// The chain is safe for concurrent use - each call operates independently.
func (c *DocumentMapReduceChain) Execute(ctx context.Context, input any) (string, error) {
	// MAP phase
	docs, err := c.mapper(ctx, input)
	if err != nil {
		return "", fmt.Errorf("map phase failed: %w", err)
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("mapper returned no documents")
	}

	// Validate documents before processing
	if err := c.validateDocuments(docs); err != nil {
		return "", fmt.Errorf("document validation failed: %w", err)
	}

	// STORE phase - write-only persistence/caching
	if c.store != nil {
		if err := c.storeBatch(ctx, docs); err != nil {
			return "", fmt.Errorf("store phase failed: %w", err)
		}
	}

	// REDUCE phase - operates on raw mapped documents
	return c.reducer(ctx, docs)
}

// validateDocuments checks that documents have valid content.
func (c *DocumentMapReduceChain) validateDocuments(docs []schema.Document) error {
	for i, doc := range docs {
		if doc.PageContent == "" {
			return fmt.Errorf("document %d has empty content", i)
		}
	}
	return nil
}

// storeBatch stores documents in batches with context cancellation support.
func (c *DocumentMapReduceChain) storeBatch(ctx context.Context, docs []schema.Document) error {
	batchSize := c.batchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	for i := 0; i < len(docs); i += batchSize {
		// Check for context cancellation between batches
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[i:end]

		if _, err := c.store.AddDocuments(ctx, batch, c.storeOptions...); err != nil {
			return fmt.Errorf("failed to add documents batch %d-%d: %w", i, end, err)
		}
	}

	return nil
}
