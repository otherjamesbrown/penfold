package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// StageDefinition represents a single stage in a pipeline definition.
type StageDefinition struct {
	ID             int
	TenantID       string
	Pipeline       string
	Stage          string
	StageOrder     int
	Enabled        bool
	ModelOverride  *string
	PromptOverride *int
	SkipWhenLow    bool
	Optional       bool
	TimeoutSeconds int
	CreatedAt      time.Time
}

// PipelineDefinition groups a pipeline name with its ordered stages.
type PipelineDefinition struct {
	Pipeline string
	Stages   []StageDefinition
}

// ListPipelines returns all pipeline definitions for a tenant, grouped by pipeline name.
func (r *Repository) ListPipelines(ctx context.Context, tenantID string) ([]PipelineDefinition, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, pipeline, stage, stage_order, enabled,
		       model_override, prompt_override, skip_when_low, optional,
		       timeout_seconds, created_at
		FROM pipeline_definitions
		WHERE tenant_id = $1
		ORDER BY pipeline, stage_order
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing pipeline definitions: %w", err)
	}
	defer rows.Close()

	byPipeline := make(map[string][]StageDefinition)
	var order []string
	for rows.Next() {
		var sd StageDefinition
		if err := rows.Scan(
			&sd.ID, &sd.TenantID, &sd.Pipeline, &sd.Stage, &sd.StageOrder,
			&sd.Enabled, &sd.ModelOverride, &sd.PromptOverride,
			&sd.SkipWhenLow, &sd.Optional, &sd.TimeoutSeconds, &sd.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning pipeline definition: %w", err)
		}
		if _, exists := byPipeline[sd.Pipeline]; !exists {
			order = append(order, sd.Pipeline)
		}
		byPipeline[sd.Pipeline] = append(byPipeline[sd.Pipeline], sd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pipeline definitions: %w", err)
	}

	result := make([]PipelineDefinition, 0, len(order))
	for _, name := range order {
		result = append(result, PipelineDefinition{
			Pipeline: name,
			Stages:   byPipeline[name],
		})
	}
	return result, nil
}

// GetPipelineStages returns the ordered stages for a specific pipeline.
func (r *Repository) GetPipelineStages(ctx context.Context, tenantID, pipeline string) ([]StageDefinition, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, pipeline, stage, stage_order, enabled,
		       model_override, prompt_override, skip_when_low, optional,
		       timeout_seconds, created_at
		FROM pipeline_definitions
		WHERE tenant_id = $1 AND pipeline = $2
		ORDER BY stage_order
	`, tenantID, pipeline)
	if err != nil {
		return nil, fmt.Errorf("getting pipeline stages: %w", err)
	}
	defer rows.Close()

	var stages []StageDefinition
	for rows.Next() {
		var sd StageDefinition
		if err := rows.Scan(
			&sd.ID, &sd.TenantID, &sd.Pipeline, &sd.Stage, &sd.StageOrder,
			&sd.Enabled, &sd.ModelOverride, &sd.PromptOverride,
			&sd.SkipWhenLow, &sd.Optional, &sd.TimeoutSeconds, &sd.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning pipeline stage: %w", err)
		}
		stages = append(stages, sd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pipeline stages: %w", err)
	}
	return stages, nil
}

// UpdateStageConfig updates configuration fields for a specific stage in a pipeline.
// Only non-nil fields are updated.
type StageConfigUpdate struct {
	ModelOverride  *string
	Enabled        *bool
	SkipWhenLow    *bool
	PromptOverride *int
	TimeoutSeconds *int
}

func (r *Repository) UpdateStageConfig(ctx context.Context, tenantID, pipeline, stage string, update StageConfigUpdate) (*StageDefinition, error) {
	// Build dynamic SET clause
	setClauses := []string{}
	args := []interface{}{tenantID, pipeline, stage}
	argIdx := 4

	if update.ModelOverride != nil {
		setClauses = append(setClauses, fmt.Sprintf("model_override = $%d", argIdx))
		args = append(args, *update.ModelOverride)
		argIdx++
	}
	if update.Enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *update.Enabled)
		argIdx++
	}
	if update.SkipWhenLow != nil {
		setClauses = append(setClauses, fmt.Sprintf("skip_when_low = $%d", argIdx))
		args = append(args, *update.SkipWhenLow)
		argIdx++
	}
	if update.PromptOverride != nil {
		setClauses = append(setClauses, fmt.Sprintf("prompt_override = $%d", argIdx))
		args = append(args, *update.PromptOverride)
		argIdx++
	}
	if update.TimeoutSeconds != nil {
		setClauses = append(setClauses, fmt.Sprintf("timeout_seconds = $%d", argIdx))
		args = append(args, *update.TimeoutSeconds)
		argIdx++
	}

	if len(setClauses) == 0 {
		// Nothing to update — just return the current row.
		return r.getStageDefinition(ctx, tenantID, pipeline, stage)
	}

	query := fmt.Sprintf(`
		UPDATE pipeline_definitions
		SET %s
		WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3
		RETURNING id, tenant_id, pipeline, stage, stage_order, enabled,
		          model_override, prompt_override, skip_when_low, optional,
		          timeout_seconds, created_at
	`, joinStrings(setClauses, ", "))

	var sd StageDefinition
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&sd.ID, &sd.TenantID, &sd.Pipeline, &sd.Stage, &sd.StageOrder,
		&sd.Enabled, &sd.ModelOverride, &sd.PromptOverride,
		&sd.SkipWhenLow, &sd.Optional, &sd.TimeoutSeconds, &sd.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("stage %q in pipeline %q not found", stage, pipeline)
	}
	if err != nil {
		return nil, fmt.Errorf("updating stage config: %w", err)
	}
	return &sd, nil
}

// CreatePipeline creates a new pipeline definition.
// If fromPipeline is non-empty, stages are cloned from the source pipeline.
func (r *Repository) CreatePipeline(ctx context.Context, tenantID, name, fromPipeline string) ([]StageDefinition, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check pipeline doesn't already exist.
	var count int
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM pipeline_definitions
		WHERE tenant_id = $1 AND pipeline = $2
	`, tenantID, name).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("checking existing pipeline: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("pipeline %q already exists", name)
	}

	if fromPipeline != "" {
		// Clone from source pipeline.
		_, err = tx.Exec(ctx, `
			INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled,
			                                  model_override, prompt_override, skip_when_low,
			                                  optional, timeout_seconds)
			SELECT tenant_id, $3, stage, stage_order, enabled,
			       model_override, prompt_override, skip_when_low,
			       optional, timeout_seconds
			FROM pipeline_definitions
			WHERE tenant_id = $1 AND pipeline = $2
			ORDER BY stage_order
		`, tenantID, fromPipeline, name)
		if err != nil {
			return nil, fmt.Errorf("cloning pipeline from %q: %w", fromPipeline, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	// Return the newly created pipeline stages.
	return r.GetPipelineStages(ctx, tenantID, name)
}

// ListAllDefinedStages returns all unique stage names across all tenants and pipelines.
// Used at startup for registry validation.
func (r *Repository) ListAllDefinedStages(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT DISTINCT stage FROM pipeline_definitions ORDER BY stage`)
	if err != nil {
		return nil, fmt.Errorf("listing all defined stages: %w", err)
	}
	defer rows.Close()

	var stages []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scanning stage name: %w", err)
		}
		stages = append(stages, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stage names: %w", err)
	}
	return stages, nil
}

// getStageDefinition retrieves a single stage definition.
func (r *Repository) getStageDefinition(ctx context.Context, tenantID, pipeline, stage string) (*StageDefinition, error) {
	var sd StageDefinition
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, pipeline, stage, stage_order, enabled,
		       model_override, prompt_override, skip_when_low, optional,
		       timeout_seconds, created_at
		FROM pipeline_definitions
		WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3
	`, tenantID, pipeline, stage).Scan(
		&sd.ID, &sd.TenantID, &sd.Pipeline, &sd.Stage, &sd.StageOrder,
		&sd.Enabled, &sd.ModelOverride, &sd.PromptOverride,
		&sd.SkipWhenLow, &sd.Optional, &sd.TimeoutSeconds, &sd.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("stage %q in pipeline %q not found", stage, pipeline)
	}
	if err != nil {
		return nil, fmt.Errorf("getting stage definition: %w", err)
	}
	return &sd, nil
}

// joinStrings joins a slice with a separator (avoids importing strings for one use).
func joinStrings(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for _, v := range s[1:] {
		result += sep + v
	}
	return result
}
