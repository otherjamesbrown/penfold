// Package cache provides result caching for the Search service.
// It implements an in-memory cache with TTL-based expiration, tenant isolation,
// and metrics tracking for cache performance monitoring.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
)

// Default configuration values for the search cache.
const (
	// DefaultTTL is the default time-to-live for cached search results.
	DefaultTTL = 5 * time.Minute

	// RealtimeTTL is the shorter TTL for real-time queries.
	RealtimeTTL = 30 * time.Second

	// DefaultMaxEntries is the default maximum number of cache entries.
	DefaultMaxEntries = 10000

	// CleanupInterval is how often the cache runs cleanup for expired entries.
	CleanupInterval = 1 * time.Minute
)

// CacheEntry holds a cached search result with metadata.
type CacheEntry struct {
	// Response is the cached search response.
	Response *searchv1.SearchResponse

	// CreatedAt is when the entry was cached.
	CreatedAt time.Time

	// ExpiresAt is when the entry expires.
	ExpiresAt time.Time

	// TenantID is the tenant this entry belongs to (for isolation).
	TenantID string
}

// IsExpired returns true if the cache entry has expired.
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// Metrics tracks cache performance statistics.
type Metrics struct {
	// Hits is the number of cache hits.
	Hits atomic.Uint64

	// Misses is the number of cache misses.
	Misses atomic.Uint64

	// Evictions is the number of cache evictions (due to size or TTL).
	Evictions atomic.Uint64

	// Invalidations is the number of explicit cache invalidations.
	Invalidations atomic.Uint64
}

// HitRate returns the cache hit rate as a percentage (0.0 to 1.0).
func (m *Metrics) HitRate() float64 {
	hits := m.Hits.Load()
	misses := m.Misses.Load()
	total := hits + misses
	if total == 0 {
		return 0.0
	}
	return float64(hits) / float64(total)
}

// MissRate returns the cache miss rate as a percentage (0.0 to 1.0).
func (m *Metrics) MissRate() float64 {
	return 1.0 - m.HitRate()
}

// Stats returns a snapshot of cache metrics.
type Stats struct {
	Hits          uint64
	Misses        uint64
	Evictions     uint64
	Invalidations uint64
	HitRate       float64
	MissRate      float64
	EntryCount    int
}

// Config holds configuration options for the cache.
type Config struct {
	// DefaultTTL is the default TTL for cache entries.
	DefaultTTL time.Duration

	// RealtimeTTL is the TTL for real-time query cache entries.
	RealtimeTTL time.Duration

	// MaxEntries is the maximum number of entries in the cache.
	MaxEntries int

	// CleanupInterval is how often to run cleanup for expired entries.
	CleanupInterval time.Duration
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		DefaultTTL:      DefaultTTL,
		RealtimeTTL:     RealtimeTTL,
		MaxEntries:      DefaultMaxEntries,
		CleanupInterval: CleanupInterval,
	}
}

// SearchCache provides thread-safe caching for search results.
type SearchCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	metrics *Metrics
	config  *Config

	// stopCleanup signals the cleanup goroutine to stop.
	stopCleanup chan struct{}
}

// New creates a new SearchCache with default configuration.
func New() *SearchCache {
	return NewWithConfig(DefaultConfig())
}

// NewWithConfig creates a new SearchCache with the provided configuration.
func NewWithConfig(cfg *Config) *SearchCache {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	c := &SearchCache{
		entries:     make(map[string]*CacheEntry),
		metrics:     &Metrics{},
		config:      cfg,
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go c.cleanupLoop()

	return c
}

// Get retrieves a cached search result by key.
// Returns nil if the key is not found or has expired.
func (c *SearchCache) Get(key string) *searchv1.SearchResponse {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.metrics.Misses.Add(1)
		return nil
	}

	if entry.IsExpired() {
		// Entry has expired, remove it and count as miss
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		c.metrics.Misses.Add(1)
		c.metrics.Evictions.Add(1)
		return nil
	}

	c.metrics.Hits.Add(1)
	return entry.Response
}

// Set stores a search result in the cache with the specified TTL.
// If ttl is 0, the default TTL is used.
func (c *SearchCache) Set(key string, response *searchv1.SearchResponse, tenantID string, ttl time.Duration) {
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	now := time.Now()
	entry := &CacheEntry{
		Response:  response,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		TenantID:  tenantID,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries to stay under max
	if len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	c.entries[key] = entry
}

// SetWithRealtimeTTL stores a search result with the shorter real-time TTL.
func (c *SearchCache) SetWithRealtimeTTL(key string, response *searchv1.SearchResponse, tenantID string) {
	c.Set(key, response, tenantID, c.config.RealtimeTTL)
}

// Invalidate removes cache entries matching the given patterns.
// Patterns can be:
// - Empty string: invalidates all entries
// - "tenant:TENANT_ID": invalidates all entries for a specific tenant
// - "doc:DOCUMENT_ID": invalidates entries that might contain the document
// - Exact key: invalidates a specific entry
func (c *SearchCache) Invalidate(patterns ...string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	invalidated := 0

	if len(patterns) == 0 {
		// Invalidate everything
		invalidated = len(c.entries)
		c.entries = make(map[string]*CacheEntry)
		c.metrics.Invalidations.Add(uint64(invalidated))
		return invalidated
	}

	for _, pattern := range patterns {
		if pattern == "" {
			// Skip empty patterns
			continue
		}

		// Check for tenant pattern
		if strings.HasPrefix(pattern, "tenant:") {
			tenantID := strings.TrimPrefix(pattern, "tenant:")
			invalidated += c.invalidateByTenant(tenantID)
			continue
		}

		// Check for document pattern - invalidate all entries for the tenant
		// since document changes could affect any search result
		if strings.HasPrefix(pattern, "doc:") {
			// Document invalidation requires knowing the tenant
			// For now, this is a no-op unless combined with tenant pattern
			// In practice, the caller should use InvalidateForDocument
			continue
		}

		// Exact key match
		if _, exists := c.entries[pattern]; exists {
			delete(c.entries, pattern)
			invalidated++
		}
	}

	c.metrics.Invalidations.Add(uint64(invalidated))
	return invalidated
}

// InvalidateForTenant removes all cache entries for a specific tenant.
func (c *SearchCache) InvalidateForTenant(tenantID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	invalidated := c.invalidateByTenant(tenantID)
	c.metrics.Invalidations.Add(uint64(invalidated))
	return invalidated
}

// invalidateByTenant removes entries for a tenant (caller must hold lock).
func (c *SearchCache) invalidateByTenant(tenantID string) int {
	invalidated := 0
	for key, entry := range c.entries {
		if entry.TenantID == tenantID {
			delete(c.entries, key)
			invalidated++
		}
	}
	return invalidated
}

// InvalidateForDocument removes cache entries that might contain results
// for the specified document. Since we can't know which searches include
// a document without re-executing them, we invalidate all entries for the tenant.
func (c *SearchCache) InvalidateForDocument(tenantID, documentID string) int {
	// Document changes can affect any search result for that tenant,
	// so we invalidate all tenant entries.
	return c.InvalidateForTenant(tenantID)
}

// GenerateKey creates a cache key from search parameters.
// The key includes: query hash, filter hash, tenant_id, and sort order.
// CRITICAL: tenant_id is ALWAYS included to ensure tenant isolation.
func GenerateKey(query string, tenantID string, filters *searchv1.FilterOptions, sortOrder searchv1.SortOrder, limit, offset int32) string {
	// Build a deterministic key from search parameters
	keyParts := []string{
		"tenant:" + tenantID, // CRITICAL: Tenant isolation
		"query:" + hashString(query),
		"sort:" + sortOrder.String(),
		"limit:" + intToString(limit),
		"offset:" + intToString(offset),
	}

	// Add filter hash if filters are present
	if filters != nil {
		filterHash := hashFilters(filters)
		keyParts = append(keyParts, "filters:"+filterHash)
	}

	return strings.Join(keyParts, "|")
}

// GenerateKeyForHybridSearch creates a cache key for hybrid search requests.
func GenerateKeyForHybridSearch(req *searchv1.SearchRequest) string {
	// For hybrid search, also include the weights in the key
	// since different weights produce different results
	baseKey := GenerateKey(
		req.GetQuery(),
		req.GetTenantId(),
		req.GetFilters(),
		searchv1.SortOrder_SORT_ORDER_RELEVANCE, // Default for hybrid
		req.GetLimit(),
		req.GetOffset(),
	)

	// Add weights to the key
	textWeight := req.GetTextWeight()
	vectorWeight := req.GetVectorWeight()
	minScore := req.GetMinScore()

	weightKey := hashString(floatToString(textWeight) + ":" + floatToString(vectorWeight) + ":" + floatToString(minScore))

	return baseKey + "|weights:" + weightKey
}

// GenerateKeyForSemanticSearch creates a cache key for semantic search requests.
func GenerateKeyForSemanticSearch(req *searchv1.SemanticSearchRequest) string {
	baseKey := GenerateKey(
		req.GetQuery(),
		req.GetTenantId(),
		req.GetFilters(),
		searchv1.SortOrder_SORT_ORDER_RELEVANCE, // Default for semantic
		req.GetLimit(),
		req.GetOffset(),
	)

	// Add min similarity to the key
	minSimilarity := req.GetMinSimilarity()
	return baseKey + "|minSim:" + floatToString(minSimilarity)
}

// GenerateKeyForKeywordSearch creates a cache key for keyword search requests.
func GenerateKeyForKeywordSearch(req *searchv1.KeywordSearchRequest) string {
	baseKey := GenerateKey(
		req.GetQuery(),
		req.GetTenantId(),
		req.GetFilters(),
		searchv1.SortOrder_SORT_ORDER_RELEVANCE, // Default for keyword
		req.GetLimit(),
		req.GetOffset(),
	)

	// Add fuzzy matching flag to the key
	fuzzy := req.GetFuzzyMatching()
	if fuzzy {
		return baseKey + "|fuzzy:true"
	}
	return baseKey + "|fuzzy:false"
}

// Stats returns current cache statistics.
func (c *SearchCache) Stats() Stats {
	c.mu.RLock()
	entryCount := len(c.entries)
	c.mu.RUnlock()

	return Stats{
		Hits:          c.metrics.Hits.Load(),
		Misses:        c.metrics.Misses.Load(),
		Evictions:     c.metrics.Evictions.Load(),
		Invalidations: c.metrics.Invalidations.Load(),
		HitRate:       c.metrics.HitRate(),
		MissRate:      c.metrics.MissRate(),
		EntryCount:    entryCount,
	}
}

// Metrics returns the raw metrics for external tracking.
func (c *SearchCache) Metrics() *Metrics {
	return c.metrics
}

// Size returns the current number of entries in the cache.
func (c *SearchCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Close stops the background cleanup goroutine and clears the cache.
func (c *SearchCache) Close() {
	close(c.stopCleanup)

	c.mu.Lock()
	c.entries = make(map[string]*CacheEntry)
	c.mu.Unlock()
}

// cleanupLoop periodically removes expired entries.
func (c *SearchCache) cleanupLoop() {
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

// cleanup removes all expired entries.
func (c *SearchCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	evicted := 0
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
			evicted++
		}
	}

	if evicted > 0 {
		c.metrics.Evictions.Add(uint64(evicted))
	}
}

// evictOldest removes the oldest entry (caller must hold lock).
func (c *SearchCache) evictOldest() {
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
		c.metrics.Evictions.Add(1)
	}
}

// Helper functions for key generation

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8]) // Use first 8 bytes for shorter keys
}

func hashFilters(f *searchv1.FilterOptions) string {
	if f == nil {
		return "none"
	}

	// Create a deterministic representation of filters
	data := map[string]interface{}{
		"content_types": sortedStrings(f.GetContentTypes()),
		"source_ids":    sortedStrings(f.GetSourceIds()),
		"tags":          sortedStrings(f.GetTags()),
		"entity_ids":    sortedStrings(f.GetEntityIds()),
		"sources":       sortedStrings(f.GetSources()),
		"exclude_ids":   sortedStrings(f.GetExcludeIds()),
	}

	// Add date filters if present
	if f.GetDateFrom() != nil {
		data["date_from"] = f.GetDateFrom().AsTime().Unix()
	}
	if f.GetDateTo() != nil {
		data["date_to"] = f.GetDateTo().AsTime().Unix()
	}

	// Marshal to JSON for consistent hashing
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		// Fallback to a simple string representation
		return hashString(f.String())
	}

	return hashString(string(jsonBytes))
}

func sortedStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	sorted := make([]string, len(s))
	copy(sorted, s)
	sort.Strings(sorted)
	return sorted
}

func intToString(i int32) string {
	return fmt.Sprintf("%d", i)
}

func floatToString(f float32) string {
	// Use a fixed precision to ensure consistent hashing
	return fmt.Sprintf("%.4f", f)
}
