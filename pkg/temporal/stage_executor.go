package temporal

import (
	"context"
	"encoding/json"
	"time"
)

// PipelineStageConfig describes a single pipeline stage's execution configuration,
// as read from pipeline_definitions. This is the type used by all new executor
// implementations (EmbeddingExecutor, StructuredExtractExecutor, LLMStageExecutor)
// and the generic dispatch loop (pf-7f13e6).
type PipelineStageConfig struct {
	Stage          string   `json:"stage"`
	StageKind      string   `json:"stage_kind"`
	StageOrder     int      `json:"stage_order"`
	Enabled        bool     `json:"enabled"`
	SkipWhenLow    bool     `json:"skip_when_low"`
	Optional       bool     `json:"optional"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	ModelOverride  string   `json:"model_override,omitempty"`
	PromptOverride int32    `json:"prompt_override,omitempty"`
	PersistKey     string   `json:"persist_key,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
}

// StageConfig is the original stage configuration type used by CodeOnlyExecutor
// and its test suite. Kept for backward compatibility — new executors use
// PipelineStageConfig. CodeOnlyExecutor will migrate in the dispatch loop task (pf-7f13e6).
type StageConfig struct {
	Stage          string `json:"stage"`
	StageKind      string `json:"stage_kind"`
	StageOrder     int    `json:"stage_order"`
	Enabled        bool   `json:"enabled"`
	SkipWhenLow    bool   `json:"skip_when_low"`
	Optional       bool   `json:"optional"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ModelOverride  string `json:"model_override,omitempty"`
	PromptOverride int32  `json:"prompt_override,omitempty"`
	PersistKey     string `json:"persist_key,omitempty"`
}

// StageInput carries everything a StageExecutor needs to execute a single
// pipeline stage. It is JSON-serialisable so the dispatch loop can accumulate
// state across stages.
//
// Legacy fields (Config, Payload, SkipDeep) are used by CodeOnlyExecutor.
// New fields (TenantID through LangfuseTraceID) are used by EmbeddingExecutor,
// StructuredExtractExecutor, LLMStageExecutor, and the generic dispatch loop.
type StageInput struct {
	// Legacy fields — used by CodeOnlyExecutor (workflow-native pattern).
	Config   StageConfig `json:"config,omitempty"`
	Payload  interface{} `json:"payload,omitempty"`
	SkipDeep bool        `json:"skip_deep,omitempty"`

	// Fields for the generic executor-based pipeline.
	TenantID        string                 `json:"tenant_id,omitempty"`
	SourceID        int64                  `json:"source_id,omitempty"`
	ContentID       string                 `json:"content_id,omitempty"`
	JobID           string                 `json:"job_id,omitempty"`
	Content         string                 `json:"content,omitempty"`
	PreviousOutputs map[string]StageOutput `json:"previous_outputs,omitempty"`
	LangfuseTraceID string                 `json:"langfuse_trace_id,omitempty"`
}

// StageOutput is the standardised envelope produced by all stage executors.
// All fields are JSON-serialisable so the dispatch loop can accumulate outputs
// and pass them as PreviousOutputs to downstream stages.
type StageOutput struct {
	// Success is true when the stage completed without a blocking error.
	// Optional stages set Success=true even when the underlying call failed.
	Success bool `json:"success"`
	// Skipped is true when the stage was bypassed (e.g. skip_when_low + SkipDeep).
	Skipped bool `json:"skipped"`
	// SkipReason explains why the stage was skipped.
	SkipReason string `json:"skip_reason,omitempty"`
	// Error captures the error message. Set for both blocking and non-blocking
	// (Optional) failures; only non-blocking failures have Success=true.
	Error string `json:"error,omitempty"`
	// RawData is the JSON-serialised output from the underlying activity or AI call.
	// Nil on failure or when the stage was skipped.
	RawData json.RawMessage `json:"raw_data,omitempty"`
	// Duration is the wall-clock time taken by the stage. Set by the executor
	// (or by the LangfuseWrappedExecutor in the registry).
	Duration time.Duration `json:"duration,omitempty"`
	// LangfusePhaseID is set by LangfuseWrappedExecutor after each execution.
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
	// StageName is the stage identifier (e.g. "triage", "embed").
	StageName string `json:"stage_name,omitempty"`
	// StageKind identifies the executor kind (e.g. "llm", "embedding").
	StageKind string `json:"stage_kind,omitempty"`
}

// StageExecutor is implemented by all generic executor types (EmbeddingExecutor,
// StructuredExtractExecutor, LLMStageExecutor). The context.Context may carry an
// embedded workflow.Context via WithWorkflowContext for executors that call
// workflow.ExecuteActivity internally.
type StageExecutor interface {
	Execute(ctx context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error)
}
