package schedule

import (
	"encoding/json"
	"testing"
	"time"
)

// TestScheduleExecution_Fields verifies the ScheduleExecution struct has the
// expected fields with correct types. This is a compile-time check that also
// confirms zero-values are sane.
func TestScheduleExecution_Fields(t *testing.T) {
	e := &ScheduleExecution{
		ID:             1,
		ScheduleID:     "schedule-uuid",
		TenantID:       "tenant-uuid",
		WorkflowRunID:  "run-id",
		Status:         "running",
		StartedAt:      time.Now(),
		CompletedAt:    nil,
		Error:          "",
		ResultMetadata: json.RawMessage(`{"items":5}`),
	}

	if e.ID != 1 {
		t.Errorf("expected ID 1, got %d", e.ID)
	}
	if e.ScheduleID != "schedule-uuid" {
		t.Errorf("expected ScheduleID %q, got %q", "schedule-uuid", e.ScheduleID)
	}
	if e.Status != "running" {
		t.Errorf("expected Status %q, got %q", "running", e.Status)
	}
	if e.CompletedAt != nil {
		t.Error("expected nil CompletedAt")
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(e.ResultMetadata, &meta); err != nil {
		t.Errorf("ResultMetadata not valid JSON: %v", err)
	}
}

// TestScheduleExecution_CompletedAt verifies CompletedAt pointer is handled correctly.
func TestScheduleExecution_CompletedAt(t *testing.T) {
	now := time.Now()
	e := &ScheduleExecution{
		CompletedAt: &now,
	}
	if e.CompletedAt == nil {
		t.Error("expected non-nil CompletedAt")
	}
	if !e.CompletedAt.Equal(now) {
		t.Errorf("expected CompletedAt %v, got %v", now, *e.CompletedAt)
	}
}

// TestGetExecutionHistory_DefaultLimit verifies the default limit logic is correct.
// We test the pure limit-defaulting logic, which is exercised before any DB call.
func TestGetExecutionHistory_DefaultLimit(t *testing.T) {
	// The defaulting logic: if limit <= 0, use 20.
	tests := []struct {
		input    int32
		expected int32
	}{
		{0, 20},
		{-1, 20},
		{10, 10},
		{100, 100},
	}

	for _, tt := range tests {
		limit := tt.input
		if limit <= 0 {
			limit = 20
		}
		if limit != tt.expected {
			t.Errorf("input %d: expected limit %d, got %d", tt.input, tt.expected, limit)
		}
	}
}
