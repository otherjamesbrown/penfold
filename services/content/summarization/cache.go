package summarization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Cache-related errors.
var (
	ErrCacheMiss    = errors.New("cache miss")
	ErrCacheExpired = errors.New("cache entry expired")
)

// SummaryCache defines the interface for caching summary results.
type SummaryCache interface {
	// Get retrieves a cached summary result.
	Get(ctx context.Context, req *SummaryRequest) (*SummaryResult, error)

	// Set stores a summary result in the cache.
	Set(ctx context.Context, req *SummaryRequest, result *SummaryResult, ttl time.Duration) error

	// Delete removes a cached entry.
	Delete(ctx context.Context, req *SummaryRequest) error

	// Clear removes all cached entries.
	Clear(ctx context.Context) error

	// Stats returns cache statistics.
	Stats() CacheStats
}

// CacheStats holds cache performance statistics.
type CacheStats struct {
	// Hits is the number of cache hits.
	Hits uint64

	// Misses is the number of cache misses.
	Misses uint64

	// Size is the current number of entries.
	Size int

	// MemoryUsageBytes is the approximate memory usage.
	MemoryUsageBytes int64
}

// HitRate returns the cache hit rate (0.0 to 1.0).
func (s CacheStats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0.0
	}
	return float64(s.Hits) / float64(total)
}

// CacheEntry holds a cached summary with metadata.
type CacheEntry struct {
	// Result is the cached summary result.
	Result *SummaryResult

	// CreatedAt is when the entry was cached.
	CreatedAt time.Time

	// ExpiresAt is when the entry expires.
	ExpiresAt time.Time

	// ContentHash identifies the source content.
	ContentHash string

	// Strategy is the strategy used.
	Strategy StrategyType

	// TenantID for multi-tenant isolation.
	TenantID string
}

// IsExpired returns true if the entry has expired.
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// MemoryCacheConfig holds configuration for the in-memory cache.
type MemoryCacheConfig struct {
	// MaxEntries is the maximum number of cache entries.
	MaxEntries int

	// DefaultTTL is the default time-to-live.
	DefaultTTL time.Duration

	// CleanupInterval is how often to run cleanup.
	CleanupInterval time.Duration
}

// DefaultMemoryCacheConfig returns sensible defaults.
func DefaultMemoryCacheConfig() *MemoryCacheConfig {
	return &MemoryCacheConfig{
		MaxEntries:      1000,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 5 * time.Minute,
	}
}

// MemoryCache is an in-memory implementation of SummaryCache.
type MemoryCache struct {
	entries     map[string]*CacheEntry
	config      *MemoryCacheConfig
	mu          sync.RWMutex
	hits        uint64
	misses      uint64
	stopCleanup chan struct{}
}

// NewMemoryCache creates a new in-memory cache.
func NewMemoryCache(cfg *MemoryCacheConfig) *MemoryCache {
	if cfg == nil {
		cfg = DefaultMemoryCacheConfig()
	}

	c := &MemoryCache{
		entries:     make(map[string]*CacheEntry),
		config:      cfg,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

// Get retrieves a cached summary result.
func (c *MemoryCache) Get(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	key := generateCacheKey(req)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, ErrCacheMiss
	}

	if entry.IsExpired() {
		c.mu.Lock()
		delete(c.entries, key)
		c.misses++
		c.mu.Unlock()
		return nil, ErrCacheExpired
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()

	return entry.Result, nil
}

// Set stores a summary result in the cache.
func (c *MemoryCache) Set(ctx context.Context, req *SummaryRequest, result *SummaryResult, ttl time.Duration) error {
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	key := generateCacheKey(req)
	now := time.Now()

	entry := &CacheEntry{
		Result:      result,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
		ContentHash: hashContent(req.Content),
		Strategy:    req.Strategy,
		TenantID:    req.TenantID,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	c.entries[key] = entry
	return nil
}

// Delete removes a cached entry.
func (c *MemoryCache) Delete(ctx context.Context, req *SummaryRequest) error {
	key := generateCacheKey(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
	return nil
}

// Clear removes all cached entries.
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	return nil
}

// Stats returns cache statistics.
func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Estimate memory usage (rough approximation)
	var memUsage int64
	for _, entry := range c.entries {
		memUsage += int64(len(entry.Result.Summary))
		for _, kp := range entry.Result.KeyPoints {
			memUsage += int64(len(kp))
		}
		memUsage += 200 // overhead for struct fields
	}

	return CacheStats{
		Hits:             c.hits,
		Misses:           c.misses,
		Size:             len(c.entries),
		MemoryUsageBytes: memUsage,
	}
}

// Close stops the cleanup goroutine.
func (c *MemoryCache) Close() {
	close(c.stopCleanup)
}

// cleanupLoop periodically removes expired entries.
func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries.
func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
		}
	}
}

// evictOldest removes the oldest entry (caller must hold lock).
func (c *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// RedisCache is a Redis-backed implementation of SummaryCache.
type RedisCache struct {
	client    RedisClient
	keyPrefix string
	config    *RedisCacheConfig
	mu        sync.RWMutex
	hits      uint64
	misses    uint64
}

// RedisCacheConfig holds configuration for Redis cache.
type RedisCacheConfig struct {
	// KeyPrefix is the prefix for all cache keys.
	KeyPrefix string

	// DefaultTTL is the default time-to-live.
	DefaultTTL time.Duration

	// Compression enables compression for large entries.
	Compression bool

	// CompressionThreshold is the minimum size for compression.
	CompressionThreshold int
}

// DefaultRedisCacheConfig returns sensible defaults.
func DefaultRedisCacheConfig() *RedisCacheConfig {
	return &RedisCacheConfig{
		KeyPrefix:            "penfold:summarization:",
		DefaultTTL:           1 * time.Hour,
		Compression:          true,
		CompressionThreshold: 1024, // 1KB
	}
}

// RedisClient defines the interface for Redis operations.
type RedisClient interface {
	// Get retrieves a value by key.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with TTL.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key.
	Delete(ctx context.Context, key string) error

	// Scan returns keys matching a pattern.
	Scan(ctx context.Context, pattern string) ([]string, error)
}

// NewRedisCache creates a new Redis-backed cache.
func NewRedisCache(client RedisClient, cfg *RedisCacheConfig) *RedisCache {
	if cfg == nil {
		cfg = DefaultRedisCacheConfig()
	}

	return &RedisCache{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
		config:    cfg,
	}
}

// Get retrieves a cached summary result from Redis.
func (c *RedisCache) Get(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	key := c.keyPrefix + generateCacheKey(req)

	data, err := c.client.Get(ctx, key)
	if err != nil {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, ErrCacheMiss
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()

	return entry.Result, nil
}

// Set stores a summary result in Redis.
func (c *RedisCache) Set(ctx context.Context, req *SummaryRequest, result *SummaryResult, ttl time.Duration) error {
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	key := c.keyPrefix + generateCacheKey(req)
	now := time.Now()

	entry := &CacheEntry{
		Result:      result,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
		ContentHash: hashContent(req.Content),
		Strategy:    req.Strategy,
		TenantID:    req.TenantID,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, ttl)
}

// Delete removes a cached entry from Redis.
func (c *RedisCache) Delete(ctx context.Context, req *SummaryRequest) error {
	key := c.keyPrefix + generateCacheKey(req)
	return c.client.Delete(ctx, key)
}

// Clear removes all cached entries (matching prefix).
func (c *RedisCache) Clear(ctx context.Context) error {
	keys, err := c.client.Scan(ctx, c.keyPrefix+"*")
	if err != nil {
		return err
	}

	for _, key := range keys {
		if err := c.client.Delete(ctx, key); err != nil {
			return err
		}
	}

	return nil
}

// Stats returns cache statistics.
func (c *RedisCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Hits:   c.hits,
		Misses: c.misses,
		// Size and memory can't be easily determined from Redis
	}
}

// TieredCache implements a two-level cache (memory + Redis).
type TieredCache struct {
	l1     *MemoryCache // Fast local cache
	l2     *RedisCache  // Shared cache
	logger func(string)
}

// NewTieredCache creates a two-level cache.
func NewTieredCache(l1 *MemoryCache, l2 *RedisCache) *TieredCache {
	return &TieredCache{
		l1: l1,
		l2: l2,
	}
}

// Get retrieves from L1 first, then L2.
func (c *TieredCache) Get(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	// Try L1 first
	if result, err := c.l1.Get(ctx, req); err == nil {
		return result, nil
	}

	// Try L2
	if c.l2 != nil {
		if result, err := c.l2.Get(ctx, req); err == nil {
			// Populate L1 on L2 hit
			_ = c.l1.Set(ctx, req, result, 0)
			return result, nil
		}
	}

	return nil, ErrCacheMiss
}

// Set stores in both L1 and L2.
func (c *TieredCache) Set(ctx context.Context, req *SummaryRequest, result *SummaryResult, ttl time.Duration) error {
	// Store in L1
	if err := c.l1.Set(ctx, req, result, ttl); err != nil {
		return err
	}

	// Store in L2 if available
	if c.l2 != nil {
		if err := c.l2.Set(ctx, req, result, ttl); err != nil {
			// Log but don't fail
			if c.logger != nil {
				c.logger("L2 cache set failed: " + err.Error())
			}
		}
	}

	return nil
}

// Delete removes from both caches.
func (c *TieredCache) Delete(ctx context.Context, req *SummaryRequest) error {
	_ = c.l1.Delete(ctx, req)
	if c.l2 != nil {
		_ = c.l2.Delete(ctx, req)
	}
	return nil
}

// Clear clears both caches.
func (c *TieredCache) Clear(ctx context.Context) error {
	_ = c.l1.Clear(ctx)
	if c.l2 != nil {
		_ = c.l2.Clear(ctx)
	}
	return nil
}

// Stats returns combined statistics.
func (c *TieredCache) Stats() CacheStats {
	l1Stats := c.l1.Stats()
	stats := CacheStats{
		Hits:             l1Stats.Hits,
		Misses:           l1Stats.Misses,
		Size:             l1Stats.Size,
		MemoryUsageBytes: l1Stats.MemoryUsageBytes,
	}

	if c.l2 != nil {
		l2Stats := c.l2.Stats()
		// L2 hits that weren't L1 hits
		stats.Hits += l2Stats.Hits
	}

	return stats
}

// Helper functions

// generateCacheKey creates a unique key for a summary request.
func generateCacheKey(req *SummaryRequest) string {
	// Key components: content hash + strategy + length + format + tenant
	contentHash := hashContent(req.Content)

	keyData := map[string]string{
		"content":  contentHash,
		"strategy": string(req.Strategy),
		"length":   string(req.Length),
		"format":   string(req.Format),
		"tenant":   req.TenantID,
	}

	// Include title if present (affects summary)
	if req.Title != "" {
		keyData["title"] = hashContent(req.Title)
	}

	// Include model preference if specified
	if req.ModelPreference != "" {
		keyData["model"] = req.ModelPreference
	}

	// Include max tokens if specified
	if req.MaxTokens > 0 {
		keyData["tokens"] = string(rune(req.MaxTokens))
	}

	// Serialize and hash
	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// hashContent creates a hash of content for cache keys.
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes
}

// NullCache is a no-op cache implementation.
type NullCache struct{}

// NewNullCache creates a cache that does nothing.
func NewNullCache() *NullCache {
	return &NullCache{}
}

// Get always returns cache miss.
func (c *NullCache) Get(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	return nil, ErrCacheMiss
}

// Set does nothing.
func (c *NullCache) Set(ctx context.Context, req *SummaryRequest, result *SummaryResult, ttl time.Duration) error {
	return nil
}

// Delete does nothing.
func (c *NullCache) Delete(ctx context.Context, req *SummaryRequest) error {
	return nil
}

// Clear does nothing.
func (c *NullCache) Clear(ctx context.Context) error {
	return nil
}

// Stats returns empty stats.
func (c *NullCache) Stats() CacheStats {
	return CacheStats{}
}
