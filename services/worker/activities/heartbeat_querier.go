package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgHeartbeatQuerier implements HeartbeatQuerier using direct SQL queries
// for tables that don't have existing repository abstractions.
type PgHeartbeatQuerier struct {
	db *pgxpool.Pool
}

// NewPgHeartbeatQuerier creates a new PgHeartbeatQuerier.
func NewPgHeartbeatQuerier(db *pgxpool.Pool) *PgHeartbeatQuerier {
	if db == nil {
		panic("NewPgHeartbeatQuerier: db is required")
	}
	return &PgHeartbeatQuerier{db: db}
}

// CountWatchMatches counts assertions created within the given window (using SQL-side NOW()).
func (q *PgHeartbeatQuerier) CountWatchMatches(ctx context.Context, tenantID string, windowHours int) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1
		AND created_at > NOW() - make_interval(hours => $2)
	`, tenantID, windowHours).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count watch matches: %w", err)
	}
	return count, nil
}

// CountStaleContent counts sources stuck in processing for longer than the threshold (using SQL-side NOW()).
func (q *PgHeartbeatQuerier) CountStaleContent(ctx context.Context, tenantID string, thresholdHours int) (int, error) {
	var count int
	err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sources
		WHERE tenant_id = $1
		AND processing_status IN ('processing', 'pending')
		AND updated_at < NOW() - make_interval(hours => $2)
	`, tenantID, thresholdHours).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count stale content: %w", err)
	}
	return count, nil
}
