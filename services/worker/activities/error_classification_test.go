package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// Note: Mock implementations are defined in other test files (constructor_test.go, extraction_test.go, etc.)

// TestClassifyErrorStageNames verifies that ClassifyError is called with correct stage names.
func TestClassifyErrorStageNames(t *testing.T) {
	tests := []struct {
		name          string
		testError     error
		expectedStage string
	}{
		{
			name:          "extract_ner stage",
			testError:     errors.New("extraction failed"),
			expectedStage: "extract_ner",
		},
		{
			name:          "extract_assertions stage",
			testError:     errors.New("assertion extraction failed"),
			expectedStage: "extract_assertions",
		},
		{
			name:          "triage stage",
			testError:     errors.New("triage failed"),
			expectedStage: "triage",
		},
		{
			name:          "analyze stage",
			testError:     errors.New("analysis failed"),
			expectedStage: "analyze",
		},
		{
			name:          "embed stage",
			testError:     errors.New("embedding failed"),
			expectedStage: "embed",
		},
		{
			name:          "persist stage",
			testError:     errors.New("persist failed"),
			expectedStage: "persist",
		},
		{
			name:          "resolve stage",
			testError:     errors.New("context building failed"),
			expectedStage: "resolve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Classify the error
			pe := perrors.ClassifyError(tt.testError, tt.expectedStage)

			// Verify the stage is set correctly
			assert.Equal(t, tt.expectedStage, pe.Stage, "stage should match")
			assert.NotEmpty(t, pe.Code, "error code should be set")
			assert.NotEmpty(t, pe.Message, "error message should be set")
		})
	}
}

// TestClassifyErrorCodes verifies that different error types get classified correctly.
func TestClassifyErrorCodes(t *testing.T) {
	tests := []struct {
		name         string
		testError    error
		expectedCode perrors.ErrorCode
	}{
		{
			name:         "timeout gets classified as ErrTimeout",
			testError:    context.DeadlineExceeded,
			expectedCode: perrors.ErrTimeout,
		},
		{
			name:         "context cancelled gets classified as ErrContextCancelled",
			testError:    context.Canceled,
			expectedCode: perrors.ErrContextCancelled,
		},
		{
			name:         "rate limit gets classified as ErrRateLimit",
			testError:    errors.New("rate limit exceeded"),
			expectedCode: perrors.ErrRateLimit,
		},
		{
			name:         "429 status gets classified as ErrRateLimit",
			testError:    errors.New("429 too many requests"),
			expectedCode: perrors.ErrRateLimit,
		},
		{
			name:         "quota exceeded gets classified as ErrRateLimit",
			testError:    errors.New("quota exceeded"),
			expectedCode: perrors.ErrRateLimit,
		},
		{
			name:         "embedding rate limit gets classified as ErrRateLimitEmbedding",
			testError:    errors.New("embedding rate limit exceeded"),
			expectedCode: perrors.ErrRateLimitEmbedding,
		},
		{
			name:         "connection refused gets classified as ErrModelUnavailable",
			testError:    errors.New("connection refused"),
			expectedCode: perrors.ErrModelUnavailable,
		},
		{
			name:         "503 gets classified as ErrModelUnavailable",
			testError:    errors.New("503 service unavailable"),
			expectedCode: perrors.ErrModelUnavailable,
		},
		{
			name:         "no such host gets classified as ErrModelUnavailable",
			testError:    errors.New("no such host"),
			expectedCode: perrors.ErrModelUnavailable,
		},
		{
			name:         "empty content gets classified as ErrEmptyContent",
			testError:    errors.New("content is empty"),
			expectedCode: perrors.ErrEmptyContent,
		},
		{
			name:         "duplicate gets classified as ErrDuplicateContent",
			testError:    errors.New("duplicate content detected"),
			expectedCode: perrors.ErrDuplicateContent,
		},
		{
			name:         "generic error gets classified as ErrProcessingError",
			testError:    errors.New("something went wrong"),
			expectedCode: perrors.ErrProcessingError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Classify the error
			pe := perrors.ClassifyError(tt.testError, "test_stage")

			// Verify the error code
			assert.Equal(t, tt.expectedCode, pe.Code, "error code should match")
			assert.Equal(t, "test_stage", pe.Stage, "stage should be set")
		})
	}
}

// TestClassifiedErrorWrapping verifies that classified errors can be unwrapped.
func TestClassifiedErrorWrapping(t *testing.T) {
	originalErr := errors.New("original error")
	pe := perrors.ClassifyError(originalErr, "test_stage")

	// Verify the error can be unwrapped
	assert.True(t, errors.Is(pe, originalErr), "classified error should wrap original error")

	// Verify it's a PipelineError
	var pipelineErr *perrors.PipelineError
	assert.True(t, errors.As(pe, &pipelineErr), "should be a PipelineError type")
}

// TestErrorRetryability verifies that classified errors have correct retryability based on error code.
func TestErrorRetryability(t *testing.T) {
	tests := []struct {
		name        string
		errorCode   perrors.ErrorCode
		shouldRetry bool
	}{
		{
			name:        "timeout is retryable",
			errorCode:   perrors.ErrTimeout,
			shouldRetry: true,
		},
		{
			name:        "rate limit is retryable",
			errorCode:   perrors.ErrRateLimit,
			shouldRetry: true,
		},
		{
			name:        "model unavailable is retryable",
			errorCode:   perrors.ErrModelUnavailable,
			shouldRetry: true,
		},
		{
			name:        "context cancelled is not retryable",
			errorCode:   perrors.ErrContextCancelled,
			shouldRetry: false,
		},
		{
			name:        "empty content is not retryable",
			errorCode:   perrors.ErrEmptyContent,
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a pipeline error with the given code
			pe := &perrors.PipelineError{
				Code:    tt.errorCode,
				Stage:   "test",
				Message: "test error",
			}

			// Check retryability
			isRetryable := perrors.IsErrorRetryable(pe)
			assert.Equal(t, tt.shouldRetry, isRetryable, "retryability should match expected")
		})
	}
}
