// Package workflows provides Temporal error classification utilities.
package workflows

import (
	"errors"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
)

// classifyTemporalError returns a string classification for a Temporal error,
// suitable for use as error.type in Langfuse stage spans. This gives operators
// visibility into why a stage failed (heartbeat timeout, LLM error, etc.).
func classifyTemporalError(err error) string {
	var timeoutErr *temporal.TimeoutError
	if errors.As(err, &timeoutErr) {
		switch timeoutErr.TimeoutType() {
		case enumspb.TIMEOUT_TYPE_HEARTBEAT:
			return "heartbeat_timeout"
		case enumspb.TIMEOUT_TYPE_START_TO_CLOSE:
			return "start_to_close_timeout"
		case enumspb.TIMEOUT_TYPE_SCHEDULE_TO_CLOSE:
			return "schedule_to_close_timeout"
		default:
			return "timeout_unknown"
		}
	}
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		if appErr.NonRetryable() {
			return "non_retryable: " + appErr.Type()
		}
		return "application_error: " + appErr.Type()
	}
	var cancelErr *temporal.CanceledError
	if errors.As(err, &cancelErr) {
		return "cancelled"
	}
	return "unknown"
}
