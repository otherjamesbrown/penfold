// Package undo provides undo/redo functionality for review actions.
package undo

import (
	"time"
)

// Default limit values.
const (
	// DefaultMaxStackDepth is the default maximum number of actions in the undo stack.
	DefaultMaxStackDepth = 100

	// DefaultActionTTL is the default time-to-live for actions.
	DefaultActionTTL = 24 * time.Hour

	// DefaultMaxMemoryMB is the default maximum memory usage in megabytes.
	DefaultMaxMemoryMB = 50

	// DefaultMaxActionsPerSession is the default maximum actions per session.
	DefaultMaxActionsPerSession = 500

	// DefaultSessionTTL is the default time-to-live for sessions.
	DefaultSessionTTL = 7 * 24 * time.Hour

	// DefaultHistoryRetention is the default retention period for history.
	DefaultHistoryRetention = 90 * 24 * time.Hour
)

// Limits defines constraints on the undo stack.
type Limits struct {
	// MaxStackDepth is the maximum number of actions in the undo stack.
	// When exceeded, the oldest actions are removed.
	MaxStackDepth int `json:"max_stack_depth"`

	// ActionTTL is the time-to-live for actions.
	// Actions older than this cannot be undone.
	ActionTTL time.Duration `json:"action_ttl"`

	// MaxMemoryMB is the maximum memory usage for the stack in megabytes.
	MaxMemoryMB int `json:"max_memory_mb"`

	// MaxActionsPerSession is the maximum number of actions allowed per session.
	MaxActionsPerSession int `json:"max_actions_per_session"`

	// SessionTTL is the time-to-live for sessions.
	// Sessions older than this are pruned.
	SessionTTL time.Duration `json:"session_ttl"`

	// HistoryRetention is how long to retain history for compliance.
	HistoryRetention time.Duration `json:"history_retention"`

	// CrossSessionUndoWindow is the time window within which
	// actions from previous sessions can be undone.
	CrossSessionUndoWindow time.Duration `json:"cross_session_undo_window"`

	// MaxBatchSize is the maximum number of actions in a batch.
	MaxBatchSize int `json:"max_batch_size"`
}

// DefaultLimits returns sensible default limits.
func DefaultLimits() *Limits {
	return &Limits{
		MaxStackDepth:          DefaultMaxStackDepth,
		ActionTTL:              DefaultActionTTL,
		MaxMemoryMB:            DefaultMaxMemoryMB,
		MaxActionsPerSession:   DefaultMaxActionsPerSession,
		SessionTTL:             DefaultSessionTTL,
		HistoryRetention:       DefaultHistoryRetention,
		CrossSessionUndoWindow: 1 * time.Hour,
		MaxBatchSize:           50,
	}
}

// StrictLimits returns stricter limits for resource-constrained environments.
func StrictLimits() *Limits {
	return &Limits{
		MaxStackDepth:          25,
		ActionTTL:              4 * time.Hour,
		MaxMemoryMB:            10,
		MaxActionsPerSession:   100,
		SessionTTL:             24 * time.Hour,
		HistoryRetention:       30 * 24 * time.Hour,
		CrossSessionUndoWindow: 15 * time.Minute,
		MaxBatchSize:           10,
	}
}

// PermissiveLimits returns more permissive limits for high-resource environments.
func PermissiveLimits() *Limits {
	return &Limits{
		MaxStackDepth:          500,
		ActionTTL:              72 * time.Hour,
		MaxMemoryMB:            200,
		MaxActionsPerSession:   2000,
		SessionTTL:             30 * 24 * time.Hour,
		HistoryRetention:       365 * 24 * time.Hour,
		CrossSessionUndoWindow: 4 * time.Hour,
		MaxBatchSize:           100,
	}
}

// Validate checks if the limits are valid.
func (l *Limits) Validate() error {
	if l.MaxStackDepth <= 0 {
		l.MaxStackDepth = DefaultMaxStackDepth
	}
	if l.ActionTTL <= 0 {
		l.ActionTTL = DefaultActionTTL
	}
	if l.MaxMemoryMB <= 0 {
		l.MaxMemoryMB = DefaultMaxMemoryMB
	}
	if l.MaxActionsPerSession <= 0 {
		l.MaxActionsPerSession = DefaultMaxActionsPerSession
	}
	if l.SessionTTL <= 0 {
		l.SessionTTL = DefaultSessionTTL
	}
	if l.HistoryRetention <= 0 {
		l.HistoryRetention = DefaultHistoryRetention
	}
	if l.CrossSessionUndoWindow <= 0 {
		l.CrossSessionUndoWindow = 1 * time.Hour
	}
	if l.MaxBatchSize <= 0 {
		l.MaxBatchSize = 50
	}
	return nil
}

// Clone creates a deep copy of the limits.
func (l *Limits) Clone() *Limits {
	return &Limits{
		MaxStackDepth:          l.MaxStackDepth,
		ActionTTL:              l.ActionTTL,
		MaxMemoryMB:            l.MaxMemoryMB,
		MaxActionsPerSession:   l.MaxActionsPerSession,
		SessionTTL:             l.SessionTTL,
		HistoryRetention:       l.HistoryRetention,
		CrossSessionUndoWindow: l.CrossSessionUndoWindow,
		MaxBatchSize:           l.MaxBatchSize,
	}
}

// WithMaxStackDepth returns a copy with modified MaxStackDepth.
func (l *Limits) WithMaxStackDepth(depth int) *Limits {
	clone := l.Clone()
	clone.MaxStackDepth = depth
	return clone
}

// WithActionTTL returns a copy with modified ActionTTL.
func (l *Limits) WithActionTTL(ttl time.Duration) *Limits {
	clone := l.Clone()
	clone.ActionTTL = ttl
	return clone
}

// WithMaxMemoryMB returns a copy with modified MaxMemoryMB.
func (l *Limits) WithMaxMemoryMB(mb int) *Limits {
	clone := l.Clone()
	clone.MaxMemoryMB = mb
	return clone
}

// WithMaxActionsPerSession returns a copy with modified MaxActionsPerSession.
func (l *Limits) WithMaxActionsPerSession(max int) *Limits {
	clone := l.Clone()
	clone.MaxActionsPerSession = max
	return clone
}

// WithSessionTTL returns a copy with modified SessionTTL.
func (l *Limits) WithSessionTTL(ttl time.Duration) *Limits {
	clone := l.Clone()
	clone.SessionTTL = ttl
	return clone
}

// WithHistoryRetention returns a copy with modified HistoryRetention.
func (l *Limits) WithHistoryRetention(retention time.Duration) *Limits {
	clone := l.Clone()
	clone.HistoryRetention = retention
	return clone
}

// WithCrossSessionUndoWindow returns a copy with modified CrossSessionUndoWindow.
func (l *Limits) WithCrossSessionUndoWindow(window time.Duration) *Limits {
	clone := l.Clone()
	clone.CrossSessionUndoWindow = window
	return clone
}

// WithMaxBatchSize returns a copy with modified MaxBatchSize.
func (l *Limits) WithMaxBatchSize(size int) *Limits {
	clone := l.Clone()
	clone.MaxBatchSize = size
	return clone
}

// LimitChecker provides utilities for checking limits.
type LimitChecker struct {
	limits *Limits
}

// NewLimitChecker creates a new limit checker.
func NewLimitChecker(limits *Limits) *LimitChecker {
	if limits == nil {
		limits = DefaultLimits()
	}
	return &LimitChecker{limits: limits}
}

// CanAddAction checks if a new action can be added to the stack.
func (c *LimitChecker) CanAddAction(currentSize int) bool {
	return currentSize < c.limits.MaxStackDepth
}

// NeedsEviction returns how many actions need to be evicted to add one more.
func (c *LimitChecker) NeedsEviction(currentSize int) int {
	if currentSize < c.limits.MaxStackDepth {
		return 0
	}
	return currentSize - c.limits.MaxStackDepth + 1
}

// IsActionExpired checks if an action has expired based on its creation time.
func (c *LimitChecker) IsActionExpired(createdAt time.Time) bool {
	if c.limits.ActionTTL <= 0 {
		return false
	}
	return time.Since(createdAt) > c.limits.ActionTTL
}

// IsSessionExpired checks if a session has expired based on its last update.
func (c *LimitChecker) IsSessionExpired(lastUpdated time.Time) bool {
	if c.limits.SessionTTL <= 0 {
		return false
	}
	return time.Since(lastUpdated) > c.limits.SessionTTL
}

// CanUndoCrossSession checks if an action from a previous session can be undone.
func (c *LimitChecker) CanUndoCrossSession(actionCreatedAt time.Time) bool {
	if c.limits.CrossSessionUndoWindow <= 0 {
		return false
	}
	return time.Since(actionCreatedAt) <= c.limits.CrossSessionUndoWindow
}

// IsHistoryExpired checks if a history entry should be deleted.
func (c *LimitChecker) IsHistoryExpired(entryTimestamp time.Time) bool {
	if c.limits.HistoryRetention <= 0 {
		return false
	}
	return time.Since(entryTimestamp) > c.limits.HistoryRetention
}

// CanAddToBatch checks if more actions can be added to a batch.
func (c *LimitChecker) CanAddToBatch(currentBatchSize int) bool {
	return currentBatchSize < c.limits.MaxBatchSize
}

// MaxSessionActions returns the maximum actions allowed per session.
func (c *LimitChecker) MaxSessionActions() int {
	return c.limits.MaxActionsPerSession
}

// GetLimits returns the underlying limits.
func (c *LimitChecker) GetLimits() *Limits {
	return c.limits.Clone()
}

// MemoryEstimator provides utilities for estimating memory usage.
type MemoryEstimator struct {
	// BaseActionSizeBytes is the estimated base size of an action in bytes.
	BaseActionSizeBytes int

	// PerCharacterBytes is the estimated bytes per character in metadata.
	PerCharacterBytes int
}

// DefaultMemoryEstimator returns a default memory estimator.
func DefaultMemoryEstimator() *MemoryEstimator {
	return &MemoryEstimator{
		BaseActionSizeBytes: 512,  // 512 bytes base per action
		PerCharacterBytes:   2,    // 2 bytes per character (UTF-16)
	}
}

// EstimateActionSize estimates the memory size of an action in bytes.
func (e *MemoryEstimator) EstimateActionSize(action UndoableAction) int {
	size := e.BaseActionSizeBytes

	// Add description size
	size += len(action.Description()) * e.PerCharacterBytes

	// Estimate metadata size
	for k, v := range action.Metadata() {
		size += len(k) * e.PerCharacterBytes
		if strVal, ok := v.(string); ok {
			size += len(strVal) * e.PerCharacterBytes
		} else {
			size += 32 // Estimate for non-string values
		}
	}

	return size
}

// EstimateStackSize estimates the total memory size of a stack in bytes.
func (e *MemoryEstimator) EstimateStackSize(stack *Stack) int {
	size := 256 // Base stack overhead

	for _, action := range stack.GetUndoActions() {
		size += e.EstimateActionSize(action)
	}

	for _, action := range stack.GetRedoActions() {
		size += e.EstimateActionSize(action)
	}

	return size
}

// ExceedsMemoryLimit checks if the estimated size exceeds the memory limit.
func (e *MemoryEstimator) ExceedsMemoryLimit(stack *Stack, limitMB int) bool {
	estimatedBytes := e.EstimateStackSize(stack)
	limitBytes := limitMB * 1024 * 1024
	return estimatedBytes > limitBytes
}
