// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"regexp"

	"go.temporal.io/sdk/temporal"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// PipelineActivities holds dependencies for pipeline metadata activities.
type PipelineActivities struct {
	logger         logging.Logger
	pipelineRepo   PipelineRepository
	baseRepo       *pipeline.Repository // Direct access for lookups
	pipelineClient pipelinev1.PipelineServiceClient
}

// NewPipelineActivities creates a new PipelineActivities instance.
func NewPipelineActivities(
	logger logging.Logger,
	pipelineRepo PipelineRepository,
	baseRepo *pipeline.Repository,
) *PipelineActivities {
	if logger == nil {
		panic("NewPipelineActivities: logger is required")
	}
	if pipelineRepo == nil {
		panic("NewPipelineActivities: pipelineRepo is required")
	}
	if baseRepo == nil {
		panic("NewPipelineActivities: baseRepo is required")
	}
	return &PipelineActivities{
		logger:       logger.With(logging.F("component", "pipeline_activities")),
		pipelineRepo: pipelineRepo,
		baseRepo:     baseRepo,
	}
}

// WithPipelineClient sets the gateway pipeline client for KickNextPending activity.
// The client is optional — if nil, KickNextPending will be a no-op.
func (a *PipelineActivities) WithPipelineClient(client pipelinev1.PipelineServiceClient) *PipelineActivities {
	a.pipelineClient = client
	return a
}

// KickNextPending triggers the gateway's KickProcessing RPC to auto-drain the pending queue.
// This is a best-effort activity: if the client is nil or the RPC fails, a warning is logged
// but the calling workflow is not failed.
func (a *PipelineActivities) KickNextPending(ctx context.Context, input workflows.KickNextPendingInput) (*workflows.KickNextPendingOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "KickNextPending"),
		logging.F("tenant_id", input.TenantID),
		logging.F("limit", input.Limit),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting kick next pending")

	if a.pipelineClient == nil {
		logger.Warn("KickNextPending: pipeline client not configured, skipping auto-drain")
		return &workflows.KickNextPendingOutput{}, nil
	}

	logger.Info("Kicking next pending pipeline item")

	resp, err := a.pipelineClient.KickProcessing(ctx, &pipelinev1.KickProcessingRequest{
		TenantId: input.TenantID,
		Limit:    input.Limit,
	})
	if err != nil {
		logger.Error("KickProcessing RPC failed", logging.Err(err))
		return nil, temporal.NewApplicationErrorWithCause(
			"KickProcessing RPC failed",
			"GRPCError",
			err,
		)
	}

	// Record heartbeat after successful RPC
	recordHeartbeat(ctx, "kick next pending complete")

	logger.Info("KickNextPending complete",
		logging.F("queued_count", resp.QueuedCount),
		logging.F("message", resp.Message),
	)

	return &workflows.KickNextPendingOutput{
		QueuedCount: resp.QueuedCount,
		Message:     resp.Message,
	}, nil
}

// RecordOverrides records override parameters in the latest pipeline run for a source.
// It looks up the most recent run ID for the source, then calls the repository to record the overrides.
func (a *PipelineActivities) RecordOverrides(ctx context.Context, input workflows.RecordOverridesInput) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "RecordOverrides"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting record overrides")

	logger.Info("Recording overrides for latest pipeline run")

	// Check for cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Validate input
	if input.SourceID <= 0 {
		return temporal.NewApplicationError(
			"source_id must be positive",
			"ValidationError",
		)
	}

	if len(input.Overrides) == 0 {
		logger.Warn("No overrides provided, skipping")
		return nil
	}

	// Get the latest run ID for this source
	runIDs, err := a.baseRepo.GetLatestRunIDs(ctx, input.SourceID, 1)
	if err != nil {
		logger.Error("Failed to get latest run ID", logging.Err(err))
		return temporal.NewApplicationErrorWithCause(
			fmt.Sprintf("failed to get latest run ID for source %d", input.SourceID),
			"RepositoryError",
			err,
		)
	}

	if len(runIDs) == 0 {
		logger.Warn("No pipeline runs found for source")
		return temporal.NewApplicationError(
			fmt.Sprintf("no pipeline runs found for source %d", input.SourceID),
			"NotFoundError",
		)
	}

	runID := runIDs[0]
	logger = logger.With(logging.F("run_id", runID))

	// Record the overrides in the pipeline run
	err = a.pipelineRepo.RecordOverrides(ctx, runID, input.Overrides)
	if err != nil {
		logger.Error("Failed to record overrides", logging.Err(err))
		return temporal.NewApplicationErrorWithCause(
			fmt.Sprintf("failed to record overrides for run %d", runID),
			"RepositoryError",
			err,
		)
	}

	// Record heartbeat after processing
	recordHeartbeat(ctx, "record overrides complete")

	logger.Info("Overrides recorded successfully",
		logging.F("override_count", len(input.Overrides)),
	)

	return nil
}

// RecordSkippedStage records pipeline_runs rows with status "skipped" for all stages
// that were bypassed due to contribution-based or category-based gating.
// It is best-effort: failures are logged but do not fail the calling workflow.
func (a *PipelineActivities) RecordSkippedStage(ctx context.Context, input workflows.RecordSkippedStageInput) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "RecordSkippedStage"),
		logging.F("source_id", input.SourceID),
		logging.F("stage_count", len(input.Stages)),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting record skipped stages")

	logger.Info("Recording skipped pipeline stages")

	if len(input.Stages) == 0 {
		logger.Warn("RecordSkippedStage called with no stages, skipping")
		return nil
	}

	var firstErr error
	for _, s := range input.Stages {
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:        input.SourceID,
			Stage:           s.Stage,
			Status:          "skipped",
			SkipReason:      s.SkipReason,
			LangfuseTraceID: input.LangfuseTraceID,
			// DurationMS, InputTokens, OutputTokens are all 0 for skipped stages
		})
		if runErr != nil {
			logger.Error("Failed to record skipped pipeline run",
				logging.F("stage", s.Stage),
				logging.F("skip_reason", s.SkipReason),
				logging.Err(runErr),
			)
			// Accumulate the first error; continue recording remaining stages.
			if firstErr == nil {
				firstErr = fmt.Errorf("record skipped stage %q: %w", s.Stage, runErr)
			}
		} else {
			logger.Info("Recorded skipped stage",
				logging.F("stage", s.Stage),
				logging.F("skip_reason", s.SkipReason),
			)
		}
	}

	recordHeartbeat(ctx, "record skipped stages complete")

	return firstErr
}

// FetchPipelineDefinition fetches the pipeline definition for a given pipeline name.
// If the pipeline is not found in the database, it returns Found=false with an empty stage list.
func (a *PipelineActivities) FetchPipelineDefinition(ctx context.Context, input workflows.FetchPipelineDefinitionInput) (*workflows.FetchPipelineDefinitionOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "FetchPipelineDefinition"),
		logging.F("tenant_id", input.TenantID),
		logging.F("pipeline", input.Pipeline),
	)

	recordHeartbeat(ctx, "fetching pipeline definition")

	if input.Pipeline == "" {
		logger.Warn("FetchPipelineDefinition: empty pipeline name, returning not found")
		return &workflows.FetchPipelineDefinitionOutput{Found: false}, nil
	}

	stages, err := a.baseRepo.GetPipelineStages(ctx, input.TenantID, input.Pipeline)
	if err != nil {
		logger.Error("Failed to fetch pipeline definition", logging.Err(err))
		return nil, temporal.NewApplicationErrorWithCause(
			fmt.Sprintf("failed to fetch pipeline definition for %q", input.Pipeline),
			"RepositoryError",
			err,
		)
	}

	if len(stages) == 0 {
		logger.Info("Pipeline definition not found, workflow will use fallback")
		return &workflows.FetchPipelineDefinitionOutput{Found: false}, nil
	}

	out := &workflows.FetchPipelineDefinitionOutput{
		Found:  true,
		Stages: make([]workflows.PipelineStageConfig, len(stages)),
	}
	// ContentType is per-pipeline (same for all stages); take from first stage.
	if len(stages) > 0 && stages[0].ContentType != nil {
		out.ContentType = *stages[0].ContentType
	}
	for i, s := range stages {
		out.Stages[i] = workflows.PipelineStageConfig{
			Stage:          s.Stage,
			StageOrder:     s.StageOrder,
			Enabled:        s.Enabled,
			SkipWhenLow:    s.SkipWhenLow,
			Optional:       s.Optional,
			TimeoutSeconds: s.TimeoutSeconds,
		}
		if s.ModelOverride != nil {
			out.Stages[i].ModelOverride = *s.ModelOverride
		}
		if s.PromptOverride != nil {
			out.Stages[i].PromptOverride = int32(*s.PromptOverride)
		}
	}

	logger.Info("Pipeline definition fetched",
		logging.F("stage_count", len(out.Stages)),
	)

	recordHeartbeat(ctx, "pipeline definition fetched")
	return out, nil
}

// contentIDPattern matches the standard content ID format: <type:2>-<base62:8>
// Example: "em-abc12XYZ"
var contentIDPattern = regexp.MustCompile(`^[a-z]{2}-[A-Za-z0-9]{8}$`)

// Ensure PipelineActivities implements required interfaces at compile time.
var _ interface {
	RecordOverrides(ctx context.Context, input workflows.RecordOverridesInput) error
	KickNextPending(ctx context.Context, input workflows.KickNextPendingInput) (*workflows.KickNextPendingOutput, error)
	RecordSkippedStage(ctx context.Context, input workflows.RecordSkippedStageInput) error
	FetchPipelineDefinition(ctx context.Context, input workflows.FetchPipelineDefinitionInput) (*workflows.FetchPipelineDefinitionOutput, error)
} = (*PipelineActivities)(nil)
