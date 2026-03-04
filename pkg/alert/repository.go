// Package alert provides the repository layer for alert management.
package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Alert represents an alert record.
type Alert struct {
	ID              string
	TenantID        string
	ProjectID       int64
	InstructionID   int64
	Severity        string
	Title           string
	Body            string
	SourceID        *int64
	Acknowledged    bool
	AcknowledgedAt  *time.Time
	AcknowledgedBy  *string
	CreatedAt       time.Time
	ProjectName     string // joined from projects table
	InstructionName string // joined from watch_instructions table
}

// Repository provides database operations for alerts.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new alert repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// List returns alerts for a tenant with optional filters.
func (r *Repository) List(ctx context.Context, tenantID string, projectID int64, unackedOnly bool, severity string, limit int32) ([]*Alert, error) {
	if limit <= 0 {
		limit = 20
	}

	conditions := []string{"a.tenant_id = $1"}
	args := []interface{}{tenantID}
	argIdx := 2

	if projectID > 0 {
		conditions = append(conditions, fmt.Sprintf("a.project_id = $%d", argIdx))
		args = append(args, projectID)
		argIdx++
	}

	if unackedOnly {
		conditions = append(conditions, "a.acknowledged = false")
	}

	if severity != "" {
		conditions = append(conditions, fmt.Sprintf("a.severity = $%d", argIdx))
		args = append(args, severity)
		argIdx++
	}

	where := conditions[0]
	for _, c := range conditions[1:] {
		where += " AND " + c
	}

	query := fmt.Sprintf(`
		SELECT a.id, a.tenant_id, a.project_id, a.instruction_id, a.severity,
			a.title, COALESCE(a.body, ''), a.acknowledged, a.created_at,
			COALESCE(p.name, ''), COALESCE(wi.name, '')
		FROM alerts a
		LEFT JOIN projects p ON p.id = a.project_id
		LEFT JOIN watch_instructions wi ON wi.id = a.instruction_id
		WHERE %s
		ORDER BY a.created_at DESC
		LIMIT $%d
	`, where, argIdx)

	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		a := &Alert{}
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.ProjectID, &a.InstructionID, &a.Severity,
			&a.Title, &a.Body, &a.Acknowledged, &a.CreatedAt,
			&a.ProjectName, &a.InstructionName,
		); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list alerts rows: %w", err)
	}

	return alerts, nil
}

// Acknowledge marks an alert as acknowledged.
func (r *Repository) Acknowledge(ctx context.Context, tenantID, alertID string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE alerts
		SET acknowledged = true, acknowledged_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, alertID, tenantID)
	if err != nil {
		return fmt.Errorf("acknowledge alert: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found: %s", alertID)
	}
	return nil
}

// ResolveProjectID looks up the project ID by name for a tenant.
func (r *Repository) ResolveProjectID(ctx context.Context, tenantID, projectName string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM projects
		WHERE tenant_id = $1 AND name = $2
	`, tenantID, projectName).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("resolve project %q: %w", projectName, err)
	}
	return id, nil
}
