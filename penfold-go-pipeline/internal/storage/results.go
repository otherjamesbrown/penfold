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

// ProcessingResult represents output from AI processors.
type ProcessingResult struct {
	ID               int64
	TenantID         string
	JobID            string // UUID as string
	ResultType       string
	ResultData       json.RawMessage
	ConfidenceScore  *float64
	ModelName        *string
	ModelVersion     *string
	ProcessorVersion *string
	ProcessingTimeMs *int32
	QualityScore     *float64
	ValidationStatus *string
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

// ProcessingResultRepository provides methods for storing and retrieving processing results.
type ProcessingResultRepository struct {
	pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewProcessingResultRepository creates a new processing result repository.
func NewProcessingResultRepository(pool *pgxpool.Pool, logger zerolog.Logger) *ProcessingResultRepository {
	return &ProcessingResultRepository{
		pool:   pool,
		logger: logger.With().Str("component", "result_repository").Logger(),
	}
}

// Store inserts a new processing result.
func (r *ProcessingResultRepository) Store(ctx context.Context, result *ProcessingResult) error {
	query := `
		INSERT INTO processing_results (
			tenant_id, job_id, result_type, result_data,
			confidence_score, model_name, model_version,
			processor_version, processing_time_ms, quality_score,
			validation_status, expires_at, created_at
		) VALUES (
			$1, $2::uuid, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, COALESCE($12, NOW() + INTERVAL '30 days'), NOW()
		)
		RETURNING id, expires_at, created_at
	`

	var expiresAt *time.Time
	if !result.ExpiresAt.IsZero() {
		expiresAt = &result.ExpiresAt
	}

	err := r.pool.QueryRow(ctx, query,
		result.TenantID,
		result.JobID,
		result.ResultType,
		result.ResultData,
		result.ConfidenceScore,
		result.ModelName,
		result.ModelVersion,
		result.ProcessorVersion,
		result.ProcessingTimeMs,
		result.QualityScore,
		result.ValidationStatus,
		expiresAt,
	).Scan(&result.ID, &result.ExpiresAt, &result.CreatedAt)

	if err != nil {
		r.logger.Error().
			Err(err).
			Str("tenant_id", result.TenantID).
			Str("job_id", result.JobID).
			Str("result_type", result.ResultType).
			Msg("Failed to store processing result")
		return fmt.Errorf("failed to store processing result: %w", err)
	}

	r.logger.Debug().
		Int64("id", result.ID).
		Str("job_id", result.JobID).
		Str("result_type", result.ResultType).
		Msg("Processing result stored successfully")

	return nil
}

// StoreBatch inserts multiple processing results in a single transaction.
func (r *ProcessingResultRepository) StoreBatch(ctx context.Context, results []*ProcessingResult) error {
	if len(results) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, result := range results {
		query := `
			INSERT INTO processing_results (
				tenant_id, job_id, result_type, result_data,
				confidence_score, model_name, model_version,
				processor_version, processing_time_ms, quality_score,
				validation_status, expires_at, created_at
			) VALUES (
				$1, $2::uuid, $3, $4,
				$5, $6, $7,
				$8, $9, $10,
				$11, COALESCE($12, NOW() + INTERVAL '30 days'), NOW()
			)
			RETURNING id, expires_at, created_at
		`

		var expiresAt *time.Time
		if !result.ExpiresAt.IsZero() {
			expiresAt = &result.ExpiresAt
		}

		err := tx.QueryRow(ctx, query,
			result.TenantID,
			result.JobID,
			result.ResultType,
			result.ResultData,
			result.ConfidenceScore,
			result.ModelName,
			result.ModelVersion,
			result.ProcessorVersion,
			result.ProcessingTimeMs,
			result.QualityScore,
			result.ValidationStatus,
			expiresAt,
		).Scan(&result.ID, &result.ExpiresAt, &result.CreatedAt)

		if err != nil {
			return fmt.Errorf("failed to store result in batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Debug().
		Int("count", len(results)).
		Msg("Batch results stored successfully")

	return nil
}

// GetByJobID retrieves all results for a specific job.
func (r *ProcessingResultRepository) GetByJobID(ctx context.Context, tenantID string, jobID string) ([]*ProcessingResult, error) {
	query := `
		SELECT
			id, tenant_id, job_id, result_type, result_data,
			confidence_score, model_name, model_version,
			processor_version, processing_time_ms, quality_score,
			validation_status, expires_at, created_at
		FROM processing_results
		WHERE tenant_id = $1 AND job_id = $2::uuid
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, tenantID, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to query results by job ID: %w", err)
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetByResultType retrieves results of a specific type.
func (r *ProcessingResultRepository) GetByResultType(ctx context.Context, tenantID string, resultType string, limit int) ([]*ProcessingResult, error) {
	query := `
		SELECT
			id, tenant_id, job_id, result_type, result_data,
			confidence_score, model_name, model_version,
			processor_version, processing_time_ms, quality_score,
			validation_status, expires_at, created_at
		FROM processing_results
		WHERE tenant_id = $1 AND result_type = $2
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, tenantID, resultType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query results by type: %w", err)
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// GetByID retrieves a single result by ID.
func (r *ProcessingResultRepository) GetByID(ctx context.Context, tenantID string, id int64) (*ProcessingResult, error) {
	query := `
		SELECT
			id, tenant_id, job_id, result_type, result_data,
			confidence_score, model_name, model_version,
			processor_version, processing_time_ms, quality_score,
			validation_status, expires_at, created_at
		FROM processing_results
		WHERE tenant_id = $1 AND id = $2
	`

	result := &ProcessingResult{}
	err := r.pool.QueryRow(ctx, query, tenantID, id).Scan(
		&result.ID,
		&result.TenantID,
		&result.JobID,
		&result.ResultType,
		&result.ResultData,
		&result.ConfidenceScore,
		&result.ModelName,
		&result.ModelVersion,
		&result.ProcessorVersion,
		&result.ProcessingTimeMs,
		&result.QualityScore,
		&result.ValidationStatus,
		&result.ExpiresAt,
		&result.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get result by ID: %w", err)
	}

	return result, nil
}

// GetPendingValidation retrieves results awaiting validation.
func (r *ProcessingResultRepository) GetPendingValidation(ctx context.Context, tenantID string, limit int) ([]*ProcessingResult, error) {
	query := `
		SELECT
			id, tenant_id, job_id, result_type, result_data,
			confidence_score, model_name, model_version,
			processor_version, processing_time_ms, quality_score,
			validation_status, expires_at, created_at
		FROM processing_results
		WHERE tenant_id = $1 AND validation_status = 'pending'
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending validation results: %w", err)
	}
	defer rows.Close()

	return r.scanResults(rows)
}

// UpdateValidationStatus updates the validation status of a result.
func (r *ProcessingResultRepository) UpdateValidationStatus(ctx context.Context, tenantID string, id int64, status string) error {
	query := `
		UPDATE processing_results
		SET validation_status = $3
		WHERE tenant_id = $1 AND id = $2
	`

	result, err := r.pool.Exec(ctx, query, tenantID, id, status)
	if err != nil {
		return fmt.Errorf("failed to update validation status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("result not found: id=%d", id)
	}

	r.logger.Debug().
		Int64("id", id).
		Str("status", status).
		Msg("Validation status updated")

	return nil
}

// DeleteExpired removes results that have passed their expiration date.
func (r *ProcessingResultRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM processing_results WHERE expires_at < NOW()`
	result, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired results: %w", err)
	}

	count := result.RowsAffected()
	if count > 0 {
		r.logger.Info().
			Int64("deleted", count).
			Msg("Expired processing results deleted")
	}

	return count, nil
}

// scanResults scans rows into ProcessingResult structs.
func (r *ProcessingResultRepository) scanResults(rows pgx.Rows) ([]*ProcessingResult, error) {
	var results []*ProcessingResult
	for rows.Next() {
		result := &ProcessingResult{}
		err := rows.Scan(
			&result.ID,
			&result.TenantID,
			&result.JobID,
			&result.ResultType,
			&result.ResultData,
			&result.ConfidenceScore,
			&result.ModelName,
			&result.ModelVersion,
			&result.ProcessorVersion,
			&result.ProcessingTimeMs,
			&result.QualityScore,
			&result.ValidationStatus,
			&result.ExpiresAt,
			&result.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return results, nil
}

// CountByJobID returns the number of results for a job.
func (r *ProcessingResultRepository) CountByJobID(ctx context.Context, tenantID string, jobID string) (int64, error) {
	query := `SELECT COUNT(*) FROM processing_results WHERE tenant_id = $1 AND job_id = $2::uuid`
	var count int64
	err := r.pool.QueryRow(ctx, query, tenantID, jobID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count results: %w", err)
	}
	return count, nil
}

// GetAggregatedStats returns aggregate statistics for processing results.
func (r *ProcessingResultRepository) GetAggregatedStats(ctx context.Context, tenantID string, resultType string) (*ResultStats, error) {
	query := `
		SELECT
			COUNT(*) as total_count,
			AVG(confidence_score) as avg_confidence,
			AVG(quality_score) as avg_quality,
			AVG(processing_time_ms) as avg_processing_time,
			COUNT(CASE WHEN validation_status = 'approved' THEN 1 END) as approved_count,
			COUNT(CASE WHEN validation_status = 'rejected' THEN 1 END) as rejected_count,
			COUNT(CASE WHEN validation_status = 'pending' THEN 1 END) as pending_count
		FROM processing_results
		WHERE tenant_id = $1 AND result_type = $2
	`

	stats := &ResultStats{}
	err := r.pool.QueryRow(ctx, query, tenantID, resultType).Scan(
		&stats.TotalCount,
		&stats.AvgConfidence,
		&stats.AvgQuality,
		&stats.AvgProcessingTimeMs,
		&stats.ApprovedCount,
		&stats.RejectedCount,
		&stats.PendingCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregated stats: %w", err)
	}

	return stats, nil
}

// ResultStats holds aggregate statistics for processing results.
type ResultStats struct {
	TotalCount          int64
	AvgConfidence       *float64
	AvgQuality          *float64
	AvgProcessingTimeMs *float64
	ApprovedCount       int64
	RejectedCount       int64
	PendingCount        int64
}
