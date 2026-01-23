// Package conflict provides conflict detection and resolution for discovered relationships.
package conflict

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// InMemoryReviewQueue provides an in-memory implementation of ReviewQueueIntegration
// for testing and development purposes. Production use should integrate with the
// review queue service.
type InMemoryReviewQueue struct {
	mu       sync.RWMutex
	items    map[string]*QueuedConflict
	logger   logging.Logger
	onChange func(conflict *Conflict, action string)
}

// QueuedConflict wraps a conflict with queue-specific metadata.
type QueuedConflict struct {
	Conflict   *Conflict
	QueuedAt   time.Time
	Priority   string
	AssignedTo string
	Notes      string
	Status     QueueStatus
}

// QueueStatus represents the status of a queued conflict.
type QueueStatus string

const (
	// QueueStatusPending indicates the conflict is waiting for review.
	QueueStatusPending QueueStatus = "pending"

	// QueueStatusInProgress indicates the conflict is being reviewed.
	QueueStatusInProgress QueueStatus = "in_progress"

	// QueueStatusResolved indicates the conflict has been resolved.
	QueueStatusResolved QueueStatus = "resolved"

	// QueueStatusDeferred indicates the conflict was deferred for later.
	QueueStatusDeferred QueueStatus = "deferred"
)

// NewInMemoryReviewQueue creates a new InMemoryReviewQueue.
func NewInMemoryReviewQueue(logger logging.Logger) *InMemoryReviewQueue {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &InMemoryReviewQueue{
		items:  make(map[string]*QueuedConflict),
		logger: logger.With(logging.F("component", "review_queue")),
	}
}

// SetOnChangeCallback sets a callback for queue changes.
func (q *InMemoryReviewQueue) SetOnChangeCallback(callback func(conflict *Conflict, action string)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onChange = callback
}

// QueueForReview adds a conflict to the review queue.
func (q *InMemoryReviewQueue) QueueForReview(ctx context.Context, conflict *Conflict) error {
	if conflict == nil {
		return fmt.Errorf("conflict is nil")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if already queued
	if _, exists := q.items[conflict.ID]; exists {
		q.logger.Debug("Conflict already queued",
			logging.F("conflict_id", conflict.ID),
		)
		return nil
	}

	// Determine priority based on severity
	priority := "medium"
	switch conflict.Severity {
	case SeverityCritical:
		priority = "urgent"
	case SeverityHigh:
		priority = "high"
	case SeverityLow:
		priority = "low"
	}

	q.items[conflict.ID] = &QueuedConflict{
		Conflict: conflict,
		QueuedAt: time.Now(),
		Priority: priority,
		Status:   QueueStatusPending,
	}

	q.logger.Info("Conflict queued for review",
		logging.F("conflict_id", conflict.ID),
		logging.F("priority", priority),
		logging.F("type", conflict.Type),
	)

	if q.onChange != nil {
		q.onChange(conflict, "queued")
	}

	return nil
}

// GetPendingConflicts retrieves pending conflicts for a tenant.
func (q *InMemoryReviewQueue) GetPendingConflicts(ctx context.Context, tenantID string, limit int) ([]*Conflict, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	pending := make([]*QueuedConflict, 0)

	for _, item := range q.items {
		if item.Status != QueueStatusPending {
			continue
		}
		if tenantID != "" && item.Conflict.TenantID != tenantID {
			continue
		}
		pending = append(pending, item)
	}

	// Sort by priority and queue time
	sort.Slice(pending, func(i, j int) bool {
		// Priority order: urgent > high > medium > low
		priorityOrder := map[string]int{
			"urgent": 4,
			"high":   3,
			"medium": 2,
			"low":    1,
		}
		pi := priorityOrder[pending[i].Priority]
		pj := priorityOrder[pending[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return pending[i].QueuedAt.Before(pending[j].QueuedAt)
	})

	// Apply limit
	if limit > 0 && limit < len(pending) {
		pending = pending[:limit]
	}

	// Extract conflicts
	conflicts := make([]*Conflict, len(pending))
	for i, item := range pending {
		conflicts[i] = item.Conflict
	}

	return conflicts, nil
}

// ApplyResolution applies a resolution to a queued conflict.
func (q *InMemoryReviewQueue) ApplyResolution(ctx context.Context, conflictID string, resolution *Resolution) error {
	if resolution == nil {
		return fmt.Errorf("resolution is nil")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	// Update conflict with resolution
	now := time.Now()
	item.Conflict.ResolvedAt = &now
	item.Conflict.Resolution = resolution
	item.Status = QueueStatusResolved

	q.logger.Info("Resolution applied to queued conflict",
		logging.F("conflict_id", conflictID),
		logging.F("action", resolution.Action),
	)

	if q.onChange != nil {
		q.onChange(item.Conflict, "resolved")
	}

	return nil
}

// RemoveFromQueue removes a conflict from the queue.
func (q *InMemoryReviewQueue) RemoveFromQueue(ctx context.Context, conflictID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return nil // Not an error if not found
	}

	delete(q.items, conflictID)

	q.logger.Info("Conflict removed from queue",
		logging.F("conflict_id", conflictID),
	)

	if q.onChange != nil {
		q.onChange(item.Conflict, "removed")
	}

	return nil
}

// AssignConflict assigns a conflict to a reviewer.
func (q *InMemoryReviewQueue) AssignConflict(ctx context.Context, conflictID, assignee string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	item.AssignedTo = assignee
	item.Status = QueueStatusInProgress

	q.logger.Info("Conflict assigned",
		logging.F("conflict_id", conflictID),
		logging.F("assignee", assignee),
	)

	if q.onChange != nil {
		q.onChange(item.Conflict, "assigned")
	}

	return nil
}

// DeferConflict defers a conflict for later review.
func (q *InMemoryReviewQueue) DeferConflict(ctx context.Context, conflictID string, notes string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	item.Status = QueueStatusDeferred
	item.Notes = notes

	q.logger.Info("Conflict deferred",
		logging.F("conflict_id", conflictID),
		logging.F("notes", notes),
	)

	if q.onChange != nil {
		q.onChange(item.Conflict, "deferred")
	}

	return nil
}

// ReactivateConflict moves a deferred conflict back to pending.
func (q *InMemoryReviewQueue) ReactivateConflict(ctx context.Context, conflictID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	if item.Status != QueueStatusDeferred {
		return fmt.Errorf("conflict is not deferred: %s", conflictID)
	}

	item.Status = QueueStatusPending
	item.QueuedAt = time.Now() // Reset queue time

	q.logger.Info("Conflict reactivated",
		logging.F("conflict_id", conflictID),
	)

	if q.onChange != nil {
		q.onChange(item.Conflict, "reactivated")
	}

	return nil
}

// GetQueueStats returns queue statistics.
func (q *InMemoryReviewQueue) GetQueueStats(ctx context.Context, tenantID string) (*QueueStats, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := &QueueStats{
		ByPriority: make(map[string]int64),
		ByStatus:   make(map[QueueStatus]int64),
	}

	var totalWaitTime time.Duration
	now := time.Now()

	for _, item := range q.items {
		if tenantID != "" && item.Conflict.TenantID != tenantID {
			continue
		}

		stats.Total++
		stats.ByPriority[item.Priority]++
		stats.ByStatus[item.Status]++

		if item.Status == QueueStatusPending {
			stats.Pending++
			waitTime := now.Sub(item.QueuedAt)
			totalWaitTime += waitTime

			if item.QueuedAt.Before(stats.OldestQueueTime) || stats.OldestQueueTime.IsZero() {
				stats.OldestQueueTime = item.QueuedAt
			}
		}

		if item.Status == QueueStatusInProgress {
			stats.InProgress++
		}
	}

	if stats.Pending > 0 {
		stats.AverageWaitTime = totalWaitTime / time.Duration(stats.Pending)
	}

	return stats, nil
}

// QueueStats provides statistics about the review queue.
type QueueStats struct {
	Total            int64
	Pending          int64
	InProgress       int64
	ByPriority       map[string]int64
	ByStatus         map[QueueStatus]int64
	OldestQueueTime  time.Time
	AverageWaitTime  time.Duration
}

// GetConflictsByAssignee returns conflicts assigned to a specific reviewer.
func (q *InMemoryReviewQueue) GetConflictsByAssignee(ctx context.Context, assignee string) ([]*Conflict, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	conflicts := make([]*Conflict, 0)

	for _, item := range q.items {
		if item.AssignedTo == assignee {
			conflicts = append(conflicts, item.Conflict)
		}
	}

	return conflicts, nil
}

// UpdatePriority changes the priority of a queued conflict.
func (q *InMemoryReviewQueue) UpdatePriority(ctx context.Context, conflictID, priority string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	// Validate priority
	validPriorities := map[string]bool{
		"urgent": true,
		"high":   true,
		"medium": true,
		"low":    true,
	}
	if !validPriorities[priority] {
		return fmt.Errorf("invalid priority: %s", priority)
	}

	item.Priority = priority

	q.logger.Info("Conflict priority updated",
		logging.F("conflict_id", conflictID),
		logging.F("priority", priority),
	)

	return nil
}

// AddNotes adds notes to a queued conflict.
func (q *InMemoryReviewQueue) AddNotes(ctx context.Context, conflictID, notes string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, exists := q.items[conflictID]
	if !exists {
		return fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	if item.Notes != "" {
		item.Notes += "\n---\n"
	}
	item.Notes += fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), notes)

	return nil
}

// GetQueuedConflict returns the full queued conflict with metadata.
func (q *InMemoryReviewQueue) GetQueuedConflict(ctx context.Context, conflictID string) (*QueuedConflict, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	item, exists := q.items[conflictID]
	if !exists {
		return nil, fmt.Errorf("conflict not found in queue: %s", conflictID)
	}

	// Return a copy
	return &QueuedConflict{
		Conflict:   item.Conflict,
		QueuedAt:   item.QueuedAt,
		Priority:   item.Priority,
		AssignedTo: item.AssignedTo,
		Notes:      item.Notes,
		Status:     item.Status,
	}, nil
}

// ClearResolved removes all resolved conflicts from the queue.
func (q *InMemoryReviewQueue) ClearResolved(ctx context.Context) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	cleared := 0
	for id, item := range q.items {
		if item.Status == QueueStatusResolved {
			delete(q.items, id)
			cleared++
		}
	}

	q.logger.Info("Cleared resolved conflicts from queue",
		logging.F("count", cleared),
	)

	return cleared, nil
}
