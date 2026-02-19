// Package activities provides Langfuse ingestion activities for the pipeline workflow.
package activities

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// LangfuseActivities provides Temporal activities for reporting pipeline runs to Langfuse.
// If ingestion is nil (Langfuse not configured), all activities become no-ops.
type LangfuseActivities struct {
	ingestion *langfuse.Ingestion
	logger    logging.Logger
}

// NewLangfuseActivities creates a new LangfuseActivities.
// If ingestion is nil, all activities will be no-ops (Langfuse disabled).
func NewLangfuseActivities(ingestion *langfuse.Ingestion, logger logging.Logger) *LangfuseActivities {
	return &LangfuseActivities{
		ingestion: ingestion,
		logger:    logger,
	}
}

// newLangfuseID generates a fresh UUID for use as Langfuse observation IDs.
func newLangfuseID() string {
	return uuid.New().String()
}

// CreateLangfuseTrace creates a root trace in Langfuse for a pipeline run.
// This is the top-level container for all pipeline observations.
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) CreateLangfuseTrace(ctx context.Context, input workflows.CreateLangfuseTraceInput) (*workflows.CreateLangfuseTraceOutput, error) {
	logger := activity.GetLogger(ctx)

	if a.ingestion == nil {
		logger.Debug("Langfuse not configured — skipping CreateLangfuseTrace")
		return &workflows.CreateLangfuseTraceOutput{TraceID: input.TraceID}, nil
	}

	a.ingestion.CreateTrace(langfuse.TraceEvent{
		ID:        input.TraceID,
		Name:      input.Name,
		Tags:      input.Tags,
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"content_id": input.ContentID,
			"tenant_id":  input.TenantID,
		},
	})

	if err := a.ingestion.Flush(ctx); err != nil {
		// Non-fatal: log and continue
		logger.Warn("Langfuse CreateLangfuseTrace flush failed", "error", err)
		return &workflows.CreateLangfuseTraceOutput{TraceID: input.TraceID}, nil
	}

	return &workflows.CreateLangfuseTraceOutput{TraceID: input.TraceID}, nil
}

// ReportLangfusePhase creates a span observation in Langfuse for a pipeline phase.
// The span captures the wall-clock duration of the phase.
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) ReportLangfusePhase(ctx context.Context, input workflows.ReportLangfusePhaseInput) error {
	logger := activity.GetLogger(ctx)

	if a.ingestion == nil {
		logger.Debug("Langfuse not configured — skipping ReportLangfusePhase")
		return nil
	}

	a.ingestion.CreateSpan(langfuse.SpanEvent{
		ID:        input.PhaseID,
		TraceID:   input.TraceID,
		ParentID:  "", // direct child of trace (no parent observation)
		Name:      input.PhaseName,
		StartTime: input.StartTime,
		EndTime:   input.EndTime,
	})

	if err := a.ingestion.Flush(ctx); err != nil {
		// Non-fatal: log and continue
		logger.Warn("Langfuse ReportLangfusePhase flush failed", "error", err, "phase", input.PhaseName)
	}

	return nil
}

// ReportLangfuseGeneration creates a generation observation in Langfuse nested under a phase span.
// A generation captures an individual LLM/AI call with its model, tokens, and I/O.
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) ReportLangfuseGeneration(ctx context.Context, input workflows.ReportLangfuseGenerationInput) error {
	logger := activity.GetLogger(ctx)

	if a.ingestion == nil {
		logger.Debug("Langfuse not configured — skipping ReportLangfuseGeneration")
		return nil
	}

	a.ingestion.CreateGeneration(langfuse.GenerationEvent{
		ID:               newLangfuseID(),
		TraceID:          input.TraceID,
		ParentID:         input.PhaseID,
		Name:             input.Name,
		Model:            input.Model,
		Input:            input.Input,
		Output:           input.Output,
		PromptTokens:     input.InputTokens,
		CompletionTokens: input.OutputTokens,
		StartTime:        input.StartTime,
		EndTime:          input.EndTime,
	})

	if err := a.ingestion.Flush(ctx); err != nil {
		// Non-fatal: log and continue
		logger.Warn("Langfuse ReportLangfuseGeneration flush failed", "error", err, "name", input.Name)
	}

	return nil
}

// FinishLangfuseTrace performs a final flush of any remaining buffered events.
// This ensures all events are delivered to Langfuse before the workflow completes.
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) FinishLangfuseTrace(ctx context.Context, input workflows.FinishLangfuseTraceInput) error {
	logger := activity.GetLogger(ctx)

	if a.ingestion == nil {
		logger.Debug("Langfuse not configured — skipping FinishLangfuseTrace")
		return nil
	}

	if err := a.ingestion.Flush(ctx); err != nil {
		// Non-fatal: log and continue
		logger.Warn("Langfuse FinishLangfuseTrace flush failed", "error", err, "trace_id", input.TraceID)
	}

	return nil
}
