// Package instructions provides the repository layer for watch instructions management.
package instructions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// Instruction represents a watch instruction definition.
type Instruction struct {
	ID            int64
	TenantID      string
	ProjectID     *int64
	ProjectName   *string // from JOIN, nullable
	Name          string
	Instruction   string
	Priority      string
	Enabled       bool
	ModelHint     string
	CreatedBy     *string
	Version       int32
	LastMatchedAt *time.Time
	MatchCount    int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// InstructionMatch represents a recorded match between an instruction and a content item.
type InstructionMatch struct {
	ID              int64
	TenantID        string
	InstructionID   int64
	InstructionName string // from JOIN
	ContentID       string
	SourceID        int64
	Confidence      float64
	Explanation     string
	MatchedAt       time.Time
}

// ListFilter controls filtering and pagination for List queries.
type ListFilter struct {
	EnabledOnly bool
	ProjectID   int64
	ProjectName string
	Priority    string
	Limit       int32
	Offset      int32
}

// CreateInput holds the fields required to create a new instruction.
type CreateInput struct {
	Name        string
	Instruction string
	ProjectID   *int64
	Priority    string
	ModelHint   string
	CreatedBy   string
}

// UpdateInput holds the fields that may be partially updated.
type UpdateInput struct {
	Name        *string
	Instruction *string
	Priority    *string
	ModelHint   *string
	ProjectID   *int64
}

// Repository provides database operations for watch instructions.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new instructions repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// instructionSelectCols is the standard column list for instruction queries with a project JOIN.
const instructionSelectCols = `
	wi.id, wi.tenant_id, wi.project_id, p.name,
	wi.name, wi.instruction, wi.priority, wi.enabled, wi.model_hint,
	wi.created_by, wi.version, wi.last_matched_at, wi.match_count,
	wi.created_at, wi.updated_at
`

// scanInstruction scans a single instruction row produced by instructionSelectCols.
func scanInstruction(row pgx.Row) (*Instruction, error) {
	i := &Instruction{}
	err := row.Scan(
		&i.ID, &i.TenantID, &i.ProjectID, &i.ProjectName,
		&i.Name, &i.Instruction, &i.Priority, &i.Enabled, &i.ModelHint,
		&i.CreatedBy, &i.Version, &i.LastMatchedAt, &i.MatchCount,
		&i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// Create inserts a new watch instruction.
// Returns pferrors.ErrAlreadyExists if the tenant already has an instruction with the same name.
func (r *Repository) Create(ctx context.Context, tenantID string, input CreateInput) (*Instruction, error) {
	priority := input.Priority
	if priority == "" {
		priority = "normal"
	}
	modelHint := input.ModelHint
	if modelHint == "" {
		modelHint = "fast"
	}

	var createdBy *string
	if input.CreatedBy != "" {
		createdBy = &input.CreatedBy
	}

	insert := `
		INSERT INTO watch_instructions (tenant_id, project_id, name, instruction, priority, model_hint, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`
	var id int64
	err := r.pool.QueryRow(ctx, insert,
		tenantID, input.ProjectID, input.Name, input.Instruction, priority, modelHint, createdBy,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("instruction with name %q already exists for tenant: %w", input.Name, pferrors.ErrAlreadyExists)
		}
		return nil, fmt.Errorf("create instruction: %w", err)
	}

	return r.Get(ctx, tenantID, id)
}

// Get returns a single instruction by ID for the given tenant.
func (r *Repository) Get(ctx context.Context, tenantID string, id int64) (*Instruction, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM watch_instructions wi
		LEFT JOIN projects p ON p.id = wi.project_id
		WHERE wi.id = $1 AND wi.tenant_id = $2
	`, instructionSelectCols)

	inst, err := scanInstruction(r.pool.QueryRow(ctx, query, id, tenantID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pferrors.ErrNotFound
		}
		return nil, fmt.Errorf("get instruction: %w", err)
	}
	return inst, nil
}

// Update performs a partial update on an instruction, incrementing version on every change.
func (r *Repository) Update(ctx context.Context, tenantID string, id int64, input UpdateInput) (*Instruction, error) {
	setClauses := []string{"version = version + 1", "updated_at = NOW()"}
	args := []interface{}{tenantID, id}
	argIdx := 3

	if input.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *input.Name)
		argIdx++
	}
	if input.Instruction != nil {
		setClauses = append(setClauses, fmt.Sprintf("instruction = $%d", argIdx))
		args = append(args, *input.Instruction)
		argIdx++
	}
	if input.Priority != nil {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *input.Priority)
		argIdx++
	}
	if input.ModelHint != nil {
		setClauses = append(setClauses, fmt.Sprintf("model_hint = $%d", argIdx))
		args = append(args, *input.ModelHint)
		argIdx++
	}
	if input.ProjectID != nil {
		setClauses = append(setClauses, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, *input.ProjectID)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE watch_instructions
		SET %s
		WHERE tenant_id = $1 AND id = $2
	`, strings.Join(setClauses, ", "))

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("update instruction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, pferrors.ErrNotFound
	}

	return r.Get(ctx, tenantID, id)
}

// Delete hard-deletes an instruction by ID.
func (r *Repository) Delete(ctx context.Context, tenantID string, id int64) error {
	query := `DELETE FROM watch_instructions WHERE id = $1 AND tenant_id = $2`
	tag, err := r.pool.Exec(ctx, query, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete instruction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}
	return nil
}

// List returns instructions for a tenant matching the given filter.
func (r *Repository) List(ctx context.Context, tenantID string, filter ListFilter) ([]*Instruction, int32, error) {
	// If ProjectName filter is set, resolve it to a project_id first.
	if filter.ProjectName != "" && filter.ProjectID == 0 {
		var resolvedID int64
		err := r.pool.QueryRow(ctx,
			`SELECT id FROM projects WHERE tenant_id = $1 AND name = $2 LIMIT 1`,
			tenantID, filter.ProjectName,
		).Scan(&resolvedID)
		if err != nil && err != pgx.ErrNoRows {
			return nil, 0, fmt.Errorf("resolve project name: %w", err)
		}
		if err == nil {
			filter.ProjectID = resolvedID
		}
	}

	// Build WHERE conditions.
	conditions := []string{"wi.tenant_id = $1"}
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.EnabledOnly {
		conditions = append(conditions, "wi.enabled = true")
	}
	if filter.ProjectID != 0 {
		conditions = append(conditions, fmt.Sprintf("wi.project_id = $%d", argIdx))
		args = append(args, filter.ProjectID)
		argIdx++
	}
	if filter.Priority != "" {
		conditions = append(conditions, fmt.Sprintf("wi.priority = $%d", argIdx))
		args = append(args, filter.Priority)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count query.
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM watch_instructions wi
		WHERE %s
	`, where)

	var total int32
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count instructions: %w", err)
	}

	// Pagination defaults.
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	// Data query.
	dataQuery := fmt.Sprintf(`
		SELECT %s
		FROM watch_instructions wi
		LEFT JOIN projects p ON p.id = wi.project_id
		WHERE %s
		ORDER BY wi.name
		LIMIT $%d OFFSET $%d
	`, instructionSelectCols, where, argIdx, argIdx+1)

	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list instructions: %w", err)
	}
	defer rows.Close()

	var instructions []*Instruction
	for rows.Next() {
		inst, err := scanInstruction(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan instruction: %w", err)
		}
		instructions = append(instructions, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list instructions rows: %w", err)
	}

	return instructions, total, nil
}

// Enable sets an instruction's enabled flag to true.
func (r *Repository) Enable(ctx context.Context, tenantID string, id int64) (*Instruction, error) {
	return r.setEnabled(ctx, tenantID, id, true)
}

// Disable sets an instruction's enabled flag to false.
func (r *Repository) Disable(ctx context.Context, tenantID string, id int64) (*Instruction, error) {
	return r.setEnabled(ctx, tenantID, id, false)
}

func (r *Repository) setEnabled(ctx context.Context, tenantID string, id int64, enabled bool) (*Instruction, error) {
	query := `UPDATE watch_instructions SET enabled = $3, updated_at = NOW() WHERE id = $1 AND tenant_id = $2`
	tag, err := r.pool.Exec(ctx, query, id, tenantID, enabled)
	if err != nil {
		return nil, fmt.Errorf("set instruction enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, pferrors.ErrNotFound
	}
	return r.Get(ctx, tenantID, id)
}

// GetEnabledForTenant returns all enabled instructions for a tenant (used by pipeline stages).
func (r *Repository) GetEnabledForTenant(ctx context.Context, tenantID string) ([]*Instruction, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM watch_instructions wi
		LEFT JOIN projects p ON p.id = wi.project_id
		WHERE wi.tenant_id = $1 AND wi.enabled = true
		ORDER BY wi.priority DESC, wi.name
	`, instructionSelectCols)

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get enabled instructions: %w", err)
	}
	defer rows.Close()

	var instructions []*Instruction
	for rows.Next() {
		inst, err := scanInstruction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instruction: %w", err)
		}
		instructions = append(instructions, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get enabled instructions rows: %w", err)
	}

	return instructions, nil
}

// RecordMatch inserts a match record and increments the instruction's match counter atomically.
func (r *Repository) RecordMatch(ctx context.Context, tenantID string, instructionID int64, contentID string, sourceID int64, confidence float64, explanation string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	insert := `
		INSERT INTO instruction_matches (tenant_id, instruction_id, content_id, source_id, confidence, explanation)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	if _, err := tx.Exec(ctx, insert, tenantID, instructionID, contentID, sourceID, confidence, explanation); err != nil {
		return fmt.Errorf("record instruction match: %w", err)
	}

	update := `
		UPDATE watch_instructions
		SET match_count = match_count + 1, last_matched_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`
	if _, err := tx.Exec(ctx, update, instructionID, tenantID); err != nil {
		return fmt.Errorf("update instruction match stats: %w", err)
	}

	return tx.Commit(ctx)
}

// ListMatches returns paginated match records for a given instruction.
func (r *Repository) ListMatches(ctx context.Context, tenantID string, instructionID int64, limit, offset int32) ([]*InstructionMatch, int32, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM instruction_matches
		WHERE tenant_id = $1 AND instruction_id = $2
	`
	var total int32
	if err := r.pool.QueryRow(ctx, countQuery, tenantID, instructionID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count instruction matches: %w", err)
	}

	if limit <= 0 {
		limit = 100
	}

	dataQuery := `
		SELECT im.id, im.tenant_id, im.instruction_id, wi.name,
		       im.content_id, im.source_id, im.confidence, im.explanation, im.matched_at
		FROM instruction_matches im
		JOIN watch_instructions wi ON wi.id = im.instruction_id
		WHERE im.tenant_id = $1 AND im.instruction_id = $2
		ORDER BY im.matched_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.pool.Query(ctx, dataQuery, tenantID, instructionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list instruction matches: %w", err)
	}
	defer rows.Close()

	var matches []*InstructionMatch
	for rows.Next() {
		m := &InstructionMatch{}
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.InstructionID, &m.InstructionName,
			&m.ContentID, &m.SourceID, &m.Confidence, &m.Explanation, &m.MatchedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan instruction match: %w", err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list instruction matches rows: %w", err)
	}

	return matches, total, nil
}
