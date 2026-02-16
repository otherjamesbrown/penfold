// Package assertions provides database operations for querying assertions across all content.
package assertions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository provides database operations for assertion queries.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new assertions repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "assertions_repository")),
	}
}

// ListFilters defines filters for listing assertions.
type ListFilters struct {
	Type         *string
	Since        *time.Time
	Until        *time.Time
	AttributedTo *string
	ProjectID    *int64
	Limit        int32
	Offset       int32
}

// AssertionResult represents an assertion with source context and attributions.
type AssertionResult struct {
	ID            int64
	AssertionType string
	Description   string
	SourceQuote   *string
	Confidence    float32
	CreatedAt     time.Time

	// Source context (only if requested)
	ContentID  *string
	SourceType *string
	Subject    *string
	SourceDate *time.Time
	FromName   *string

	// Attributions (populated separately)
	Attributions []Attribution
}

// Attribution represents a person attributed to an assertion.
type Attribution struct {
	EntityID string
	Name     string
	Role     string
}

// SummaryEntry represents aggregated assertion statistics.
type SummaryEntry struct {
	Key       string
	Count     int64
	ThisWeek  int64
	LastWeek  int64
}

// ListAssertions returns assertions matching the given filters.
func (r *Repository) ListAssertions(ctx context.Context, tenantID string, filters ListFilters) ([]*AssertionResult, int64, error) {
	// Set default limit
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Limit > 500 {
		filters.Limit = 500
	}

	// Build the query dynamically based on filters.
	query := `
		WITH filtered_assertions AS (
			SELECT DISTINCT
				a.id,
				a.assertion_type,
				a.description,
				a.source_quote,
				a.confidence,
				a.created_at,
				s.content_id,
				s.source_system,
				COALESCE(s.ingestion_metadata->>'subject', '') as subject,
				s.source_timestamp as source_date,
				COALESCE(s.ingestion_metadata->>'from_name', '') as from_name
			FROM assertions a
			JOIN sources s ON s.id = a.source_id
	`

	var whereClauses []string
	var args []any
	argIdx := 1

	// Filter by tenant (assuming sources have tenant_id)
	// Note: Adjust if assertions have direct tenant_id column
	whereClauses = append(whereClauses, fmt.Sprintf("s.tenant_id = $%d", argIdx))
	args = append(args, tenantID)
	argIdx++

	// Filter by assertion type
	if filters.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.assertion_type = $%d", argIdx))
		args = append(args, *filters.Type)
		argIdx++
	}

	// Filter by since date (content date)
	if filters.Since != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.source_timestamp >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}

	// Filter by until date (content date)
	if filters.Until != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.source_timestamp <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}

	// Filter by project
	if filters.ProjectID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.project_id = $%d", argIdx))
		args = append(args, *filters.ProjectID)
		argIdx++
	}

	// Filter by attributed person (fuzzy name match via people table)
	if filters.AttributedTo != nil {
		query += `
			JOIN assertion_attributions aa ON aa.assertion_id = a.id
			JOIN people p ON p.canonical_name = aa.entity_id
		`
		// Case-insensitive LIKE for fuzzy match
		whereClauses = append(whereClauses, fmt.Sprintf("p.canonical_name ILIKE $%d", argIdx))
		args = append(args, "%"+*filters.AttributedTo+"%")
		argIdx++
	}

	// Add WHERE clauses
	if len(whereClauses) > 0 {
		query += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}

	// Complete the CTE
	query += `
		)
		SELECT * FROM filtered_assertions
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`

	// Replace placeholders for LIMIT and OFFSET
	query = fmt.Sprintf(query, argIdx, argIdx+1)
	args = append(args, filters.Limit, filters.Offset)

	// Execute query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list assertions: %w", err)
	}
	defer rows.Close()

	var results []*AssertionResult
	for rows.Next() {
		result := &AssertionResult{}
		err := rows.Scan(
			&result.ID,
			&result.AssertionType,
			&result.Description,
			&result.SourceQuote,
			&result.Confidence,
			&result.CreatedAt,
			&result.ContentID,
			&result.SourceType,
			&result.Subject,
			&result.SourceDate,
			&result.FromName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan assertion: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating assertions: %w", err)
	}

	// Get total count (without limit/offset)
	countQuery := `
		SELECT COUNT(DISTINCT a.id)
		FROM assertions a
		JOIN sources s ON s.id = a.source_id
	`

	// Re-apply the same filters for count (except LIMIT/OFFSET)
	if filters.AttributedTo != nil {
		countQuery += `
			JOIN assertion_attributions aa ON aa.assertion_id = a.id
			JOIN people p ON p.canonical_name = aa.entity_id
		`
	}

	if len(whereClauses) > 0 {
		countQuery += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}

	// Use the same args minus LIMIT and OFFSET
	countArgs := args[:len(args)-2]

	var totalCount int64
	err = r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count assertions: %w", err)
	}

	// Load attributions for each assertion
	if len(results) > 0 {
		if err := r.loadAttributions(ctx, results); err != nil {
			return nil, 0, fmt.Errorf("failed to load attributions: %w", err)
		}
	}

	return results, totalCount, nil
}

// SearchAssertions searches assertions by keyword in description and source_quote.
func (r *Repository) SearchAssertions(ctx context.Context, tenantID string, query string, filters ListFilters) ([]*AssertionResult, int64, error) {
	// Set default limit
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Limit > 500 {
		filters.Limit = 500
	}

	// Build search query
	sqlQuery := `
		WITH filtered_assertions AS (
			SELECT DISTINCT
				a.id,
				a.assertion_type,
				a.description,
				a.source_quote,
				a.confidence,
				a.created_at,
				s.content_id,
				s.source_system,
				COALESCE(s.ingestion_metadata->>'subject', '') as subject,
				s.source_timestamp as source_date,
				COALESCE(s.ingestion_metadata->>'from_name', '') as from_name
			FROM assertions a
			JOIN sources s ON s.id = a.source_id
	`

	var whereClauses []string
	var args []any
	argIdx := 1

	// Filter by tenant
	whereClauses = append(whereClauses, fmt.Sprintf("s.tenant_id = $%d", argIdx))
	args = append(args, tenantID)
	argIdx++

	// Keyword search in description and source_quote
	whereClauses = append(whereClauses, fmt.Sprintf("(a.description ILIKE $%d OR a.source_quote ILIKE $%d)", argIdx, argIdx))
	args = append(args, "%"+query+"%")
	argIdx++

	// Filter by type
	if filters.Type != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("a.assertion_type = $%d", argIdx))
		args = append(args, *filters.Type)
		argIdx++
	}

	// Filter by since date
	if filters.Since != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.source_timestamp >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}

	// Filter by until date
	if filters.Until != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.source_timestamp <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}

	// Add WHERE clauses
	if len(whereClauses) > 0 {
		sqlQuery += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}

	// Complete the CTE
	sqlQuery += `
		)
		SELECT * FROM filtered_assertions
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`

	// Replace placeholders
	sqlQuery = fmt.Sprintf(sqlQuery, argIdx, argIdx+1)
	args = append(args, filters.Limit, filters.Offset)

	// Execute query
	rows, err := r.pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search assertions: %w", err)
	}
	defer rows.Close()

	var results []*AssertionResult
	for rows.Next() {
		result := &AssertionResult{}
		err := rows.Scan(
			&result.ID,
			&result.AssertionType,
			&result.Description,
			&result.SourceQuote,
			&result.Confidence,
			&result.CreatedAt,
			&result.ContentID,
			&result.SourceType,
			&result.Subject,
			&result.SourceDate,
			&result.FromName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan assertion: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating assertions: %w", err)
	}

	// Get total count
	countQuery := `
		SELECT COUNT(DISTINCT a.id)
		FROM assertions a
		JOIN sources s ON s.id = a.source_id
	`

	if len(whereClauses) > 0 {
		countQuery += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}

	countArgs := args[:len(args)-2]

	var totalCount int64
	err = r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count assertions: %w", err)
	}

	// Load attributions
	if len(results) > 0 {
		if err := r.loadAttributions(ctx, results); err != nil {
			return nil, 0, fmt.Errorf("failed to load attributions: %w", err)
		}
	}

	return results, totalCount, nil
}

// GetAssertionSummary returns aggregate statistics for assertions.
func (r *Repository) GetAssertionSummary(ctx context.Context, tenantID string, since, until *time.Time, groupBy string) ([]*SummaryEntry, int64, error) {
	// Validate groupBy
	if groupBy != "type" && groupBy != "project" && groupBy != "person" {
		groupBy = "type" // default
	}

	// Calculate time boundaries for this_week and last_week
	now := time.Now()
	oneWeekAgo := now.Add(-7 * 24 * time.Hour)
	twoWeeksAgo := now.Add(-14 * 24 * time.Hour)

	// Build query based on groupBy
	var query string
	var args []any
	argIdx := 1

	baseQuery := `
		FROM assertions a
		JOIN sources s ON s.id = a.source_id
		WHERE s.tenant_id = $%d
	`

	// Add date filters
	if since != nil {
		baseQuery += fmt.Sprintf(" AND s.source_timestamp >= $%d", argIdx+1)
		args = append(args, tenantID, *since)
		argIdx++
	} else {
		args = append(args, tenantID)
	}

	if until != nil {
		baseQuery += fmt.Sprintf(" AND s.source_timestamp <= $%d", argIdx+1)
		args = append(args, *until)
		argIdx++
	}

	switch groupBy {
	case "type":
		query = `
			SELECT
				a.assertion_type as key,
				COUNT(a.id) as count,
				COUNT(CASE WHEN s.source_timestamp >= $%d THEN 1 END) as this_week,
				COUNT(CASE WHEN s.source_timestamp >= $%d AND s.source_timestamp < $%d THEN 1 END) as last_week
			` + baseQuery + `
			GROUP BY a.assertion_type
			ORDER BY count DESC
		`
		query = fmt.Sprintf(query, argIdx+1, argIdx+2, argIdx+3, 1)
		args = append(args, oneWeekAgo, twoWeeksAgo, oneWeekAgo)

	case "project":
		query = `
			SELECT
				COALESCE(p.name, 'Unknown') as key,
				COUNT(a.id) as count,
				COUNT(CASE WHEN s.source_timestamp >= $%d THEN 1 END) as this_week,
				COUNT(CASE WHEN s.source_timestamp >= $%d AND s.source_timestamp < $%d THEN 1 END) as last_week
			` + baseQuery + `
				LEFT JOIN projects p ON p.id = a.project_id
			GROUP BY p.name
			ORDER BY count DESC
		`
		query = fmt.Sprintf(query, argIdx+1, argIdx+2, argIdx+3, 1)
		args = append(args, oneWeekAgo, twoWeeksAgo, oneWeekAgo)

	case "person":
		query = `
			SELECT
				aa.entity_id as key,
				COUNT(DISTINCT a.id) as count,
				COUNT(DISTINCT CASE WHEN s.source_timestamp >= $%d THEN a.id END) as this_week,
				COUNT(DISTINCT CASE WHEN s.source_timestamp >= $%d AND s.source_timestamp < $%d THEN a.id END) as last_week
			` + baseQuery + `
				JOIN assertion_attributions aa ON aa.assertion_id = a.id
			GROUP BY aa.entity_id
			ORDER BY count DESC
		`
		query = fmt.Sprintf(query, argIdx+1, argIdx+2, argIdx+3, 1)
		args = append(args, oneWeekAgo, twoWeeksAgo, oneWeekAgo)
	}

	// Execute query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get summary: %w", err)
	}
	defer rows.Close()

	var entries []*SummaryEntry
	var totalAssertions int64

	for rows.Next() {
		entry := &SummaryEntry{}
		err := rows.Scan(&entry.Key, &entry.Count, &entry.ThisWeek, &entry.LastWeek)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan summary entry: %w", err)
		}
		entries = append(entries, entry)
		totalAssertions += entry.Count
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating summary: %w", err)
	}

	return entries, totalAssertions, nil
}

// loadAttributions loads attributions for a batch of assertions.
func (r *Repository) loadAttributions(ctx context.Context, assertions []*AssertionResult) error {
	if len(assertions) == 0 {
		return nil
	}

	// Build list of assertion IDs
	assertionIDs := make([]int64, len(assertions))
	for i, a := range assertions {
		assertionIDs[i] = a.ID
	}

	// Query attributions
	query := `
		SELECT aa.assertion_id, aa.entity_id, aa.role, COALESCE(p.canonical_name, aa.entity_id) as name
		FROM assertion_attributions aa
		LEFT JOIN people p ON p.canonical_name = aa.entity_id
		WHERE aa.assertion_id = ANY($1)
		ORDER BY aa.assertion_id, aa.role
	`

	rows, err := r.pool.Query(ctx, query, assertionIDs)
	if err != nil {
		return fmt.Errorf("failed to query attributions: %w", err)
	}
	defer rows.Close()

	// Build map of assertion_id -> attributions
	attributionMap := make(map[int64][]Attribution)
	for rows.Next() {
		var assertionID int64
		var attr Attribution
		err := rows.Scan(&assertionID, &attr.EntityID, &attr.Role, &attr.Name)
		if err != nil {
			return fmt.Errorf("failed to scan attribution: %w", err)
		}
		attributionMap[assertionID] = append(attributionMap[assertionID], attr)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating attributions: %w", err)
	}

	// Assign attributions to assertions
	for _, a := range assertions {
		a.Attributions = attributionMap[a.ID]
	}

	return nil
}
