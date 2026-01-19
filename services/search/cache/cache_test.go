package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Helper to create a test search response
func makeTestResponse(totalCount int64) *searchv1.SearchResponse {
	return &searchv1.SearchResponse{
		TotalCount:  totalCount,
		QueryTimeMs: 42.5,
		Results: []*searchv1.SearchResult{
			{
				DocumentId:  "doc-1",
				ContentType: "email",
				Score:       0.95,
				Snippet:     "Test snippet",
			},
		},
	}
}

func TestNew(t *testing.T) {
	c := New()
	defer c.Close()

	if c == nil {
		t.Fatal("New() returned nil")
	}

	if c.Size() != 0 {
		t.Errorf("New cache should be empty, got size %d", c.Size())
	}
}

func TestNewWithConfig(t *testing.T) {
	cfg := &Config{
		DefaultTTL:      10 * time.Second,
		RealtimeTTL:     5 * time.Second,
		MaxEntries:      100,
		CleanupInterval: 30 * time.Second,
	}

	c := NewWithConfig(cfg)
	defer c.Close()

	if c.config.DefaultTTL != 10*time.Second {
		t.Errorf("Expected DefaultTTL 10s, got %v", c.config.DefaultTTL)
	}

	if c.config.MaxEntries != 100 {
		t.Errorf("Expected MaxEntries 100, got %d", c.config.MaxEntries)
	}
}

func TestNewWithConfig_Nil(t *testing.T) {
	c := NewWithConfig(nil)
	defer c.Close()

	if c.config.DefaultTTL != DefaultTTL {
		t.Errorf("Expected default TTL %v, got %v", DefaultTTL, c.config.DefaultTTL)
	}
}

func TestGetSet(t *testing.T) {
	c := New()
	defer c.Close()

	key := "test-key"
	tenantID := "tenant-1"
	response := makeTestResponse(42)

	// Get non-existent key
	result := c.Get(key)
	if result != nil {
		t.Error("Get on non-existent key should return nil")
	}

	// Set and get
	c.Set(key, response, tenantID, 0)

	result = c.Get(key)
	if result == nil {
		t.Fatal("Get after Set should return value")
	}

	if result.TotalCount != 42 {
		t.Errorf("Expected TotalCount 42, got %d", result.TotalCount)
	}
}

func TestGetExpired(t *testing.T) {
	cfg := &Config{
		DefaultTTL:      50 * time.Millisecond,
		RealtimeTTL:     25 * time.Millisecond,
		MaxEntries:      1000,
		CleanupInterval: 1 * time.Hour, // Don't run automatic cleanup
	}
	c := NewWithConfig(cfg)
	defer c.Close()

	key := "test-key"
	tenantID := "tenant-1"
	response := makeTestResponse(42)

	c.Set(key, response, tenantID, 50*time.Millisecond)

	// Should exist immediately
	if c.Get(key) == nil {
		t.Error("Entry should exist immediately after Set")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Should be expired
	if c.Get(key) != nil {
		t.Error("Entry should be expired")
	}
}

func TestSetWithRealtimeTTL(t *testing.T) {
	cfg := &Config{
		DefaultTTL:      1 * time.Hour,
		RealtimeTTL:     50 * time.Millisecond,
		MaxEntries:      1000,
		CleanupInterval: 1 * time.Hour,
	}
	c := NewWithConfig(cfg)
	defer c.Close()

	key := "realtime-key"
	tenantID := "tenant-1"
	response := makeTestResponse(10)

	c.SetWithRealtimeTTL(key, response, tenantID)

	// Should exist immediately
	if c.Get(key) == nil {
		t.Error("Entry should exist immediately after SetWithRealtimeTTL")
	}

	// Wait for realtime TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should be expired
	if c.Get(key) != nil {
		t.Error("Entry with realtime TTL should be expired")
	}
}

func TestInvalidate_All(t *testing.T) {
	c := New()
	defer c.Close()

	// Add multiple entries
	for i := 0; i < 10; i++ {
		c.Set(fmt.Sprintf("key-%d", i), makeTestResponse(int64(i)), "tenant-1", 0)
	}

	if c.Size() != 10 {
		t.Fatalf("Expected 10 entries, got %d", c.Size())
	}

	// Invalidate all
	count := c.Invalidate()
	if count != 10 {
		t.Errorf("Expected 10 invalidated, got %d", count)
	}

	if c.Size() != 0 {
		t.Errorf("Expected 0 entries after invalidate all, got %d", c.Size())
	}
}

func TestInvalidate_ExactKey(t *testing.T) {
	c := New()
	defer c.Close()

	c.Set("key-1", makeTestResponse(1), "tenant-1", 0)
	c.Set("key-2", makeTestResponse(2), "tenant-1", 0)
	c.Set("key-3", makeTestResponse(3), "tenant-1", 0)

	count := c.Invalidate("key-2")
	if count != 1 {
		t.Errorf("Expected 1 invalidated, got %d", count)
	}

	if c.Get("key-1") == nil {
		t.Error("key-1 should still exist")
	}
	if c.Get("key-2") != nil {
		t.Error("key-2 should be invalidated")
	}
	if c.Get("key-3") == nil {
		t.Error("key-3 should still exist")
	}
}

func TestInvalidateForTenant(t *testing.T) {
	c := New()
	defer c.Close()

	// Add entries for different tenants
	c.Set("key-t1-1", makeTestResponse(1), "tenant-1", 0)
	c.Set("key-t1-2", makeTestResponse(2), "tenant-1", 0)
	c.Set("key-t2-1", makeTestResponse(3), "tenant-2", 0)
	c.Set("key-t2-2", makeTestResponse(4), "tenant-2", 0)
	c.Set("key-t3-1", makeTestResponse(5), "tenant-3", 0)

	// Invalidate tenant-1
	count := c.InvalidateForTenant("tenant-1")
	if count != 2 {
		t.Errorf("Expected 2 invalidated for tenant-1, got %d", count)
	}

	// Verify tenant-1 entries are gone
	if c.Get("key-t1-1") != nil || c.Get("key-t1-2") != nil {
		t.Error("tenant-1 entries should be invalidated")
	}

	// Verify other tenants unaffected
	if c.Get("key-t2-1") == nil || c.Get("key-t2-2") == nil {
		t.Error("tenant-2 entries should still exist")
	}
	if c.Get("key-t3-1") == nil {
		t.Error("tenant-3 entries should still exist")
	}
}

func TestInvalidate_TenantPattern(t *testing.T) {
	c := New()
	defer c.Close()

	c.Set("key-1", makeTestResponse(1), "tenant-1", 0)
	c.Set("key-2", makeTestResponse(2), "tenant-1", 0)
	c.Set("key-3", makeTestResponse(3), "tenant-2", 0)

	count := c.Invalidate("tenant:tenant-1")
	if count != 2 {
		t.Errorf("Expected 2 invalidated, got %d", count)
	}

	if c.Size() != 1 {
		t.Errorf("Expected 1 entry remaining, got %d", c.Size())
	}
}

func TestInvalidateForDocument(t *testing.T) {
	c := New()
	defer c.Close()

	// Add entries for a tenant
	c.Set("key-1", makeTestResponse(1), "tenant-1", 0)
	c.Set("key-2", makeTestResponse(2), "tenant-1", 0)
	c.Set("key-3", makeTestResponse(3), "tenant-2", 0)

	// Invalidate for a document in tenant-1
	count := c.InvalidateForDocument("tenant-1", "doc-123")

	// Should invalidate all tenant-1 entries
	if count != 2 {
		t.Errorf("Expected 2 invalidated, got %d", count)
	}

	// Verify tenant-2 unaffected
	if c.Get("key-3") == nil {
		t.Error("tenant-2 entries should still exist")
	}
}

func TestGenerateKey_TenantIsolation(t *testing.T) {
	// CRITICAL: Same query for different tenants must produce different keys
	query := "test query"
	filters := &searchv1.FilterOptions{}
	sortOrder := searchv1.SortOrder_SORT_ORDER_RELEVANCE

	key1 := GenerateKey(query, "tenant-1", filters, sortOrder, 10, 0)
	key2 := GenerateKey(query, "tenant-2", filters, sortOrder, 10, 0)

	if key1 == key2 {
		t.Error("CRITICAL: Keys for different tenants must be different!")
	}
}

func TestGenerateKey_Consistency(t *testing.T) {
	// Same parameters should always produce the same key
	query := "test query"
	tenantID := "tenant-1"
	filters := &searchv1.FilterOptions{
		ContentTypes: []string{"email", "document"},
		Tags:         []string{"important", "urgent"},
	}
	sortOrder := searchv1.SortOrder_SORT_ORDER_DATE_DESC

	key1 := GenerateKey(query, tenantID, filters, sortOrder, 20, 10)
	key2 := GenerateKey(query, tenantID, filters, sortOrder, 20, 10)

	if key1 != key2 {
		t.Error("Same parameters should produce the same key")
	}
}

func TestGenerateKey_DifferentParams(t *testing.T) {
	tenantID := "tenant-1"
	filters := &searchv1.FilterOptions{}
	sortOrder := searchv1.SortOrder_SORT_ORDER_RELEVANCE

	// Different queries
	key1 := GenerateKey("query1", tenantID, filters, sortOrder, 10, 0)
	key2 := GenerateKey("query2", tenantID, filters, sortOrder, 10, 0)
	if key1 == key2 {
		t.Error("Different queries should produce different keys")
	}

	// Different limits
	key3 := GenerateKey("query", tenantID, filters, sortOrder, 10, 0)
	key4 := GenerateKey("query", tenantID, filters, sortOrder, 20, 0)
	if key3 == key4 {
		t.Error("Different limits should produce different keys")
	}

	// Different offsets
	key5 := GenerateKey("query", tenantID, filters, sortOrder, 10, 0)
	key6 := GenerateKey("query", tenantID, filters, sortOrder, 10, 20)
	if key5 == key6 {
		t.Error("Different offsets should produce different keys")
	}

	// Different sort orders
	key7 := GenerateKey("query", tenantID, filters, searchv1.SortOrder_SORT_ORDER_RELEVANCE, 10, 0)
	key8 := GenerateKey("query", tenantID, filters, searchv1.SortOrder_SORT_ORDER_DATE_DESC, 10, 0)
	if key7 == key8 {
		t.Error("Different sort orders should produce different keys")
	}
}

func TestGenerateKey_FilterHashing(t *testing.T) {
	tenantID := "tenant-1"
	sortOrder := searchv1.SortOrder_SORT_ORDER_RELEVANCE

	// Same filters in different order should produce same key
	filters1 := &searchv1.FilterOptions{
		ContentTypes: []string{"email", "document"},
		Tags:         []string{"urgent", "important"},
	}
	filters2 := &searchv1.FilterOptions{
		ContentTypes: []string{"document", "email"},
		Tags:         []string{"important", "urgent"},
	}

	key1 := GenerateKey("query", tenantID, filters1, sortOrder, 10, 0)
	key2 := GenerateKey("query", tenantID, filters2, sortOrder, 10, 0)

	if key1 != key2 {
		t.Error("Filters with same values in different order should produce same key")
	}

	// Different filters should produce different keys
	filters3 := &searchv1.FilterOptions{
		ContentTypes: []string{"email"},
	}
	key3 := GenerateKey("query", tenantID, filters3, sortOrder, 10, 0)

	if key1 == key3 {
		t.Error("Different filters should produce different keys")
	}

	// Nil filters vs empty filters
	key4 := GenerateKey("query", tenantID, nil, sortOrder, 10, 0)
	key5 := GenerateKey("query", tenantID, &searchv1.FilterOptions{}, sortOrder, 10, 0)

	// Both should work, but may differ
	if key4 == "" || key5 == "" {
		t.Error("Keys should not be empty")
	}
}

func TestGenerateKey_DateFilters(t *testing.T) {
	tenantID := "tenant-1"
	sortOrder := searchv1.SortOrder_SORT_ORDER_RELEVANCE
	now := time.Now()

	filters1 := &searchv1.FilterOptions{
		DateFrom: timestamppb.New(now),
	}
	filters2 := &searchv1.FilterOptions{
		DateFrom: timestamppb.New(now.Add(-time.Hour)),
	}

	key1 := GenerateKey("query", tenantID, filters1, sortOrder, 10, 0)
	key2 := GenerateKey("query", tenantID, filters2, sortOrder, 10, 0)

	if key1 == key2 {
		t.Error("Different date filters should produce different keys")
	}
}

func TestGenerateKeyForHybridSearch(t *testing.T) {
	req := &searchv1.SearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
		Offset:   0,
	}

	key := GenerateKeyForHybridSearch(req)
	if key == "" {
		t.Error("Generated key should not be empty")
	}

	// Key should contain tenant info
	if !containsSubstring(key, "tenant:tenant-1") {
		t.Error("Key should contain tenant ID")
	}
}

func TestGenerateKeyForSemanticSearch(t *testing.T) {
	req := &searchv1.SemanticSearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	key := GenerateKeyForSemanticSearch(req)
	if key == "" {
		t.Error("Generated key should not be empty")
	}

	// Should contain minSim
	if !containsSubstring(key, "minSim:") {
		t.Error("Key should contain minSim")
	}
}

func TestGenerateKeyForKeywordSearch(t *testing.T) {
	req := &searchv1.KeywordSearchRequest{
		Query:    "test query",
		TenantId: "tenant-1",
		Limit:    10,
	}

	key1 := GenerateKeyForKeywordSearch(req)
	if key1 == "" {
		t.Error("Generated key should not be empty")
	}

	// Should contain fuzzy flag
	if !containsSubstring(key1, "fuzzy:") {
		t.Error("Key should contain fuzzy flag")
	}
}

func TestMetrics_HitMiss(t *testing.T) {
	c := New()
	defer c.Close()

	// Initial state
	stats := c.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("Initial stats should be zero")
	}

	// Cache miss
	c.Get("nonexistent")
	stats = c.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	// Cache hit
	c.Set("key", makeTestResponse(1), "tenant-1", 0)
	c.Get("key")
	stats = c.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
}

func TestMetrics_HitRate(t *testing.T) {
	c := New()
	defer c.Close()

	// Set up some hits and misses
	c.Set("key", makeTestResponse(1), "tenant-1", 0)

	// 2 misses
	c.Get("miss1")
	c.Get("miss2")

	// 3 hits
	c.Get("key")
	c.Get("key")
	c.Get("key")

	stats := c.Stats()

	// Hit rate should be 3/5 = 0.6
	expectedHitRate := 0.6
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("Expected hit rate ~0.6, got %f", stats.HitRate)
	}

	// Miss rate should be 2/5 = 0.4
	expectedMissRate := 0.4
	if stats.MissRate < expectedMissRate-0.01 || stats.MissRate > expectedMissRate+0.01 {
		t.Errorf("Expected miss rate ~0.4, got %f", stats.MissRate)
	}
}

func TestMetrics_Evictions(t *testing.T) {
	cfg := &Config{
		DefaultTTL:      50 * time.Millisecond,
		RealtimeTTL:     25 * time.Millisecond,
		MaxEntries:      1000,
		CleanupInterval: 1 * time.Hour,
	}
	c := NewWithConfig(cfg)
	defer c.Close()

	c.Set("key", makeTestResponse(1), "tenant-1", 50*time.Millisecond)

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Get should trigger eviction
	c.Get("key")

	stats := c.Stats()
	if stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestMetrics_Invalidations(t *testing.T) {
	c := New()
	defer c.Close()

	// Add entries
	for i := 0; i < 5; i++ {
		c.Set(fmt.Sprintf("key-%d", i), makeTestResponse(int64(i)), "tenant-1", 0)
	}

	// Invalidate all
	c.Invalidate()

	stats := c.Stats()
	if stats.Invalidations != 5 {
		t.Errorf("Expected 5 invalidations, got %d", stats.Invalidations)
	}
}

func TestMaxEntries_Eviction(t *testing.T) {
	cfg := &Config{
		DefaultTTL:      1 * time.Hour,
		RealtimeTTL:     30 * time.Second,
		MaxEntries:      3, // Very small for testing
		CleanupInterval: 1 * time.Hour,
	}
	c := NewWithConfig(cfg)
	defer c.Close()

	// Add entries up to and beyond max
	c.Set("key-1", makeTestResponse(1), "tenant-1", 0)
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	c.Set("key-2", makeTestResponse(2), "tenant-1", 0)
	time.Sleep(1 * time.Millisecond)
	c.Set("key-3", makeTestResponse(3), "tenant-1", 0)

	if c.Size() != 3 {
		t.Fatalf("Expected 3 entries, got %d", c.Size())
	}

	time.Sleep(1 * time.Millisecond)
	// This should evict the oldest (key-1)
	c.Set("key-4", makeTestResponse(4), "tenant-1", 0)

	if c.Size() != 3 {
		t.Errorf("Expected 3 entries after eviction, got %d", c.Size())
	}

	// key-1 should be evicted
	if c.Get("key-1") != nil {
		t.Error("key-1 should have been evicted")
	}

	// key-4 should exist
	if c.Get("key-4") == nil {
		t.Error("key-4 should exist")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New()
	defer c.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				c.Set(key, makeTestResponse(int64(id*numOperations+j)), fmt.Sprintf("tenant-%d", id%3), 0)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				c.Get(key)
			}
		}(i)
	}

	// Concurrent invalidations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.InvalidateForTenant(fmt.Sprintf("tenant-%d", id%3))
		}(i)
	}

	wg.Wait()

	// Should not panic or deadlock - if we get here, test passes
	stats := c.Stats()
	t.Logf("After concurrent access: hits=%d, misses=%d, evictions=%d, invalidations=%d",
		stats.Hits, stats.Misses, stats.Evictions, stats.Invalidations)
}

func TestCleanupLoop(t *testing.T) {
	cfg := &Config{
		DefaultTTL:      50 * time.Millisecond,
		RealtimeTTL:     25 * time.Millisecond,
		MaxEntries:      1000,
		CleanupInterval: 30 * time.Millisecond, // Fast cleanup for testing
	}
	c := NewWithConfig(cfg)
	defer c.Close()

	// Add entries
	for i := 0; i < 5; i++ {
		c.Set(fmt.Sprintf("key-%d", i), makeTestResponse(int64(i)), "tenant-1", 50*time.Millisecond)
	}

	if c.Size() != 5 {
		t.Fatalf("Expected 5 entries, got %d", c.Size())
	}

	// Wait for entries to expire and cleanup to run
	time.Sleep(100 * time.Millisecond)

	// All entries should be cleaned up
	if c.Size() != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", c.Size())
	}

	stats := c.Stats()
	if stats.Evictions != 5 {
		t.Errorf("Expected 5 evictions from cleanup, got %d", stats.Evictions)
	}
}

func TestClose(t *testing.T) {
	c := New()

	// Add some entries
	c.Set("key-1", makeTestResponse(1), "tenant-1", 0)
	c.Set("key-2", makeTestResponse(2), "tenant-1", 0)

	// Close should clear the cache
	c.Close()

	if c.Size() != 0 {
		t.Errorf("Cache should be empty after Close, got %d", c.Size())
	}
}

func TestCacheEntry_IsExpired(t *testing.T) {
	entry := &CacheEntry{
		Response:  makeTestResponse(1),
		CreatedAt: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Minute), // Already expired
		TenantID:  "tenant-1",
	}

	if !entry.IsExpired() {
		t.Error("Entry should be expired")
	}

	entry2 := &CacheEntry{
		Response:  makeTestResponse(1),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour), // Not expired
		TenantID:  "tenant-1",
	}

	if entry2.IsExpired() {
		t.Error("Entry should not be expired")
	}
}

func TestMetrics_ZeroState(t *testing.T) {
	m := &Metrics{}

	// Hit rate with no operations should be 0
	if m.HitRate() != 0.0 {
		t.Errorf("Expected 0 hit rate, got %f", m.HitRate())
	}

	if m.MissRate() != 1.0 {
		t.Errorf("Expected 1.0 miss rate, got %f", m.MissRate())
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultTTL != DefaultTTL {
		t.Errorf("Expected DefaultTTL %v, got %v", DefaultTTL, cfg.DefaultTTL)
	}

	if cfg.RealtimeTTL != RealtimeTTL {
		t.Errorf("Expected RealtimeTTL %v, got %v", RealtimeTTL, cfg.RealtimeTTL)
	}

	if cfg.MaxEntries != DefaultMaxEntries {
		t.Errorf("Expected MaxEntries %d, got %d", DefaultMaxEntries, cfg.MaxEntries)
	}

	if cfg.CleanupInterval != CleanupInterval {
		t.Errorf("Expected CleanupInterval %v, got %v", CleanupInterval, cfg.CleanupInterval)
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkGet(b *testing.B) {
	c := New()
	defer c.Close()

	c.Set("benchmark-key", makeTestResponse(1), "tenant-1", 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("benchmark-key")
	}
}

func BenchmarkSet(b *testing.B) {
	c := New()
	defer c.Close()

	response := makeTestResponse(1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(fmt.Sprintf("key-%d", i), response, "tenant-1", 0)
	}
}

func BenchmarkGenerateKey(b *testing.B) {
	filters := &searchv1.FilterOptions{
		ContentTypes: []string{"email", "document"},
		Tags:         []string{"important", "urgent"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateKey("test query", "tenant-1", filters, searchv1.SortOrder_SORT_ORDER_RELEVANCE, 10, 0)
	}
}

func BenchmarkConcurrentAccess(b *testing.B) {
	c := New()
	defer c.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), makeTestResponse(int64(i)), "tenant-1", 0)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				c.Get(key)
			} else {
				c.Set(key, makeTestResponse(int64(i)), "tenant-1", 0)
			}
			i++
		}
	})
}
