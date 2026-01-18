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

// Source represents raw content from external systems.
type Source struct {
	ID          int64
	TenantID    string
	ContentText string
	ContentType *string
	Metadata    json.RawMessage
}

// SourceWithDetails includes additional fields from the sources table.
type SourceWithDetails struct {
	Source
	SourceSystem      string
	ExternalID        string
	ContentHash       string
	ContentSize       *int32
	ProcessingStatus  string
	SourceTimestamp   *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SourceRepository provides methods for fetching source content.
type SourceRepository struct {
	pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewSourceRepository creates a new source repository.
func NewSourceRepository(pool *pgxpool.Pool, logger zerolog.Logger) *SourceRepository {
	return &SourceRepository{
		pool:   pool,
		logger: logger.With().Str("component", "source_repository").Logger(),
	}
}

// GetByID fetches source content by ID.
func (r *SourceRepository) GetByID(ctx context.Context, tenantID string, id int64) (*Source, error) {
	query := `
		SELECT
			id, tenant_id, raw_content, content_type, ingestion_metadata
		FROM sources
		WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
	`

	source := &Source{}
	var contentText *string
	err := r.pool.QueryRow(ctx, query, tenantID, id).Scan(
		&source.ID,
		&source.TenantID,
		&contentText,
		&source.ContentType,
		&source.Metadata,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Error().
			Err(err).
			Int64("id", id).
			Str("tenant_id", tenantID).
			Msg("Failed to fetch source")
		return nil, fmt.Errorf("failed to fetch source: %w", err)
	}

	if contentText != nil {
		source.ContentText = *contentText
	}

	r.logger.Debug().
		Int64("id", id).
		Str("tenant_id", tenantID).
		Msg("Source fetched successfully")

	return source, nil
}

// GetByIDWithDetails fetches source content with all details.
func (r *SourceRepository) GetByIDWithDetails(ctx context.Context, tenantID string, id int64) (*SourceWithDetails, error) {
	query := `
		SELECT
			id, tenant_id, source_system, external_id,
			content_hash, raw_content, content_type, content_size,
			ingestion_metadata, processing_status, source_timestamp,
			created_at, updated_at
		FROM sources
		WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
	`

	source := &SourceWithDetails{}
	var contentText *string
	err := r.pool.QueryRow(ctx, query, tenantID, id).Scan(
		&source.ID,
		&source.TenantID,
		&source.SourceSystem,
		&source.ExternalID,
		&source.ContentHash,
		&contentText,
		&source.ContentType,
		&source.ContentSize,
		&source.Metadata,
		&source.ProcessingStatus,
		&source.SourceTimestamp,
		&source.CreatedAt,
		&source.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		r.logger.Error().
			Err(err).
			Int64("id", id).
			Str("tenant_id", tenantID).
			Msg("Failed to fetch source with details")
		return nil, fmt.Errorf("failed to fetch source with details: %w", err)
	}

	if contentText != nil {
		source.ContentText = *contentText
	}

	return source, nil
}

// GetByIDs fetches multiple sources by their IDs.
func (r *SourceRepository) GetByIDs(ctx context.Context, tenantID string, ids []int64) ([]*Source, error) {
	if len(ids) == 0 {
		return []*Source{}, nil
	}

	query := `
		SELECT
			id, tenant_id, raw_content, content_type, ingestion_metadata
		FROM sources
		WHERE tenant_id = $1 AND id = ANY($2) AND deleted_at IS NULL
		ORDER BY id
	`

	rows, err := r.pool.Query(ctx, query, tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sources: %w", err)
	}
	defer rows.Close()

	var sources []*Source
	for rows.Next() {
		source := &Source{}
		var contentText *string
		err := rows.Scan(
			&source.ID,
			&source.TenantID,
			&contentText,
			&source.ContentType,
			&source.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan source: %w", err)
		}
		if contentText != nil {
			source.ContentText = *contentText
		}
		sources = append(sources, source)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sources: %w", err)
	}

	r.logger.Debug().
		Int("requested", len(ids)).
		Int("found", len(sources)).
		Str("tenant_id", tenantID).
		Msg("Sources fetched")

	return sources, nil
}

// GetPendingProcessing fetches sources that are pending processing.
func (r *SourceRepository) GetPendingProcessing(ctx context.Context, tenantID string, limit int) ([]*SourceWithDetails, error) {
	query := `
		SELECT
			id, tenant_id, source_system, external_id,
			content_hash, raw_content, content_type, content_size,
			ingestion_metadata, processing_status, source_timestamp,
			created_at, updated_at
		FROM sources
		WHERE tenant_id = $1 AND processing_status = 'pending' AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending sources: %w", err)
	}
	defer rows.Close()

	var sources []*SourceWithDetails
	for rows.Next() {
		source := &SourceWithDetails{}
		var contentText *string
		err := rows.Scan(
			&source.ID,
			&source.TenantID,
			&source.SourceSystem,
			&source.ExternalID,
			&source.ContentHash,
			&contentText,
			&source.ContentType,
			&source.ContentSize,
			&source.Metadata,
			&source.ProcessingStatus,
			&source.SourceTimestamp,
			&source.CreatedAt,
			&source.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan source: %w", err)
		}
		if contentText != nil {
			source.ContentText = *contentText
		}
		sources = append(sources, source)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sources: %w", err)
	}

	r.logger.Debug().
		Int("count", len(sources)).
		Str("tenant_id", tenantID).
		Msg("Pending sources fetched")

	return sources, nil
}

// UpdateProcessingStatus updates the processing status of a source.
func (r *SourceRepository) UpdateProcessingStatus(ctx context.Context, tenantID string, id int64, status string) error {
	query := `
		UPDATE sources
		SET processing_status = $3, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, tenantID, id, status)
	if err != nil {
		return fmt.Errorf("failed to update processing status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found: id=%d", id)
	}

	r.logger.Debug().
		Int64("id", id).
		Str("status", status).
		Msg("Processing status updated")

	return nil
}

// GetByContentHash fetches a source by its content hash.
func (r *SourceRepository) GetByContentHash(ctx context.Context, tenantID string, contentHash string) (*Source, error) {
	query := `
		SELECT
			id, tenant_id, raw_content, content_type, ingestion_metadata
		FROM sources
		WHERE tenant_id = $1 AND content_hash = $2 AND deleted_at IS NULL
		LIMIT 1
	`

	source := &Source{}
	var contentText *string
	err := r.pool.QueryRow(ctx, query, tenantID, contentHash).Scan(
		&source.ID,
		&source.TenantID,
		&contentText,
		&source.ContentType,
		&source.Metadata,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch source by content hash: %w", err)
	}

	if contentText != nil {
		source.ContentText = *contentText
	}

	return source, nil
}

// CountByStatus returns the count of sources by processing status.
func (r *SourceRepository) CountByStatus(ctx context.Context, tenantID string) (map[string]int64, error) {
	query := `
		SELECT processing_status, COUNT(*)
		FROM sources
		WHERE tenant_id = $1 AND deleted_at IS NULL
		GROUP BY processing_status
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to count sources by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating counts: %w", err)
	}

	return counts, nil
}
