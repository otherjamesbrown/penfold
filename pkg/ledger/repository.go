package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors.
var (
	// ErrNotFound is returned when a ledger entry or consolidation is not found.
	ErrNotFound = pferrors.ErrNotFound
)

// Repository provides database operations for session ledger management.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new ledger repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "ledger_repository")),
	}
}

// ==================== Entry Operations ====================

// CreateEntry inserts a new session ledger entry and populates id and created_at.
func (r *Repository) CreateEntry(ctx context.Context, entry *Entry) error {
	if entry.Labels == nil {
		entry.Labels = []string{}
	}
	if entry.ShardRefs == nil {
		entry.ShardRefs = []string{}
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]string{}
	}

	metaBytes, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO session_ledger_entries (
			tenant_id, session_id, entry_type, title, body,
			source, agent, labels, shard_refs, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`

	err = r.pool.QueryRow(ctx, query,
		entry.TenantID,
		entry.SessionID,
		entry.EntryType,
		entry.Title,
		entry.Body,
		entry.Source,
		entry.Agent,
		entry.Labels,
		entry.ShardRefs,
		metaBytes,
	).Scan(&entry.ID, &entry.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create ledger entry: %w", err)
	}

	r.logger.Debug("Ledger entry created",
		logging.F("id", entry.ID),
		logging.F("session_id", entry.SessionID),
		logging.F("entry_type", entry.EntryType))

	return nil
}

// GetEntry retrieves a single ledger entry by tenant and id.
func (r *Repository) GetEntry(ctx context.Context, tenantID string, id int64) (*Entry, error) {
	query := `
		SELECT id, tenant_id, session_id, entry_type, title, body,
		       source, agent, labels, shard_refs, metadata, created_at
		FROM session_ledger_entries
		WHERE tenant_id = $1 AND id = $2
	`

	entry := &Entry{}
	var metaBytes []byte

	err := r.pool.QueryRow(ctx, query, tenantID, id).Scan(
		&entry.ID,
		&entry.TenantID,
		&entry.SessionID,
		&entry.EntryType,
		&entry.Title,
		&entry.Body,
		&entry.Source,
		&entry.Agent,
		&entry.Labels,
		&entry.ShardRefs,
		&metaBytes,
		&entry.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ledger entry: %w", err)
	}

	if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return entry, nil
}

// ListEntries returns a filtered, paginated list of entries and the total count.
func (r *Repository) ListEntries(ctx context.Context, filter EntryFilter) ([]*Entry, int32, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	where := "WHERE 1=1"
	var args []any
	argNum := 1

	if filter.TenantID != nil {
		where += fmt.Sprintf(" AND tenant_id = $%d", argNum)
		args = append(args, *filter.TenantID)
		argNum++
	}
	if filter.SessionID != nil {
		where += fmt.Sprintf(" AND session_id = $%d", argNum)
		args = append(args, *filter.SessionID)
		argNum++
	}
	if filter.EntryType != nil {
		where += fmt.Sprintf(" AND entry_type = $%d", argNum)
		args = append(args, *filter.EntryType)
		argNum++
	}
	if filter.Source != nil {
		where += fmt.Sprintf(" AND source = $%d", argNum)
		args = append(args, *filter.Source)
		argNum++
	}
	if filter.Agent != nil {
		where += fmt.Sprintf(" AND agent = $%d", argNum)
		args = append(args, *filter.Agent)
		argNum++
	}
	for _, label := range filter.Labels {
		where += fmt.Sprintf(" AND labels @> $%d", argNum)
		args = append(args, []string{label})
		argNum++
	}

	countQuery := "SELECT COUNT(*) FROM session_ledger_entries " + where
	var total int32
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count ledger entries: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, tenant_id, session_id, entry_type, title, body,
		       source, agent, labels, shard_refs, metadata, created_at
		FROM session_ledger_entries
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argNum, argNum+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		entry := &Entry{}
		var metaBytes []byte

		err := rows.Scan(
			&entry.ID,
			&entry.TenantID,
			&entry.SessionID,
			&entry.EntryType,
			&entry.Title,
			&entry.Body,
			&entry.Source,
			&entry.Agent,
			&entry.Labels,
			&entry.ShardRefs,
			&metaBytes,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan ledger entry: %w", err)
		}
		if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, total, rows.Err()
}

// ==================== Session Operations ====================

// ListSessions returns aggregated session summaries and the total session count.
func (r *Repository) ListSessions(ctx context.Context, filter SessionFilter) ([]*SessionSummary, int32, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	agentClause := ""
	var args []any
	args = append(args, filter.TenantID)
	argNum := 2

	if filter.Agent != nil {
		agentClause = fmt.Sprintf(" AND agent = $%d", argNum)
		args = append(args, *filter.Agent)
		argNum++
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT session_id)
		FROM session_ledger_entries
		WHERE tenant_id = $1%s
	`, agentClause)

	var total int32
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count sessions: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT
			session_id,
			MAX(agent) AS agent,
			COUNT(*) AS entry_count,
			MIN(created_at) AS first_entry,
			MAX(created_at) AS last_entry,
			ARRAY_AGG(DISTINCT entry_type) AS entry_types,
			ARRAY(SELECT DISTINCT x FROM unnest(ARRAY_AGG(labels)) AS x WHERE x IS NOT NULL) AS all_labels,
			(
				SELECT title
				FROM session_ledger_entries e2
				WHERE e2.session_id = session_ledger_entries.session_id
				  AND e2.tenant_id = $1
				ORDER BY created_at DESC
				LIMIT 1
			) AS latest_title
		FROM session_ledger_entries
		WHERE tenant_id = $1%s
		GROUP BY session_id
		ORDER BY MAX(created_at) DESC
		LIMIT $%d OFFSET $%d
	`, agentClause, argNum, argNum+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var summaries []*SessionSummary
	for rows.Next() {
		s := &SessionSummary{}
		err := rows.Scan(
			&s.SessionID,
			&s.Agent,
			&s.EntryCount,
			&s.FirstEntry,
			&s.LastEntry,
			&s.EntryTypes,
			&s.Labels,
			&s.LatestTitle,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan session summary: %w", err)
		}
		if s.Labels == nil {
			s.Labels = []string{}
		}
		summaries = append(summaries, s)
	}

	return summaries, total, rows.Err()
}

// GetSession returns the aggregated summary and all entries for a single session.
func (r *Repository) GetSession(ctx context.Context, tenantID, sessionID string) (*SessionSummary, []*Entry, error) {
	summaryQuery := `
		SELECT
			session_id,
			MAX(agent) AS agent,
			COUNT(*) AS entry_count,
			MIN(created_at) AS first_entry,
			MAX(created_at) AS last_entry,
			ARRAY_AGG(DISTINCT entry_type) AS entry_types,
			ARRAY(SELECT DISTINCT x FROM unnest(ARRAY_AGG(labels)) AS x WHERE x IS NOT NULL) AS all_labels,
			(
				SELECT title
				FROM session_ledger_entries e2
				WHERE e2.session_id = session_ledger_entries.session_id
				  AND e2.tenant_id = $1
				ORDER BY created_at DESC
				LIMIT 1
			) AS latest_title
		FROM session_ledger_entries
		WHERE tenant_id = $1 AND session_id = $2
		GROUP BY session_id
	`

	s := &SessionSummary{}
	err := r.pool.QueryRow(ctx, summaryQuery, tenantID, sessionID).Scan(
		&s.SessionID,
		&s.Agent,
		&s.EntryCount,
		&s.FirstEntry,
		&s.LastEntry,
		&s.EntryTypes,
		&s.Labels,
		&s.LatestTitle,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("failed to get session summary: %w", err)
	}
	if s.Labels == nil {
		s.Labels = []string{}
	}

	entriesQuery := `
		SELECT id, tenant_id, session_id, entry_type, title, body,
		       source, agent, labels, shard_refs, metadata, created_at
		FROM session_ledger_entries
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, entriesQuery, tenantID, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get session entries: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		entry := &Entry{}
		var metaBytes []byte

		err := rows.Scan(
			&entry.ID,
			&entry.TenantID,
			&entry.SessionID,
			&entry.EntryType,
			&entry.Title,
			&entry.Body,
			&entry.Source,
			&entry.Agent,
			&entry.Labels,
			&entry.ShardRefs,
			&metaBytes,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan session entry: %w", err)
		}
		if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return s, entries, nil
}

// ==================== Search ====================

// SearchEntries performs full-text search over ledger entries for a tenant.
func (r *Repository) SearchEntries(ctx context.Context, filter SearchFilter) ([]*Entry, int32, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	countQuery := `
		SELECT COUNT(*)
		FROM session_ledger_entries
		WHERE tenant_id = $1
		  AND search_vector @@ plainto_tsquery('english', $2)
	`

	var total int32
	if err := r.pool.QueryRow(ctx, countQuery, filter.TenantID, filter.Query).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count search results: %w", err)
	}

	dataQuery := `
		SELECT id, tenant_id, session_id, entry_type, title, body,
		       source, agent, labels, shard_refs, metadata, created_at
		FROM session_ledger_entries
		WHERE tenant_id = $1
		  AND search_vector @@ plainto_tsquery('english', $2)
		ORDER BY ts_rank(search_vector, plainto_tsquery('english', $2)) DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, dataQuery, filter.TenantID, filter.Query, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		entry := &Entry{}
		var metaBytes []byte

		err := rows.Scan(
			&entry.ID,
			&entry.TenantID,
			&entry.SessionID,
			&entry.EntryType,
			&entry.Title,
			&entry.Body,
			&entry.Source,
			&entry.Agent,
			&entry.Labels,
			&entry.ShardRefs,
			&metaBytes,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan search result: %w", err)
		}
		if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, total, rows.Err()
}

// ==================== Handoff ====================

// GetLatestHandoff returns the most recent handoff entry, optionally filtered by agent.
func (r *Repository) GetLatestHandoff(ctx context.Context, tenantID string, agent *string) (*Entry, error) {
	where := "WHERE tenant_id = $1 AND entry_type = 'handoff'"
	args := []any{tenantID}
	argNum := 2

	if agent != nil {
		where += fmt.Sprintf(" AND agent = $%d", argNum)
		args = append(args, *agent)
	}

	query := fmt.Sprintf(`
		SELECT id, tenant_id, session_id, entry_type, title, body,
		       source, agent, labels, shard_refs, metadata, created_at
		FROM session_ledger_entries
		%s
		ORDER BY created_at DESC
		LIMIT 1
	`, where)

	entry := &Entry{}
	var metaBytes []byte

	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&entry.ID,
		&entry.TenantID,
		&entry.SessionID,
		&entry.EntryType,
		&entry.Title,
		&entry.Body,
		&entry.Source,
		&entry.Agent,
		&entry.Labels,
		&entry.ShardRefs,
		&metaBytes,
		&entry.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get latest handoff: %w", err)
	}

	if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	r.logger.Debug("Latest handoff retrieved",
		logging.F("id", entry.ID),
		logging.F("session_id", entry.SessionID))

	return entry, nil
}

// ==================== Consolidation Operations ====================

// ListConsolidations returns paginated consolidations for a tenant.
func (r *Repository) ListConsolidations(ctx context.Context, tenantID string, limit, offset int32) ([]*Consolidation, int32, error) {
	if limit <= 0 {
		limit = 50
	}

	var total int32
	if err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM session_ledger_consolidations WHERE tenant_id = $1",
		tenantID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count consolidations: %w", err)
	}

	query := `
		SELECT id, tenant_id, title, body, source_entry_ids, session_ids,
		       decisions, patterns, time_start, time_end, model_id, metadata, created_at
		FROM session_ledger_consolidations
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list consolidations: %w", err)
	}
	defer rows.Close()

	var consolidations []*Consolidation
	for rows.Next() {
		c, err := scanConsolidation(rows)
		if err != nil {
			return nil, 0, err
		}
		consolidations = append(consolidations, c)
	}

	return consolidations, total, rows.Err()
}

// GetConsolidation retrieves a single consolidation by tenant and id.
func (r *Repository) GetConsolidation(ctx context.Context, tenantID string, id int64) (*Consolidation, error) {
	query := `
		SELECT id, tenant_id, title, body, source_entry_ids, session_ids,
		       decisions, patterns, time_start, time_end, model_id, metadata, created_at
		FROM session_ledger_consolidations
		WHERE tenant_id = $1 AND id = $2
	`

	row := r.pool.QueryRow(ctx, query, tenantID, id)
	c, err := scanConsolidation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get consolidation: %w", err)
	}

	r.logger.Debug("Consolidation retrieved",
		logging.F("id", c.ID),
		logging.F("tenant_id", c.TenantID))

	return c, nil
}

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanConsolidation scans a consolidation row from any scanner.
func scanConsolidation(row scanner) (*Consolidation, error) {
	c := &Consolidation{}
	var decisionsBytes, patternsBytes, metaBytes []byte

	err := row.Scan(
		&c.ID,
		&c.TenantID,
		&c.Title,
		&c.Body,
		&c.SourceEntryIDs,
		&c.SessionIDs,
		&decisionsBytes,
		&patternsBytes,
		&c.TimeStart,
		&c.TimeEnd,
		&c.ModelID,
		&metaBytes,
		&c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(decisionsBytes, &c.Decisions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decisions: %w", err)
	}
	if err := json.Unmarshal(patternsBytes, &c.Patterns); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patterns: %w", err)
	}
	if err := json.Unmarshal(metaBytes, &c.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	if c.Decisions == nil {
		c.Decisions = []ConsolidationDecision{}
	}
	if c.Patterns == nil {
		c.Patterns = []ConsolidationPattern{}
	}
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	}

	return c, nil
}
