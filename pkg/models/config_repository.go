package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ModelConfigEntry represents a single model configuration entry.
type ModelConfigEntry struct {
	ConfigKey string
	ModelName string
	UpdatedAt time.Time
	UpdatedBy string
}

// Repository provides database operations for model configuration.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new model config repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger,
	}
}

// ErrNotFound is returned when a model config is not found.
var ErrNotFound = errors.New("model config not found")

// GetModelConfig retrieves the model name for a specific config key.
// Returns ErrNotFound if the config does not exist.
func (r *Repository) GetModelConfig(ctx context.Context, tenantID uuid.UUID, configKey string) (string, error) {
	query := `
		SELECT model_name
		FROM model_config
		WHERE tenant_id = $1 AND config_key = $2
	`

	var modelName string
	err := r.pool.QueryRow(ctx, query, tenantID, configKey).Scan(&modelName)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: config_key=%s", ErrNotFound, configKey)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get model config: %w", err)
	}

	return modelName, nil
}

// SetModelConfig creates or updates a model configuration (upsert).
func (r *Repository) SetModelConfig(ctx context.Context, tenantID uuid.UUID, configKey, modelName, updatedBy string) error {
	query := `
		INSERT INTO model_config (tenant_id, config_key, model_name, updated_at, updated_by)
		VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (tenant_id, config_key)
		DO UPDATE SET
			model_name = EXCLUDED.model_name,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`

	_, err := r.pool.Exec(ctx, query, tenantID, configKey, modelName, updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set model config: %w", err)
	}

	return nil
}

// DeleteModelConfig removes a model configuration.
// Does not return an error if the config does not exist (idempotent).
func (r *Repository) DeleteModelConfig(ctx context.Context, tenantID uuid.UUID, configKey string) error {
	query := `
		DELETE FROM model_config
		WHERE tenant_id = $1 AND config_key = $2
	`

	_, err := r.pool.Exec(ctx, query, tenantID, configKey)
	if err != nil {
		return fmt.Errorf("failed to delete model config: %w", err)
	}

	return nil
}

// ListModelConfig returns all model configurations for a tenant.
// Returns an empty slice if the tenant has no configurations.
func (r *Repository) ListModelConfig(ctx context.Context, tenantID uuid.UUID) ([]ModelConfigEntry, error) {
	query := `
		SELECT config_key, model_name, updated_at, updated_by
		FROM model_config
		WHERE tenant_id = $1
		ORDER BY config_key
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list model configs: %w", err)
	}
	defer rows.Close()

	var configs []ModelConfigEntry
	for rows.Next() {
		var cfg ModelConfigEntry
		err := rows.Scan(&cfg.ConfigKey, &cfg.ModelName, &cfg.UpdatedAt, &cfg.UpdatedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model config: %w", err)
		}
		configs = append(configs, cfg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating model configs: %w", err)
	}

	// Return empty slice instead of nil
	if configs == nil {
		configs = []ModelConfigEntry{}
	}

	return configs, nil
}
