package routing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository loads pipeline routing rules from the database.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new routing repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// LoadRoutes loads active routing rules for a tenant.
func (r *Repository) LoadRoutes(ctx context.Context, tenantID string) ([]Route, error) {
	const query = `
		SELECT id, tenant_id, COALESCE(content_type, ''), COALESCE(content_subtype, ''), pipeline, active
		FROM pipeline_routing
		WHERE tenant_id = $1 AND active = true
		ORDER BY id
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying pipeline routing: %w", err)
	}
	defer rows.Close()

	var routes []Route
	for rows.Next() {
		var route Route
		if err := rows.Scan(
			&route.ID,
			&route.TenantID,
			&route.ContentType,
			&route.ContentSubtype,
			&route.Pipeline,
			&route.Active,
		); err != nil {
			return nil, fmt.Errorf("scanning pipeline routing row: %w", err)
		}
		routes = append(routes, route)
	}

	return routes, rows.Err()
}
