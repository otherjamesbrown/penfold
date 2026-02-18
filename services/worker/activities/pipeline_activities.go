// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"regexp"

	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
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

// StartPipelineTracing creates a root tracing span for the content processing pipeline.
// This ensures all AI operations are grouped under a single trace in Langfuse with a
// meaningful trace title (e.g., "slm-pipeline" instead of "ai.embedding").
//
// Returns the actual OTel TraceID and SpanID from the created span. The TraceID may differ
// from the input pipelineTraceID when OTel creates a new root span (e.g., when the
// SpanContext is invalid). This ensures the workflow propagates the correct TraceID to
// all downstream activities for proper trace grouping in Langfuse.
//
// The pipelineTraceID should be a 32-character hex trace ID. If empty, a new root trace
// will be created. The contentID must follow the standard format: <type:2>-<base62:8>.
func (a *PipelineActivities) StartPipelineTracing(ctx context.Context, input workflows.StartPipelineTracingInput) (workflows.StartPipelineTracingOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "StartPipelineTracing"),
		logging.F("content_id", input.ContentID),
		logging.F("content_type", input.ContentType),
	)

	// Validate input
	if input.ContentType == "" {
		return workflows.StartPipelineTracingOutput{}, temporal.NewApplicationError(
			"content_type is required",
			"ValidationError",
		)
	}

	if input.ContentID == "" {
		return workflows.StartPipelineTracingOutput{}, temporal.NewApplicationError(
			"content_id is required",
			"ValidationError",
		)
	}

	// Validate content ID format: <type:2>-<base62:8> (11 chars total)
	if !contentIDPattern.MatchString(input.ContentID) {
		return workflows.StartPipelineTracingOutput{}, temporal.NewApplicationError(
			fmt.Sprintf("invalid content_id format: %s (expected format: <type:2>-<base62:8>)", input.ContentID),
			"ValidationError",
		)
	}

	logger.Info("Starting pipeline tracing span")

	// Create the root pipeline span.
	// IMPORTANT: Do NOT use defer span.End() here. If span.End() is deferred,
	// the slm-pipeline span ends when this activity returns (giving it zero
	// duration). Instead, the span is ended immediately after recording its
	// IDs. The span's start time is back-dated by pkg/tracing.StartPipeline
	// (2ms) to ensure measurable duration in observability tooling.
	//
	// In a future enhancement a FinishPipelineTracing activity could be added
	// to record the span with the real pipeline end timestamp; for now the
	// back-dated start ensures the span is visible with non-zero duration.
	_, span := tracing.StartPipeline(ctx, "slm-pipeline", input.ContentID, input.ContentType, input.PipelineTraceID)

	// Extract the actual TraceID and SpanID from the created span
	// Note: The TraceID may differ from input.PipelineTraceID if OTel created a new root span
	actualTraceID := span.SpanContext().TraceID().String()
	actualSpanID := span.SpanContext().SpanID().String()

	// End the span now that we have captured the IDs. The span has a back-dated
	// start time (set by StartPipeline), so its recorded duration is > 0.
	span.End()

	logger.Info("Pipeline tracing span created successfully",
		logging.F("actual_trace_id", actualTraceID),
		logging.F("actual_span_id", actualSpanID),
	)

	return workflows.StartPipelineTracingOutput{
		TraceID: actualTraceID,
		SpanID:  actualSpanID,
	}, nil
}

// Ensure PipelineActivities implements required interfaces at compile time.
var _ interface {
	RecordOverrides(ctx context.Context, input workflows.RecordOverridesInput) error
	StartPipelineTracing(ctx context.Context, input workflows.StartPipelineTracingInput) (workflows.StartPipelineTracingOutput, error)
} = (*PipelineActivities)(nil)
