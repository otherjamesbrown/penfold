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

// TestUpdateSourceStatus_NoClassificationInMetadata verifies that classification fields
// (triage_category, triage_importance, content_subtype, source_system) are NOT written
// to the ingestion_metadata JSONB. These fields now live in proper DB columns.
// Only skip_deep (an operational flag) is still written to JSONB.
func TestUpdateSourceStatus_NoClassificationInMetadata(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	var updateStatusCalled bool
	var capturedTriageMetadata map[string]interface{}

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

	// Populate all classification fields — none should end up in ingestion_metadata JSONB
	skipDeepValue := true
	input := workflows.UpdateSourceStatusInput{
		TenantID:         "test-tenant",
		SourceID:         123,
		Status:           "completed",
		TriageCategory:   "technical_discussion",
		TriageImportance: "high",
		SkipDeep:         &skipDeepValue,
		ContentSubtype:   "architectural_decision",
		SourceSystem:     "human_email",
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatusWithFailure was called")

	// skip_deep should be present (operational flag)
	require.NotNil(t, capturedTriageMetadata, "Metadata should be passed for skip_deep")
	require.Equal(t, true, capturedTriageMetadata["skip_deep"])

	// Classification fields must NOT be in JSONB metadata
	require.NotContains(t, capturedTriageMetadata, "triage_category", "triage_category must not be in ingestion_metadata JSONB")
	require.NotContains(t, capturedTriageMetadata, "triage_importance", "triage_importance must not be in ingestion_metadata JSONB")
	require.NotContains(t, capturedTriageMetadata, "content_subtype", "content_subtype must not be in ingestion_metadata JSONB")
	require.NotContains(t, capturedTriageMetadata, "source_system", "source_system must not be in ingestion_metadata JSONB")
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

// TestUpdateSourceStatus_SkipDeepStillWritten verifies that skip_deep is still
// written to ingestion_metadata when SkipDeep is set. skip_deep is an operational
// flag (not a classification field) and remains in JSONB for workflow routing decisions.
func TestUpdateSourceStatus_SkipDeepStillWritten(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	var updateStatusCalled bool
	var capturedTriageMetadata map[string]interface{}

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

	skipDeepValue := true
	input := workflows.UpdateSourceStatusInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Status:   "completed",
		SkipDeep: &skipDeepValue,
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatusWithFailure was called")
	require.NotNil(t, capturedTriageMetadata, "Metadata should be passed when SkipDeep is set")
	require.Equal(t, true, capturedTriageMetadata["skip_deep"], "skip_deep should be true in ingestion_metadata")
	require.Len(t, capturedTriageMetadata, 1, "Only skip_deep should be in the metadata map")
}

// TestUpdateSourceStatus_SourceSystemNotInMetadata verifies that setting SourceSystem
// alone does NOT trigger any JSONB metadata write. source_system is now stored in
// a proper DB column, not in ingestion_metadata.
func TestUpdateSourceStatus_SourceSystemNotInMetadata(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	var updateStatusCalled bool
	var capturedTriageMetadata map[string]interface{}

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

	// Set SourceSystem but NOT SkipDeep — no JSONB metadata should be written
	input := workflows.UpdateSourceStatusInput{
		TenantID:     "test-tenant",
		SourceID:     123,
		Status:       "completed",
		SourceSystem: "human_email",
	}

	_, err := env.ExecuteActivity(activities.UpdateSourceStatus, input)
	require.NoError(t, err)

	require.True(t, updateStatusCalled, "UpdateSourceStatusWithFailure was called")

	// No metadata map should be passed since only SourceSystem is set (no SkipDeep)
	require.Nil(t, capturedTriageMetadata, "No JSONB metadata should be written when only SourceSystem is set")
}

// --- FetchSource tests (pf-cb680e) ---

// TestFetchSource_PopulatesAllFields verifies that FetchSource returns all available
// fields from the Source record: Subject, SenderEmail, SenderName, BodyHTML,
// SourceSystem, ContentID, and ParticipantEmails.
func TestFetchSource_PopulatesAllFields(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	mockRepo := &mockSourceRepositoryTracking{
		getSourceFunc: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:           sourceID,
				TenantID:     tenantID,
				ContentText:  "Hello, please review this.",
				ContentType:  "email",
				ContentHash:  "abc123",
				Status:       "pending",
				SourceSystem: "gmail_api",
				ContentID:    "em-AbCdEfGh",
				Metadata: map[string]string{
					"subject":      "Project Update Q1",
					"from_address": "alice@example.com",
					"from_name":    "Alice Smith",
					"body_html":    "<p>Hello</p>",
					"to":           `[{"name":"Bob Jones","address":"bob@example.com"}]`,
					"cc":           `[{"name":"Carol White","address":"carol@example.com"}]`,
				},
			}, nil
		},
	}

	logger := logging.NewNopLogger()
	acts := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(acts.FetchSource)

	input := workflows.FetchSourceInput{
		TenantID: "test-tenant",
		SourceID: 42,
	}

	val, err := env.ExecuteActivity(acts.FetchSource, input)
	require.NoError(t, err)

	var out workflows.FetchSourceOutput
	require.NoError(t, val.Get(&out))

	require.Equal(t, "Hello, please review this.", out.ContentText)
	require.Equal(t, "email", out.ContentType)
	require.Equal(t, "em-AbCdEfGh", out.ContentID)
	require.Equal(t, "gmail_api", out.SourceSystem)
	require.Equal(t, "Project Update Q1", out.Subject)
	require.Equal(t, "alice@example.com", out.SenderEmail)
	require.Equal(t, "Alice Smith", out.SenderName)
	require.Equal(t, "<p>Hello</p>", out.BodyHTML)

	require.Len(t, out.ParticipantEmails, 2)
	require.Equal(t, "bob@example.com", out.ParticipantEmails[0].Email)
	require.Equal(t, "Bob Jones", out.ParticipantEmails[0].DisplayName)
	require.Equal(t, "to", out.ParticipantEmails[0].HeaderRole)
	require.Equal(t, "carol@example.com", out.ParticipantEmails[1].Email)
	require.Equal(t, "Carol White", out.ParticipantEmails[1].DisplayName)
	require.Equal(t, "cc", out.ParticipantEmails[1].HeaderRole)
}

// TestFetchSource_EmptyMetadata verifies that FetchSource does not panic and returns
// only ContentText and ContentType when no metadata is present (e.g. non-email sources).
func TestFetchSource_EmptyMetadata(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	mockRepo := &mockSourceRepositoryTracking{
		getSourceFunc: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          sourceID,
				TenantID:    tenantID,
				ContentText: "Plain text note.",
				ContentType: "text/plain",
				Metadata:    map[string]string{},
			}, nil
		},
	}

	logger := logging.NewNopLogger()
	acts := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(acts.FetchSource)

	val, err := env.ExecuteActivity(acts.FetchSource, workflows.FetchSourceInput{
		TenantID: "test-tenant",
		SourceID: 7,
	})
	require.NoError(t, err)

	var out workflows.FetchSourceOutput
	require.NoError(t, val.Get(&out))

	require.Equal(t, "Plain text note.", out.ContentText)
	require.Empty(t, out.Subject)
	require.Empty(t, out.SenderEmail)
	require.Empty(t, out.ContentID)
	require.Empty(t, out.SourceSystem)
	require.Nil(t, out.ParticipantEmails)
}

// TestFetchSource_SubjectPopulated verifies the specific fix for pf-cb680e:
// FetchSource must return Subject from ingestion_metadata so the auto-reply
// short-circuit in triage fires correctly during reprocess.
func TestFetchSource_SubjectPopulated(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	mockRepo := &mockSourceRepositoryTracking{
		getSourceFunc: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          sourceID,
				TenantID:    tenantID,
				ContentText: "Thank you for your email.",
				ContentType: "email",
				Metadata: map[string]string{
					"subject": "Automatic reply: Out of office",
				},
			}, nil
		},
	}

	logger := logging.NewNopLogger()
	acts := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(acts.FetchSource)

	val, err := env.ExecuteActivity(acts.FetchSource, workflows.FetchSourceInput{
		TenantID: "test-tenant",
		SourceID: 99,
	})
	require.NoError(t, err)

	var out workflows.FetchSourceOutput
	require.NoError(t, val.Get(&out))

	// The auto-reply short-circuit checks HasPrefix(ToLower(TrimSpace(Subject)), "automatic reply:")
	require.Equal(t, "Automatic reply: Out of office", out.Subject,
		"Subject must be populated from ingestion_metadata so triage auto-reply detection fires")
}

// TestFetchSource_MalformedParticipantJSON verifies that FetchSource silently skips
// malformed JSON in "to"/"cc" metadata rather than failing the activity.
func TestFetchSource_MalformedParticipantJSON(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	mockRepo := &mockSourceRepositoryTracking{
		getSourceFunc: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          sourceID,
				TenantID:    tenantID,
				ContentText: "Hello.",
				ContentType: "email",
				Metadata: map[string]string{
					"subject": "Test",
					"to":      "not-valid-json",
					"cc":      `[{"name":"Valid","address":"valid@example.com"}]`,
				},
			}, nil
		},
	}

	logger := logging.NewNopLogger()
	acts := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(acts.FetchSource)

	val, err := env.ExecuteActivity(acts.FetchSource, workflows.FetchSourceInput{
		TenantID: "test-tenant",
		SourceID: 55,
	})
	require.NoError(t, err)

	var out workflows.FetchSourceOutput
	require.NoError(t, val.Get(&out))

	// Malformed "to" is skipped; valid "cc" is included.
	require.Len(t, out.ParticipantEmails, 1)
	require.Equal(t, "valid@example.com", out.ParticipantEmails[0].Email)
}

// TestFetchSource_ParticipantWithoutAddress verifies that participant entries
// missing an address are omitted from ParticipantEmails.
func TestFetchSource_ParticipantWithoutAddress(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	mockRepo := &mockSourceRepositoryTracking{
		getSourceFunc: func(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
			return &Source{
				ID:          sourceID,
				TenantID:    tenantID,
				ContentText: "Hello.",
				ContentType: "email",
				Metadata: map[string]string{
					"to": `[{"name":"No Address","address":""},{"name":"Has Address","address":"has@example.com"}]`,
				},
			}, nil
		},
	}

	logger := logging.NewNopLogger()
	acts := NewSourceActivities(logger, mockRepo)
	env.RegisterActivity(acts.FetchSource)

	val, err := env.ExecuteActivity(acts.FetchSource, workflows.FetchSourceInput{
		TenantID: "test-tenant",
		SourceID: 66,
	})
	require.NoError(t, err)

	var out workflows.FetchSourceOutput
	require.NoError(t, val.Get(&out))

	require.Len(t, out.ParticipantEmails, 1, "Entry without address must be omitted")
	require.Equal(t, "has@example.com", out.ParticipantEmails[0].Email)
}
