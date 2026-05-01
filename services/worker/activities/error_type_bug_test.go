// Package activities provides activity tests.
// Bug pf-55f498: FetchSource and UpdateSourceStatus return plain fmt.Errorf instead of temporal.ApplicationError
package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/testutil"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TestFetchSource_NotFound_ReturnsNotFoundError verifies that FetchSource returns
// a temporal.ApplicationError with type "NotFoundError" when the source doesn't exist.
//
// BUG: Currently FAILS because FetchSource wraps pgx.ErrNoRows with plain fmt.Errorf
// instead of using NewNotFoundError.
func TestFetchSource_NotFound_ReturnsNotFoundError(t *testing.T) {
	pool := testutil.OpenDB(t)
	defer pool.Close()

	// Create activity with real DB
	logger := logging.NewNopLogger()
	acts := NewActivitiesWithDB(logger, pool, "http://ai-service:8080")

	// Execute FetchSource with a source ID that's extremely unlikely to exist
	ctx := context.Background()
	input := workflows.FetchSourceInput{
		TenantID: DefaultTenantID,
		SourceID: 9999999999, // Very high ID unlikely to exist
	}

	result, err := acts.FetchSource(ctx, input)

	// Verify error behavior
	require.Error(t, err, "FetchSource should return error for non-existent source")
	require.Nil(t, result, "Result should be nil on error")

	t.Logf("Error returned: %v (type: %T)", err, err)

	// BUG: This assertion SHOULD pass but currently FAILS
	// The error should be a temporal.ApplicationError with type "NotFoundError"
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Errorf("ERROR TYPE BUG CONFIRMED: error should be temporal.ApplicationError, got: %T", err)
		t.Logf("This is the bug - FetchSource returns plain fmt.Errorf instead of NewNotFoundError")
		return
	}

	if appErr.Type() != ErrorTypeNotFound {
		t.Errorf("ERROR TYPE BUG CONFIRMED: error type should be %q, got: %q", ErrorTypeNotFound, appErr.Type())
		t.Logf("This is the bug - FetchSource should use NewNotFoundError")
	}
}

// TestUpdateSourceStatus_NotFound_ReturnsNotFoundError verifies that UpdateSourceStatus
// returns a temporal.ApplicationError with type "NotFoundError" when the source doesn't exist.
//
// BUG: Currently FAILS because UpdateSourceStatus returns plain fmt.Errorf("source not found: %d")
// instead of using NewNotFoundError.
func TestUpdateSourceStatus_NotFound_ReturnsNotFoundError(t *testing.T) {
	pool := testutil.OpenDB(t)
	defer pool.Close()

	// Create activity with real DB
	logger := logging.NewNopLogger()
	acts := NewActivitiesWithDB(logger, pool, "http://ai-service:8080")

	// Execute UpdateSourceStatus with a source ID that's extremely unlikely to exist
	ctx := context.Background()
	input := workflows.UpdateSourceStatusInput{
		TenantID: DefaultTenantID,
		SourceID: 9999999999, // Very high ID unlikely to exist
		Status:   "completed",
	}

	err := acts.UpdateSourceStatus(ctx, input)

	// Verify error behavior
	require.Error(t, err, "UpdateSourceStatus should return error for non-existent source")

	t.Logf("Error returned: %v (type: %T)", err, err)

	// BUG: This assertion SHOULD pass but currently FAILS
	// The error should be a temporal.ApplicationError with type "NotFoundError"
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Errorf("ERROR TYPE BUG CONFIRMED: error should be temporal.ApplicationError, got: %T", err)
		t.Logf("This is the bug - UpdateSourceStatus returns plain fmt.Errorf instead of NewNotFoundError")
		return
	}

	if appErr.Type() != ErrorTypeNotFound {
		t.Errorf("ERROR TYPE BUG CONFIRMED: error type should be %q, got: %q", ErrorTypeNotFound, appErr.Type())
		t.Logf("This is the bug - UpdateSourceStatus should use NewNotFoundError")
	}
}

// TestFetchSource_NotFound_IsNonRetryable verifies that NotFoundError is configured
// as non-retryable in Temporal retry policies.
//
// This test documents the expected behavior: NotFoundError should be in the
// NonRetryableErrorTypes list to prevent infinite retries.
func TestFetchSource_NotFound_IsNonRetryable(t *testing.T) {
	// Verify that NotFoundError is in the non-retryable error types list
	nonRetryable := NonRetryableErrorTypes()
	assert.Contains(t, nonRetryable, ErrorTypeNotFound,
		"NotFoundError should be in NonRetryableErrorTypes()")

	// Verify IsRetryableError helper recognizes NotFoundError as non-retryable
	notFoundErr := NewNotFoundError("source not found: 999")
	assert.False(t, IsRetryableError(notFoundErr),
		"NotFoundError should be non-retryable")
}

// TestUpdateSourceStatus_NotFound_IsNonRetryable verifies that NotFoundError
// from UpdateSourceStatus is non-retryable.
func TestUpdateSourceStatus_NotFound_IsNonRetryable(t *testing.T) {
	// Create a NotFoundError as UpdateSourceStatus should return
	notFoundErr := NewNotFoundError("source not found: 999")

	// Verify it's properly typed
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(notFoundErr, &appErr))
	assert.Equal(t, ErrorTypeNotFound, appErr.Type())

	// Verify it's non-retryable
	assert.False(t, IsRetryableError(notFoundErr),
		"NotFoundError should be non-retryable")
}
