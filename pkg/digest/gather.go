// Package digest provides data gathering functions for digest generation.
package digest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ContentSummary holds a summary of attributed content for digest generation.
type ContentSummary struct {
	SourceID int64
	Subject  string
	From     string
	Date     time.Time
	Summary  string
}

// AssertionSummary holds assertion data for digest generation.
type AssertionSummary struct {
	ID            int64
	AssertionType string // "risk", "action", "decision"
	Description   string
	SourceQuote   string
	SourceID      int64
}

// InstructionMatchSummary holds instruction match data for digest generation.
type InstructionMatchSummary struct {
	InstructionID   int64
	InstructionName string
	Confidence      float64
	Explanation     string
	MatchedAt       time.Time
}

// LedgerEntrySummary holds session ledger entries for digest generation.
type LedgerEntrySummary struct {
	ID       int64
	Category string
	Content  string
	Source   string
}

// GatherAttributedContent queries sources attributed to the given project on the specified date.
// Returns an empty slice (not nil) if no results are found.
func GatherAttributedContent(ctx context.Context, pool *pgxpool.Pool, tenantID string, projectID int64, date time.Time) ([]ContentSummary, error) {
	query := `
		SELECT s.id,
		       COALESCE(s.ingestion_metadata->>'subject', ''),
		       COALESCE(s.ingestion_metadata->>'from_address', ''),
		       s.source_timestamp,
		       COALESCE(ce.extracted_data->>'summary', ce.extracted_data->'newsletter'->>'summary', '')
		FROM sources s
		LEFT JOIN content_enrichment ce ON ce.source_id = s.id AND ce.tenant_id = s.tenant_id::text
		WHERE s.tenant_id = $1::uuid
		  AND $2 = ANY(s.attributed_project_ids)
		  AND s.source_timestamp >= $3::date
		  AND s.source_timestamp < ($3::date + interval '1 day')
		ORDER BY s.source_timestamp
	`

	rows, err := pool.Query(ctx, query, tenantID, projectID, date)
	if err != nil {
		return nil, fmt.Errorf("gather attributed content: %w", err)
	}
	defer rows.Close()

	result := []ContentSummary{}
	for rows.Next() {
		var cs ContentSummary
		if err := rows.Scan(&cs.SourceID, &cs.Subject, &cs.From, &cs.Date, &cs.Summary); err != nil {
			return nil, fmt.Errorf("scan attributed content: %w", err)
		}
		result = append(result, cs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather attributed content rows: %w", err)
	}

	return result, nil
}

// GatherAssertions queries assertions extracted from sources attributed to the given project on the specified date.
// Returns an empty slice (not nil) if no results are found.
func GatherAssertions(ctx context.Context, pool *pgxpool.Pool, tenantID string, projectID int64, date time.Time) ([]AssertionSummary, error) {
	query := `
		SELECT a.id, COALESCE(a.assertion_type::text, 'unknown'), COALESCE(a.description, ''), COALESCE(a.source_quote, ''), a.source_id
		FROM assertions a
		JOIN sources s ON s.id = a.source_id AND s.tenant_id = a.tenant_id
		WHERE a.tenant_id = $1::uuid
		  AND $2 = ANY(s.attributed_project_ids)
		  AND s.source_timestamp >= $3::date
		  AND s.source_timestamp < ($3::date + interval '1 day')
		ORDER BY a.id
	`

	rows, err := pool.Query(ctx, query, tenantID, projectID, date)
	if err != nil {
		return nil, fmt.Errorf("gather assertions: %w", err)
	}
	defer rows.Close()

	result := []AssertionSummary{}
	for rows.Next() {
		var as AssertionSummary
		if err := rows.Scan(&as.ID, &as.AssertionType, &as.Description, &as.SourceQuote, &as.SourceID); err != nil {
			return nil, fmt.Errorf("scan assertion: %w", err)
		}
		result = append(result, as)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather assertions rows: %w", err)
	}

	return result, nil
}

// GatherInstructionMatches queries instruction matches for the given tenant and project on the specified date.
// Matches for global instructions (project_id IS NULL) are included alongside project-specific ones.
// Returns an empty slice (not nil) if no results are found.
func GatherInstructionMatches(ctx context.Context, pool *pgxpool.Pool, tenantID string, projectID int64, date time.Time) ([]InstructionMatchSummary, error) {
	query := `
		SELECT im.instruction_id, wi.name, im.confidence, im.explanation, im.matched_at
		FROM instruction_matches im
		JOIN watch_instructions wi ON wi.id = im.instruction_id
		WHERE im.tenant_id = $1::uuid
		  AND (wi.project_id = $2 OR wi.project_id IS NULL)
		  AND im.matched_at >= $3::date
		  AND im.matched_at < ($3::date + interval '1 day')
		ORDER BY im.matched_at
	`

	rows, err := pool.Query(ctx, query, tenantID, projectID, date)
	if err != nil {
		return nil, fmt.Errorf("gather instruction matches: %w", err)
	}
	defer rows.Close()

	result := []InstructionMatchSummary{}
	for rows.Next() {
		var ims InstructionMatchSummary
		if err := rows.Scan(&ims.InstructionID, &ims.InstructionName, &ims.Confidence, &ims.Explanation, &ims.MatchedAt); err != nil {
			return nil, fmt.Errorf("scan instruction match: %w", err)
		}
		result = append(result, ims)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather instruction matches rows: %w", err)
	}

	return result, nil
}

// GatherLedgerEntries queries session ledger entries for the given tenant on the specified date.
// The session_ledger_entries table has no project_id column, so all tenant entries for the date are returned.
// Returns an empty slice (not nil) if no results are found.
func GatherLedgerEntries(ctx context.Context, pool *pgxpool.Pool, tenantID string, date time.Time) ([]LedgerEntrySummary, error) {
	query := `
		SELECT id, COALESCE(entry_type, ''), COALESCE(body, ''), COALESCE(source, '')
		FROM session_ledger_entries
		WHERE tenant_id = $1
		  AND created_at >= $2::date
		  AND created_at < ($2::date + interval '1 day')
		ORDER BY created_at
	`

	rows, err := pool.Query(ctx, query, tenantID, date)
	if err != nil {
		return nil, fmt.Errorf("gather ledger entries: %w", err)
	}
	defer rows.Close()

	result := []LedgerEntrySummary{}
	for rows.Next() {
		var le LedgerEntrySummary
		if err := rows.Scan(&le.ID, &le.Category, &le.Content, &le.Source); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		result = append(result, le)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather ledger entries rows: %w", err)
	}

	return result, nil
}

// DailyDigestSummary holds a daily digest record for weekly rollup generation.
type DailyDigestSummary struct {
	ID      string
	Date    time.Time
	Summary string
}

// ThemeSummary holds topic/theme data for weekly digest generation.
type ThemeSummary struct {
	TopicID        int64
	Name           string
	Description    string
	RunningContext string
	Keywords       []string
}

// GatherDailyDigests returns daily digests for a project within the given week.
// Only the summary field is extracted from the body JSON to avoid carrying full payloads.
// Returns an empty slice (not nil) if no results are found.
func GatherDailyDigests(ctx context.Context, pool *pgxpool.Pool, tenantID string, projectID int64, weekStart, weekEnd time.Time) ([]DailyDigestSummary, error) {
	query := `
		SELECT id, period_start, COALESCE(body->>'summary', '')
		FROM digests
		WHERE tenant_id = $1
		  AND project_id = $2
		  AND digest_type = 'daily'
		  AND period_start >= $3
		  AND period_start <= $4
		ORDER BY period_start
	`

	rows, err := pool.Query(ctx, query, tenantID, projectID, weekStart, weekEnd)
	if err != nil {
		return nil, fmt.Errorf("gather daily digests: %w", err)
	}
	defer rows.Close()

	result := []DailyDigestSummary{}
	for rows.Next() {
		var d DailyDigestSummary
		if err := rows.Scan(&d.ID, &d.Date, &d.Summary); err != nil {
			return nil, fmt.Errorf("scan daily digest: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather daily digests rows: %w", err)
	}

	return result, nil
}

// SourceFilter defines content filtering criteria for digest rollup.
type SourceFilter struct {
	Subtypes []string `json:"subtypes,omitempty"` // e.g. ["NOTIFICATION"]
	Sources  []string `json:"sources,omitempty"`  // e.g. ["aha"] — matched with ILIKE
}

// GatherContentBySourceFilter queries content matching the given source filter within a time window.
// Filters on content_enrichment.content_subtype (exact match, ANY) and/or
// content_enrichment.notification_source (ILIKE, ANY).
// Returns up to maxItems results ordered by source_timestamp DESC.
// Returns an empty slice (not nil) if no results are found.
func GatherContentBySourceFilter(ctx context.Context, pool *pgxpool.Pool,
	tenantID string, window TimeWindow, filter SourceFilter, maxItems int) ([]ContentSummary, error) {

	if maxItems <= 0 {
		maxItems = 50
	}

	// Build dynamic WHERE clauses based on filter
	conditions := []string{
		"s.tenant_id = $1::uuid",
		"s.source_timestamp >= $2",
		"s.source_timestamp < $3",
	}
	args := []interface{}{tenantID, window.From, window.To}
	argIdx := 4

	if len(filter.Subtypes) > 0 {
		conditions = append(conditions, fmt.Sprintf("ce.content_subtype = ANY($%d)", argIdx))
		args = append(args, filter.Subtypes)
		argIdx++
	}

	if len(filter.Sources) > 0 {
		// Build ILIKE ANY pattern matching for sources
		patterns := make([]string, len(filter.Sources))
		for i, src := range filter.Sources {
			patterns[i] = "%" + src + "%"
		}
		conditions = append(conditions, fmt.Sprintf("ce.notification_source ILIKE ANY($%d)", argIdx))
		args = append(args, patterns)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT s.id,
		       COALESCE(s.ingestion_metadata->>'subject', ''),
		       COALESCE(s.ingestion_metadata->>'from_address', ''),
		       s.source_timestamp,
		       COALESCE(ce.extracted_data->>'summary', ce.extracted_data->'newsletter'->>'summary', '')
		FROM sources s
		LEFT JOIN content_enrichment ce ON ce.source_id = s.id AND ce.tenant_id = s.tenant_id::text
		WHERE %s
		ORDER BY s.source_timestamp DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, maxItems)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("gather content by source filter: %w", err)
	}
	defer rows.Close()

	result := []ContentSummary{}
	for rows.Next() {
		var cs ContentSummary
		if err := rows.Scan(&cs.SourceID, &cs.Subject, &cs.From, &cs.Date, &cs.Summary); err != nil {
			return nil, fmt.Errorf("scan content by source filter: %w", err)
		}
		result = append(result, cs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather content by source filter rows: %w", err)
	}

	return result, nil
}

// GatherProjectThemes returns active project-scoped topics with running_context.
// Returns an empty slice (not nil) if no results are found.
func GatherProjectThemes(ctx context.Context, pool *pgxpool.Pool, tenantID string, projectID int64) ([]ThemeSummary, error) {
	query := `
		SELECT id, name, COALESCE(description, ''), COALESCE(running_context, ''), COALESCE(keywords, '{}')
		FROM topics
		WHERE tenant_id = $1
		  AND project_id = $2
		  AND status = 'active'
		ORDER BY name
	`

	rows, err := pool.Query(ctx, query, tenantID, projectID)
	if err != nil {
		return nil, fmt.Errorf("gather project themes: %w", err)
	}
	defer rows.Close()

	result := []ThemeSummary{}
	for rows.Next() {
		var t ThemeSummary
		if err := rows.Scan(&t.TopicID, &t.Name, &t.Description, &t.RunningContext, &t.Keywords); err != nil {
			return nil, fmt.Errorf("scan project theme: %w", err)
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("gather project themes rows: %w", err)
	}

	return result, nil
}
