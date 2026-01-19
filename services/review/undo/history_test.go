package undo

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryHistoryStoreRecordOperation(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	action := NewAcceptAction("tenant-1", "user-1", "session-1", "content-1", "email", "pending")
	_ = action.Execute(ctx)

	if err := store.RecordOperation(ctx, action, OperationExecute); err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	entries, err := store.GetHistory(ctx, HistoryFilter{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestInMemoryHistoryStoreGetHistory(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	// Record multiple operations
	action1 := NewAcceptAction("tenant-1", "user-1", "session-1", "c1", "email", "pending")
	action2 := NewRejectAction("tenant-1", "user-1", "session-1", "c2", "email", "pending", "spam")
	action3 := NewAcceptAction("tenant-1", "user-2", "session-2", "c3", "email", "pending")

	_ = action1.Execute(ctx)
	_ = action2.Execute(ctx)
	_ = action3.Execute(ctx)

	_ = store.RecordOperation(ctx, action1, OperationExecute)
	_ = store.RecordOperation(ctx, action2, OperationExecute)
	_ = store.RecordOperation(ctx, action3, OperationExecute)

	// Filter by session
	entries, err := store.GetHistory(ctx, HistoryFilter{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for session-1, got %d", len(entries))
	}

	// Filter by user
	entries, err = store.GetHistory(ctx, HistoryFilter{UserID: "user-1"})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for user-1, got %d", len(entries))
	}

	// Filter by action type
	actionType := ActionTypeAccept
	entries, err = store.GetHistory(ctx, HistoryFilter{ActionType: &actionType})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 accept entries, got %d", len(entries))
	}
}

func TestInMemoryHistoryStoreGetHistoryWithLimit(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	// Record 5 operations
	for i := 0; i < 5; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = store.RecordOperation(ctx, action, OperationExecute)
	}

	// Get with limit
	entries, err := store.GetHistory(ctx, HistoryFilter{Limit: 3})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries with limit, got %d", len(entries))
	}
}

func TestInMemoryHistoryStoreGetHistoryWithOffset(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	// Record 5 operations
	for i := 0; i < 5; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = store.RecordOperation(ctx, action, OperationExecute)
	}

	// Get with offset
	entries, err := store.GetHistory(ctx, HistoryFilter{Offset: 2})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries after offset, got %d", len(entries))
	}
}

func TestInMemoryHistoryStoreGetActionHistory(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)

	// Record multiple operations for same action
	_ = store.RecordOperation(ctx, action, OperationExecute)
	action.SetState(ActionStateUndone)
	_ = store.RecordOperation(ctx, action, OperationUndo)
	action.SetState(ActionStateRedone)
	_ = store.RecordOperation(ctx, action, OperationRedo)

	entries, err := store.GetActionHistory(ctx, action.ID())
	if err != nil {
		t.Fatalf("GetActionHistory failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 history entries for action, got %d", len(entries))
	}
}

func TestInMemoryHistoryStoreDeleteHistory(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = store.RecordOperation(ctx, action, OperationExecute)

	// Wait and delete
	time.Sleep(10 * time.Millisecond)
	deleted, err := store.DeleteHistory(ctx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("DeleteHistory failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	entries, _ := store.GetHistory(ctx, HistoryFilter{})
	if len(entries) != 0 {
		t.Error("expected history to be empty")
	}
}

func TestInMemoryHistoryStoreGetHistoryCount(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = store.RecordOperation(ctx, action, OperationExecute)
	}

	count, err := store.GetHistoryCount(ctx, HistoryFilter{})
	if err != nil {
		t.Fatalf("GetHistoryCount failed: %v", err)
	}

	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}
}

func TestHistoryEntryToAuditLog(t *testing.T) {
	entry := HistoryEntry{
		ID:          "entry-1",
		ActionID:    "action-1",
		ActionType:  ActionTypeAccept,
		Description: "Accept email: content-1",
		Operation:   OperationExecute,
		TenantID:    "tenant-1",
		UserID:      "user-1",
		SessionID:   "session-1",
		Timestamp:   time.Now(),
		ActionData: map[string]interface{}{
			"content_id":   "content-1",
			"content_type": "email",
		},
	}

	audit := entry.ToAuditLog()

	if audit.UserID != "user-1" {
		t.Errorf("expected user ID 'user-1', got '%s'", audit.UserID)
	}
	if audit.TenantID != "tenant-1" {
		t.Errorf("expected tenant ID 'tenant-1', got '%s'", audit.TenantID)
	}
	if audit.Operation != OperationExecute {
		t.Errorf("expected operation %s, got %s", OperationExecute, audit.Operation)
	}
	if audit.TargetID != "content-1" {
		t.Errorf("expected target ID 'content-1', got '%s'", audit.TargetID)
	}
	if audit.TargetType != "email" {
		t.Errorf("expected target type 'email', got '%s'", audit.TargetType)
	}
	if audit.Result != "success" {
		t.Errorf("expected result 'success', got '%s'", audit.Result)
	}
}

func TestHistoryEntryToAuditLogMerge(t *testing.T) {
	entry := HistoryEntry{
		ActionType: ActionTypeMerge,
		ActionData: map[string]interface{}{
			"target_id": "merged-1",
		},
	}

	audit := entry.ToAuditLog()

	if audit.TargetType != "merged_item" {
		t.Errorf("expected target type 'merged_item', got '%s'", audit.TargetType)
	}
	if audit.TargetID != "merged-1" {
		t.Errorf("expected target ID 'merged-1', got '%s'", audit.TargetID)
	}
}

func TestHistoryEntryToAuditLogBatch(t *testing.T) {
	entry := HistoryEntry{
		ActionType: ActionTypeBatch,
		ActionData: map[string]interface{}{
			"action_count": float64(5), // JSON numbers are float64
		},
	}

	audit := entry.ToAuditLog()

	if audit.TargetType != "batch" {
		t.Errorf("expected target type 'batch', got '%s'", audit.TargetType)
	}
	if audit.TargetID != "5 actions" {
		t.Errorf("expected target ID '5 actions', got '%s'", audit.TargetID)
	}
}

func TestGenerateAuditReport(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	// Create some history
	for i := 0; i < 3; i++ {
		action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
		_ = action.Execute(ctx)
		_ = store.RecordOperation(ctx, action, OperationExecute)
	}

	logs, err := GenerateAuditReport(ctx, store, HistoryFilter{})
	if err != nil {
		t.Fatalf("GenerateAuditReport failed: %v", err)
	}

	if len(logs) != 3 {
		t.Errorf("expected 3 audit logs, got %d", len(logs))
	}

	for _, log := range logs {
		if log.Result != "success" {
			t.Errorf("expected result 'success', got '%s'", log.Result)
		}
	}
}

func TestHistoryFilterByTimeRange(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	now := time.Now()

	// Create action at known time
	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = store.RecordOperation(ctx, action, OperationExecute)

	// Filter with time range
	startTime := now.Add(-1 * time.Hour)
	endTime := now.Add(1 * time.Hour)

	entries, err := store.GetHistory(ctx, HistoryFilter{
		StartTime: &startTime,
		EndTime:   &endTime,
	})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry in time range, got %d", len(entries))
	}

	// Filter outside time range
	pastStart := now.Add(-2 * time.Hour)
	pastEnd := now.Add(-1 * time.Hour)

	entries, err = store.GetHistory(ctx, HistoryFilter{
		StartTime: &pastStart,
		EndTime:   &pastEnd,
	})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries outside time range, got %d", len(entries))
	}
}

func TestHistoryFilterByOperation(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)

	_ = store.RecordOperation(ctx, action, OperationExecute)
	action.SetState(ActionStateUndone)
	_ = store.RecordOperation(ctx, action, OperationUndo)

	// Filter by undo operations
	undoOp := OperationUndo
	entries, err := store.GetHistory(ctx, HistoryFilter{Operation: &undoOp})
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 undo entry, got %d", len(entries))
	}
	if entries[0].Operation != OperationUndo {
		t.Errorf("expected operation %s, got %s", OperationUndo, entries[0].Operation)
	}
}

func TestInMemoryHistoryStoreClear(t *testing.T) {
	store := NewInMemoryHistoryStore()
	ctx := context.Background()

	action := NewAcceptAction("t", "u", "s", "c", "email", "pending")
	_ = action.Execute(ctx)
	_ = store.RecordOperation(ctx, action, OperationExecute)

	store.Clear()

	entries, _ := store.GetHistory(ctx, HistoryFilter{})
	if len(entries) != 0 {
		t.Error("expected history to be cleared")
	}
}
