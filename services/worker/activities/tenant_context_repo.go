package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresTenantContextRepository implements TenantContextRepository using pgx.
type PostgresTenantContextRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresTenantContextRepository creates a new tenant context repository.
func NewPostgresTenantContextRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresTenantContextRepository {
	return &PostgresTenantContextRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "tenant_context_repo")),
	}
}

// GetActiveEntries returns all active tenant context entries for the given tenant.
func (r *PostgresTenantContextRepository) GetActiveEntries(ctx context.Context, tenantID string) ([]TenantContextEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, category, label, details::text, always_inject, active
		FROM tenant_context
		WHERE tenant_id = $1 AND active = true
		ORDER BY category, label
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tenant context entries: %w", err)
	}
	defer rows.Close()

	var entries []TenantContextEntry
	for rows.Next() {
		var e TenantContextEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Category, &e.Label, &e.DetailsJSON, &e.AlwaysInject, &e.Active); err != nil {
			return nil, fmt.Errorf("failed to scan tenant context entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tenant context entries: %w", err)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	// Load conditions for all entries in one query.
	entryIDs := make([]int, len(entries))
	for i, e := range entries {
		entryIDs[i] = e.ID
	}

	condRows, err := r.pool.Query(ctx, `
		SELECT id, context_id, field, match_type, value, case_sensitive
		FROM tenant_context_conditions
		WHERE context_id = ANY($1)
		ORDER BY context_id, id
	`, entryIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query tenant context conditions: %w", err)
	}
	defer condRows.Close()

	// Index entries by ID for fast lookup.
	entryIdx := make(map[int]int, len(entries))
	for i, e := range entries {
		entryIdx[e.ID] = i
	}

	for condRows.Next() {
		var cond TenantContextCondition
		var contextID int
		if err := condRows.Scan(&cond.ID, &contextID, &cond.Field, &cond.MatchType, &cond.Value, &cond.CaseSensitive); err != nil {
			return nil, fmt.Errorf("failed to scan tenant context condition: %w", err)
		}
		if idx, ok := entryIdx[contextID]; ok {
			entries[idx].Conditions = append(entries[idx].Conditions, cond)
		}
	}
	if err := condRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tenant context conditions: %w", err)
	}

	return entries, nil
}
