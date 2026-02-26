package topics

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors - aliases to centralized errors for backward compatibility.
var (
	ErrNotFound = pferrors.ErrNotFound
	ErrConflict = pferrors.ErrConflict
)

// Repository provides database operations for topics.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new topic repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "topic_repository")),
	}
}

// Create creates a new topic.
func (r *Repository) Create(ctx context.Context, t *Topic) error {
	query := `
		INSERT INTO topics (
			tenant_id, name, description, keywords,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	keywords := t.Keywords
	if keywords == nil {
		keywords = []string{}
	}

	err := r.pool.QueryRow(ctx, query,
		t.TenantID,
		t.Name,
		t.Description,
		keywords,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "topics_tenant_name_unique") {
			return fmt.Errorf("%w: topic name '%s' already exists", ErrConflict, t.Name)
		}
		return fmt.Errorf("failed to create topic: %w", err)
	}

	r.logger.Debug("Topic created",
		logging.F("id", t.ID),
		logging.F("name", t.Name))

	return nil
}

// GetByID retrieves a topic by ID.
func (r *Repository) GetByID(ctx context.Context, id int64) (*Topic, error) {
	query := `
		SELECT id, tenant_id, name, description, keywords, created_at, updated_at
		FROM topics
		WHERE id = $1
	`
	return r.scanTopic(ctx, query, id)
}

// GetByName retrieves a topic by name within a tenant.
func (r *Repository) GetByName(ctx context.Context, tenantID, name string) (*Topic, error) {
	query := `
		SELECT id, tenant_id, name, description, keywords, created_at, updated_at
		FROM topics
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
	`
	return r.scanTopic(ctx, query, tenantID, name)
}

// Resolve resolves a topic by ID or name.
func (r *Repository) Resolve(ctx context.Context, tenantID, identifier string) (*Topic, error) {
	// Try by numeric ID first
	var id int64
	if _, err := fmt.Sscanf(identifier, "%d", &id); err == nil {
		t, err := r.GetByID(ctx, id)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	// Try by name
	return r.GetByName(ctx, tenantID, identifier)
}

// Update updates an existing topic.
func (r *Repository) Update(ctx context.Context, t *Topic) error {
	keywords := t.Keywords
	if keywords == nil {
		keywords = []string{}
	}

	query := `
		UPDATE topics SET
			name = $2,
			description = $3,
			keywords = $4,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		t.ID,
		t.Name,
		t.Description,
		keywords,
	).Scan(&t.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if strings.Contains(err.Error(), "topics_tenant_name_unique") {
			return fmt.Errorf("%w: topic name '%s' already exists", ErrConflict, t.Name)
		}
		return fmt.Errorf("failed to update topic: %w", err)
	}

	return nil
}

// Delete deletes a topic by ID.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM topics WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete topic: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lists topics with optional filtering.
func (r *Repository) List(ctx context.Context, filter TopicFilter) ([]*Topic, error) {
	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, filter.TenantID)
	argIdx++

	if filter.NameSearch != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(name) LIKE '%%' || LOWER($%d) || '%%'", argIdx))
		args = append(args, filter.NameSearch)
		argIdx++
	}

	if filter.Keyword != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(keywords)", argIdx))
		args = append(args, filter.Keyword)
		argIdx++
	}

	limit := 100
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
	}

	query := fmt.Sprintf(`
		SELECT id, tenant_id, name, description, keywords, created_at, updated_at
		FROM topics
		WHERE %s
		ORDER BY name
		LIMIT %d
	`, strings.Join(conditions, " AND "), limit)

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}
	defer rows.Close()

	return r.scanTopics(rows)
}

// ResolveByKeyword resolves a topic by keyword match within a tenant.
func (r *Repository) ResolveByKeyword(ctx context.Context, tenantID, keyword string) (*int64, error) {
	query := `
		SELECT id FROM topics
		WHERE tenant_id = $1 AND LOWER($2) = ANY(SELECT LOWER(unnest(keywords)))
		LIMIT 1
	`
	var id int64
	err := r.pool.QueryRow(ctx, query, tenantID, keyword).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to resolve topic by keyword: %w", err)
	}
	return &id, nil
}

// ListForContext returns topics with their descriptions for context enrichment.
// Used by the pipeline to add topic descriptions to the context package.
// Matches topics by exact name OR by keyword containment (any topic keyword
// found as a substring within any candidate name).
func (r *Repository) ListForContext(ctx context.Context, tenantID string, names []string) ([]*Topic, error) {
	if len(names) == 0 {
		return nil, nil
	}

	query := `
		SELECT id, tenant_id, name, description, keywords, created_at, updated_at
		FROM topics
		WHERE tenant_id = $1 AND (
			LOWER(name) = ANY($2)
			OR EXISTS (
				SELECT 1 FROM unnest(keywords) AS kw
				WHERE EXISTS (
					SELECT 1 FROM unnest($2::text[]) AS candidate
					WHERE candidate LIKE '%' || LOWER(kw) || '%'
				)
			)
		)
	`

	lowerNames := make([]string, len(names))
	for i, name := range names {
		lowerNames[i] = strings.ToLower(name)
	}

	rows, err := r.pool.Query(ctx, query, tenantID, lowerNames)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics for context: %w", err)
	}
	defer rows.Close()

	return r.scanTopics(rows)
}

// ==================== Helper Functions ====================

func (r *Repository) scanTopic(ctx context.Context, query string, args ...any) (*Topic, error) {
	t := &Topic{}
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&t.ID,
		&t.TenantID,
		&t.Name,
		&t.Description,
		&t.Keywords,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan topic: %w", err)
	}
	return t, nil
}

func (r *Repository) scanTopics(rows pgx.Rows) ([]*Topic, error) {
	var topics []*Topic
	for rows.Next() {
		t := &Topic{}
		err := rows.Scan(
			&t.ID,
			&t.TenantID,
			&t.Name,
			&t.Description,
			&t.Keywords,
			&t.CreatedAt,
			&t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}
