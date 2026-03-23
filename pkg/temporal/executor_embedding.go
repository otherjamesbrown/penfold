package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// workflowContextKey is the key used to embed a workflow.Context inside a standard
// context.Context. The dispatch loop sets this so StageExecutor implementations can
// call workflow.ExecuteActivity via the context bridge.
type workflowContextKey struct{}

// WithWorkflowContext returns a context.Context carrying the given workflow.Context.
// The dispatch loop (pf-7f13e6) calls this before invoking each StageExecutor so
// executors can extract the workflow context and call workflow.ExecuteActivity.
func WithWorkflowContext(ctx context.Context, wfCtx workflow.Context) context.Context {
	return context.WithValue(ctx, workflowContextKey{}, wfCtx)
}

// extractWorkflowContext retrieves the workflow.Context embedded by WithWorkflowContext.
func extractWorkflowContext(ctx context.Context) (workflow.Context, bool) {
	wfCtx, ok := ctx.Value(workflowContextKey{}).(workflow.Context)
	return wfCtx, ok
}

// embeddingActivityInput mirrors workflows.GenerateEmbeddingInput.
// Defined locally to avoid a circular import between pkg/temporal and services/worker/workflows.
// JSON tags match exactly so Temporal's JSON codec routes to the correct activity parameter.
type embeddingActivityInput struct {
	TenantID        string `json:"tenant_id"`
	SourceID        int64  `json:"source_id"`
	ContentID       string `json:"content_id,omitempty"`
	Content         string `json:"content"`
	ContentHash     string `json:"content_hash"`
	LangfuseTraceID string `json:"langfuse_trace_id,omitempty"`
	LangfusePhaseID string `json:"langfuse_phase_id,omitempty"`
}

// EmbeddingResult is the stage-specific payload stored in StageOutput.RawData for the
// "embedding" stage_kind. Downstream stages can decode it from PreviousOutputs.
type EmbeddingResult struct {
	EmbeddingID int64 `json:"embedding_id"`
}

// EmbeddingExecutor implements StageExecutor for the "embedding" stage_kind.
// It calls ActivityGenerateContentEmbedding with the document content from StageInput.
//
// Register it under "embedding" in the ExecutorRegistry:
//
//	registry.Register("embedding", &EmbeddingExecutor{})
type EmbeddingExecutor struct{}

// Execute generates a vector embedding for the document content.
//
// Timeout: stage.TimeoutSeconds overrides the default EmbeddingActivityOptions (30s) when > 0.
//
// skip_when_low: the embed stage has SkipWhenLow=false by design; this executor does not
// implement skip logic. Skip-when-low gating is the dispatch loop's responsibility.
//
// Optional handling: when stage.Optional is true and the activity fails, Execute returns a
// failed StageOutput with a nil error so the dispatch loop can continue. When stage.Optional
// is false and the activity fails, both StageOutput.Success=false and a non-nil error are
// returned so the dispatch loop can treat the stage as a pipeline failure.
func (e *EmbeddingExecutor) Execute(ctx context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error) {
	wfCtx, ok := extractWorkflowContext(ctx)
	if !ok {
		return StageOutput{}, fmt.Errorf("embedding executor: workflow context not available")
	}

	opts := EmbeddingActivityOptions()
	if stage.TimeoutSeconds > 0 {
		opts.StartToCloseTimeout = time.Duration(stage.TimeoutSeconds) * time.Second
	}

	actCtx := workflow.WithActivityOptions(wfCtx, opts)

	var embeddingID int64
	err := workflow.ExecuteActivity(actCtx, ActivityGenerateContentEmbedding, embeddingActivityInput{
		TenantID:        input.TenantID,
		SourceID:        input.SourceID,
		ContentID:       input.ContentID,
		Content:         input.Content,
		LangfuseTraceID: input.LangfuseTraceID,
		// ContentHash is not in StageInput; left empty. The activity uses it for
		// provenance tracking only and handles empty values gracefully.
	}).Get(wfCtx, &embeddingID)

	if err != nil {
		out := StageOutput{
			Success:   false,
			Error:     err.Error(),
			StageName: stage.Stage,
			StageKind: stage.StageKind,
		}
		if stage.Optional {
			return out, nil
		}
		return out, err
	}

	raw, jsonErr := json.Marshal(EmbeddingResult{EmbeddingID: embeddingID})
	if jsonErr != nil {
		return StageOutput{}, fmt.Errorf("embedding executor: marshal result: %w", jsonErr)
	}

	return StageOutput{
		Success:   true,
		RawData:   raw,
		StageName: stage.Stage,
		StageKind: stage.StageKind,
	}, nil
}
