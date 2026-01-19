// Package scheduler provides intelligent task scheduling for Gmail sync operations.
package scheduler

import (
	"container/heap"
	"math"
	"sync"
	"time"
)

// Priority represents the calculated priority for a sync task.
type Priority struct {
	// Score is the numeric priority score (higher = more urgent, 0-100).
	Score int `json:"score"`

	// Factors contains the breakdown of priority factors.
	Factors PriorityFactors `json:"factors"`

	// CalculatedAt is when the priority was calculated.
	CalculatedAt time.Time `json:"calculated_at"`
}

// PriorityFactors contains the individual factors contributing to priority.
type PriorityFactors struct {
	// LastSync factor based on time since last sync (0-25).
	LastSync int `json:"last_sync"`

	// ActivityLevel factor based on account activity (0-25).
	ActivityLevel int `json:"activity_level"`

	// VIPStatus factor for VIP accounts (0-25).
	VIPStatus int `json:"vip_status"`

	// PendingNotifications factor based on pending push notifications (0-25).
	PendingNotifications int `json:"pending_notifications"`

	// DynamicAdjustment based on queue depth and system load (-10 to +10).
	DynamicAdjustment int `json:"dynamic_adjustment"`

	// RateLimitPenalty penalty for rate-limited accounts (-20 to 0).
	RateLimitPenalty int `json:"rate_limit_penalty"`
}

// PriorityContext provides context for priority calculation.
type PriorityContext struct {
	// QueueDepth is the current number of tasks in the queue.
	QueueDepth int

	// CurrentTime is the current time for calculations.
	CurrentTime time.Time

	// GlobalBackoff indicates if the system is in a global backoff state.
	GlobalBackoff bool

	// TenantPriority is an optional tenant-level priority multiplier (0-2).
	TenantPriority float64
}

// PriorityCalculator is a function that calculates priority for an account.
type PriorityCalculator func(account *Account, ctx PriorityContext) Priority

// DefaultPriorityCalculator is the default priority calculation function.
func DefaultPriorityCalculator(account *Account, ctx PriorityContext) Priority {
	factors := PriorityFactors{}
	currentTime := ctx.CurrentTime
	if currentTime.IsZero() {
		currentTime = time.Now()
	}

	// Factor 1: Time since last sync (0-25 points)
	// More points for accounts that haven't synced recently.
	if !account.LastSyncAt.IsZero() {
		timeSinceSync := currentTime.Sub(account.LastSyncAt)
		syncFrequency := account.SyncFrequency
		if syncFrequency == 0 {
			syncFrequency = 15 * time.Minute // Default sync frequency
		}

		// Calculate how overdue the sync is.
		overdueRatio := float64(timeSinceSync) / float64(syncFrequency)
		if overdueRatio > 4 {
			overdueRatio = 4 // Cap at 4x overdue
		}
		factors.LastSync = int(math.Min(25, overdueRatio*6.25))
	} else {
		// Never synced, high priority.
		factors.LastSync = 25
	}

	// Factor 2: Activity level (0-25 points)
	// More active accounts get higher priority.
	factors.ActivityLevel = int(math.Min(25, float64(account.ActivityLevel)*0.25))

	// Factor 3: VIP status (0-25 points)
	// VIP accounts get a significant boost.
	if account.IsVIP {
		factors.VIPStatus = 25
	}

	// Factor 4: Pending notifications (0-25 points)
	// More pending notifications = higher priority.
	pendingScore := float64(account.PendingNotifications) * 5
	factors.PendingNotifications = int(math.Min(25, pendingScore))

	// Factor 5: Dynamic adjustment based on queue depth (-10 to +10)
	// If queue is deep, boost priority of urgent tasks.
	if ctx.QueueDepth > 1000 {
		// In high load, prioritize VIP and high-activity accounts more.
		if account.IsVIP || account.ActivityLevel > 80 {
			factors.DynamicAdjustment = 10
		}
	} else if ctx.QueueDepth < 100 {
		// Low load, slight boost to all tasks.
		factors.DynamicAdjustment = 2
	}

	// Factor 6: Rate limit penalty (-20 to 0)
	// Accounts in backoff get deprioritized.
	if !account.RateLimitBackoff.IsZero() && account.RateLimitBackoff.After(currentTime) {
		// Calculate penalty based on how long until backoff ends.
		backoffRemaining := account.RateLimitBackoff.Sub(currentTime)
		penaltyRatio := float64(backoffRemaining) / float64(time.Minute*5)
		factors.RateLimitPenalty = -int(math.Min(20, penaltyRatio*10))
	}

	// Global backoff penalty.
	if ctx.GlobalBackoff {
		factors.RateLimitPenalty -= 5
		if factors.RateLimitPenalty < -20 {
			factors.RateLimitPenalty = -20
		}
	}

	// Calculate total score.
	score := factors.LastSync + factors.ActivityLevel + factors.VIPStatus +
		factors.PendingNotifications + factors.DynamicAdjustment + factors.RateLimitPenalty

	// Apply tenant priority multiplier if provided.
	if ctx.TenantPriority > 0 && ctx.TenantPriority != 1.0 {
		score = int(float64(score) * ctx.TenantPriority)
	}

	// Clamp to 0-100 range.
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return Priority{
		Score:        score,
		Factors:      factors,
		CalculatedAt: currentTime,
	}
}

// VIPPriorityCalculator prioritizes VIP accounts more heavily.
func VIPPriorityCalculator(account *Account, ctx PriorityContext) Priority {
	priority := DefaultPriorityCalculator(account, ctx)

	// Apply additional VIP boost.
	if account.IsVIP {
		priority.Score += 15
		if priority.Score > 100 {
			priority.Score = 100
		}
	}

	return priority
}

// ActivityBasedCalculator prioritizes high-activity accounts.
func ActivityBasedCalculator(account *Account, ctx PriorityContext) Priority {
	priority := DefaultPriorityCalculator(account, ctx)

	// Extra weight on activity level.
	activityBoost := account.ActivityLevel / 10 // 0-10 points
	priority.Score += activityBoost
	if priority.Score > 100 {
		priority.Score = 100
	}

	return priority
}

// FairSchedulingCalculator ensures fair scheduling across tenants.
func FairSchedulingCalculator(account *Account, ctx PriorityContext) Priority {
	// Base priority calculation.
	priority := DefaultPriorityCalculator(account, ctx)

	// Cap maximum priority to ensure fairness.
	if priority.Score > 80 {
		priority.Score = 80
	}

	// Boost if account hasn't been synced in a while to ensure fairness.
	if !account.LastSyncAt.IsZero() {
		timeSinceSync := ctx.CurrentTime.Sub(account.LastSyncAt)
		if timeSinceSync > 30*time.Minute {
			priority.Score += 10
		}
		if timeSinceSync > time.Hour {
			priority.Score += 10
		}
	}

	if priority.Score > 100 {
		priority.Score = 100
	}

	return priority
}

// PriorityQueue implements a priority queue for sync tasks.
type PriorityQueue struct {
	mu    sync.Mutex
	items []*SyncTask
	index map[string]int // task ID -> index in items
}

// NewPriorityQueue creates a new priority queue.
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		items: make([]*SyncTask, 0),
		index: make(map[string]int),
	}
	heap.Init(pq)
	return pq
}

// Len returns the number of items in the queue.
func (pq *PriorityQueue) Len() int {
	return len(pq.items)
}

// Less compares two items by priority (higher priority first).
func (pq *PriorityQueue) Less(i, j int) bool {
	// Higher score = higher priority = should come first.
	if pq.items[i].Priority.Score != pq.items[j].Priority.Score {
		return pq.items[i].Priority.Score > pq.items[j].Priority.Score
	}
	// Tie-breaker: earlier scheduled time.
	return pq.items[i].ScheduledAt.Before(pq.items[j].ScheduledAt)
}

// Swap swaps two items in the queue.
func (pq *PriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.index[pq.items[i].ID] = i
	pq.index[pq.items[j].ID] = j
}

// Push adds an item to the queue (used by heap.Push).
func (pq *PriorityQueue) Push(x interface{}) {
	task := x.(*SyncTask)
	pq.index[task.ID] = len(pq.items)
	pq.items = append(pq.items, task)
}

// Pop removes and returns the highest priority item (used by heap.Pop).
func (pq *PriorityQueue) Pop() interface{} {
	if len(pq.items) == 0 {
		return nil
	}
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items = pq.items[:n-1]
	delete(pq.index, item.ID)
	return item
}

// PushTask adds a task to the queue (thread-safe).
func (pq *PriorityQueue) PushTask(task *SyncTask) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	heap.Push(pq, task)
}

// PopTask removes and returns the highest priority task (thread-safe).
func (pq *PriorityQueue) PopTask() *SyncTask {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	item := heap.Pop(pq)
	if item == nil {
		return nil
	}
	return item.(*SyncTask)
}

// Update updates the priority of a task in the queue.
func (pq *PriorityQueue) Update(task *SyncTask) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	idx, ok := pq.index[task.ID]
	if !ok {
		return
	}

	heap.Fix(pq, idx)
}

// Remove removes a task from the queue by ID.
func (pq *PriorityQueue) Remove(taskID string) *SyncTask {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	idx, ok := pq.index[taskID]
	if !ok {
		return nil
	}

	item := heap.Remove(pq, idx)
	if item == nil {
		return nil
	}
	return item.(*SyncTask)
}

// Peek returns the highest priority task without removing it.
func (pq *PriorityQueue) Peek() *SyncTask {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	return pq.items[0]
}

// Contains checks if a task is in the queue.
func (pq *PriorityQueue) Contains(taskID string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	_, ok := pq.index[taskID]
	return ok
}

// Clear removes all tasks from the queue.
func (pq *PriorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.items = make([]*SyncTask, 0)
	pq.index = make(map[string]int)
}

// CalculatePriority is a convenience function to calculate priority using the default calculator.
func CalculatePriority(account *Account, ctx PriorityContext) Priority {
	return DefaultPriorityCalculator(account, ctx)
}

// CompareAccounts compares two accounts by priority.
// Returns negative if a1 < a2, zero if equal, positive if a1 > a2.
func CompareAccounts(a1, a2 *Account, ctx PriorityContext) int {
	p1 := DefaultPriorityCalculator(a1, ctx)
	p2 := DefaultPriorityCalculator(a2, ctx)

	return p1.Score - p2.Score
}
