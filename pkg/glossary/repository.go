package glossary

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
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
	var linkedType, linkedID interface{}

	// Handle linked entity
	if input.LinkedEntityType != "" && input.LinkedEntityID != nil {
		linkedType = input.LinkedEntityType
		linkedID = *input.LinkedEntityID
	}

	err := r.db.QueryRow(ctx, `
		INSERT INTO glossary (term, expansion, definition, context, aliases, expand_in_search, source, created_by, linked_entity_type, linked_entity_id)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6, $7, $8, $9, $10)
		RETURNING id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
	`,
		input.Term,
		input.Expansion,
		input.Definition,
		string(contextJSON),
		string(aliasesJSON),
		expandInSearch,
		source,
		input.CreatedBy,
		linkedType,
		linkedID,
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
		&term.LinkedEntityType,
		&term.LinkedEntityID,
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
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
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
		&term.LinkedEntityType,
		&term.LinkedEntityID,
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
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
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
		&term.LinkedEntityType,
		&term.LinkedEntityID,
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
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
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
		// Match any context tag using JSONB containment
		// Build conditions for each context value
		conditions := []string{}
		for _, ctx := range filter.Context {
			ctxJSON, _ := json.Marshal(ctx)
			conditions = append(conditions, fmt.Sprintf("context @> '[%s]'::jsonb", string(ctxJSON)))
		}
		query += fmt.Sprintf(" AND (%s)", strings.Join(conditions, " OR "))
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
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
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
		var definition, source, createdBy sql.NullString
		var contextRaw, aliasesRaw []byte
		err := rows.Scan(
			&term.ID,
			&term.TenantID,
			&term.Term,
			&term.Expansion,
			&definition,
			&contextRaw,
			&aliasesRaw,
			&term.ExpandInSearch,
			&source,
			&term.CreatedAt,
			&term.UpdatedAt,
			&createdBy,
			&term.LinkedEntityType,
			&term.LinkedEntityID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan glossary term: %w", err)
		}
		if definition.Valid {
			term.Definition = definition.String
		}
		if source.Valid {
			term.Source = source.String
		}
		if createdBy.Valid {
			term.CreatedBy = &createdBy.String
		}
		// Handle context - can be array, string, or null
		if len(contextRaw) > 0 {
			if contextRaw[0] == '[' {
				json.Unmarshal(contextRaw, &term.Context)
			} else if contextRaw[0] == '"' {
				var singleContext string
				json.Unmarshal(contextRaw, &singleContext)
				if singleContext != "" {
					term.Context = []string{singleContext}
				}
			}
		}
		// Handle aliases similarly
		if len(aliasesRaw) > 0 {
			if aliasesRaw[0] == '[' {
				json.Unmarshal(aliasesRaw, &term.Aliases)
			} else if aliasesRaw[0] == '"' {
				var singleAlias string
				json.Unmarshal(aliasesRaw, &singleAlias)
				if singleAlias != "" {
					term.Aliases = []string{singleAlias}
				}
			}
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

	var linkedType, linkedID interface{}
	if input.LinkedEntityType != "" && input.LinkedEntityID != nil {
		linkedType = input.LinkedEntityType
		linkedID = *input.LinkedEntityID
	}

	var term Term
	err := r.db.QueryRow(ctx, `
		UPDATE glossary
		SET term = $2, expansion = $3, definition = $4, context = $5::jsonb, aliases = $6::jsonb, expand_in_search = $7, linked_entity_type = $8, linked_entity_id = $9
		WHERE id = $1
		RETURNING id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
	`,
		id,
		input.Term,
		input.Expansion,
		input.Definition,
		string(contextJSON),
		string(aliasesJSON),
		expandInSearch,
		linkedType,
		linkedID,
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
		&term.LinkedEntityType,
		&term.LinkedEntityID,
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
		return pferrors.ErrNotFound
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
		return pferrors.ErrNotFound
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
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
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
		&result.LinkedEntityType,
		&result.LinkedEntityID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup glossary term by alias: %w", err)
	}
	return &result, nil
}

// LinkEntity links a glossary term to an entity (product, project, company).
func (r *Repository) LinkEntity(ctx context.Context, termID int64, entityType string, entityID int64) (*Term, error) {
	// Validate entity type
	validTypes := map[string]bool{"product": true, "project": true, "company": true}
	if !validTypes[entityType] {
		return nil, fmt.Errorf("%w: invalid entity type: %s (must be product, project, or company)", pferrors.ErrValidation, entityType)
	}

	var term Term
	err := r.db.QueryRow(ctx, `
		UPDATE glossary
		SET linked_entity_type = $2, linked_entity_id = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
	`, termID, entityType, entityID).Scan(
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
		&term.LinkedEntityType,
		&term.LinkedEntityID,
	)
	if err == pgx.ErrNoRows {
		return nil, pferrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("link entity: %w", err)
	}
	return &term, nil
}

// UnlinkEntity removes the entity link from a glossary term.
func (r *Repository) UnlinkEntity(ctx context.Context, termID int64) (*Term, error) {
	var term Term
	err := r.db.QueryRow(ctx, `
		UPDATE glossary
		SET linked_entity_type = NULL, linked_entity_id = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
	`, termID).Scan(
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
		&term.LinkedEntityType,
		&term.LinkedEntityID,
	)
	if err == pgx.ErrNoRows {
		return nil, pferrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("unlink entity: %w", err)
	}
	return &term, nil
}

// LinkedTermFilter specifies criteria for listing linked terms.
type LinkedTermFilter struct {
	EntityType string // Filter by entity type
	EntityID   int64  // Filter by specific entity ID
	Limit      int    // Max results (0 = default 100)
	Offset     int    // Pagination offset
}

// ListLinked retrieves terms that are linked to entities.
func (r *Repository) ListLinked(ctx context.Context, filter LinkedTermFilter) ([]*Term, int64, error) {
	query := `
		SELECT id, tenant_id, term, expansion, definition, context, aliases, expand_in_search, source, created_at, updated_at, created_by, linked_entity_type, linked_entity_id
		FROM glossary
		WHERE linked_entity_type IS NOT NULL
	`
	countQuery := `SELECT COUNT(*) FROM glossary WHERE linked_entity_type IS NOT NULL`
	args := []interface{}{}
	argNum := 1

	if filter.EntityType != "" {
		query += fmt.Sprintf(" AND linked_entity_type = $%d", argNum)
		countQuery += fmt.Sprintf(" AND linked_entity_type = $%d", argNum)
		args = append(args, filter.EntityType)
		argNum++
	}

	if filter.EntityID > 0 {
		query += fmt.Sprintf(" AND linked_entity_id = $%d", argNum)
		countQuery += fmt.Sprintf(" AND linked_entity_id = $%d", argNum)
		args = append(args, filter.EntityID)
		argNum++
	}

	// Get total count
	var totalCount int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count linked terms: %w", err)
	}

	query += " ORDER BY term ASC"

	limit := 100
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list linked terms: %w", err)
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
			&term.LinkedEntityType,
			&term.LinkedEntityID,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan linked term: %w", err)
		}
		terms = append(terms, &term)
	}

	return terms, totalCount, nil
}

// ListForContext returns glossary terms formatted for LLM prompt context.
// This is the production function that E2E tests should use.
// Note: For semantic (embedding-based) context, use the Matcher.BuildContext() method instead.
func (r *Repository) ListForContext(ctx context.Context, limit int) (string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}

	rows, err := r.db.Query(ctx, `
		SELECT term, expansion, definition
		FROM glossary
		WHERE expansion IS NOT NULL AND expansion != ''
		ORDER BY term
		LIMIT $1
	`, limit)
	if err != nil {
		return "", fmt.Errorf("list glossary: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Glossary terms:\n")

	for rows.Next() {
		var term, expansion string
		var definition *string
		if err := rows.Scan(&term, &expansion, &definition); err != nil {
			return "", fmt.Errorf("scan glossary term: %w", err)
		}

		sb.WriteString(fmt.Sprintf("- %s: %s", term, expansion))
		if definition != nil && *definition != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", *definition))
		}
		sb.WriteString("\n")
	}

	return sb.String(), rows.Err()
}
