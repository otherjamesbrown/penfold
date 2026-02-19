// Package workflows provides Langfuse activity input/output types used by the pipeline workflow.
package workflows

import "time"

// CreateLangfuseTraceInput is the input for the CreateLangfuseTrace activity.
type CreateLangfuseTraceInput struct {
	TraceID   string   `json:"trace_id"`
	Name      string   `json:"name"`
	ContentID string   `json:"content_id,omitempty"`
	TenantID  string   `json:"tenant_id,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// CreateLangfuseTraceOutput is the output from the CreateLangfuseTrace activity.
type CreateLangfuseTraceOutput struct {
	TraceID string `json:"trace_id"`
}

// ReportLangfusePhaseInput is the input for the ReportLangfusePhase activity.
type ReportLangfusePhaseInput struct {
	PhaseID   string    `json:"phase_id"`
	TraceID   string    `json:"trace_id"`
	PhaseName string    `json:"phase_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// ReportLangfuseGenerationInput is the input for the ReportLangfuseGeneration activity.
type ReportLangfuseGenerationInput struct {
	TraceID      string    `json:"trace_id"`
	PhaseID      string    `json:"phase_id"` // parent span ID
	Name         string    `json:"name"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Input        string    `json:"input,omitempty"`  // prompt / content summary
	Output       string    `json:"output,omitempty"` // completion / result summary
}

// FinishLangfuseTraceInput is the input for the FinishLangfuseTrace activity.
type FinishLangfuseTraceInput struct {
	TraceID string `json:"trace_id"`
}
