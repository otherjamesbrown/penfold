package classification

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository loads classification rules from the database.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new classification rules repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger,
	}
}

// LoadRules loads all active rules with conditions for a tenant, ordered by priority.
func (r *Repository) LoadRules(ctx context.Context, tenantID string) ([]ClassificationRule, error) {
	const query = `
		SELECT
			cr.id,
			cr.tenant_id,
			cr.name,
			cr.priority,
			COALESCE(cr.content_type_scope, '') AS content_type_scope,
			cr.content_type,
			cr.content_subtype,
			COALESCE(cr.notification_source, '') AS notification_source,
			cr.active,
			cmc.id AS condition_id,
			cmc.field,
			cmc.match_type,
			cmc.value,
			cmc.case_sensitive
		FROM classification_rules cr
		LEFT JOIN classification_match_conditions cmc ON cmc.rule_id = cr.id
		WHERE cr.tenant_id = $1
		  AND cr.active = true
		ORDER BY cr.priority ASC, cr.id ASC, cmc.id ASC
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying classification rules: %w", err)
	}
	defer rows.Close()

	// Group conditions by rule ID
	var rules []ClassificationRule
	ruleIndex := map[int]int{} // rule ID → index in rules slice

	for rows.Next() {
		var (
			ruleID             int
			ruleTenantID       string
			ruleName           string
			rulePriority       int
			contentTypeScope   string
			contentType        string
			contentSubtype     string
			notificationSource string
			active             bool
			condID             *int
			condField          *string
			condMatchType      *string
			condValue          *string
			condCaseSensitive  *bool
		)

		if err := rows.Scan(
			&ruleID,
			&ruleTenantID,
			&ruleName,
			&rulePriority,
			&contentTypeScope,
			&contentType,
			&contentSubtype,
			&notificationSource,
			&active,
			&condID,
			&condField,
			&condMatchType,
			&condValue,
			&condCaseSensitive,
		); err != nil {
			return nil, fmt.Errorf("scanning classification rule row: %w", err)
		}

		idx, exists := ruleIndex[ruleID]
		if !exists {
			rules = append(rules, ClassificationRule{
				ID:                 ruleID,
				TenantID:           ruleTenantID,
				Name:               ruleName,
				Priority:           rulePriority,
				ContentTypeScope:   contentTypeScope,
				ContentType:        contentType,
				ContentSubtype:     contentSubtype,
				NotificationSource: notificationSource,
				Active:             active,
			})
			idx = len(rules) - 1
			ruleIndex[ruleID] = idx
		}

		// Append condition if present (LEFT JOIN may yield NULL when no conditions exist)
		if condID != nil {
			rules[idx].Conditions = append(rules[idx].Conditions, MatchCondition{
				ID:            *condID,
				Field:         *condField,
				MatchType:     *condMatchType,
				Value:         *condValue,
				CaseSensitive: *condCaseSensitive,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating classification rule rows: %w", err)
	}

	r.logger.Info("loaded classification rules",
		logging.F("tenant_id", tenantID),
		logging.F("count", len(rules)),
	)

	return rules, nil
}
