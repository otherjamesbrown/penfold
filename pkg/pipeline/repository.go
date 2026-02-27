// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrBatchNotFound is returned when a batch cannot be found by ID.
var ErrBatchNotFound = errors.New("batch not found")

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

	// Sources by failure category (for failed/rejected items)
	stats.SourcesByFailureCategory, err = r.GetFailureCategoryCounts(ctx)
	if err != nil {
		// Don't fail if failure categories aren't populated yet - just log it
		stats.SourcesByFailureCategory = []StatusCount{}
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
		SELECT s.id, s.tenant_id, COALESCE(t.display_name, ''), s.source_system,
		       COALESCE(s.content_hash, ''), COALESCE(s.content_id, '')
		FROM sources s
		LEFT JOIN tenants t ON t.id = s.tenant_id::uuid AND NOT t.is_deleted
		WHERE s.processing_status = 'pending'
		  AND s.is_deleted = false
		  AND ($1 = '' OR s.ingestion_metadata->>'source_tag' = $1)
		ORDER BY s.created_at ASC
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
		if err := rows.Scan(&s.ID, &s.TenantID, &s.TenantName, &s.SourceSystem, &s.ContentHash, &s.ContentID); err != nil {
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
func (r *Repository) RetryFailedItems(ctx context.Context, jobID string, stage string) (int, error) {
	// Reset processing_status to 'pending' for failed items
	result, err := r.db.Exec(ctx, `
		UPDATE sources
		SET processing_status = 'pending',
		    updated_at = NOW()
		WHERE processing_status = 'failed'
		  AND is_deleted = false
		  AND ($1 = '' OR ingest_job_id = $1)
	`, jobID)
	if err != nil {
		return 0, fmt.Errorf("resetting failed items: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// GetSourceByContentID retrieves a pending source by content_id.
func (r *Repository) GetSourceByContentID(ctx context.Context, contentID string) (*PendingSource, error) {
	var src PendingSource
	err := r.db.QueryRow(ctx, `
		SELECT id,
		       tenant_id,
		       source_system,
		       COALESCE(content_hash, '') AS content_hash,
		       COALESCE(content_id, '') AS content_id,
		       COALESCE(processing_status, 'pending') AS processing_status
		FROM sources
		WHERE content_id = $1
		  AND is_deleted = false
	`, contentID).Scan(&src.ID, &src.TenantID, &src.SourceSystem, &src.ContentHash, &src.ContentID, &src.ProcessingStatus)
	if err != nil {
		return nil, fmt.Errorf("getting source by content_id: %w", err)
	}

	return &src, nil
}

// ResetSourceStatus resets a specific source's processing status to pending.
func (r *Repository) ResetSourceStatus(ctx context.Context, sourceID int64) error {
	result, err := r.db.Exec(ctx, `
		UPDATE sources
		SET processing_status = 'pending',
		    updated_at = NOW()
		WHERE id = $1
		  AND is_deleted = false
	`, sourceID)
	if err != nil {
		return fmt.Errorf("resetting source status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found or already deleted")
	}

	return nil
}

// ClaimReprocessSlot atomically checks the pipeline concurrency limit and claims
// a processing slot by setting the source to 'processing'. Uses an advisory lock
// to prevent TOCTOU races when multiple reprocess requests arrive simultaneously.
// Returns whether the slot was claimed, plus the current in-flight count and
// max_concurrent value for logging/error messages. (pf-a7866b)
func (r *Repository) ClaimReprocessSlot(ctx context.Context, sourceID int64, tenantID string) (claimed bool, inFlight int, maxConcurrent int, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, 0, 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Serialize concurrent reprocess claims via advisory lock.
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('pipeline_reprocess_gate'))")
	if err != nil {
		return false, 0, 0, fmt.Errorf("advisory lock: %w", err)
	}

	// Read max_concurrent from config.
	var valueStr string
	err = tx.QueryRow(ctx, `
		SELECT value FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = 'pipeline.max_concurrent'
	`, tenantID).Scan(&valueStr)
	if err == pgx.ErrNoRows {
		maxConcurrent = 1
	} else if err != nil {
		return false, 0, 0, fmt.Errorf("query max_concurrent: %w", err)
	} else {
		maxConcurrent, err = strconv.Atoi(valueStr)
		if err != nil {
			return false, 0, 0, fmt.Errorf("parse max_concurrent: %w", err)
		}
	}

	// Count in-flight sources.
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE processing_status = 'processing' AND is_deleted = false
	`).Scan(&inFlight)
	if err != nil {
		return false, 0, 0, fmt.Errorf("count in-flight: %w", err)
	}

	if inFlight >= maxConcurrent {
		return false, inFlight, maxConcurrent, nil
	}

	// Claim the slot by setting source to 'processing'.
	result, err := tx.Exec(ctx, `
		UPDATE sources SET processing_status = 'processing', updated_at = NOW()
		WHERE id = $1 AND is_deleted = false
	`, sourceID)
	if err != nil {
		return false, inFlight, maxConcurrent, fmt.Errorf("claim slot: %w", err)
	}
	if result.RowsAffected() == 0 {
		return false, inFlight, maxConcurrent, fmt.Errorf("source %d not found or deleted", sourceID)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, inFlight, maxConcurrent, fmt.Errorf("commit: %w", err)
	}

	return true, inFlight, maxConcurrent, nil
}

// DeleteRunsBySource removes all pipeline_run records for a source (pf-04a2de).
// Called during reprocess to clear stale records from previous runs,
// so the new run's results are the only ones visible in pipeline history.
func (r *Repository) DeleteRunsBySource(ctx context.Context, sourceID int64) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM pipeline_runs WHERE source_id = $1`, sourceID)
	if err != nil {
		return 0, fmt.Errorf("deleting pipeline runs for source %d: %w", sourceID, err)
	}
	return result.RowsAffected(), nil
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

// GetFailureCategoryCounts retrieves counts of sources grouped by failure category.
// Only includes sources with failed/rejected status that have a failure_category set.
func (r *Repository) GetFailureCategoryCounts(ctx context.Context) ([]StatusCount, error) {
	query := `
		SELECT failure_category, COUNT(*)
		FROM sources
		WHERE is_deleted = false
		  AND processing_status IN ('failed', 'rejected')
		  AND failure_category IS NOT NULL
		  AND failure_category != ''
		GROUP BY failure_category
		ORDER BY COUNT(*) DESC
	`
	return r.getStatusCounts(ctx, query)
}

// ListStages retrieves pipeline stage information, optionally filtered by stage.
func (r *Repository) ListStages(ctx context.Context, stageFilter string) ([]StageInfo, error) {
	query := `
		SELECT
			ps.stage,
			ps.display_name,
			ps.description,
			ps.stage_type,
			ps.model_dependent,
			ps.has_prompt,
			ps.depends_on,
			ps.downstream,
			COALESCE(pt.version, 0) as active_prompt_version
		FROM pipeline_stages ps
		LEFT JOIN prompt_templates pt ON ps.stage = pt.stage AND pt.is_active = true
		WHERE ($1 = '' OR ps.stage = $1)
		ORDER BY ps.stage
	`

	rows, err := r.db.Query(ctx, query, stageFilter)
	if err != nil {
		return nil, fmt.Errorf("querying pipeline stages: %w", err)
	}
	defer rows.Close()

	var stages []StageInfo
	for rows.Next() {
		var s StageInfo
		if err := rows.Scan(
			&s.Stage,
			&s.DisplayName,
			&s.Description,
			&s.StageType,
			&s.ModelDependent,
			&s.HasPrompt,
			&s.DependsOn,
			&s.Downstream,
			&s.ActivePromptVersion,
		); err != nil {
			return nil, fmt.Errorf("scanning stage info: %w", err)
		}
		// TODO: Add active_model_id from a config table if needed
		s.ActiveModelID = ""
		stages = append(stages, s)
	}

	return stages, rows.Err()
}

// GetPromptByStage retrieves the active prompt for a stage, or a specific version.
func (r *Repository) GetPromptByStage(ctx context.Context, stage string, version int) (*PromptTemplate, error) {
	var query string
	var args []interface{}

	if version > 0 {
		query = `
			SELECT id, stage, version, content, description, is_active, created_by, created_at
			FROM prompt_templates
			WHERE stage = $1 AND version = $2
		`
		args = []interface{}{stage, version}
	} else {
		query = `
			SELECT id, stage, version, content, description, is_active, created_by, created_at
			FROM prompt_templates
			WHERE stage = $1 AND is_active = true
		`
		args = []interface{}{stage}
	}

	var pt PromptTemplate
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&pt.ID,
		&pt.Stage,
		&pt.Version,
		&pt.Content,
		&pt.Description,
		&pt.IsActive,
		&pt.CreatedBy,
		&pt.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("prompt not found for stage %s: %w", stage, err)
	}

	return &pt, nil
}

// ListPromptVersions retrieves all prompt versions for a stage.
func (r *Repository) ListPromptVersions(ctx context.Context, stage string) ([]PromptTemplate, error) {
	query := `
		SELECT id, stage, version, content, description, is_active, created_by, created_at
		FROM prompt_templates
		WHERE stage = $1
		ORDER BY version DESC
	`

	rows, err := r.db.Query(ctx, query, stage)
	if err != nil {
		return nil, fmt.Errorf("querying prompt versions: %w", err)
	}
	defer rows.Close()

	var versions []PromptTemplate
	for rows.Next() {
		var pt PromptTemplate
		if err := rows.Scan(
			&pt.ID,
			&pt.Stage,
			&pt.Version,
			&pt.Content,
			&pt.Description,
			&pt.IsActive,
			&pt.CreatedBy,
			&pt.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning prompt version: %w", err)
		}
		versions = append(versions, pt)
	}

	return versions, rows.Err()
}

// CreatePromptVersion creates a new prompt version and activates it.
func (r *Repository) CreatePromptVersion(ctx context.Context, stage string, content string, description string, createdBy string) (*PromptTemplate, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get max version for this stage
	var maxVersion int
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM prompt_templates WHERE stage = $1`, stage).Scan(&maxVersion)
	if err != nil {
		return nil, fmt.Errorf("getting max version: %w", err)
	}

	newVersion := maxVersion + 1

	// Deactivate all existing versions for this stage
	_, err = tx.Exec(ctx, `UPDATE prompt_templates SET is_active = false WHERE stage = $1`, stage)
	if err != nil {
		return nil, fmt.Errorf("deactivating old versions: %w", err)
	}

	// Insert new version as active
	var pt PromptTemplate
	err = tx.QueryRow(ctx, `
		INSERT INTO prompt_templates (stage, version, content, description, is_active, created_by, created_at)
		VALUES ($1, $2, $3, $4, true, $5, NOW())
		RETURNING id, stage, version, content, description, is_active, created_by, created_at
	`, stage, newVersion, content, description, createdBy).Scan(
		&pt.ID,
		&pt.Stage,
		&pt.Version,
		&pt.Content,
		&pt.Description,
		&pt.IsActive,
		&pt.CreatedBy,
		&pt.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting new prompt version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &pt, nil
}

// ActivatePromptVersion activates a specific prompt version (for rollback).
func (r *Repository) ActivatePromptVersion(ctx context.Context, stage string, version int) (*PromptTemplate, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify the version exists
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM prompt_templates WHERE stage = $1 AND version = $2)`, stage, version).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("checking version exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("version %d not found for stage %s", version, stage)
	}

	// Deactivate all versions for this stage
	_, err = tx.Exec(ctx, `UPDATE prompt_templates SET is_active = false WHERE stage = $1`, stage)
	if err != nil {
		return nil, fmt.Errorf("deactivating versions: %w", err)
	}

	// Activate the target version
	_, err = tx.Exec(ctx, `UPDATE prompt_templates SET is_active = true WHERE stage = $1 AND version = $2`, stage, version)
	if err != nil {
		return nil, fmt.Errorf("activating version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	// Fetch and return the activated prompt
	return r.GetPromptByStage(ctx, stage, version)
}

// ListSourceHistory retrieves all pipeline runs for a given source.
func (r *Repository) ListSourceHistory(ctx context.Context, sourceID int64, stageFilter string) ([]PipelineRun, error) {
	query := `
		SELECT id, source_id, stage, model_id, prompt_version, config_hash, status, created_at, duration_ms,
		       input_data, output_data, parsed_data, input_tokens, output_tokens, skip_reason, langfuse_trace_id
		FROM pipeline_runs
		WHERE source_id = $1
		  AND ($2 = '' OR stage = $2)
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, sourceID, stageFilter)
	if err != nil {
		return nil, fmt.Errorf("querying pipeline runs: %w", err)
	}
	defer rows.Close()

	var runs []PipelineRun
	for rows.Next() {
		var pr PipelineRun
		if err := rows.Scan(
			&pr.ID,
			&pr.SourceID,
			&pr.Stage,
			&pr.ModelID,
			&pr.PromptVersion,
			&pr.ConfigHash,
			&pr.Status,
			&pr.CreatedAt,
			&pr.DurationMS,
			&pr.InputData,
			&pr.OutputData,
			&pr.ParsedData,
			&pr.InputTokens,
			&pr.OutputTokens,
			&pr.SkipReason,
			&pr.LangfuseTraceID,
		); err != nil {
			return nil, fmt.Errorf("scanning pipeline run: %w", err)
		}
		runs = append(runs, pr)
	}

	return runs, rows.Err()
}

// CountSourcesByStage counts sources that would be affected by reprocessing a stage.
func (r *Repository) CountSourcesByStage(ctx context.Context, stage string, sourceTag string, sourceIDs []int64) (int64, error) {
	// Build query dynamically based on filters
	query := `
		SELECT COUNT(DISTINCT s.id)
		FROM sources s
		WHERE s.is_deleted = false
		  AND s.processing_status IN ('completed', 'failed')
	`

	args := []interface{}{}
	argNum := 1

	if sourceTag != "" {
		query += fmt.Sprintf(" AND s.ingestion_metadata->>'source_tag' = $%d", argNum)
		args = append(args, sourceTag)
		argNum++
	}

	if len(sourceIDs) > 0 {
		query += fmt.Sprintf(" AND s.id = ANY($%d)", argNum)
		args = append(args, sourceIDs)
	}

	var count int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting sources: %w", err)
	}

	return count, nil
}

// GetDownstreamStages retrieves all downstream stages recursively.
func (r *Repository) GetDownstreamStages(ctx context.Context, stage string) ([]string, error) {
	// Use a recursive CTE to find all downstream stages
	query := `
		WITH RECURSIVE downstream_stages AS (
			-- Base case: get immediate downstream stages
			SELECT stage, downstream
			FROM pipeline_stages
			WHERE stage = $1

			UNION

			-- Recursive case: get downstream of downstream
			SELECT ps.stage, ps.downstream
			FROM pipeline_stages ps
			JOIN downstream_stages ds ON ps.stage = ANY(ds.downstream)
		)
		SELECT DISTINCT unnest(downstream) as stage
		FROM downstream_stages
		WHERE downstream IS NOT NULL AND array_length(downstream, 1) > 0
	`

	rows, err := r.db.Query(ctx, query, stage)
	if err != nil {
		return nil, fmt.Errorf("querying downstream stages: %w", err)
	}
	defer rows.Close()

	var stages []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scanning downstream stage: %w", err)
		}
		stages = append(stages, s)
	}

	return stages, rows.Err()
}

// PipelineRunInput contains the data needed to record a pipeline run.
type PipelineRunInput struct {
	SourceID        int64
	Stage           string          // 'triage', 'extract_ner', 'extract_semantic', 'resolve', 'analyze', 'persist', 'embed'
	ModelID         string          // 'qwen2.5-7b', 'gemini-2.0-flash', '' for code-only
	PromptVersion   int             // which prompt version, 0 for no-prompt stages
	ConfigHash      string          // hash of relevant config
	Status          string          // 'completed', 'failed'
	DurationMS      int             // how long this stage took
	InputData       json.RawMessage // optional, nil = not captured
	OutputData      json.RawMessage // optional, nil = not captured
	ParsedData      json.RawMessage // optional, nil = not captured
	InputTokens     int             // LLM input token count, 0 for code-only stages
	OutputTokens    int             // LLM output token count, 0 for code-only stages
	SkipReason      string          // e.g. "contribution_gating:LOW", empty if not skipped
	LangfuseTraceID string          // trace ID for Langfuse drill-through
}

// CreateRun records a pipeline run for provenance tracking.
func (r *Repository) CreateRun(ctx context.Context, input PipelineRunInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO pipeline_runs (source_id, stage, model_id, prompt_version, config_hash, status, duration_ms, input_data, output_data, parsed_data, input_tokens, output_tokens, skip_reason, langfuse_trace_id)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, 0), NULLIF($5, ''), $6, $7, $8, $9, $10, NULLIF($11, 0), NULLIF($12, 0), NULLIF($13, ''), NULLIF($14, ''))
	`, input.SourceID, input.Stage, input.ModelID, input.PromptVersion, input.ConfigHash, input.Status, input.DurationMS, input.InputData, input.OutputData, input.ParsedData, input.InputTokens, input.OutputTokens, input.SkipReason, input.LangfuseTraceID)
	return err
}

// CountSummaries returns the count of content items with summaries.
func (r *Repository) CountSummaries(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM content_insights
		WHERE insight_type = 'summary'
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting summaries: %w", err)
	}
	return count, nil
}

// CountExtracted returns the count of unique sources with extracted assertions.
func (r *Repository) CountExtracted(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT source_id)
		FROM assertions
		WHERE is_deleted = false
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting extracted sources: %w", err)
	}
	return count, nil
}

// GetStageIO retrieves pipeline runs with IO data for a source+stage.
func (r *Repository) GetStageIO(ctx context.Context, sourceID int64, stage string, limit int) ([]PipelineRun, error) {
	if limit <= 0 {
		limit = 3
	}

	query := `
		SELECT id, source_id, stage, model_id, prompt_version, config_hash, status,
		       created_at, duration_ms, input_data, output_data, parsed_data,
		       input_tokens, output_tokens, skip_reason, langfuse_trace_id
		FROM pipeline_runs
		WHERE source_id = $1
		  AND ($2 = '' OR stage = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, sourceID, stage, limit)
	if err != nil {
		return nil, fmt.Errorf("querying pipeline runs with IO: %w", err)
	}
	defer rows.Close()

	var runs []PipelineRun
	for rows.Next() {
		var pr PipelineRun
		if err := rows.Scan(
			&pr.ID,
			&pr.SourceID,
			&pr.Stage,
			&pr.ModelID,
			&pr.PromptVersion,
			&pr.ConfigHash,
			&pr.Status,
			&pr.CreatedAt,
			&pr.DurationMS,
			&pr.InputData,
			&pr.OutputData,
			&pr.ParsedData,
			&pr.InputTokens,
			&pr.OutputTokens,
			&pr.SkipReason,
			&pr.LangfuseTraceID,
		); err != nil {
			return nil, fmt.Errorf("scanning pipeline run: %w", err)
		}
		runs = append(runs, pr)
	}

	return runs, rows.Err()
}

// GetRunByID fetches a single pipeline run by ID.
func (r *Repository) GetRunByID(ctx context.Context, runID int64) (*PipelineRun, error) {
	query := `
		SELECT id, source_id, stage, model_id, prompt_version, config_hash, status,
		       created_at, duration_ms, input_data, output_data, parsed_data,
		       input_tokens, output_tokens, skip_reason, langfuse_trace_id
		FROM pipeline_runs
		WHERE id = $1
	`

	var pr PipelineRun
	err := r.db.QueryRow(ctx, query, runID).Scan(
		&pr.ID,
		&pr.SourceID,
		&pr.Stage,
		&pr.ModelID,
		&pr.PromptVersion,
		&pr.ConfigHash,
		&pr.Status,
		&pr.CreatedAt,
		&pr.DurationMS,
		&pr.InputData,
		&pr.OutputData,
		&pr.ParsedData,
		&pr.InputTokens,
		&pr.OutputTokens,
		&pr.SkipReason,
		&pr.LangfuseTraceID,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching pipeline run %d: %w", runID, err)
	}

	return &pr, nil
}

// PruneOldRunIO nulls out IO data for runs beyond keepN per source+stage.
func (r *Repository) PruneOldRunIO(ctx context.Context, sourceID int64, stage string, keepN int) (int64, error) {
	if keepN <= 0 {
		keepN = 3
	}

	query := `
		UPDATE pipeline_runs
		SET input_data = NULL,
		    output_data = NULL,
		    parsed_data = NULL
		WHERE id NOT IN (
			SELECT id
			FROM pipeline_runs
			WHERE source_id = $1
			  AND ($2 = '' OR stage = $2)
			ORDER BY created_at DESC
			LIMIT $3
		)
		AND source_id = $1
		AND ($2 = '' OR stage = $2)
		AND (input_data IS NOT NULL OR output_data IS NOT NULL OR parsed_data IS NOT NULL)
	`

	result, err := r.db.Exec(ctx, query, sourceID, stage, keepN)
	if err != nil {
		return 0, fmt.Errorf("pruning old run IO: %w", err)
	}

	return result.RowsAffected(), nil
}

// GetLatestRunIDs retrieves the N most recent run IDs for a given source.
// Returns IDs in descending order (newest first).
func (r *Repository) GetLatestRunIDs(ctx context.Context, sourceID int64, count int) ([]int64, error) {
	// Use default count if not specified
	if count <= 0 {
		count = 10
	}

	query := `
		SELECT id
		FROM pipeline_runs
		WHERE source_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, sourceID, count)
	if err != nil {
		return nil, fmt.Errorf("querying latest run IDs: %w", err)
	}
	defer rows.Close()

	var runIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning run ID: %w", err)
		}
		runIDs = append(runIDs, id)
	}

	return runIDs, rows.Err()
}

// DiffPipelineRuns compares parsed_data JSONB between two pipeline runs.
// If runIDA is 0, uses the second-most-recent run for the source.
// If runIDB is 0, uses the most recent run for the source.
func (r *Repository) DiffPipelineRuns(ctx context.Context, sourceID, runIDA, runIDB int64) ([]StageDiff, error) {
	// Auto-select run IDs if needed
	if runIDA == 0 || runIDB == 0 {
		latestIDs, err := r.GetLatestRunIDs(ctx, sourceID, 2)
		if err != nil {
			return nil, fmt.Errorf("fetching latest run IDs: %w", err)
		}
		if len(latestIDs) == 0 {
			return nil, fmt.Errorf("no runs found for source %d", sourceID)
		}
		if len(latestIDs) < 2 && (runIDA == 0 && runIDB == 0) {
			return nil, fmt.Errorf("need at least 2 runs for comparison, found %d", len(latestIDs))
		}

		// runIDB = 0 means latest (index 0)
		if runIDB == 0 {
			runIDB = latestIDs[0]
		}
		// runIDA = 0 means second-latest (index 1)
		if runIDA == 0 {
			if len(latestIDs) < 2 {
				return nil, fmt.Errorf("need at least 2 runs for second-latest selection, found %d", len(latestIDs))
			}
			runIDA = latestIDs[1]
		}
	}

	// Fetch both runs
	runA, err := r.GetRunByID(ctx, runIDA)
	if err != nil {
		return nil, fmt.Errorf("fetching run A (%d): %w", runIDA, err)
	}

	runB, err := r.GetRunByID(ctx, runIDB)
	if err != nil {
		return nil, fmt.Errorf("fetching run B (%d): %w", runIDB, err)
	}

	// Verify both runs belong to the same source
	if runA.SourceID != sourceID || runB.SourceID != sourceID {
		return nil, fmt.Errorf("runs do not belong to source %d", sourceID)
	}

	// Compare same run (no diffs)
	if runIDA == runIDB {
		return []StageDiff{}, nil
	}

	// Compare parsed_data for each stage
	var diffs []StageDiff

	// Parse both runs' parsed_data
	var parsedA, parsedB map[string]interface{}
	if len(runA.ParsedData) > 0 {
		if err := json.Unmarshal(runA.ParsedData, &parsedA); err != nil {
			return nil, fmt.Errorf("unmarshaling parsed_data for run A: %w", err)
		}
	}
	if len(runB.ParsedData) > 0 {
		if err := json.Unmarshal(runB.ParsedData, &parsedB); err != nil {
			return nil, fmt.Errorf("unmarshaling parsed_data for run B: %w", err)
		}
	}

	// Compare top-level keys
	diffs = append(diffs, r.compareJSONObjects(runA.Stage, parsedA, parsedB)...)

	return diffs, nil
}

// compareJSONObjects compares two JSON objects and returns diffs.
func (r *Repository) compareJSONObjects(stage string, oldData, newData map[string]interface{}) []StageDiff {
	var diffs []StageDiff

	// Track all keys from both objects
	allKeys := make(map[string]bool)
	for k := range oldData {
		allKeys[k] = true
	}
	for k := range newData {
		allKeys[k] = true
	}

	// Compare each key
	for key := range allKeys {
		oldVal, oldExists := oldData[key]
		newVal, newExists := newData[key]

		if !oldExists && newExists {
			// Added
			diffs = append(diffs, StageDiff{
				Stage:      stage,
				Field:      key,
				OldValue:   "",
				NewValue:   jsonValueToString(newVal),
				ChangeType: "added",
			})
		} else if oldExists && !newExists {
			// Removed
			diffs = append(diffs, StageDiff{
				Stage:      stage,
				Field:      key,
				OldValue:   jsonValueToString(oldVal),
				NewValue:   "",
				ChangeType: "removed",
			})
		} else if oldExists && newExists {
			// Compare values
			oldStr := jsonValueToString(oldVal)
			newStr := jsonValueToString(newVal)
			if oldStr != newStr {
				diffs = append(diffs, StageDiff{
					Stage:      stage,
					Field:      key,
					OldValue:   oldStr,
					NewValue:   newStr,
					ChangeType: "modified",
				})
			}
		}
	}

	return diffs
}

// jsonValueToString converts a JSON value to a string representation.
func jsonValueToString(val interface{}) string {
	if val == nil {
		return ""
	}
	// Marshal to JSON for consistent string representation
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Sprintf("%v", val)
	}
	return string(b)
}

// CreateBatch creates a new pipeline batch record and returns the generated UUID.
func (r *Repository) CreateBatch(ctx context.Context, batch *Batch) (string, error) {
	query := `
		INSERT INTO pipeline_batches (tenant_id, workflow_id, total_items, completed_items, failed_items, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	var id string
	err := r.db.QueryRow(ctx, query,
		batch.TenantID,
		batch.WorkflowID,
		batch.TotalItems,
		batch.CompletedItems,
		batch.FailedItems,
		batch.Status,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating batch: %w", err)
	}

	return id, nil
}

// UpdateBatchProgress updates the completed and failed item counts for a batch.
func (r *Repository) UpdateBatchProgress(ctx context.Context, batchID string, completed, failed int) error {
	result, err := r.db.Exec(ctx, `
		UPDATE pipeline_batches
		SET completed_items = $2,
		    failed_items = $3,
		    updated_at = NOW()
		WHERE id = $1
	`, batchID, completed, failed)
	if err != nil {
		return fmt.Errorf("updating batch progress for %s: %w", batchID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("updating batch progress %s: %w", batchID, ErrBatchNotFound)
	}

	return nil
}

// UpdateBatchStatus updates the status of a batch.
func (r *Repository) UpdateBatchStatus(ctx context.Context, batchID string, status string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE pipeline_batches
		SET status = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, batchID, status)
	if err != nil {
		return fmt.Errorf("updating batch status for %s: %w", batchID, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("updating batch status %s: %w", batchID, ErrBatchNotFound)
	}

	return nil
}

// GetBatch retrieves a batch by ID.
func (r *Repository) GetBatch(ctx context.Context, batchID string) (*Batch, error) {
	query := `
		SELECT id, tenant_id, workflow_id, total_items, completed_items, failed_items, status, created_at, updated_at
		FROM pipeline_batches
		WHERE id = $1
	`

	var b Batch
	err := r.db.QueryRow(ctx, query, batchID).Scan(
		&b.ID,
		&b.TenantID,
		&b.WorkflowID,
		&b.TotalItems,
		&b.CompletedItems,
		&b.FailedItems,
		&b.Status,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("getting batch %s: %w", batchID, err)
	}

	return &b, nil
}

// ListBatches lists recent batches for a tenant, ordered by created_at descending.
// limit is capped at 1000.
func (r *Repository) ListBatches(ctx context.Context, tenantID string, limit int) ([]*Batch, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT id, tenant_id, workflow_id, total_items, completed_items, failed_items, status, created_at, updated_at
		FROM pipeline_batches
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing batches for tenant %s: %w", tenantID, err)
	}
	defer rows.Close()

	var batches []*Batch
	for rows.Next() {
		var b Batch
		if err := rows.Scan(
			&b.ID,
			&b.TenantID,
			&b.WorkflowID,
			&b.TotalItems,
			&b.CompletedItems,
			&b.FailedItems,
			&b.Status,
			&b.CreatedAt,
			&b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning batch: %w", err)
		}
		batches = append(batches, &b)
	}

	return batches, rows.Err()
}

// RecordOverrides stores override parameters in pipeline_runs input_data JSONB.
// Merges the overrides under an "overrides" key without overwriting existing input_data.
func (r *Repository) RecordOverrides(ctx context.Context, runID int64, overrides map[string]string) error {
	// Convert overrides map to JSON
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("marshaling overrides: %w", err)
	}

	// Use jsonb_set to merge overrides into input_data
	query := `
		UPDATE pipeline_runs
		SET input_data = COALESCE(input_data, '{}'::jsonb) || jsonb_build_object('overrides', $2::jsonb)
		WHERE id = $1
	`

	result, err := r.db.Exec(ctx, query, runID, overridesJSON)
	if err != nil {
		return fmt.Errorf("recording overrides for run %d: %w", runID, err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("run %d not found", runID)
	}

	return nil
}
