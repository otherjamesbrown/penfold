// Package activities provides activity tests.
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TestUpdateSourceStatus_ShouldPassFailureFields is a reproduction test for bug pf-236440.
//
// BUG: The activity receives FailureCategory and FailureReason in the input struct,
// but only passes status to the repository via UpdateSourceStatus(ctx, tenantID, sourceID, status).
// This leaves failure_category and failure_reason stale in the database when transitioning
// from FAILED -> COMPLETED.
//
// THIS TEST FAILS to demonstrate the bug: the activity ignores the failure fields.
func TestUpdateSourceStatus_ShouldPassFailureFields(t *testing.T) {
	// Set up Temporal test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	// Track what arguments were passed to UpdateSourceStatusWithFailure
	var updateStatusCalled bool
	var capturedArgs []interface{}

	mockRepo := &mockSourceRepositoryTracking{
		updateStatusWithFailureFunc: func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string) error {
			updateStatusCalled = true
			// Capture all args to verify what was passed
			capturedArgs = []interface{}{tenantID, sourceID, status, failureCategory, failureReason}
			return nil
		},
	}

	logger := logging.NewNopLogger()
	activities := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(activities.UpdateSourceStatus)

	// Create input with status=COMPLETED and empty failure fields
	// This simulates clearing a previous failure (status was FAILED, now COMPLETED)
	input := workflows.UpdateSourceStatusInput{
		TenantID:        "test-tenant",
		SourceID:        123,
		Status:          "completed",
		FailureCategory: "", // Should be passed to clear previous failure_category in DB
		FailureReason:   "", // Should be passed to clear previous failure_reason in DB
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatus was called")

	// BUG ASSERTION: The activity currently only passes 3 args to the repository.
	// This test will FAIL to demonstrate that failure fields are not being handled.
	//
	// The activity receives FailureCategory and FailureReason in input but ignores them.
	// When status=COMPLETED with empty failure fields, those fields should be cleared in DB,
	// but the current implementation doesn't pass them to the repository at all.
	//
	// EXPECTED: Repository method should receive failure fields (5+ args total)
	// ACTUAL: Repository method receives only status (3 args: tenantID, sourceID, status)
	require.Len(t, capturedArgs, 5, "UpdateSourceStatus should pass 5 args: tenantID, sourceID, status, failureCategory, failureReason")
}

// TestUpdateSourceStatus_WithFailureFields tests the scenario where status=FAILED
// and failure fields are provided and properly passed to the repository.
func TestUpdateSourceStatus_WithFailureFields(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	var updateStatusCalled bool
	var capturedStatus string
	var capturedCategory string
	var capturedReason string

	mockRepo := &mockSourceRepositoryTracking{
		updateStatusWithFailureFunc: func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string) error {
			updateStatusCalled = true
			capturedStatus = status
			capturedCategory = failureCategory
			capturedReason = failureReason
			return nil
		},
	}

	logger := logging.NewNopLogger()
	activities := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(activities.UpdateSourceStatus)

	// Create input with status=FAILED and populated failure fields
	input := workflows.UpdateSourceStatusInput{
		TenantID:        "test-tenant",
		SourceID:        123,
		Status:          "failed",
		FailureCategory: "PROCESSING_ERROR",
		FailureReason:   "Timeout during embedding generation",
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatusWithFailure was called")
	require.Equal(t, "failed", capturedStatus)
	require.Equal(t, "PROCESSING_ERROR", capturedCategory)
	require.Equal(t, "Timeout during embedding generation", capturedReason)
}

// mockSourceRepositoryTracking is a mock that tracks calls to UpdateSourceStatus.
type mockSourceRepositoryTracking struct {
	updateStatusFunc             func(ctx context.Context, tenantID string, sourceID int64, status string) error
	updateStatusWithFailureFunc  func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string) error
	getSourceFunc                func(ctx context.Context, tenantID string, sourceID int64) (*Source, error)
}

func (m *mockSourceRepositoryTracking) UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, tenantID, sourceID, status)
	}
	return nil
}

func (m *mockSourceRepositoryTracking) UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string) error {
	if m.updateStatusWithFailureFunc != nil {
		return m.updateStatusWithFailureFunc(ctx, tenantID, sourceID, status, failureCategory, failureReason)
	}
	return nil
}

func (m *mockSourceRepositoryTracking) GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
	if m.getSourceFunc != nil {
		return m.getSourceFunc(ctx, tenantID, sourceID)
	}
	return nil, nil
}

// Ensure mockSourceRepositoryTracking implements SourceRepository.
var _ SourceRepository = (*mockSourceRepositoryTracking)(nil)
