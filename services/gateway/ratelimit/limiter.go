// Package ratelimit provides rate limiting functionality for the API Gateway.
// It implements a token bucket algorithm with per-tenant and per-endpoint limits.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ContextKey type for context values to avoid collisions.
type ContextKey string

// Context keys for rate limiting.
const (
	// TenantIDKey is the context key for the tenant identifier.
	TenantIDKey ContextKey = "tenant_id"
)

// Errors returned by the rate limiter.
var (
	// ErrRateLimitExceeded is returned when a request exceeds the rate limit.
	ErrRateLimitExceeded = fmt.Errorf("rate limit exceeded")
)

// Config holds the configuration for the rate limiter.
type Config struct {
	// DefaultRPS is the default requests per second for tenants without specific limits.
	DefaultRPS float64

	// DefaultBurst is the default burst capacity for tenants without specific limits.
	DefaultBurst int

	// TenantLimits maps tenant IDs to their specific rate limits.
	TenantLimits map[string]TenantLimit

	// EndpointLimits maps endpoint patterns to their specific rate limits.
	EndpointLimits map[string]EndpointLimit

	// CleanupInterval is how often to clean up expired buckets.
	CleanupInterval time.Duration

	// BucketTTL is how long to keep inactive buckets before cleanup.
	BucketTTL time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DefaultRPS:      100.0,
		DefaultBurst:    150,
		TenantLimits:    make(map[string]TenantLimit),
		EndpointLimits:  make(map[string]EndpointLimit),
		CleanupInterval: 5 * time.Minute,
		BucketTTL:       10 * time.Minute,
	}
}

// TenantLimit defines rate limits for a specific tenant.
type TenantLimit struct {
	// RPS is the requests per second allowed for this tenant.
	RPS float64

	// Burst is the maximum burst capacity for this tenant.
	Burst int

	// Disabled indicates if rate limiting is disabled for this tenant (e.g., internal services).
	Disabled bool
}

// EndpointLimit defines rate limits for a specific endpoint.
type EndpointLimit struct {
	// RPS is the requests per second allowed for this endpoint.
	RPS float64

	// Burst is the maximum burst capacity for this endpoint.
	Burst int

	// PerTenant indicates if this limit should be applied per-tenant (true) or globally (false).
	PerTenant bool
}

// tokenBucket implements the token bucket algorithm.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// newTokenBucket creates a new token bucket with the specified parameters.
func newTokenBucket(rps float64, burst int) *tokenBucket {
	maxTokens := float64(burst)
	return &tokenBucket{
		tokens:     maxTokens, // Start with full bucket
		maxTokens:  maxTokens,
		refillRate: rps,
		lastRefill: time.Now(),
	}
}

// allow checks if a request is allowed and consumes a token if so.
// Returns true if allowed, false if rate limited.
func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	// Check if we have a token available
	if tb.tokens < 1.0 {
		return false
	}

	// Consume a token
	tb.tokens--
	return true
}

// tokensAvailable returns the current number of available tokens.
func (tb *tokenBucket) tokensAvailable() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Calculate current tokens without updating state
	tokens := tb.tokens + elapsed*tb.refillRate
	if tokens > tb.maxTokens {
		tokens = tb.maxTokens
	}
	return tokens
}

// retryAfter returns the time to wait before the next request is allowed.
func (tb *tokenBucket) retryAfter() time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.tokens >= 1.0 {
		return 0
	}

	// Calculate how long until we have 1 token
	tokensNeeded := 1.0 - tb.tokens
	seconds := tokensNeeded / tb.refillRate
	return time.Duration(seconds*1000) * time.Millisecond
}

// bucketKey identifies a unique rate limit bucket.
type bucketKey struct {
	tenantID string
	endpoint string
}

// RateLimiter manages rate limiting for the gateway.
type RateLimiter struct {
	config  Config
	buckets map[bucketKey]*tokenBucket
	mu      sync.RWMutex
	done    chan struct{}
}

// NewRateLimiter creates a new RateLimiter with the given configuration.
func NewRateLimiter(config Config) *RateLimiter {
	rl := &RateLimiter{
		config:  config,
		buckets: make(map[bucketKey]*tokenBucket),
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request is allowed for the given tenant and endpoint.
// Returns nil if allowed, ErrRateLimitExceeded if rate limited.
func (rl *RateLimiter) Allow(ctx context.Context, tenantID, endpoint string) error {
	// Check if tenant has rate limiting disabled
	if limit, ok := rl.config.TenantLimits[tenantID]; ok && limit.Disabled {
		return nil
	}

	// Determine the limits to apply
	rps, burst := rl.getLimits(tenantID, endpoint)

	// Get or create the bucket
	bucket := rl.getBucket(tenantID, endpoint, rps, burst)

	if !bucket.allow() {
		return ErrRateLimitExceeded
	}

	return nil
}

// AllowWithInfo is like Allow but returns additional information about the rate limit status.
func (rl *RateLimiter) AllowWithInfo(ctx context.Context, tenantID, endpoint string) (*LimitInfo, error) {
	// Check if tenant has rate limiting disabled
	if limit, ok := rl.config.TenantLimits[tenantID]; ok && limit.Disabled {
		return &LimitInfo{
			Allowed:    true,
			Remaining:  -1, // Unlimited
			Limit:      -1, // Unlimited
			RetryAfter: 0,
		}, nil
	}

	// Determine the limits to apply
	rps, burst := rl.getLimits(tenantID, endpoint)

	// Get or create the bucket
	bucket := rl.getBucket(tenantID, endpoint, rps, burst)

	allowed := bucket.allow()
	info := &LimitInfo{
		Allowed:    allowed,
		Remaining:  int(bucket.tokensAvailable()),
		Limit:      burst,
		RetryAfter: bucket.retryAfter(),
	}

	if !allowed {
		return info, ErrRateLimitExceeded
	}

	return info, nil
}

// LimitInfo contains information about the rate limit status.
type LimitInfo struct {
	// Allowed indicates if the request was allowed.
	Allowed bool

	// Remaining is the number of remaining requests in the current window.
	Remaining int

	// Limit is the maximum number of requests allowed in the window.
	Limit int

	// RetryAfter is the time to wait before retrying if rate limited.
	RetryAfter time.Duration
}

// getLimits returns the RPS and burst limits for a tenant/endpoint combination.
func (rl *RateLimiter) getLimits(tenantID, endpoint string) (float64, int) {
	rps := rl.config.DefaultRPS
	burst := rl.config.DefaultBurst

	// Apply tenant-specific limits if configured
	if tenantLimit, ok := rl.config.TenantLimits[tenantID]; ok {
		if tenantLimit.RPS > 0 {
			rps = tenantLimit.RPS
		}
		if tenantLimit.Burst > 0 {
			burst = tenantLimit.Burst
		}
	}

	// Apply endpoint-specific limits if configured (overrides tenant limits)
	if endpointLimit, ok := rl.config.EndpointLimits[endpoint]; ok {
		if endpointLimit.RPS > 0 {
			rps = endpointLimit.RPS
		}
		if endpointLimit.Burst > 0 {
			burst = endpointLimit.Burst
		}
	}

	return rps, burst
}

// getBucket gets or creates a token bucket for the given tenant/endpoint.
func (rl *RateLimiter) getBucket(tenantID, endpoint string, rps float64, burst int) *tokenBucket {
	// Determine the bucket key based on endpoint configuration
	key := bucketKey{tenantID: tenantID, endpoint: endpoint}

	// Check if endpoint limit is global (not per-tenant)
	if endpointLimit, ok := rl.config.EndpointLimits[endpoint]; ok && !endpointLimit.PerTenant {
		key.tenantID = "" // Use global bucket for this endpoint
	}

	// Try read lock first
	rl.mu.RLock()
	bucket, ok := rl.buckets[key]
	rl.mu.RUnlock()

	if ok {
		return bucket
	}

	// Need to create a new bucket
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if bucket, ok = rl.buckets[key]; ok {
		return bucket
	}

	bucket = newTokenBucket(rps, burst)
	rl.buckets[key] = bucket
	return bucket
}

// cleanup periodically removes inactive buckets.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupBuckets()
		case <-rl.done:
			return
		}
	}
}

// cleanupBuckets removes buckets that haven't been used recently.
func (rl *RateLimiter) cleanupBuckets() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, bucket := range rl.buckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastRefill) > rl.config.BucketTTL {
			delete(rl.buckets, key)
		}
		bucket.mu.Unlock()
	}
}

// Close stops the rate limiter cleanup goroutine.
func (rl *RateLimiter) Close() {
	close(rl.done)
}

// Stats returns statistics about the rate limiter.
func (rl *RateLimiter) Stats() RateLimiterStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return RateLimiterStats{
		ActiveBuckets: len(rl.buckets),
		DefaultRPS:    rl.config.DefaultRPS,
		DefaultBurst:  rl.config.DefaultBurst,
	}
}

// RateLimiterStats contains statistics about the rate limiter.
type RateLimiterStats struct {
	// ActiveBuckets is the number of active token buckets.
	ActiveBuckets int

	// DefaultRPS is the default requests per second limit.
	DefaultRPS float64

	// DefaultBurst is the default burst capacity.
	DefaultBurst int
}

// GetTenantID extracts the tenant ID from the context.
// Returns empty string if not found.
func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	return ""
}

// WithTenantID returns a new context with the tenant ID set.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// ConfigFromExport creates a rate limiter Config from exported configuration.
// This is used to convert the gateway config to the ratelimit package's Config type.
func ConfigFromExport(
	defaultRPS float64,
	defaultBurst int,
	cleanupInterval, bucketTTL time.Duration,
	tenantLimits map[string]TenantLimit,
	endpointLimits map[string]EndpointLimit,
) Config {
	cfg := Config{
		DefaultRPS:      defaultRPS,
		DefaultBurst:    defaultBurst,
		CleanupInterval: cleanupInterval,
		BucketTTL:       bucketTTL,
		TenantLimits:    tenantLimits,
		EndpointLimits:  endpointLimits,
	}

	// Ensure maps are initialized
	if cfg.TenantLimits == nil {
		cfg.TenantLimits = make(map[string]TenantLimit)
	}
	if cfg.EndpointLimits == nil {
		cfg.EndpointLimits = make(map[string]EndpointLimit)
	}

	return cfg
}
