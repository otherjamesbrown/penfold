package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AutomationRuleRepository defines data access for automation rules and their executions.
type AutomationRuleRepository interface {
	Create(ctx context.Context, rule *AutomationRule) error
	Get(ctx context.Context, id uuid.UUID) (*AutomationRule, error)
	GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*AutomationRule, error)
	List(ctx context.Context, tenantID uuid.UUID, opts ListRuleOpts) ([]*AutomationRule, error)
	Update(ctx context.Context, rule *AutomationRule) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListEnabledByTriggerType(ctx context.Context, tenantID uuid.UUID, triggerType string) ([]*AutomationRule, error)

	// Execution history
	RecordExecution(ctx context.Context, exec *AutomationRuleExecution) error
	UpdateExecution(ctx context.Context, exec *AutomationRuleExecution) error
	ListExecutions(ctx context.Context, ruleID uuid.UUID, limit int) ([]*AutomationRuleExecution, error)
}

// PostgresRepository is the PostgreSQL implementation of AutomationRuleRepository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Compile-time check that PostgresRepository satisfies the interface.
var _ AutomationRuleRepository = (*PostgresRepository)(nil)

// Create inserts a new automation rule and populates id, created_at, updated_at.
func (r *PostgresRepository) Create(ctx context.Context, rule *AutomationRule) error {
	triggerJSON, err := json.Marshal(rule.TriggerConfig)
	if err != nil {
		return fmt.Errorf("marshal trigger_config: %w", err)
	}
	selectorJSON, err := json.Marshal(rule.SelectorConfig)
	if err != nil {
		return fmt.Errorf("marshal selector_config: %w", err)
	}
	outputJSON, err := json.Marshal(rule.OutputConfig)
	if err != nil {
		return fmt.Errorf("marshal output_config: %w", err)
	}

	query := `
		INSERT INTO automation_rules (
			tenant_id, name, description, enabled,
			trigger_type, trigger_config,
			selector_config, skill_name, output_config,
			created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at
	`

	return r.pool.QueryRow(ctx, query,
		rule.TenantID, rule.Name, rule.Description, rule.Enabled,
		rule.TriggerType, triggerJSON,
		selectorJSON, rule.SkillName, outputJSON,
		rule.CreatedBy,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

// Get retrieves a single automation rule by ID.
func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (*AutomationRule, error) {
	query := `
		SELECT id, tenant_id, name, description, enabled,
		       trigger_type, trigger_config,
		       selector_config, skill_name, output_config,
		       created_at, updated_at, created_by
		FROM automation_rules
		WHERE id = $1
	`
	rule, err := r.scanRule(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("automation rule not found: %w", err)
		}
		return nil, fmt.Errorf("get automation rule: %w", err)
	}
	return rule, nil
}

// GetByName retrieves an automation rule by tenant and name.
func (r *PostgresRepository) GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*AutomationRule, error) {
	query := `
		SELECT id, tenant_id, name, description, enabled,
		       trigger_type, trigger_config,
		       selector_config, skill_name, output_config,
		       created_at, updated_at, created_by
		FROM automation_rules
		WHERE tenant_id = $1 AND name = $2
	`
	rule, err := r.scanRule(r.pool.QueryRow(ctx, query, tenantID, name))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("automation rule not found: %w", err)
		}
		return nil, fmt.Errorf("get automation rule by name: %w", err)
	}
	return rule, nil
}

// List retrieves automation rules for a tenant with optional filters.
func (r *PostgresRepository) List(ctx context.Context, tenantID uuid.UUID, opts ListRuleOpts) ([]*AutomationRule, error) {
	conds := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	argIdx := 2

	if opts.Enabled != nil {
		conds = append(conds, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *opts.Enabled)
		argIdx++
	}
	if opts.TriggerType != "" {
		conds = append(conds, fmt.Sprintf("trigger_type = $%d", argIdx))
		args = append(args, opts.TriggerType)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT id, tenant_id, name, description, enabled,
		       trigger_type, trigger_config,
		       selector_config, skill_name, output_config,
		       created_at, updated_at, created_by
		FROM automation_rules
		WHERE %s
		ORDER BY name
	`, strings.Join(conds, " AND "))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	defer rows.Close()

	var rules []*AutomationRule
	for rows.Next() {
		rule, err := r.scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan automation rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate automation rules: %w", err)
	}
	return rules, nil
}

// Update replaces the mutable fields of an automation rule.
func (r *PostgresRepository) Update(ctx context.Context, rule *AutomationRule) error {
	triggerJSON, err := json.Marshal(rule.TriggerConfig)
	if err != nil {
		return fmt.Errorf("marshal trigger_config: %w", err)
	}
	selectorJSON, err := json.Marshal(rule.SelectorConfig)
	if err != nil {
		return fmt.Errorf("marshal selector_config: %w", err)
	}
	outputJSON, err := json.Marshal(rule.OutputConfig)
	if err != nil {
		return fmt.Errorf("marshal output_config: %w", err)
	}

	query := `
		UPDATE automation_rules
		SET name = $2,
		    description = $3,
		    enabled = $4,
		    trigger_type = $5,
		    trigger_config = $6,
		    selector_config = $7,
		    skill_name = $8,
		    output_config = $9,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`

	err = r.pool.QueryRow(ctx, query,
		rule.ID, rule.Name, rule.Description, rule.Enabled,
		rule.TriggerType, triggerJSON,
		selectorJSON, rule.SkillName, outputJSON,
	).Scan(&rule.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("automation rule not found: %w", err)
		}
		return fmt.Errorf("update automation rule: %w", err)
	}
	return nil
}

// Delete removes an automation rule by ID.
func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM automation_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete automation rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("automation rule not found")
	}
	return nil
}

// ListEnabledByTriggerType retrieves all enabled rules for a tenant with a given trigger type.
func (r *PostgresRepository) ListEnabledByTriggerType(ctx context.Context, tenantID uuid.UUID, triggerType string) ([]*AutomationRule, error) {
	query := `
		SELECT id, tenant_id, name, description, enabled,
		       trigger_type, trigger_config,
		       selector_config, skill_name, output_config,
		       created_at, updated_at, created_by
		FROM automation_rules
		WHERE tenant_id = $1 AND trigger_type = $2 AND enabled = true
		ORDER BY name
	`
	rows, err := r.pool.Query(ctx, query, tenantID, triggerType)
	if err != nil {
		return nil, fmt.Errorf("list enabled rules by trigger type: %w", err)
	}
	defer rows.Close()

	var rules []*AutomationRule
	for rows.Next() {
		rule, err := r.scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan automation rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate automation rules: %w", err)
	}
	return rules, nil
}

// RecordExecution inserts a new execution record and populates id and created_at.
func (r *PostgresRepository) RecordExecution(ctx context.Context, exec *AutomationRuleExecution) error {
	deliveryJSON, err := marshalNullable(exec.DeliveryStatus)
	if err != nil {
		return fmt.Errorf("marshal delivery_status: %w", err)
	}
	chainJSON, err := marshalNullable(exec.ChainExecutions)
	if err != nil {
		return fmt.Errorf("marshal chain_executions: %w", err)
	}

	query := `
		INSERT INTO automation_rule_executions (
			rule_id, trigger_type, trigger_source,
			started_at, status,
			items_selected, skill_tokens_used,
			delivery_status, chain_executions, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`

	return r.pool.QueryRow(ctx, query,
		exec.RuleID, exec.TriggerType, nullString(exec.TriggerSource),
		exec.StartedAt, exec.Status,
		exec.ItemsSelected, exec.SkillTokensUsed,
		deliveryJSON, chainJSON, nullString(exec.Error),
	).Scan(&exec.ID, &exec.CreatedAt)
}

// UpdateExecution updates status, completion time, and result fields of an execution.
func (r *PostgresRepository) UpdateExecution(ctx context.Context, exec *AutomationRuleExecution) error {
	deliveryJSON, err := marshalNullable(exec.DeliveryStatus)
	if err != nil {
		return fmt.Errorf("marshal delivery_status: %w", err)
	}
	chainJSON, err := marshalNullable(exec.ChainExecutions)
	if err != nil {
		return fmt.Errorf("marshal chain_executions: %w", err)
	}

	query := `
		UPDATE automation_rule_executions
		SET completed_at = $2,
		    status = $3,
		    items_selected = $4,
		    skill_tokens_used = $5,
		    delivery_status = $6,
		    chain_executions = $7,
		    error = $8
		WHERE id = $1
	`

	tag, err := r.pool.Exec(ctx, query,
		exec.ID, exec.CompletedAt, exec.Status,
		exec.ItemsSelected, exec.SkillTokensUsed,
		deliveryJSON, chainJSON, nullString(exec.Error),
	)
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("execution not found")
	}
	return nil
}

// ListExecutions retrieves recent executions for a rule, newest first.
func (r *PostgresRepository) ListExecutions(ctx context.Context, ruleID uuid.UUID, limit int) ([]*AutomationRuleExecution, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, rule_id, trigger_type, trigger_source,
		       started_at, completed_at, status,
		       items_selected, skill_tokens_used,
		       delivery_status, chain_executions, error, created_at
		FROM automation_rule_executions
		WHERE rule_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, ruleID, limit)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	var execs []*AutomationRuleExecution
	for rows.Next() {
		exec, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		execs = append(execs, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate executions: %w", err)
	}
	return execs, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanRule scans a single automation rule row.
func (r *PostgresRepository) scanRule(row rowScanner) (*AutomationRule, error) {
	var (
		rule          AutomationRule
		triggerJSON   []byte
		selectorJSON  []byte
		outputJSON    []byte
		description   *string
	)

	err := row.Scan(
		&rule.ID, &rule.TenantID, &rule.Name, &description, &rule.Enabled,
		&rule.TriggerType, &triggerJSON,
		&selectorJSON, &rule.SkillName, &outputJSON,
		&rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	if description != nil {
		rule.Description = *description
	}

	if err := json.Unmarshal(triggerJSON, &rule.TriggerConfig); err != nil {
		return nil, fmt.Errorf("unmarshal trigger_config: %w", err)
	}
	if err := json.Unmarshal(selectorJSON, &rule.SelectorConfig); err != nil {
		return nil, fmt.Errorf("unmarshal selector_config: %w", err)
	}
	if err := json.Unmarshal(outputJSON, &rule.OutputConfig); err != nil {
		return nil, fmt.Errorf("unmarshal output_config: %w", err)
	}

	return &rule, nil
}

// scanExecution scans a single execution row.
func scanExecution(row rowScanner) (*AutomationRuleExecution, error) {
	var (
		exec          AutomationRuleExecution
		triggerSource *string
		deliveryJSON  []byte
		chainJSON     []byte
		errStr        *string
	)

	err := row.Scan(
		&exec.ID, &exec.RuleID, &exec.TriggerType, &triggerSource,
		&exec.StartedAt, &exec.CompletedAt, &exec.Status,
		&exec.ItemsSelected, &exec.SkillTokensUsed,
		&deliveryJSON, &chainJSON, &errStr, &exec.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if triggerSource != nil {
		exec.TriggerSource = *triggerSource
	}
	if errStr != nil {
		exec.Error = *errStr
	}

	if len(deliveryJSON) > 0 && string(deliveryJSON) != "null" {
		if err := json.Unmarshal(deliveryJSON, &exec.DeliveryStatus); err != nil {
			return nil, fmt.Errorf("unmarshal delivery_status: %w", err)
		}
	}
	if len(chainJSON) > 0 && string(chainJSON) != "null" {
		if err := json.Unmarshal(chainJSON, &exec.ChainExecutions); err != nil {
			return nil, fmt.Errorf("unmarshal chain_executions: %w", err)
		}
	}

	return &exec, nil
}

// marshalNullable marshals v to JSON, returning nil for nil/empty inputs.
func marshalNullable(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// nullString returns nil for empty strings (maps to SQL NULL).
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

