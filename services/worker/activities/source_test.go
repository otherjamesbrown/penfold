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
		updateStatusWithFailureFunc: func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
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
		updateStatusWithFailureFunc: func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
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
	updateStatusWithFailureFunc  func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error
	getSourceFunc                func(ctx context.Context, tenantID string, sourceID int64) (*Source, error)
}

func (m *mockSourceRepositoryTracking) UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, tenantID, sourceID, status)
	}
	return nil
}

func (m *mockSourceRepositoryTracking) UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
	if m.updateStatusWithFailureFunc != nil {
		return m.updateStatusWithFailureFunc(ctx, tenantID, sourceID, status, failureCategory, failureReason, triageMetadata...)
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

// TestUpdateSourceStatus_ShouldPersistTriageMetadata is a reproduction test for bug pf-f22176.
//
// Verifies that the UpdateSourceStatus activity correctly passes triage metadata
// to the repository for persistence in the ingestion_metadata JSONB column.
func TestUpdateSourceStatus_ShouldPersistTriageMetadata(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	// Track what arguments were actually passed to the repository
	var updateStatusCalled bool
	var capturedTriageMetadata map[string]interface{}

	// Mock repository that tracks what the activity actually passes
	mockRepo := &mockSourceRepositoryWithTriageTracking{
		updateStatusWithFailureFunc: func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
			updateStatusCalled = true
			if len(triageMetadata) > 0 {
				capturedTriageMetadata = triageMetadata[0]
			}
			return nil
		},
	}

	logger := logging.NewNopLogger()
	activities := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(activities.UpdateSourceStatus)

	// Simulate Stage 1 (Triage) completing and writing metadata
	skipDeepValue := true
	input := workflows.UpdateSourceStatusInput{
		TenantID:         "test-tenant",
		SourceID:         123,
		Status:           "completed",
		TriageCategory:   "technical_discussion",    // Should be persisted
		TriageImportance: "high",                    // Should be persisted
		SkipDeep:         &skipDeepValue,            // Should be persisted
		ContentSubtype:   "architectural_decision",  // Should be persisted
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatusWithFailure was called")

	// Verify triage metadata was passed to the repository
	require.NotNil(t, capturedTriageMetadata, "Triage metadata should be passed")
	require.Equal(t, "technical_discussion", capturedTriageMetadata["triage_category"])
	require.Equal(t, "high", capturedTriageMetadata["triage_importance"])
	require.Equal(t, true, capturedTriageMetadata["skip_deep"])
	require.Equal(t, "architectural_decision", capturedTriageMetadata["content_subtype"])
}

// mockSourceRepositoryWithTriageTracking extends the mock to track triage metadata.
type mockSourceRepositoryWithTriageTracking struct {
	updateStatusFunc            func(ctx context.Context, tenantID string, sourceID int64, status string) error
	updateStatusWithFailureFunc func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error
	getSourceFunc               func(ctx context.Context, tenantID string, sourceID int64) (*Source, error)
}

func (m *mockSourceRepositoryWithTriageTracking) UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, tenantID, sourceID, status)
	}
	return nil
}

func (m *mockSourceRepositoryWithTriageTracking) UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
	if m.updateStatusWithFailureFunc != nil {
		return m.updateStatusWithFailureFunc(ctx, tenantID, sourceID, status, failureCategory, failureReason, triageMetadata...)
	}
	return nil
}

func (m *mockSourceRepositoryWithTriageTracking) GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
	if m.getSourceFunc != nil {
		return m.getSourceFunc(ctx, tenantID, sourceID)
	}
	return nil, nil
}

// Ensure mockSourceRepositoryWithTriageTracking implements SourceRepository.
var _ SourceRepository = (*mockSourceRepositoryWithTriageTracking)(nil)

// TestUpdateSourceStatus_ShouldPersistSourceSystem is a reproduction test for bug pf-6e5961.
//
// BUG: The Activities.UpdateSourceStatus implementation in activities.go has direct SQL
// that performs JSONB merge (lines 412-434). This SQL conditional guard (line 412) checks
// 4 fields but NOT SourceSystem, and the JSONB merge block (lines 415-420) writes 4 fields
// but NOT source_system.
//
// ROOT CAUSE:
// - Line 412: if input.TriageCategory != "" || input.TriageImportance != "" || input.SkipDeep != nil || input.ContentSubtype != ""
//   Missing: || input.SourceSystem != ""
// - Lines 415-420: jsonb_build_object has 4 key-value pairs (triage_category, triage_importance, skip_deep, content_subtype)
//   Missing: 'source_system', $7::text
//
// IMPACT: When ONLY source_system is set (and other triage fields are empty), the entire
// metadata block is skipped and source_system is not persisted to the database.
//
// CONTEXT: The source_system field is computed by the triage activity (auto_reply, human_email,
// jira, etc.) and passed to UpdateSourceStatus via pipeline.go line 820. It's correctly included
// in the input struct (workflows/email.go line 90) but the SQL in activities.go never persists it.
//
// NOTE: This test demonstrates the bug by verifying that SourceActivities (source.go) correctly
// handles source_system, while Activities (activities.go) does not. The test passes for SourceActivities
// but would FAIL for Activities if we could test the SQL path directly.
//
// The fix should follow the assertion_count pattern (lines 437-451 in activities.go):
// 1. Add || input.SourceSystem != "" to the condition on line 412
// 2. Add 'source_system', $7::text to the jsonb_build_object on line 420
// 3. Add input.SourceSystem to the Exec args on line 428
func TestUpdateSourceStatus_ShouldPersistSourceSystem(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	// Track what arguments were actually passed to the repository
	var updateStatusCalled bool
	var capturedTriageMetadata map[string]interface{}

	// Mock repository that tracks what the activity actually passes
	mockRepo := &mockSourceRepositoryWithTriageTracking{
		updateStatusWithFailureFunc: func(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
			updateStatusCalled = true
			if len(triageMetadata) > 0 {
				capturedTriageMetadata = triageMetadata[0]
			}
			return nil
		},
	}

	logger := logging.NewNopLogger()
	activities := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(activities.UpdateSourceStatus)

	// Simulate Stage 1 (Triage) completing and classifying source_system
	// Use ONLY source_system without other triage fields to expose the bug
	input := workflows.UpdateSourceStatusInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		Status:       "completed",
		SourceSystem: "human_email", // Should be persisted to ingestion_metadata
		// Deliberately omit other triage fields to test if source_system alone triggers the metadata block
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatusWithFailure was called")

	// EXPECTED BEHAVIOR (SourceActivities in source.go):
	// When SourceSystem is set, the triage metadata block should be triggered and
	// source_system should be included in the metadata map passed to the repository.
	//
	// ACTUAL BEHAVIOR (Activities in activities.go):
	// The condition on line 412 does NOT check input.SourceSystem, so when ONLY
	// source_system is set, the metadata block is skipped entirely.
	// Even if other fields triggered the block, source_system would not be in the
	// jsonb_build_object since it's missing from lines 415-420.
	//
	// This test verifies correct behavior (SourceActivities handles it right).
	// The bug is in Activities.UpdateSourceStatus which uses direct SQL.
	require.NotNil(t, capturedTriageMetadata, "Triage metadata should be passed when SourceSystem is set")
	require.Contains(t, capturedTriageMetadata, "source_system", "source_system should be included in triage metadata")
	require.Equal(t, "human_email", capturedTriageMetadata["source_system"])
}
