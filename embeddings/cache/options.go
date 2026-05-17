package cache

import (
	"log/slog"
	"time"
)

type cacheConfig struct {
	maxEntries int
	ttl        time.Duration
	logger     *slog.Logger
}

// CacheOption configures a cache backend.
type CacheOption func(*cacheConfig)

// WithMaxEntries sets the maximum number of cached entries (LRU eviction).
func WithMaxEntries(n int) CacheOption {
	return func(cfg *cacheConfig) {
		if n > 0 {
			cfg.maxEntries = n
		}
	}
}

// WithTTL sets the time-to-live for cache entries.
func WithTTL(ttl time.Duration) CacheOption {
	return func(cfg *cacheConfig) {
		if ttl > 0 {
			cfg.ttl = ttl
		}
	}
}

// WithCacheLogger sets a custom logger for cache operations.
func WithCacheLogger(logger *slog.Logger) CacheOption {
	return func(cfg *cacheConfig) {
		if logger != nil {
			cfg.logger = logger
		}
	}
}
