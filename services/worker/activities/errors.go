// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"errors"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"go.temporal.io/sdk/temporal"
)

// Error types for activity failures.
const (
	// ErrorTypeConfiguration indicates a configuration error that should not be retried.
	ErrorTypeConfiguration = "ConfigurationError"

	// ErrorTypeValidation indicates a validation error that should not be retried.
	ErrorTypeValidation = "ValidationError"

	// ErrorTypeNotFound indicates a resource was not found.
	ErrorTypeNotFound = "NotFoundError"

	// ErrorTypePermissionDenied indicates insufficient permissions.
	ErrorTypePermissionDenied = "PermissionDeniedError"

	// ErrorTypeInvalidArgument indicates an invalid argument was provided.
	ErrorTypeInvalidArgument = "InvalidArgumentError"

	// ErrorTypeTimeout indicates an operation timed out.
	ErrorTypeTimeout = "TimeoutError"

	// ErrorTypeTemporary indicates a temporary error that can be retried.
	ErrorTypeTemporary = "TemporaryError"
)

// NewConfigurationError creates a non-retryable configuration error.
// Configuration errors indicate that the activity cannot proceed due to
// missing dependencies or invalid configuration.
func NewConfigurationError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypeConfiguration)
}

// NewValidationError creates a non-retryable validation error.
// Validation errors indicate that the input data is invalid and
// retrying will not help.
func NewValidationError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypeValidation)
}

// NewNotFoundError creates a non-retryable not found error.
// Not found errors indicate that the requested resource does not exist.
func NewNotFoundError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypeNotFound)
}

// NewPermissionDeniedError creates a non-retryable permission denied error.
// Permission denied errors indicate insufficient access rights.
func NewPermissionDeniedError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypePermissionDenied)
}

// NewInvalidArgumentError creates a non-retryable invalid argument error.
// Invalid argument errors indicate that one or more arguments are invalid.
func NewInvalidArgumentError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypeInvalidArgument)
}

// NewTimeoutError creates a retryable timeout error.
// Timeout errors indicate that an operation did not complete in time.
func NewTimeoutError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypeTimeout)
}

// NewTemporaryError creates a retryable temporary error.
// Temporary errors indicate transient failures that may succeed on retry.
func NewTemporaryError(message string) error {
	return temporal.NewApplicationError(message, ErrorTypeTemporary)
}

// NewConfigurationErrorWithCause creates a non-retryable configuration error with a cause.
func NewConfigurationErrorWithCause(message string, cause error) error {
	return temporal.NewApplicationErrorWithCause(message, ErrorTypeConfiguration, cause)
}

// NewValidationErrorWithCause creates a non-retryable validation error with a cause.
func NewValidationErrorWithCause(message string, cause error) error {
	return temporal.NewApplicationErrorWithCause(message, ErrorTypeValidation, cause)
}

// NewNotFoundErrorWithCause creates a non-retryable not found error with a cause.
func NewNotFoundErrorWithCause(message string, cause error) error {
	return temporal.NewApplicationErrorWithCause(message, ErrorTypeNotFound, cause)
}

// NewTemporaryErrorWithCause creates a retryable temporary error with a cause.
func NewTemporaryErrorWithCause(message string, cause error) error {
	return temporal.NewApplicationErrorWithCause(message, ErrorTypeTemporary, cause)
}

// IsConfigurationError checks if an error is a configuration error.
func IsConfigurationError(err error) bool {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Type() == ErrorTypeConfiguration
	}
	return false
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Type() == ErrorTypeValidation
	}
	return false
}

// IsNotFoundError checks if an error is a not found error.
func IsNotFoundError(err error) bool {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Type() == ErrorTypeNotFound
	}
	return false
}

// IsRetryableError checks if an error should be retried.
// Returns true for temporary and timeout errors, false for all others.
func IsRetryableError(err error) bool {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		switch appErr.Type() {
		case ErrorTypeTemporary, ErrorTypeTimeout:
			return true
		case ErrorTypeConfiguration, ErrorTypeValidation, ErrorTypeNotFound, ErrorTypePermissionDenied, ErrorTypeInvalidArgument:
			return false
		}
	}
	// Default to retryable for unknown errors
	return true
}

// NonRetryableErrorTypes returns a list of error types that should not be retried.
// This is useful for configuring Temporal retry policies.
func NonRetryableErrorTypes() []string {
	return []string{
		ErrorTypeConfiguration,
		ErrorTypeValidation,
		ErrorTypeNotFound,
		ErrorTypePermissionDenied,
		ErrorTypeInvalidArgument,
	}
}

// WrapForTemporal converts non-retryable PipelineError to temporal.ApplicationError
// so Temporal respects the non-retryable classification.
//
// PipelineError uses a different error system than Temporal ApplicationError.
// Without this bridge, Temporal cannot check NonRetryableErrorTypes and will
// retry all PipelineErrors regardless of their retryable flag.
//
// This function checks if the error is a PipelineError and if it's non-retryable
// (based on its error code in the ErrorCodeRegistry), then wraps it as a
// temporal.ApplicationError with type "PipelineError" so Temporal can match it
// against the NonRetryableErrorTypes list.
func WrapForTemporal(err error) error {
	if err == nil {
		return nil
	}

	var pe *perrors.PipelineError
	if errors.As(err, &pe) {
		// Check if this error code is non-retryable
		if !perrors.IsRetryable(pe.Code) {
			return temporal.NewApplicationErrorWithCause(
				pe.Error(), "PipelineError", pe,
			)
		}
	}
	return err
}
