package storage

import (
	"testing"
	"time"
)

func TestEmailSourceStructure(t *testing.T) {
	source := &EmailSource{
		TenantID:        "tenant-123",
		SourceSystem:    SourceSystemManualEML,
		ExternalID:      "<test@example.com>",
		ContentHash:     "abc123",
		RawContent:      "raw email content",
		ContentType:     "text/plain",
		ContentSize:     100,
		Metadata:        map[string]interface{}{"key": "value"},
		SourceTimestamp: time.Now(),
	}

	if source.TenantID != "tenant-123" {
		t.Errorf("unexpected tenant id: %s", source.TenantID)
	}
	if source.SourceSystem != SourceSystemManualEML {
		t.Errorf("unexpected source system: %s", source.SourceSystem)
	}
}

func TestIngestJobStructure(t *testing.T) {
	job := &IngestJob{
		ID:             "job-123",
		TenantID:       "tenant-123",
		Status:         IngestJobStatusPending,
		SourceTag:      "test-import",
		TotalItems:     100,
		ProcessedItems: 0,
		FailedItems:    0,
		LastFilePath:   "",
		Labels:         []string{"label1", "label2"},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if job.Status != IngestJobStatusPending {
		t.Errorf("unexpected status: %s", job.Status)
	}
	if len(job.Labels) != 2 {
		t.Errorf("unexpected labels count: %d", len(job.Labels))
	}
}

func TestIngestJobStatusValues(t *testing.T) {
	tests := []struct {
		status IngestJobStatus
		valid  bool
	}{
		{IngestJobStatusPending, true},
		{IngestJobStatusRunning, true},
		{IngestJobStatusCompleted, true},
		{IngestJobStatusFailed, true},
		{IngestJobStatusCancelled, true},
	}

	for _, tt := range tests {
		if tt.status == "" {
			t.Errorf("status should not be empty")
		}
	}
}

func TestSourceSystemConstants(t *testing.T) {
	if SourceSystemManualEML != "manual_eml" {
		t.Errorf("unexpected SourceSystemManualEML value: %s", SourceSystemManualEML)
	}
	if SourceSystemGmail != "gmail" {
		t.Errorf("unexpected SourceSystemGmail value: %s", SourceSystemGmail)
	}
}

func TestProcessingStatusConstants(t *testing.T) {
	if ProcessingStatusPending != "pending" {
		t.Errorf("unexpected ProcessingStatusPending: %s", ProcessingStatusPending)
	}
	if ProcessingStatusProcessing != "processing" {
		t.Errorf("unexpected ProcessingStatusProcessing: %s", ProcessingStatusProcessing)
	}
	if ProcessingStatusCompleted != "completed" {
		t.Errorf("unexpected ProcessingStatusCompleted: %s", ProcessingStatusCompleted)
	}
	if ProcessingStatusFailed != "failed" {
		t.Errorf("unexpected ProcessingStatusFailed: %s", ProcessingStatusFailed)
	}
}

func TestIngestErrorStructure(t *testing.T) {
	ingestErr := &IngestError{
		ID:        1,
		JobID:     "job-123",
		FilePath:  "/path/to/file.eml",
		ErrorMsg:  "parsing failed",
		CreatedAt: time.Now(),
	}

	if ingestErr.JobID != "job-123" {
		t.Errorf("unexpected job id: %s", ingestErr.JobID)
	}
	if ingestErr.FilePath != "/path/to/file.eml" {
		t.Errorf("unexpected file path: %s", ingestErr.FilePath)
	}
}

func TestCreatedSourceStructure(t *testing.T) {
	created := &CreatedSource{
		ID:        42,
		CreatedAt: time.Now(),
	}

	if created.ID != 42 {
		t.Errorf("unexpected id: %d", created.ID)
	}
	if created.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}
