package temporal

import (
	"strings"
	"testing"
)

func TestGenerateWorkflowID(t *testing.T) {
	prefix := "test-workflow"
	id := GenerateWorkflowID(prefix)

	if !strings.HasPrefix(id, prefix+"-") {
		t.Errorf("expected ID to start with %s-, got %s", prefix, id)
	}

	// Should start with prefix and contain a UUID
	// Format: prefix-uuid (where uuid is 36 chars)
	expectedMinLen := len(prefix) + 1 + 36 // prefix + hyphen + UUID
	if len(id) != expectedMinLen {
		t.Errorf("expected ID length %d, got %d for ID: %s", expectedMinLen, len(id), id)
	}

	// Generate another and ensure uniqueness
	id2 := GenerateWorkflowID(prefix)
	if id == id2 {
		t.Error("expected different IDs for consecutive calls")
	}
}

func TestGenerateTimestampedWorkflowID(t *testing.T) {
	prefix := "batch"
	id := GenerateTimestampedWorkflowID(prefix)

	if !strings.HasPrefix(id, prefix+"-") {
		t.Errorf("expected ID to start with %s-, got %s", prefix, id)
	}

	// Format: prefix-YYYYMMDD-HHMMSS-uuid8
	parts := strings.Split(id, "-")
	if len(parts) < 4 {
		t.Errorf("expected at least 4 parts separated by -, got %d", len(parts))
	}

	// Check timestamp format (YYYYMMDD)
	if len(parts[1]) != 8 {
		t.Errorf("expected date part to be 8 chars (YYYYMMDD), got %d", len(parts[1]))
	}

	// Check timestamp format (HHMMSS)
	if len(parts[2]) != 6 {
		t.Errorf("expected time part to be 6 chars (HHMMSS), got %d", len(parts[2]))
	}

	// Check short UUID (8 chars)
	if len(parts[3]) != 8 {
		t.Errorf("expected UUID part to be 8 chars, got %d", len(parts[3]))
	}
}

func TestGenerateDeterministicWorkflowID(t *testing.T) {
	id := GenerateDeterministicWorkflowID("process", "email", "msg123")

	expected := "process-email-msg123"
	if id != expected {
		t.Errorf("expected %s, got %s", expected, id)
	}

	// Same inputs should produce same ID
	id2 := GenerateDeterministicWorkflowID("process", "email", "msg123")
	if id != id2 {
		t.Error("expected same ID for same inputs")
	}

	// Different inputs should produce different ID
	id3 := GenerateDeterministicWorkflowID("process", "email", "msg456")
	if id == id3 {
		t.Error("expected different ID for different inputs")
	}
}

func TestGenerateEmailWorkflowID(t *testing.T) {
	id := GenerateEmailWorkflowID("tenant1", "msg123")

	expected := "email-tenant1-msg123"
	if id != expected {
		t.Errorf("expected %s, got %s", expected, id)
	}

	// Should be deterministic
	id2 := GenerateEmailWorkflowID("tenant1", "msg123")
	if id != id2 {
		t.Error("expected same ID for same inputs")
	}
}

func TestGenerateIngestWorkflowID(t *testing.T) {
	id := GenerateIngestWorkflowID("tenant1", "document", "doc456")

	expected := "ingest-tenant1-document-doc456"
	if id != expected {
		t.Errorf("expected %s, got %s", expected, id)
	}

	// Should be deterministic
	id2 := GenerateIngestWorkflowID("tenant1", "document", "doc456")
	if id != id2 {
		t.Error("expected same ID for same inputs")
	}
}

func TestNewWorkflowStarter(t *testing.T) {
	// We can't easily test with a real client, but we can verify the struct creation
	starter := NewWorkflowStarter(nil, "test-queue")

	if starter == nil {
		t.Fatal("expected non-nil starter")
	}

	if starter.TaskQueue() != "test-queue" {
		t.Errorf("expected task queue test-queue, got %s", starter.TaskQueue())
	}

	if starter.Client() != nil {
		t.Error("expected nil client since we passed nil")
	}
}
