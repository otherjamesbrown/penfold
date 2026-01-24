// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import "time"

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
	SourcesTotal      int64
	SourcesByStatus   []StatusCount
	EmbeddingsTotal   int64
	EmbeddingsRecent  int64
	AttachmentsTotal  int64
	AttachmentsByTier []StatusCount
	JobsTotal         int64
	JobsByStatus      []StatusCount
	RecentJobs        []JobSummary
	Timestamp         time.Time
}

// JobFilter contains filter parameters for listing jobs.
type JobFilter struct {
	Limit     int
	Offset    int
	Status    string
	SourceTag string
}
