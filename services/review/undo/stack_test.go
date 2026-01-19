package undo

import (
	"context"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})
}

func TestNewStack(t *testing.T) {
	stack := NewStack(&StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Logger:    testLogger(),
	})

	if stack.TenantID() != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", stack.TenantID())
	}
	if stack.UserID() != "user-1" {
		t.Errorf("expected user ID 'user-1', got '%s'", stack.UserID())
	}
	if stack.SessionID() != "session-1" {
		t.Errorf("expected session ID 'session-1', got '%s'", stack.SessionID())
	}
	if stack.UndoSize() != 0 {
		t.Errorf("expected undo size 0, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 0 {
		t.Errorf("expected redo size 0, got %d", stack.RedoSize())
	}
	if stack.CreatedAt().IsZero() {
		t.Error("expected created_at to be set")
	}
}

func TestStackPush(t *testing.T) {
	stack := NewStack(&StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Logger:    testLogger(),
	})

	action := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	if err := stack.Push(action); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if stack.UndoSize() != 1 {
		t.Errorf("expected undo size 1, got %d", stack.UndoSize())
	}
	if !stack.CanUndo() {
		t.Error("expected CanUndo to be true")
	}
}

func TestStackPushClearsRedoStack(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Logger:    testLogger(),
	})

	action := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	_ = action.Execute(ctx) // Mark as executed
	_ = stack.Push(action)
	_, _ = stack.Undo(ctx)

	if stack.RedoSize() != 1 {
		t.Fatalf("expected redo size 1, got %d", stack.RedoSize())
	}

	// Push new action should clear redo stack
	newAction := NewAcceptAction("t", "u", "s", "c2", "email", "pending")
	_ = stack.Push(newAction)

	if stack.RedoSize() != 0 {
		t.Errorf("expected redo size 0 after push, got %d", stack.RedoSize())
	}
}

func TestStackPop(t *testing.T) {
	stack := NewStack(&StackConfig{SessionID: "s"})

	action := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	_ = stack.Push(action)

	popped, err := stack.Pop()
	if err != nil {
		t.Fatalf("Pop failed: %v", err)
	}

	if popped.ID() != action.ID() {
		t.Errorf("expected popped action ID %s, got %s", action.ID(), popped.ID())
	}

	if stack.UndoSize() != 0 {
		t.Errorf("expected undo size 0 after pop, got %d", stack.UndoSize())
	}
}

func TestStackPopEmpty(t *testing.T) {
	stack := NewStack(&StackConfig{SessionID: "s"})

	_, err := stack.Pop()
	if err != ErrStackEmpty {
		t.Errorf("expected ErrStackEmpty, got %v", err)
	}
}

func TestStackUndo(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Logger:    testLogger(),
	})

	undoCalled := false
	action := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	action.Executor = func(ctx context.Context, contentID string, act string) error {
		if act != "accept" {
			undoCalled = true
		}
		return nil
	}
	_ = action.Execute(ctx) // Execute first
	_ = stack.Push(action)

	undone, err := stack.Undo(ctx)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	if !undoCalled {
		t.Error("expected undo executor to be called")
	}
	if undone.ID() != action.ID() {
		t.Errorf("expected undone action ID %s, got %s", action.ID(), undone.ID())
	}
	if stack.UndoSize() != 0 {
		t.Errorf("expected undo size 0, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 1 {
		t.Errorf("expected redo size 1, got %d", stack.RedoSize())
	}
}

func TestStackUndoEmpty(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	_, err := stack.Undo(ctx)
	if err != ErrStackEmpty {
		t.Errorf("expected ErrStackEmpty, got %v", err)
	}
}

func TestStackRedo(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Logger:    testLogger(),
	})

	redoCalled := false
	action := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	_ = action.Execute(ctx)
	action.Executor = func(ctx context.Context, contentID string, act string) error {
		if act == "accept" {
			redoCalled = true
		}
		return nil
	}
	_ = stack.Push(action)
	_, _ = stack.Undo(ctx)

	redone, err := stack.Redo(ctx)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	if !redoCalled {
		t.Error("expected redo executor to be called")
	}
	if redone.ID() != action.ID() {
		t.Errorf("expected redone action ID %s, got %s", action.ID(), redone.ID())
	}
	if stack.UndoSize() != 1 {
		t.Errorf("expected undo size 1, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 0 {
		t.Errorf("expected redo size 0, got %d", stack.RedoSize())
	}
}

func TestStackRedoEmpty(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	_, err := stack.Redo(ctx)
	if err != ErrRedoStackEmpty {
		t.Errorf("expected ErrRedoStackEmpty, got %v", err)
	}
}

func TestStackUndoN(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	// Push 3 actions
	for i := 0; i < 3; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = stack.Push(action)
	}

	// Undo 2
	undone, err := stack.UndoN(ctx, 2)
	if err != nil {
		t.Fatalf("UndoN failed: %v", err)
	}

	if len(undone) != 2 {
		t.Errorf("expected 2 undone actions, got %d", len(undone))
	}
	if stack.UndoSize() != 1 {
		t.Errorf("expected undo size 1, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 2 {
		t.Errorf("expected redo size 2, got %d", stack.RedoSize())
	}
}

func TestStackRedoN(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	// Push and undo 3 actions
	for i := 0; i < 3; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = stack.Push(action)
	}
	_, _ = stack.UndoN(ctx, 3)

	// Redo 2
	redone, err := stack.RedoN(ctx, 2)
	if err != nil {
		t.Fatalf("RedoN failed: %v", err)
	}

	if len(redone) != 2 {
		t.Errorf("expected 2 redone actions, got %d", len(redone))
	}
	if stack.UndoSize() != 2 {
		t.Errorf("expected undo size 2, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 1 {
		t.Errorf("expected redo size 1, got %d", stack.RedoSize())
	}
}

func TestStackClear(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = stack.Push(action)
	_, _ = stack.Undo(ctx)

	stack.Clear()

	if stack.UndoSize() != 0 {
		t.Errorf("expected undo size 0, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 0 {
		t.Errorf("expected redo size 0, got %d", stack.RedoSize())
	}
}

func TestStackClearRedo(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = stack.Push(action)
	_, _ = stack.Undo(ctx)

	stack.ClearRedo()

	if stack.UndoSize() != 0 {
		t.Errorf("expected undo size 0, got %d", stack.UndoSize())
	}
	if stack.RedoSize() != 0 {
		t.Errorf("expected redo size 0, got %d", stack.RedoSize())
	}
}

func TestStackMaxDepth(t *testing.T) {
	limits := DefaultLimits().WithMaxStackDepth(3)
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Limits:    limits,
	})

	// Push 5 actions
	for i := 0; i < 5; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = stack.Push(action)
	}

	// Should only have 3 (max depth)
	if stack.UndoSize() != 3 {
		t.Errorf("expected undo size 3 (max depth), got %d", stack.UndoSize())
	}
}

func TestStackPeekUndo(t *testing.T) {
	stack := NewStack(&StackConfig{SessionID: "s"})

	action1 := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	action2 := NewAcceptAction("t", "u", "s", "c2", "email", "pending")
	_ = stack.Push(action1)
	_ = stack.Push(action2)

	peeked, err := stack.PeekUndo()
	if err != nil {
		t.Fatalf("PeekUndo failed: %v", err)
	}

	if peeked.ID() != action2.ID() {
		t.Errorf("expected peeked action2, got %s", peeked.ID())
	}

	// Stack should be unchanged
	if stack.UndoSize() != 2 {
		t.Errorf("expected undo size 2, got %d", stack.UndoSize())
	}
}

func TestStackPeekRedo(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = stack.Push(action)
	_, _ = stack.Undo(ctx)

	peeked, err := stack.PeekRedo()
	if err != nil {
		t.Fatalf("PeekRedo failed: %v", err)
	}

	if peeked.ID() != action.ID() {
		t.Errorf("expected peeked action, got %s", peeked.ID())
	}

	// Stack should be unchanged
	if stack.RedoSize() != 1 {
		t.Errorf("expected redo size 1, got %d", stack.RedoSize())
	}
}

func TestStackGetActions(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	action1 := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	action2 := NewAcceptAction("t", "u", "s", "c2", "email", "pending")
	_ = action1.Execute(ctx)
	_ = action2.Execute(ctx)
	_ = stack.Push(action1)
	_ = stack.Push(action2)
	_, _ = stack.Undo(ctx)

	undoActions := stack.GetUndoActions()
	if len(undoActions) != 1 {
		t.Errorf("expected 1 undo action, got %d", len(undoActions))
	}

	redoActions := stack.GetRedoActions()
	if len(redoActions) != 1 {
		t.Errorf("expected 1 redo action, got %d", len(redoActions))
	}
}

func TestStackCallbacks(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	pushCalled := false
	undoCalled := false
	redoCalled := false

	stack.SetOnPush(func(action UndoableAction) {
		pushCalled = true
	})
	stack.SetOnUndo(func(action UndoableAction) {
		undoCalled = true
	})
	stack.SetOnRedo(func(action UndoableAction) {
		redoCalled = true
	})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = stack.Push(action)

	if !pushCalled {
		t.Error("expected push callback to be called")
	}

	_, _ = stack.Undo(ctx)
	if !undoCalled {
		t.Error("expected undo callback to be called")
	}

	_, _ = stack.Redo(ctx)
	if !redoCalled {
		t.Error("expected redo callback to be called")
	}
}

func TestStackPruneExpired(t *testing.T) {
	limits := DefaultLimits().WithActionTTL(10 * time.Millisecond)
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Limits:    limits,
	})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = stack.Push(action)

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	pruned := stack.PruneExpired()
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	if stack.UndoSize() != 0 {
		t.Errorf("expected undo size 0, got %d", stack.UndoSize())
	}
}

func TestStackGroupedUndo(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Logger:    testLogger(),
	})

	var actionIDs []string
	for i := 0; i < 5; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = stack.Push(action)
		actionIDs = append(actionIDs, action.ID())
	}

	// Grouped undo from action 3 (index 2)
	undone, err := stack.GroupedUndo(ctx, actionIDs[2])
	if err != nil {
		t.Fatalf("GroupedUndo failed: %v", err)
	}

	// Should have undone actions 5, 4, 3 (3 actions)
	if len(undone) != 3 {
		t.Errorf("expected 3 undone actions, got %d", len(undone))
	}

	if stack.UndoSize() != 2 {
		t.Errorf("expected undo size 2, got %d", stack.UndoSize())
	}

	if stack.RedoSize() != 3 {
		t.Errorf("expected redo size 3, got %d", stack.RedoSize())
	}
}

func TestStackSelectiveUndo(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	action1 := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	action2 := NewAcceptAction("t", "u", "s", "c2", "email", "pending")
	_ = action1.Execute(ctx)
	_ = action2.Execute(ctx)
	_ = stack.Push(action1)
	_ = stack.Push(action2)

	// Try to undo action1 (not at top) - should fail
	_, err := stack.SelectiveUndo(ctx, action1.ID())
	if err != ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}

	// Undo action2 (at top) - should work
	undone, err := stack.SelectiveUndo(ctx, action2.ID())
	if err != nil {
		t.Fatalf("SelectiveUndo failed: %v", err)
	}
	if undone.ID() != action2.ID() {
		t.Errorf("expected undone action2, got %s", undone.ID())
	}
}

func TestStackStats(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Limits:    DefaultLimits(),
	})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = stack.Push(action)

	stats := stack.GetStats()

	if stats.UndoCount != 1 {
		t.Errorf("expected undo count 1, got %d", stats.UndoCount)
	}
	if stats.RedoCount != 0 {
		t.Errorf("expected redo count 0, got %d", stats.RedoCount)
	}
	if stats.MaxStackDepth != DefaultMaxStackDepth {
		t.Errorf("expected max stack depth %d, got %d", DefaultMaxStackDepth, stats.MaxStackDepth)
	}
	if stats.OldestActionAt == nil {
		t.Error("expected oldest_action_at to be set")
	}
	if stats.NewestActionAt == nil {
		t.Error("expected newest_action_at to be set")
	}
}

func TestStackActionExpired(t *testing.T) {
	ctx := context.Background()
	limits := DefaultLimits().WithActionTTL(10 * time.Millisecond)
	stack := NewStack(&StackConfig{
		SessionID: "s",
		Limits:    limits,
	})

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = stack.Push(action)

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	_, err := stack.Undo(ctx)
	if err != ErrActionExpired {
		t.Errorf("expected ErrActionExpired, got %v", err)
	}
}

func TestStackConcurrency(t *testing.T) {
	ctx := context.Background()
	stack := NewStack(&StackConfig{SessionID: "s"})

	// Concurrent pushes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
			_ = action.Execute(ctx)
			_ = stack.Push(action)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if stack.UndoSize() != 10 {
		t.Errorf("expected undo size 10, got %d", stack.UndoSize())
	}
}
