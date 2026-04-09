package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// Type aliases for domain types defined in the workflows package.
// These allow activities to reference them without re-defining them.
type (
	AutomationRule = workflows.AutomationRule
	SelectorConfig = workflows.SelectorConfig
	OutputChannel  = workflows.OutputChannel
	ChainRule      = workflows.ChainRule
	OutputConfig   = workflows.OutputConfig
)

// AutomationRuleExecution is a row written to automation_rule_executions.
// Only used internally by activities; not shared with the workflow.
type AutomationRuleExecution struct {
	ID              string
	RuleID          string
	TriggerType     string
	TriggerSource   string
	StartedAt       time.Time
	CompletedAt     *time.Time
	Status          string // 'running', 'completed', 'failed'
	ItemsSelected   int
	SkillTokensUsed int
	DeliveryStatus  json.RawMessage
	ChainExecutions json.RawMessage
	Error           string
}

// AutomationRuleRepository defines data access for automation rules.
type AutomationRuleRepository interface {
	// GetByID fetches an automation rule by UUID.
	GetByID(ctx context.Context, ruleID string) (*AutomationRule, error)

	// GetByName fetches an enabled automation rule by name for a tenant.
	GetByName(ctx context.Context, tenantID, name string) (*AutomationRule, error)

	// CreateExecution inserts a new execution record and returns its UUID.
	CreateExecution(ctx context.Context, exec *AutomationRuleExecution) (string, error)

	// CompleteExecution marks an execution as completed or failed.
	CompleteExecution(ctx context.Context, execID, status string, exec *AutomationRuleExecution) error
}

// PostgresAutomationRuleRepository implements AutomationRuleRepository using pgx.
type PostgresAutomationRuleRepository struct {
	db *pgxpool.Pool
}

// NewPostgresAutomationRuleRepository creates a new PostgreSQL-backed rule repository.
func NewPostgresAutomationRuleRepository(db *pgxpool.Pool) *PostgresAutomationRuleRepository {
	return &PostgresAutomationRuleRepository{db: db}
}

// GetByID fetches an automation rule by its UUID.
func (r *PostgresAutomationRuleRepository) GetByID(ctx context.Context, ruleID string) (*AutomationRule, error) {
	query := `
		SELECT id, tenant_id, name, COALESCE(description, ''), enabled,
		       trigger_type, trigger_config, selector_config, skill_name,
		       output_config, created_at, updated_at, created_by
		FROM automation_rules
		WHERE id = $1
	`
	return r.scanRule(r.db.QueryRow(ctx, query, ruleID))
}

// GetByName fetches an enabled automation rule by tenant and name.
func (r *PostgresAutomationRuleRepository) GetByName(ctx context.Context, tenantID, name string) (*AutomationRule, error) {
	query := `
		SELECT id, tenant_id, name, COALESCE(description, ''), enabled,
		       trigger_type, trigger_config, selector_config, skill_name,
		       output_config, created_at, updated_at, created_by
		FROM automation_rules
		WHERE tenant_id = $1::uuid AND name = $2 AND enabled = true
	`
	return r.scanRule(r.db.QueryRow(ctx, query, tenantID, name))
}

func (r *PostgresAutomationRuleRepository) scanRule(row pgx.Row) (*AutomationRule, error) {
	var rule AutomationRule
	err := row.Scan(
		&rule.ID, &rule.TenantID, &rule.Name, &rule.Description, &rule.Enabled,
		&rule.TriggerType, &rule.TriggerConfig, &rule.SelectorConfig, &rule.SkillName,
		&rule.OutputConfig, &rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("automation rule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan automation rule: %w", err)
	}
	return &rule, nil
}

// CreateExecution inserts a new automation_rule_executions row.
func (r *PostgresAutomationRuleRepository) CreateExecution(ctx context.Context, exec *AutomationRuleExecution) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO automation_rule_executions
			(rule_id, trigger_type, trigger_source, started_at, status)
		VALUES ($1::uuid, $2, $3, $4, 'running')
		RETURNING id
	`, exec.RuleID, exec.TriggerType, exec.TriggerSource, exec.StartedAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create automation rule execution: %w", err)
	}
	return id, nil
}

// CompleteExecution marks an execution as completed or failed with final metadata.
func (r *PostgresAutomationRuleRepository) CompleteExecution(ctx context.Context, execID, status string, exec *AutomationRuleExecution) error {
	now := time.Now().UTC()
	deliveryJSON, _ := json.Marshal(exec.DeliveryStatus)
	chainJSON, _ := json.Marshal(exec.ChainExecutions)

	_, err := r.db.Exec(ctx, `
		UPDATE automation_rule_executions
		SET status = $2,
		    completed_at = $3,
		    items_selected = $4,
		    skill_tokens_used = $5,
		    delivery_status = $6,
		    chain_executions = $7,
		    error = $8
		WHERE id = $1::uuid
	`, execID, status, now,
		exec.ItemsSelected, exec.SkillTokensUsed,
		deliveryJSON, chainJSON, exec.Error,
	)
	if err != nil {
		return fmt.Errorf("complete automation rule execution: %w", err)
	}
	return nil
}
