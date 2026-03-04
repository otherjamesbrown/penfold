// Package workflows provides Langfuse activity input/output types used by the pipeline workflow.
package workflows

import "time"

// CreateLangfuseTraceInput is the input for the CreateLangfuseTrace activity.
type CreateLangfuseTraceInput struct {
	TraceID      string   `json:"trace_id"`
	Name         string   `json:"name"`
	ContentID    string   `json:"content_id,omitempty"`
	TenantID     string   `json:"tenant_id,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	TenantName   string   `json:"tenant_name,omitempty"`   // For Langfuse Environment; falls back to TenantID if empty
	SourceSystem string   `json:"source_system,omitempty"` // e.g. "gmail", "human_email"
	Subject      string   `json:"subject,omitempty"`       // Email subject line
	ContentType  string   `json:"content_type,omitempty"`  // e.g. "email", "meeting"

	// Content fields for the root span Input display in Langfuse.
	SenderEmail string `json:"sender_email,omitempty"`
	SenderName  string `json:"sender_name,omitempty"`
	Recipients  string `json:"recipients,omitempty"`  // Comma-separated To/CC
	Date        string `json:"date,omitempty"`        // Formatted date string
	BodyPreview string `json:"body_preview,omitempty"` // Truncated body text
}

// PersistLangfuseTraceIDInput is the input for the PersistLangfuseTraceID activity.
type PersistLangfuseTraceIDInput struct {
	SourceID string `json:"source_id"`
	TraceID  string `json:"trace_id"`
}

// CreateLangfuseTraceOutput is the output from the CreateLangfuseTrace activity.
type CreateLangfuseTraceOutput struct {
	TraceID    string `json:"trace_id"`
	RootSpanID string `json:"root_span_id,omitempty"` // Langfuse root SPAN observation ID; used by FinishLangfuseTrace to set EndTime
}

// ReportLangfusePhaseInput is the input for the ReportLangfusePhase activity.
type ReportLangfusePhaseInput struct {
	PhaseID      string    `json:"phase_id"`
	TraceID      string    `json:"trace_id"`
	PhaseName    string    `json:"phase_name"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	ParentSpanID string    `json:"parent_span_id,omitempty"` // Root span ID; nests phase under pipeline root
}

// ReportLangfuseGenerationInput was the input for the ReportLangfuseGeneration activity.
// Deprecated: Removed in pf-37ebe8 — generation reporting now happens in the AI coordinator
// via gRPC metadata. This type is retained only for test code that must compile against it
// to verify the activity is no longer called.
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
	TraceID    string `json:"trace_id"`
	RootSpanID string `json:"root_span_id,omitempty"` // If set, updates the root SPAN observation with EndTime
}

// UpdateLangfuseTraceTagsInput is the input for the UpdateLangfuseTraceTags activity.
type UpdateLangfuseTraceTagsInput struct {
	TraceID string   `json:"trace_id"`
	Tags    []string `json:"tags"`
	Name    string   `json:"name,omitempty"` // Optional: update trace name (e.g. pipeline-aware name)
}

// UpdateLangfuseTraceMetadataInput is the input for the UpdateLangfuseTraceMetadata activity.
type UpdateLangfuseTraceMetadataInput struct {
	TraceID  string         `json:"trace_id"`
	Metadata map[string]any `json:"metadata"`
}
