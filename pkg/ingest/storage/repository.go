// Package storage provides database operations for email ingest.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// SourceSystem identifies the origin system for ingested content.
const (
	SourceSystemManualEML = "manual_eml"
	SourceSystemGmail     = "gmail"
)

// ProcessingStatus represents the current state of source processing.
const (
	ProcessingStatusPending    = "pending"
	ProcessingStatusProcessing = "processing"
	ProcessingStatusCompleted  = "completed"
	ProcessingStatusFailed     = "failed"
)

// IngestJobStatus represents the state of an ingest batch job.
type IngestJobStatus string

const (
	IngestJobStatusPending    IngestJobStatus = "pending"
	IngestJobStatusRunning    IngestJobStatus = "running"
	IngestJobStatusCompleted  IngestJobStatus = "completed"
	IngestJobStatusFailed     IngestJobStatus = "failed"
	IngestJobStatusCancelled  IngestJobStatus = "cancelled"
)

// EmailSource represents the data needed to create a source record.
type EmailSource struct {
	TenantID      string
	SourceSystem  string
	ExternalID    string // Message-ID
	ContentHash   string
	RawContent    string
	ContentType   string
	ContentSize   int32
	Metadata      map[string]interface{}
	SourceTimestamp time.Time
}

// CreatedSource contains the result of creating a source.
type CreatedSource struct {
	ID        int64
	CreatedAt time.Time
}

// IngestJob tracks a batch ingest operation.
type IngestJob struct {
	ID           string
	TenantID     string
	Status       IngestJobStatus
	SourceTag    string
	TotalItems   int
	ProcessedItems int
	FailedItems  int
	LastFilePath string
	Labels       []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  *time.Time
}

// IngestError records an error for a specific file during ingest.
type IngestError struct {
	ID        int64
	JobID     string
	FilePath  string
	ErrorMsg  string
	CreatedAt time.Time
}

// Repository provides database operations for email ingest.
type Repository struct {
	pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewRepository creates a new ingest repository.
func NewRepository(pool *pgxpool.Pool, logger zerolog.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With().Str("component", "ingest_repository").Logger(),
	}
}

// CreateSource inserts a new source record and returns the created ID.
func (r *Repository) CreateSource(ctx context.Context, source *EmailSource) (*CreatedSource, error) {
	metadataJSON, err := json.Marshal(source.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO sources (
			tenant_id, source_system, external_id, content_hash,
			raw_content, content_type, content_size,
			ingestion_metadata, processing_status, source_timestamp,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			NOW(), NOW()
		)
		RETURNING id, created_at
	`

	var result CreatedSource
	err = r.pool.QueryRow(ctx, query,
		source.TenantID,
		source.SourceSystem,
		source.ExternalID,
		source.ContentHash,
		source.RawContent,
		source.ContentType,
		source.ContentSize,
		metadataJSON,
		ProcessingStatusPending,
		source.SourceTimestamp,
	).Scan(&result.ID, &result.CreatedAt)

	if err != nil {
		r.logger.Error().
			Err(err).
			Str("tenant_id", source.TenantID).
			Str("external_id", source.ExternalID).
			Msg("Failed to create source")
		return nil, fmt.Errorf("failed to create source: %w", err)
	}

	r.logger.Debug().
		Int64("id", result.ID).
		Str("tenant_id", source.TenantID).
		Str("external_id", source.ExternalID).
		Msg("Source created")

	return &result, nil
}

// ExistsByExternalID checks if a source with the given external ID exists.
func (r *Repository) ExistsByExternalID(ctx context.Context, tenantID, externalID string) (bool, int64, error) {
	query := `
		SELECT id FROM sources
		WHERE tenant_id = $1 AND external_id = $2 AND deleted_at IS NULL
		LIMIT 1
	`

	var id int64
	err := r.pool.QueryRow(ctx, query, tenantID, externalID).Scan(&id)

	if err == pgx.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to check existence by external_id: %w", err)
	}

	return true, id, nil
}

// ExistsByContentHash checks if a source with the given content hash exists.
func (r *Repository) ExistsByContentHash(ctx context.Context, tenantID, contentHash string) (bool, int64, error) {
	query := `
		SELECT id FROM sources
		WHERE tenant_id = $1 AND content_hash = $2 AND deleted_at IS NULL
		LIMIT 1
	`

	var id int64
	err := r.pool.QueryRow(ctx, query, tenantID, contentHash).Scan(&id)

	if err == pgx.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to check existence by content_hash: %w", err)
	}

	return true, id, nil
}

// CheckDuplicate checks if an email is a duplicate by message ID or content hash.
// Returns (isDuplicate, existingID, duplicateReason, error)
func (r *Repository) CheckDuplicate(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error) {
	// First check by message ID (exact duplicate)
	exists, id, err := r.ExistsByExternalID(ctx, tenantID, messageID)
	if err != nil {
		return false, 0, "", err
	}
	if exists {
		return true, id, "message_id", nil
	}

	// Then check by content hash (content duplicate)
	exists, id, err = r.ExistsByContentHash(ctx, tenantID, contentHash)
	if err != nil {
		return false, 0, "", err
	}
	if exists {
		return true, id, "content_hash", nil
	}

	return false, 0, "", nil
}

// CreateJob creates a new ingest job record.
func (r *Repository) CreateJob(ctx context.Context, job *IngestJob) error {
	labelsJSON, err := json.Marshal(job.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	query := `
		INSERT INTO ingest_jobs (
			id, tenant_id, status, source_tag,
			total_items, processed_items, failed_items,
			last_file_path, labels,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			NOW(), NOW()
		)
	`

	_, err = r.pool.Exec(ctx, query,
		job.ID,
		job.TenantID,
		job.Status,
		job.SourceTag,
		job.TotalItems,
		job.ProcessedItems,
		job.FailedItems,
		job.LastFilePath,
		labelsJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	r.logger.Debug().
		Str("job_id", job.ID).
		Str("tenant_id", job.TenantID).
		Msg("Ingest job created")

	return nil
}

// GetJob retrieves an ingest job by ID.
func (r *Repository) GetJob(ctx context.Context, jobID string) (*IngestJob, error) {
	query := `
		SELECT
			id, tenant_id, status, source_tag,
			total_items, processed_items, failed_items,
			last_file_path, labels,
			created_at, updated_at, completed_at
		FROM ingest_jobs
		WHERE id = $1
	`

	job := &IngestJob{}
	var labelsJSON []byte
	err := r.pool.QueryRow(ctx, query, jobID).Scan(
		&job.ID,
		&job.TenantID,
		&job.Status,
		&job.SourceTag,
		&job.TotalItems,
		&job.ProcessedItems,
		&job.FailedItems,
		&job.LastFilePath,
		&labelsJSON,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.CompletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if err := json.Unmarshal(labelsJSON, &job.Labels); err != nil {
		job.Labels = []string{}
	}

	return job, nil
}

// UpdateJobProgress updates the progress of an ingest job.
func (r *Repository) UpdateJobProgress(ctx context.Context, jobID string, processed, failed int, lastFilePath string) error {
	query := `
		UPDATE ingest_jobs
		SET processed_items = $2, failed_items = $3, last_file_path = $4, updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, jobID, processed, failed, lastFilePath)
	if err != nil {
		return fmt.Errorf("failed to update job progress: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}

	return nil
}

// CompleteJob marks an ingest job as completed.
func (r *Repository) CompleteJob(ctx context.Context, jobID string, status IngestJobStatus) error {
	query := `
		UPDATE ingest_jobs
		SET status = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, jobID, status)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("job not found: %s", jobID)
	}

	r.logger.Debug().
		Str("job_id", jobID).
		Str("status", string(status)).
		Msg("Job completed")

	return nil
}

// RecordError records an error that occurred during ingest.
func (r *Repository) RecordError(ctx context.Context, jobID, filePath, errorMsg string) error {
	query := `
		INSERT INTO ingest_errors (job_id, file_path, error_message, created_at)
		VALUES ($1, $2, $3, NOW())
	`

	_, err := r.pool.Exec(ctx, query, jobID, filePath, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to record error: %w", err)
	}

	return nil
}

// GetJobErrors retrieves all errors for a job.
func (r *Repository) GetJobErrors(ctx context.Context, jobID string) ([]*IngestError, error) {
	query := `
		SELECT id, job_id, file_path, error_message, created_at
		FROM ingest_errors
		WHERE job_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job errors: %w", err)
	}
	defer rows.Close()

	var errors []*IngestError
	for rows.Next() {
		e := &IngestError{}
		if err := rows.Scan(&e.ID, &e.JobID, &e.FilePath, &e.ErrorMsg, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan error: %w", err)
		}
		errors = append(errors, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating errors: %w", err)
	}

	return errors, nil
}

// GetRemainingFilesForJob returns files that haven't been processed yet for a resumed job.
// This requires the caller to provide the full file list and we filter out already processed files.
func (r *Repository) GetRemainingFilesForJob(ctx context.Context, jobID string, allFiles []string) ([]string, error) {
	job, err := r.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// If no files have been processed, return all
	if job.LastFilePath == "" || job.ProcessedItems == 0 {
		return allFiles, nil
	}

	// Find the index of the last processed file and return everything after it
	for i, f := range allFiles {
		if f == job.LastFilePath {
			if i+1 < len(allFiles) {
				return allFiles[i+1:], nil
			}
			return []string{}, nil
		}
	}

	// Last file not found in list, return all files
	return allFiles, nil
}
