// Package temporal provides activity and workflow option presets for Penfold services.
package temporal

import (
	"fmt"
	"time"

	timeoutpkg "github.com/otherjamesbrown/penfold/pkg/timeout"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Activity option presets for different operation types.
// These provide consistent configuration across all workflows.
//
// NOTE: These functions currently use hardcoded timeout values. In the future,
// they could accept a *timeout.Config parameter to read values from pipeline_config.
// However, Temporal activity options are set at workflow dispatch time in workflow code,
// which cannot access runtime state. For dynamic timeout configuration, consider:
// 1. Passing timeout values through workflow input
// 2. Using workflow signals to update timeout behavior
// 3. Adding a gRPC API for penf to update config values and restart workflows

// FastActivityOptions returns activity options suitable for quick operations
// like database queries or simple transformations.
//
// Configuration:
//   - 30 second timeout
//   - 2 minute schedule-to-close timeout (safety net for retries)
//   - 3 retries with exponential backoff (1s, 2s, 4s)
//   - No heartbeat (operations complete quickly)
func FastActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    30 * time.Second,
		ScheduleToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
}

// EmbeddingActivityOptions returns activity options suitable for embedding
// generation operations (local MLX inference, typically 1-5 seconds).
//
// Configuration:
//   - 30 second timeout
//   - 5 minute schedule-to-close timeout (safety net for retries and heartbeat rescheduling)
//   - 10 second heartbeat interval
//   - 3 retries with exponential backoff (2s, 4s, 8s)
func EmbeddingActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    30 * time.Second,
		ScheduleToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:       10 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
}

// LLMActivityOptions returns activity options suitable for LLM operations
// (local MLX inference, multi-stage pipelines like mention resolution).
//
// Configuration:
//   - 10 minute start-to-close timeout (4-stage pipeline on a queued MLX server)
//   - 15 minute schedule-to-close timeout (allows for retries)
//   - 5 minute heartbeat interval (bulk reprocessing can overwhelm MLX, causing delays)
//   - 2 retries with exponential backoff (5s, 10s)
//   - Fewer retries because LLM operations are expensive
func LLMActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    10 * time.Minute,
		ScheduleToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:       5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2, // Fewer retries for expensive operations
		},
	}
}

// BatchActivityOptions returns activity options suitable for batch processing
// operations that may take longer but should still heartbeat.
//
// Configuration:
//   - 5 minute timeout
//   - 10 minute schedule-to-close timeout (allows for retries)
//   - 30 second heartbeat interval
//   - 2 retries with exponential backoff
func BatchActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    5 * time.Minute,
		ScheduleToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:       30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2,
		},
	}
}

// LongRunningActivityOptions returns activity options for operations that
// may take a significant amount of time (like large file processing).
//
// Configuration:
//   - 30 minute timeout
//   - 1 hour schedule-to-close timeout (allows for retries)
//   - 1 minute heartbeat interval
//   - 1 retry (long operations are usually idempotent)
func LongRunningActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    30 * time.Minute,
		ScheduleToCloseTimeout: 1 * time.Hour,
		HeartbeatTimeout:       1 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    30 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    1,
		},
	}
}

// CustomActivityOptions creates activity options with custom timeouts and retry settings.
func CustomActivityOptions(
	startToClose time.Duration,
	heartbeat time.Duration,
	maxAttempts int32,
	initialInterval time.Duration,
) workflow.ActivityOptions {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: startToClose,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    initialInterval,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    maxAttempts,
		},
	}
	if heartbeat > 0 {
		opts.HeartbeatTimeout = heartbeat
	}
	return opts
}

// DefaultWorkflowOptions returns default workflow execution options.
//
// Configuration:
//   - 24 hour execution timeout
//   - 10 minute run timeout (per run, for continued-as-new)
func DefaultWorkflowOptions(workflowID, taskQueue string) workflow.ChildWorkflowOptions {
	return workflow.ChildWorkflowOptions{
		WorkflowID:               workflowID,
		TaskQueue:                taskQueue,
		WorkflowExecutionTimeout: 24 * time.Hour,
		WorkflowRunTimeout:       10 * time.Minute,
	}
}

// LongRunningWorkflowOptions returns workflow options for long-running workflows.
//
// Configuration:
//   - 7 day execution timeout
//   - 1 hour run timeout
func LongRunningWorkflowOptions(workflowID, taskQueue string) workflow.ChildWorkflowOptions {
	return workflow.ChildWorkflowOptions{
		WorkflowID:               workflowID,
		TaskQueue:                taskQueue,
		WorkflowExecutionTimeout: 7 * 24 * time.Hour,
		WorkflowRunTimeout:       1 * time.Hour,
	}
}

// StageActivityOptions returns activity options for a specific pipeline stage,
// reading per-stage timeout values from the timeout config. Falls back to
// category defaults if per-stage keys are not set.
func StageActivityOptions(cfg *timeoutpkg.Config, stage string) workflow.ActivityOptions {
	stcKey := fmt.Sprintf("timeout.stage.%s.start_to_close", stage)
	hbKey := fmt.Sprintf("timeout.stage.%s.heartbeat", stage)

	stc := cfg.Get(stcKey)
	hb := cfg.Get(hbKey)

	// Fall back to category defaults if per-stage not set
	if stc == 0 {
		stc = 120 * time.Second
	}
	if hb == 0 {
		hb = 30 * time.Second
	}

	return workflow.ActivityOptions{
		StartToCloseTimeout: stc,
		HeartbeatTimeout:    hb,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
}

// NonRetryableErrors returns a list of error types that should not be retried.
func NonRetryableErrors() []string {
	return []string{
		"ConfigurationError",
		"ValidationError",
		"NotFoundError",
		"PermissionDeniedError",
		"InvalidArgumentError",
		"PipelineError",
	}
}

// WithNonRetryableErrors modifies activity options to exclude certain errors from retry.
func WithNonRetryableErrors(opts workflow.ActivityOptions, errorTypes ...string) workflow.ActivityOptions {
	if opts.RetryPolicy == nil {
		opts.RetryPolicy = &temporal.RetryPolicy{}
	}
	opts.RetryPolicy.NonRetryableErrorTypes = append(opts.RetryPolicy.NonRetryableErrorTypes, errorTypes...)
	return opts
}
