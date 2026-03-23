package workflows

// PipelineFallbackRemovalTestSuite verifies that the workflow fails visibly when pipeline
// definitions are missing or misconfigured — no silent fallback to hardcoded stages.
//
// These tests cover the three failure cases from pf-4ec560:
//  1. No pipeline_definitions rows → FetchPipelineDefinition returns ConfigurationError →
//     workflow sets source status=failed with failure_category=configuration_error
//  2. All stages disabled → FetchPipelineDefinition returns definition with 0 enabled stages →
//     workflow fails with NonRetryableApplicationError "has no enabled stages"
//  3. Valid definition → workflow proceeds normally (positive control)
//
// Tests run in the Temporal workflow test environment with mocked activities.
// They exercise the post-triage FetchPipelineDefinition code path in pipeline.go.

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// PipelineFallbackRemovalTestSuite tests that missing pipeline definitions fail visibly.
type PipelineFallbackRemovalTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *PipelineFallbackRemovalTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &PipelineMockActivities{}

	// Register all activities that may be called across the three test scenarios.
	s.env.RegisterActivityWithOptions(s.activities.ParseEmail, activity.RegisterOptions{Name: "ParseEmail"})
	s.env.RegisterActivityWithOptions(s.activities.Triage, activity.RegisterOptions{Name: "Triage"})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	s.env.RegisterActivityWithOptions(s.activities.FetchPipelineDefinition, activity.RegisterOptions{Name: pkgtemporal.ActivityFetchPipelineDefinition})
	s.env.RegisterActivityWithOptions(s.activities.KickNextPending, activity.RegisterOptions{Name: pkgtemporal.ActivityKickNextPending})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	s.env.RegisterActivityWithOptions(s.activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: "CreateEnrichmentRecord"})
	s.env.RegisterActivityWithOptions(s.activities.GroupEmailThread, activity.RegisterOptions{Name: "GroupEmailThread"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentSummary})
	s.env.RegisterActivityWithOptions(s.activities.DeleteAssertions, activity.RegisterOptions{Name: pkgtemporal.ActivityDeleteAssertions})

	// Default best-effort activity mocks — individual tests override as needed.
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
	s.activities.On("DeleteAssertions", mock.Anything, mock.Anything).Maybe().Return(&DeleteAssertionsOutput{Deleted: 0}, nil)
	// KickNextPending is called by the deferred cleanup on failure and on success.
	s.activities.On("KickNextPending", mock.Anything, mock.Anything).Maybe().Return(&KickNextPendingOutput{QueuedCount: 0}, nil)
}

func (s *PipelineFallbackRemovalTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestMissingPipelineDefinition_FailsWithConfigurationError verifies that when no
// pipeline definition exists in the DB, the workflow marks the source as failed with
// failure_category=configuration_error rather than silently falling back to SLMPipelineStages.
//
// This covers the defErr != nil branch in the post-triage FetchPipelineDefinition block:
// the activity returns a NonRetryableApplicationError (ConfigurationError), and the workflow
// handles it by setting source status=failed and returning nil (no workflow error).
func (s *PipelineFallbackRemovalTestSuite) TestMissingPipelineDefinition_FailsWithConfigurationError() {
	const tenantID = "tenant-1"
	const sourceID = int64(1001)

	input := PipelineInput{
		TenantID:    tenantID,
		SourceID:    sourceID,
		ContentID:   "em-missing-def",
		JobID:       "job-missing-def",
		ContentType: "email",
		BodyText:    "Pipeline definition missing test email",
		Subject:     "Test",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  input.BodyText,
		NewContent: input.BodyText,
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — routes to a pipeline that has no DB definition.
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		Pipelines:  []string{"no-such-pipeline"},
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// FetchPipelineDefinition: the activity returns NonRetryableApplicationError when no
	// pipeline definition rows exist — this is what the real activity does (not Found=false).
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "no-such-pipeline" && in.TenantID == tenantID
	})).Return(nil, temporal.NewNonRetryableApplicationError(
		"pipeline definition not found: pipeline=no-such-pipeline tenant=tenant-1",
		"ConfigurationError",
		nil,
	))

	// The workflow explicitly calls UpdateContentStatus with failure_category=configuration_error
	// before returning. Accept any "failed" call — the deferred cleanup may also call it.
	configErrCalled := false
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		if in.Status == "failed" && in.FailureCategory == "configuration_error" {
			configErrCalled = true
		}
		return in.Status == "failed"
	})).Maybe().Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted(), "workflow should complete")
	// The workflow handles the error internally: defErr != nil branch sets status=failed and
	// returns state.result, nil — the workflow itself does not return an error.
	require.NoError(s.T(), s.env.GetWorkflowError(), "workflow should return nil error (error handled internally)")

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "failed", result.Status, "source should be marked failed")
	require.True(s.T(), configErrCalled,
		"UpdateContentStatus should have been called with failure_category=configuration_error")
}

// TestAllStagesDisabled_FailsWithNoEnabledStages verifies that when a pipeline definition
// exists but all stages have enabled=false, the workflow fails with a visible error
// rather than silently producing no output.
func (s *PipelineFallbackRemovalTestSuite) TestAllStagesDisabled_FailsWithNoEnabledStages() {
	const tenantID = "tenant-1"
	const sourceID = int64(1002)

	input := PipelineInput{
		TenantID:    tenantID,
		SourceID:    sourceID,
		ContentID:   "em-disabled-stages",
		JobID:       "job-disabled-stages",
		ContentType: "email",
		BodyText:    "All stages disabled test email",
		Subject:     "Test",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  input.BodyText,
		NewContent: input.BodyText,
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — routes to the disabled pipeline.
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		Pipelines:  []string{"disabled-pipeline"},
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// FetchPipelineDefinition returns a definition where ALL stages have enabled=false.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "disabled-pipeline"
	})).Return(&FetchPipelineDefinitionOutput{
		Found: true,
		Stages: []PipelineStageConfig{
			{Stage: "parse", StageOrder: 0, Enabled: false},
			{Stage: "triage", StageOrder: 1, Enabled: false},
			{Stage: "embed", StageOrder: 2, Enabled: false},
		},
	}, nil)

	// Deferred cleanup runs on workflow error: accept any failed status call.
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "failed"
	})).Maybe().Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted(), "workflow should complete")

	// The workflow returns a NonRetryableApplicationError when all stages are disabled.
	workflowErr := s.env.GetWorkflowError()
	require.Error(s.T(), workflowErr, "workflow should fail with an error when all stages disabled")

	// Verify the error message contains "has no enabled stages".
	require.Contains(s.T(), workflowErr.Error(), "has no enabled stages",
		"workflow error should mention 'has no enabled stages'")
}

// TestValidDefinition_WorkflowSucceeds is a positive control verifying that when a valid
// pipeline definition exists with enabled stages, the workflow processes normally.
// This ensures the fallback removal did not break the happy path.
func (s *PipelineFallbackRemovalTestSuite) TestValidDefinition_WorkflowSucceeds() {
	const tenantID = "tenant-1"
	const sourceID = int64(1003)

	input := PipelineInput{
		TenantID:    tenantID,
		SourceID:    sourceID,
		ContentID:   "em-valid-def",
		JobID:       "job-valid-def",
		ContentType: "email",
		BodyText:    "Valid pipeline definition test email",
		Subject:     "Test",
		SenderEmail: "sender@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  input.BodyText,
		NewContent: input.BodyText,
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — routes to our valid-pipeline.
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		Pipelines:  []string{"valid-pipeline"},
		SkipDeep:   true, // skip deep processing to keep test minimal
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// FetchPipelineDefinition returns a valid definition with only the embed stage enabled.
	// SkipDeep=true from triage means extract/analyze stages are skipped regardless.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "valid-pipeline"
	})).Return(&FetchPipelineDefinitionOutput{
		Found: true,
		Stages: []PipelineStageConfig{
			{Stage: "parse", StageOrder: 0, Enabled: true, TimeoutSeconds: 60},
			{Stage: "triage", StageOrder: 1, Enabled: true, TimeoutSeconds: 120},
			{Stage: "embed", StageKind: "embedding", StageOrder: 5, Enabled: true, TimeoutSeconds: 60},
		},
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(9001), nil)

	// Final status update
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted(), "workflow should complete")
	require.NoError(s.T(), s.env.GetWorkflowError(), "workflow should succeed with valid definition")

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status,
		"source should be marked completed when valid definition is present")
}

func TestPipelineFallbackRemovalTestSuite(t *testing.T) {
	suite.Run(t, new(PipelineFallbackRemovalTestSuite))
}
