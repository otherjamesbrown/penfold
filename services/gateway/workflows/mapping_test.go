package workflows

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
	commonv1 "github.com/otherjamesbrown/penfold/api/proto/common/v1"
)

func TestMapProcessEmailToIngestRequest(t *testing.T) {
	t.Run("nil request returns error", func(t *testing.T) {
		result, err := MapProcessEmailToIngestRequest(nil)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request is required")
	})

	t.Run("missing tenant_id returns error", func(t *testing.T) {
		req := &gatewayv1.ProcessEmailRequest{
			MessageId: "msg-123",
		}
		result, err := MapProcessEmailToIngestRequest(req)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tenant_id is required")
	})

	t.Run("missing message_id returns error", func(t *testing.T) {
		req := &gatewayv1.ProcessEmailRequest{
			TenantId: "tenant-1",
		}
		result, err := MapProcessEmailToIngestRequest(req)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "message_id is required")
	})

	t.Run("valid request maps correctly", func(t *testing.T) {
		fromName := "John Doe"
		subject := "Test Email"
		req := &gatewayv1.ProcessEmailRequest{
			TenantId:  "tenant-1",
			MessageId: "msg-123",
			FromEmail: "john@example.com",
			FromName:  &fromName,
			Subject:   &subject,
			ThreadId:  "thread-456",
			Metadata: map[string]string{
				"label": "inbox",
			},
		}

		result, err := MapProcessEmailToIngestRequest(req)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "tenant-1", result.TenantID)
		assert.Equal(t, "email", result.SourceType)
		assert.Equal(t, "msg-123", result.SourceID)

		// Check metadata
		assert.Equal(t, "john@example.com", result.Metadata["from_email"])
		assert.Equal(t, "John Doe", result.Metadata["from_name"])
		assert.Equal(t, "Test Email", result.Metadata["subject"])
		assert.Equal(t, "thread-456", result.Metadata["thread_id"])
		assert.Equal(t, "inbox", result.Metadata["label"])
	})

	t.Run("optional fields are handled correctly", func(t *testing.T) {
		req := &gatewayv1.ProcessEmailRequest{
			TenantId:  "tenant-1",
			MessageId: "msg-123",
		}

		result, err := MapProcessEmailToIngestRequest(req)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should not have from_name or subject since they're nil
		_, hasFromName := result.Metadata["from_name"]
		_, hasSubject := result.Metadata["subject"]
		assert.False(t, hasFromName)
		assert.False(t, hasSubject)
	})
}

func TestMapIngestResultToProcessEmailResponse(t *testing.T) {
	t.Run("successful result", func(t *testing.T) {
		resp := MapIngestResultToProcessEmailResponse("wf-123", 456, nil)

		assert.Equal(t, "wf-123", resp.WorkflowId)
		assert.Equal(t, int64(456), resp.SourceId)
		assert.Equal(t, commonv1.Status_STATUS_PENDING, resp.Status)
		assert.Equal(t, "Workflow started successfully", resp.Message)
	})

	t.Run("error result", func(t *testing.T) {
		err := errors.New("workflow failed")
		resp := MapIngestResultToProcessEmailResponse("wf-123", 0, err)

		assert.Equal(t, "wf-123", resp.WorkflowId)
		assert.Equal(t, commonv1.Status_STATUS_FAILED, resp.Status)
		assert.Equal(t, "workflow failed", resp.Message)
	})
}

func TestMapWorkflowStatusToAPIStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected commonv1.Status
	}{
		{"Running", commonv1.Status_STATUS_IN_PROGRESS},
		{"Completed", commonv1.Status_STATUS_COMPLETED},
		{"Failed", commonv1.Status_STATUS_FAILED},
		{"Canceled", commonv1.Status_STATUS_CANCELLED},
		{"Terminated", commonv1.Status_STATUS_FAILED},
		{"TimedOut", commonv1.Status_STATUS_FAILED},
		{"Unknown", commonv1.Status_STATUS_UNKNOWN},
		{"", commonv1.Status_STATUS_UNKNOWN},
		{"InvalidStatus", commonv1.Status_STATUS_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := MapWorkflowStatusToAPIStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapAPIStatusToString(t *testing.T) {
	tests := []struct {
		status   commonv1.Status
		expected string
	}{
		{commonv1.Status_STATUS_UNKNOWN, "unknown"},
		{commonv1.Status_STATUS_PENDING, "pending"},
		{commonv1.Status_STATUS_IN_PROGRESS, "in_progress"},
		{commonv1.Status_STATUS_COMPLETED, "completed"},
		{commonv1.Status_STATUS_FAILED, "failed"},
		{commonv1.Status_STATUS_CANCELLED, "cancelled"},
		{commonv1.Status(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := MapAPIStatusToString(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIngestRequestOptions(t *testing.T) {
	opts := IngestRequestOptions{
		Priority:    1,
		Tags:        []string{"urgent", "email"},
		CallbackURL: "https://example.com/callback",
		Options:     map[string]string{"key": "value"},
	}

	assert.Equal(t, 1, opts.Priority)
	assert.Len(t, opts.Tags, 2)
	assert.Equal(t, "https://example.com/callback", opts.CallbackURL)
	assert.Equal(t, "value", opts.Options["key"])
}

func TestSyncRequestOptions(t *testing.T) {
	opts := SyncRequestOptions{
		Force:     true,
		DryRun:    false,
		BatchSize: 100,
		Options:   map[string]string{"key": "value"},
	}

	assert.True(t, opts.Force)
	assert.False(t, opts.DryRun)
	assert.Equal(t, 100, opts.BatchSize)
}

func TestReviewRequestOptions(t *testing.T) {
	opts := ReviewRequestOptions{
		IncludeArchived: true,
		MaxItems:        50,
		Categories:      []string{"email", "meeting"},
		Options:         map[string]string{"key": "value"},
	}

	assert.True(t, opts.IncludeArchived)
	assert.Equal(t, 50, opts.MaxItems)
	assert.Len(t, opts.Categories, 2)
}

func TestAnalysisRequestOptions(t *testing.T) {
	opts := AnalysisRequestOptions{
		Depth:      3,
		Confidence: 0.95,
		IncludeRaw: true,
		Options:    map[string]string{"key": "value"},
	}

	assert.Equal(t, 3, opts.Depth)
	assert.Equal(t, 0.95, opts.Confidence)
	assert.True(t, opts.IncludeRaw)
}

func TestWorkflowResult(t *testing.T) {
	result := WorkflowResult{
		WorkflowID: "wf-123",
		RunID:      "run-456",
		Status:     "Completed",
		Output:     map[string]interface{}{"key": "value"},
		Error:      "",
	}

	assert.Equal(t, "wf-123", result.WorkflowID)
	assert.Equal(t, "run-456", result.RunID)
	assert.Equal(t, "Completed", result.Status)
	assert.Equal(t, "value", result.Output["key"])
	assert.Empty(t, result.Error)
}

func TestMapWorkflowResultToAPIResponse(t *testing.T) {
	t.Run("successful result", func(t *testing.T) {
		result := &WorkflowResult{
			WorkflowID: "wf-123",
			RunID:      "run-456",
			Status:     "Completed",
			Output:     map[string]interface{}{"count": 10},
		}

		resp := MapWorkflowResultToAPIResponse(result)

		assert.Equal(t, "wf-123", resp["workflow_id"])
		assert.Equal(t, "run-456", resp["run_id"])
		assert.Equal(t, "Completed", resp["status"])
		assert.NotNil(t, resp["output"])
		_, hasError := resp["error"]
		assert.False(t, hasError)
	})

	t.Run("error result", func(t *testing.T) {
		result := &WorkflowResult{
			WorkflowID: "wf-123",
			RunID:      "run-456",
			Status:     "Failed",
			Error:      "workflow failed",
		}

		resp := MapWorkflowResultToAPIResponse(result)

		assert.Equal(t, "wf-123", resp["workflow_id"])
		assert.Equal(t, "Failed", resp["status"])
		assert.Equal(t, "workflow failed", resp["error"])
	})

	t.Run("result without output", func(t *testing.T) {
		result := &WorkflowResult{
			WorkflowID: "wf-123",
			RunID:      "run-456",
			Status:     "Running",
		}

		resp := MapWorkflowResultToAPIResponse(result)

		_, hasOutput := resp["output"]
		assert.False(t, hasOutput)
	})
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, 3, cfg.MaxAttempts)
	assert.Equal(t, 2.0, cfg.BackoffFactor)
	assert.Equal(t, 1000, cfg.InitialInterval)
	assert.Equal(t, 60000, cfg.MaxInterval)
}

func TestRetryConfig_Fields(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:     5,
		BackoffFactor:   1.5,
		InitialInterval: 500,
		MaxInterval:     30000,
	}

	assert.Equal(t, 5, cfg.MaxAttempts)
	assert.Equal(t, 1.5, cfg.BackoffFactor)
	assert.Equal(t, 500, cfg.InitialInterval)
	assert.Equal(t, 30000, cfg.MaxInterval)
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("TEST_CODE", "test message", true)

	assert.Equal(t, "TEST_CODE", resp.Code)
	assert.Equal(t, "test message", resp.Message)
	assert.True(t, resp.Retryable)
	assert.Nil(t, resp.Details)
	assert.Equal(t, 0, resp.RetryAfter)
}

func TestErrorResponse_WithDetails(t *testing.T) {
	resp := NewErrorResponse("TEST_CODE", "test message", false)
	details := map[string]string{"field": "value"}
	result := resp.WithDetails(details)

	assert.Same(t, resp, result) // Should return same instance
	assert.Equal(t, "value", resp.Details["field"])
}

func TestErrorResponse_WithRetryAfter(t *testing.T) {
	resp := NewErrorResponse("TEST_CODE", "test message", true)
	result := resp.WithRetryAfter(5000)

	assert.Same(t, resp, result) // Should return same instance
	assert.Equal(t, 5000, resp.RetryAfter)
}

func TestErrorResponse_Error(t *testing.T) {
	resp := NewErrorResponse("TEST_CODE", "test message", false)
	errStr := resp.Error()

	assert.Equal(t, "TEST_CODE: test message", errStr)
}

func TestErrorResponse_Chaining(t *testing.T) {
	resp := NewErrorResponse("TEST_CODE", "test message", true).
		WithDetails(map[string]string{"key": "value"}).
		WithRetryAfter(3000)

	assert.Equal(t, "TEST_CODE", resp.Code)
	assert.Equal(t, "value", resp.Details["key"])
	assert.Equal(t, 3000, resp.RetryAfter)
}

func TestErrorCodes(t *testing.T) {
	assert.Equal(t, "INVALID_INPUT", ErrCodeInvalidInput)
	assert.Equal(t, "NOT_FOUND", ErrCodeNotFound)
	assert.Equal(t, "ALREADY_EXISTS", ErrCodeAlreadyExists)
	assert.Equal(t, "TIMEOUT", ErrCodeTimeout)
	assert.Equal(t, "INTERNAL_ERROR", ErrCodeInternal)
	assert.Equal(t, "UNAVAILABLE", ErrCodeUnavailable)
	assert.Equal(t, "PERMISSION_DENIED", ErrCodePermissionDenied)
	assert.Equal(t, "RATE_LIMITED", ErrCodeRateLimited)
}

func TestMapErrorToResponse(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		resp := MapErrorToResponse(nil)
		assert.Nil(t, resp)
	})

	t.Run("not found error", func(t *testing.T) {
		err := errors.New("resource not found")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeNotFound, resp.Code)
		assert.False(t, resp.Retryable)
	})

	t.Run("already exists error", func(t *testing.T) {
		err := errors.New("workflow already exists")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeAlreadyExists, resp.Code)
		assert.False(t, resp.Retryable)
	})

	t.Run("already started error", func(t *testing.T) {
		err := errors.New("workflow already started")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeAlreadyExists, resp.Code)
	})

	t.Run("timeout error", func(t *testing.T) {
		err := errors.New("operation timeout")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeTimeout, resp.Code)
		assert.True(t, resp.Retryable)
		assert.Equal(t, 5000, resp.RetryAfter)
	})

	t.Run("invalid error", func(t *testing.T) {
		err := errors.New("invalid argument")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeInvalidInput, resp.Code)
		assert.False(t, resp.Retryable)
	})

	t.Run("required error", func(t *testing.T) {
		err := errors.New("tenant_id is required")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeInvalidInput, resp.Code)
	})

	t.Run("unavailable error", func(t *testing.T) {
		err := errors.New("service unavailable")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeUnavailable, resp.Code)
		assert.True(t, resp.Retryable)
		assert.Equal(t, 10000, resp.RetryAfter)
	})

	t.Run("connection error", func(t *testing.T) {
		err := errors.New("connection refused")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeUnavailable, resp.Code)
	})

	t.Run("permission denied error", func(t *testing.T) {
		err := errors.New("permission denied")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodePermissionDenied, resp.Code)
		assert.False(t, resp.Retryable)
	})

	t.Run("rate limit error", func(t *testing.T) {
		err := errors.New("rate limit exceeded")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeRateLimited, resp.Code)
		assert.True(t, resp.Retryable)
		assert.Equal(t, 60000, resp.RetryAfter)
	})

	t.Run("unknown error defaults to internal", func(t *testing.T) {
		err := errors.New("something unexpected happened")
		resp := MapErrorToResponse(err)

		assert.Equal(t, ErrCodeInternal, resp.Code)
		assert.False(t, resp.Retryable)
	})
}
