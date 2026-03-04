package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresOperationalConfigReader reads from pipeline_operational_config using pgx.
type PostgresOperationalConfigReader struct {
	pool *pgxpool.Pool
}

// NewPostgresOperationalConfigReader creates a new operational config reader.
func NewPostgresOperationalConfigReader(pool *pgxpool.Pool) *PostgresOperationalConfigReader {
	return &PostgresOperationalConfigReader{pool: pool}
}

// GetString reads a single string value from pipeline_operational_config.
// Returns pgx.ErrNoRows if the key doesn't exist for the tenant.
func (r *PostgresOperationalConfigReader) GetString(ctx context.Context, tenantID, key string) (string, error) {
	var value string
	err := r.pool.QueryRow(ctx, `
		SELECT value FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = $2
	`, tenantID, key).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("config key %q not found for tenant %s", key, tenantID)
	}
	if err != nil {
		return "", fmt.Errorf("query operational config %q: %w", key, err)
	}
	return value, nil
}
