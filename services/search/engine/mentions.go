package engine

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresMentionExpander expands search terms via mention resolution.
// It finds content IDs where mentions were resolved to entities matching the search terms.
type PostgresMentionExpander struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresMentionExpander creates a new PostgreSQL-based mention expander.
func NewPostgresMentionExpander(pool *pgxpool.Pool, logger logging.Logger) *PostgresMentionExpander {
	return &PostgresMentionExpander{
		pool:   pool,
		logger: logger.With(logging.F("component", "mention_expander")),
	}
}

// ExpandSearchTerms finds content IDs that have mentions resolved to entities matching the search terms.
// This enables searching for "LKE" to also find content where "OKEE" was mentioned and resolved to LKE.
func (e *PostgresMentionExpander) ExpandSearchTerms(ctx context.Context, tenantID string, searchTerms []string) ([]string, error) {
	if len(searchTerms) == 0 {
		return nil, nil
	}

	// Step 1: Find glossary terms that match the search terms
	// This includes direct term matches and terms linked to entities
	glossarySQL := `
		SELECT DISTINCT g.id, g.term, g.linked_entity_type, g.linked_entity_id
		FROM glossary g
		WHERE g.tenant_id = $1
		AND (
			LOWER(g.term) = ANY($2)
			OR LOWER(g.expansion) ILIKE ANY($3)
			OR EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(g.aliases) alias
				WHERE LOWER(alias) = ANY($2)
			)
		)
	`

	// Prepare search patterns
	lowerTerms := make([]string, len(searchTerms))
	likePatterns := make([]string, len(searchTerms))
	for i, term := range searchTerms {
		lowerTerms[i] = term
		likePatterns[i] = "%" + term + "%"
	}

	rows, err := e.pool.Query(ctx, glossarySQL, tenantID, lowerTerms, likePatterns)
	if err != nil {
		return nil, fmt.Errorf("querying glossary: %w", err)
	}
	defer rows.Close()

	// Collect entity IDs from glossary matches
	var entityMatches []entityMatch
	for rows.Next() {
		var (
			id               int64
			term             string
			linkedEntityType *string
			linkedEntityID   *int64
		)
		if err := rows.Scan(&id, &term, &linkedEntityType, &linkedEntityID); err != nil {
			return nil, fmt.Errorf("scanning glossary row: %w", err)
		}

		if linkedEntityType != nil && linkedEntityID != nil {
			entityMatches = append(entityMatches, entityMatch{
				entityType: *linkedEntityType,
				entityID:   *linkedEntityID,
			})
		}
	}

	e.logger.Debug("Found glossary matches",
		logging.F("search_terms", searchTerms),
		logging.F("entity_matches", len(entityMatches)),
	)

	// Step 2: Find content IDs that have mentions resolved to these entities
	if len(entityMatches) == 0 {
		// No glossary matches, try direct entity lookup via mention patterns
		return e.findContentViaPatterns(ctx, tenantID, lowerTerms)
	}

	return e.findContentViaMentions(ctx, tenantID, entityMatches)
}

type entityMatch struct {
	entityType string
	entityID   int64
}

// findContentViaMentions finds content IDs that have mentions resolved to the given entities.
func (e *PostgresMentionExpander) findContentViaMentions(ctx context.Context, tenantID string, entities []entityMatch) ([]string, error) {
	if len(entities) == 0 {
		return nil, nil
	}

	// Build query to find content with mentions resolved to these entities
	sql := `
		SELECT DISTINCT cm.content_id::text
		FROM content_mentions cm
		WHERE cm.tenant_id = $1
		AND cm.resolved_entity_id IS NOT NULL
		AND cm.resolution_source IS NOT NULL
		AND (
	`

	args := []interface{}{tenantID}
	argIdx := 2
	conditions := []string{}

	for _, em := range entities {
		conditions = append(conditions, fmt.Sprintf("(cm.entity_type = $%d AND cm.resolved_entity_id = $%d)", argIdx, argIdx+1))
		args = append(args, em.entityType, em.entityID)
		argIdx += 2
	}

	sql += conditions[0]
	for i := 1; i < len(conditions); i++ {
		sql += " OR " + conditions[i]
	}
	sql += ") LIMIT 100"

	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("querying content mentions: %w", err)
	}
	defer rows.Close()

	var contentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning content ID: %w", err)
		}
		contentIDs = append(contentIDs, id)
	}

	e.logger.Debug("Found content via mentions",
		logging.F("entity_count", len(entities)),
		logging.F("content_ids", len(contentIDs)),
	)

	return contentIDs, nil
}

// findContentViaPatterns finds content IDs via mention patterns when no glossary match exists.
// This handles cases where the search term matches historical mention text that was resolved.
func (e *PostgresMentionExpander) findContentViaPatterns(ctx context.Context, tenantID string, terms []string) ([]string, error) {
	if len(terms) == 0 {
		return nil, nil
	}

	// Find content where the mentioned text matches but was resolved to a different entity
	sql := `
		SELECT DISTINCT cm.content_id::text
		FROM content_mentions cm
		WHERE cm.tenant_id = $1
		AND cm.resolved_entity_id IS NOT NULL
		AND LOWER(cm.mentioned_text) = ANY($2)
		LIMIT 100
	`

	rows, err := e.pool.Query(ctx, sql, tenantID, terms)
	if err != nil {
		return nil, fmt.Errorf("querying mention patterns: %w", err)
	}
	defer rows.Close()

	var contentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning content ID: %w", err)
		}
		contentIDs = append(contentIDs, id)
	}

	e.logger.Debug("Found content via patterns",
		logging.F("terms", terms),
		logging.F("content_ids", len(contentIDs)),
	)

	return contentIDs, nil
}

// Verify interface compliance
var _ MentionExpander = (*PostgresMentionExpander)(nil)

// Helper function to convert int64 to string
func int64ToString(i int64) string {
	return strconv.FormatInt(i, 10)
}
