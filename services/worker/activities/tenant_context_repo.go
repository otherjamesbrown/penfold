package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTenantContextRepository implements TenantContextRepository using pgxpool.
type PostgresTenantContextRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresTenantContextRepository creates a new PostgresTenantContextRepository.
func NewPostgresTenantContextRepository(pool *pgxpool.Pool) *PostgresTenantContextRepository {
	return &PostgresTenantContextRepository{pool: pool}
}

// GetActiveEntries returns all active tenant context entries for the given tenant,
// including their trigger conditions. Returns an empty slice for an empty tenantID.
func (r *PostgresTenantContextRepository) GetActiveEntries(ctx context.Context, tenantID string) ([]TenantContextEntry, error) {
	if tenantID == "" {
		return nil, nil
	}

	query := `
		SELECT
			tc.id, tc.tenant_id, tc.category, tc.label, tc.details, tc.always_inject,
			tcc.id, tcc.context_id, tcc.field, tcc.match_type, tcc.value, tcc.case_sensitive
		FROM tenant_context tc
		LEFT JOIN tenant_context_conditions tcc ON tcc.context_id = tc.id
		WHERE tc.tenant_id = $1 AND tc.active = true
		ORDER BY tc.id, tcc.id
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tenant context: %w", err)
	}
	defer rows.Close()

	var entries []TenantContextEntry
	currentID := -1
	currentIdx := -1

	for rows.Next() {
		var (
			entryID      int
			tenID        string
			category     string
			label        string
			detailsJSON  []byte
			alwaysInject bool
			condID       *int
			condCtxID    *int
			condField    *string
			condMatch    *string
			condValue    *string
			condCase     *bool
		)
		if err := rows.Scan(
			&entryID, &tenID, &category, &label, &detailsJSON, &alwaysInject,
			&condID, &condCtxID, &condField, &condMatch, &condValue, &condCase,
		); err != nil {
			return nil, fmt.Errorf("failed to scan tenant context row: %w", err)
		}

		if entryID != currentID {
			var details map[string]interface{}
			if len(detailsJSON) > 0 {
				if err := json.Unmarshal(detailsJSON, &details); err != nil {
					return nil, fmt.Errorf("failed to unmarshal details for entry %d: %w", entryID, err)
				}
			}
			entries = append(entries, TenantContextEntry{
				ID:           entryID,
				TenantID:     tenID,
				Category:     category,
				Label:        label,
				Details:      details,
				AlwaysInject: alwaysInject,
				Conditions:   []TenantContextCondition{},
			})
			currentID = entryID
			currentIdx = len(entries) - 1
		}

		if condID != nil {
			entries[currentIdx].Conditions = append(entries[currentIdx].Conditions, TenantContextCondition{
				ID:            *condID,
				ContextID:     *condCtxID,
				Field:         *condField,
				MatchType:     *condMatch,
				Value:         *condValue,
				CaseSensitive: *condCase,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tenant context rows: %w", err)
	}

	return entries, nil
}
