// Package temporal provides activity and workflow option presets for Penfold services.
package temporal

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Activity option presets for different operation types.
// These provide consistent configuration across all workflows.

// FastActivityOptions returns activity options suitable for quick operations
// like database queries or simple transformations.
//
// Configuration:
//   - 30 second timeout
//   - 3 retries with exponential backoff (1s, 2s, 4s)
//   - No heartbeat (operations complete quickly)
func FastActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
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
//   - 10 second heartbeat interval
//   - 3 retries with exponential backoff (2s, 4s, 8s)
func EmbeddingActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		HeartbeatTimeout:    10 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
}

// LLMActivityOptions returns activity options suitable for LLM operations
// (local MLX inference, typically 11-65 seconds).
//
// Configuration:
//   - 3 minute start-to-close timeout
//   - 10 minute schedule-to-close timeout (allows for retries)
//   - 90 second heartbeat interval (AI calls take 11-65s)
//   - 2 retries with exponential backoff (5s, 10s)
//   - Fewer retries because LLM operations are expensive
func LLMActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    3 * time.Minute,
		ScheduleToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:       90 * time.Second,
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
//   - 30 second heartbeat interval
//   - 2 retries with exponential backoff
func BatchActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
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
//   - 1 minute heartbeat interval
//   - 1 retry (long operations are usually idempotent)
func LongRunningActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    1 * time.Minute,
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

// NonRetryableErrors returns a list of error types that should not be retried.
func NonRetryableErrors() []string {
	return []string{
		"ValidationError",
		"NotFoundError",
		"PermissionDeniedError",
		"InvalidArgumentError",
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
