package activities

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresInstructionContentDB provides content data access from PostgreSQL.
type PostgresInstructionContentDB struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresInstructionContentDB creates a new PostgresInstructionContentDB.
func NewPostgresInstructionContentDB(pool *pgxpool.Pool, logger logging.Logger) *PostgresInstructionContentDB {
	return &PostgresInstructionContentDB{
		pool:   pool,
		logger: logger,
	}
}

// GetSourceContent returns subject, from, date, and body text for a given source.
func (r *PostgresInstructionContentDB) GetSourceContent(ctx context.Context, sourceID int64) (*SourceContentData, error) {
	query := `
		SELECT
			COALESCE(ingestion_metadata->>'subject', ''),
			COALESCE(ingestion_metadata->>'from', ''),
			COALESCE(ingestion_metadata->>'date', ''),
			COALESCE(LEFT(raw_content, 8000), '')
		FROM sources
		WHERE id = $1
	`
	var data SourceContentData
	err := r.pool.QueryRow(ctx, query, sourceID).Scan(
		&data.Subject, &data.From, &data.Date, &data.BodyText,
	)
	if err != nil {
		return nil, fmt.Errorf("get source content: %w", err)
	}

	return &data, nil
}

// GetAssertionTexts returns assertion descriptions for a given source.
func (r *PostgresInstructionContentDB) GetAssertionTexts(ctx context.Context, tenantID string, sourceID int64) ([]string, error) {
	query := `
		SELECT COALESCE(description, '')
		FROM assertions
		WHERE tenant_id = $1 AND source_id = $2
		ORDER BY id
		LIMIT 30
	`
	rows, err := r.pool.Query(ctx, query, tenantID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("get assertion texts: %w", err)
	}
	defer rows.Close()

	var texts []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, fmt.Errorf("scan assertion: %w", err)
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	return texts, rows.Err()
}

// GetMaxInstructionsConfig reads the max_instructions_per_evaluation config value.
func (r *PostgresInstructionContentDB) GetMaxInstructionsConfig(ctx context.Context, tenantID string) (int, error) {
	query := `
		SELECT value FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = 'max_instructions_per_evaluation'
	`
	var val string
	err := r.pool.QueryRow(ctx, query, tenantID).Scan(&val)
	if err != nil {
		return 0, fmt.Errorf("get max_instructions_per_evaluation: %w", err)
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("parse max_instructions_per_evaluation: %w", err)
	}
	return n, nil
}
