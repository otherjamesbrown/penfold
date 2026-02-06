// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresSummaryRepository implements SummaryRepository using PostgreSQL.
type PostgresSummaryRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresSummaryRepository creates a new PostgreSQL summary repository.
func NewPostgresSummaryRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresSummaryRepository {
	return &PostgresSummaryRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "summary_repository")),
	}
}

// StoreSummary stores a generated summary for a source.
// It stores the summary in the content_insights table with insight_type='summary'.
func (r *PostgresSummaryRepository) StoreSummary(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	summary string,
	keyPoints []string,
	model string,
) (int64, error) {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("source_id", sourceID),
		logging.F("model", model),
		logging.F("summary_length", len(summary)),
		logging.F("key_points_count", len(keyPoints)),
	)

	logger.Debug("Storing summary")

	// Validate input
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	if sourceID <= 0 {
		return 0, fmt.Errorf("source_id is required")
	}
	if summary == "" {
		return 0, fmt.Errorf("summary is empty")
	}

	// Build content_id from source_id
	// Format: src-<sourceID> (e.g., src-12345)
	contentID := fmt.Sprintf("src-%d", sourceID)

	// Build data JSON for content_insights table
	data := map[string]interface{}{
		"summary":    summary,
		"key_points": keyPoints,
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal summary data", logging.Err(err))
		return 0, fmt.Errorf("failed to marshal summary data: %w", err)
	}

	// Insert into content_insights with ON CONFLICT to handle duplicates
	query := `
		INSERT INTO content_insights (
			content_id,
			insight_type,
			data,
			model_version
		) VALUES (
			$1,
			'summary',
			$2,
			$3
		)
		ON CONFLICT (content_id, insight_type) DO UPDATE SET
			data = EXCLUDED.data,
			model_version = EXCLUDED.model_version,
			extracted_at = NOW()
		RETURNING id
	`

	var insightID string
	err = r.pool.QueryRow(
		ctx,
		query,
		contentID,
		dataJSON,
		model,
	).Scan(&insightID)

	if err != nil {
		logger.Error("Failed to store summary", logging.Err(err))
		return 0, fmt.Errorf("failed to store summary: %w", err)
	}

	logger.Info("Summary stored successfully",
		logging.F("insight_id", insightID),
	)

	// Return a numeric ID (hash the string ID for compatibility)
	// Since the interface expects int64, we'll use a simple approach
	// In practice, you might want to add a numeric ID column or change the interface
	return sourceID, nil
}

// DeleteSummary deletes a summary by source ID.
// Used for saga compensation when downstream workflow stages fail.
// Summaries are stored in content_insights with content_id = "src-<sourceID>".
func (r *PostgresSummaryRepository) DeleteSummary(
	ctx context.Context,
	sourceID int64,
) error {
	logger := r.logger.With(
		logging.F("source_id", sourceID),
	)

	logger.Debug("Deleting summary by source ID")

	if sourceID <= 0 {
		return fmt.Errorf("invalid source_id: %d", sourceID)
	}

	contentID := fmt.Sprintf("src-%d", sourceID)

	query := `DELETE FROM content_insights WHERE content_id = $1 AND insight_type = 'summary'`

	result, err := r.pool.Exec(ctx, query, contentID)
	if err != nil {
		logger.Error("Failed to delete summary", logging.Err(err))
		return fmt.Errorf("failed to delete summary for source %d: %w", sourceID, err)
	}

	rowsAffected := result.RowsAffected()
	logger.Info("Summary deleted",
		logging.F("rows_affected", rowsAffected),
	)

	return nil
}

// Ensure PostgresSummaryRepository implements the SummaryRepository interface at compile time.
var _ SummaryRepository = (*PostgresSummaryRepository)(nil)
