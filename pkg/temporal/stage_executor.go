package temporal

import (
	"encoding/json"

	"go.temporal.io/sdk/workflow"
)

// StageConfig describes a single pipeline stage's execution configuration,
// as read from pipeline_definitions. Executors receive this to determine
// timeout, skip, and optional behaviour without accessing the DB directly.
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

// StageInput is passed to StageExecutor.Execute. It combines the stage's
// DB-driven configuration with the runtime payload (the activity's input).
type StageInput struct {
	// Config is the stage's configuration from pipeline_definitions.
	Config StageConfig
	// Payload is the activity-specific input. The executor passes it verbatim
	// to workflow.ExecuteActivity; Temporal serialises it as JSON.
	Payload interface{}
	// SkipDeep is true when triage returned SkipDeep. Stages marked
	// SkipWhenLow are bypassed when SkipDeep is true.
	SkipDeep bool
}

// StageOutput is the standardised envelope produced by all stage executors.
// All fields are JSON-serialisable so the dispatch loop can accumulate outputs.
type StageOutput struct {
	// Success is true when the stage completed without a blocking error.
	// Optional stages set Success=true even when the activity failed.
	Success bool `json:"success"`
	// Skipped is true when the stage was bypassed (e.g. skip_when_low).
	Skipped bool `json:"skipped"`
	// SkipReason explains why the stage was skipped.
	SkipReason string `json:"skip_reason,omitempty"`
	// Error captures the activity error message. Set for both blocking and
	// non-blocking (Optional) failures; only non-blocking failures have Success=true.
	Error string `json:"error,omitempty"`
	// RawData is the JSON-serialised output from the underlying activity.
	// Nil on failure or when the stage was skipped.
	RawData json.RawMessage `json:"raw_data,omitempty"`
	// DurationMs is the wall-clock duration of the stage in milliseconds
	// as observed by the workflow (workflow.Now subtraction).
	DurationMs int64 `json:"duration_ms"`
	// LangfusePhaseID is set by the executor registry when Langfuse wrapping is applied.
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
}

// StageExecutor executes a single pipeline stage within a Temporal workflow.
// Implementations call workflow.ExecuteActivity and therefore must only be
// invoked from within workflow code.
type StageExecutor interface {
	Execute(ctx workflow.Context, input StageInput) (StageOutput, error)
}
