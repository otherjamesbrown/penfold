package summarization

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCache_BasicOperations(t *testing.T) {
	cache := NewMemoryCache(nil)
	defer cache.Close()

	ctx := context.Background()

	req := &SummaryRequest{
		Content:  "Test content for caching",
		Strategy: StrategyTypeExtractive,
		Length:   SummaryLengthParagraph,
		TenantID: "test-tenant",
	}
	req.SetDefaults()

	result := &SummaryResult{
		Summary:   "This is a cached summary",
		KeyPoints: []string{"Point 1", "Point 2"},
		ModelUsed: "test-model",
	}

	// Test Set
	err := cache.Set(ctx, req, result, 1*time.Hour)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Test Get
	cached, err := cache.Get(ctx, req)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if cached.Summary != result.Summary {
		t.Errorf("Cached summary = %q, want %q", cached.Summary, result.Summary)
	}

	if len(cached.KeyPoints) != len(result.KeyPoints) {
		t.Errorf("Cached key points count = %d, want %d", len(cached.KeyPoints), len(result.KeyPoints))
	}

	// Test Delete
	err = cache.Delete(ctx, req)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Should be cache miss after delete
	_, err = cache.Get(ctx, req)
	if err != ErrCacheMiss {
		t.Errorf("Expected ErrCacheMiss after delete, got %v", err)
	}
}

func TestMemoryCache_Expiration(t *testing.T) {
	cfg := &MemoryCacheConfig{
		MaxEntries:      100,
		DefaultTTL:      100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	}
	cache := NewMemoryCache(cfg)
	defer cache.Close()

	ctx := context.Background()

	req := &SummaryRequest{
		Content:  "Expiring content",
		Strategy: StrategyTypeExtractive,
	}
	req.SetDefaults()

	result := &SummaryResult{
		Summary: "This will expire",
	}

	// Set with short TTL
	err := cache.Set(ctx, req, result, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Should be available immediately
	_, err = cache.Get(ctx, req)
	if err != nil {
		t.Error("Expected cache hit before expiration")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	_, err = cache.Get(ctx, req)
	if err != ErrCacheMiss && err != ErrCacheExpired {
		t.Errorf("Expected cache miss/expired after TTL, got %v", err)
	}
}

func TestMemoryCache_MaxEntries(t *testing.T) {
	cfg := &MemoryCacheConfig{
		MaxEntries:      3,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 1 * time.Hour, // Don't clean up during test
	}
	cache := NewMemoryCache(cfg)
	defer cache.Close()

	ctx := context.Background()

	// Add entries up to max
	for i := 0; i < 5; i++ {
		req := &SummaryRequest{
			Content:  string(rune('A' + i)),
			Strategy: StrategyTypeExtractive,
		}
		req.SetDefaults()

		result := &SummaryResult{
			Summary: "Summary " + string(rune('A'+i)),
		}

		cache.Set(ctx, req, result, 0)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	stats := cache.Stats()
	if stats.Size > cfg.MaxEntries {
		t.Errorf("Cache size = %d, exceeded max %d", stats.Size, cfg.MaxEntries)
	}
}

func TestMemoryCache_Stats(t *testing.T) {
	cache := NewMemoryCache(nil)
	defer cache.Close()

	ctx := context.Background()

	req := &SummaryRequest{
		Content:  "Stats test content",
		Strategy: StrategyTypeExtractive,
	}
	req.SetDefaults()

	result := &SummaryResult{
		Summary: "Stats test summary",
	}

	// Initial stats
	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("Expected zero hits and misses initially")
	}

	// Cache miss
	cache.Get(ctx, req)
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	// Add entry and get hit
	cache.Set(ctx, req, result, 0)
	cache.Get(ctx, req)
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}

	// Check hit rate
	hitRate := stats.HitRate()
	if hitRate < 0.4 || hitRate > 0.6 { // 1 hit, 1 miss = 0.5
		t.Errorf("Expected hit rate around 0.5, got %f", hitRate)
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(nil)
	defer cache.Close()

	ctx := context.Background()

	// Add multiple entries
	for i := 0; i < 5; i++ {
		req := &SummaryRequest{
			Content:  string(rune('A' + i)),
			Strategy: StrategyTypeExtractive,
		}
		req.SetDefaults()

		result := &SummaryResult{
			Summary: "Summary " + string(rune('A'+i)),
		}

		cache.Set(ctx, req, result, 0)
	}

	// Verify entries exist
	stats := cache.Stats()
	if stats.Size == 0 {
		t.Error("Expected non-zero cache size before clear")
	}

	// Clear cache
	err := cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify empty
	stats = cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected zero cache size after clear, got %d", stats.Size)
	}
}

func TestGenerateCacheKey(t *testing.T) {
	req1 := &SummaryRequest{
		Content:  "Test content",
		Strategy: StrategyTypeExtractive,
		Length:   SummaryLengthParagraph,
		Format:   SummaryFormatProse,
		TenantID: "tenant1",
	}

	req2 := &SummaryRequest{
		Content:  "Test content",
		Strategy: StrategyTypeExtractive,
		Length:   SummaryLengthParagraph,
		Format:   SummaryFormatProse,
		TenantID: "tenant1",
	}

	req3 := &SummaryRequest{
		Content:  "Different content",
		Strategy: StrategyTypeExtractive,
		Length:   SummaryLengthParagraph,
		Format:   SummaryFormatProse,
		TenantID: "tenant1",
	}

	req4 := &SummaryRequest{
		Content:  "Test content",
		Strategy: StrategyTypeAbstractive, // Different strategy
		Length:   SummaryLengthParagraph,
		Format:   SummaryFormatProse,
		TenantID: "tenant1",
	}

	key1 := generateCacheKey(req1)
	key2 := generateCacheKey(req2)
	key3 := generateCacheKey(req3)
	key4 := generateCacheKey(req4)

	// Same requests should have same key
	if key1 != key2 {
		t.Error("Identical requests should produce same key")
	}

	// Different content should have different key
	if key1 == key3 {
		t.Error("Different content should produce different keys")
	}

	// Different strategy should have different key
	if key1 == key4 {
		t.Error("Different strategy should produce different keys")
	}
}

func TestHashContent(t *testing.T) {
	hash1 := hashContent("test content")
	hash2 := hashContent("test content")
	hash3 := hashContent("different content")

	// Same content should produce same hash
	if hash1 != hash2 {
		t.Error("Same content should produce same hash")
	}

	// Different content should produce different hash
	if hash1 == hash3 {
		t.Error("Different content should produce different hash")
	}

	// Hash should be a reasonable length
	if len(hash1) == 0 {
		t.Error("Hash should not be empty")
	}
}

func TestNullCache(t *testing.T) {
	cache := NewNullCache()
	ctx := context.Background()

	req := &SummaryRequest{
		Content:  "Test",
		Strategy: StrategyTypeExtractive,
	}

	result := &SummaryResult{
		Summary: "Summary",
	}

	// Set should not error
	err := cache.Set(ctx, req, result, 0)
	if err != nil {
		t.Errorf("NullCache Set() error = %v", err)
	}

	// Get should always return miss
	_, err = cache.Get(ctx, req)
	if err != ErrCacheMiss {
		t.Errorf("NullCache Get() should return ErrCacheMiss, got %v", err)
	}

	// Stats should be empty
	stats := cache.Stats()
	if stats.Size != 0 || stats.Hits != 0 {
		t.Error("NullCache should have empty stats")
	}
}

func TestCacheEntry_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "future expiry",
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  false,
		},
		{
			name:      "past expiry",
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &CacheEntry{
				ExpiresAt: tt.expiresAt,
			}
			if got := entry.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCacheStats_HitRate(t *testing.T) {
	tests := []struct {
		name     string
		hits     uint64
		misses   uint64
		expected float64
	}{
		{"all hits", 10, 0, 1.0},
		{"all misses", 0, 10, 0.0},
		{"half and half", 5, 5, 0.5},
		{"no requests", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := CacheStats{
				Hits:   tt.hits,
				Misses: tt.misses,
			}
			if got := stats.HitRate(); got != tt.expected {
				t.Errorf("HitRate() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTieredCache(t *testing.T) {
	l1 := NewMemoryCache(&MemoryCacheConfig{
		MaxEntries:      10,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 1 * time.Hour,
	})
	defer l1.Close()

	// No L2 for this test (would need Redis)
	cache := NewTieredCache(l1, nil)

	ctx := context.Background()

	req := &SummaryRequest{
		Content:  "Tiered cache test",
		Strategy: StrategyTypeExtractive,
	}
	req.SetDefaults()

	result := &SummaryResult{
		Summary:   "Tiered summary",
		KeyPoints: []string{"Point 1"},
	}

	// Set in tiered cache
	err := cache.Set(ctx, req, result, 0)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get should find in L1
	cached, err := cache.Get(ctx, req)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if cached.Summary != result.Summary {
		t.Errorf("Cached summary = %q, want %q", cached.Summary, result.Summary)
	}

	// Clear both levels
	err = cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Should be miss after clear
	_, err = cache.Get(ctx, req)
	if err != ErrCacheMiss {
		t.Errorf("Expected ErrCacheMiss after clear, got %v", err)
	}
}

// Benchmark tests

func BenchmarkMemoryCache_Get(b *testing.B) {
	cache := NewMemoryCache(nil)
	defer cache.Close()

	ctx := context.Background()

	req := &SummaryRequest{
		Content:  "Benchmark content for cache",
		Strategy: StrategyTypeExtractive,
	}
	req.SetDefaults()

	result := &SummaryResult{
		Summary:   "Benchmark summary",
		KeyPoints: []string{"Point 1", "Point 2"},
	}

	cache.Set(ctx, req, result, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, req)
	}
}

func BenchmarkMemoryCache_Set(b *testing.B) {
	cache := NewMemoryCache(&MemoryCacheConfig{
		MaxEntries:      100000,
		DefaultTTL:      1 * time.Hour,
		CleanupInterval: 1 * time.Hour,
	})
	defer cache.Close()

	ctx := context.Background()

	result := &SummaryResult{
		Summary:   "Benchmark summary",
		KeyPoints: []string{"Point 1", "Point 2"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &SummaryRequest{
			Content:  string(rune(i % 1000)), // Vary content
			Strategy: StrategyTypeExtractive,
		}
		req.SetDefaults()
		cache.Set(ctx, req, result, 0)
	}
}

func BenchmarkGenerateCacheKey(b *testing.B) {
	req := &SummaryRequest{
		Content:  "Benchmark content for key generation",
		Strategy: StrategyTypeExtractive,
		Length:   SummaryLengthParagraph,
		Format:   SummaryFormatProse,
		TenantID: "tenant-123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateCacheKey(req)
	}
}

func BenchmarkHashContent(b *testing.B) {
	content := "This is some sample content to hash for benchmarking purposes."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashContent(content)
	}
}
