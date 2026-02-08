package watchlist

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors.
var (
	// ErrNotFound is returned when a watch item is not found.
	ErrNotFound = pferrors.ErrNotFound
)

// Repository provides database operations for watch list management.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new watchlist repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "watchlist_repository")),
	}
}

// ==================== Watch List Operations ====================

// AddItem adds a watch list item.
func (r *Repository) AddItem(ctx context.Context, item *WatchItem) error {
	// Validate that at least one of assertion_root_id or project_id is set
	if item.AssertionRootID == nil && item.ProjectID == nil {
		return fmt.Errorf("at least one of assertion_root_id or project_id must be set")
	}

	query := `
		INSERT INTO watch_list (user_id, assertion_root_id, project_id, notes, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
		item.UserID,
		item.AssertionRootID,
		item.ProjectID,
		item.Notes,
	).Scan(&item.ID, &item.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to add watch item: %w", err)
	}

	r.logger.Debug("Watch item added",
		logging.F("id", item.ID),
		logging.F("user_id", item.UserID))

	return nil
}

// RemoveItem removes a watch list item.
func (r *Repository) RemoveItem(ctx context.Context, id int64) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM watch_list WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to remove watch item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListItems lists watch items for a user, optionally filtered by project.
func (r *Repository) ListItems(ctx context.Context, userID string, projectID *int64) ([]*WatchItem, error) {
	query := `
		SELECT
			w.id,
			w.user_id,
			w.assertion_root_id,
			w.project_id,
			w.notes,
			w.created_at,
			COALESCE(a.description, '') as assertion_description,
			COALESCE(p.name, '') as project_name
		FROM watch_list w
		LEFT JOIN assertions a ON w.assertion_root_id = a.assertion_root_id AND a.is_current = true
		LEFT JOIN projects p ON w.project_id = p.id
		WHERE w.user_id = $1
	`

	var args []any
	args = append(args, userID)

	if projectID != nil {
		query += " AND w.project_id = $2"
		args = append(args, *projectID)
	}

	query += " ORDER BY w.created_at DESC LIMIT 1000"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list watch items: %w", err)
	}
	defer rows.Close()

	var items []*WatchItem
	for rows.Next() {
		item := &WatchItem{}
		err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.AssertionRootID,
			&item.ProjectID,
			&item.Notes,
			&item.CreatedAt,
			&item.AssertionDescription,
			&item.ProjectName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan watch item: %w", err)
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// UpdateItem updates a watch item's notes.
func (r *Repository) UpdateItem(ctx context.Context, id int64, notes string) (*WatchItem, error) {
	query := `
		UPDATE watch_list
		SET notes = $2
		WHERE id = $1
		RETURNING id, user_id, assertion_root_id, project_id, notes, created_at
	`

	item := &WatchItem{}
	err := r.pool.QueryRow(ctx, query, id, notes).Scan(
		&item.ID,
		&item.UserID,
		&item.AssertionRootID,
		&item.ProjectID,
		&item.Notes,
		&item.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update watch item: %w", err)
	}

	return item, nil
}

// ==================== Trust Management ====================

// SetTrust sets trust level and domains for a person.
func (r *Repository) SetTrust(ctx context.Context, personID int64, trustLevel int32, trustDomains []string) (*PersonTrust, error) {
	// Validate trust level
	if trustLevel < 0 || trustLevel > 5 {
		return nil, fmt.Errorf("trust_level must be between 0 and 5, got %d", trustLevel)
	}

	// Handle nil slice for postgres array types
	if trustDomains == nil {
		trustDomains = []string{}
	}

	query := `
		UPDATE people
		SET trust_level = $2, trust_domains = $3
		WHERE id = $1
		RETURNING id, canonical_name, trust_level, trust_domains
	`

	person := &PersonTrust{}
	err := r.pool.QueryRow(ctx, query, personID, trustLevel, trustDomains).Scan(
		&person.ID,
		&person.Name,
		&person.TrustLevel,
		&person.TrustDomains,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to set trust: %w", err)
	}

	r.logger.Debug("Trust set",
		logging.F("person_id", personID),
		logging.F("trust_level", trustLevel))

	return person, nil
}

// ==================== Seniority Management ====================

// SetSeniority sets seniority tier for a person.
func (r *Repository) SetSeniority(ctx context.Context, personID int64, seniorityTier int32) (*PersonSeniority, error) {
	// Validate seniority tier
	if seniorityTier < 1 || seniorityTier > 7 {
		return nil, fmt.Errorf("seniority_tier must be between 1 and 7, got %d", seniorityTier)
	}

	query := `
		UPDATE people
		SET seniority_tier = $2
		WHERE id = $1
		RETURNING id, canonical_name, seniority_tier, COALESCE(job_title, '')
	`

	person := &PersonSeniority{}
	err := r.pool.QueryRow(ctx, query, personID, seniorityTier).Scan(
		&person.ID,
		&person.Name,
		&person.SeniorityTier,
		&person.Title,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to set seniority: %w", err)
	}

	r.logger.Debug("Seniority set",
		logging.F("person_id", personID),
		logging.F("seniority_tier", seniorityTier))

	return person, nil
}

// ==================== Briefing Query ====================

// GetBriefingAssertions returns prioritized assertions for briefing.
// Priority tiers:
// 1 = watched (in user's watch list)
// 2 = trusted (trust_level >= 3, with domain refinement)
// 3 = senior (seniority_tier >= 5)
// 4 = other
func (r *Repository) GetBriefingAssertions(ctx context.Context, userID string, projectID int64, limit int32) ([]*BriefingAssertion, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT
			a.id,
			a.type,
			a.description,
			a.severity,
			COALESCE(a.lifecycle_event, '') as lifecycle_event,
			p.canonical_name AS owner_name,
			COALESCE(p.seniority_tier, 0) as seniority_tier,
			COALESCE(p.trust_level, 0) as trust_level,
			w.id IS NOT NULL AS is_watched,
			CASE
				WHEN w.id IS NOT NULL THEN 1
				WHEN p.trust_level >= 3 AND (
					p.trust_domains IS NULL OR
					array_length(p.trust_domains, 1) IS NULL OR
					a.type = ANY(p.trust_domains)
				) THEN 2
				WHEN p.seniority_tier >= 5 THEN 3
				ELSE 4
			END AS priority_tier,
			a.updated_at
		FROM assertions a
		JOIN people p ON p.id = a.owner_person_id
		LEFT JOIN watch_list w ON w.assertion_root_id = a.assertion_root_id AND w.user_id = $1
		WHERE a.project_id = $2 AND a.is_current = true
		ORDER BY priority_tier ASC,
			CASE a.severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
				ELSE 5
			END ASC,
			a.updated_at DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, userID, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get briefing assertions: %w", err)
	}
	defer rows.Close()

	var assertions []*BriefingAssertion
	for rows.Next() {
		a := &BriefingAssertion{}
		err := rows.Scan(
			&a.ID,
			&a.Type,
			&a.Description,
			&a.Severity,
			&a.LifecycleEvent,
			&a.OwnerName,
			&a.SeniorityTier,
			&a.TrustLevel,
			&a.IsWatched,
			&a.PriorityTier,
			&a.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan briefing assertion: %w", err)
		}
		assertions = append(assertions, a)
	}

	return assertions, rows.Err()
}

// ==================== Seniority Escalation ====================

// GetSeniorityEscalations detects seniority escalations for assertions
// by comparing max seniority of previous participants vs current source participants.
func (r *Repository) GetSeniorityEscalations(ctx context.Context, sourceID int64) ([]*SeniorityEscalation, error) {
	query := `
		WITH current_participants AS (
			SELECT DISTINCT sp.assertion_root_id, sp.person_id
			FROM source_participants sp
			WHERE sp.source_id = $1
		),
		current_max AS (
			SELECT
				cp.assertion_root_id,
				MAX(COALESCE(p.seniority_tier, 0)) as max_seniority
			FROM current_participants cp
			JOIN people p ON p.id = cp.person_id
			GROUP BY cp.assertion_root_id
		),
		previous_max AS (
			SELECT
				sp.assertion_root_id,
				MAX(COALESCE(p.seniority_tier, 0)) as max_seniority
			FROM source_participants sp
			JOIN people p ON p.id = sp.person_id
			WHERE sp.source_id != $1
				AND sp.assertion_root_id IN (SELECT assertion_root_id FROM current_participants)
			GROUP BY sp.assertion_root_id
		)
		SELECT
			cm.assertion_root_id,
			COALESCE(pm.max_seniority, 0) as previous_max_seniority,
			cm.max_seniority as current_max_seniority,
			COALESCE(a.description, '') as assertion_description
		FROM current_max cm
		LEFT JOIN previous_max pm ON pm.assertion_root_id = cm.assertion_root_id
		LEFT JOIN assertions a ON a.assertion_root_id = cm.assertion_root_id AND a.is_current = true
		WHERE cm.max_seniority > COALESCE(pm.max_seniority, 0)
		ORDER BY (cm.max_seniority - COALESCE(pm.max_seniority, 0)) DESC
		LIMIT 100
	`

	rows, err := r.pool.Query(ctx, query, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get seniority escalations: %w", err)
	}
	defer rows.Close()

	var escalations []*SeniorityEscalation
	for rows.Next() {
		e := &SeniorityEscalation{}
		err := rows.Scan(
			&e.AssertionRootID,
			&e.PreviousMaxSeniority,
			&e.CurrentMaxSeniority,
			&e.AssertionDescription,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan seniority escalation: %w", err)
		}
		escalations = append(escalations, e)
	}

	return escalations, rows.Err()
}
