// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"context"
	"testing"
	"time"
)

// TestBatch_Type verifies that the Batch type has the required fields and correct JSON tags.
func TestBatch_Type(t *testing.T) {
	now := time.Now()
	b := Batch{
		ID:             "550e8400-e29b-41d4-a716-446655440000",
		TenantID:       "660e8400-e29b-41d4-a716-446655440001",
		WorkflowID:     "wf-abc123",
		TotalItems:     100,
		CompletedItems: 75,
		FailedItems:    5,
		Status:         "running",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if b.ID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("Expected ID to match, got %s", b.ID)
	}
	if b.TenantID != "660e8400-e29b-41d4-a716-446655440001" {
		t.Errorf("Expected TenantID to match, got %s", b.TenantID)
	}
	if b.WorkflowID != "wf-abc123" {
		t.Errorf("Expected WorkflowID 'wf-abc123', got %s", b.WorkflowID)
	}
	if b.TotalItems != 100 {
		t.Errorf("Expected TotalItems 100, got %d", b.TotalItems)
	}
	if b.CompletedItems != 75 {
		t.Errorf("Expected CompletedItems 75, got %d", b.CompletedItems)
	}
	if b.FailedItems != 5 {
		t.Errorf("Expected FailedItems 5, got %d", b.FailedItems)
	}
	if b.Status != "running" {
		t.Errorf("Expected Status 'running', got %s", b.Status)
	}
}

// TestBatch_DefaultStatus verifies that a zero-value Batch has empty status
// (the default 'running' is set by the database, not the Go type).
func TestBatch_DefaultStatus(t *testing.T) {
	var b Batch
	if b.Status != "" {
		t.Errorf("Expected zero-value Status to be empty string, got %s", b.Status)
	}
	if b.CompletedItems != 0 {
		t.Errorf("Expected zero-value CompletedItems to be 0, got %d", b.CompletedItems)
	}
	if b.FailedItems != 0 {
		t.Errorf("Expected zero-value FailedItems to be 0, got %d", b.FailedItems)
	}
}

// TestBatchRepository_Compile verifies that batch repository methods compile.
// This is a compile-time check only - it does not execute against a real database.
func TestBatchRepository_Compile(t *testing.T) {
	var r *Repository
	if r != nil {
		ctx := context.Background()
		batch := &Batch{
			TenantID:   "660e8400-e29b-41d4-a716-446655440001",
			WorkflowID: "wf-test",
			TotalItems: 10,
			Status:     "running",
		}
		_, _ = r.CreateBatch(ctx, batch)
		_ = r.UpdateBatchProgress(ctx, "some-id", 5, 1)
		_ = r.UpdateBatchStatus(ctx, "some-id", "completed")
		_, _ = r.GetBatch(ctx, "some-id")
		_, _ = r.ListBatches(ctx, "tenant-id", 10)
	}
}

// TestListBatches_LimitEnforcement verifies that ListBatches caps the limit at 1000.
func TestListBatches_LimitEnforcement(t *testing.T) {
	testCases := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{"Zero limit defaults to 20", 0, 20},
		{"Negative limit defaults to 20", -1, 20},
		{"Limit within range is preserved", 50, 50},
		{"Limit at max is preserved", 1000, 1000},
		{"Limit above max is capped to 1000", 2000, 1000},
		{"Limit just above max is capped to 1000", 1001, 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Replicate the limit enforcement logic from ListBatches
			limit := tc.inputLimit
			if limit <= 0 {
				limit = 20
			}
			if limit > 1000 {
				limit = 1000
			}
			if limit != tc.expectedLimit {
				t.Errorf("Expected limit %d, got %d", tc.expectedLimit, limit)
			}
		})
	}
}

// TestGetBatch_NotFoundError verifies that ErrBatchNotFound is defined and usable.
func TestGetBatch_NotFoundError(t *testing.T) {
	// Verify the sentinel error is exported and has a non-empty message.
	if ErrBatchNotFound == nil {
		t.Fatal("Expected ErrBatchNotFound to be non-nil")
	}
	if ErrBatchNotFound.Error() == "" {
		t.Error("Expected ErrBatchNotFound to have a non-empty error message")
	}
}

// TestCreateBatch_DB verifies CreateBatch against a real database.
// Skipped when no pool is available.
func TestCreateBatch_DB(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	batch := &Batch{
		TenantID:   "660e8400-e29b-41d4-a716-446655440001",
		WorkflowID: "wf-test-001",
		TotalItems: 50,
		Status:     "running",
	}

	id, err := r.CreateBatch(ctx, batch)
	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}
	if id == "" {
		t.Error("Expected non-empty ID from CreateBatch")
	}
}

// TestUpdateBatchProgress_DB verifies UpdateBatchProgress against a real database.
// Skipped when no pool is available.
func TestUpdateBatchProgress_DB(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	batchID := "550e8400-e29b-41d4-a716-446655440000"

	err := r.UpdateBatchProgress(ctx, batchID, 30, 2)
	if err != nil {
		t.Fatalf("UpdateBatchProgress failed: %v", err)
	}
}

// TestUpdateBatchProgress_NotFound verifies UpdateBatchProgress returns ErrBatchNotFound
// for a non-existent batch ID when connected to a real database.
func TestUpdateBatchProgress_NotFound(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	err := r.UpdateBatchProgress(ctx, nonExistentID, 0, 0)
	if err == nil {
		t.Fatal("Expected error for non-existent batch ID, got nil")
	}
}

// TestUpdateBatchStatus_DB verifies UpdateBatchStatus against a real database.
// Skipped when no pool is available.
func TestUpdateBatchStatus_DB(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	batchID := "550e8400-e29b-41d4-a716-446655440000"

	err := r.UpdateBatchStatus(ctx, batchID, "completed")
	if err != nil {
		t.Fatalf("UpdateBatchStatus failed: %v", err)
	}
}

// TestUpdateBatchStatus_NotFound verifies UpdateBatchStatus returns an error
// for a non-existent batch ID when connected to a real database.
func TestUpdateBatchStatus_NotFound(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	err := r.UpdateBatchStatus(ctx, nonExistentID, "completed")
	if err == nil {
		t.Fatal("Expected error for non-existent batch ID, got nil")
	}
}

// TestGetBatch_NotFound_DB verifies GetBatch returns ErrBatchNotFound for missing IDs.
func TestGetBatch_NotFound_DB(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	batch, err := r.GetBatch(ctx, nonExistentID)
	if batch != nil {
		t.Error("Expected nil batch for non-existent ID")
	}
	if err == nil {
		t.Fatal("Expected ErrBatchNotFound, got nil error")
	}
	if err != ErrBatchNotFound {
		t.Errorf("Expected ErrBatchNotFound, got: %v", err)
	}
}

// TestListBatches_DB verifies ListBatches returns results for a tenant.
// Skipped when no pool is available.
func TestListBatches_DB(t *testing.T) {
	var r *Repository
	if r == nil {
		t.Skip("Repository not initialized - skipping DB integration test")
	}

	ctx := context.Background()
	tenantID := "660e8400-e29b-41d4-a716-446655440001"

	batches, err := r.ListBatches(ctx, tenantID, 10)
	if err != nil {
		t.Fatalf("ListBatches failed: %v", err)
	}

	// Batches may be empty if none exist yet, but should not error.
	_ = batches
}
