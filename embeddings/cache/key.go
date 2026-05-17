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
}

// Hash returns a stable SHA-256 hash of the cache key for map lookups.
func (k CacheKey) Hash() string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%t:%s", k.Provider, k.Model, k.Dimensions, k.Truncate, k.Text)))
	return hex.EncodeToString(h[:])
}
