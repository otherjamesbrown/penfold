package undo

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBaseAction(t *testing.T) {
	action := NewBaseAction(ActionTypeAccept, "Test action", "tenant-1", "user-1", "session-1")

	if action.ID() == "" {
		t.Error("expected action ID to be set")
	}
	if action.Type() != ActionTypeAccept {
		t.Errorf("expected type %s, got %s", ActionTypeAccept, action.Type())
	}
	if action.Description() != "Test action" {
		t.Errorf("expected description 'Test action', got '%s'", action.Description())
	}
	if action.State() != ActionStatePending {
		t.Errorf("expected state %s, got %s", ActionStatePending, action.State())
	}
	if action.TenantID() != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", action.TenantID())
	}
	if action.UserID() != "user-1" {
		t.Errorf("expected user ID 'user-1', got '%s'", action.UserID())
	}
	if action.SessionID() != "session-1" {
		t.Errorf("expected session ID 'session-1', got '%s'", action.SessionID())
	}
	if action.CreatedAt().IsZero() {
		t.Error("expected created_at to be set")
	}
	if action.ExecutedAt() != nil {
		t.Error("expected executed_at to be nil")
	}
}

func TestBaseActionCanUndoRedo(t *testing.T) {
	action := NewBaseAction(ActionTypeAccept, "Test", "t", "u", "s")

	// Pending state - cannot undo or redo
	if action.CanUndo() {
		t.Error("pending action should not be undoable")
	}
	if action.CanRedo() {
		t.Error("pending action should not be redoable")
	}

	// Executed state - can undo
	action.MarkExecuted()
	if !action.CanUndo() {
		t.Error("executed action should be undoable")
	}
	if action.CanRedo() {
		t.Error("executed action should not be redoable")
	}
	if action.ExecutedAt() == nil {
		t.Error("expected executed_at to be set after MarkExecuted")
	}

	// Undone state - can redo
	action.SetState(ActionStateUndone)
	if action.CanUndo() {
		t.Error("undone action should not be undoable")
	}
	if !action.CanRedo() {
		t.Error("undone action should be redoable")
	}

	// Redone state - can undo again
	action.SetState(ActionStateRedone)
	if !action.CanUndo() {
		t.Error("redone action should be undoable")
	}
	if action.CanRedo() {
		t.Error("redone action should not be redoable")
	}
}

func TestAcceptAction(t *testing.T) {
	ctx := context.Background()
	executorCalled := false
	executorAction := ""

	action := NewAcceptAction("tenant-1", "user-1", "session-1", "content-1", "email", "pending")
	action.Executor = func(ctx context.Context, contentID string, act string) error {
		executorCalled = true
		executorAction = act
		return nil
	}

	// Test Execute
	if err := action.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !executorCalled {
		t.Error("executor should have been called")
	}
	if executorAction != "accept" {
		t.Errorf("expected action 'accept', got '%s'", executorAction)
	}
	if action.State() != ActionStateExecuted {
		t.Errorf("expected state %s, got %s", ActionStateExecuted, action.State())
	}

	// Test Undo
	executorCalled = false
	if err := action.Undo(ctx); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !executorCalled {
		t.Error("executor should have been called on undo")
	}
	if executorAction != "pending" {
		t.Errorf("expected undo to restore 'pending', got '%s'", executorAction)
	}
	if action.State() != ActionStateUndone {
		t.Errorf("expected state %s, got %s", ActionStateUndone, action.State())
	}

	// Test Redo
	executorCalled = false
	if err := action.Redo(ctx); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !executorCalled {
		t.Error("executor should have been called on redo")
	}
	if executorAction != "accept" {
		t.Errorf("expected redo to reapply 'accept', got '%s'", executorAction)
	}
	if action.State() != ActionStateRedone {
		t.Errorf("expected state %s, got %s", ActionStateRedone, action.State())
	}
}

func TestAcceptActionJSON(t *testing.T) {
	action := NewAcceptAction("tenant-1", "user-1", "session-1", "content-1", "email", "pending")

	data, err := action.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}

	if parsed["content_id"] != "content-1" {
		t.Errorf("expected content_id 'content-1', got '%v'", parsed["content_id"])
	}
	if parsed["content_type"] != "email" {
		t.Errorf("expected content_type 'email', got '%v'", parsed["content_type"])
	}
	if parsed["previous_state"] != "pending" {
		t.Errorf("expected previous_state 'pending', got '%v'", parsed["previous_state"])
	}
}

func TestRejectAction(t *testing.T) {
	ctx := context.Background()
	executorCalled := false

	action := NewRejectAction("tenant-1", "user-1", "session-1", "content-1", "email", "pending", "spam")
	action.Executor = func(ctx context.Context, contentID string, act string) error {
		executorCalled = true
		return nil
	}

	if action.Reason != "spam" {
		t.Errorf("expected reason 'spam', got '%s'", action.Reason)
	}

	if err := action.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !executorCalled {
		t.Error("executor should have been called")
	}
}

func TestDeferAction(t *testing.T) {
	ctx := context.Background()
	deferUntil := time.Now().Add(24 * time.Hour)

	action := NewDeferAction("tenant-1", "user-1", "session-1", "content-1", "email", "pending", "too busy", &deferUntil)

	if action.Reason != "too busy" {
		t.Errorf("expected reason 'too busy', got '%s'", action.Reason)
	}
	if action.DeferUntil == nil {
		t.Error("expected defer_until to be set")
	}

	// Execute without executor should work
	if err := action.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if action.State() != ActionStateExecuted {
		t.Errorf("expected state %s, got %s", ActionStateExecuted, action.State())
	}
}

func TestTagAction(t *testing.T) {
	ctx := context.Background()
	addedTags := []string(nil)
	removedTags := []string(nil)

	action := NewTagAction(
		"tenant-1", "user-1", "session-1",
		"content-1", "email",
		[]string{"important", "urgent"},
		[]string{"spam"},
		[]string{"spam", "read"},
	)
	action.TagExecutor = func(ctx context.Context, contentID string, add, remove []string) error {
		addedTags = add
		removedTags = remove
		return nil
	}

	// Execute
	if err := action.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(addedTags) != 2 {
		t.Errorf("expected 2 added tags, got %d", len(addedTags))
	}
	if len(removedTags) != 1 {
		t.Errorf("expected 1 removed tag, got %d", len(removedTags))
	}

	// Undo should reverse the tags
	if err := action.Undo(ctx); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	// After undo: addedTags should be what was removed, removedTags should be what was added
	if len(addedTags) != 1 {
		t.Errorf("expected 1 added tag on undo (restoring 'spam'), got %d", len(addedTags))
	}
	if len(removedTags) != 2 {
		t.Errorf("expected 2 removed tags on undo (removing 'important', 'urgent'), got %d", len(removedTags))
	}
}

func TestMergeAction(t *testing.T) {
	ctx := context.Background()
	mergeSourceIDs := []string(nil)
	mergeTargetID := ""
	splitCalled := false

	action := NewMergeAction("tenant-1", "user-1", "session-1", []string{"item-1", "item-2"}, "merged-1", "email")
	action.MergeExecutor = func(ctx context.Context, sourceIDs []string, targetID string) error {
		mergeSourceIDs = sourceIDs
		mergeTargetID = targetID
		return nil
	}
	action.SplitExecutor = func(ctx context.Context, targetID string, sourceIDs []string) error {
		splitCalled = true
		return nil
	}

	// Execute
	if err := action.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(mergeSourceIDs) != 2 {
		t.Errorf("expected 2 source IDs, got %d", len(mergeSourceIDs))
	}
	if mergeTargetID != "merged-1" {
		t.Errorf("expected target ID 'merged-1', got '%s'", mergeTargetID)
	}

	// Undo should call split
	if err := action.Undo(ctx); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !splitCalled {
		t.Error("split executor should have been called on undo")
	}
}

func TestSplitAction(t *testing.T) {
	ctx := context.Background()
	splitSourceID := ""
	mergeCalled := false

	action := NewSplitAction("tenant-1", "user-1", "session-1", "source-1", "email")
	action.SplitExecutor = func(ctx context.Context, sourceID string) ([]string, error) {
		splitSourceID = sourceID
		return []string{"split-1", "split-2"}, nil
	}
	action.MergeExecutor = func(ctx context.Context, targetIDs []string, sourceID string) error {
		mergeCalled = true
		return nil
	}

	// Execute
	if err := action.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if splitSourceID != "source-1" {
		t.Errorf("expected source ID 'source-1', got '%s'", splitSourceID)
	}
	if len(action.TargetIDs) != 2 {
		t.Errorf("expected 2 target IDs, got %d", len(action.TargetIDs))
	}

	// Undo should call merge
	if err := action.Undo(ctx); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !mergeCalled {
		t.Error("merge executor should have been called on undo")
	}
}

func TestBatchAction(t *testing.T) {
	ctx := context.Background()
	executeCount := 0
	undoCount := 0
	redoCount := 0

	// Create actions for batch
	action1 := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	action1.Executor = func(ctx context.Context, contentID string, act string) error {
		if act == "accept" {
			executeCount++
		}
		return nil
	}

	action2 := NewAcceptAction("t", "u", "s", "c2", "email", "pending")
	action2.Executor = func(ctx context.Context, contentID string, act string) error {
		if act == "accept" {
			executeCount++
		}
		return nil
	}

	batch := NewBatchAction("t", "u", "s", "Batch accept", []UndoableAction{action1, action2})

	// Execute batch
	if err := batch.Execute(ctx); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if executeCount != 2 {
		t.Errorf("expected 2 executions, got %d", executeCount)
	}
	if batch.ActionCount != 2 {
		t.Errorf("expected action count 2, got %d", batch.ActionCount)
	}

	// Reset executor to track undos
	action1.Executor = func(ctx context.Context, contentID string, act string) error {
		undoCount++
		return nil
	}
	action2.Executor = func(ctx context.Context, contentID string, act string) error {
		undoCount++
		return nil
	}

	// Undo batch
	if err := batch.Undo(ctx); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if undoCount != 2 {
		t.Errorf("expected 2 undos, got %d", undoCount)
	}

	// Reset executor to track redos
	action1.Executor = func(ctx context.Context, contentID string, act string) error {
		redoCount++
		return nil
	}
	action2.Executor = func(ctx context.Context, contentID string, act string) error {
		redoCount++
		return nil
	}

	// Redo batch
	if err := batch.Redo(ctx); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if redoCount != 2 {
		t.Errorf("expected 2 redos, got %d", redoCount)
	}
}

func TestBatchActionGetActions(t *testing.T) {
	action1 := NewAcceptAction("t", "u", "s", "c1", "email", "pending")
	action2 := NewRejectAction("t", "u", "s", "c2", "email", "pending", "reason")

	batch := NewBatchAction("t", "u", "s", "Mixed batch", []UndoableAction{action1, action2})

	actions := batch.GetActions()
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}

func TestActionNotUndoableError(t *testing.T) {
	ctx := context.Background()
	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")

	// Try to undo before execute
	err := action.Undo(ctx)
	if err != ErrActionNotUndoable {
		t.Errorf("expected ErrActionNotUndoable, got %v", err)
	}
}

func TestActionNotRedoableError(t *testing.T) {
	ctx := context.Background()
	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")

	// Execute first
	_ = action.Execute(ctx)

	// Try to redo without undo
	err := action.Redo(ctx)
	if err != ErrActionNotRedoable {
		t.Errorf("expected ErrActionNotRedoable, got %v", err)
	}
}

func TestBaseActionMetadata(t *testing.T) {
	action := NewBaseAction(ActionTypeAccept, "Test", "t", "u", "s")

	action.SetMetadata("key1", "value1")
	action.SetMetadata("key2", 42)

	meta := action.Metadata()
	if meta["key1"] != "value1" {
		t.Errorf("expected key1='value1', got '%v'", meta["key1"])
	}
	if meta["key2"] != 42 {
		t.Errorf("expected key2=42, got '%v'", meta["key2"])
	}
}
