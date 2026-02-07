package errors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrorCode represents a classified pipeline error.
type ErrorCode string

const (
	ErrTimeout          ErrorCode = "timeout"
	ErrRateLimit        ErrorCode = "rate_limit"
	ErrModelUnavailable ErrorCode = "model_unavailable"
	ErrContextCancelled ErrorCode = "context_cancelled"
	ErrParseError       ErrorCode = "parse_error"
	ErrEmptyContent     ErrorCode = "empty_content"
	ErrProcessingError  ErrorCode = "processing_error"
)

// PipelineError is a structured error for pipeline failures.
type PipelineError struct {
	Code     ErrorCode
	Stage    string
	Message  string
	Duration time.Duration
	Timeout  time.Duration
	Cause    error
}

func (e *PipelineError) Error() string {
	if e.Timeout > 0 && e.Duration > 0 {
		return fmt.Sprintf("%s: %s timed out after %s (limit: %s)", e.Code, e.Stage, e.Duration.Truncate(time.Second), e.Timeout.Truncate(time.Second))
	}
	if e.Stage != "" {
		return fmt.Sprintf("%s: %s: %s", e.Code, e.Stage, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *PipelineError) Unwrap() error {
	return e.Cause
}

// ClassifyError inspects an error and returns a *PipelineError with the appropriate code.
// If the error doesn't match any known pattern, it returns a PipelineError with ErrProcessingError.
func ClassifyError(err error, stage string) *PipelineError {
	if err == nil {
		return nil
	}

	pe := &PipelineError{
		Stage: stage,
		Cause: err,
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		pe.Code = ErrTimeout
		pe.Message = "operation timed out"
		return pe
	}

	// Check for context cancelled
	if errors.Is(err, context.Canceled) {
		pe.Code = ErrContextCancelled
		pe.Message = "operation cancelled"
		return pe
	}

	// Check error message patterns
	msg := err.Error()
	lower := strings.ToLower(msg)

	// Empty content patterns
	if strings.Contains(lower, "empty content") || strings.Contains(lower, "content is empty") || strings.Contains(lower, "no content") {
		pe.Code = ErrEmptyContent
		pe.Message = msg
		return pe
	}

	// Rate limit patterns
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "quota exceeded") || strings.Contains(lower, "resource_exhausted") {
		pe.Code = ErrRateLimit
		pe.Message = msg
		return pe
	}

	// Model unavailable patterns
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "unavailable") || strings.Contains(lower, "503") || strings.Contains(lower, "service unavailable") || strings.Contains(lower, "no such host") {
		pe.Code = ErrModelUnavailable
		pe.Message = msg
		return pe
	}

	// Default to processing error
	pe.Code = ErrProcessingError
	pe.Message = msg
	return pe
}

// IsTimeout returns true if the error is a timeout error.
func IsTimeout(err error) bool {
	var pe *PipelineError
	if errors.As(err, &pe) {
		return pe.Code == ErrTimeout
	}
	return false
}

// IsRetryable returns true if the error is likely transient and worth retrying.
func IsRetryable(err error) bool {
	var pe *PipelineError
	if errors.As(err, &pe) {
		return pe.Code == ErrTimeout || pe.Code == ErrRateLimit || pe.Code == ErrModelUnavailable
	}
	return false
}
