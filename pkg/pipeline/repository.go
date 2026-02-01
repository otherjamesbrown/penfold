// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles database operations for pipeline stats and jobs.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new pipeline repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetStats retrieves overall pipeline statistics.
func (r *Repository) GetStats(ctx context.Context) (*PipelineStats, error) {
	stats := &PipelineStats{
		Timestamp: time.Now(),
	}

	// Sources total (excluding deleted)
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE is_deleted = false").Scan(&stats.SourcesTotal)
	if err != nil {
		return nil, fmt.Errorf("counting sources: %w", err)
	}

	// Sources by status (excluding deleted)
	stats.SourcesByStatus, err = r.getStatusCounts(ctx, "SELECT processing_status, COUNT(*) FROM sources WHERE is_deleted = false GROUP BY processing_status")
	if err != nil {
		return nil, fmt.Errorf("counting sources by status: %w", err)
	}

	// Embeddings total (table might not exist)
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings").Scan(&stats.EmbeddingsTotal)
	if err != nil {
		stats.EmbeddingsTotal = 0
	}

	// Embeddings recent (last hour)
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE created_at > NOW() - INTERVAL '1 hour'").Scan(&stats.EmbeddingsRecent)
	if err != nil {
		stats.EmbeddingsRecent = 0
	}

	// Attachments total
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM source_attachments").Scan(&stats.AttachmentsTotal)
	if err != nil {
		stats.AttachmentsTotal = 0
	}

	// Attachments by tier
	stats.AttachmentsByTier, _ = r.getStatusCounts(ctx, "SELECT processing_tier, COUNT(*) FROM source_attachments GROUP BY processing_tier")

	// Jobs total
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM ingest_jobs").Scan(&stats.JobsTotal)
	if err != nil {
		stats.JobsTotal = 0
	}

	// Jobs by status
	stats.JobsByStatus, _ = r.getStatusCounts(ctx, "SELECT status, COUNT(*) FROM ingest_jobs GROUP BY status")

	// Recent jobs
	stats.RecentJobs, _ = r.ListJobs(ctx, JobFilter{Limit: 5})

	return stats, nil
}

// getStatusCounts is a helper to query status/count groupings.
func (r *Repository) getStatusCounts(ctx context.Context, query string) ([]StatusCount, error) {
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []StatusCount
	for rows.Next() {
		var sc StatusCount
		if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, sc)
	}

	return counts, rows.Err()
}

// GetJob retrieves a specific job by ID.
func (r *Repository) GetJob(ctx context.Context, jobID string) (*JobDetails, *SourceStats, error) {
	query := `
		SELECT id, status, source_tag, total_files, imported_count, skipped_count, failed_count,
		       created_at, completed_at, processed_files
		FROM ingest_jobs
		WHERE id = $1
	`

	var job JobDetails
	var processedJSON []byte
	err := r.db.QueryRow(ctx, query, jobID).Scan(
		&job.ID, &job.Status, &job.SourceTag,
		&job.TotalFiles, &job.ImportedCount, &job.SkippedCount, &job.FailedCount,
		&job.CreatedAt, &job.CompletedAt, &processedJSON,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("job not found: %s", jobID)
	}

	if len(processedJSON) > 0 {
		json.Unmarshal(processedJSON, &job.ProcessedFiles)
	}

	// Get source stats for this job
	sources := &SourceStats{}

	countQuery := `
		SELECT processing_status, COUNT(*)
		FROM sources
		WHERE ingestion_metadata->>'source_tag' = $1
		   OR (source_system = 'manual_eml' AND created_at >= $2 AND created_at <= COALESCE($3, NOW()))
		GROUP BY processing_status
	`
	rows, err := r.db.Query(ctx, countQuery, job.SourceTag, job.CreatedAt, job.CompletedAt)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sc StatusCount
			if err := rows.Scan(&sc.Status, &sc.Count); err == nil {
				sources.ByStatus = append(sources.ByStatus, sc)
				sources.Total += sc.Count
			}
		}
	}

	return &job, sources, nil
}

// ListJobs lists ingest jobs with optional filtering.
func (r *Repository) ListJobs(ctx context.Context, filter JobFilter) ([]JobSummary, error) {
	if filter.Limit <= 0 {
		filter.Limit = 10
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	query := `
		SELECT id, status, source_tag, total_files, imported_count, skipped_count, failed_count,
		       created_at, completed_at
		FROM ingest_jobs
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR source_tag = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.Query(ctx, query, filter.Status, filter.SourceTag, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("querying jobs: %w", err)
	}
	defer rows.Close()

	var jobs []JobSummary
	for rows.Next() {
		var job JobSummary
		err := rows.Scan(
			&job.ID, &job.Status, &job.SourceTag,
			&job.TotalFiles, &job.ImportedCount, &job.SkippedCount, &job.FailedCount,
			&job.CreatedAt, &job.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning job: %w", err)
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// CountJobs returns the total count of jobs matching the filter.
func (r *Repository) CountJobs(ctx context.Context, filter JobFilter) (int64, error) {
	query := `
		SELECT COUNT(*)
		FROM ingest_jobs
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR source_tag = $2)
	`

	var count int64
	err := r.db.QueryRow(ctx, query, filter.Status, filter.SourceTag).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting jobs: %w", err)
	}

	return count, nil
}

// KickPendingProcessing queries pending sources from the database.
func (r *Repository) KickPendingProcessing(ctx context.Context, limit int, sourceTag string) ([]PendingSource, int, error) {
	query := `
		SELECT id, tenant_id, source_system, content_hash
		FROM sources
		WHERE processing_status = 'pending'
		  AND is_deleted = false
		  AND ($1 = '' OR ingestion_metadata->>'source_tag' = $1)
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, sourceTag, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("querying pending sources: %w", err)
	}
	defer rows.Close()

	var sources []PendingSource
	for rows.Next() {
		var s PendingSource
		if err := rows.Scan(&s.ID, &s.TenantID, &s.SourceSystem, &s.ContentHash); err != nil {
			return nil, 0, fmt.Errorf("scanning pending source: %w", err)
		}
		sources = append(sources, s)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating pending sources: %w", err)
	}

	return sources, len(sources), nil
}

// RetryFailedItems retries failed pipeline items.
// TODO: Actual implementation needed - this is a stub for compilation.
func (r *Repository) RetryFailedItems(ctx context.Context, jobID string, stage string) (int, error) {
	// Stub implementation - needs to:
	// 1. Find failed items by:
	//    - job_id (from ingest_jobs + sources)
	//    - stage (embedding vs attachment failures)
	// 2. Reset processing_status or retry flags
	// 3. Return count of retried items
	return 0, fmt.Errorf("RetryFailedItems not yet implemented")
}

// UndeleteSource restores a soft-deleted source by ID.
func (r *Repository) UndeleteSource(ctx context.Context, sourceID int64) error {
	result, err := r.db.Exec(ctx, `
		UPDATE sources
		SET is_deleted = false,
		    deleted_at = NULL,
		    deleted_by = NULL,
		    deletion_reason = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND is_deleted = true
	`, sourceID)
	if err != nil {
		return fmt.Errorf("undeleting source: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source %d not found or not deleted", sourceID)
	}

	return nil
}

// ListDeletedSources returns sources that have been soft-deleted.
func (r *Repository) ListDeletedSources(ctx context.Context, limit int) ([]DeletedSource, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, source_system, external_id, deleted_at, deleted_by, deletion_reason, processing_status
		FROM sources
		WHERE is_deleted = true
		ORDER BY deleted_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying deleted sources: %w", err)
	}
	defer rows.Close()

	var sources []DeletedSource
	for rows.Next() {
		var s DeletedSource
		if err := rows.Scan(&s.ID, &s.SourceSystem, &s.ExternalID, &s.DeletedAt, &s.DeletedBy, &s.DeletionReason, &s.ProcessingStatus); err != nil {
			return nil, fmt.Errorf("scanning deleted source: %w", err)
		}
		sources = append(sources, s)
	}

	return sources, rows.Err()
}
