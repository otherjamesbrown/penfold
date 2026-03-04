// Package digest provides data gathering functions for digest generation.
package digest

import (
	"context"
	"fmt"
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
		       COALESCE(ce.extracted_data->>'summary', '')
		FROM sources s
		LEFT JOIN content_enrichment ce ON ce.source_id = s.id AND ce.tenant_id = s.tenant_id::text
		WHERE s.tenant_id = $1::uuid
		  AND $2 = ANY(s.attributed_project_ids)
		  AND s.source_timestamp::date = $3::date
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
		  AND s.source_timestamp::date = $3::date
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
		  AND im.matched_at::date = $3::date
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
func GatherLedgerEntries(ctx context.Context, pool *pgxpool.Pool, tenantID string, projectID int64, date time.Time) ([]LedgerEntrySummary, error) {
	query := `
		SELECT id, COALESCE(entry_type, ''), COALESCE(body, ''), COALESCE(source, '')
		FROM session_ledger_entries
		WHERE tenant_id = $1
		  AND created_at::date = $2::date
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
