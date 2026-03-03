// Package source_mappings provides database operations for project source mappings.
package source_mappings

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// SourceMapping represents a mapping from an external source to a project.
type SourceMapping struct {
	ID               int64
	TenantID         string
	ProjectID        int64
	SourceType       string
	SourceIdentifier string
	MatchType        string
	Confidence       float32
	Notes            *string
	Enabled          bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Repository provides database operations for source mappings.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new source mappings repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "source_mappings_repository")),
	}
}

// CreateSourceMapping creates a new source-to-project mapping.
func (r *Repository) CreateSourceMapping(ctx context.Context, m *SourceMapping) error {
	query := `
		INSERT INTO project_source_mappings (
			tenant_id, project_id, source_type, source_identifier,
			match_type, confidence, notes, enabled,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, true,
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		m.TenantID,
		m.ProjectID,
		m.SourceType,
		m.SourceIdentifier,
		m.MatchType,
		m.Confidence,
		m.Notes,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "project_source_mappings_tenant_id_source_type_source_identifier") {
			return fmt.Errorf("%w: mapping for %s/%s already exists", pferrors.ErrConflict, m.SourceType, m.SourceIdentifier)
		}
		return fmt.Errorf("failed to create source mapping: %w", err)
	}

	m.Enabled = true
	return nil
}

// ListSourceMappings returns source mappings with optional filters.
func (r *Repository) ListSourceMappings(ctx context.Context, tenantID string, projectID *int64, sourceType *string) ([]*SourceMapping, error) {
	query := `
		SELECT id, tenant_id, project_id, source_type, source_identifier,
			   match_type, confidence, notes, enabled, created_at, updated_at
		FROM project_source_mappings
		WHERE tenant_id = $1
	`

	args := []any{tenantID}
	argIdx := 2

	if projectID != nil {
		query += fmt.Sprintf(" AND project_id = $%d", argIdx)
		args = append(args, *projectID)
		argIdx++
	}

	if sourceType != nil {
		query += fmt.Sprintf(" AND source_type = $%d", argIdx)
		args = append(args, *sourceType)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list source mappings: %w", err)
	}
	defer rows.Close()

	var mappings []*SourceMapping
	for rows.Next() {
		m := &SourceMapping{}
		err := rows.Scan(
			&m.ID, &m.TenantID, &m.ProjectID, &m.SourceType, &m.SourceIdentifier,
			&m.MatchType, &m.Confidence, &m.Notes, &m.Enabled, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan source mapping: %w", err)
		}
		mappings = append(mappings, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating source mappings: %w", err)
	}

	return mappings, nil
}

// UpdateSourceMapping updates an existing source mapping.
func (r *Repository) UpdateSourceMapping(ctx context.Context, tenantID string, id int64, confidence *float32, notes *string, enabled *bool) (*SourceMapping, error) {
	var setClauses []string
	var args []any
	argIdx := 1

	if confidence != nil {
		setClauses = append(setClauses, fmt.Sprintf("confidence = $%d", argIdx))
		args = append(args, *confidence)
		argIdx++
	}
	if notes != nil {
		setClauses = append(setClauses, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *notes)
		argIdx++
	}
	if enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *enabled)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE project_source_mappings
		SET %s
		WHERE id = $%d AND tenant_id = $%d
		RETURNING id, tenant_id, project_id, source_type, source_identifier,
				  match_type, confidence, notes, enabled, created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, id, tenantID)

	m := &SourceMapping{}
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&m.ID, &m.TenantID, &m.ProjectID, &m.SourceType, &m.SourceIdentifier,
		&m.MatchType, &m.Confidence, &m.Notes, &m.Enabled, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pferrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to update source mapping: %w", err)
	}

	return m, nil
}

// DeleteSourceMapping deletes a source mapping by ID.
func (r *Repository) DeleteSourceMapping(ctx context.Context, tenantID string, id int64) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM project_source_mappings
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	if err != nil {
		return fmt.Errorf("failed to delete source mapping: %w", err)
	}

	if result.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}

	return nil
}

// FindMappingForSource finds the best matching mapping for a given source.
// It checks match_type-aware lookup: exact first, then prefix, contains, regex.
func (r *Repository) FindMappingForSource(ctx context.Context, tenantID, sourceType, sourceIdentifier string) (*SourceMapping, error) {
	// Query all enabled mappings for this source type, ordered by match priority
	query := `
		SELECT id, tenant_id, project_id, source_type, source_identifier,
			   match_type, confidence, notes, enabled, created_at, updated_at
		FROM project_source_mappings
		WHERE tenant_id = $1
		  AND source_type = $2
		  AND enabled = true
		  AND (
			(match_type = 'exact' AND source_identifier = $3) OR
			(match_type = 'prefix' AND $3 LIKE source_identifier || '%') OR
			(match_type = 'contains' AND $3 LIKE '%' || source_identifier || '%') OR
			(match_type = 'regex' AND $3 ~ source_identifier)
		  )
		ORDER BY
			CASE match_type
				WHEN 'exact' THEN 1
				WHEN 'prefix' THEN 2
				WHEN 'contains' THEN 3
				WHEN 'regex' THEN 4
			END,
			confidence DESC
		LIMIT 1
	`

	m := &SourceMapping{}
	err := r.pool.QueryRow(ctx, query, tenantID, sourceType, sourceIdentifier).Scan(
		&m.ID, &m.TenantID, &m.ProjectID, &m.SourceType, &m.SourceIdentifier,
		&m.MatchType, &m.Confidence, &m.Notes, &m.Enabled, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No mapping found is not an error
		}
		return nil, fmt.Errorf("failed to find mapping for source: %w", err)
	}

	return m, nil
}
