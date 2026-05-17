package cache

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/embeddings"
)

type fakeEmbedder struct {
	calls atomic.Int64
	dim   int
}

func (f *fakeEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	f.calls.Add(int64(len(texts)))
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (f *fakeEmbedder) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	f.calls.Add(1)
	return []float32{0.4, 0.5, 0.6}, nil
}

func (f *fakeEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return f.EmbedDocuments(ctx, texts)
}

func (f *fakeEmbedder) GetDimension(_ context.Context) (int, error) {
	return f.dim, nil
}

func (f *fakeEmbedder) EmbedDocumentsWithOpts(ctx context.Context, texts []string, _ embeddings.EmbeddingOptions) ([][]float32, error) {
	return f.EmbedDocuments(ctx, texts)
}

func (f *fakeEmbedder) EmbedQueryWithOpts(ctx context.Context, text string, _ embeddings.EmbeddingOptions) ([]float32, error) {
	return f.EmbedQuery(ctx, text)
}

var _ embeddings.EmbedderWithOptions = (*fakeEmbedder)(nil)

func TestCacheKeyHash(t *testing.T) {
	k1 := CacheKey{Provider: "ollama", Model: "nomic", Text: "hello"}
	k2 := CacheKey{Provider: "ollama", Model: "nomic", Text: "hello"}
	k3 := CacheKey{Provider: "openai", Model: "nomic", Text: "hello"}
	k4 := CacheKey{Provider: "ollama", Model: "nomic", Text: "world"}
	k5 := CacheKey{Provider: "ollama", Model: "nomic", Dimensions: 256, Text: "hello"}

	assert.Equal(t, k1.Hash(), k2.Hash(), "same keys must produce same hash")
	assert.NotEqual(t, k1.Hash(), k3.Hash(), "different providers must produce different hashes")
	assert.NotEqual(t, k1.Hash(), k4.Hash(), "different texts must produce different hashes")
	assert.NotEqual(t, k1.Hash(), k5.Hash(), "different dimensions must produce different hashes")
}

func TestMemoryCacheBasic(t *testing.T) {
	c := NewMemoryCache(WithMaxEntries(3))
	ctx := context.Background()

	_, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.False(t, ok)

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	vec, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.True(t, ok)
	assert.Equal(t, []float32{1.0}, vec)

	assert.Equal(t, 1, c.Len())
}

func TestMemoryCacheLRUEviction(t *testing.T) {
	c := NewMemoryCache(WithMaxEntries(2))
	ctx := context.Background()

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	c.Set(ctx, CacheKey{Text: "b"}, []float32{2.0})
	c.Set(ctx, CacheKey{Text: "c"}, []float32{3.0})

	assert.Equal(t, 2, c.Len())

	_, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.False(t, ok, "a should be evicted (LRU)")

	_, ok = c.Get(ctx, CacheKey{Text: "b"})
	assert.True(t, ok, "b should still be present")

	_, ok = c.Get(ctx, CacheKey{Text: "c"})
	assert.True(t, ok, "c should still be present")
}

func TestMemoryCacheClear(t *testing.T) {
	c := NewMemoryCache()
	ctx := context.Background()

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	c.Set(ctx, CacheKey{Text: "b"}, []float32{2.0})
	c.Clear(ctx)

	assert.Equal(t, 0, c.Len())
}

func TestMemoryCacheStats(t *testing.T) {
	c := NewMemoryCache(WithMaxEntries(2))
	ctx := context.Background()

	c.Get(ctx, CacheKey{Text: "miss"})
	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	c.Get(ctx, CacheKey{Text: "a"})
	c.Set(ctx, CacheKey{Text: "b"}, []float32{2.0})
	c.Set(ctx, CacheKey{Text: "c"}, []float32{3.0})

	s := c.Stats()
	assert.Equal(t, int64(1), s.Hits)
	assert.Equal(t, int64(1), s.Misses)
	assert.Equal(t, int64(1), s.Evictions)
	assert.Equal(t, 2, s.Size)
}

func TestMemoryCacheDelete(t *testing.T) {
	c := NewMemoryCache()
	ctx := context.Background()

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	c.Delete(ctx, CacheKey{Text: "a"})
	_, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.False(t, ok)
}

func TestCachedEmbedderQueryHit(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	ctx := context.Background()

	vec1, err := cached.EmbedQuery(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, int64(1), fake.calls.Load())

	vec2, err := cached.EmbedQuery(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, int64(1), fake.calls.Load(), "should be cached, no extra calls")
	assert.Equal(t, vec1, vec2)
}

func TestCachedEmbedderDocumentsPartialHit(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = cached.EmbedQuery(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, int64(1), fake.calls.Load())

	docs, err := cached.EmbedDocuments(ctx, []string{"hello", "world"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), fake.calls.Load(), "only 'world' should miss cache")
	assert.Len(t, docs, 2)
}

func TestCachedEmbedderDocumentsAllHit(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = cached.EmbedDocuments(ctx, []string{"a", "b"})
	require.NoError(t, err)
	prevCalls := fake.calls.Load()

	_, err = cached.EmbedDocuments(ctx, []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, prevCalls, fake.calls.Load(), "all cache hits — no new calls")
}

func TestCachedEmbedderGetDimension(t *testing.T) {
	fake := &fakeEmbedder{dim: 768}
	cached, err := NewCachedEmbedder(fake)
	require.NoError(t, err)

	dim, err := cached.GetDimension(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 768, dim)
}

func TestCachedEmbedderNilInner(t *testing.T) {
	_, err := NewCachedEmbedder(nil)
	require.Error(t, err)
}

func TestCachedEmbedderWithOptions(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	var _ embeddings.EmbedderWithOptions = cached

	ctx := context.Background()
	opts := embeddings.EmbeddingOptions{Dimensions: 256}

	vec1, err := cached.EmbedQueryWithOpts(ctx, "hello", opts)
	require.NoError(t, err)

	vec2, err := cached.EmbedQueryWithOpts(ctx, "hello", opts)
	require.NoError(t, err)
	assert.Equal(t, vec1, vec2)
}

func TestCachedEmbedderStats(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	ctx := context.Background()
	cached.EmbedQuery(ctx, "a")
	cached.EmbedQuery(ctx, "a")

	s := cached.Stats()
	assert.Equal(t, int64(1), s.Hits)
	assert.Equal(t, int64(1), s.Misses)
}

func TestCachedEmbedderEmptyDocuments(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake)
	require.NoError(t, err)

	docs, err := cached.EmbedDocuments(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, docs)
}
