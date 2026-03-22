package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresNewsletterContextRepository implements NewsletterContextRepository using pgx.
type PostgresNewsletterContextRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresNewsletterContextRepository creates a new repository backed by the given pool.
func NewPostgresNewsletterContextRepository(pool *pgxpool.Pool) *PostgresNewsletterContextRepository {
	return &PostgresNewsletterContextRepository{pool: pool}
}

// GetUserContextJSON returns the JSON blob at pipeline_operational_config key "newsletter.user_context".
// Returns ("", nil) when the key is absent.
func (r *PostgresNewsletterContextRepository) GetUserContextJSON(ctx context.Context, tenantID string) (string, error) {
	var value string
	err := r.pool.QueryRow(ctx, `
		SELECT value FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = 'newsletter.user_context'
	`, tenantID).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// ListActiveProjects returns at most limit projects for the tenant, ordered by name.
func (r *PostgresNewsletterContextRepository) ListActiveProjects(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT name, COALESCE(description, '')
		FROM projects
		WHERE tenant_id = $1
		ORDER BY name
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNameDescriptions(rows)
}

// ListActiveProducts returns at most limit active products for the tenant, ordered by name.
func (r *PostgresNewsletterContextRepository) ListActiveProducts(ctx context.Context, tenantID string, limit int) ([]NewsletterNameDescription, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT name, COALESCE(description, '')
		FROM products
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY name
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNameDescriptions(rows)
}

// scanNameDescriptions scans rows of (name, description) into a slice.
func scanNameDescriptions(rows pgx.Rows) ([]NewsletterNameDescription, error) {
	var results []NewsletterNameDescription
	for rows.Next() {
		var nd NewsletterNameDescription
		if err := rows.Scan(&nd.Name, &nd.Description); err != nil {
			return nil, err
		}
		results = append(results, nd)
	}
	return results, rows.Err()
}
