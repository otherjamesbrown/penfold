// Package scheduler provides intelligent task scheduling for Gmail sync operations.
package scheduler

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket implements a token bucket rate limiter for per-account throttling.
type TokenBucket struct {
	mu sync.Mutex

	// Current token count.
	tokens float64

	// Maximum tokens (bucket capacity).
	maxTokens float64

	// Refill rate (tokens per second).
	refillRate float64

	// Last refill time.
	lastRefill time.Time

	// Backoff until this time (rate limit enforced).
	backoffUntil time.Time
}

// NewTokenBucket creates a new token bucket with the given capacity and refill rate.
func NewTokenBucket(maxTokens float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens, // Start full.
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Take attempts to take a token from the bucket.
// Returns true if a token was available, false otherwise.
func (tb *TokenBucket) Take() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Check backoff.
	now := time.Now()
	if !tb.backoffUntil.IsZero() && now.Before(tb.backoffUntil) {
		return false
	}

	// Refill tokens.
	tb.refill(now)

	// Try to take a token.
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// TakeN attempts to take n tokens from the bucket.
// Returns true if all tokens were available, false otherwise.
func (tb *TokenBucket) TakeN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	if !tb.backoffUntil.IsZero() && now.Before(tb.backoffUntil) {
		return false
	}

	tb.refill(now)

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}

	return false
}

// Wait blocks until a token is available or context is cancelled.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		tb.mu.Lock()
		now := time.Now()

		// Check backoff.
		if !tb.backoffUntil.IsZero() && now.Before(tb.backoffUntil) {
			waitDuration := tb.backoffUntil.Sub(now)
			tb.mu.Unlock()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDuration):
				continue
			}
		}

		// Refill tokens.
		tb.refill(now)

		// Check if token available.
		if tb.tokens >= 1 {
			tb.tokens--
			tb.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token.
		tokensNeeded := 1 - tb.tokens
		waitDuration := time.Duration(tokensNeeded / tb.refillRate * float64(time.Second))
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Continue to next iteration.
		}
	}
}

// refill refills tokens based on elapsed time (must hold lock).
func (tb *TokenBucket) refill(now time.Time) {
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = math.Min(tb.maxTokens, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now
}

// SetBackoff sets a backoff time during which no tokens can be taken.
func (tb *TokenBucket) SetBackoff(until time.Time) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.backoffUntil = until
}

// ClearBackoff clears any active backoff.
func (tb *TokenBucket) ClearBackoff() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.backoffUntil = time.Time{}
}

// Available returns the current number of available tokens.
func (tb *TokenBucket) Available() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill(time.Now())
	return tb.tokens
}

// InBackoff returns true if the bucket is currently in backoff.
func (tb *TokenBucket) InBackoff() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return !tb.backoffUntil.IsZero() && time.Now().Before(tb.backoffUntil)
}

// GlobalRateLimiter implements a global rate limiter with backpressure signaling.
type GlobalRateLimiter struct {
	mu sync.Mutex

	// Token bucket for rate limiting.
	bucket *TokenBucket

	// Backpressure state.
	backpressure     bool
	backpressureTime time.Time

	// Metrics.
	totalRequests    int64
	totalRejected    int64
	backpressureHits int64
}

// NewGlobalRateLimiter creates a new global rate limiter.
func NewGlobalRateLimiter(requestsPerSecond, burstSize float64) *GlobalRateLimiter {
	return &GlobalRateLimiter{
		bucket: NewTokenBucket(burstSize, requestsPerSecond),
	}
}

// Allow checks if a request is allowed under the rate limit.
func (g *GlobalRateLimiter) Allow() bool {
	atomic.AddInt64(&g.totalRequests, 1)

	g.mu.Lock()
	// Check backpressure.
	if g.backpressure {
		if time.Now().After(g.backpressureTime) {
			g.backpressure = false
		} else {
			atomic.AddInt64(&g.backpressureHits, 1)
			g.mu.Unlock()
			return false
		}
	}
	g.mu.Unlock()

	if g.bucket.Take() {
		return true
	}

	atomic.AddInt64(&g.totalRejected, 1)
	return false
}

// AllowN checks if n requests are allowed under the rate limit.
func (g *GlobalRateLimiter) AllowN(n int) bool {
	atomic.AddInt64(&g.totalRequests, int64(n))

	g.mu.Lock()
	if g.backpressure {
		if time.Now().After(g.backpressureTime) {
			g.backpressure = false
		} else {
			atomic.AddInt64(&g.backpressureHits, 1)
			g.mu.Unlock()
			return false
		}
	}
	g.mu.Unlock()

	if g.bucket.TakeN(n) {
		return true
	}

	atomic.AddInt64(&g.totalRejected, int64(n))
	return false
}

// Wait blocks until the rate limit allows a request.
func (g *GlobalRateLimiter) Wait(ctx context.Context) error {
	atomic.AddInt64(&g.totalRequests, 1)

	// Check backpressure first.
	g.mu.Lock()
	if g.backpressure && time.Now().Before(g.backpressureTime) {
		waitDuration := time.Until(g.backpressureTime)
		g.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Continue.
		}
	} else {
		g.mu.Unlock()
	}

	return g.bucket.Wait(ctx)
}

// SetBackpressure enables backpressure for the given duration.
func (g *GlobalRateLimiter) SetBackpressure(duration time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.backpressure = true
	g.backpressureTime = time.Now().Add(duration)
}

// ClearBackpressure clears any active backpressure.
func (g *GlobalRateLimiter) ClearBackpressure() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.backpressure = false
}

// IsBackedOff returns true if the limiter is in a backpressure state.
func (g *GlobalRateLimiter) IsBackedOff() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.backpressure && time.Now().Before(g.backpressureTime)
}

// Stats returns rate limiter statistics.
func (g *GlobalRateLimiter) Stats() RateLimiterStats {
	return RateLimiterStats{
		TotalRequests:    atomic.LoadInt64(&g.totalRequests),
		TotalRejected:    atomic.LoadInt64(&g.totalRejected),
		BackpressureHits: atomic.LoadInt64(&g.backpressureHits),
		Available:        g.bucket.Available(),
		InBackpressure:   g.IsBackedOff(),
	}
}

// RateLimiterStats holds statistics for a rate limiter.
type RateLimiterStats struct {
	TotalRequests    int64   `json:"total_requests"`
	TotalRejected    int64   `json:"total_rejected"`
	BackpressureHits int64   `json:"backpressure_hits"`
	Available        float64 `json:"available"`
	InBackpressure   bool    `json:"in_backpressure"`
}

// BurstHandler manages burst traffic handling.
type BurstHandler struct {
	mu sync.Mutex

	// Normal rate limit.
	normalRate float64

	// Burst rate (temporary higher limit).
	burstRate float64

	// Burst capacity.
	burstCapacity int

	// Current burst count.
	burstCount int

	// Burst window.
	burstWindow time.Duration

	// Last burst time.
	lastBurst time.Time
}

// NewBurstHandler creates a new burst handler.
func NewBurstHandler(normalRate, burstRate float64, burstCapacity int, burstWindow time.Duration) *BurstHandler {
	return &BurstHandler{
		normalRate:    normalRate,
		burstRate:     burstRate,
		burstCapacity: burstCapacity,
		burstWindow:   burstWindow,
	}
}

// AllowBurst checks if a burst request is allowed.
func (b *BurstHandler) AllowBurst() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Reset burst count if window expired.
	if now.Sub(b.lastBurst) > b.burstWindow {
		b.burstCount = 0
	}

	if b.burstCount < b.burstCapacity {
		b.burstCount++
		b.lastBurst = now
		return true
	}

	return false
}

// GetEffectiveRate returns the effective rate (burst or normal).
func (b *BurstHandler) GetEffectiveRate() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.burstCount > 0 && time.Since(b.lastBurst) < b.burstWindow {
		return b.burstRate
	}
	return b.normalRate
}

// TenantThrottler manages per-tenant rate limiting.
type TenantThrottler struct {
	mu sync.RWMutex

	// Per-tenant token buckets.
	buckets map[string]*TokenBucket

	// Default bucket configuration.
	defaultMaxTokens  float64
	defaultRefillRate float64

	// Tenant-specific overrides.
	overrides map[string]ThrottleConfig
}

// ThrottleConfig holds throttle configuration for a tenant.
type ThrottleConfig struct {
	MaxTokens  float64 `json:"max_tokens"`
	RefillRate float64 `json:"refill_rate"`
}

// NewTenantThrottler creates a new tenant throttler.
func NewTenantThrottler(defaultMaxTokens, defaultRefillRate float64) *TenantThrottler {
	return &TenantThrottler{
		buckets:           make(map[string]*TokenBucket),
		defaultMaxTokens:  defaultMaxTokens,
		defaultRefillRate: defaultRefillRate,
		overrides:         make(map[string]ThrottleConfig),
	}
}

// Allow checks if a request is allowed for a tenant.
func (t *TenantThrottler) Allow(tenantID string) bool {
	bucket := t.getBucket(tenantID)
	return bucket.Take()
}

// Wait blocks until a request is allowed for a tenant.
func (t *TenantThrottler) Wait(ctx context.Context, tenantID string) error {
	bucket := t.getBucket(tenantID)
	return bucket.Wait(ctx)
}

// SetOverride sets a custom throttle configuration for a tenant.
func (t *TenantThrottler) SetOverride(tenantID string, config ThrottleConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.overrides[tenantID] = config

	// Update existing bucket if present.
	if bucket, ok := t.buckets[tenantID]; ok {
		bucket.mu.Lock()
		bucket.maxTokens = config.MaxTokens
		bucket.refillRate = config.RefillRate
		bucket.mu.Unlock()
	}
}

// RemoveOverride removes a tenant-specific override.
func (t *TenantThrottler) RemoveOverride(tenantID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.overrides, tenantID)
	delete(t.buckets, tenantID) // Force recreation with defaults.
}

// getBucket returns or creates a bucket for a tenant.
func (t *TenantThrottler) getBucket(tenantID string) *TokenBucket {
	t.mu.RLock()
	bucket, ok := t.buckets[tenantID]
	t.mu.RUnlock()

	if ok {
		return bucket
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Double-check after acquiring write lock.
	if bucket, ok := t.buckets[tenantID]; ok {
		return bucket
	}

	// Create new bucket with override or defaults.
	maxTokens := t.defaultMaxTokens
	refillRate := t.defaultRefillRate
	if override, ok := t.overrides[tenantID]; ok {
		maxTokens = override.MaxTokens
		refillRate = override.RefillRate
	}

	bucket = NewTokenBucket(maxTokens, refillRate)
	t.buckets[tenantID] = bucket
	return bucket
}

// SetBackoff sets backoff for a tenant.
func (t *TenantThrottler) SetBackoff(tenantID string, until time.Time) {
	bucket := t.getBucket(tenantID)
	bucket.SetBackoff(until)
}

// ClearBackoff clears backoff for a tenant.
func (t *TenantThrottler) ClearBackoff(tenantID string) {
	t.mu.RLock()
	bucket, ok := t.buckets[tenantID]
	t.mu.RUnlock()

	if ok {
		bucket.ClearBackoff()
	}
}

// BackpressureSignaler handles backpressure signaling.
type BackpressureSignaler struct {
	mu sync.RWMutex

	// Threshold at which backpressure is triggered (0-1).
	threshold float64

	// Current load (0-1).
	currentLoad float64

	// Backpressure state.
	inBackpressure bool

	// Listeners to notify on state change.
	listeners []chan<- bool
}

// NewBackpressureSignaler creates a new backpressure signaler.
func NewBackpressureSignaler(threshold float64) *BackpressureSignaler {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.8 // 80% default threshold
	}
	return &BackpressureSignaler{
		threshold: threshold,
	}
}

// UpdateLoad updates the current load and triggers backpressure if needed.
func (b *BackpressureSignaler) UpdateLoad(load float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.currentLoad = load
	newState := load >= b.threshold

	if newState != b.inBackpressure {
		b.inBackpressure = newState
		// Notify listeners.
		for _, ch := range b.listeners {
			select {
			case ch <- newState:
			default:
				// Don't block if listener is slow.
			}
		}
	}
}

// IsBackpressured returns true if backpressure is active.
func (b *BackpressureSignaler) IsBackpressured() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.inBackpressure
}

// Subscribe returns a channel that receives backpressure state changes.
func (b *BackpressureSignaler) Subscribe() <-chan bool {
	ch := make(chan bool, 1)
	b.mu.Lock()
	b.listeners = append(b.listeners, ch)
	b.mu.Unlock()
	return ch
}

// CurrentLoad returns the current load.
func (b *BackpressureSignaler) CurrentLoad() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentLoad
}
