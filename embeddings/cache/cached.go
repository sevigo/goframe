package cache

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sevigo/goframe/embeddings"
)

type cachedEmbedderConfig struct {
	provider string
	model    string
	cache    *MemoryCache
}

// Option configures a CachedEmbedder.
type Option func(*cachedEmbedderConfig)

// WithProviderName sets the provider name for cache key namespacing.
func WithProviderName(name string) Option {
	return func(cfg *cachedEmbedderConfig) {
		cfg.provider = name
	}
}

// WithModelName sets the model name for cache key namespacing.
func WithModelName(name string) Option {
	return func(cfg *cachedEmbedderConfig) {
		cfg.model = name
	}
}

// WithMemoryCache sets the in-memory cache backend.
// If not set, a default MemoryCache with 10,000 entries is created.
func WithMemoryCache(c *MemoryCache) Option {
	return func(cfg *cachedEmbedderConfig) {
		cfg.cache = c
	}
}

// CachedEmbedder wraps an Embedder with a caching layer.
// It implements both [embeddings.Embedder] and [embeddings.EmbedderWithOptions].
type CachedEmbedder struct {
	inner    embeddings.Embedder
	cache    *MemoryCache
	provider string
	model    string
	logger   *slog.Logger
}

var _ embeddings.Embedder = (*CachedEmbedder)(nil)
var _ embeddings.EmbedderWithOptions = (*CachedEmbedder)(nil)

// NewCachedEmbedder creates a new caching wrapper around the given embedder.
func NewCachedEmbedder(inner embeddings.Embedder, opts ...Option) (*CachedEmbedder, error) {
	if inner == nil {
		return nil, fmt.Errorf("cache: inner embedder cannot be nil")
	}

	cfg := &cachedEmbedderConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	c := cfg.cache
	if c == nil {
		c = NewMemoryCache()
	}

	return &CachedEmbedder{
		inner:    inner,
		cache:    c,
		provider: cfg.provider,
		model:    cfg.model,
		logger:   slog.Default().With("component", "embedding_cache"),
	}, nil
}

func (c *CachedEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	key := CacheKey{Provider: c.provider, Model: c.model, Text: text}
	if vec, ok := c.cache.Get(ctx, key); ok {
		return vec, nil
	}

	result, err := c.inner.EmbedQuery(ctx, text)
	if err != nil {
		return nil, err
	}

	c.cache.Set(ctx, key, result)
	return result, nil
}

func (c *CachedEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	var missedIndices []int
	var missedTexts []string

	for i, text := range texts {
		key := CacheKey{Provider: c.provider, Model: c.model, Text: text}
		if vec, ok := c.cache.Get(ctx, key); ok {
			results[i] = vec
		} else {
			missedIndices = append(missedIndices, i)
			missedTexts = append(missedTexts, text)
		}
	}

	if len(missedTexts) == 0 {
		return results, nil
	}

	fetched, err := c.inner.EmbedDocuments(ctx, missedTexts)
	if err != nil {
		return nil, err
	}

	for j, idx := range missedIndices {
		results[idx] = fetched[j]
		key := CacheKey{Provider: c.provider, Model: c.model, Text: texts[idx]}
		c.cache.Set(ctx, key, fetched[j])
	}

	return results, nil
}

func (c *CachedEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return c.EmbedDocuments(ctx, texts)
}

func (c *CachedEmbedder) GetDimension(ctx context.Context) (int, error) {
	return c.inner.GetDimension(ctx)
}

func (c *CachedEmbedder) EmbedQueryWithOpts(ctx context.Context, text string, opts embeddings.EmbeddingOptions) ([]float32, error) {
	key := CacheKey{Provider: c.provider, Model: c.model, Dimensions: opts.Dimensions, Truncate: opts.Truncate, Text: text}
	if vec, ok := c.cache.Get(ctx, key); ok {
		return vec, nil
	}

	innerWithOpts, ok := c.inner.(embeddings.EmbedderWithOptions)
	if !ok {
		return c.inner.EmbedQuery(ctx, text)
	}

	result, err := innerWithOpts.EmbedQueryWithOpts(ctx, text, opts)
	if err != nil {
		return nil, err
	}

	c.cache.Set(ctx, key, result)
	return result, nil
}

func (c *CachedEmbedder) EmbedDocumentsWithOpts(ctx context.Context, texts []string, opts embeddings.EmbeddingOptions) ([][]float32, error) {
	innerWithOpts, ok := c.inner.(embeddings.EmbedderWithOptions)
	if !ok {
		return c.inner.EmbedDocuments(ctx, texts)
	}

	results := make([][]float32, len(texts))
	var missedIndices []int
	var missedTexts []string

	for i, text := range texts {
		key := CacheKey{Provider: c.provider, Model: c.model, Dimensions: opts.Dimensions, Truncate: opts.Truncate, Text: text}
		if vec, found := c.cache.Get(ctx, key); found {
			results[i] = vec
		} else {
			missedIndices = append(missedIndices, i)
			missedTexts = append(missedTexts, text)
		}
	}

	if len(missedTexts) == 0 {
		return results, nil
	}

	fetched, err := innerWithOpts.EmbedDocumentsWithOpts(ctx, missedTexts, opts)
	if err != nil {
		return nil, err
	}

	for j, idx := range missedIndices {
		results[idx] = fetched[j]
		key := CacheKey{Provider: c.provider, Model: c.model, Dimensions: opts.Dimensions, Truncate: opts.Truncate, Text: texts[idx]}
		c.cache.Set(ctx, key, fetched[j])
	}

	return results, nil
}

// Stats returns current cache hit/miss/eviction statistics.
func (c *CachedEmbedder) Stats() Stats {
	return c.cache.Stats()
}
