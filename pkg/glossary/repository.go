package glossary

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides access to glossary data.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new glossary repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create adds a new term to the glossary.
func (r *Repository) Create(ctx context.Context, input TermInput) (*Term, error) {
	expandInSearch := true
	if input.ExpandInSearch != nil {
		expandInSearch = *input.ExpandInSearch
	}

	source := "manual"
	if input.Source != "" {
		source = input.Source
	}

	contextJSON, _ := json.Marshal(input.Context)
	aliasesJSON, _ := json.Marshal(input.Aliases)

	var term Term
	err := r.db.QueryRow(ctx, `
		INSERT INTO glossary (term, expansion, definition, context, aliases, expand_in_search, source, created_by)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6, $7, $8)
		RETURNING id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by
	`,
		input.Term,
		input.Expansion,
		input.Definition,
		string(contextJSON),
		string(aliasesJSON),
		expandInSearch,
		source,
		input.CreatedBy,
	).Scan(
		&term.ID,
		&term.TenantID,
		&term.Term,
		&term.Expansion,
		&term.Definition,
		&term.Context,
		&term.Aliases,
		&term.ExpandInSearch,
		&term.Source,
		&term.CreatedAt,
		&term.UpdatedAt,
		&term.CreatedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("create glossary term: %w", err)
	}

	return &term, nil
}

// Get retrieves a term by ID.
func (r *Repository) Get(ctx context.Context, id int64) (*Term, error) {
	var term Term
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by
		FROM glossary
		WHERE id = $1
	`, id).Scan(
		&term.ID,
		&term.TenantID,
		&term.Term,
		&term.Expansion,
		&term.Definition,
		&term.Context,
		&term.Aliases,
		&term.ExpandInSearch,
		&term.Source,
		&term.CreatedAt,
		&term.UpdatedAt,
		&term.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get glossary term: %w", err)
	}
	return &term, nil
}

// GetByTerm retrieves a term by its term string (case-insensitive).
func (r *Repository) GetByTerm(ctx context.Context, termStr string) (*Term, error) {
	var term Term
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by
		FROM glossary
		WHERE LOWER(term) = LOWER($1)
	`, termStr).Scan(
		&term.ID,
		&term.TenantID,
		&term.Term,
		&term.Expansion,
		&term.Definition,
		&term.Context,
		&term.Aliases,
		&term.ExpandInSearch,
		&term.Source,
		&term.CreatedAt,
		&term.UpdatedAt,
		&term.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get glossary term by name: %w", err)
	}
	return &term, nil
}

// List retrieves terms matching the filter.
func (r *Repository) List(ctx context.Context, filter TermFilter) ([]*Term, error) {
	query := `
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by
		FROM glossary
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.Term != "" {
		query += fmt.Sprintf(" AND LOWER(term) = LOWER($%d)", argNum)
		args = append(args, filter.Term)
		argNum++
	}

	if filter.Search != "" {
		query += fmt.Sprintf(` AND to_tsvector('english', COALESCE(term, '') || ' ' || COALESCE(expansion, '') || ' ' || COALESCE(definition, '')) @@ plainto_tsquery('english', $%d)`, argNum)
		args = append(args, filter.Search)
		argNum++
	}

	if len(filter.Context) > 0 {
		// Match any context tag
		contextJSON, _ := json.Marshal(filter.Context)
		query += fmt.Sprintf(" AND context ?| $%d", argNum)
		args = append(args, string(contextJSON))
		argNum++
	}

	if filter.Source != "" {
		query += fmt.Sprintf(" AND source = $%d", argNum)
		args = append(args, filter.Source)
		argNum++
	}

	if filter.ExpandOnly {
		query += " AND expand_in_search = true"
	}

	query += " ORDER BY term ASC"

	limit := 100
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list glossary terms: %w", err)
	}
	defer rows.Close()

	var terms []*Term
	for rows.Next() {
		var term Term
		err := rows.Scan(
			&term.ID,
			&term.TenantID,
			&term.Term,
			&term.Expansion,
			&term.Definition,
			&term.Context,
			&term.Aliases,
			&term.ExpandInSearch,
			&term.Source,
			&term.CreatedAt,
			&term.UpdatedAt,
			&term.CreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan glossary term: %w", err)
		}
		terms = append(terms, &term)
	}

	return terms, nil
}

// Update modifies an existing term.
func (r *Repository) Update(ctx context.Context, id int64, input TermInput) (*Term, error) {
	contextJSON, _ := json.Marshal(input.Context)
	aliasesJSON, _ := json.Marshal(input.Aliases)

	expandInSearch := true
	if input.ExpandInSearch != nil {
		expandInSearch = *input.ExpandInSearch
	}

	var term Term
	err := r.db.QueryRow(ctx, `
		UPDATE glossary
		SET term = $2, expansion = $3, definition = $4, context = $5::jsonb, aliases = $6::jsonb, expand_in_search = $7
		WHERE id = $1
		RETURNING id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by
	`,
		id,
		input.Term,
		input.Expansion,
		input.Definition,
		string(contextJSON),
		string(aliasesJSON),
		expandInSearch,
	).Scan(
		&term.ID,
		&term.TenantID,
		&term.Term,
		&term.Expansion,
		&term.Definition,
		&term.Context,
		&term.Aliases,
		&term.ExpandInSearch,
		&term.Source,
		&term.CreatedAt,
		&term.UpdatedAt,
		&term.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update glossary term: %w", err)
	}

	return &term, nil
}

// Delete removes a term by ID.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.Exec(ctx, `DELETE FROM glossary WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete glossary term: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("term not found")
	}
	return nil
}

// DeleteByTerm removes a term by its term string.
func (r *Repository) DeleteByTerm(ctx context.Context, termStr string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM glossary WHERE LOWER(term) = LOWER($1)`, termStr)
	if err != nil {
		return fmt.Errorf("delete glossary term: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("term not found")
	}
	return nil
}

// GetAllForExpansion retrieves all terms that should be used for query expansion.
func (r *Repository) GetAllForExpansion(ctx context.Context) ([]*Term, error) {
	return r.List(ctx, TermFilter{ExpandOnly: true, Limit: 1000})
}

// wordBoundaryPattern creates a regex pattern for matching whole words.
func wordBoundaryPattern(word string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(word)
	return regexp.MustCompile(`(?i)\b` + escaped + `\b`)
}

// ExpandQuery finds all glossary terms in a query and returns expansion info.
func (r *Repository) ExpandQuery(ctx context.Context, query string) (*QueryExpansion, error) {
	terms, err := r.GetAllForExpansion(ctx)
	if err != nil {
		return nil, err
	}

	result := &QueryExpansion{
		OriginalQuery: query,
		ExpandedTerms: []ExpansionResult{},
	}

	expandedParts := []string{query}
	matchedTerms := make(map[string]bool)

	for _, term := range terms {
		// Check if term appears in query (case-insensitive, word boundary)
		pattern := wordBoundaryPattern(term.Term)
		if pattern.MatchString(query) && !matchedTerms[strings.ToLower(term.Term)] {
			matchedTerms[strings.ToLower(term.Term)] = true
			result.ExpandedTerms = append(result.ExpandedTerms, ExpansionResult{
				OriginalTerm: term.Term,
				Expansion:    term.Expansion,
				Aliases:      term.Aliases,
				Definition:   term.Definition,
			})
			expandedParts = append(expandedParts, term.Expansion)
		}

		// Also check aliases
		for _, alias := range term.Aliases {
			aliasPattern := wordBoundaryPattern(alias)
			if aliasPattern.MatchString(query) && !matchedTerms[strings.ToLower(term.Term)] {
				matchedTerms[strings.ToLower(term.Term)] = true
				result.ExpandedTerms = append(result.ExpandedTerms, ExpansionResult{
					OriginalTerm: alias,
					Expansion:    term.Expansion,
					Aliases:      term.Aliases,
					Definition:   term.Definition,
				})
				expandedParts = append(expandedParts, term.Expansion)
				break
			}
		}
	}

	// Build expanded query (original + expansions)
	if len(result.ExpandedTerms) > 0 {
		result.ExpandedQuery = strings.Join(expandedParts, " OR ")
	} else {
		result.ExpandedQuery = query
	}

	return result, nil
}

// LookupTerm finds a term by exact match or alias match.
func (r *Repository) LookupTerm(ctx context.Context, termStr string) (*Term, error) {
	// Try exact term match first
	term, err := r.GetByTerm(ctx, termStr)
	if err != nil {
		return nil, err
	}
	if term != nil {
		return term, nil
	}

	// Try alias match
	var result Term
	err = r.db.QueryRow(ctx, `
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by
		FROM glossary
		WHERE aliases @> $1::jsonb
	`, fmt.Sprintf(`["%s"]`, strings.ToLower(termStr))).Scan(
		&result.ID,
		&result.TenantID,
		&result.Term,
		&result.Expansion,
		&result.Definition,
		&result.Context,
		&result.Aliases,
		&result.ExpandInSearch,
		&result.Source,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.CreatedBy,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup glossary term by alias: %w", err)
	}
	return &result, nil
}
