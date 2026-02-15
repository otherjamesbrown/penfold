package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides access to service logs data.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new logs repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create adds a new log entry.
func (r *Repository) Create(ctx context.Context, input EntryInput) (*Entry, error) {
	timestamp := input.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	fieldsJSON, err := json.Marshal(input.Fields)
	if err != nil {
		fieldsJSON = []byte("{}")
	}

	var entry Entry
	var fieldsRaw []byte

	err = r.db.QueryRow(ctx, `
		INSERT INTO service_logs (tenant_id, timestamp, level, service, message, fields, trace_id, span_id, caller)
		VALUES (COALESCE(NULLIF($1, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid), $2, $3, $4, $5, $6::jsonb, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''))
		RETURNING id, tenant_id, timestamp, level, service, message, fields, COALESCE(trace_id, '') AS trace_id, COALESCE(span_id, '') AS span_id, COALESCE(caller, '') AS caller, created_at
	`,
		input.TenantID,
		timestamp,
		input.Level,
		input.Service,
		input.Message,
		string(fieldsJSON),
		input.TraceID,
		input.SpanID,
		input.Caller,
	).Scan(
		&entry.ID,
		&entry.TenantID,
		&entry.Timestamp,
		&entry.Level,
		&entry.Service,
		&entry.Message,
		&fieldsRaw,
		&entry.TraceID,
		&entry.SpanID,
		&entry.Caller,
		&entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create log entry: %w", err)
	}

	// Parse fields JSON
	if len(fieldsRaw) > 0 {
		if err := json.Unmarshal(fieldsRaw, &entry.Fields); err != nil {
			entry.Fields = make(map[string]string)
		}
	}

	return &entry, nil
}

// CreateBatch adds multiple log entries efficiently.
func (r *Repository) CreateBatch(ctx context.Context, inputs []EntryInput) (int64, error) {
	if len(inputs) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, input := range inputs {
		timestamp := input.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		fieldsJSON, _ := json.Marshal(input.Fields)

		batch.Queue(`
			INSERT INTO service_logs (tenant_id, timestamp, level, service, message, fields, trace_id, span_id, caller)
			VALUES (COALESCE(NULLIF($1, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid), $2, $3, $4, $5, $6::jsonb, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''))
		`,
			input.TenantID,
			timestamp,
			input.Level,
			input.Service,
			input.Message,
			string(fieldsJSON),
			input.TraceID,
			input.SpanID,
			input.Caller,
		)
	}

	results := r.db.SendBatch(ctx, batch)
	defer results.Close()

	var inserted int64
	for i := 0; i < len(inputs); i++ {
		ct, err := results.Exec()
		if err != nil {
			return inserted, fmt.Errorf("batch insert log entry %d: %w", i, err)
		}
		inserted += ct.RowsAffected()
	}

	return inserted, nil
}

// List retrieves log entries matching the filter.
func (r *Repository) List(ctx context.Context, filter Filter) ([]*Entry, error) {
	query := `
		SELECT id, tenant_id, timestamp, level, service, message, fields, COALESCE(trace_id, '') AS trace_id, COALESCE(span_id, '') AS span_id, COALESCE(caller, '') AS caller, created_at
		FROM service_logs
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d::uuid", argNum)
		args = append(args, filter.TenantID)
		argNum++
	}

	if filter.Service != "" {
		query += fmt.Sprintf(" AND service = $%d", argNum)
		args = append(args, filter.Service)
		argNum++
	}

	if filter.Level != "" && filter.Level.IsValid() {
		// Filter by minimum level (e.g., warn includes warn and error)
		levelPriority := filter.Level.Priority()
		levels := []Level{}
		for l := range ValidLevels {
			if l.Priority() >= levelPriority {
				levels = append(levels, l)
			}
		}
		query += fmt.Sprintf(" AND level = ANY($%d)", argNum)
		args = append(args, levels)
		argNum++
	}

	if !filter.Since.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, filter.Since)
		argNum++
	}

	if !filter.Until.IsZero() {
		query += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, filter.Until)
		argNum++
	}

	if filter.Contains != "" {
		query += fmt.Sprintf(" AND to_tsvector('english', message) @@ plainto_tsquery('english', $%d)", argNum)
		args = append(args, filter.Contains)
		argNum++
	}

	if filter.TraceID != "" {
		query += fmt.Sprintf(" AND trace_id = $%d", argNum)
		args = append(args, filter.TraceID)
		argNum++
	}

	// Order by timestamp
	if filter.OrderAsc {
		query += " ORDER BY timestamp ASC"
	} else {
		query += " ORDER BY timestamp DESC"
	}

	// Apply limit
	limit := 100
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	// Apply offset
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list log entries: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		var entry Entry
		var fieldsRaw []byte

		err := rows.Scan(
			&entry.ID,
			&entry.TenantID,
			&entry.Timestamp,
			&entry.Level,
			&entry.Service,
			&entry.Message,
			&fieldsRaw,
			&entry.TraceID,
			&entry.SpanID,
			&entry.Caller,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan log entry: %w", err)
		}

		// Parse fields JSON
		if len(fieldsRaw) > 0 {
			if err := json.Unmarshal(fieldsRaw, &entry.Fields); err != nil {
				entry.Fields = make(map[string]string)
			}
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// Count returns the total count of entries matching the filter.
func (r *Repository) Count(ctx context.Context, filter Filter) (int64, error) {
	query := `SELECT COUNT(*) FROM service_logs WHERE 1=1`
	args := []interface{}{}
	argNum := 1

	if filter.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d::uuid", argNum)
		args = append(args, filter.TenantID)
		argNum++
	}

	if filter.Service != "" {
		query += fmt.Sprintf(" AND service = $%d", argNum)
		args = append(args, filter.Service)
		argNum++
	}

	if filter.Level != "" && filter.Level.IsValid() {
		levelPriority := filter.Level.Priority()
		levels := []Level{}
		for l := range ValidLevels {
			if l.Priority() >= levelPriority {
				levels = append(levels, l)
			}
		}
		query += fmt.Sprintf(" AND level = ANY($%d)", argNum)
		args = append(args, levels)
		argNum++
	}

	if !filter.Since.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, filter.Since)
		argNum++
	}

	if !filter.Until.IsZero() {
		query += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, filter.Until)
		argNum++
	}

	if filter.Contains != "" {
		query += fmt.Sprintf(" AND to_tsvector('english', message) @@ plainto_tsquery('english', $%d)", argNum)
		args = append(args, filter.Contains)
		argNum++
	}

	if filter.TraceID != "" {
		query += fmt.Sprintf(" AND trace_id = $%d", argNum)
		args = append(args, filter.TraceID)
		argNum++
	}

	var count int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count log entries: %w", err)
	}

	return count, nil
}

// GetStats returns statistics about the logs.
func (r *Repository) GetStats(ctx context.Context, tenantID string) (*Stats, error) {
	stats := &Stats{
		CountByLevel:   make(map[Level]int64),
		CountByService: make(map[string]int64),
	}

	// Get total count
	var whereClause string
	var args []interface{}
	if tenantID != "" {
		whereClause = " WHERE tenant_id = $1::uuid"
		args = append(args, tenantID)
	}

	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM service_logs"+whereClause, args...).Scan(&stats.TotalCount)
	if err != nil {
		return nil, fmt.Errorf("get total count: %w", err)
	}

	// Get counts by level
	rows, err := r.db.Query(ctx, "SELECT level, COUNT(*) FROM service_logs"+whereClause+" GROUP BY level", args...)
	if err != nil {
		return nil, fmt.Errorf("get counts by level: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var level Level
		var count int64
		if err := rows.Scan(&level, &count); err != nil {
			return nil, fmt.Errorf("scan level count: %w", err)
		}
		stats.CountByLevel[level] = count
	}

	// Get counts by service
	rows, err = r.db.Query(ctx, "SELECT service, COUNT(*) FROM service_logs"+whereClause+" GROUP BY service", args...)
	if err != nil {
		return nil, fmt.Errorf("get counts by service: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var service string
		var count int64
		if err := rows.Scan(&service, &count); err != nil {
			return nil, fmt.Errorf("scan service count: %w", err)
		}
		stats.CountByService[service] = count
	}

	// Get time range
	var oldest, newest *time.Time
	err = r.db.QueryRow(ctx, "SELECT MIN(timestamp), MAX(timestamp) FROM service_logs"+whereClause, args...).Scan(&oldest, &newest)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get time range: %w", err)
	}
	if oldest != nil {
		stats.OldestLog = *oldest
	}
	if newest != nil {
		stats.NewestLog = *newest
	}

	return stats, nil
}

// ListServices returns a list of all distinct service names.
func (r *Repository) ListServices(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, "SELECT DISTINCT service FROM service_logs ORDER BY service")
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var service string
		if err := rows.Scan(&service); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		services = append(services, service)
	}

	return services, rows.Err()
}

// Cleanup removes logs older than the specified number of days.
func (r *Repository) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}

	result, err := r.db.Exec(ctx, `
		DELETE FROM service_logs
		WHERE timestamp < NOW() - ($1 || ' days')::INTERVAL
	`, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("cleanup old logs: %w", err)
	}

	return result.RowsAffected(), nil
}
