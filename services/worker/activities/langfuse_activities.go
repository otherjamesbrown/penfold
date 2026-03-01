// Package activities provides Langfuse ingestion activities for the pipeline workflow.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// LangfuseActivities provides Temporal activities for reporting pipeline runs to Langfuse.
// If ingestion is nil (Langfuse not configured), all activities become no-ops.
type LangfuseActivities struct {
	ingestion *langfuse.Ingestion
	logger    logging.Logger
	db        *pgxpool.Pool
}

// NewLangfuseActivities creates a new LangfuseActivities.
// If ingestion is nil, all activities will be no-ops (Langfuse disabled).
func NewLangfuseActivities(ingestion *langfuse.Ingestion, logger logging.Logger) *LangfuseActivities {
	return &LangfuseActivities{
		ingestion: ingestion,
		logger:    logger,
	}
}

// WithDB sets the database pool for activities that require database access (e.g. PersistLangfuseTraceID).
// If db is nil, those activities will be no-ops.
func (a *LangfuseActivities) WithDB(db *pgxpool.Pool) *LangfuseActivities {
	a.db = db
	return a
}

// newLangfuseID generates a fresh UUID for use as Langfuse observation IDs.
func newLangfuseID() string {
	return uuid.New().String()
}

// CreateLangfuseTrace creates a root trace in Langfuse for a pipeline run.
// This is the top-level container for all pipeline observations.
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) CreateLangfuseTrace(ctx context.Context, input workflows.CreateLangfuseTraceInput) (*workflows.CreateLangfuseTraceOutput, error) {
	if a.ingestion == nil {
		a.logger.Debug("Langfuse not configured — skipping CreateLangfuseTrace")
		return &workflows.CreateLangfuseTraceOutput{TraceID: input.TraceID}, nil
	}

	// Resolve Environment: use TenantName if set, fall back to TenantID.
	env := input.TenantName
	if env == "" {
		env = input.TenantID
	}

	// Build enriched metadata map starting with required fields.
	metadata := map[string]any{
		"content_id": input.ContentID,
		"tenant_id":  input.TenantID,
	}
	if input.SourceSystem != "" {
		metadata["source_system"] = input.SourceSystem
	}
	if input.Subject != "" {
		metadata["subject"] = input.Subject
	}
	if input.ContentType != "" {
		metadata["content_type"] = input.ContentType
	}

	now := time.Now()

	a.ingestion.CreateTrace(langfuse.TraceEvent{
		ID:          input.TraceID,
		Name:        input.Name,
		Tags:        input.Tags,
		Timestamp:   now,
		Environment: env,
		Metadata:    metadata,
	})

	// Create a root SPAN observation that wraps the entire pipeline run.
	// FinishLangfuseTrace will close this span with an EndTime, giving the
	// trace a real duration instead of near-zero (pf-1bfbaf).
	rootSpanID := newLangfuseID()
	a.ingestion.CreateSpan(langfuse.SpanEvent{
		ID:        rootSpanID,
		TraceID:   input.TraceID,
		Name:      input.Name,
		StartTime: now,
	})

	// Events are buffered; FinishLangfuseTrace will flush the batch at pipeline end.
	return &workflows.CreateLangfuseTraceOutput{TraceID: input.TraceID, RootSpanID: rootSpanID}, nil
}

// ReportLangfusePhase creates a span observation in Langfuse for a pipeline phase.
// The span captures the wall-clock duration of the phase.
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) ReportLangfusePhase(ctx context.Context, input workflows.ReportLangfusePhaseInput) error {
	if a.ingestion == nil {
		a.logger.Debug("Langfuse not configured — skipping ReportLangfusePhase")
		return nil
	}

	a.ingestion.CreateSpan(langfuse.SpanEvent{
		ID:        input.PhaseID,
		TraceID:   input.TraceID,
		ParentID:  input.ParentSpanID, // nest under root pipeline span if set
		Name:      input.PhaseName,
		StartTime: input.StartTime,
		EndTime:   input.EndTime,
	})

	if err := a.ingestion.Flush(ctx); err != nil {
		// Non-fatal: log and continue
		a.logger.Warn("Langfuse ReportLangfusePhase flush failed", logging.Err(err), logging.F("phase", input.PhaseName))
	}

	return nil
}

// FinishLangfuseTrace closes the root pipeline SPAN (if set) and performs a
// final flush of any remaining buffered events. This gives the pipeline trace
// a real duration instead of near-zero (pf-1bfbaf).
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) FinishLangfuseTrace(ctx context.Context, input workflows.FinishLangfuseTraceInput) error {
	if a.ingestion == nil {
		a.logger.Debug("Langfuse not configured — skipping FinishLangfuseTrace")
		return nil
	}

	// Close the root SPAN observation with the current time as EndTime.
	if input.RootSpanID != "" {
		a.ingestion.UpdateSpan(input.RootSpanID, time.Now())
	}

	if err := a.ingestion.Flush(ctx); err != nil {
		// Non-fatal: log and continue
		a.logger.Warn("Langfuse FinishLangfuseTrace flush failed", logging.Err(err), logging.F("trace_id", input.TraceID))
	}

	return nil
}

// UpdateLangfuseTraceTags updates the tags on an existing Langfuse trace.
// Uses the Langfuse upsert semantics (trace-create with same ID updates the trace).
// If Langfuse is not configured, this is a no-op.
func (a *LangfuseActivities) UpdateLangfuseTraceTags(ctx context.Context, input workflows.UpdateLangfuseTraceTagsInput) error {
	if a.ingestion == nil {
		a.logger.Debug("Langfuse not configured — skipping UpdateLangfuseTraceTags")
		return nil
	}

	a.ingestion.UpdateTrace(input.TraceID, input.Tags, input.Name)

	if err := a.ingestion.Flush(ctx); err != nil {
		a.logger.Warn("Langfuse UpdateLangfuseTraceTags flush failed", logging.Err(err), logging.F("trace_id", input.TraceID))
	}

	return nil
}

// UpdateLangfuseTraceMetadata updates metadata on an existing Langfuse trace.
func (a *LangfuseActivities) UpdateLangfuseTraceMetadata(ctx context.Context, input workflows.UpdateLangfuseTraceMetadataInput) error {
	if a.ingestion == nil {
		a.logger.Debug("Langfuse not configured — skipping UpdateLangfuseTraceMetadata")
		return nil
	}

	a.ingestion.UpdateTraceMetadata(input.TraceID, input.Metadata)

	if err := a.ingestion.Flush(ctx); err != nil {
		a.logger.Warn("Langfuse UpdateLangfuseTraceMetadata flush failed", logging.Err(err), logging.F("trace_id", input.TraceID))
	}

	return nil
}

// PersistLangfuseTraceID persists the Langfuse trace ID back to the sources table.
// If no database pool is configured, this is a no-op.
func (a *LangfuseActivities) PersistLangfuseTraceID(ctx context.Context, input workflows.PersistLangfuseTraceIDInput) error {
	if a.db == nil {
		a.logger.Debug("No DB configured — skipping PersistLangfuseTraceID")
		return nil
	}

	_, err := a.db.Exec(ctx,
		"UPDATE sources SET langfuse_trace_id = $1 WHERE id = $2",
		input.TraceID, input.SourceID,
	)
	if err != nil {
		return fmt.Errorf("PersistLangfuseTraceID: %w", err)
	}

	return nil
}
