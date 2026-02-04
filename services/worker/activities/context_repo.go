package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ContextPackageRepo implements ContextPackageRepository for Stage 3 context assembly.
type ContextPackageRepo struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewContextPackageRepo creates a new context package repository.
func NewContextPackageRepo(pool *pgxpool.Pool, logger logging.Logger) *ContextPackageRepo {
	return &ContextPackageRepo{
		pool:   pool,
		logger: logger.With(logging.F("component", "context_package_repo")),
	}
}

// GetActiveRisks returns active risks/issues for the given projects, ordered by severity.
func (r *ContextPackageRepo) GetActiveRisks(ctx context.Context, projectIDs []int64, limit int) ([]ContextAssertion, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT
			a.description,
			a.severity,
			a.source_quote,
			COALESCE(p.canonical_name, '') AS owner_name,
			COALESCE(pr.name, '') AS project_name
		FROM assertions a
		LEFT JOIN people p ON a.owner_person_id = p.id
		LEFT JOIN projects pr ON a.project_id = pr.id
		WHERE a.type IN ('risk', 'issue')
		  AND a.is_current = true
		  AND a.project_id = ANY($1)
		ORDER BY
		  CASE a.severity
		    WHEN 'critical' THEN 1
		    WHEN 'high' THEN 2
		    WHEN 'medium' THEN 3
		    ELSE 4
		  END,
		  a.created_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, projectIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get active risks: %w", err)
	}
	defer rows.Close()

	var results []ContextAssertion
	for rows.Next() {
		var ca ContextAssertion
		var sourceQuote *string
		if err := rows.Scan(
			&ca.Description,
			&ca.Severity,
			&sourceQuote,
			&ca.OwnerName,
			&ca.ProjectName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan active risk: %w", err)
		}
		if sourceQuote != nil {
			ca.SourceQuote = *sourceQuote
		}
		results = append(results, ca)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating active risks: %w", err)
	}

	return results, nil
}

// GetOpenActions returns open action items for the given projects, ordered by due date.
func (r *ContextPackageRepo) GetOpenActions(ctx context.Context, projectIDs []int64, limit int) ([]ContextAssertion, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT
			a.description,
			a.due_date,
			a.status,
			COALESCE(p.canonical_name, '') AS assignee_name
		FROM assertions a
		LEFT JOIN people p ON a.assignee_person_id = p.id
		WHERE a.type = 'action'
		  AND a.status = 'open'
		  AND a.project_id = ANY($1)
		ORDER BY
		  a.due_date ASC NULLS LAST,
		  a.created_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, projectIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get open actions: %w", err)
	}
	defer rows.Close()

	var results []ContextAssertion
	for rows.Next() {
		var ca ContextAssertion
		if err := rows.Scan(
			&ca.Description,
			&ca.DueDate,
			&ca.Status,
			&ca.AssigneeName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan open action: %w", err)
		}
		results = append(results, ca)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating open actions: %w", err)
	}

	return results, nil
}

// GetRecentDecisions returns recent decisions for the given projects.
func (r *ContextPackageRepo) GetRecentDecisions(ctx context.Context, projectIDs []int64, days int, limit int) ([]ContextAssertion, error) {
	if limit <= 0 {
		limit = 5
	}
	if days <= 0 {
		days = 60
	}

	query := `
		SELECT
			a.description,
			a.rationale,
			COALESCE(p.canonical_name, '') AS decision_maker
		FROM assertions a
		LEFT JOIN people p ON a.decision_maker_person_id = p.id
		WHERE a.type = 'decision'
		  AND a.project_id = ANY($1)
		  AND a.created_at > now() - ($2 || ' days')::interval
		ORDER BY a.created_at DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, projectIDs, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent decisions: %w", err)
	}
	defer rows.Close()

	var results []ContextAssertion
	for rows.Next() {
		var ca ContextAssertion
		var rationale *string
		if err := rows.Scan(
			&ca.Description,
			&rationale,
			&ca.DecisionMaker,
		); err != nil {
			return nil, fmt.Errorf("failed to scan recent decision: %w", err)
		}
		if rationale != nil {
			ca.Rationale = *rationale
		}
		results = append(results, ca)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating recent decisions: %w", err)
	}

	return results, nil
}

// GetProductEvents returns recent product timeline events.
func (r *ContextPackageRepo) GetProductEvents(ctx context.Context, productIDs []int64, days int, limit int) ([]ContextProductEvent, error) {
	if limit <= 0 {
		limit = 10
	}
	if days <= 0 {
		days = 90
	}

	query := `
		SELECT
			pe.title,
			pe.description,
			pe.event_type,
			pe.occurred_at
		FROM product_events pe
		WHERE pe.product_id = ANY($1)
		  AND pe.occurred_at > now() - ($2 || ' days')::interval
		ORDER BY pe.occurred_at DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, productIDs, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get product events: %w", err)
	}
	defer rows.Close()

	var results []ContextProductEvent
	for rows.Next() {
		var cpe ContextProductEvent
		var description *string
		if err := rows.Scan(
			&cpe.Title,
			&description,
			&cpe.EventType,
			&cpe.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan product event: %w", err)
		}
		if description != nil {
			cpe.Description = *description
		}
		results = append(results, cpe)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating product events: %w", err)
	}

	return results, nil
}

// GetGlossaryTerms returns glossary expansions for the given terms.
func (r *ContextPackageRepo) GetGlossaryTerms(ctx context.Context, terms []string, productIDs []int64, limit int) ([]ContextGlossaryTerm, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT
			g.term,
			g.expansion,
			g.definition
		FROM glossary g
		WHERE g.term = ANY($1)
		  OR g.linked_entity_id = ANY($2)
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, terms, productIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get glossary terms: %w", err)
	}
	defer rows.Close()

	var results []ContextGlossaryTerm
	for rows.Next() {
		var cgt ContextGlossaryTerm
		var expansion, definition *string
		if err := rows.Scan(
			&cgt.Term,
			&expansion,
			&definition,
		); err != nil {
			return nil, fmt.Errorf("failed to scan glossary term: %w", err)
		}
		if expansion != nil {
			cgt.Expansion = *expansion
		}
		if definition != nil {
			cgt.Definition = *definition
		}
		results = append(results, cgt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating glossary terms: %w", err)
	}

	return results, nil
}

// ResolveProjectByName resolves a project/product name to an ID.
// Tries exact name match first, then keyword array match.
func (r *ContextPackageRepo) ResolveProjectByName(ctx context.Context, tenantID string, name string) (*int64, error) {
	// Try exact name match first
	query := `
		SELECT id
		FROM projects
		WHERE tenant_id = $1 AND name = $2
		LIMIT 1
	`

	var projectID int64
	err := r.pool.QueryRow(ctx, query, tenantID, name).Scan(&projectID)
	if err == nil {
		return &projectID, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("failed to resolve project by name: %w", err)
	}

	// Try keyword array match
	query = `
		SELECT id
		FROM projects
		WHERE tenant_id = $1 AND $2 = ANY(keywords)
		LIMIT 1
	`

	err = r.pool.QueryRow(ctx, query, tenantID, name).Scan(&projectID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project by keyword: %w", err)
	}

	return &projectID, nil
}

// ResolveProjectByKeyword resolves a project by keyword match.
func (r *ContextPackageRepo) ResolveProjectByKeyword(ctx context.Context, tenantID string, keyword string) (*int64, error) {
	query := `
		SELECT id
		FROM projects
		WHERE tenant_id = $1 AND $2 = ANY(keywords)
		LIMIT 1
	`

	var projectID int64
	err := r.pool.QueryRow(ctx, query, tenantID, keyword).Scan(&projectID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project by keyword: %w", err)
	}

	return &projectID, nil
}
