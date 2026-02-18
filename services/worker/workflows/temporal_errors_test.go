// Package workflows provides tests for Temporal error classification.
package workflows

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
)

// TestClassifyTemporalError verifies that classifyTemporalError correctly identifies
// each Temporal error type and returns the expected classification string.
// These classifications appear as error.type in Langfuse stage spans, giving
// operators visibility into why a stage failed (heartbeat vs timeout vs LLM error).
func TestClassifyTemporalError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "heartbeat timeout",
			err:      temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_HEARTBEAT, nil),
			expected: "heartbeat_timeout",
		},
		{
			name:     "start_to_close timeout",
			err:      temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_START_TO_CLOSE, nil),
			expected: "start_to_close_timeout",
		},
		{
			name:     "schedule_to_close timeout",
			err:      temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_SCHEDULE_TO_CLOSE, nil),
			expected: "schedule_to_close_timeout",
		},
		{
			name:     "retryable application error",
			err:      temporal.NewApplicationError("LLM call failed", "LLMError"),
			expected: "application_error: LLMError",
		},
		{
			name:     "retryable application error with different type",
			err:      temporal.NewApplicationError("rate limit exceeded", "RateLimitError"),
			expected: "application_error: RateLimitError",
		},
		{
			name:     "non-retryable application error",
			err:      temporal.NewNonRetryableApplicationError("invalid input", "ValidationError", nil),
			expected: "non_retryable: ValidationError",
		},
		{
			name:     "non-retryable application error with different type",
			err:      temporal.NewNonRetryableApplicationError("model not found", "ModelNotFound", nil),
			expected: "non_retryable: ModelNotFound",
		},
		{
			name:     "cancelled error",
			err:      temporal.NewCanceledError("user requested cancellation"),
			expected: "cancelled",
		},
		{
			name:     "generic error returns unknown",
			err:      errors.New("something went wrong"),
			expected: "unknown",
		},
		{
			name:     "nil-wrapped generic error returns unknown",
			err:      fmt.Errorf("wrapped: %w", errors.New("inner error")),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyTemporalError(tt.err)
			assert.Equal(t, tt.expected, result,
				"classifyTemporalError(%T) = %q, want %q", tt.err, result, tt.expected)
		})
	}
}

// TestClassifyTemporalError_UnspecifiedTimeout verifies that an unspecified timeout type
// falls through to the "timeout_unknown" classification (not a panic or wrong type).
func TestClassifyTemporalError_UnspecifiedTimeout(t *testing.T) {
	err := temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_UNSPECIFIED, nil)
	result := classifyTemporalError(err)
	assert.Equal(t, "timeout_unknown", result)
}

// TestClassifyTemporalError_ScheduleToStartTimeout verifies schedule_to_start timeout
// falls through to "timeout_unknown" (not one of the explicitly handled types).
func TestClassifyTemporalError_ScheduleToStartTimeout(t *testing.T) {
	err := temporal.NewTimeoutError(enumspb.TIMEOUT_TYPE_SCHEDULE_TO_START, nil)
	result := classifyTemporalError(err)
	// schedule_to_start is not explicitly handled, falls to "timeout_unknown"
	assert.Equal(t, "timeout_unknown", result)
}
