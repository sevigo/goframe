package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"log/slog"
)

type entry struct {
	key        string
	vector     []float32
	accessTime time.Time
}

// MemoryCache is an in-memory LRU cache for embedding vectors.
type MemoryCache struct {
	entries    map[string]*list.Element
	lruList    *list.List
	maxEntries int
	mu         sync.RWMutex
	logger     *slog.Logger
	hits       int64
	misses     int64
	evictions  int64
}

// NewMemoryCache creates an in-memory LRU cache.
func NewMemoryCache(opts ...CacheOption) *MemoryCache {
	cfg := &cacheConfig{
		maxEntries: 10000,
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return &MemoryCache{
		entries:    make(map[string]*list.Element),
		lruList:    list.New(),
		maxEntries: cfg.maxEntries,
		logger:     cfg.logger.With("component", "embedding_cache"),
	}
}

func (m *MemoryCache) Get(_ context.Context, key CacheKey) ([]float32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hash := key.Hash()
	if elem, ok := m.entries[hash]; ok {
		e, _ := elem.Value.(*entry)
		e.accessTime = time.Now()
		m.lruList.MoveToFront(elem)
		m.hits++
		return e.vector, true
	}

	m.misses++
	return nil, false
}

func (m *MemoryCache) Set(_ context.Context, key CacheKey, vector []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hash := key.Hash()
	if elem, ok := m.entries[hash]; ok {
		e, _ := elem.Value.(*entry)
		e.vector = vector
		e.accessTime = time.Now()
		m.lruList.MoveToFront(elem)
		return
	}

	for m.lruList.Len() >= m.maxEntries {
		oldest := m.lruList.Back()
		if oldest != nil {
			e, _ := oldest.Value.(*entry)
			delete(m.entries, e.key)
			m.lruList.Remove(oldest)
			m.evictions++
		}
	}

	e := &entry{
		key:        hash,
		vector:     vector,
		accessTime: time.Now(),
	}
	elem := m.lruList.PushFront(e)
	m.entries[hash] = elem
}

func (m *MemoryCache) Delete(_ context.Context, key CacheKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hash := key.Hash()
	if elem, ok := m.entries[hash]; ok {
		m.lruList.Remove(elem)
		delete(m.entries, hash)
	}
}

func (m *MemoryCache) Clear(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]*list.Element)
	m.lruList.Init()
	m.evictions = 0
}

func (m *MemoryCache) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lruList.Len()
}

// Stats returns current cache statistics.
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}

func (m *MemoryCache) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Stats{
		Hits:      m.hits,
		Misses:    m.misses,
		Evictions: m.evictions,
		Size:      m.lruList.Len(),
	}
}

func (m *MemoryCache) ResetStats() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hits = 0
	m.misses = 0
	m.evictions = 0
}
