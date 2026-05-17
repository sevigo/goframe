// Package cache provides an in-memory caching layer for embedding vectors.
// It wraps an [embeddings.Embedder] and avoids redundant network calls by
// caching vectors keyed by provider, model, options, and text content.
//
// Use [NewCachedEmbedder] to wrap any existing embedder with caching:
//
//	cached, _ := cache.NewCachedEmbedder(ollamaLLM,
//	    cache.WithProviderName("ollama"),
//	    cache.WithModelName("nomic-embed-text"),
//	)
//	embedder, _ := embeddings.NewEmbedder(cached)
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CacheKey uniquely identifies a specific embedding result.
type CacheKey struct {
	Provider   string
	Model      string
	Dimensions int
	Truncate   bool
	Text       string
	ImageHash  string
}

// String returns a deterministic string representation for map lookups.
func (k CacheKey) String() string {
	if k.ImageHash != "" {
		return fmt.Sprintf("%s:%s:%d:%t:img:%s", k.Provider, k.Model, k.Dimensions, k.Truncate, k.ImageHash)
	}
	return fmt.Sprintf("%s:%s:%d:%t:%s", k.Provider, k.Model, k.Dimensions, k.Truncate, k.Text)
}

// ImageCacheKey creates a CacheKey for an image embedding.
// It hashes the image data for the cache key instead of storing raw bytes.
func ImageCacheKey(provider, model string, img ImageData) CacheKey {
	h := sha256.Sum256(img.Data)
	return CacheKey{
		Provider:  provider,
		Model:     model,
		ImageHash: hex.EncodeToString(h[:]),
	}
}
