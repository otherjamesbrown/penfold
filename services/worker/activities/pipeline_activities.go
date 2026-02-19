// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"regexp"

	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// PipelineActivities holds dependencies for pipeline metadata activities.
type PipelineActivities struct {
	logger       logging.Logger
	pipelineRepo PipelineRepository
	baseRepo     *pipeline.Repository // Direct access for lookups
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

// contentIDPattern matches the standard content ID format: <type:2>-<base62:8>
// Example: "em-abc12XYZ"
var contentIDPattern = regexp.MustCompile(`^[a-z]{2}-[A-Za-z0-9]{8}$`)

// Ensure PipelineActivities implements required interfaces at compile time.
var _ interface {
	RecordOverrides(ctx context.Context, input workflows.RecordOverridesInput) error
} = (*PipelineActivities)(nil)
