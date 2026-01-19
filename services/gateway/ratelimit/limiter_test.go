package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		bucket := newTokenBucket(10.0, 10) // 10 RPS, burst of 10

		// Should allow burst of requests up to capacity
		for i := 0; i < 10; i++ {
			if !bucket.allow() {
				t.Errorf("request %d should have been allowed", i)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		bucket := newTokenBucket(10.0, 5) // 10 RPS, burst of 5

		// Drain the bucket
		for i := 0; i < 5; i++ {
			bucket.allow()
		}

		// Next request should be blocked
		if bucket.allow() {
			t.Error("request should have been blocked after exceeding burst")
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		bucket := newTokenBucket(100.0, 5) // 100 RPS, burst of 5

		// Drain the bucket
		for i := 0; i < 5; i++ {
			bucket.allow()
		}

		// Wait for refill (100 RPS = 1 token per 10ms)
		time.Sleep(15 * time.Millisecond)

		// Should have at least 1 token now
		if !bucket.allow() {
			t.Error("request should have been allowed after refill")
		}
	})

	t.Run("caps tokens at max", func(t *testing.T) {
		bucket := newTokenBucket(1000.0, 5) // 1000 RPS, burst of 5

		// Wait longer than needed to refill
		time.Sleep(20 * time.Millisecond)

		// Should still only have 5 tokens (the max)
		for i := 0; i < 5; i++ {
			if !bucket.allow() {
				t.Errorf("request %d should have been allowed", i)
			}
		}

		// 6th should be blocked
		if bucket.allow() {
			t.Error("request should have been blocked after exceeding max tokens")
		}
	})
}

func TestTokenBucket_RetryAfter(t *testing.T) {
	t.Run("returns zero when tokens available", func(t *testing.T) {
		bucket := newTokenBucket(10.0, 5)

		retryAfter := bucket.retryAfter()
		if retryAfter != 0 {
			t.Errorf("expected retry after 0, got %v", retryAfter)
		}
	})

	t.Run("returns positive duration when no tokens", func(t *testing.T) {
		bucket := newTokenBucket(10.0, 2) // 10 RPS, burst of 2

		// Drain the bucket
		bucket.allow()
		bucket.allow()

		retryAfter := bucket.retryAfter()
		if retryAfter <= 0 {
			t.Errorf("expected positive retry after, got %v", retryAfter)
		}

		// At 10 RPS, should need ~100ms to get 1 token
		if retryAfter > 150*time.Millisecond {
			t.Errorf("retry after too long: %v", retryAfter)
		}
	})
}

func TestRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within default limit", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 10
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Should allow burst of requests
		for i := 0; i < 10; i++ {
			err := limiter.Allow(ctx, "tenant1", "/api/test")
			if err != nil {
				t.Errorf("request %d should have been allowed: %v", i, err)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 5
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Drain the limit
		for i := 0; i < 5; i++ {
			limiter.Allow(ctx, "tenant1", "/api/test")
		}

		// Next should be blocked
		err := limiter.Allow(ctx, "tenant1", "/api/test")
		if err != ErrRateLimitExceeded {
			t.Errorf("expected ErrRateLimitExceeded, got %v", err)
		}
	})

	t.Run("applies per-tenant limits", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 10
		cfg.TenantLimits = map[string]TenantLimit{
			"premium": {RPS: 200.0, Burst: 20},
			"basic":   {RPS: 50.0, Burst: 3},
		}
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Premium tenant should have higher burst
		for i := 0; i < 20; i++ {
			err := limiter.Allow(ctx, "premium", "/api/test")
			if err != nil {
				t.Errorf("premium request %d should have been allowed: %v", i, err)
			}
		}

		// Basic tenant should have lower burst
		for i := 0; i < 3; i++ {
			err := limiter.Allow(ctx, "basic", "/api/test")
			if err != nil {
				t.Errorf("basic request %d should have been allowed: %v", i, err)
			}
		}
		err := limiter.Allow(ctx, "basic", "/api/test")
		if err != ErrRateLimitExceeded {
			t.Error("basic request should have been blocked after exceeding limit")
		}
	})

	t.Run("disabled tenant bypasses rate limiting", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultBurst = 3
		cfg.TenantLimits = map[string]TenantLimit{
			"internal": {Disabled: true},
		}
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Internal tenant should never be rate limited
		for i := 0; i < 100; i++ {
			err := limiter.Allow(ctx, "internal", "/api/test")
			if err != nil {
				t.Errorf("internal request %d should have been allowed: %v", i, err)
			}
		}
	})

	t.Run("applies per-endpoint limits", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 10
		cfg.EndpointLimits = map[string]EndpointLimit{
			"/api/expensive": {RPS: 10.0, Burst: 2, PerTenant: true},
		}
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Expensive endpoint should have lower limit
		err := limiter.Allow(ctx, "tenant1", "/api/expensive")
		if err != nil {
			t.Errorf("first expensive request should have been allowed: %v", err)
		}
		err = limiter.Allow(ctx, "tenant1", "/api/expensive")
		if err != nil {
			t.Errorf("second expensive request should have been allowed: %v", err)
		}
		err = limiter.Allow(ctx, "tenant1", "/api/expensive")
		if err != ErrRateLimitExceeded {
			t.Error("third expensive request should have been blocked")
		}

		// Other endpoints should use default
		for i := 0; i < 10; i++ {
			err := limiter.Allow(ctx, "tenant1", "/api/normal")
			if err != nil {
				t.Errorf("normal request %d should have been allowed: %v", i, err)
			}
		}
	})

	t.Run("global endpoint limits apply across tenants", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 10
		cfg.EndpointLimits = map[string]EndpointLimit{
			"/api/shared": {RPS: 10.0, Burst: 3, PerTenant: false}, // Global limit
		}
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Requests from different tenants should share the same limit
		err := limiter.Allow(ctx, "tenant1", "/api/shared")
		if err != nil {
			t.Error("request 1 should have been allowed")
		}
		err = limiter.Allow(ctx, "tenant2", "/api/shared")
		if err != nil {
			t.Error("request 2 should have been allowed")
		}
		err = limiter.Allow(ctx, "tenant3", "/api/shared")
		if err != nil {
			t.Error("request 3 should have been allowed")
		}
		err = limiter.Allow(ctx, "tenant4", "/api/shared")
		if err != ErrRateLimitExceeded {
			t.Error("request 4 should have been blocked (global limit)")
		}
	})

	t.Run("different tenants have separate limits", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 3
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Exhaust tenant1's limit
		for i := 0; i < 3; i++ {
			limiter.Allow(ctx, "tenant1", "/api/test")
		}

		// tenant2 should still have full limit
		for i := 0; i < 3; i++ {
			err := limiter.Allow(ctx, "tenant2", "/api/test")
			if err != nil {
				t.Errorf("tenant2 request %d should have been allowed: %v", i, err)
			}
		}
	})
}

func TestRateLimiter_AllowWithInfo(t *testing.T) {
	t.Run("returns correct limit info", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 100.0
		cfg.DefaultBurst = 5
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		info, err := limiter.AllowWithInfo(ctx, "tenant1", "/api/test")
		if err != nil {
			t.Errorf("request should have been allowed: %v", err)
		}

		if !info.Allowed {
			t.Error("info.Allowed should be true")
		}
		if info.Limit != 5 {
			t.Errorf("expected limit 5, got %d", info.Limit)
		}
		// Remaining should be less than limit after consuming a token
		if info.Remaining >= info.Limit {
			t.Errorf("remaining %d should be less than limit %d", info.Remaining, info.Limit)
		}
	})

	t.Run("returns unlimited for disabled tenants", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.TenantLimits = map[string]TenantLimit{
			"internal": {Disabled: true},
		}
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		info, err := limiter.AllowWithInfo(ctx, "internal", "/api/test")
		if err != nil {
			t.Errorf("request should have been allowed: %v", err)
		}

		if info.Limit != -1 {
			t.Errorf("expected unlimited (-1), got limit %d", info.Limit)
		}
		if info.Remaining != -1 {
			t.Errorf("expected unlimited (-1), got remaining %d", info.Remaining)
		}
	})

	t.Run("returns retry info when rate limited", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.DefaultRPS = 10.0
		cfg.DefaultBurst = 2
		limiter := NewRateLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Exhaust limit
		limiter.Allow(ctx, "tenant1", "/api/test")
		limiter.Allow(ctx, "tenant1", "/api/test")

		info, err := limiter.AllowWithInfo(ctx, "tenant1", "/api/test")
		if err != ErrRateLimitExceeded {
			t.Errorf("expected ErrRateLimitExceeded, got %v", err)
		}

		if info.Allowed {
			t.Error("info.Allowed should be false")
		}
		if info.RetryAfter <= 0 {
			t.Error("RetryAfter should be positive")
		}
	})
}

func TestRateLimiter_Concurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultRPS = 1000.0
	cfg.DefaultBurst = 100
	limiter := NewRateLimiter(cfg)
	defer limiter.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// Run concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := limiter.Allow(ctx, "tenant1", "/api/test")
			allowed <- (err == nil)
		}()
	}

	wg.Wait()
	close(allowed)

	// Count allowed requests
	count := 0
	for a := range allowed {
		if a {
			count++
		}
	}

	// Should allow approximately burst count
	// Due to concurrent access and timing, allow some variance
	if count < 90 || count > 110 {
		t.Errorf("expected ~100 allowed requests, got %d", count)
	}
}

func TestRateLimiter_Stats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultRPS = 100.0
	cfg.DefaultBurst = 10
	limiter := NewRateLimiter(cfg)
	defer limiter.Close()

	// Initially no buckets
	stats := limiter.Stats()
	if stats.ActiveBuckets != 0 {
		t.Errorf("expected 0 active buckets, got %d", stats.ActiveBuckets)
	}

	ctx := context.Background()

	// Make requests from different tenants/endpoints
	limiter.Allow(ctx, "tenant1", "/api/a")
	limiter.Allow(ctx, "tenant1", "/api/b")
	limiter.Allow(ctx, "tenant2", "/api/a")

	stats = limiter.Stats()
	if stats.ActiveBuckets != 3 {
		t.Errorf("expected 3 active buckets, got %d", stats.ActiveBuckets)
	}
	if stats.DefaultRPS != 100.0 {
		t.Errorf("expected default RPS 100.0, got %f", stats.DefaultRPS)
	}
	if stats.DefaultBurst != 10 {
		t.Errorf("expected default burst 10, got %d", stats.DefaultBurst)
	}
}

func TestContextHelpers(t *testing.T) {
	t.Run("WithTenantID and GetTenantID", func(t *testing.T) {
		ctx := context.Background()

		// Should return empty string for context without tenant
		if tenantID := GetTenantID(ctx); tenantID != "" {
			t.Errorf("expected empty string, got %s", tenantID)
		}

		// Should return tenant ID after setting it
		ctx = WithTenantID(ctx, "test-tenant")
		if tenantID := GetTenantID(ctx); tenantID != "test-tenant" {
			t.Errorf("expected test-tenant, got %s", tenantID)
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultRPS != 100.0 {
		t.Errorf("expected default RPS 100.0, got %f", cfg.DefaultRPS)
	}
	if cfg.DefaultBurst != 150 {
		t.Errorf("expected default burst 150, got %d", cfg.DefaultBurst)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("expected cleanup interval 5m, got %v", cfg.CleanupInterval)
	}
	if cfg.BucketTTL != 10*time.Minute {
		t.Errorf("expected bucket TTL 10m, got %v", cfg.BucketTTL)
	}
}
