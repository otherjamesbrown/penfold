// Package scheduler provides intelligent task scheduling for Gmail sync operations.
package scheduler

import (
	"math"
	"math/rand"
	"time"
)

// TimingCalculator provides timing calculations for sync operations.
type TimingCalculator interface {
	// OptimalSyncTime calculates the optimal time to sync an account.
	OptimalSyncTime(account *Account) time.Time

	// BackoffDuration calculates backoff duration after a rate limit.
	BackoffDuration(attempt int, baseBackoff time.Duration) time.Duration

	// IsQuietHours checks if the current time is within quiet hours.
	IsQuietHours(account *Account, t time.Time) bool

	// BatchTimeWindow returns the window for batching operations.
	BatchTimeWindow() time.Duration
}

// DefaultTimingConfig holds configuration for timing calculations.
type DefaultTimingConfig struct {
	// MinSyncInterval is the minimum time between syncs for an account.
	MinSyncInterval time.Duration

	// MaxSyncInterval is the maximum time between syncs.
	MaxSyncInterval time.Duration

	// BaseBackoff is the base backoff duration for rate limits.
	BaseBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64

	// BackoffJitter is the jitter factor for randomizing backoff (0-1).
	BackoffJitter float64

	// BatchWindow is the time window for batching operations.
	BatchWindow time.Duration

	// QuietHoursEnabled enables quiet hours support.
	QuietHoursEnabled bool

	// DefaultQuietStart is the default quiet hours start (hour, 0-23).
	DefaultQuietStart int

	// DefaultQuietEnd is the default quiet hours end (hour, 0-23).
	DefaultQuietEnd int
}

// DefaultTimingConfigValues returns timing config with sensible defaults.
func DefaultTimingConfigValues() *DefaultTimingConfig {
	return &DefaultTimingConfig{
		MinSyncInterval:   5 * time.Minute,
		MaxSyncInterval:   4 * time.Hour,
		BaseBackoff:       30 * time.Second,
		MaxBackoff:        5 * time.Minute,
		BackoffMultiplier: 2.0,
		BackoffJitter:     0.2,
		BatchWindow:       30 * time.Second,
		QuietHoursEnabled: true,
		DefaultQuietStart: 23, // 11 PM
		DefaultQuietEnd:   6,  // 6 AM
	}
}

// DefaultTimingCalculatorImpl implements TimingCalculator with sensible defaults.
type DefaultTimingCalculatorImpl struct {
	config *DefaultTimingConfig
}

// NewDefaultTimingCalculator creates a new DefaultTimingCalculatorImpl.
func NewDefaultTimingCalculator(config *DefaultTimingConfig) *DefaultTimingCalculatorImpl {
	if config == nil {
		config = DefaultTimingConfigValues()
	}
	return &DefaultTimingCalculatorImpl{config: config}
}

// OptimalSyncTime calculates the optimal time to sync an account.
func (t *DefaultTimingCalculatorImpl) OptimalSyncTime(account *Account) time.Time {
	now := time.Now()

	// If in backoff, return backoff time.
	if !account.RateLimitBackoff.IsZero() && account.RateLimitBackoff.After(now) {
		return account.RateLimitBackoff
	}

	// Calculate ideal sync time based on activity level.
	var idealInterval time.Duration
	if account.SyncFrequency > 0 {
		idealInterval = account.SyncFrequency
	} else {
		// Calculate based on activity level.
		// High activity (100) = min interval, low activity (0) = max interval.
		activityRatio := float64(account.ActivityLevel) / 100.0
		intervalRange := float64(t.config.MaxSyncInterval - t.config.MinSyncInterval)
		idealInterval = t.config.MaxSyncInterval - time.Duration(activityRatio*intervalRange)
	}

	// VIP accounts get more frequent syncs.
	if account.IsVIP {
		idealInterval = idealInterval / 2
		if idealInterval < t.config.MinSyncInterval {
			idealInterval = t.config.MinSyncInterval
		}
	}

	// Calculate next sync time.
	var nextSync time.Time
	if account.LastSyncAt.IsZero() {
		// Never synced, sync now.
		nextSync = now
	} else {
		nextSync = account.LastSyncAt.Add(idealInterval)
		if nextSync.Before(now) {
			// Overdue, sync now.
			nextSync = now
		}
	}

	// Adjust for quiet hours if enabled.
	if t.config.QuietHoursEnabled && t.IsQuietHours(account, nextSync) {
		nextSync = t.NextNonQuietTime(account, nextSync)
	}

	// Add small jitter to prevent thundering herd.
	jitterDuration := time.Duration(rand.Float64() * float64(t.config.BatchWindow))
	nextSync = nextSync.Add(jitterDuration)

	return nextSync
}

// BackoffDuration calculates backoff duration with exponential backoff and jitter.
func (t *DefaultTimingCalculatorImpl) BackoffDuration(attempt int, baseBackoff time.Duration) time.Duration {
	if baseBackoff <= 0 {
		baseBackoff = t.config.BaseBackoff
	}

	// Calculate exponential backoff.
	backoff := float64(baseBackoff) * math.Pow(t.config.BackoffMultiplier, float64(attempt))

	// Cap at max backoff.
	if backoff > float64(t.config.MaxBackoff) {
		backoff = float64(t.config.MaxBackoff)
	}

	// Add jitter.
	jitter := backoff * t.config.BackoffJitter * (rand.Float64()*2 - 1) // -jitter to +jitter
	backoff += jitter

	return time.Duration(backoff)
}

// IsQuietHours checks if the given time falls within quiet hours for the account.
func (t *DefaultTimingCalculatorImpl) IsQuietHours(account *Account, checkTime time.Time) bool {
	if !t.config.QuietHoursEnabled {
		return false
	}

	// Use account-specific quiet hours or defaults.
	quietStart := account.QuietHoursStart
	quietEnd := account.QuietHoursEnd
	if quietStart == 0 && quietEnd == 0 {
		quietStart = t.config.DefaultQuietStart
		quietEnd = t.config.DefaultQuietEnd
	}

	hour := checkTime.Hour()

	// Handle wrap-around (e.g., 23:00 to 06:00).
	if quietStart > quietEnd {
		// Quiet hours span midnight.
		return hour >= quietStart || hour < quietEnd
	}

	// Normal range (e.g., 22:00 to 05:00 would not work this way, use above).
	return hour >= quietStart && hour < quietEnd
}

// NextNonQuietTime returns the next time outside of quiet hours.
func (t *DefaultTimingCalculatorImpl) NextNonQuietTime(account *Account, from time.Time) time.Time {
	if !t.IsQuietHours(account, from) {
		return from
	}

	// Get quiet end time.
	quietEnd := account.QuietHoursEnd
	if quietEnd == 0 {
		quietEnd = t.config.DefaultQuietEnd
	}

	// Calculate the next occurrence of quiet end.
	result := time.Date(from.Year(), from.Month(), from.Day(), quietEnd, 0, 0, 0, from.Location())

	// If the quiet end is earlier in the day, move to tomorrow.
	if result.Before(from) {
		result = result.Add(24 * time.Hour)
	}

	return result
}

// BatchTimeWindow returns the window for batching operations.
func (t *DefaultTimingCalculatorImpl) BatchTimeWindow() time.Duration {
	return t.config.BatchWindow
}

// BackoffCalculator provides exponential backoff calculations.
type BackoffCalculator struct {
	// BaseBackoff is the initial backoff duration.
	BaseBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration

	// Multiplier is the backoff multiplier.
	Multiplier float64

	// Jitter is the jitter factor (0-1).
	Jitter float64
}

// NewBackoffCalculator creates a new BackoffCalculator with defaults.
func NewBackoffCalculator() *BackoffCalculator {
	return &BackoffCalculator{
		BaseBackoff: 1 * time.Second,
		MaxBackoff:  5 * time.Minute,
		Multiplier:  2.0,
		Jitter:      0.1,
	}
}

// Calculate returns the backoff duration for a given attempt.
func (b *BackoffCalculator) Calculate(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	// Calculate exponential backoff.
	backoff := float64(b.BaseBackoff) * math.Pow(b.Multiplier, float64(attempt-1))

	// Cap at max.
	if backoff > float64(b.MaxBackoff) {
		backoff = float64(b.MaxBackoff)
	}

	// Apply jitter.
	if b.Jitter > 0 {
		jitterRange := backoff * b.Jitter
		jitter := (rand.Float64()*2 - 1) * jitterRange
		backoff += jitter
	}

	return time.Duration(backoff)
}

// CalculateWithRetryAfter calculates backoff respecting a Retry-After header value.
func (b *BackoffCalculator) CalculateWithRetryAfter(attempt int, retryAfter time.Duration) time.Duration {
	calculated := b.Calculate(attempt)
	if retryAfter > calculated {
		return retryAfter
	}
	return calculated
}

// SyncSchedule represents a scheduled sync with timing information.
type SyncSchedule struct {
	// AccountID is the account to sync.
	AccountID string

	// NextSync is the scheduled sync time.
	NextSync time.Time

	// Interval is the calculated sync interval.
	Interval time.Duration

	// IsUrgent indicates if this is an urgent sync.
	IsUrgent bool

	// Reason describes why this sync was scheduled.
	Reason string
}

// BatchScheduler groups sync operations for efficiency.
type BatchScheduler struct {
	// Window is the time window for batching.
	Window time.Duration

	// MaxBatchSize is the maximum batch size.
	MaxBatchSize int

	// batches holds pending batches.
	batches map[string][]*SyncSchedule
}

// NewBatchScheduler creates a new BatchScheduler.
func NewBatchScheduler(window time.Duration, maxBatchSize int) *BatchScheduler {
	if window <= 0 {
		window = 30 * time.Second
	}
	if maxBatchSize <= 0 {
		maxBatchSize = 10
	}
	return &BatchScheduler{
		Window:       window,
		MaxBatchSize: maxBatchSize,
		batches:      make(map[string][]*SyncSchedule),
	}
}

// Add adds a schedule to a batch.
func (b *BatchScheduler) Add(tenantID string, schedule *SyncSchedule) {
	b.batches[tenantID] = append(b.batches[tenantID], schedule)
}

// GetBatch returns schedules for a tenant that fall within the batch window.
func (b *BatchScheduler) GetBatch(tenantID string, now time.Time) []*SyncSchedule {
	schedules := b.batches[tenantID]
	if len(schedules) == 0 {
		return nil
	}

	var batch []*SyncSchedule
	var remaining []*SyncSchedule

	windowEnd := now.Add(b.Window)

	for _, s := range schedules {
		if s.NextSync.Before(windowEnd) && len(batch) < b.MaxBatchSize {
			batch = append(batch, s)
		} else {
			remaining = append(remaining, s)
		}
	}

	b.batches[tenantID] = remaining
	return batch
}

// Clear removes all batches for a tenant.
func (b *BatchScheduler) Clear(tenantID string) {
	delete(b.batches, tenantID)
}

// AdaptiveSyncInterval calculates adaptive sync interval based on historical data.
type AdaptiveSyncInterval struct {
	// MinInterval is the minimum interval.
	MinInterval time.Duration

	// MaxInterval is the maximum interval.
	MaxInterval time.Duration

	// RecentSyncTimes holds recent sync completion times.
	RecentSyncTimes []time.Duration

	// MaxSamples is the maximum samples to track.
	MaxSamples int
}

// NewAdaptiveSyncInterval creates a new AdaptiveSyncInterval.
func NewAdaptiveSyncInterval(min, max time.Duration) *AdaptiveSyncInterval {
	return &AdaptiveSyncInterval{
		MinInterval:     min,
		MaxInterval:     max,
		RecentSyncTimes: make([]time.Duration, 0),
		MaxSamples:      10,
	}
}

// RecordSyncTime records a sync duration.
func (a *AdaptiveSyncInterval) RecordSyncTime(duration time.Duration) {
	a.RecentSyncTimes = append(a.RecentSyncTimes, duration)
	if len(a.RecentSyncTimes) > a.MaxSamples {
		a.RecentSyncTimes = a.RecentSyncTimes[1:]
	}
}

// Calculate returns the adaptive sync interval.
func (a *AdaptiveSyncInterval) Calculate(messagesCount int64, activityLevel int) time.Duration {
	// Base calculation on activity level.
	activityRatio := float64(activityLevel) / 100.0
	intervalRange := float64(a.MaxInterval - a.MinInterval)
	baseInterval := a.MaxInterval - time.Duration(activityRatio*intervalRange)

	// Adjust based on recent sync times.
	if len(a.RecentSyncTimes) > 0 {
		var avgDuration time.Duration
		for _, d := range a.RecentSyncTimes {
			avgDuration += d
		}
		avgDuration /= time.Duration(len(a.RecentSyncTimes))

		// If syncs are taking longer, increase interval slightly.
		if avgDuration > 30*time.Second {
			baseInterval += avgDuration
		}
	}

	// Adjust based on message count.
	if messagesCount > 1000 {
		// High volume, sync more frequently.
		baseInterval = time.Duration(float64(baseInterval) * 0.8)
	}

	// Clamp to bounds.
	if baseInterval < a.MinInterval {
		baseInterval = a.MinInterval
	}
	if baseInterval > a.MaxInterval {
		baseInterval = a.MaxInterval
	}

	return baseInterval
}

// Ensure interface compliance.
var _ TimingCalculator = (*DefaultTimingCalculatorImpl)(nil)

// DefaultTimingCalculator is the package-level default timing calculator.
var DefaultTimingCalculator = NewDefaultTimingCalculator(nil)
