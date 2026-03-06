// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"encoding/json"
	"time"
)

// StatusCount represents a count grouped by status.
type StatusCount struct {
	Status string
	Count  int64
}

// JobSummary represents summary information about an ingest job.
type JobSummary struct {
	ID            string
	Status        string
	SourceTag     string
	TotalFiles    int
	ImportedCount int
	SkippedCount  int
	FailedCount   int
	CreatedAt     time.Time
	CompletedAt   *time.Time
}

// JobDetails contains full job details including processed files.
type JobDetails struct {
	JobSummary
	ProcessedFiles []string
}

// SourceStats contains statistics about sources from a job.
type SourceStats struct {
	Total    int64
	ByStatus []StatusCount
}

// PipelineStats contains overall pipeline statistics.
type PipelineStats struct {
	SourcesTotal              int64
	SourcesByStatus           []StatusCount
	SourcesByFailureCategory  []StatusCount
	EmbeddingsTotal           int64
	EmbeddingsRecent          int64
	AttachmentsTotal          int64
	AttachmentsByTier         []StatusCount
	JobsTotal                 int64
	JobsByStatus              []StatusCount
	RecentJobs                []JobSummary
	Timestamp                 time.Time
}

// JobFilter contains filter parameters for listing jobs.
type JobFilter struct {
	Limit     int
	Offset    int
	Status    string
	SourceTag string
}

// PendingSource represents a source pending processing.
type PendingSource struct {
	ID               int64
	TenantID         string
	TenantName       string // Human-readable tenant name for Langfuse environment; may be empty
	SourceSystem     string
	ContentHash      string
	ContentID        string
	ProcessingStatus string // Authoritative processing status from the sources table
}

// DeletedSource represents a soft-deleted source.
type DeletedSource struct {
	ID               int64
	SourceSystem     string
	ExternalID       string
	DeletedAt        *time.Time
	DeletedBy        *string
	DeletionReason   *string
	ProcessingStatus string
}

// StageInfo contains information about a pipeline stage.
type StageInfo struct {
	Stage                string
	DisplayName          string
	Description          string
	StageType            string
	ModelDependent       bool
	HasPrompt            bool
	DependsOn            []string
	Downstream           []string
	ActivePromptVersion  int
	ActiveModelID        string
}

// PromptTemplate represents a versioned prompt template.
type PromptTemplate struct {
	ID             int64
	Stage          string
	Version        int
	Content        string
	Description    *string
	IsActive       bool
	CreatedBy      *string
	CreatedAt      time.Time
	ResponseSchema json.RawMessage `json:"response_schema,omitempty"`
}

// PipelineRun represents a single pipeline execution.
type PipelineRun struct {
	ID              int64
	SourceID        int64
	Stage           string
	ModelID         *string
	PromptVersion   *int
	ConfigHash      *string
	Status          string
	CreatedAt       time.Time
	DurationMS      *int
	InputData       json.RawMessage // JSONB snapshot of input data (prompt + context)
	OutputData      json.RawMessage // JSONB snapshot of raw output (model response)
	ParsedData      json.RawMessage // JSONB snapshot of parsed/structured output
	InputTokens     *int
	OutputTokens    *int
	SkipReason      *string
	LangfuseTraceID *string
}

// Batch represents a pipeline batch processing job.
type Batch struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	WorkflowID     string    `json:"workflow_id"`
	TotalItems     int       `json:"total_items"`
	CompletedItems int       `json:"completed_items"`
	FailedItems    int       `json:"failed_items"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// StageDiff represents a difference between two pipeline runs.
type StageDiff struct {
	Stage      string // Pipeline stage (e.g., "triage", "extract_ner")
	Field      string // Field name that changed
	OldValue   string // Previous value
	NewValue   string // New value
	ChangeType string // "added", "removed", "modified"
}
