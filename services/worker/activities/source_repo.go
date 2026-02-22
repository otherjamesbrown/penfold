package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresSourceRepository implements SourceRepository using pgxpool.
type PostgresSourceRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresSourceRepository creates a new PostgresSourceRepository.
func NewPostgresSourceRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresSourceRepository {
	return &PostgresSourceRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "source_repository")),
	}
}

// GetSource fetches source content by ID.
func (r *PostgresSourceRepository) GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
	query := `
		SELECT id, tenant_id, raw_content, content_type, content_hash, processing_status, ingestion_metadata, source_system, content_id
		FROM sources
		WHERE id = $1 AND tenant_id = $2
	`

	var source Source
	var metadataJSON []byte
	var sourceSystem string
	var contentID *string // nullable: content_id may be NULL (added via migration 021)
	err := r.pool.QueryRow(ctx, query, sourceID, tenantID).Scan(
		&source.ID,
		&source.TenantID,
		&source.ContentText,
		&source.ContentType,
		&source.ContentHash,
		&source.Status,
		&metadataJSON,
		&sourceSystem,
		&contentID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	// Store the raw source_system value so callers can use it.
	source.SourceSystem = sourceSystem

	// Store content_id if present.
	if contentID != nil {
		source.ContentID = *contentID
	}

	// Map source_system to logical content type (e.g., "manual_eml" -> "email")
	// This overrides the MIME type from content_type column
	source.ContentType = mapSourceSystemToContentType(sourceSystem)

	// Parse metadata JSON
	if len(metadataJSON) > 0 {
		var metadata map[string]interface{}
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse metadata: %w", err)
		}
		// Convert to map[string]string
		source.Metadata = make(map[string]string)
		for k, v := range metadata {
			if s, ok := v.(string); ok {
				source.Metadata[k] = s
			} else if arr, ok := v.([]interface{}); ok {
				// Handle arrays (e.g., references header)
				// Convert to JSON string representation
				if jsonBytes, err := json.Marshal(arr); err == nil {
					source.Metadata[k] = string(jsonBytes)
				}
			} else if v != nil {
				// Handle other types (numbers, booleans, etc.)
				source.Metadata[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return &source, nil
}

// UpdateSourceStatus updates the processing status of a source.
func (r *PostgresSourceRepository) UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error {
	return r.UpdateSourceStatusWithFailure(ctx, tenantID, sourceID, status, "", "")
}

// UpdateSourceStatusWithFailure updates the processing status and failure info of a source.
func (r *PostgresSourceRepository) UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
	query := `
		UPDATE sources
		SET processing_status = $1,
		    failure_category = $2,
		    failure_reason = $3,
		    updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5
	`

	// If triage metadata is provided, merge it into ingestion_metadata
	if len(triageMetadata) > 0 && triageMetadata[0] != nil {
		query = `
			UPDATE sources
			SET processing_status = $1,
			    failure_category = $2,
			    failure_reason = $3,
			    ingestion_metadata = COALESCE(ingestion_metadata, '{}'::jsonb) || $6::jsonb,
			    updated_at = NOW()
			WHERE id = $4 AND tenant_id = $5
		`
		metadataJSON, err := json.Marshal(triageMetadata[0])
		if err != nil {
			return fmt.Errorf("failed to marshal triage metadata: %w", err)
		}
		_, err = r.pool.Exec(ctx, query, status, failureCategory, failureReason, sourceID, tenantID, metadataJSON)
		return err
	}

	_, err := r.pool.Exec(ctx, query, status, failureCategory, failureReason, sourceID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update source status: %w", err)
	}

	return nil
}
