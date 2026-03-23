package workflows

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

// AutoDrainTestSuite tests the auto-drain behavior after pipeline completion.
// The auto-drain feature ensures that when a pipeline completes (success or failure),
// the worker automatically kicks the next pending item to maintain the concurrency window.
type AutoDrainTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *AutoDrainTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &PipelineMockActivities{}

	// Register all pipeline activities
	s.env.RegisterActivityWithOptions(s.activities.ParseEmail, activity.RegisterOptions{Name: "ParseEmail"})
	s.env.RegisterActivityWithOptions(s.activities.ParseTranscript, activity.RegisterOptions{Name: "ParseTranscript"})
	s.env.RegisterActivityWithOptions(s.activities.Triage, activity.RegisterOptions{Name: "Triage"})
	s.env.RegisterActivityWithOptions(s.activities.ExtractEntitiesActivity, activity.RegisterOptions{Name: "ExtractEntitiesActivity"})
	s.env.RegisterActivityWithOptions(s.activities.ExtractAssertions, activity.RegisterOptions{Name: "ExtractAssertions"})
	s.env.RegisterActivityWithOptions(s.activities.BuildContextPackage, activity.RegisterOptions{Name: "BuildContextPackage"})
	s.env.RegisterActivityWithOptions(s.activities.DeepAnalyze, activity.RegisterOptions{Name: "DeepAnalyze"})
	s.env.RegisterActivityWithOptions(s.activities.PersistFindings, activity.RegisterOptions{Name: "PersistFindings"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	s.env.RegisterActivityWithOptions(s.activities.DeleteEmbedding, activity.RegisterOptions{Name: "DeleteEmbedding"})
	s.env.RegisterActivityWithOptions(s.activities.RecordOverrides, activity.RegisterOptions{Name: "RecordOverrides"})
	s.env.RegisterActivityWithOptions(s.activities.FetchSource, activity.RegisterOptions{Name: "FetchContent"})
	s.env.RegisterActivityWithOptions(s.activities.GroupEmailThread, activity.RegisterOptions{Name: "GroupEmailThread"})
	s.env.RegisterActivityWithOptions(s.activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: "CreateEnrichmentRecord"})

	// Register the kick activity that should be called after completion
	s.env.RegisterActivityWithOptions(s.activities.KickNextPending, activity.RegisterOptions{Name: "KickNextPending"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{Name: "GenerateContentSummary"})
	s.env.RegisterActivityWithOptions(s.activities.FetchPipelineDefinition, activity.RegisterOptions{Name: "FetchPipelineDefinition"})

	// Default mock expectations for enrichment/threading activities (blocking since pf-67502c fix).
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Maybe().Return(standardTestPipelineDef(), nil)
}

func (s *AutoDrainTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestPipelineAutoKick_OnCompletion verifies that after successful pipeline completion,
// a kick activity is called to trigger processing of the next pending item.
func (s *AutoDrainTestSuite) TestPipelineAutoKick_OnCompletion() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    100,
		ContentID:   "em-test01",
		JobID:       "job-100",
		ContentType: "email",
		ContentHash: "hash100",
		BodyText:    "Project update email",
		Subject:     "Update",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  "Project update email",
		NewContent: "Project update email",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage (skip deep to make test faster)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b",
		Reason:     "Personal email",
	}, nil)

	// Stage 2: Extract (skipped due to SkipDeep)
	// Stage 3: Context (skipped due to SkipDeep)
	// Stage 4: Analyze (skipped due to SkipDeep)
	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5001), nil)

	// Final status update to "completed"
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// CRITICAL: The auto-drain feature should call KickNextPending after completion
	// This is the key assertion for this test
	kickCalled := false
	s.activities.On("KickNextPending", mock.Anything, mock.MatchedBy(func(in KickNextPendingInput) bool {
		kickCalled = true
		return in.TenantID == "tenant-1"
	})).Return(&KickNextPendingOutput{
		QueuedCount: 1,
		Message:     "Kicked 1 pending item",
	}, nil)

	// Execute
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// VERIFY: The kick activity was called after completion
	s.True(kickCalled, "Expected KickNextPending to be called after pipeline completion, but it was not")
	s.activities.AssertCalled(s.T(), "KickNextPending", mock.Anything, mock.Anything)
}

// TestPipelineAutoKick_OnFailure verifies that after pipeline failure,
// a kick activity is still called to drain the queue and maintain concurrency.
func (s *AutoDrainTestSuite) TestPipelineAutoKick_OnFailure() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    200,
		ContentID:   "em-test02",
		JobID:       "job-200",
		ContentType: "email",
		ContentHash: "hash200",
		BodyText:    "Test email",
		Subject:     "Test",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse succeeds
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  "Test email",
		NewContent: "Test email",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage FAILS
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("triage failed: model timeout"))

	// When triage fails, the workflow updates status to "rejected" (graceful handling)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "rejected"
	})).Return(nil)

	// CRITICAL: Even on failure, the auto-drain should kick the next pending item
	kickCalled := false
	s.activities.On("KickNextPending", mock.Anything, mock.MatchedBy(func(in KickNextPendingInput) bool {
		kickCalled = true
		return in.TenantID == "tenant-1"
	})).Return(&KickNextPendingOutput{
		QueuedCount: 1,
		Message:     "Kicked 1 pending item",
	}, nil)

	// Execute
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	// The workflow completes successfully even when triage fails (graceful handling)
	// It sets status to "rejected" and returns no error
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("rejected", result.Status)

	// VERIFY: The kick activity was called even though the pipeline was rejected
	s.True(kickCalled, "Expected KickNextPending to be called after triage rejection, but it was not")
	s.activities.AssertCalled(s.T(), "KickNextPending", mock.Anything, mock.Anything)
}

// TestPipelineAutoKick_KickFailureNonBlocking verifies that if the kick fails,
// the pipeline still returns success. The kick is best-effort and should not block completion.
func (s *AutoDrainTestSuite) TestPipelineAutoKick_KickFailureNonBlocking() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    300,
		ContentID:   "em-test03",
		JobID:       "job-300",
		ContentType: "email",
		ContentHash: "hash300",
		BodyText:    "Test email",
		Subject:     "Test",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  "Test email",
		NewContent: "Test email",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage (skip deep to make test faster)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b",
		Reason:     "Personal email",
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5003), nil)

	// Final status update to "completed"
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// CRITICAL: The kick fails, but the pipeline should still complete successfully
	s.activities.On("KickNextPending", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("kick failed: gateway unavailable"))

	// Execute
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	// The workflow should NOT error just because the kick failed
	require.NoError(s.T(), s.env.GetWorkflowError(), "Pipeline should complete successfully even if kick fails")

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// VERIFY: The kick was attempted
	s.activities.AssertCalled(s.T(), "KickNextPending", mock.Anything, mock.Anything)
}

// TestPipelineAutoKick_NoPending verifies that when kick returns 0 queued,
// no error is raised. This is a normal condition when there are no pending items.
func (s *AutoDrainTestSuite) TestPipelineAutoKick_NoPending() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    400,
		ContentID:   "em-test04",
		JobID:       "job-400",
		ContentType: "email",
		ContentHash: "hash400",
		BodyText:    "Test email",
		Subject:     "Test",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  "Test email",
		NewContent: "Test email",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage (skip deep to make test faster)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b",
		Reason:     "Personal email",
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5004), nil)

	// Final status update to "completed"
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// CRITICAL: The kick returns 0 queued (no pending items), which is normal and should not error
	s.activities.On("KickNextPending", mock.Anything, mock.Anything).Return(&KickNextPendingOutput{
		QueuedCount: 0,
		Message:     "No pending items to kick",
	}, nil)

	// Execute
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// VERIFY: The kick was called and returned 0 queued
	s.activities.AssertCalled(s.T(), "KickNextPending", mock.Anything, mock.Anything)
}

// TestRunner for the auto-drain test suite
func TestAutoDrainSuite(t *testing.T) {
	suite.Run(t, new(AutoDrainTestSuite))
}

// KickNextPending mock activity implementation
func (m *PipelineMockActivities) KickNextPending(ctx context.Context, input KickNextPendingInput) (*KickNextPendingOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*KickNextPendingOutput), args.Error(1)
}
