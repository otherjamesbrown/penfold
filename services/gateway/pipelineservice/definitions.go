package pipelineservice

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// =============================================================================
// Pipeline Definition RPCs
// =============================================================================

// ListPipelineDefinitions lists all defined pipelines with their stages.
func (s *Service) ListPipelineDefinitions(ctx context.Context, req *pipelinev1.ListPipelineDefinitionsRequest) (*pipelinev1.ListPipelineDefinitionsResponse, error) {
	s.logger.Debug("ListPipelineDefinitions called", logging.F("tenant_id", req.TenantId))

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	defs, err := s.repo.ListPipelines(ctx, tenantID)
	if err != nil {
		s.logger.Error("Error listing pipeline definitions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list pipeline definitions: %v", err)
	}

	pipelines := make([]*pipelinev1.PipelineDefinition, 0, len(defs))
	for _, d := range defs {
		pipelines = append(pipelines, pipelineDefToProto(d))
	}

	return &pipelinev1.ListPipelineDefinitionsResponse{Pipelines: pipelines}, nil
}

// GetPipelineDefinition retrieves a single pipeline definition with its stages.
func (s *Service) GetPipelineDefinition(ctx context.Context, req *pipelinev1.GetPipelineDefinitionRequest) (*pipelinev1.GetPipelineDefinitionResponse, error) {
	s.logger.Debug("GetPipelineDefinition called",
		logging.F("tenant_id", req.TenantId),
		logging.F("pipeline", req.Pipeline),
	)

	if req.Pipeline == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline name is required")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	stages, err := s.repo.GetPipelineStages(ctx, tenantID, req.Pipeline)
	if err != nil {
		s.logger.Error("Error getting pipeline definition", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get pipeline definition: %v", err)
	}
	if len(stages) == 0 {
		return nil, status.Errorf(codes.NotFound, "pipeline %q not found", req.Pipeline)
	}

	def := pipeline.PipelineDefinition{
		Pipeline: req.Pipeline,
		Stages:   stages,
	}
	return &pipelinev1.GetPipelineDefinitionResponse{Definition: pipelineDefToProto(def)}, nil
}

// UpdatePipelineStageConfig updates configuration for a specific stage in a pipeline.
func (s *Service) UpdatePipelineStageConfig(ctx context.Context, req *pipelinev1.UpdatePipelineStageConfigRequest) (*pipelinev1.UpdatePipelineStageConfigResponse, error) {
	s.logger.Debug("UpdatePipelineStageConfig called",
		logging.F("tenant_id", req.TenantId),
		logging.F("pipeline", req.Pipeline),
		logging.F("stage", req.Stage),
	)

	if req.Pipeline == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline name is required")
	}
	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage name is required")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	update := pipeline.StageConfigUpdate{}
	if req.ModelOverride != nil {
		v := *req.ModelOverride
		update.ModelOverride = &v
	}
	if req.Enabled != nil {
		v := *req.Enabled
		update.Enabled = &v
	}
	if req.SkipWhenLow != nil {
		v := *req.SkipWhenLow
		update.SkipWhenLow = &v
	}
	if req.PromptOverride != nil {
		v := int(*req.PromptOverride)
		update.PromptOverride = &v
	}
	if req.TimeoutSeconds != nil {
		v := int(*req.TimeoutSeconds)
		update.TimeoutSeconds = &v
	}
	if req.Temperature != nil {
		v := float64(*req.Temperature)
		update.Temperature = &v
	}
	if req.MaxTokens != nil {
		v := int(*req.MaxTokens)
		update.MaxTokens = &v
	}
	if req.MaxRetries != nil {
		v := int(*req.MaxRetries)
		update.MaxRetries = &v
	}

	sd, err := s.repo.UpdateStageConfig(ctx, tenantID, req.Pipeline, req.Stage, update)
	if err != nil {
		s.logger.Error("Error updating stage config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update stage config: %v", err)
	}

	return &pipelinev1.UpdatePipelineStageConfigResponse{
		Stage:   stageDefToProto(*sd),
		Message: fmt.Sprintf("Updated stage %q in pipeline %q", req.Stage, req.Pipeline),
	}, nil
}

// CreatePipelineDefinition creates a new pipeline, optionally cloning from an existing one.
func (s *Service) CreatePipelineDefinition(ctx context.Context, req *pipelinev1.CreatePipelineDefinitionRequest) (*pipelinev1.CreatePipelineDefinitionResponse, error) {
	s.logger.Debug("CreatePipelineDefinition called",
		logging.F("tenant_id", req.TenantId),
		logging.F("pipeline", req.Pipeline),
		logging.F("from_pipeline", req.FromPipeline),
	)

	if req.Pipeline == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline name is required")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	stages, err := s.repo.CreatePipeline(ctx, tenantID, req.Pipeline, req.FromPipeline)
	if err != nil {
		s.logger.Error("Error creating pipeline definition", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create pipeline: %v", err)
	}

	def := pipeline.PipelineDefinition{
		Pipeline: req.Pipeline,
		Stages:   stages,
	}

	msg := fmt.Sprintf("Created pipeline %q", req.Pipeline)
	if req.FromPipeline != "" {
		msg = fmt.Sprintf("Created pipeline %q (cloned from %q)", req.Pipeline, req.FromPipeline)
	}

	return &pipelinev1.CreatePipelineDefinitionResponse{
		Definition: pipelineDefToProto(def),
		Message:    msg,
	}, nil
}

// =============================================================================
// Pipeline Audit RPCs
// =============================================================================

// AuditPipelineCompleteness finds completed items missing expected pipeline_run records.
func (s *Service) AuditPipelineCompleteness(ctx context.Context, req *pipelinev1.AuditPipelineCompletenessRequest) (*pipelinev1.AuditPipelineCompletenessResponse, error) {
	s.logger.Debug("AuditPipelineCompleteness called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
		logging.F("pipeline", req.Pipeline),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	limit := int(req.Limit)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Find completed sources and cross-reference with pipeline_definitions to detect missing stages.
	// For each completed source, find which pipeline it was routed to, get expected stages,
	// and compare with actual pipeline_runs.
	// Pipeline assignment is determined by routing rules, not stored on the source.
	// Default all sources to 'standard' since that's the only pipeline currently in use.
	query := `
		WITH completed_sources AS (
			SELECT s.id AS source_id, s.content_id, 'standard'::text AS pipeline
			FROM sources s
			WHERE s.tenant_id = $1
			  AND s.processing_status = 'completed'
			  AND s.is_deleted = false
			  AND ($3 = '' OR 'standard' = $3)
			ORDER BY s.id DESC
			LIMIT $2
		),
		expected_stages AS (
			SELECT cs.source_id, cs.content_id, cs.pipeline, pd.stage
			FROM completed_sources cs
			JOIN pipeline_definitions pd ON pd.tenant_id = $1
			  AND pd.pipeline = cs.pipeline
			  AND pd.enabled = true
			  AND pd.optional = false
		),
		actual_runs AS (
			SELECT pr.source_id, pr.stage
			FROM pipeline_runs pr
			JOIN completed_sources cs ON cs.source_id = pr.source_id
			WHERE pr.status = 'completed'
		)
		SELECT es.source_id, es.content_id, es.pipeline,
		       array_agg(DISTINCT es.stage) FILTER (WHERE ar.stage IS NULL) AS missing_stages,
		       array_agg(DISTINCT ar.stage) FILTER (WHERE ar.stage IS NOT NULL) AS completed_stages
		FROM expected_stages es
		LEFT JOIN actual_runs ar ON ar.source_id = es.source_id AND ar.stage = es.stage
		GROUP BY es.source_id, es.content_id, es.pipeline
		HAVING COUNT(*) FILTER (WHERE ar.stage IS NULL) > 0
		ORDER BY es.source_id DESC
	`

	pipelineFilter := ""
	if req.Pipeline != "" {
		pipelineFilter = req.Pipeline
	}

	rows, err := s.db.QueryContext(ctx, query, tenantID, limit, pipelineFilter)
	if err != nil {
		s.logger.Error("Error auditing pipeline completeness", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to audit pipeline completeness: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*pipelinev1.AuditItem
	for rows.Next() {
		var item pipelinev1.AuditItem
		var missingStages, completedStages []string
		if err := rows.Scan(
			&item.SourceId, &item.ContentId, &item.Pipeline,
			&missingStages, &completedStages,
		); err != nil {
			s.logger.Error("Error scanning audit item", logging.Err(err))
			continue
		}
		item.MissingStages = missingStages
		item.CompletedStages = completedStages
		items = append(items, &item)
	}

	return &pipelinev1.AuditPipelineCompletenessResponse{
		Items:      items,
		TotalCount: int64(len(items)),
	}, nil
}

// =============================================================================
// Pipeline Comparison RPCs
// =============================================================================

// ComparePipelineRuns compares pipeline run statistics grouped by model and prompt version.
func (s *Service) ComparePipelineRuns(ctx context.Context, req *pipelinev1.ComparePipelineRunsRequest) (*pipelinev1.ComparePipelineRunsResponse, error) {
	s.logger.Debug("ComparePipelineRuns called",
		logging.F("tenant_id", req.TenantId),
		logging.F("stage", req.Stage),
		logging.F("limit", req.Limit),
	)

	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	limit := int(req.Limit)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			COALESCE(pr.model_id, 'unknown') AS model_id,
			COALESCE(pr.prompt_version, 0) AS prompt_version,
			COUNT(*) AS run_count,
			AVG(pr.duration_ms) AS avg_duration_ms,
			AVG(COALESCE(pr.input_tokens, 0)) AS avg_input_tokens,
			AVG(COALESCE(pr.output_tokens, 0)) AS avg_output_tokens,
			COUNT(*) FILTER (WHERE pr.status = 'completed') AS success_count,
			COUNT(*) FILTER (WHERE pr.status = 'failed') AS failure_count
		FROM pipeline_runs pr
		JOIN sources s ON s.id = pr.source_id AND s.tenant_id = $1
		WHERE pr.stage = $2
		GROUP BY pr.model_id, pr.prompt_version
		ORDER BY run_count DESC
		LIMIT $3
	`, tenantID, req.Stage, limit)
	if err != nil {
		s.logger.Error("Error comparing pipeline runs", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to compare pipeline runs: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var stats []*pipelinev1.PipelineRunStats
	for rows.Next() {
		var s pipelinev1.PipelineRunStats
		if err := rows.Scan(
			&s.ModelId, &s.PromptVersion,
			&s.RunCount, &s.AvgDurationMs,
			&s.AvgInputTokens, &s.AvgOutputTokens,
			&s.SuccessCount, &s.FailureCount,
		); err != nil {
			continue
		}
		stats = append(stats, &s)
	}

	return &pipelinev1.ComparePipelineRunsResponse{
		Stage: req.Stage,
		Stats: stats,
	}, nil
}

// =============================================================================
// Proto conversion helpers
// =============================================================================

func stageDefToProto(sd pipeline.StageDefinition) *pipelinev1.PipelineStageDefinition {
	p := &pipelinev1.PipelineStageDefinition{
		Stage:          sd.Stage,
		StageOrder:     int32(sd.StageOrder),
		Enabled:        sd.Enabled,
		SkipWhenLow:    sd.SkipWhenLow,
		Optional:       sd.Optional,
		TimeoutSeconds: int32(sd.TimeoutSeconds),
	}
	if sd.ModelOverride != nil {
		p.ModelOverride = *sd.ModelOverride
	}
	if sd.PromptOverride != nil {
		p.PromptOverride = int32(*sd.PromptOverride)
	}
	if sd.Temperature != nil {
		v := float32(*sd.Temperature)
		p.Temperature = &v
	}
	if sd.MaxTokens != nil {
		v := int32(*sd.MaxTokens)
		p.MaxTokens = &v
	}
	if sd.MaxRetries != nil {
		v := int32(*sd.MaxRetries)
		p.MaxRetries = &v
	}
	return p
}

func pipelineDefToProto(d pipeline.PipelineDefinition) *pipelinev1.PipelineDefinition {
	stages := make([]*pipelinev1.PipelineStageDefinition, 0, len(d.Stages))
	for _, s := range d.Stages {
		stages = append(stages, stageDefToProto(s))
	}
	pd := &pipelinev1.PipelineDefinition{
		Pipeline: d.Pipeline,
		Stages:   stages,
	}
	// ContentType will be visible via proto once the proto field is added.
	// For now, the data is stored and available through the Go API.
	_ = d.ContentType
	return pd
}
