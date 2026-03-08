// Package schedule provides the repository and Temporal wrapper for DB-driven schedule management.
package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schedule represents a DB-driven schedule definition.
type Schedule struct {
	ID             string
	TenantID       string
	Name           string
	Description    string
	ScheduleType   string
	ScheduleExpr   string
	WorkflowType   string
	WorkflowParams json.RawMessage
	Enabled        bool
	OverlapPolicy  string
	LastRunAt      *time.Time
	NextRunAt      *time.Time
	LastStatus     *string
	LastError      *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Repository provides database operations for schedules.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new schedule repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new schedule and returns the generated ID.
func (r *Repository) Create(ctx context.Context, s *Schedule) (string, error) {
	if s.WorkflowParams == nil {
		s.WorkflowParams = json.RawMessage("{}")
	}

	query := `
		INSERT INTO schedules (tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params, overlap_policy)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id string
	err := r.pool.QueryRow(ctx, query,
		s.TenantID, s.Name, s.Description, s.ScheduleType, s.ScheduleExpr,
		s.WorkflowType, s.WorkflowParams, s.OverlapPolicy,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create schedule: %w", err)
	}
	return id, nil
}

// Get returns a single schedule by ID and tenant.
func (r *Repository) Get(ctx context.Context, tenantID, id string) (*Schedule, error) {
	query := `
		SELECT id, tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params,
		       enabled, overlap_policy, last_run_at, next_run_at, last_status, last_error, created_at, updated_at
		FROM schedules
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`
	s := &Schedule{}
	err := r.pool.QueryRow(ctx, query, id, tenantID).Scan(
		&s.ID, &s.TenantID, &s.Name, &s.Description, &s.ScheduleType, &s.ScheduleExpr,
		&s.WorkflowType, &s.WorkflowParams, &s.Enabled, &s.OverlapPolicy,
		&s.LastRunAt, &s.NextRunAt, &s.LastStatus, &s.LastError, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("schedule not found")
		}
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return s, nil
}

// List returns schedules for a tenant with pagination.
func (r *Repository) List(ctx context.Context, tenantID string, limit, offset int32) ([]*Schedule, int32, error) {
	countQuery := `SELECT COUNT(*) FROM schedules WHERE tenant_id = $1 AND deleted_at IS NULL`
	var total int32
	if err := r.pool.QueryRow(ctx, countQuery, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count schedules: %w", err)
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params,
		       enabled, overlap_policy, last_run_at, next_run_at, last_status, last_error, created_at, updated_at
		FROM schedules
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY name
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()

	var schedules []*Schedule
	for rows.Next() {
		s := &Schedule{}
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.Name, &s.Description, &s.ScheduleType, &s.ScheduleExpr,
			&s.WorkflowType, &s.WorkflowParams, &s.Enabled, &s.OverlapPolicy,
			&s.LastRunAt, &s.NextRunAt, &s.LastStatus, &s.LastError, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, total, nil
}

// Update performs a partial update on a schedule.
func (r *Repository) Update(ctx context.Context, tenantID, id string, updates map[string]interface{}) error {
	// Build dynamic update query
	setClauses := "updated_at = NOW()"
	args := []interface{}{tenantID, id}
	argIdx := 3

	for col, val := range updates {
		setClauses += fmt.Sprintf(", %s = $%d", col, argIdx)
		args = append(args, val)
		argIdx++
	}

	query := fmt.Sprintf(`
		UPDATE schedules SET %s
		WHERE id = $2 AND tenant_id = $1 AND deleted_at IS NULL
	`, setClauses)

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

// Delete performs a soft delete on a schedule.
func (r *Repository) Delete(ctx context.Context, tenantID, id string) error {
	query := `UPDATE schedules SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	tag, err := r.pool.Exec(ctx, query, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

// SetEnabled updates the enabled flag for pause/resume.
func (r *Repository) SetEnabled(ctx context.Context, tenantID, id string, enabled bool) error {
	query := `UPDATE schedules SET enabled = $3, updated_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	tag, err := r.pool.Exec(ctx, query, id, tenantID, enabled)
	if err != nil {
		return fmt.Errorf("set enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

// ScheduleExecution represents a row in schedule_execution_history.
type ScheduleExecution struct {
	ID             int64
	ScheduleID     string
	TenantID       string
	WorkflowRunID  string
	Status         string // "running", "completed", "failed"
	StartedAt      time.Time
	CompletedAt    *time.Time
	Error          string
	ResultMetadata json.RawMessage
	CreatedAt      time.Time
}

// CreateExecution inserts a new execution history record. Returns the generated ID.
func (r *Repository) CreateExecution(ctx context.Context, exec *ScheduleExecution) (int64, error) {
	query := `
		INSERT INTO schedule_execution_history (schedule_id, tenant_id, workflow_run_id, status, started_at, result_metadata)
		VALUES ($1, $2::uuid, $3, $4, $5, $6)
		RETURNING id
	`
	var id int64
	err := r.pool.QueryRow(ctx, query,
		exec.ScheduleID, exec.TenantID, exec.WorkflowRunID, exec.Status, exec.StartedAt, exec.ResultMetadata,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create execution: %w", err)
	}
	return id, nil
}

// CompleteExecution updates an execution record with completion status and metadata.
func (r *Repository) CompleteExecution(ctx context.Context, id int64, status string, resultMetadata json.RawMessage, errMsg string) error {
	query := `
		UPDATE schedule_execution_history
		SET status = $2, completed_at = NOW(), result_metadata = $3, error = $4
		WHERE id = $1
	`
	tag, err := r.pool.Exec(ctx, query, id, status, resultMetadata, errMsg)
	if err != nil {
		return fmt.Errorf("complete execution: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("execution not found: %d", id)
	}
	return nil
}

// GetExecutionHistory returns recent executions for a schedule.
func (r *Repository) GetExecutionHistory(ctx context.Context, scheduleID string, limit int32) ([]*ScheduleExecution, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, schedule_id, tenant_id, workflow_run_id, status, started_at, completed_at, error, result_metadata, created_at
		FROM schedule_execution_history
		WHERE schedule_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, scheduleID, limit)
	if err != nil {
		return nil, fmt.Errorf("get execution history: %w", err)
	}
	defer rows.Close()

	var results []*ScheduleExecution
	for rows.Next() {
		e := &ScheduleExecution{}
		if err := rows.Scan(&e.ID, &e.ScheduleID, &e.TenantID, &e.WorkflowRunID, &e.Status, &e.StartedAt, &e.CompletedAt, &e.Error, &e.ResultMetadata, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		results = append(results, e)
	}
	return results, nil
}

// ListEnabled returns all enabled, non-deleted schedules for a tenant (used by reconciliation).
func (r *Repository) ListEnabled(ctx context.Context, tenantID string) ([]*Schedule, error) {
	query := `
		SELECT id, tenant_id, name, description, schedule_type, schedule_expr, workflow_type, workflow_params,
		       enabled, overlap_policy, last_run_at, next_run_at, last_status, last_error, created_at, updated_at
		FROM schedules
		WHERE tenant_id = $1 AND enabled = true AND deleted_at IS NULL
		ORDER BY name
	`
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}
	defer rows.Close()

	var schedules []*Schedule
	for rows.Next() {
		s := &Schedule{}
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.Name, &s.Description, &s.ScheduleType, &s.ScheduleExpr,
			&s.WorkflowType, &s.WorkflowParams, &s.Enabled, &s.OverlapPolicy,
			&s.LastRunAt, &s.NextRunAt, &s.LastStatus, &s.LastError, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}
