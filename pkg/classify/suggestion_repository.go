// Package classify repository
package classify

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ClassificationSuggestionRepository implements classification suggestion data access using pgxpool.
type ClassificationSuggestionRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewClassificationSuggestionRepository creates a new ClassificationSuggestionRepository.
func NewClassificationSuggestionRepository(pool *pgxpool.Pool, logger logging.Logger) *ClassificationSuggestionRepository {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &ClassificationSuggestionRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "classification_suggestion_repository")),
	}
}

// CreateSuggestion creates a new classification suggestion.
func (r *ClassificationSuggestionRepository) CreateSuggestion(ctx context.Context, suggestion *ClassificationSuggestion) error {
	query := `
		INSERT INTO classification_suggestions (
			content_id,
			source_system,
			pattern,
			confidence,
			status,
			created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
		suggestion.ContentID,
		suggestion.SourceSystem,
		suggestion.Pattern,
		suggestion.Confidence,
		suggestion.Status,
	).Scan(&suggestion.ID, &suggestion.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create classification suggestion: %w", err)
	}

	return nil
}

// ListPending retrieves all classification suggestions with status='pending'.
func (r *ClassificationSuggestionRepository) ListPending(ctx context.Context) ([]*ClassificationSuggestion, error) {
	query := `
		SELECT
			id,
			content_id,
			source_system,
			pattern,
			confidence,
			status,
			created_at
		FROM classification_suggestions
		WHERE status = 'pending'
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending suggestions: %w", err)
	}
	defer rows.Close()

	var suggestions []*ClassificationSuggestion
	for rows.Next() {
		var s ClassificationSuggestion
		err := rows.Scan(
			&s.ID,
			&s.ContentID,
			&s.SourceSystem,
			&s.Pattern,
			&s.Confidence,
			&s.Status,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan suggestion: %w", err)
		}
		suggestions = append(suggestions, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating suggestions: %w", err)
	}

	return suggestions, nil
}

// UpdateStatus updates the status of a classification suggestion.
func (r *ClassificationSuggestionRepository) UpdateStatus(ctx context.Context, id int, status string) error {
	query := `
		UPDATE classification_suggestions
		SET status = $1
		WHERE id = $2
	`

	result, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update suggestion status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("suggestion with id %d not found", id)
	}

	return nil
}

// GetByContentID retrieves all classification suggestions for a given content ID.
func (r *ClassificationSuggestionRepository) GetByContentID(ctx context.Context, contentID string) ([]*ClassificationSuggestion, error) {
	query := `
		SELECT
			id,
			content_id,
			source_system,
			pattern,
			confidence,
			status,
			created_at
		FROM classification_suggestions
		WHERE content_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, contentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query suggestions by content_id: %w", err)
	}
	defer rows.Close()

	var suggestions []*ClassificationSuggestion
	for rows.Next() {
		var s ClassificationSuggestion
		err := rows.Scan(
			&s.ID,
			&s.ContentID,
			&s.SourceSystem,
			&s.Pattern,
			&s.Confidence,
			&s.Status,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan suggestion: %w", err)
		}
		suggestions = append(suggestions, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating suggestions: %w", err)
	}

	return suggestions, nil
}
