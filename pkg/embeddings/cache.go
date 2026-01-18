// Package embeddings provides MLX embedding generation for semantic search.
package embeddings

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Cache defines the interface for caching embeddings.
// Implementations should be thread-safe.
type Cache interface {
	// Get retrieves an embedding from the cache.
	// Returns ErrCacheMiss if the key is not found.
	Get(ctx context.Context, text string) ([]float32, error)

	// Set stores an embedding in the cache.
	Set(ctx context.Context, text string, embedding []float32) error

	// Delete removes an embedding from the cache.
	Delete(ctx context.Context, text string) error

	// Clear removes all embeddings from the cache.
	Clear(ctx context.Context) error

	// Size returns the number of items in the cache.
	Size() int
}

// hashKey creates a cache key from text content.
// Uses SHA-256 to create a consistent, fixed-size key.
func hashKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// MemoryCacheConfig holds configuration for the memory cache.
type MemoryCacheConfig struct {
	// MaxSize is the maximum number of embeddings to cache.
	// When exceeded, the least recently used item is evicted.
	MaxSize int

	// TTL is the time-to-live for cached embeddings.
	// Zero means no expiration.
	TTL time.Duration
}

// DefaultMemoryCacheConfig returns sensible defaults for the memory cache.
func DefaultMemoryCacheConfig() *MemoryCacheConfig {
	return &MemoryCacheConfig{
		MaxSize: 10000, // 10k embeddings * ~4KB each = ~40MB max
		TTL:     time.Hour,
	}
}

// cacheEntry holds a cached embedding with metadata.
type cacheEntry struct {
	embedding []float32
	expiresAt time.Time
	key       string
}

// MemoryCache provides an in-memory LRU cache for embeddings.
// It is thread-safe and supports TTL-based expiration.
type MemoryCache struct {
	config    *MemoryCacheConfig
	cache     map[string]*list.Element
	lruList   *list.List
	mu        sync.RWMutex
	hitCount  int64
	missCount int64
}

// NewMemoryCache creates a new memory cache with the given configuration.
// If config is nil, DefaultMemoryCacheConfig() is used.
func NewMemoryCache(config *MemoryCacheConfig) *MemoryCache {
	if config == nil {
		config = DefaultMemoryCacheConfig()
	}
	return &MemoryCache{
		config:  config,
		cache:   make(map[string]*list.Element),
		lruList: list.New(),
	}
}

// Get retrieves an embedding from the cache.
func (c *MemoryCache) Get(ctx context.Context, text string) ([]float32, error) {
	key := hashKey(text)

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[key]
	if !ok {
		c.missCount++
		return nil, ErrCacheMiss
	}

	entry := elem.Value.(*cacheEntry)

	// Check expiration
	if c.config.TTL > 0 && time.Now().After(entry.expiresAt) {
		c.lruList.Remove(elem)
		delete(c.cache, key)
		c.missCount++
		return nil, ErrCacheMiss
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)
	c.hitCount++

	// Return a copy to prevent mutation
	result := make([]float32, len(entry.embedding))
	copy(result, entry.embedding)

	return result, nil
}

// Set stores an embedding in the cache.
func (c *MemoryCache) Set(ctx context.Context, text string, embedding []float32) error {
	key := hashKey(text)

	// Make a copy to prevent external mutation
	embeddingCopy := make([]float32, len(embedding))
	copy(embeddingCopy, embedding)

	entry := &cacheEntry{
		embedding: embeddingCopy,
		key:       key,
	}

	if c.config.TTL > 0 {
		entry.expiresAt = time.Now().Add(c.config.TTL)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.cache[key]; ok {
		c.lruList.MoveToFront(elem)
		elem.Value = entry
		return nil
	}

	// Evict if at capacity
	for c.lruList.Len() >= c.config.MaxSize {
		c.evictOldest()
	}

	// Add new entry
	elem := c.lruList.PushFront(entry)
	c.cache[key] = elem

	return nil
}

// evictOldest removes the least recently used entry.
// Must be called with lock held.
func (c *MemoryCache) evictOldest() {
	elem := c.lruList.Back()
	if elem != nil {
		entry := elem.Value.(*cacheEntry)
		c.lruList.Remove(elem)
		delete(c.cache, entry.key)
	}
}

// Delete removes an embedding from the cache.
func (c *MemoryCache) Delete(ctx context.Context, text string) error {
	key := hashKey(text)

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lruList.Remove(elem)
		delete(c.cache, key)
	}

	return nil
}

// Clear removes all embeddings from the cache.
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lruList = list.New()
	c.hitCount = 0
	c.missCount = 0

	return nil
}

// Size returns the number of items in the cache.
func (c *MemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Stats returns cache statistics.
func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hitCount + c.missCount
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hitCount) / float64(total)
	}

	return CacheStats{
		Size:      len(c.cache),
		MaxSize:   c.config.MaxSize,
		HitCount:  c.hitCount,
		MissCount: c.missCount,
		HitRate:   hitRate,
	}
}

// CacheStats contains cache statistics.
type CacheStats struct {
	Size      int
	MaxSize   int
	HitCount  int64
	MissCount int64
	HitRate   float64
}

// RedisCacheConfig holds configuration for the Redis cache.
type RedisCacheConfig struct {
	// Address is the Redis server address (host:port).
	Address string

	// Password is the Redis password (optional).
	Password string

	// DB is the Redis database number.
	DB int

	// KeyPrefix is the prefix for all cache keys.
	KeyPrefix string

	// TTL is the time-to-live for cached embeddings.
	TTL time.Duration
}

// DefaultRedisCacheConfig returns sensible defaults for the Redis cache.
func DefaultRedisCacheConfig() *RedisCacheConfig {
	return &RedisCacheConfig{
		Address:   "localhost:6379",
		Password:  "",
		DB:        0,
		KeyPrefix: "penfold:embedding:",
		TTL:       24 * time.Hour,
	}
}

// RedisClient defines the interface for Redis operations.
// This allows for easier testing and abstraction over different Redis clients.
type RedisClient interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	FlushDB(ctx context.Context) error
	DBSize(ctx context.Context) (int64, error)
}

// RedisCache provides a Redis-backed cache for embeddings.
// It supports TTL-based expiration and distributed caching.
type RedisCache struct {
	config *RedisCacheConfig
	client RedisClient
}

// NewRedisCache creates a new Redis cache with the given configuration.
// The RedisClient is passed in to allow for custom implementations.
func NewRedisCache(config *RedisCacheConfig, client RedisClient) (*RedisCache, error) {
	if config == nil {
		config = DefaultRedisCacheConfig()
	}
	if client == nil {
		return nil, ErrCacheUnavailable
	}
	return &RedisCache{
		config: config,
		client: client,
	}, nil
}

// Get retrieves an embedding from Redis.
func (c *RedisCache) Get(ctx context.Context, text string) ([]float32, error) {
	key := c.config.KeyPrefix + hashKey(text)

	data, err := c.client.Get(ctx, key)
	if err != nil {
		return nil, ErrCacheMiss
	}

	var embedding []float32
	if err := json.Unmarshal(data, &embedding); err != nil {
		return nil, ErrCacheMiss
	}

	return embedding, nil
}

// Set stores an embedding in Redis.
func (c *RedisCache) Set(ctx context.Context, text string, embedding []float32) error {
	key := c.config.KeyPrefix + hashKey(text)

	data, err := json.Marshal(embedding)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, c.config.TTL)
}

// Delete removes an embedding from Redis.
func (c *RedisCache) Delete(ctx context.Context, text string) error {
	key := c.config.KeyPrefix + hashKey(text)
	return c.client.Del(ctx, key)
}

// Clear removes all embeddings from the Redis database.
// WARNING: This uses FlushDB which clears the entire database.
// Use with caution if sharing the database with other applications.
func (c *RedisCache) Clear(ctx context.Context) error {
	return c.client.FlushDB(ctx)
}

// Size returns the approximate number of items in the Redis database.
// Note: This returns the total DB size, not just embeddings.
func (c *RedisCache) Size() int {
	size, err := c.client.DBSize(context.Background())
	if err != nil {
		return 0
	}
	return int(size)
}

// NopCache is a no-op cache implementation that never stores anything.
// Useful for testing or when caching is disabled.
type NopCache struct{}

// NewNopCache creates a new no-op cache.
func NewNopCache() *NopCache {
	return &NopCache{}
}

// Get always returns ErrCacheMiss.
func (c *NopCache) Get(ctx context.Context, text string) ([]float32, error) {
	return nil, ErrCacheMiss
}

// Set is a no-op.
func (c *NopCache) Set(ctx context.Context, text string, embedding []float32) error {
	return nil
}

// Delete is a no-op.
func (c *NopCache) Delete(ctx context.Context, text string) error {
	return nil
}

// Clear is a no-op.
func (c *NopCache) Clear(ctx context.Context) error {
	return nil
}

// Size always returns 0.
func (c *NopCache) Size() int {
	return 0
}
