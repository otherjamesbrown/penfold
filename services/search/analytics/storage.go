// Package analytics provides search analytics tracking and aggregation.
package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Common errors for the analytics storage layer.
var (
	ErrEventNotFound   = errors.New("event not found")
	ErrInvalidTenantID = errors.New("tenant_id is required")
	ErrInvalidQueryID  = errors.New("query_id is required")
	ErrInvalidFilter   = errors.New("invalid filter parameters")
)

// Store defines the interface for analytics data persistence.
// Implementations must ensure tenant isolation for all operations.
type Store interface {
	// Query event operations
	SaveQueryEvent(ctx context.Context, event *QueryEvent) error
	GetQueryEvent(ctx context.Context, tenantID, queryID string) (*QueryEvent, error)
	ListQueryEvents(ctx context.Context, filter StatsFilter) ([]QueryEvent, error)

	// Click event operations
	SaveClickEvent(ctx context.Context, event *ClickEvent) error
	GetClickEvent(ctx context.Context, tenantID, clickID string) (*ClickEvent, error)
	ListClickEvents(ctx context.Context, filter StatsFilter) ([]ClickEvent, error)
	GetClicksForQuery(ctx context.Context, tenantID, queryID string) ([]ClickEvent, error)

	// Conversion event operations
	SaveConversionEvent(ctx context.Context, event *ConversionEvent) error
	GetConversionEvent(ctx context.Context, tenantID, conversionID string) (*ConversionEvent, error)
	ListConversionEvents(ctx context.Context, filter StatsFilter) ([]ConversionEvent, error)
	GetConversionsForQuery(ctx context.Context, tenantID, queryID string) ([]ConversionEvent, error)

	// Aggregation operations
	GetQueryStats(ctx context.Context, filter StatsFilter) (*QueryStats, error)
	GetTopQueries(ctx context.Context, filter StatsFilter, limit int) ([]QueryFrequency, error)
	GetZeroResultQueries(ctx context.Context, filter StatsFilter, limit int) ([]string, error)
	GetClickThroughRate(ctx context.Context, filter StatsFilter) (float64, error)

	// Rollup operations for time-series optimization
	SaveDailyRollup(ctx context.Context, rollup *DailyRollup) error
	GetDailyRollup(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error)
	ListDailyRollups(ctx context.Context, filter StatsFilter) ([]DailyRollup, error)
	ComputeDailyRollup(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error)

	// A/B test operations
	GetABTestResults(ctx context.Context, filter StatsFilter, testName string) (*ABTestResult, error)

	// Maintenance operations
	DeleteOldEvents(ctx context.Context, tenantID string, before time.Time) (int64, error)
	Close() error
}

// PostgresStore implements Store using PostgreSQL with time-series optimizations.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgreSQL-backed analytics store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// SaveQueryEvent persists a query event to the database.
func (s *PostgresStore) SaveQueryEvent(ctx context.Context, event *QueryEvent) error {
	if event.TenantID == "" {
		return ErrInvalidTenantID
	}

	// Generate query hash if not provided
	if event.QueryHash == "" {
		event.QueryHash = HashQuery(event.QueryText)
	}

	query := `
		INSERT INTO search_analytics.query_events (
			id, query_text, query_hash, user_id, tenant_id, session_id,
			timestamp, result_count, total_matches, latency_ms, search_mode,
			filters, ab_test_variant, top_results
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
		ON CONFLICT (id) DO UPDATE SET
			result_count = EXCLUDED.result_count,
			latency_ms = EXCLUDED.latency_ms
	`

	_, err := s.pool.Exec(ctx, query,
		event.ID,
		event.QueryText,
		event.QueryHash,
		nullableString(event.UserID),
		event.TenantID,
		nullableString(event.SessionID),
		event.Timestamp,
		event.ResultCount,
		event.TotalMatches,
		event.LatencyMs,
		event.SearchMode,
		event.Filters,
		nullableString(event.ABTestVariant),
		event.TopResults,
	)

	return err
}

// GetQueryEvent retrieves a query event by ID.
func (s *PostgresStore) GetQueryEvent(ctx context.Context, tenantID, queryID string) (*QueryEvent, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT id, query_text, query_hash, user_id, tenant_id, session_id,
			   timestamp, result_count, total_matches, latency_ms, search_mode,
			   filters, ab_test_variant, top_results
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND id = $2
	`

	var event QueryEvent
	var userID, sessionID, abTestVariant *string

	err := s.pool.QueryRow(ctx, query, tenantID, queryID).Scan(
		&event.ID,
		&event.QueryText,
		&event.QueryHash,
		&userID,
		&event.TenantID,
		&sessionID,
		&event.Timestamp,
		&event.ResultCount,
		&event.TotalMatches,
		&event.LatencyMs,
		&event.SearchMode,
		&event.Filters,
		&abTestVariant,
		&event.TopResults,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, err
	}

	event.UserID = derefString(userID)
	event.SessionID = derefString(sessionID)
	event.ABTestVariant = derefString(abTestVariant)

	return &event, nil
}

// ListQueryEvents retrieves query events matching the filter.
func (s *PostgresStore) ListQueryEvents(ctx context.Context, filter StatsFilter) ([]QueryEvent, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT id, query_text, query_hash, user_id, tenant_id, session_id,
			   timestamp, result_count, total_matches, latency_ms, search_mode,
			   filters, ab_test_variant, top_results
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
	`
	args := []interface{}{filter.TenantID, filter.DateFrom, filter.DateTo}
	argIdx := 4

	if filter.SearchMode != "" {
		query += fmt.Sprintf(" AND search_mode = $%d", argIdx)
		args = append(args, filter.SearchMode)
		argIdx++
	}

	if filter.UserID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, filter.UserID)
		argIdx++
	}

	if filter.ABTestVariant != "" {
		query += fmt.Sprintf(" AND ab_test_variant = $%d", argIdx)
		args = append(args, filter.ABTestVariant)
		argIdx++
	}

	if !filter.IncludeZeroResults {
		query += " AND result_count > 0"
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []QueryEvent
	for rows.Next() {
		var event QueryEvent
		var userID, sessionID, abTestVariant *string

		if err := rows.Scan(
			&event.ID,
			&event.QueryText,
			&event.QueryHash,
			&userID,
			&event.TenantID,
			&sessionID,
			&event.Timestamp,
			&event.ResultCount,
			&event.TotalMatches,
			&event.LatencyMs,
			&event.SearchMode,
			&event.Filters,
			&abTestVariant,
			&event.TopResults,
		); err != nil {
			return nil, err
		}

		event.UserID = derefString(userID)
		event.SessionID = derefString(sessionID)
		event.ABTestVariant = derefString(abTestVariant)
		events = append(events, event)
	}

	return events, rows.Err()
}

// SaveClickEvent persists a click event to the database.
func (s *PostgresStore) SaveClickEvent(ctx context.Context, event *ClickEvent) error {
	if event.TenantID == "" {
		return ErrInvalidTenantID
	}
	if event.QueryID == "" {
		return ErrInvalidQueryID
	}

	query := `
		INSERT INTO search_analytics.click_events (
			id, query_id, result_id, position, tenant_id, user_id,
			session_id, timestamp, dwell_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			dwell_time_ms = COALESCE(EXCLUDED.dwell_time_ms, search_analytics.click_events.dwell_time_ms)
	`

	_, err := s.pool.Exec(ctx, query,
		event.ID,
		event.QueryID,
		event.ResultID,
		event.Position,
		event.TenantID,
		nullableString(event.UserID),
		nullableString(event.SessionID),
		event.Timestamp,
		event.DwellTimeMs,
	)

	return err
}

// GetClickEvent retrieves a click event by ID.
func (s *PostgresStore) GetClickEvent(ctx context.Context, tenantID, clickID string) (*ClickEvent, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT id, query_id, result_id, position, tenant_id, user_id,
			   session_id, timestamp, dwell_time_ms
		FROM search_analytics.click_events
		WHERE tenant_id = $1 AND id = $2
	`

	var event ClickEvent
	var userID, sessionID *string

	err := s.pool.QueryRow(ctx, query, tenantID, clickID).Scan(
		&event.ID,
		&event.QueryID,
		&event.ResultID,
		&event.Position,
		&event.TenantID,
		&userID,
		&sessionID,
		&event.Timestamp,
		&event.DwellTimeMs,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, err
	}

	event.UserID = derefString(userID)
	event.SessionID = derefString(sessionID)

	return &event, nil
}

// ListClickEvents retrieves click events matching the filter.
func (s *PostgresStore) ListClickEvents(ctx context.Context, filter StatsFilter) ([]ClickEvent, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT id, query_id, result_id, position, tenant_id, user_id,
			   session_id, timestamp, dwell_time_ms
		FROM search_analytics.click_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
		ORDER BY timestamp DESC
	`
	args := []interface{}{filter.TenantID, filter.DateFrom, filter.DateTo}

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ClickEvent
	for rows.Next() {
		var event ClickEvent
		var userID, sessionID *string

		if err := rows.Scan(
			&event.ID,
			&event.QueryID,
			&event.ResultID,
			&event.Position,
			&event.TenantID,
			&userID,
			&sessionID,
			&event.Timestamp,
			&event.DwellTimeMs,
		); err != nil {
			return nil, err
		}

		event.UserID = derefString(userID)
		event.SessionID = derefString(sessionID)
		events = append(events, event)
	}

	return events, rows.Err()
}

// GetClicksForQuery retrieves all clicks for a specific query.
func (s *PostgresStore) GetClicksForQuery(ctx context.Context, tenantID, queryID string) ([]ClickEvent, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if queryID == "" {
		return nil, ErrInvalidQueryID
	}

	query := `
		SELECT id, query_id, result_id, position, tenant_id, user_id,
			   session_id, timestamp, dwell_time_ms
		FROM search_analytics.click_events
		WHERE tenant_id = $1 AND query_id = $2
		ORDER BY timestamp ASC
	`

	rows, err := s.pool.Query(ctx, query, tenantID, queryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ClickEvent
	for rows.Next() {
		var event ClickEvent
		var userID, sessionID *string

		if err := rows.Scan(
			&event.ID,
			&event.QueryID,
			&event.ResultID,
			&event.Position,
			&event.TenantID,
			&userID,
			&sessionID,
			&event.Timestamp,
			&event.DwellTimeMs,
		); err != nil {
			return nil, err
		}

		event.UserID = derefString(userID)
		event.SessionID = derefString(sessionID)
		events = append(events, event)
	}

	return events, rows.Err()
}

// SaveConversionEvent persists a conversion event to the database.
func (s *PostgresStore) SaveConversionEvent(ctx context.Context, event *ConversionEvent) error {
	if event.TenantID == "" {
		return ErrInvalidTenantID
	}
	if event.QueryID == "" {
		return ErrInvalidQueryID
	}

	query := `
		INSERT INTO search_analytics.conversion_events (
			id, query_id, result_id, action, tenant_id, user_id,
			session_id, timestamp, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`

	_, err := s.pool.Exec(ctx, query,
		event.ID,
		event.QueryID,
		event.ResultID,
		event.Action,
		event.TenantID,
		nullableString(event.UserID),
		nullableString(event.SessionID),
		event.Timestamp,
		event.Metadata,
	)

	return err
}

// GetConversionEvent retrieves a conversion event by ID.
func (s *PostgresStore) GetConversionEvent(ctx context.Context, tenantID, conversionID string) (*ConversionEvent, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT id, query_id, result_id, action, tenant_id, user_id,
			   session_id, timestamp, metadata
		FROM search_analytics.conversion_events
		WHERE tenant_id = $1 AND id = $2
	`

	var event ConversionEvent
	var userID, sessionID *string

	err := s.pool.QueryRow(ctx, query, tenantID, conversionID).Scan(
		&event.ID,
		&event.QueryID,
		&event.ResultID,
		&event.Action,
		&event.TenantID,
		&userID,
		&sessionID,
		&event.Timestamp,
		&event.Metadata,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, err
	}

	event.UserID = derefString(userID)
	event.SessionID = derefString(sessionID)

	return &event, nil
}

// ListConversionEvents retrieves conversion events matching the filter.
func (s *PostgresStore) ListConversionEvents(ctx context.Context, filter StatsFilter) ([]ConversionEvent, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT id, query_id, result_id, action, tenant_id, user_id,
			   session_id, timestamp, metadata
		FROM search_analytics.conversion_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
		ORDER BY timestamp DESC
	`
	args := []interface{}{filter.TenantID, filter.DateFrom, filter.DateTo}

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ConversionEvent
	for rows.Next() {
		var event ConversionEvent
		var userID, sessionID *string

		if err := rows.Scan(
			&event.ID,
			&event.QueryID,
			&event.ResultID,
			&event.Action,
			&event.TenantID,
			&userID,
			&sessionID,
			&event.Timestamp,
			&event.Metadata,
		); err != nil {
			return nil, err
		}

		event.UserID = derefString(userID)
		event.SessionID = derefString(sessionID)
		events = append(events, event)
	}

	return events, rows.Err()
}

// GetConversionsForQuery retrieves all conversions for a specific query.
func (s *PostgresStore) GetConversionsForQuery(ctx context.Context, tenantID, queryID string) ([]ConversionEvent, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if queryID == "" {
		return nil, ErrInvalidQueryID
	}

	query := `
		SELECT id, query_id, result_id, action, tenant_id, user_id,
			   session_id, timestamp, metadata
		FROM search_analytics.conversion_events
		WHERE tenant_id = $1 AND query_id = $2
		ORDER BY timestamp ASC
	`

	rows, err := s.pool.Query(ctx, query, tenantID, queryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ConversionEvent
	for rows.Next() {
		var event ConversionEvent
		var userID, sessionID *string

		if err := rows.Scan(
			&event.ID,
			&event.QueryID,
			&event.ResultID,
			&event.Action,
			&event.TenantID,
			&userID,
			&sessionID,
			&event.Timestamp,
			&event.Metadata,
		); err != nil {
			return nil, err
		}

		event.UserID = derefString(userID)
		event.SessionID = derefString(sessionID)
		events = append(events, event)
	}

	return events, rows.Err()
}

// GetQueryStats retrieves aggregated query statistics.
func (s *PostgresStore) GetQueryStats(ctx context.Context, filter StatsFilter) (*QueryStats, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	// First, try to use rollup tables for efficiency
	stats, err := s.getStatsFromRollups(ctx, filter)
	if err == nil && stats != nil {
		return stats, nil
	}

	// Fall back to computing from raw events
	return s.computeStatsFromEvents(ctx, filter)
}

// getStatsFromRollups attempts to compute stats from pre-aggregated rollup tables.
func (s *PostgresStore) getStatsFromRollups(ctx context.Context, filter StatsFilter) (*QueryStats, error) {
	query := `
		SELECT
			COALESCE(SUM(total_queries), 0) as total_queries,
			COALESCE(SUM(unique_users), 0) as unique_users,
			COALESCE(SUM(total_clicks), 0) as total_clicks,
			COALESCE(SUM(total_conversions), 0) as total_conversions,
			COALESCE(SUM(zero_result_queries), 0) as zero_result_queries,
			COALESCE(SUM(sum_latency_ms), 0) as sum_latency_ms,
			COALESCE(SUM(sum_result_count), 0) as sum_result_count
		FROM search_analytics.daily_rollups
		WHERE tenant_id = $1 AND date >= $2 AND date <= $3
	`

	var totalQueries, uniqueUsers, totalClicks, totalConversions, zeroResultQueries int64
	var sumLatencyMs float64
	var sumResultCount int64

	err := s.pool.QueryRow(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo).Scan(
		&totalQueries,
		&uniqueUsers,
		&totalClicks,
		&totalConversions,
		&zeroResultQueries,
		&sumLatencyMs,
		&sumResultCount,
	)
	if err != nil {
		return nil, err
	}

	if totalQueries == 0 {
		return nil, nil // No data, fall back to raw computation
	}

	stats := &QueryStats{
		TotalQueries:    totalQueries,
		UniqueUsers:     uniqueUsers,
		ZeroResultCount: zeroResultQueries,
		Period: TimePeriod{
			Start:       filter.DateFrom,
			End:         filter.DateTo,
			Granularity: filter.Granularity,
		},
	}

	if totalQueries > 0 {
		stats.AvgLatencyMs = sumLatencyMs / float64(totalQueries)
		stats.AvgResultCount = float64(sumResultCount) / float64(totalQueries)
		stats.ZeroResultRate = float64(zeroResultQueries) / float64(totalQueries)
	}

	if totalQueries > 0 {
		stats.ClickThroughRate = float64(totalClicks) / float64(totalQueries)
	}

	if totalClicks > 0 {
		stats.ConversionRate = float64(totalConversions) / float64(totalClicks)
	}

	// Get top queries
	topQueries, err := s.GetTopQueries(ctx, filter, filter.Limit)
	if err == nil {
		stats.TopQueries = topQueries
	}

	return stats, nil
}

// computeStatsFromEvents computes stats directly from raw event tables.
func (s *PostgresStore) computeStatsFromEvents(ctx context.Context, filter StatsFilter) (*QueryStats, error) {
	query := `
		SELECT
			COUNT(*) as total_queries,
			COUNT(DISTINCT user_id) as unique_users,
			AVG(latency_ms) as avg_latency_ms,
			PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms) as p50_latency,
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms) as p95_latency,
			PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms) as p99_latency,
			AVG(result_count) as avg_result_count,
			COUNT(*) FILTER (WHERE result_count = 0) as zero_result_count
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
	`

	var stats QueryStats
	var avgLatency, p50, p95, p99, avgResultCount *float64

	err := s.pool.QueryRow(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo).Scan(
		&stats.TotalQueries,
		&stats.UniqueUsers,
		&avgLatency,
		&p50,
		&p95,
		&p99,
		&avgResultCount,
		&stats.ZeroResultCount,
	)
	if err != nil {
		return nil, err
	}

	if avgLatency != nil {
		stats.AvgLatencyMs = *avgLatency
	}
	if p50 != nil {
		stats.P50LatencyMs = *p50
	}
	if p95 != nil {
		stats.P95LatencyMs = *p95
	}
	if p99 != nil {
		stats.P99LatencyMs = *p99
	}
	if avgResultCount != nil {
		stats.AvgResultCount = *avgResultCount
	}

	if stats.TotalQueries > 0 {
		stats.ZeroResultRate = float64(stats.ZeroResultCount) / float64(stats.TotalQueries)
	}

	// Get click count
	clickQuery := `
		SELECT COUNT(*) FROM search_analytics.click_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
	`
	var clickCount int64
	if err := s.pool.QueryRow(ctx, clickQuery, filter.TenantID, filter.DateFrom, filter.DateTo).Scan(&clickCount); err == nil {
		if stats.TotalQueries > 0 {
			stats.ClickThroughRate = float64(clickCount) / float64(stats.TotalQueries)
		}
	}

	// Get conversion count
	convQuery := `
		SELECT COUNT(*) FROM search_analytics.conversion_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
	`
	var convCount int64
	if err := s.pool.QueryRow(ctx, convQuery, filter.TenantID, filter.DateFrom, filter.DateTo).Scan(&convCount); err == nil {
		if clickCount > 0 {
			stats.ConversionRate = float64(convCount) / float64(clickCount)
		}
	}

	// Get search mode breakdown
	modeQuery := `
		SELECT search_mode, COUNT(*) as count
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
		GROUP BY search_mode
	`
	rows, err := s.pool.Query(ctx, modeQuery, filter.TenantID, filter.DateFrom, filter.DateTo)
	if err == nil {
		defer rows.Close()
		stats.SearchModeBreakdown = make(map[string]int64)
		for rows.Next() {
			var mode string
			var count int64
			if err := rows.Scan(&mode, &count); err == nil {
				stats.SearchModeBreakdown[mode] = count
			}
		}
	}

	// Get top queries
	topQueries, err := s.GetTopQueries(ctx, filter, filter.Limit)
	if err == nil {
		stats.TopQueries = topQueries
	}

	stats.Period = TimePeriod{
		Start:       filter.DateFrom,
		End:         filter.DateTo,
		Granularity: filter.Granularity,
	}

	return &stats, nil
}

// GetTopQueries retrieves the most frequent queries.
func (s *PostgresStore) GetTopQueries(ctx context.Context, filter StatsFilter, limit int) ([]QueryFrequency, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT
			query_hash,
			MIN(query_text) as query_text,
			COUNT(*) as count,
			AVG(latency_ms) as avg_latency_ms,
			AVG(result_count) as avg_result_count
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
		GROUP BY query_hash
		ORDER BY count DESC
		LIMIT $4
	`

	rows, err := s.pool.Query(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []QueryFrequency
	for rows.Next() {
		var qf QueryFrequency
		if err := rows.Scan(
			&qf.QueryHash,
			&qf.QueryText,
			&qf.Count,
			&qf.AvgLatencyMs,
			&qf.AvgResultCount,
		); err != nil {
			return nil, err
		}
		queries = append(queries, qf)
	}

	return queries, rows.Err()
}

// GetZeroResultQueries retrieves queries that returned no results.
func (s *PostgresStore) GetZeroResultQueries(ctx context.Context, filter StatsFilter, limit int) ([]string, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT DISTINCT query_text
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND result_count = 0
		ORDER BY query_text
		LIMIT $4
	`

	rows, err := s.pool.Query(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []string
	for rows.Next() {
		var queryText string
		if err := rows.Scan(&queryText); err != nil {
			return nil, err
		}
		queries = append(queries, queryText)
	}

	return queries, rows.Err()
}

// GetClickThroughRate calculates the CTR for queries matching a pattern.
func (s *PostgresStore) GetClickThroughRate(ctx context.Context, filter StatsFilter) (float64, error) {
	if filter.TenantID == "" {
		return 0, ErrInvalidTenantID
	}

	query := `
		WITH query_counts AS (
			SELECT COUNT(*) as total
			FROM search_analytics.query_events
			WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
		),
		click_counts AS (
			SELECT COUNT(DISTINCT c.query_id) as clicked
			FROM search_analytics.click_events c
			JOIN search_analytics.query_events q ON c.query_id = q.id
			WHERE q.tenant_id = $1 AND c.timestamp >= $2 AND c.timestamp <= $3
		)
		SELECT
			CASE
				WHEN qc.total = 0 THEN 0
				ELSE cc.clicked::float / qc.total::float
			END as ctr
		FROM query_counts qc, click_counts cc
	`

	var ctr float64
	err := s.pool.QueryRow(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo).Scan(&ctr)
	if err != nil {
		return 0, err
	}

	return ctr, nil
}

// SaveDailyRollup persists a daily rollup to the database.
func (s *PostgresStore) SaveDailyRollup(ctx context.Context, rollup *DailyRollup) error {
	if rollup.TenantID == "" {
		return ErrInvalidTenantID
	}

	query := `
		INSERT INTO search_analytics.daily_rollups (
			date, tenant_id, total_queries, unique_users, total_clicks,
			total_conversions, zero_result_queries, sum_latency_ms, sum_result_count,
			latency_histogram, search_mode_breakdown, top_queries_json, zero_result_queries_json
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (tenant_id, date) DO UPDATE SET
			total_queries = EXCLUDED.total_queries,
			unique_users = EXCLUDED.unique_users,
			total_clicks = EXCLUDED.total_clicks,
			total_conversions = EXCLUDED.total_conversions,
			zero_result_queries = EXCLUDED.zero_result_queries,
			sum_latency_ms = EXCLUDED.sum_latency_ms,
			sum_result_count = EXCLUDED.sum_result_count,
			latency_histogram = EXCLUDED.latency_histogram,
			search_mode_breakdown = EXCLUDED.search_mode_breakdown,
			top_queries_json = EXCLUDED.top_queries_json,
			zero_result_queries_json = EXCLUDED.zero_result_queries_json
	`

	_, err := s.pool.Exec(ctx, query,
		rollup.Date,
		rollup.TenantID,
		rollup.TotalQueries,
		rollup.UniqueUsers,
		rollup.TotalClicks,
		rollup.TotalConversions,
		rollup.ZeroResultQueries,
		rollup.SumLatencyMs,
		rollup.SumResultCount,
		rollup.LatencyHistogram,
		rollup.SearchModeBreakdown,
		rollup.TopQueriesJSON,
		rollup.ZeroResultQueriesJSON,
	)

	return err
}

// GetDailyRollup retrieves a specific daily rollup.
func (s *PostgresStore) GetDailyRollup(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT date, tenant_id, total_queries, unique_users, total_clicks,
			   total_conversions, zero_result_queries, sum_latency_ms, sum_result_count,
			   latency_histogram, search_mode_breakdown, top_queries_json, zero_result_queries_json
		FROM search_analytics.daily_rollups
		WHERE tenant_id = $1 AND date = $2
	`

	var rollup DailyRollup
	err := s.pool.QueryRow(ctx, query, tenantID, date.Truncate(24*time.Hour)).Scan(
		&rollup.Date,
		&rollup.TenantID,
		&rollup.TotalQueries,
		&rollup.UniqueUsers,
		&rollup.TotalClicks,
		&rollup.TotalConversions,
		&rollup.ZeroResultQueries,
		&rollup.SumLatencyMs,
		&rollup.SumResultCount,
		&rollup.LatencyHistogram,
		&rollup.SearchModeBreakdown,
		&rollup.TopQueriesJSON,
		&rollup.ZeroResultQueriesJSON,
	)

	if err == pgx.ErrNoRows {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, err
	}

	return &rollup, nil
}

// ListDailyRollups retrieves daily rollups for a date range.
func (s *PostgresStore) ListDailyRollups(ctx context.Context, filter StatsFilter) ([]DailyRollup, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT date, tenant_id, total_queries, unique_users, total_clicks,
			   total_conversions, zero_result_queries, sum_latency_ms, sum_result_count,
			   latency_histogram, search_mode_breakdown, top_queries_json, zero_result_queries_json
		FROM search_analytics.daily_rollups
		WHERE tenant_id = $1 AND date >= $2 AND date <= $3
		ORDER BY date ASC
	`

	rows, err := s.pool.Query(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rollups []DailyRollup
	for rows.Next() {
		var rollup DailyRollup
		if err := rows.Scan(
			&rollup.Date,
			&rollup.TenantID,
			&rollup.TotalQueries,
			&rollup.UniqueUsers,
			&rollup.TotalClicks,
			&rollup.TotalConversions,
			&rollup.ZeroResultQueries,
			&rollup.SumLatencyMs,
			&rollup.SumResultCount,
			&rollup.LatencyHistogram,
			&rollup.SearchModeBreakdown,
			&rollup.TopQueriesJSON,
			&rollup.ZeroResultQueriesJSON,
		); err != nil {
			return nil, err
		}
		rollups = append(rollups, rollup)
	}

	return rollups, rows.Err()
}

// ComputeDailyRollup computes a daily rollup from raw events.
func (s *PostgresStore) ComputeDailyRollup(ctx context.Context, tenantID string, date time.Time) (*DailyRollup, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	startOfDay := date.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	filter := StatsFilter{
		TenantID: tenantID,
		DateFrom: startOfDay,
		DateTo:   endOfDay,
		Limit:    50, // Top 50 queries for rollup
	}

	// Get basic stats
	stats, err := s.computeStatsFromEvents(ctx, filter)
	if err != nil {
		return nil, err
	}

	rollup := &DailyRollup{
		Date:                date.Truncate(24 * time.Hour),
		TenantID:            tenantID,
		TotalQueries:        stats.TotalQueries,
		UniqueUsers:         stats.UniqueUsers,
		ZeroResultQueries:   stats.ZeroResultCount,
		SumLatencyMs:        stats.AvgLatencyMs * float64(stats.TotalQueries),
		SumResultCount:      int64(stats.AvgResultCount * float64(stats.TotalQueries)),
		SearchModeBreakdown: stats.SearchModeBreakdown,
	}

	// Get click count
	clickQuery := `
		SELECT COUNT(*) FROM search_analytics.click_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp < $3
	`
	s.pool.QueryRow(ctx, clickQuery, tenantID, startOfDay, endOfDay).Scan(&rollup.TotalClicks)

	// Get conversion count
	convQuery := `
		SELECT COUNT(*) FROM search_analytics.conversion_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp < $3
	`
	s.pool.QueryRow(ctx, convQuery, tenantID, startOfDay, endOfDay).Scan(&rollup.TotalConversions)

	// Compute latency histogram
	rollup.LatencyHistogram = s.computeLatencyHistogram(ctx, tenantID, startOfDay, endOfDay)

	return rollup, nil
}

// computeLatencyHistogram computes latency distribution buckets.
func (s *PostgresStore) computeLatencyHistogram(ctx context.Context, tenantID string, start, end time.Time) map[string]int64 {
	query := `
		SELECT
			CASE
				WHEN latency_ms < 50 THEN '0-50ms'
				WHEN latency_ms < 100 THEN '50-100ms'
				WHEN latency_ms < 200 THEN '100-200ms'
				WHEN latency_ms < 500 THEN '200-500ms'
				WHEN latency_ms < 1000 THEN '500-1000ms'
				ELSE '1000ms+'
			END as bucket,
			COUNT(*) as count
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp < $3
		GROUP BY bucket
	`

	rows, err := s.pool.Query(ctx, query, tenantID, start, end)
	if err != nil {
		return nil
	}
	defer rows.Close()

	histogram := make(map[string]int64)
	for rows.Next() {
		var bucket string
		var count int64
		if err := rows.Scan(&bucket, &count); err == nil {
			histogram[bucket] = count
		}
	}

	return histogram
}

// GetABTestResults retrieves A/B test comparison data.
func (s *PostgresStore) GetABTestResults(ctx context.Context, filter StatsFilter, testName string) (*ABTestResult, error) {
	if filter.TenantID == "" {
		return nil, ErrInvalidTenantID
	}

	query := `
		SELECT
			ab_test_variant,
			COUNT(*) as query_count,
			AVG(latency_ms) as avg_latency_ms,
			AVG(result_count) as avg_result_count
		FROM search_analytics.query_events
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp <= $3
			AND ab_test_variant IS NOT NULL AND ab_test_variant LIKE $4
		GROUP BY ab_test_variant
	`

	pattern := testName + ":%"
	rows, err := s.pool.Query(ctx, query, filter.TenantID, filter.DateFrom, filter.DateTo, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &ABTestResult{
		TestName: testName,
		Variants: make(map[string]VariantStats),
		Period: TimePeriod{
			Start:       filter.DateFrom,
			End:         filter.DateTo,
			Granularity: filter.Granularity,
		},
	}

	for rows.Next() {
		var variant string
		var stats VariantStats

		if err := rows.Scan(
			&variant,
			&stats.QueryCount,
			&stats.AvgLatencyMs,
			&stats.AvgResultCount,
		); err != nil {
			return nil, err
		}

		// Extract variant name from "testName:variantName" format
		parts := strings.SplitN(variant, ":", 2)
		if len(parts) == 2 {
			variant = parts[1]
		}

		result.Variants[variant] = stats
	}

	return result, rows.Err()
}

// DeleteOldEvents removes events older than the specified time.
func (s *PostgresStore) DeleteOldEvents(ctx context.Context, tenantID string, before time.Time) (int64, error) {
	if tenantID == "" {
		return 0, ErrInvalidTenantID
	}

	var totalDeleted int64

	// Delete query events
	queryResult, err := s.pool.Exec(ctx,
		"DELETE FROM search_analytics.query_events WHERE tenant_id = $1 AND timestamp < $2",
		tenantID, before,
	)
	if err != nil {
		return 0, err
	}
	totalDeleted += queryResult.RowsAffected()

	// Delete click events
	clickResult, err := s.pool.Exec(ctx,
		"DELETE FROM search_analytics.click_events WHERE tenant_id = $1 AND timestamp < $2",
		tenantID, before,
	)
	if err != nil {
		return totalDeleted, err
	}
	totalDeleted += clickResult.RowsAffected()

	// Delete conversion events
	convResult, err := s.pool.Exec(ctx,
		"DELETE FROM search_analytics.conversion_events WHERE tenant_id = $1 AND timestamp < $2",
		tenantID, before,
	)
	if err != nil {
		return totalDeleted, err
	}
	totalDeleted += convResult.RowsAffected()

	return totalDeleted, nil
}

// Close closes the underlying connection pool.
func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

// HashQuery creates a normalized hash for grouping similar queries.
func HashQuery(query string) string {
	// Normalize the query
	normalized := strings.ToLower(strings.TrimSpace(query))
	// Remove extra whitespace
	normalized = strings.Join(strings.Fields(normalized), " ")

	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:8]) // Use first 8 bytes for shorter hashes
}

// Helper functions for nullable string handling.

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
