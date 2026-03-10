// Package digest provides the repository layer for digest management.
package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// Digest type constants.
const (
	DigestTypeDaily   = "daily"
	DigestTypeWeekly  = "weekly"
	DigestTypeJournal = "journal"

	// Rollup and journal rollup digest types.
	DigestTypeRollup          = "rollup"
	DigestTypeJournalDaily    = "journal_daily"
	DigestTypeJournalWeekly   = "journal_weekly"
	DigestTypeJournalOverview = "journal_overview"
)

// Digest represents a generated digest record.
type Digest struct {
	ID               string
	TenantID         string
	ProjectID        *int64
	DigestType       string // "daily", "weekly", "journal"
	PeriodStart      time.Time
	PeriodEnd        time.Time
	Body             json.RawMessage
	ModelUsed        string
	PromptTemplateID int64
	InputTokenCount  int
	OutputTokenCount int
	SourceContentIDs []int64
	CreatedAt        time.Time
}

// Repository provides database operations for digests.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new digest repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// digestSelectCols is the standard column list for digest queries.
const digestSelectCols = `
	id, tenant_id, project_id, digest_type,
	period_start, period_end, body, model_used, prompt_template_id,
	input_token_count, output_token_count, source_content_ids,
	created_at
`

// scanDigest scans a single digest row produced by digestSelectCols.
func scanDigest(row pgx.Row) (*Digest, error) {
	d := &Digest{}
	var sourceContentIDsRaw []byte
	err := row.Scan(
		&d.ID, &d.TenantID, &d.ProjectID, &d.DigestType,
		&d.PeriodStart, &d.PeriodEnd, &d.Body, &d.ModelUsed, &d.PromptTemplateID,
		&d.InputTokenCount, &d.OutputTokenCount, &sourceContentIDsRaw,
		&d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSONB source_content_ids — treat null or empty array as empty slice.
	d.SourceContentIDs = []int64{}
	if len(sourceContentIDsRaw) > 0 && string(sourceContentIDsRaw) != "null" {
		if err := json.Unmarshal(sourceContentIDsRaw, &d.SourceContentIDs); err != nil {
			return nil, fmt.Errorf("unmarshal source_content_ids: %w", err)
		}
	}

	return d, nil
}

// Create inserts a new digest record and returns the generated ID.
func (r *Repository) Create(ctx context.Context, digest *Digest) (string, error) {
	sourceContentIDsJSON, err := json.Marshal(digest.SourceContentIDs)
	if err != nil {
		return "", fmt.Errorf("marshal source_content_ids: %w", err)
	}

	insert := `
		INSERT INTO digests (
			tenant_id, project_id, digest_type,
			period_start, period_end, body, model_used, prompt_template_id,
			input_token_count, output_token_count, source_content_ids
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`
	var id string
	err = r.pool.QueryRow(ctx, insert,
		digest.TenantID, digest.ProjectID, digest.DigestType,
		digest.PeriodStart, digest.PeriodEnd, digest.Body, digest.ModelUsed, digest.PromptTemplateID,
		digest.InputTokenCount, digest.OutputTokenCount, sourceContentIDsJSON,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create digest: %w", err)
	}

	return id, nil
}

// Get returns a single digest by ID for the given tenant.
func (r *Repository) Get(ctx context.Context, tenantID string, digestID string) (*Digest, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM digests
		WHERE id = $1 AND tenant_id = $2
	`, digestSelectCols)

	d, err := scanDigest(r.pool.QueryRow(ctx, query, digestID, tenantID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pferrors.ErrNotFound
		}
		return nil, fmt.Errorf("get digest: %w", err)
	}
	return d, nil
}

// GetLatest returns the most recent digest for a project and type.
// Returns pferrors.ErrNotFound if no digest exists matching the criteria.
func (r *Repository) GetLatest(ctx context.Context, tenantID string, projectID int64, digestType string) (*Digest, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM digests
		WHERE tenant_id = $1 AND project_id = $2 AND digest_type = $3
		ORDER BY period_start DESC
		LIMIT 1
	`, digestSelectCols)

	d, err := scanDigest(r.pool.QueryRow(ctx, query, tenantID, projectID, digestType))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pferrors.ErrNotFound
		}
		return nil, fmt.Errorf("get latest digest: %w", err)
	}
	return d, nil
}

// List returns digests for a project within the given time range.
// If digestType is non-empty, results are filtered to that type.
// If until is zero, no upper bound is applied.
// Defaults limit to 20 if <= 0.
func (r *Repository) List(ctx context.Context, tenantID string, projectID int64, since, until time.Time, digestType string, limit int32) ([]*Digest, error) {
	if limit <= 0 {
		limit = 20
	}

	conditions := []string{
		"tenant_id = $1",
	}
	args := []interface{}{tenantID}
	argIdx := 2

	if projectID != 0 {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, projectID)
		argIdx++
	}

	conditions = append(conditions, fmt.Sprintf("period_start >= $%d", argIdx))
	args = append(args, since)
	argIdx++

	if !until.IsZero() {
		conditions = append(conditions, fmt.Sprintf("period_start <= $%d", argIdx))
		args = append(args, until)
		argIdx++
	}

	if digestType != "" {
		conditions = append(conditions, fmt.Sprintf("digest_type = $%d", argIdx))
		args = append(args, digestType)
		argIdx++
	}

	where := ""
	for i, c := range conditions {
		if i == 0 {
			where = c
		} else {
			where += " AND " + c
		}
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM digests
		WHERE %s
		ORDER BY period_start DESC
		LIMIT $%d
	`, digestSelectCols, where, argIdx)

	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list digests: %w", err)
	}
	defer rows.Close()

	digests := []*Digest{}
	for rows.Next() {
		d, err := scanDigest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan digest: %w", err)
		}
		digests = append(digests, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list digests rows: %w", err)
	}

	return digests, nil
}

// Exists reports whether a digest already exists for the given project, type, and period start.
// Used for idempotency checks before generating a new digest.
func (r *Repository) Exists(ctx context.Context, tenantID string, projectID int64, digestType string, periodStart time.Time) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM digests
			WHERE tenant_id = $1 AND project_id = $2 AND digest_type = $3 AND period_start = $4
		)
	`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, tenantID, projectID, digestType, periodStart).Scan(&exists); err != nil {
		return false, fmt.Errorf("exists digest: %w", err)
	}
	return exists, nil
}
