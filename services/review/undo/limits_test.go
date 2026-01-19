package undo

import (
	"testing"
	"time"
)

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.MaxStackDepth != DefaultMaxStackDepth {
		t.Errorf("expected max stack depth %d, got %d", DefaultMaxStackDepth, limits.MaxStackDepth)
	}
	if limits.ActionTTL != DefaultActionTTL {
		t.Errorf("expected action TTL %v, got %v", DefaultActionTTL, limits.ActionTTL)
	}
	if limits.MaxMemoryMB != DefaultMaxMemoryMB {
		t.Errorf("expected max memory %dMB, got %d", DefaultMaxMemoryMB, limits.MaxMemoryMB)
	}
	if limits.MaxActionsPerSession != DefaultMaxActionsPerSession {
		t.Errorf("expected max actions per session %d, got %d", DefaultMaxActionsPerSession, limits.MaxActionsPerSession)
	}
	if limits.SessionTTL != DefaultSessionTTL {
		t.Errorf("expected session TTL %v, got %v", DefaultSessionTTL, limits.SessionTTL)
	}
	if limits.HistoryRetention != DefaultHistoryRetention {
		t.Errorf("expected history retention %v, got %v", DefaultHistoryRetention, limits.HistoryRetention)
	}
}

func TestStrictLimits(t *testing.T) {
	limits := StrictLimits()

	if limits.MaxStackDepth != 25 {
		t.Errorf("expected max stack depth 25, got %d", limits.MaxStackDepth)
	}
	if limits.ActionTTL != 4*time.Hour {
		t.Errorf("expected action TTL 4h, got %v", limits.ActionTTL)
	}
	if limits.MaxMemoryMB != 10 {
		t.Errorf("expected max memory 10MB, got %d", limits.MaxMemoryMB)
	}
}

func TestPermissiveLimits(t *testing.T) {
	limits := PermissiveLimits()

	if limits.MaxStackDepth != 500 {
		t.Errorf("expected max stack depth 500, got %d", limits.MaxStackDepth)
	}
	if limits.ActionTTL != 72*time.Hour {
		t.Errorf("expected action TTL 72h, got %v", limits.ActionTTL)
	}
	if limits.MaxMemoryMB != 200 {
		t.Errorf("expected max memory 200MB, got %d", limits.MaxMemoryMB)
	}
}

func TestLimitsValidate(t *testing.T) {
	limits := &Limits{
		MaxStackDepth: -1,
		ActionTTL:     0,
	}

	if err := limits.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Invalid values should be replaced with defaults
	if limits.MaxStackDepth != DefaultMaxStackDepth {
		t.Errorf("expected max stack depth to be corrected to %d, got %d", DefaultMaxStackDepth, limits.MaxStackDepth)
	}
	if limits.ActionTTL != DefaultActionTTL {
		t.Errorf("expected action TTL to be corrected to %v, got %v", DefaultActionTTL, limits.ActionTTL)
	}
}

func TestLimitsClone(t *testing.T) {
	original := DefaultLimits()
	clone := original.Clone()

	// Modify clone
	clone.MaxStackDepth = 999

	// Original should be unchanged
	if original.MaxStackDepth != DefaultMaxStackDepth {
		t.Error("clone modified original")
	}
}

func TestLimitsWithMethods(t *testing.T) {
	limits := DefaultLimits()

	// Test each With method
	modified := limits.WithMaxStackDepth(50)
	if modified.MaxStackDepth != 50 {
		t.Errorf("WithMaxStackDepth: expected 50, got %d", modified.MaxStackDepth)
	}
	if limits.MaxStackDepth == 50 {
		t.Error("WithMaxStackDepth modified original")
	}

	modified = limits.WithActionTTL(1 * time.Hour)
	if modified.ActionTTL != 1*time.Hour {
		t.Errorf("WithActionTTL: expected 1h, got %v", modified.ActionTTL)
	}

	modified = limits.WithMaxMemoryMB(100)
	if modified.MaxMemoryMB != 100 {
		t.Errorf("WithMaxMemoryMB: expected 100, got %d", modified.MaxMemoryMB)
	}

	modified = limits.WithMaxActionsPerSession(1000)
	if modified.MaxActionsPerSession != 1000 {
		t.Errorf("WithMaxActionsPerSession: expected 1000, got %d", modified.MaxActionsPerSession)
	}

	modified = limits.WithSessionTTL(48 * time.Hour)
	if modified.SessionTTL != 48*time.Hour {
		t.Errorf("WithSessionTTL: expected 48h, got %v", modified.SessionTTL)
	}

	modified = limits.WithHistoryRetention(180 * 24 * time.Hour)
	if modified.HistoryRetention != 180*24*time.Hour {
		t.Errorf("WithHistoryRetention: expected 180d, got %v", modified.HistoryRetention)
	}

	modified = limits.WithCrossSessionUndoWindow(2 * time.Hour)
	if modified.CrossSessionUndoWindow != 2*time.Hour {
		t.Errorf("WithCrossSessionUndoWindow: expected 2h, got %v", modified.CrossSessionUndoWindow)
	}

	modified = limits.WithMaxBatchSize(25)
	if modified.MaxBatchSize != 25 {
		t.Errorf("WithMaxBatchSize: expected 25, got %d", modified.MaxBatchSize)
	}
}

func TestLimitChecker(t *testing.T) {
	limits := DefaultLimits().WithMaxStackDepth(10)
	checker := NewLimitChecker(limits)

	// CanAddAction
	if !checker.CanAddAction(5) {
		t.Error("expected CanAddAction(5) to be true")
	}
	if !checker.CanAddAction(9) {
		t.Error("expected CanAddAction(9) to be true")
	}
	if checker.CanAddAction(10) {
		t.Error("expected CanAddAction(10) to be false")
	}
}

func TestLimitCheckerNeedsEviction(t *testing.T) {
	limits := DefaultLimits().WithMaxStackDepth(10)
	checker := NewLimitChecker(limits)

	if checker.NeedsEviction(5) != 0 {
		t.Errorf("expected no eviction for size 5, got %d", checker.NeedsEviction(5))
	}
	if checker.NeedsEviction(10) != 1 {
		t.Errorf("expected 1 eviction for size 10, got %d", checker.NeedsEviction(10))
	}
	if checker.NeedsEviction(15) != 6 {
		t.Errorf("expected 6 evictions for size 15, got %d", checker.NeedsEviction(15))
	}
}

func TestLimitCheckerIsActionExpired(t *testing.T) {
	limits := DefaultLimits().WithActionTTL(1 * time.Hour)
	checker := NewLimitChecker(limits)

	// Recent action
	if checker.IsActionExpired(time.Now().Add(-30 * time.Minute)) {
		t.Error("expected 30min old action to not be expired")
	}

	// Old action
	if !checker.IsActionExpired(time.Now().Add(-2 * time.Hour)) {
		t.Error("expected 2h old action to be expired")
	}
}

func TestLimitCheckerIsSessionExpired(t *testing.T) {
	limits := DefaultLimits().WithSessionTTL(24 * time.Hour)
	checker := NewLimitChecker(limits)

	// Recent session
	if checker.IsSessionExpired(time.Now().Add(-12 * time.Hour)) {
		t.Error("expected 12h old session to not be expired")
	}

	// Old session
	if !checker.IsSessionExpired(time.Now().Add(-48 * time.Hour)) {
		t.Error("expected 48h old session to be expired")
	}
}

func TestLimitCheckerCanUndoCrossSession(t *testing.T) {
	limits := DefaultLimits().WithCrossSessionUndoWindow(1 * time.Hour)
	checker := NewLimitChecker(limits)

	// Within window
	if !checker.CanUndoCrossSession(time.Now().Add(-30 * time.Minute)) {
		t.Error("expected cross-session undo within 30min")
	}

	// Outside window
	if checker.CanUndoCrossSession(time.Now().Add(-2 * time.Hour)) {
		t.Error("expected no cross-session undo after 2h")
	}
}

func TestLimitCheckerIsHistoryExpired(t *testing.T) {
	limits := DefaultLimits().WithHistoryRetention(90 * 24 * time.Hour)
	checker := NewLimitChecker(limits)

	// Recent history
	if checker.IsHistoryExpired(time.Now().Add(-30 * 24 * time.Hour)) {
		t.Error("expected 30d old history to not be expired")
	}

	// Old history
	if !checker.IsHistoryExpired(time.Now().Add(-180 * 24 * time.Hour)) {
		t.Error("expected 180d old history to be expired")
	}
}

func TestLimitCheckerCanAddToBatch(t *testing.T) {
	limits := DefaultLimits().WithMaxBatchSize(10)
	checker := NewLimitChecker(limits)

	if !checker.CanAddToBatch(5) {
		t.Error("expected CanAddToBatch(5) to be true")
	}
	if checker.CanAddToBatch(10) {
		t.Error("expected CanAddToBatch(10) to be false")
	}
}

func TestLimitCheckerMaxSessionActions(t *testing.T) {
	limits := DefaultLimits().WithMaxActionsPerSession(1000)
	checker := NewLimitChecker(limits)

	if checker.MaxSessionActions() != 1000 {
		t.Errorf("expected max session actions 1000, got %d", checker.MaxSessionActions())
	}
}

func TestLimitCheckerGetLimits(t *testing.T) {
	limits := DefaultLimits()
	checker := NewLimitChecker(limits)

	retrieved := checker.GetLimits()

	if retrieved.MaxStackDepth != limits.MaxStackDepth {
		t.Errorf("expected same max stack depth")
	}

	// Should be a clone
	retrieved.MaxStackDepth = 999
	if limits.MaxStackDepth == 999 {
		t.Error("GetLimits should return a clone")
	}
}

func TestNewLimitCheckerNilLimits(t *testing.T) {
	checker := NewLimitChecker(nil)

	// Should use defaults
	if checker.GetLimits().MaxStackDepth != DefaultMaxStackDepth {
		t.Error("expected default limits when nil passed")
	}
}

func TestMemoryEstimator(t *testing.T) {
	estimator := DefaultMemoryEstimator()

	if estimator.BaseActionSizeBytes != 512 {
		t.Errorf("expected base action size 512, got %d", estimator.BaseActionSizeBytes)
	}
	if estimator.PerCharacterBytes != 2 {
		t.Errorf("expected per character bytes 2, got %d", estimator.PerCharacterBytes)
	}
}

func TestMemoryEstimatorEstimateActionSize(t *testing.T) {
	estimator := DefaultMemoryEstimator()

	action := NewAcceptAction("tenant-1", "user-1", "session-1", "content-1", "email", "pending")
	action.SetMetadata("key", "value")

	size := estimator.EstimateActionSize(action)

	// Should be at least base size
	if size < estimator.BaseActionSizeBytes {
		t.Errorf("expected size >= %d, got %d", estimator.BaseActionSizeBytes, size)
	}

	// Adding more metadata should increase size
	action.SetMetadata("longkey", "this is a much longer value that should increase the estimated size")
	largerSize := estimator.EstimateActionSize(action)

	if largerSize <= size {
		t.Error("expected larger size with more metadata")
	}
}

func TestMemoryEstimatorEstimateStackSize(t *testing.T) {
	estimator := DefaultMemoryEstimator()

	stack := NewStack(&StackConfig{SessionID: "s"})

	emptySize := estimator.EstimateStackSize(stack)

	// Add actions
	for i := 0; i < 10; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = stack.Push(action)
	}

	populatedSize := estimator.EstimateStackSize(stack)

	if populatedSize <= emptySize {
		t.Error("expected larger size with actions")
	}
}

func TestMemoryEstimatorExceedsMemoryLimit(t *testing.T) {
	estimator := DefaultMemoryEstimator()

	stack := NewStack(&StackConfig{SessionID: "s"})

	// Empty stack should not exceed limit
	if estimator.ExceedsMemoryLimit(stack, 1) {
		t.Error("empty stack should not exceed 1MB limit")
	}

	// Add many actions
	for i := 0; i < 1000; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = stack.Push(action)
	}

	// With small limit, should exceed
	if !estimator.ExceedsMemoryLimit(stack, 0) {
		t.Error("stack should exceed 0MB limit")
	}
}

func TestLimitCheckerZeroTTL(t *testing.T) {
	limits := &Limits{
		MaxStackDepth: 100,
		ActionTTL:     0, // Disabled
		SessionTTL:    0, // Disabled
	}
	checker := NewLimitChecker(limits)

	// With TTL disabled, nothing should expire
	if checker.IsActionExpired(time.Now().Add(-100 * 24 * time.Hour)) {
		t.Error("action should not expire with TTL=0")
	}
	if checker.IsSessionExpired(time.Now().Add(-100 * 24 * time.Hour)) {
		t.Error("session should not expire with TTL=0")
	}
}
