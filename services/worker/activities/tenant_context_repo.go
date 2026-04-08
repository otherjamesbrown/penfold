package activities

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
)

// PostgresTenantContextRepository implements TenantContextRepository using pgx.
type PostgresTenantContextRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresTenantContextRepository creates a new repository backed by the given pool.
func NewPostgresTenantContextRepository(pool *pgxpool.Pool) *PostgresTenantContextRepository {
	return &PostgresTenantContextRepository{pool: pool}
}

// GetActiveEntries returns all active tenant context entries with their conditions.
func (r *PostgresTenantContextRepository) GetActiveEntries(ctx context.Context, tenantID string) ([]TenantContextEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tc.id, tc.tenant_id, tc.category, tc.label, tc.details,
		       tc.always_inject,
		       tcc.id, tcc.field, tcc.match_type, tcc.value, tcc.case_sensitive
		FROM tenant_context tc
		LEFT JOIN tenant_context_conditions tcc ON tcc.context_id = tc.id
		WHERE tc.tenant_id = $1 AND tc.active = true
		ORDER BY tc.id ASC, tcc.id ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entryMap := make(map[int]*TenantContextEntry)
	var order []int

	for rows.Next() {
		var (
			entryID      int
			entryTenant  string
			category     string
			label        string
			detailsJSON  []byte
			alwaysInject bool
			condID       *int
			condField    *string
			condType     *string
			condValue    *string
			condCase     *bool
		)
		if err := rows.Scan(
			&entryID, &entryTenant, &category, &label, &detailsJSON,
			&alwaysInject,
			&condID, &condField, &condType, &condValue, &condCase,
		); err != nil {
			return nil, err
		}

		if _, seen := entryMap[entryID]; !seen {
			var details map[string]interface{}
			if len(detailsJSON) > 0 {
				if err := json.Unmarshal(detailsJSON, &details); err != nil {
					details = map[string]interface{}{}
				}
			}
			entryMap[entryID] = &TenantContextEntry{
				ID:           entryID,
				TenantID:     entryTenant,
				Category:     category,
				Label:        label,
				Details:      details,
				AlwaysInject: alwaysInject,
				Active:       true,
			}
			order = append(order, entryID)
		}

		if condID != nil {
			cond := classification.MatchCondition{
				ID:            *condID,
				Field:         *condField,
				MatchType:     *condType,
				Value:         *condValue,
				CaseSensitive: *condCase,
			}
			entryMap[entryID].Conditions = append(entryMap[entryID].Conditions, cond)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]TenantContextEntry, 0, len(order))
	for _, id := range order {
		results = append(results, *entryMap[id])
	}
	return results, nil
}

