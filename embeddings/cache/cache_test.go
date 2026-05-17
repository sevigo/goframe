package cache

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sevigo/goframe/embeddings"
)

type fakeEmbedder struct {
	calls      atomic.Int64
	imageCalls atomic.Int64
	dim        int
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

func (f *fakeEmbedder) EmbedImages(_ context.Context, images []ImageData) ([][]float32, error) {
	f.imageCalls.Add(int64(len(images)))
	result := make([][]float32, len(images))
	for i := range images {
		result[i] = []float32{0.7, 0.8, 0.9}
	}
	return result, nil
}

func (f *fakeEmbedder) EmbedImage(ctx context.Context, image ImageData) ([]float32, error) {
	imgs, err := f.EmbedImages(ctx, []ImageData{image})
	if err != nil {
		return nil, err
	}
	return imgs[0], nil
}

var _ embeddings.ImageEmbedder = (*fakeEmbedder)(nil)
var _ embeddings.EmbedderWithOptions = (*fakeEmbedder)(nil)

func TestCacheKeyHash(t *testing.T) {
	k1 := CacheKey{Provider: "ollama", Model: "nomic", Text: "hello"}
	k2 := CacheKey{Provider: "ollama", Model: "nomic", Text: "hello"}
	k3 := CacheKey{Provider: "openai", Model: "nomic", Text: "hello"}
	k4 := CacheKey{Provider: "ollama", Model: "nomic", Text: "world"}
	k5 := CacheKey{Provider: "ollama", Model: "nomic", Dimensions: 256, Text: "hello"}

	assert.Equal(t, k1.String(), k2.String(), "same keys must produce same hash")
	assert.NotEqual(t, k1.String(), k3.String(), "different providers must produce different hashes")
	assert.NotEqual(t, k1.String(), k4.String(), "different texts must produce different hashes")
	assert.NotEqual(t, k1.String(), k5.String(), "different dimensions must produce different hashes")
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

func TestMemoryCacheTTLExpiration(t *testing.T) {
	c := NewMemoryCache(WithMaxEntries(100), WithTTL(100*time.Millisecond))
	ctx := context.Background()

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	vec, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.True(t, ok)
	assert.Equal(t, []float32{1.0}, vec)

	time.Sleep(150 * time.Millisecond)

	_, ok = c.Get(ctx, CacheKey{Text: "a"})
	assert.False(t, ok, "entry should be expired after TTL")
}

func TestMemoryCacheTTLNotExpired(t *testing.T) {
	c := NewMemoryCache(WithMaxEntries(100), WithTTL(1*time.Second))
	ctx := context.Background()

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	vec, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.True(t, ok)
	assert.Equal(t, []float32{1.0}, vec)
}

func TestMemoryCacheNoTTL(t *testing.T) {
	c := NewMemoryCache(WithMaxEntries(100))
	ctx := context.Background()

	c.Set(ctx, CacheKey{Text: "a"}, []float32{1.0})
	_, ok := c.Get(ctx, CacheKey{Text: "a"})
	assert.True(t, ok, "without TTL, entries should never expire")
}

func TestImageCacheKey(t *testing.T) {
	img1 := ImageData{Data: []byte("hello"), MimeType: "image/png"}
	img2 := ImageData{Data: []byte("hello"), MimeType: "image/png"}
	img3 := ImageData{Data: []byte("world"), MimeType: "image/png"}

	k1 := ImageCacheKey("gemini", "gemini-embedding-001", img1)
	k2 := ImageCacheKey("gemini", "gemini-embedding-001", img2)
	k3 := ImageCacheKey("gemini", "gemini-embedding-001", img3)

	assert.Equal(t, k1.String(), k2.String(), "same image data must produce same key")
	assert.NotEqual(t, k1.String(), k3.String(), "different image data must produce different keys")
}

func TestCachedEmbedderImageHit(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	var _ embeddings.ImageEmbedder = cached

	ctx := context.Background()
	img := ImageData{Data: []byte("test-image"), MimeType: "image/png"}

	vec1, err := cached.EmbedImage(ctx, img)
	require.NoError(t, err)
	assert.Equal(t, int64(1), fake.imageCalls.Load())

	vec2, err := cached.EmbedImage(ctx, img)
	require.NoError(t, err)
	assert.Equal(t, int64(1), fake.imageCalls.Load(), "should be cached, no extra calls")
	assert.Equal(t, vec1, vec2)
}

func TestCachedEmbedderImagesPartialHit(t *testing.T) {
	fake := &fakeEmbedder{dim: 3}
	cached, err := NewCachedEmbedder(fake,
		WithProviderName("test"),
		WithModelName("fake"),
	)
	require.NoError(t, err)

	ctx := context.Background()
	imgA := ImageData{Data: []byte("image-a"), MimeType: "image/png"}
	imgB := ImageData{Data: []byte("image-b"), MimeType: "image/png"}

	_, err = cached.EmbedImage(ctx, imgA)
	require.NoError(t, err)
	assert.Equal(t, int64(1), fake.imageCalls.Load())

	_, err = cached.EmbedImages(ctx, []ImageData{imgA, imgB})
	require.NoError(t, err)
	assert.Equal(t, int64(2), fake.imageCalls.Load(), "only imgB should miss cache")
}

func TestCachedEmbedderImageNoImpl(t *testing.T) {
	type embedderOnly struct {
		embeddings.Embedder
	}

	inner := embedderOnly{}
	cached, err := NewCachedEmbedder(&inner)
	require.NoError(t, err)

	ctx := context.Background()
	img := ImageData{Data: []byte("test"), MimeType: "image/png"}

	_, err = cached.EmbedImage(ctx, img)
	require.Error(t, err, "should error when inner does not implement ImageEmbedder")

	_, err = cached.EmbedImages(ctx, []ImageData{img})
	require.Error(t, err, "should error when inner does not implement ImageEmbedder")
}
