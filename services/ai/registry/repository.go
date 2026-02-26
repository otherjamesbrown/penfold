package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository defines the interface for model storage.
type Repository interface {
	// Models
	CreateModel(ctx context.Context, model *ModelConfig) error
	GetModel(ctx context.Context, modelID string) (*ModelConfig, error)
	UpdateModel(ctx context.Context, model *ModelConfig) error
	DeleteModel(ctx context.Context, modelID string) error
	ListModels(ctx context.Context, filter *ModelFilter) ([]*ModelConfig, error)

	// Health
	GetHealth(ctx context.Context, modelID string) (*ModelHealth, error)
	UpdateHealth(ctx context.Context, modelID string, health *ModelHealth) error

	// Model lookups by short name (model_name column, not compound id)
	GetModelContextWindowByName(ctx context.Context, modelName string) (int, error)
	GetModelProviderByName(ctx context.Context, modelName string) (string, error)

	// Routing rules
	GetRoutingRules(ctx context.Context) ([]*RoutingRule, error)
	GetRoutingRulesByTask(ctx context.Context, taskType string) ([]*RoutingRule, error)
	GetRoutingRuleByName(ctx context.Context, name string) (*RoutingRule, error)
	CreateRoutingRule(ctx context.Context, rule *RoutingRule) error
	UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error
	DeleteRoutingRule(ctx context.Context, ruleID string) error
}

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresRepository creates a new PostgreSQL repository.
func NewPostgresRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresRepository {
	return &PostgresRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "ai_model_repository")),
	}
}

// CreateModel inserts a new model into the database.
func (r *PostgresRepository) CreateModel(ctx context.Context, model *ModelConfig) error {
	if err := model.Validate(); err != nil {
		return err
	}

	// Convert capabilities to string array
	caps := make([]string, len(model.Capabilities.Capabilities))
	for i, c := range model.Capabilities.Capabilities {
		caps[i] = string(c)
	}

	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(model.Metadata)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal metadata: %v", ErrDatabaseError, err)
	}

	query := `
		INSERT INTO ai_models (
			id, name, provider, model_name, endpoint,
			capabilities, context_window, max_output_tokens,
			input_cost_per_1k, output_cost_per_1k,
			is_local, is_enabled, priority, metadata,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10,
			$11, $12, $13, $14,
			NOW(), NOW()
		)
	`

	_, err = r.pool.Exec(ctx, query,
		model.ID,
		model.Name,
		string(model.Provider),
		model.ModelName,
		nullableString(model.Endpoint),
		caps,
		model.Limits.ContextWindow,
		model.Limits.MaxOutputTokens,
		model.Cost.InputCostPer1K,
		model.Cost.OutputCostPer1K,
		model.IsLocal,
		true, // is_enabled defaults to true
		model.Priority,
		metadataJSON,
	)

	if err != nil {
		r.logger.Error("Failed to create model",
			logging.Err(err),
			logging.F("model_id", model.ID))
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	r.logger.Debug("Model created",
		logging.F("model_id", model.ID),
		logging.F("provider", model.Provider))

	return nil
}

// GetModel retrieves a model by ID.
func (r *PostgresRepository) GetModel(ctx context.Context, modelID string) (*ModelConfig, error) {
	query := `
		SELECT
			id, name, provider, model_name, endpoint,
			capabilities, context_window, max_output_tokens,
			input_cost_per_1k, output_cost_per_1k,
			is_local, is_enabled, priority, metadata,
			created_at, updated_at
		FROM ai_models
		WHERE id = $1
	`

	var (
		model          ModelConfig
		provider       string
		endpoint       *string
		caps           []string
		inputCost      float64
		outputCost     float64
		isEnabled      bool
		metadataJSON   []byte
	)

	err := r.pool.QueryRow(ctx, query, modelID).Scan(
		&model.ID,
		&model.Name,
		&provider,
		&model.ModelName,
		&endpoint,
		&caps,
		&model.Limits.ContextWindow,
		&model.Limits.MaxOutputTokens,
		&inputCost,
		&outputCost,
		&model.IsLocal,
		&isEnabled,
		&model.Priority,
		&metadataJSON,
		&model.CreatedAt,
		&model.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	// Map fields
	model.Provider = Provider(provider)
	if endpoint != nil {
		model.Endpoint = *endpoint
	}
	model.Capabilities.Capabilities = make([]Capability, len(caps))
	for i, c := range caps {
		model.Capabilities.Capabilities[i] = Capability(c)
	}
	model.Cost.InputCostPer1K = inputCost
	model.Cost.OutputCostPer1K = outputCost
	model.Cost.Currency = "USD"

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &model.Metadata); err != nil {
			model.Metadata = make(map[string]interface{})
		}
	}

	return &model, nil
}

// UpdateModel updates an existing model.
func (r *PostgresRepository) UpdateModel(ctx context.Context, model *ModelConfig) error {
	if err := model.Validate(); err != nil {
		return err
	}

	// Convert capabilities to string array
	caps := make([]string, len(model.Capabilities.Capabilities))
	for i, c := range model.Capabilities.Capabilities {
		caps[i] = string(c)
	}

	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(model.Metadata)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal metadata: %v", ErrDatabaseError, err)
	}

	query := `
		UPDATE ai_models SET
			name = $2,
			provider = $3,
			model_name = $4,
			endpoint = $5,
			capabilities = $6,
			context_window = $7,
			max_output_tokens = $8,
			input_cost_per_1k = $9,
			output_cost_per_1k = $10,
			is_local = $11,
			priority = $12,
			metadata = $13,
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query,
		model.ID,
		model.Name,
		string(model.Provider),
		model.ModelName,
		nullableString(model.Endpoint),
		caps,
		model.Limits.ContextWindow,
		model.Limits.MaxOutputTokens,
		model.Cost.InputCostPer1K,
		model.Cost.OutputCostPer1K,
		model.IsLocal,
		model.Priority,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	if result.RowsAffected() == 0 {
		return ErrModelNotFound
	}

	r.logger.Debug("Model updated",
		logging.F("model_id", model.ID))

	return nil
}

// DeleteModel removes a model from the database.
func (r *PostgresRepository) DeleteModel(ctx context.Context, modelID string) error {
	query := `DELETE FROM ai_models WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, modelID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	if result.RowsAffected() == 0 {
		return ErrModelNotFound
	}

	r.logger.Debug("Model deleted",
		logging.F("model_id", modelID))

	return nil
}

// ListModels returns all models matching the optional filter.
func (r *PostgresRepository) ListModels(ctx context.Context, filter *ModelFilter) ([]*ModelConfig, error) {
	// Base query with limit to prevent memory exhaustion
	query := `
		SELECT
			id, name, provider, model_name, endpoint,
			capabilities, context_window, max_output_tokens,
			input_cost_per_1k, output_cost_per_1k,
			is_local, is_enabled, priority, metadata,
			created_at, updated_at
		FROM ai_models
		WHERE is_enabled = true
		ORDER BY priority DESC
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	var models []*ModelConfig
	for rows.Next() {
		var (
			model          ModelConfig
			provider       string
			endpoint       *string
			caps           []string
			inputCost      float64
			outputCost     float64
			isEnabled      bool
			metadataJSON   []byte
		)

		err := rows.Scan(
			&model.ID,
			&model.Name,
			&provider,
			&model.ModelName,
			&endpoint,
			&caps,
			&model.Limits.ContextWindow,
			&model.Limits.MaxOutputTokens,
			&inputCost,
			&outputCost,
			&model.IsLocal,
			&isEnabled,
			&model.Priority,
			&metadataJSON,
			&model.CreatedAt,
			&model.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
		}

		// Map fields
		model.Provider = Provider(provider)
		if endpoint != nil {
			model.Endpoint = *endpoint
		}
		model.Capabilities.Capabilities = make([]Capability, len(caps))
		for i, c := range caps {
			model.Capabilities.Capabilities[i] = Capability(c)
		}
		model.Cost.InputCostPer1K = inputCost
		model.Cost.OutputCostPer1K = outputCost
		model.Cost.Currency = "USD"

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &model.Metadata); err != nil {
				model.Metadata = make(map[string]interface{})
			}
		}

		// Apply in-memory filter if provided
		if filter == nil || filter.Match(&model) {
			models = append(models, &model)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	return models, nil
}

// GetModelContextWindowByName returns the context_window for a model identified
// by its short name (model_name column, e.g., "gemini-2.0-flash").
// Returns ErrModelNotFound if no enabled model matches.
func (r *PostgresRepository) GetModelContextWindowByName(ctx context.Context, modelName string) (int, error) {
	var contextWindow int
	err := r.pool.QueryRow(ctx, `
		SELECT context_window FROM ai_models
		WHERE model_name = $1 AND is_enabled = true
		LIMIT 1
	`, modelName).Scan(&contextWindow)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrModelNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	return contextWindow, nil
}

// GetModelProviderByName returns the provider for a model identified by its
// short name (model_name column, e.g., "gemini-2.0-flash").
// Returns ErrModelNotFound if no enabled model matches.
func (r *PostgresRepository) GetModelProviderByName(ctx context.Context, modelName string) (string, error) {
	var provider string
	err := r.pool.QueryRow(ctx, `
		SELECT provider FROM ai_models
		WHERE model_name = $1 AND is_enabled = true
		LIMIT 1
	`, modelName).Scan(&provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrModelNotFound
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	return provider, nil
}

// GetHealth retrieves health metrics for a model.
func (r *PostgresRepository) GetHealth(ctx context.Context, modelID string) (*ModelHealth, error) {
	query := `
		SELECT
			status, last_check, avg_latency_ms,
			error_count, success_count, last_error, last_error_at
		FROM ai_model_health
		WHERE model_id = $1
	`

	var (
		health      ModelHealth
		status      string
		lastCheck   *time.Time
		avgLatency  *float64
		lastError   *string
		lastErrorAt *time.Time
		errorCount  int64
		successCount int64
	)

	err := r.pool.QueryRow(ctx, query, modelID).Scan(
		&status,
		&lastCheck,
		&avgLatency,
		&errorCount,
		&successCount,
		&lastError,
		&lastErrorAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		// No health record yet - return unknown status
		return &ModelHealth{Status: HealthStatusUnknown}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	health.Status = HealthStatus(status)
	if lastCheck != nil {
		health.LastCheck = *lastCheck
	}
	if avgLatency != nil {
		health.AvgLatencyMs = *avgLatency
	}
	health.ErrorCount = errorCount
	health.RequestCount = errorCount + successCount
	if lastError != nil {
		health.ErrorMessage = *lastError
	}

	return &health, nil
}

// UpdateHealth updates or inserts health metrics for a model.
func (r *PostgresRepository) UpdateHealth(ctx context.Context, modelID string, health *ModelHealth) error {
	// Upsert health record
	query := `
		INSERT INTO ai_model_health (
			model_id, status, last_check, avg_latency_ms,
			error_count, success_count, last_error, last_error_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, NOW()
		)
		ON CONFLICT (model_id) DO UPDATE SET
			status = EXCLUDED.status,
			last_check = EXCLUDED.last_check,
			avg_latency_ms = EXCLUDED.avg_latency_ms,
			error_count = EXCLUDED.error_count,
			success_count = EXCLUDED.success_count,
			last_error = EXCLUDED.last_error,
			last_error_at = EXCLUDED.last_error_at,
			updated_at = NOW()
	`

	// Calculate success count from request count and error count
	successCount := health.RequestCount - health.ErrorCount
	if successCount < 0 {
		successCount = 0
	}

	var lastError *string
	var lastErrorAt *time.Time
	if health.ErrorMessage != "" {
		lastError = &health.ErrorMessage
		now := time.Now()
		lastErrorAt = &now
	}

	_, err := r.pool.Exec(ctx, query,
		modelID,
		string(health.Status),
		nullableTime(health.LastCheck),
		nullableFloat64(health.AvgLatencyMs),
		health.ErrorCount,
		successCount,
		lastError,
		lastErrorAt,
	)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	r.logger.Debug("Health updated",
		logging.F("model_id", modelID),
		logging.F("status", health.Status))

	return nil
}

// GetRoutingRules retrieves all enabled routing rules.
func (r *PostgresRepository) GetRoutingRules(ctx context.Context) ([]*RoutingRule, error) {
	query := `
		SELECT
			id, name, task_type, preferred_models, fallback_models,
			require_local, max_cost_per_request, optimization_mode,
			priority, is_enabled, created_at
		FROM ai_routing_rules
		WHERE is_enabled = true
		ORDER BY priority DESC
		LIMIT 100
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	return r.scanRoutingRules(rows)
}

// GetRoutingRulesByTask retrieves routing rules for a specific task type.
func (r *PostgresRepository) GetRoutingRulesByTask(ctx context.Context, taskType string) ([]*RoutingRule, error) {
	query := `
		SELECT
			id, name, task_type, preferred_models, fallback_models,
			require_local, max_cost_per_request, optimization_mode,
			priority, is_enabled, created_at
		FROM ai_routing_rules
		WHERE task_type = $1 AND is_enabled = true
		ORDER BY priority DESC
		LIMIT 100
	`

	rows, err := r.pool.Query(ctx, query, taskType)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}
	defer rows.Close()

	return r.scanRoutingRules(rows)
}

// GetRoutingRuleByName retrieves a routing rule by its unique name.
func (r *PostgresRepository) GetRoutingRuleByName(ctx context.Context, name string) (*RoutingRule, error) {
	query := `
		SELECT
			id, name, task_type, preferred_models, fallback_models,
			require_local, max_cost_per_request, optimization_mode,
			priority, is_enabled, created_at
		FROM ai_routing_rules
		WHERE name = $1
	`

	var rule RoutingRule
	var maxCost *float64

	err := r.pool.QueryRow(ctx, query, name).Scan(
		&rule.ID,
		&rule.Name,
		&rule.TaskType,
		&rule.PreferredModels,
		&rule.FallbackModels,
		&rule.RequireLocal,
		&maxCost,
		&rule.OptimizationMode,
		&rule.Priority,
		&rule.IsEnabled,
		&rule.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoutingRuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	rule.MaxCostPerRequest = maxCost
	return &rule, nil
}

// CreateRoutingRule inserts a new routing rule.
func (r *PostgresRepository) CreateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}

	query := `
		INSERT INTO ai_routing_rules (
			name, task_type, preferred_models, fallback_models,
			require_local, max_cost_per_request, optimization_mode,
			priority, is_enabled, created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, NOW()
		)
		RETURNING id
	`

	optMode := rule.OptimizationMode
	if optMode == "" {
		optMode = OptimizationModeBalanced
	}

	err := r.pool.QueryRow(ctx, query,
		rule.Name,
		rule.TaskType,
		rule.PreferredModels,
		rule.FallbackModels,
		rule.RequireLocal,
		rule.MaxCostPerRequest,
		optMode,
		rule.Priority,
		rule.IsEnabled,
	).Scan(&rule.ID)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	r.logger.Debug("Routing rule created",
		logging.F("rule_id", rule.ID),
		logging.F("name", rule.Name))

	return nil
}

// UpdateRoutingRule updates an existing routing rule.
func (r *PostgresRepository) UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}

	query := `
		UPDATE ai_routing_rules SET
			name = $2,
			task_type = $3,
			preferred_models = $4,
			fallback_models = $5,
			require_local = $6,
			max_cost_per_request = $7,
			optimization_mode = $8,
			priority = $9,
			is_enabled = $10
		WHERE id = $1
	`

	optMode := rule.OptimizationMode
	if optMode == "" {
		optMode = OptimizationModeBalanced
	}

	result, err := r.pool.Exec(ctx, query,
		rule.ID,
		rule.Name,
		rule.TaskType,
		rule.PreferredModels,
		rule.FallbackModels,
		rule.RequireLocal,
		rule.MaxCostPerRequest,
		optMode,
		rule.Priority,
		rule.IsEnabled,
	)

	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	if result.RowsAffected() == 0 {
		return ErrRoutingRuleNotFound
	}

	r.logger.Debug("Routing rule updated",
		logging.F("rule_id", rule.ID))

	return nil
}

// DeleteRoutingRule removes a routing rule.
func (r *PostgresRepository) DeleteRoutingRule(ctx context.Context, ruleID string) error {
	query := `DELETE FROM ai_routing_rules WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, ruleID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	if result.RowsAffected() == 0 {
		return ErrRoutingRuleNotFound
	}

	r.logger.Debug("Routing rule deleted",
		logging.F("rule_id", ruleID))

	return nil
}

// scanRoutingRules scans rows into RoutingRule slices.
func (r *PostgresRepository) scanRoutingRules(rows pgx.Rows) ([]*RoutingRule, error) {
	var rules []*RoutingRule
	for rows.Next() {
		var (
			rule            RoutingRule
			maxCost         *float64
		)

		err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.TaskType,
			&rule.PreferredModels,
			&rule.FallbackModels,
			&rule.RequireLocal,
			&maxCost,
			&rule.OptimizationMode,
			&rule.Priority,
			&rule.IsEnabled,
			&rule.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
		}

		rule.MaxCostPerRequest = maxCost
		rules = append(rules, &rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabaseError, err)
	}

	return rules, nil
}

// nullableString returns nil if s is empty, otherwise returns a pointer to s.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullableTime returns nil if t is zero, otherwise returns a pointer to t.
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// nullableFloat64 returns nil if f is 0, otherwise returns a pointer to f.
func nullableFloat64(f float64) *float64 {
	if f == 0 {
		return nil
	}
	return &f
}
