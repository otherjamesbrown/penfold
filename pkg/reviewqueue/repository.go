package reviewqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// Repository provides access to the review queue.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new review queue repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create adds a new item to the review queue.
func (r *Repository) Create(ctx context.Context, input ReviewItemInput) (*ReviewItem, error) {
	metadataJSON, _ := json.Marshal(input.Metadata)
	if input.Metadata == nil {
		metadataJSON = []byte("{}")
	}

	var item ReviewItem
	err := r.db.QueryRow(ctx, `
		INSERT INTO review_queue (
			question_type, priority, question, context,
			source_type, source_id, source_reference,
			suggested_term, suggested_expansion,
			candidate_person_ids, matched_text,
			confidence, metadata, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, 'pending')
		RETURNING id, tenant_id, question_type, priority, question, context,
			source_type, source_id, source_reference,
			suggested_term, suggested_expansion,
			candidate_person_ids, matched_text,
			status, resolution, resolved_at, resolved_by,
			confidence, metadata, created_at, updated_at
	`,
		input.QuestionType,
		input.Priority,
		input.Question,
		input.Context,
		input.SourceType,
		input.SourceID,
		input.SourceReference,
		input.SuggestedTerm,
		input.SuggestedExpansion,
		input.CandidatePersonIDs,
		input.MatchedText,
		input.Confidence,
		string(metadataJSON),
	).Scan(
		&item.ID, &item.TenantID, &item.QuestionType, &item.Priority, &item.Question, &item.Context,
		&item.SourceType, &item.SourceID, &item.SourceReference,
		&item.SuggestedTerm, &item.SuggestedExpansion,
		&item.CandidatePersonIDs, &item.MatchedText,
		&item.Status, &item.Resolution, &item.ResolvedAt, &item.ResolvedBy,
		&item.Confidence, &item.Metadata, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create review item: %w", err)
	}

	return &item, nil
}

// Get retrieves a review item by ID.
func (r *Repository) Get(ctx context.Context, id int64) (*ReviewItem, error) {
	var item ReviewItem
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, question_type, priority, question, context,
			source_type, source_id, source_reference,
			suggested_term, suggested_expansion,
			candidate_person_ids, matched_text,
			status, resolution, resolved_at, resolved_by,
			confidence, metadata, created_at, updated_at
		FROM review_queue
		WHERE id = $1
	`, id).Scan(
		&item.ID, &item.TenantID, &item.QuestionType, &item.Priority, &item.Question, &item.Context,
		&item.SourceType, &item.SourceID, &item.SourceReference,
		&item.SuggestedTerm, &item.SuggestedExpansion,
		&item.CandidatePersonIDs, &item.MatchedText,
		&item.Status, &item.Resolution, &item.ResolvedAt, &item.ResolvedBy,
		&item.Confidence, &item.Metadata, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get review item: %w", err)
	}
	return &item, nil
}

// List retrieves review items matching the filter.
func (r *Repository) List(ctx context.Context, filter ReviewFilter) ([]*ReviewItem, error) {
	query := `
		SELECT id, tenant_id, question_type, priority, question, context,
			source_type, source_id, source_reference,
			suggested_term, suggested_expansion,
			candidate_person_ids, matched_text,
			status, resolution, resolved_at, resolved_by,
			confidence, metadata, created_at, updated_at
		FROM review_queue
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	// Default to pending if no status specified
	if filter.Status == "" {
		filter.Status = StatusPending
	}

	query += fmt.Sprintf(" AND status = $%d", argNum)
	args = append(args, filter.Status)
	argNum++

	if filter.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argNum)
		args = append(args, filter.TenantID)
		argNum++
	}

	if filter.QuestionType != "" {
		query += fmt.Sprintf(" AND question_type = $%d", argNum)
		args = append(args, filter.QuestionType)
		argNum++
	}

	if filter.Priority != "" {
		query += fmt.Sprintf(" AND priority = $%d", argNum)
		args = append(args, filter.Priority)
		argNum++
	}

	if filter.SourceType != "" {
		query += fmt.Sprintf(" AND source_type = $%d", argNum)
		args = append(args, filter.SourceType)
		argNum++
	}

	// Order by priority (high first) then by created_at
	query += ` ORDER BY
		CASE priority
			WHEN 'high' THEN 1
			WHEN 'medium' THEN 2
			WHEN 'low' THEN 3
		END,
		created_at ASC`

	limit := 50
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list review items: %w", err)
	}
	defer rows.Close()

	var items []*ReviewItem
	for rows.Next() {
		var item ReviewItem
		err := rows.Scan(
			&item.ID, &item.TenantID, &item.QuestionType, &item.Priority, &item.Question, &item.Context,
			&item.SourceType, &item.SourceID, &item.SourceReference,
			&item.SuggestedTerm, &item.SuggestedExpansion,
			&item.CandidatePersonIDs, &item.MatchedText,
			&item.Status, &item.Resolution, &item.ResolvedAt, &item.ResolvedBy,
			&item.Confidence, &item.Metadata, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan review item: %w", err)
		}
		items = append(items, &item)
	}

	return items, nil
}

// Resolve marks a review item as resolved with the given resolution.
func (r *Repository) Resolve(ctx context.Context, id int64, resolution, resolvedBy string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE review_queue
		SET status = 'resolved', resolution = $2, resolved_at = NOW(), resolved_by = $3
		WHERE id = $1 AND status = 'pending'
	`, id, resolution, resolvedBy)
	if err != nil {
		return fmt.Errorf("resolve review item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}
	return nil
}

// Dismiss marks a review item as dismissed.
func (r *Repository) Dismiss(ctx context.Context, id int64, reason, dismissedBy string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE review_queue
		SET status = 'dismissed', resolution = $2, resolved_at = NOW(), resolved_by = $3
		WHERE id = $1 AND status = 'pending'
	`, id, reason, dismissedBy)
	if err != nil {
		return fmt.Errorf("dismiss review item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}
	return nil
}

// Defer marks a review item as deferred for later.
func (r *Repository) Defer(ctx context.Context, id int64) error {
	result, err := r.db.Exec(ctx, `
		UPDATE review_queue
		SET status = 'deferred'
		WHERE id = $1 AND status = 'pending'
	`, id)
	if err != nil {
		return fmt.Errorf("defer review item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}
	return nil
}

// GetNext retrieves the next pending item to review (highest priority, oldest).
func (r *Repository) GetNext(ctx context.Context, questionType QuestionType) (*ReviewItem, error) {
	filter := ReviewFilter{
		Status:       StatusPending,
		QuestionType: questionType,
		Limit:        1,
	}
	items, err := r.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items[0], nil
}

// GetStats retrieves summary statistics for the review queue.
func (r *Repository) GetStats(ctx context.Context) (*QueueStats, error) {
	stats := &QueueStats{
		ByPriority: make(map[string]int),
		ByType:     make(map[string]int),
	}

	// Total pending
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM review_queue WHERE status = 'pending'
	`).Scan(&stats.TotalPending)
	if err != nil {
		return nil, fmt.Errorf("count pending: %w", err)
	}

	// By priority
	rows, err := r.db.Query(ctx, `
		SELECT priority, COUNT(*) FROM review_queue
		WHERE status = 'pending'
		GROUP BY priority
	`)
	if err != nil {
		return nil, fmt.Errorf("count by priority: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var priority string
		var count int
		if err := rows.Scan(&priority, &count); err != nil {
			return nil, err
		}
		stats.ByPriority[priority] = count
	}

	// By type
	rows, err = r.db.Query(ctx, `
		SELECT question_type, COUNT(*) FROM review_queue
		WHERE status = 'pending'
		GROUP BY question_type
	`)
	if err != nil {
		return nil, fmt.Errorf("count by type: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var qtype string
		var count int
		if err := rows.Scan(&qtype, &count); err != nil {
			return nil, err
		}
		stats.ByType[qtype] = count
	}

	// Resolved today
	err = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM review_queue
		WHERE status = 'resolved'
		AND resolved_at >= CURRENT_DATE
	`).Scan(&stats.ResolvedToday)
	if err != nil {
		return nil, fmt.Errorf("count resolved today: %w", err)
	}

	// Oldest pending
	var oldest *time.Time
	err = r.db.QueryRow(ctx, `
		SELECT MIN(created_at) FROM review_queue WHERE status = 'pending'
	`).Scan(&oldest)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get oldest: %w", err)
	}
	stats.OldestPending = oldest

	return stats, nil
}

// normalizeAcronymTerm normalizes plural acronyms to singular form for dedup.
// Strips trailing 's' from all-uppercase acronyms like "SLIs" -> "SLI".
func normalizeAcronymTerm(term string) string {
	if len(term) < 2 {
		return term
	}

	// Check if ends with lowercase 's' and rest is all uppercase (e.g., "SLIs")
	if term[len(term)-1] == 's' {
		rest := term[:len(term)-1]
		// Only strip 's' if the rest is all uppercase (acronym pattern)
		allUpper := true
		for _, r := range rest {
			if r >= 'a' && r <= 'z' {
				allUpper = false
				break
			}
		}
		if allUpper && len(rest) >= 2 {
			return rest
		}
	}

	return term
}

// ExistsForTerm checks if a pending question already exists for a given term.
// Normalizes plural forms (SLIs -> SLI) to prevent duplicate entries.
func (r *Repository) ExistsForTerm(ctx context.Context, term string) (bool, error) {
	normalizedTerm := normalizeAcronymTerm(term)

	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM review_queue
			WHERE suggested_term = $1 AND status = 'pending'
		)
	`, normalizedTerm).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check term exists: %w", err)
	}
	return exists, nil
}

// CreateIfNotExists creates a review item only if one doesn't already exist for the term.
func (r *Repository) CreateIfNotExists(ctx context.Context, input ReviewItemInput) (*ReviewItem, bool, error) {
	if input.SuggestedTerm != "" {
		exists, err := r.ExistsForTerm(ctx, input.SuggestedTerm)
		if err != nil {
			return nil, false, err
		}
		if exists {
			return nil, false, nil // Already exists
		}
	}

	item, err := r.Create(ctx, input)
	if err != nil {
		return nil, false, err
	}
	return item, true, nil
}
