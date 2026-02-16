package activities

import (
	"context"
	"errors"
	"testing"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"go.temporal.io/sdk/temporal"
)

// TestPipelineError_NotTemporalApplicationError demonstrates that PipelineError
// is not a temporal.ApplicationError, which means Temporal cannot check its type
// against NonRetryableErrorTypes. This causes non-retryable errors to be retried.
//
// After fix: WrapForTemporal converts non-retryable PipelineErrors to ApplicationError.
func TestPipelineError_NotTemporalApplicationError(t *testing.T) {
	// Create a non-retryable PipelineError (parse_error)
	parseErr := errors.New("malformed JSON")
	pe := perrors.ClassifyError(parseErr, "parse")

	// Verify it's a PipelineError
	var pipelineErr *perrors.PipelineError
	if !errors.As(pe, &pipelineErr) {
		t.Fatalf("expected error to be *PipelineError")
	}

	// Check if it's retryable according to PipelineError logic
	retryable := perrors.IsErrorRetryable(pe)
	if retryable {
		t.Errorf("parse_error should be non-retryable according to ErrorCodeRegistry")
	}

	// After fix: WrapForTemporal converts non-retryable PipelineError to ApplicationError
	wrapped := WrapForTemporal(pe)

	var appErr *temporal.ApplicationError
	isAppErr := errors.As(wrapped, &appErr)

	// This SHOULD be true after the fix
	if !isAppErr {
		t.Errorf("non-retryable PipelineError should be convertible to temporal.ApplicationError, "+
			"otherwise Temporal cannot check it against NonRetryableErrorTypes. "+
			"Current behavior: PipelineError is not a temporal.ApplicationError, so it gets retried even when non-retryable.")
	}

	// Verify the ApplicationError type is correct
	if isAppErr && appErr.Type() != "PipelineError" {
		t.Errorf("expected ApplicationError type 'PipelineError', got '%s'", appErr.Type())
	}
}

// TestPipelineError_ContextCancelled demonstrates that context cancellation errors
// are classified as non-retryable but are not recognized by Temporal.
//
// After fix: WrapForTemporal converts non-retryable PipelineErrors to ApplicationError.
func TestPipelineError_ContextCancelled(t *testing.T) {
	// Create a context cancelled error
	pe := perrors.ClassifyError(context.Canceled, "stage1")

	// Verify it's a PipelineError with context_cancelled code
	var pipelineErr *perrors.PipelineError
	if !errors.As(pe, &pipelineErr) {
		t.Fatalf("expected error to be *PipelineError")
	}
	if pipelineErr.Code != perrors.ErrContextCancelled {
		t.Errorf("expected code context_cancelled, got %s", pipelineErr.Code)
	}

	// Check if it's retryable according to PipelineError logic
	retryable := perrors.IsErrorRetryable(pe)
	if retryable {
		t.Errorf("context_cancelled should be non-retryable according to ErrorCodeRegistry")
	}

	// After fix: WrapForTemporal converts non-retryable PipelineError to ApplicationError
	wrapped := WrapForTemporal(pe)

	var appErr *temporal.ApplicationError
	isAppErr := errors.As(wrapped, &appErr)

	if !isAppErr {
		t.Errorf("context_cancelled PipelineError should be temporal.ApplicationError, "+
			"otherwise Temporal will retry cancelled operations")
	}
}

// TestPipelineError_EmptyContent demonstrates that empty content errors
// are classified as non-retryable but are not recognized by Temporal.
//
// After fix: WrapForTemporal converts non-retryable PipelineErrors to ApplicationError.
func TestPipelineError_EmptyContent(t *testing.T) {
	// Create an empty content error
	emptyErr := errors.New("content is empty")
	pe := perrors.ClassifyError(emptyErr, "parse")

	// Verify it's a PipelineError with empty_content code
	var pipelineErr *perrors.PipelineError
	if !errors.As(pe, &pipelineErr) {
		t.Fatalf("expected error to be *PipelineError")
	}
	if pipelineErr.Code != perrors.ErrEmptyContent {
		t.Errorf("expected code empty_content, got %s", pipelineErr.Code)
	}

	// Check if it's retryable according to PipelineError logic
	retryable := perrors.IsErrorRetryable(pe)
	if retryable {
		t.Errorf("empty_content should be non-retryable according to ErrorCodeRegistry")
	}

	// After fix: WrapForTemporal converts non-retryable PipelineError to ApplicationError
	wrapped := WrapForTemporal(pe)

	var appErr *temporal.ApplicationError
	isAppErr := errors.As(wrapped, &appErr)

	if !isAppErr {
		t.Errorf("empty_content PipelineError should be temporal.ApplicationError, "+
			"otherwise Temporal will retry content that will always be empty")
	}
}

// TestPipelineError_RetryableVsNonRetryable demonstrates the inconsistency between
// perrors.IsErrorRetryable() and Temporal's retry policy checking.
//
// This test documents the expected behavior (currently not working).
func TestPipelineError_RetryableVsNonRetryable(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		stage      string
		expectCode perrors.ErrorCode
		expectRet  bool
	}{
		// Non-retryable errors (Retryable=false in ErrorCodeRegistry)
		// Note: ClassifyError doesn't have a pattern for "parse_error" — it will
		// return processing_error. This demonstrates the gap in error classification.
		{
			name:       "processing_error",
			err:        errors.New("malformed JSON"),
			stage:      "parse",
			expectCode: perrors.ErrProcessingError,
			expectRet:  false,
		},
		{
			name:       "empty_content",
			err:        errors.New("content is empty"),
			stage:      "parse",
			expectCode: perrors.ErrEmptyContent,
			expectRet:  false,
		},
		{
			name:       "context_cancelled",
			err:        context.Canceled,
			stage:      "stage1",
			expectCode: perrors.ErrContextCancelled,
			expectRet:  false,
		},
		// Retryable errors (Retryable=true in ErrorCodeRegistry)
		{
			name:       "timeout",
			err:        context.DeadlineExceeded,
			stage:      "stage1",
			expectCode: perrors.ErrTimeout,
			expectRet:  true,
		},
		{
			name:       "rate_limit",
			err:        errors.New("rate limit exceeded"),
			stage:      "extract",
			expectCode: perrors.ErrRateLimit,
			expectRet:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pe := perrors.ClassifyError(tc.err, tc.stage)

			var pipelineErr *perrors.PipelineError
			if !errors.As(pe, &pipelineErr) {
				t.Fatalf("expected error to be *PipelineError")
			}

			if pipelineErr.Code != tc.expectCode {
				t.Errorf("expected code %s, got %s", tc.expectCode, pipelineErr.Code)
			}

			retryable := perrors.IsErrorRetryable(pe)
			if retryable != tc.expectRet {
				t.Errorf("expected retryable=%v, got %v", tc.expectRet, retryable)
			}

			// After fix: WrapForTemporal converts non-retryable errors to ApplicationError
			wrapped := WrapForTemporal(pe)
			var appErr *temporal.ApplicationError
			isAppErr := errors.As(wrapped, &appErr)
			if !tc.expectRet && !isAppErr {
				t.Errorf("non-retryable error %s should be temporal.ApplicationError", tc.name)
			}

			// Retryable errors should NOT be wrapped
			if tc.expectRet && isAppErr {
				t.Errorf("retryable error %s should NOT be wrapped as ApplicationError", tc.name)
			}
		})
	}
}
