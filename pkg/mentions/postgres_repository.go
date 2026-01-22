// Package mentions provides unified mention resolution for all entity types.
package mentions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// PostgresRepository implements the Repository interface using PostgreSQL.
type PostgresRepository struct {
	db *sqlx.DB
}

// NewPostgresRepository creates a new PostgreSQL repository.
func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// mentionRow represents a database row for content_mentions.
type mentionRow struct {
	ID                   int64          `db:"id"`
	TenantID             string         `db:"tenant_id"`
	ContentID            int64          `db:"content_id"`
	EntityType           string         `db:"entity_type"`
	MentionedText        string         `db:"mentioned_text"`
	Position             sql.NullInt64  `db:"position"`
	ContextSnippet       sql.NullString `db:"context_snippet"`
	ResolvedEntityID     sql.NullInt64  `db:"resolved_entity_id"`
	ResolutionConfidence sql.NullFloat64 `db:"resolution_confidence"`
	ResolutionSource     sql.NullString `db:"resolution_source"`
	ResolvedExpansion    sql.NullString `db:"resolved_expansion"`
	Candidates           []byte         `db:"candidates"`
	Status               string         `db:"status"`
	ResolvedAt           sql.NullTime   `db:"resolved_at"`
	ResolvedBy           sql.NullString `db:"resolved_by"`
	ProjectContextID     sql.NullInt64  `db:"project_context_id"`
	CreatedAt            time.Time      `db:"created_at"`
}

// patternRow represents a database row for mention_patterns.
type patternRow struct {
	ID                int64          `db:"id"`
	TenantID          string         `db:"tenant_id"`
	EntityType        string         `db:"entity_type"`
	PatternText       string         `db:"pattern_text"`
	ResolvedEntityID  sql.NullInt64  `db:"resolved_entity_id"`
	ResolvedExpansion sql.NullString `db:"resolved_expansion"`
	ProjectID         sql.NullInt64  `db:"project_id"`
	IsPermanent       bool           `db:"is_permanent"`
	TimesSeen         int            `db:"times_seen"`
	TimesLinked       int            `db:"times_linked"`
	LastSeenAt        time.Time      `db:"last_seen_at"`
	LastLinkedAt      sql.NullTime   `db:"last_linked_at"`
	FirstContentID    sql.NullInt64  `db:"first_content_id"`
	CreatedAt         time.Time      `db:"created_at"`
}

// affinityRow represents a database row for entity_project_affinity.
type affinityRow struct {
	ID              int64          `db:"id"`
	TenantID        string         `db:"tenant_id"`
	EntityType      string         `db:"entity_type"`
	EntityID        int64          `db:"entity_id"`
	ProjectID       int64          `db:"project_id"`
	MentionCount    int            `db:"mention_count"`
	LastMentionedAt sql.NullTime   `db:"last_mentioned_at"`
	IsMember        bool           `db:"is_member"`
	Role            sql.NullString `db:"role"`
	AffinityScore   float32        `db:"affinity_score"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
}

// CreateMention creates a new mention record.
func (r *PostgresRepository) CreateMention(ctx context.Context, input MentionInput) (*ContentMention, error) {
	query := `
		INSERT INTO content_mentions (
			tenant_id, content_id, entity_type, mentioned_text,
			position, context_snippet, project_context_id, status, candidates
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, 'pending', '[]'::jsonb
		)
		RETURNING id, created_at
	`

	var id int64
	var createdAt time.Time

	// Get tenant from context or use default
	tenantID := getTenantFromContext(ctx)

	err := r.db.QueryRowContext(ctx, query,
		tenantID,
		input.ContentID,
		string(input.EntityType),
		input.MentionedText,
		nullIntPtr(input.Position),
		nullString(input.ContextSnippet),
		nullInt64Ptr(input.ProjectContextID),
	).Scan(&id, &createdAt)

	if err != nil {
		return nil, fmt.Errorf("creating mention: %w", err)
	}

	return &ContentMention{
		ID:               id,
		TenantID:         tenantID,
		ContentID:        input.ContentID,
		EntityType:       input.EntityType,
		MentionedText:    input.MentionedText,
		Position:         input.Position,
		ContextSnippet:   input.ContextSnippet,
		ProjectContextID: input.ProjectContextID,
		Status:           MentionStatusPending,
		Candidates:       []Candidate{},
		CreatedAt:        createdAt,
	}, nil
}

// GetMention retrieves a mention by ID.
func (r *PostgresRepository) GetMention(ctx context.Context, id int64) (*ContentMention, error) {
	query := `
		SELECT id, tenant_id, content_id, entity_type, mentioned_text,
			position, context_snippet, resolved_entity_id, resolution_confidence,
			resolution_source, resolved_expansion, candidates, status,
			resolved_at, resolved_by, project_context_id, created_at
		FROM content_mentions
		WHERE id = $1
	`

	var row mentionRow
	err := r.db.GetContext(ctx, &row, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting mention: %w", err)
	}

	return rowToMention(&row)
}

// ListMentions lists mentions based on filter criteria.
func (r *PostgresRepository) ListMentions(ctx context.Context, filter MentionFilter) ([]ContentMention, error) {
	query := `
		SELECT id, tenant_id, content_id, entity_type, mentioned_text,
			position, context_snippet, resolved_entity_id, resolution_confidence,
			resolution_source, resolved_expansion, candidates, status,
			resolved_at, resolved_by, project_context_id, created_at
		FROM content_mentions
		WHERE tenant_id = $1
	`
	args := []interface{}{filter.TenantID}
	argNum := 2

	if filter.ContentID != nil {
		query += fmt.Sprintf(" AND content_id = $%d", argNum)
		args = append(args, *filter.ContentID)
		argNum++
	}

	if filter.EntityType != nil {
		query += fmt.Sprintf(" AND entity_type = $%d", argNum)
		args = append(args, string(*filter.EntityType))
		argNum++
	}

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, string(*filter.Status))
		argNum++
	}

	if filter.ProjectID != nil {
		query += fmt.Sprintf(" AND project_context_id = $%d", argNum)
		args = append(args, *filter.ProjectID)
		argNum++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	var rows []mentionRow
	err := r.db.SelectContext(ctx, &rows, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing mentions: %w", err)
	}

	mentions := make([]ContentMention, len(rows))
	for i, row := range rows {
		m, err := rowToMention(&row)
		if err != nil {
			return nil, err
		}
		mentions[i] = *m
	}

	return mentions, nil
}

// UpdateMentionResolution updates a mention with resolution information.
func (r *PostgresRepository) UpdateMentionResolution(ctx context.Context, id int64, resolution ResolutionInput) error {
	query := `
		UPDATE content_mentions
		SET resolved_entity_id = $2,
			resolution_source = $3,
			resolution_confidence = 1.0,
			status = $4,
			resolved_at = NOW(),
			resolved_by = $5
		WHERE id = $1
	`

	status := MentionStatusUserResolved
	if resolution.Source == ResolutionSourceExactMatch ||
		resolution.Source == ResolutionSourceAlias ||
		resolution.Source == ResolutionSourceProjectContext ||
		resolution.Source == ResolutionSourcePriorLink {
		status = MentionStatusAutoResolved
	}

	_, err := r.db.ExecContext(ctx, query,
		id,
		resolution.EntityID,
		string(resolution.Source),
		string(status),
		nullString(resolution.ResolvedBy),
	)
	if err != nil {
		return fmt.Errorf("updating mention resolution: %w", err)
	}

	return nil
}

// DismissMention marks a mention as dismissed.
func (r *PostgresRepository) DismissMention(ctx context.Context, id int64, dismissal DismissalInput) error {
	query := `
		UPDATE content_mentions
		SET status = 'dismissed',
			resolved_at = NOW(),
			resolved_by = $2
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, nullString(dismissal.DismissedBy))
	if err != nil {
		return fmt.Errorf("dismissing mention: %w", err)
	}

	return nil
}

// GetPattern retrieves a pattern by text and optional project scope.
func (r *PostgresRepository) GetPattern(ctx context.Context, tenantID string, entityType EntityType, text string, projectID *int64) (*MentionPattern, error) {
	query := `
		SELECT id, tenant_id, entity_type, pattern_text, resolved_entity_id,
			resolved_expansion, project_id, is_permanent, times_seen,
			times_linked, last_seen_at, last_linked_at, first_content_id, created_at
		FROM mention_patterns
		WHERE tenant_id = $1 AND entity_type = $2 AND LOWER(pattern_text) = LOWER($3)
	`
	args := []interface{}{tenantID, string(entityType), text}

	if projectID != nil {
		query += " AND project_id = $4"
		args = append(args, *projectID)
	} else {
		query += " AND project_id IS NULL"
	}

	var row patternRow
	err := r.db.GetContext(ctx, &row, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting pattern: %w", err)
	}

	return rowToPattern(&row), nil
}

// GetPatternsByText retrieves all patterns matching text (any project scope).
func (r *PostgresRepository) GetPatternsByText(ctx context.Context, tenantID string, entityType EntityType, text string) ([]MentionPattern, error) {
	query := `
		SELECT id, tenant_id, entity_type, pattern_text, resolved_entity_id,
			resolved_expansion, project_id, is_permanent, times_seen,
			times_linked, last_seen_at, last_linked_at, first_content_id, created_at
		FROM mention_patterns
		WHERE tenant_id = $1 AND entity_type = $2 AND LOWER(pattern_text) = LOWER($3)
		ORDER BY times_linked DESC, project_id NULLS LAST
	`

	var rows []patternRow
	err := r.db.SelectContext(ctx, &rows, query, tenantID, string(entityType), text)
	if err != nil {
		return nil, fmt.Errorf("getting patterns by text: %w", err)
	}

	patterns := make([]MentionPattern, len(rows))
	for i, row := range rows {
		patterns[i] = *rowToPattern(&row)
	}

	return patterns, nil
}

// CreateOrUpdatePattern creates or updates a pattern.
func (r *PostgresRepository) CreateOrUpdatePattern(ctx context.Context, pattern *MentionPattern) error {
	query := `
		INSERT INTO mention_patterns (
			tenant_id, entity_type, pattern_text, resolved_entity_id,
			resolved_expansion, project_id, is_permanent, times_seen,
			times_linked, last_seen_at, first_content_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (tenant_id, entity_type, pattern_text, project_id)
		DO UPDATE SET
			resolved_entity_id = EXCLUDED.resolved_entity_id,
			resolved_expansion = EXCLUDED.resolved_expansion,
			is_permanent = EXCLUDED.is_permanent,
			times_seen = mention_patterns.times_seen + 1,
			last_seen_at = NOW()
		RETURNING id
	`

	err := r.db.QueryRowContext(ctx, query,
		pattern.TenantID,
		string(pattern.EntityType),
		pattern.PatternText,
		nullInt64Ptr(pattern.ResolvedEntityID),
		nullString(pattern.ResolvedExpansion),
		nullInt64Ptr(pattern.ProjectID),
		pattern.IsPermanent,
		pattern.TimesSeen,
		pattern.TimesLinked,
		pattern.LastSeenAt,
		nullInt64Ptr(pattern.FirstContentID),
	).Scan(&pattern.ID)

	if err != nil {
		return fmt.Errorf("creating/updating pattern: %w", err)
	}

	return nil
}

// IncrementPatternSeen increments the times_seen counter.
func (r *PostgresRepository) IncrementPatternSeen(ctx context.Context, id int64) error {
	query := `
		UPDATE mention_patterns
		SET times_seen = times_seen + 1, last_seen_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("incrementing pattern seen: %w", err)
	}

	return nil
}

// IncrementPatternLinked increments the times_linked counter and updates entity.
func (r *PostgresRepository) IncrementPatternLinked(ctx context.Context, id int64, entityID int64) error {
	query := `
		UPDATE mention_patterns
		SET times_linked = times_linked + 1,
			last_linked_at = NOW(),
			resolved_entity_id = $2
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, entityID)
	if err != nil {
		return fmt.Errorf("incrementing pattern linked: %w", err)
	}

	return nil
}

// GetAffinity retrieves an affinity record.
func (r *PostgresRepository) GetAffinity(ctx context.Context, tenantID string, entityType EntityType, entityID, projectID int64) (*EntityProjectAffinity, error) {
	query := `
		SELECT id, tenant_id, entity_type, entity_id, project_id,
			mention_count, last_mentioned_at, is_member, role,
			affinity_score, created_at, updated_at
		FROM entity_project_affinity
		WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3 AND project_id = $4
	`

	var row affinityRow
	err := r.db.GetContext(ctx, &row, query, tenantID, string(entityType), entityID, projectID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting affinity: %w", err)
	}

	return rowToAffinity(&row), nil
}

// GetAffinitiesForProject retrieves all affinities for a project.
func (r *PostgresRepository) GetAffinitiesForProject(ctx context.Context, tenantID string, projectID int64, entityType EntityType) ([]EntityProjectAffinity, error) {
	query := `
		SELECT id, tenant_id, entity_type, entity_id, project_id,
			mention_count, last_mentioned_at, is_member, role,
			affinity_score, created_at, updated_at
		FROM entity_project_affinity
		WHERE tenant_id = $1 AND project_id = $2 AND entity_type = $3
		ORDER BY affinity_score DESC
	`

	var rows []affinityRow
	err := r.db.SelectContext(ctx, &rows, query, tenantID, projectID, string(entityType))
	if err != nil {
		return nil, fmt.Errorf("getting affinities for project: %w", err)
	}

	affinities := make([]EntityProjectAffinity, len(rows))
	for i, row := range rows {
		affinities[i] = *rowToAffinity(&row)
	}

	return affinities, nil
}

// GetAffinitiesForEntity retrieves all affinities for an entity.
func (r *PostgresRepository) GetAffinitiesForEntity(ctx context.Context, tenantID string, entityType EntityType, entityID int64) ([]EntityProjectAffinity, error) {
	query := `
		SELECT id, tenant_id, entity_type, entity_id, project_id,
			mention_count, last_mentioned_at, is_member, role,
			affinity_score, created_at, updated_at
		FROM entity_project_affinity
		WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY affinity_score DESC
	`

	var rows []affinityRow
	err := r.db.SelectContext(ctx, &rows, query, tenantID, string(entityType), entityID)
	if err != nil {
		return nil, fmt.Errorf("getting affinities for entity: %w", err)
	}

	affinities := make([]EntityProjectAffinity, len(rows))
	for i, row := range rows {
		affinities[i] = *rowToAffinity(&row)
	}

	return affinities, nil
}

// UpsertAffinity creates or updates an affinity record.
func (r *PostgresRepository) UpsertAffinity(ctx context.Context, affinity *EntityProjectAffinity) error {
	query := `
		INSERT INTO entity_project_affinity (
			tenant_id, entity_type, entity_id, project_id,
			mention_count, last_mentioned_at, is_member, role, affinity_score
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, entity_type, entity_id, project_id)
		DO UPDATE SET
			mention_count = EXCLUDED.mention_count,
			last_mentioned_at = EXCLUDED.last_mentioned_at,
			is_member = EXCLUDED.is_member,
			role = EXCLUDED.role,
			affinity_score = EXCLUDED.affinity_score,
			updated_at = NOW()
		RETURNING id
	`

	err := r.db.QueryRowContext(ctx, query,
		affinity.TenantID,
		string(affinity.EntityType),
		affinity.EntityID,
		affinity.ProjectID,
		affinity.MentionCount,
		nullTime(affinity.LastMentionedAt),
		affinity.IsMember,
		nullString(affinity.Role),
		affinity.AffinityScore,
	).Scan(&affinity.ID)

	if err != nil {
		return fmt.Errorf("upserting affinity: %w", err)
	}

	return nil
}

// IncrementAffinityMentionCount increments the mention count and updates score.
func (r *PostgresRepository) IncrementAffinityMentionCount(ctx context.Context, tenantID string, entityType EntityType, entityID, projectID int64) error {
	// First try to update existing
	query := `
		UPDATE entity_project_affinity
		SET mention_count = mention_count + 1,
			last_mentioned_at = NOW(),
			affinity_score = LEAST(1.0, affinity_score + 0.01)
		WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3 AND project_id = $4
	`

	result, err := r.db.ExecContext(ctx, query, tenantID, string(entityType), entityID, projectID)
	if err != nil {
		return fmt.Errorf("incrementing affinity mention count: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Create new record
		insertQuery := `
			INSERT INTO entity_project_affinity (
				tenant_id, entity_type, entity_id, project_id,
				mention_count, last_mentioned_at, affinity_score
			) VALUES ($1, $2, $3, $4, 1, NOW(), 0.5)
		`
		_, err = r.db.ExecContext(ctx, insertQuery, tenantID, string(entityType), entityID, projectID)
		if err != nil {
			return fmt.Errorf("creating affinity record: %w", err)
		}
	}

	return nil
}

// GetMentionStats returns statistics about pending mentions.
func (r *PostgresRepository) GetMentionStats(ctx context.Context, tenantID string) (*MentionStats, error) {
	stats := &MentionStats{
		ByType: make(map[string]int),
	}

	// Total pending
	query := `SELECT COUNT(*) FROM content_mentions WHERE tenant_id = $1 AND status = 'pending'`
	err := r.db.GetContext(ctx, &stats.TotalPending, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting total pending: %w", err)
	}

	// By type
	typeQuery := `
		SELECT entity_type, COUNT(*) as count
		FROM content_mentions
		WHERE tenant_id = $1 AND status = 'pending'
		GROUP BY entity_type
	`
	rows, err := r.db.QueryContext(ctx, typeQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting counts by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var entityType string
		var count int
		if err := rows.Scan(&entityType, &count); err != nil {
			return nil, err
		}
		stats.ByType[entityType] = count
	}

	// Resolved today
	todayQuery := `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = $1 AND resolved_at >= CURRENT_DATE
	`
	err = r.db.GetContext(ctx, &stats.ResolvedToday, todayQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting resolved today: %w", err)
	}

	return stats, nil
}

// GetPendingCount returns the count of pending mentions.
func (r *PostgresRepository) GetPendingCount(ctx context.Context, tenantID string, entityType *EntityType) (int, error) {
	query := `SELECT COUNT(*) FROM content_mentions WHERE tenant_id = $1 AND status = 'pending'`
	args := []interface{}{tenantID}

	if entityType != nil {
		query += " AND entity_type = $2"
		args = append(args, string(*entityType))
	}

	var count int
	err := r.db.GetContext(ctx, &count, query, args...)
	if err != nil {
		return 0, fmt.Errorf("getting pending count: %w", err)
	}

	return count, nil
}

// BatchCreateMentions creates multiple mentions in a single transaction.
func (r *PostgresRepository) BatchCreateMentions(ctx context.Context, inputs []MentionInput) ([]ContentMention, error) {
	if len(inputs) == 0 {
		return []ContentMention{}, nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	tenantID := getTenantFromContext(ctx)
	mentions := make([]ContentMention, len(inputs))

	for i, input := range inputs {
		query := `
			INSERT INTO content_mentions (
				tenant_id, content_id, entity_type, mentioned_text,
				position, context_snippet, project_context_id, status, candidates
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', '[]'::jsonb)
			RETURNING id, created_at
		`

		var id int64
		var createdAt time.Time

		err := tx.QueryRowContext(ctx, query,
			tenantID,
			input.ContentID,
			string(input.EntityType),
			input.MentionedText,
			nullIntPtr(input.Position),
			nullString(input.ContextSnippet),
			nullInt64Ptr(input.ProjectContextID),
		).Scan(&id, &createdAt)

		if err != nil {
			return nil, fmt.Errorf("creating mention %d: %w", i, err)
		}

		mentions[i] = ContentMention{
			ID:               id,
			TenantID:         tenantID,
			ContentID:        input.ContentID,
			EntityType:       input.EntityType,
			MentionedText:    input.MentionedText,
			Position:         input.Position,
			ContextSnippet:   input.ContextSnippet,
			ProjectContextID: input.ProjectContextID,
			Status:           MentionStatusPending,
			Candidates:       []Candidate{},
			CreatedAt:        createdAt,
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return mentions, nil
}

// BatchResolveMentions resolves multiple mentions in a single transaction.
func (r *PostgresRepository) BatchResolveMentions(ctx context.Context, resolutions []ResolutionInput) (*BatchResolutionResult, error) {
	result := &BatchResolutionResult{}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	for _, res := range resolutions {
		query := `
			UPDATE content_mentions
			SET resolved_entity_id = $2,
				resolution_source = $3,
				resolution_confidence = 1.0,
				status = 'user_resolved',
				resolved_at = NOW(),
				resolved_by = $4
			WHERE id = $1
		`

		_, err := tx.ExecContext(ctx, query,
			res.MentionID,
			res.EntityID,
			string(res.Source),
			nullString(res.ResolvedBy),
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("resolve %d: %v", res.MentionID, err))
		} else {
			result.Resolved++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return result, nil
}

// Helper functions

func rowToMention(row *mentionRow) (*ContentMention, error) {
	m := &ContentMention{
		ID:            row.ID,
		TenantID:      row.TenantID,
		ContentID:     row.ContentID,
		EntityType:    EntityType(row.EntityType),
		MentionedText: row.MentionedText,
		Status:        MentionStatus(row.Status),
		CreatedAt:     row.CreatedAt,
	}

	if row.Position.Valid {
		pos := int(row.Position.Int64)
		m.Position = &pos
	}
	if row.ContextSnippet.Valid {
		m.ContextSnippet = row.ContextSnippet.String
	}
	if row.ResolvedEntityID.Valid {
		m.ResolvedEntityID = &row.ResolvedEntityID.Int64
	}
	if row.ResolutionConfidence.Valid {
		conf := float32(row.ResolutionConfidence.Float64)
		m.ResolutionConfidence = &conf
	}
	if row.ResolutionSource.Valid {
		m.ResolutionSource = ResolutionSource(row.ResolutionSource.String)
	}
	if row.ResolvedExpansion.Valid {
		m.ResolvedExpansion = row.ResolvedExpansion.String
	}
	if row.ResolvedAt.Valid {
		m.ResolvedAt = &row.ResolvedAt.Time
	}
	if row.ResolvedBy.Valid {
		m.ResolvedBy = row.ResolvedBy.String
	}
	if row.ProjectContextID.Valid {
		m.ProjectContextID = &row.ProjectContextID.Int64
	}

	// Parse candidates
	if len(row.Candidates) > 0 {
		if err := json.Unmarshal(row.Candidates, &m.Candidates); err != nil {
			return nil, fmt.Errorf("parsing candidates: %w", err)
		}
	}

	return m, nil
}

func rowToPattern(row *patternRow) *MentionPattern {
	p := &MentionPattern{
		ID:          row.ID,
		TenantID:    row.TenantID,
		EntityType:  EntityType(row.EntityType),
		PatternText: row.PatternText,
		IsPermanent: row.IsPermanent,
		TimesSeen:   row.TimesSeen,
		TimesLinked: row.TimesLinked,
		LastSeenAt:  row.LastSeenAt,
		CreatedAt:   row.CreatedAt,
	}

	if row.ResolvedEntityID.Valid {
		p.ResolvedEntityID = &row.ResolvedEntityID.Int64
	}
	if row.ResolvedExpansion.Valid {
		p.ResolvedExpansion = row.ResolvedExpansion.String
	}
	if row.ProjectID.Valid {
		p.ProjectID = &row.ProjectID.Int64
	}
	if row.LastLinkedAt.Valid {
		p.LastLinkedAt = &row.LastLinkedAt.Time
	}
	if row.FirstContentID.Valid {
		p.FirstContentID = &row.FirstContentID.Int64
	}

	return p
}

func rowToAffinity(row *affinityRow) *EntityProjectAffinity {
	a := &EntityProjectAffinity{
		ID:            row.ID,
		TenantID:      row.TenantID,
		EntityType:    EntityType(row.EntityType),
		EntityID:      row.EntityID,
		ProjectID:     row.ProjectID,
		MentionCount:  row.MentionCount,
		IsMember:      row.IsMember,
		AffinityScore: row.AffinityScore,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}

	if row.LastMentionedAt.Valid {
		a.LastMentionedAt = &row.LastMentionedAt.Time
	}
	if row.Role.Valid {
		a.Role = row.Role.String
	}

	return a
}

func nullInt64Ptr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func nullIntPtr(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func getTenantFromContext(ctx context.Context) string {
	// TODO: Extract from context when multi-tenant support is added
	return "00000001-0000-0000-0000-000000000001"
}
