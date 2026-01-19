// Package temporal provides Temporal client factory and utilities for Penfold.
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
// (cloud API calls, typically 30-60 seconds).
//
// Configuration:
//   - 2 minute start-to-close timeout
//   - 5 minute schedule-to-close timeout (allows for retries)
//   - 15 second heartbeat interval
//   - 2 retries with exponential backoff (5s, 10s)
//   - Fewer retries because LLM operations are expensive
func LLMActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout:    2 * time.Minute,
		ScheduleToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:       15 * time.Second,
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
