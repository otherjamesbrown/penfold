package temporal

import (
	"context"
	"encoding/json"
	"time"
)

// PipelineStageConfig describes a stage's configuration from pipeline_definitions.
// This is the canonical version used by StageExecutor implementations.
// The services/worker/workflows package has a parallel definition used by the
// existing workflow; both will be reconciled when the generic dispatch loop lands.
type PipelineStageConfig struct {
	Stage          string   `json:"stage"`
	StageKind      string   `json:"stage_kind"`
	PersistKey     string   `json:"persist_key,omitempty"`
	StageOrder     int      `json:"stage_order"`
	Enabled        bool     `json:"enabled"`
	SkipWhenLow    bool     `json:"skip_when_low"`
	Optional       bool     `json:"optional"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	ModelOverride  string   `json:"model_override,omitempty"`
	PromptOverride int32    `json:"prompt_override,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"` // Stage names that must complete successfully before this stage runs
}

// StageOutput is the standard envelope returned by every StageExecutor.
// All fields are exported and JSON-tagged for Temporal serialization compatibility.
// RawData carries the stage-specific payload; downstream stages unmarshal it as needed.
type StageOutput struct {
	Success         bool            `json:"success"`
	Error           string          `json:"error,omitempty"`            // Non-empty when Success is false
	Duration        time.Duration   `json:"duration_ns"`                // Execution duration in nanoseconds
	RawData         json.RawMessage `json:"raw_data,omitempty"`         // Stage-specific payload
	LangfusePhaseID string          `json:"langfuse_phase_id,omitempty"`
	StageName       string          `json:"stage_name"`
	StageKind       string          `json:"stage_kind"`
}

// StageInput carries the parsed document content and accumulated outputs from prior
// stages into each StageExecutor. All fields are exported for Temporal serialization.
type StageInput struct {
	TenantID        string                 `json:"tenant_id"`
	SourceID        int64                  `json:"source_id"`
	ContentID       string                 `json:"content_id,omitempty"`
	JobID           string                 `json:"job_id"`
	Content         string                 `json:"content"`                      // Parsed document text
	PreviousOutputs map[string]StageOutput `json:"previous_outputs,omitempty"`   // Keyed by stage name
	LangfuseTraceID string                 `json:"langfuse_trace_id,omitempty"`
}

// StageExecutor is the interface implemented by each stage-kind executor.
// The dispatch loop calls Execute for every enabled stage in pipeline_definitions order.
// Implementors must be deterministic — no random, time, or goroutine side-effects outside
// of workflow.ExecuteActivity calls (Temporal determinism requirement).
type StageExecutor interface {
	Execute(ctx context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error)
}
