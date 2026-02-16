package workflows

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

// PipelineOverridesTestSuite tests the PipelineInput override fields.
type PipelineOverridesTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *PipelineOverridesTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &PipelineMockActivities{}

	// Register all pipeline activities
	s.env.RegisterActivityWithOptions(s.activities.ParseEmail, activity.RegisterOptions{Name: "ParseEmail"})
	s.env.RegisterActivityWithOptions(s.activities.Triage, activity.RegisterOptions{Name: "Triage"})
	s.env.RegisterActivityWithOptions(s.activities.ExtractEntitiesActivity, activity.RegisterOptions{Name: "ExtractEntitiesActivity"})
	s.env.RegisterActivityWithOptions(s.activities.BuildContextPackage, activity.RegisterOptions{Name: "BuildContextPackage"})
	s.env.RegisterActivityWithOptions(s.activities.DeepAnalyze, activity.RegisterOptions{Name: "DeepAnalyze"})
	s.env.RegisterActivityWithOptions(s.activities.PersistFindings, activity.RegisterOptions{Name: "PersistFindings"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	s.env.RegisterActivityWithOptions(s.activities.RecordOverrides, activity.RegisterOptions{Name: "RecordOverrides"})
	s.env.RegisterActivityWithOptions(s.activities.GroupEmailThread, activity.RegisterOptions{Name: "GroupEmailThread"})
	s.env.RegisterActivityWithOptions(s.activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: "CreateEnrichmentRecord"})

	// Default mock expectations for enrichment/threading activities (blocking since pf-67502c fix)
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
}

func (s *PipelineOverridesTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestPipelineInput_HasOverrideFields tests that PipelineInput has ModelOverride and TimeoutOverride fields.
// This test will fail because the fields don't exist yet.
func (s *PipelineOverridesTestSuite) TestPipelineInput_HasOverrideFields() {
	// Verify PipelineInput struct has the override fields
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    100,
		ContentID:   "em-test01",
		JobID:       "job-100",
		ContentType: "email",
		BodyText:    "Test content",
	}

	// This will fail to compile because ModelOverride field doesn't exist
	input.ModelOverride = "llama-3.2-3b"
	require.Equal(s.T(), "llama-3.2-3b", input.ModelOverride, "ModelOverride field should be settable")

	// This will fail to compile because TimeoutOverride field doesn't exist
	input.TimeoutOverride = 5 * time.Minute
	require.Equal(s.T(), 5*time.Minute, input.TimeoutOverride, "TimeoutOverride field should be settable")
}

// TestPipelineWorkflow_UsesModelOverride tests that when ModelOverride is set, it's used by activities.
// This test will fail because the workflow doesn't pass model overrides to activities.
func (s *PipelineOverridesTestSuite) TestPipelineWorkflow_UsesModelOverride() {
	input := PipelineInput{
		TenantID:      "tenant-1",
		SourceID:      200,
		ContentID:     "em-test02",
		JobID:         "job-200",
		ContentType:   "email",
		BodyText:      "Test with model override",
		ModelOverride: "llama-3.2-3b", // Override to use a different model
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test with model override",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage should receive model override
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		// This will fail because TriageInput doesn't have ModelOverride field yet
		return in.ModelOverride == "llama-3.2-3b"
	})).Return(&TriageOutput{
		Category:   "INTERNAL_COMMS",
		Importance: "MEDIUM",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-3b", // Activity used the override model
	}, nil)

	// Skip deep processing for simplicity
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5002), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("llama-3.2-3b", result.ModelUsed, "Result should reflect the override model used")
}

// TestPipelineWorkflow_UsesTimeoutOverride tests that when TimeoutOverride is set, activity timeout changes.
// This test will fail because the workflow doesn't apply timeout overrides to activity options.
func (s *PipelineOverridesTestSuite) TestPipelineWorkflow_UsesTimeoutOverride() {
	input := PipelineInput{
		TenantID:        "tenant-1",
		SourceID:        300,
		ContentID:       "em-test03",
		JobID:           "job-300",
		ContentType:     "email",
		BodyText:        "Test with timeout override",
		TimeoutOverride: 15 * time.Minute, // Override to 15 minutes for slow processing
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test with timeout override",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage (should use extended timeout)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 2: Extract (should use extended timeout)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		Projects: []string{"Project X"},
	}, nil)

	// Stage 3: Context
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)

	// Stage 4: Analyze (should use extended timeout from override)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary:   "Test summary",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: Persist
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{
		AssertionsCreated: 1,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5003), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// Verify that activities were called (timeout behavior is internal to Temporal,
	// but we can verify the workflow completed successfully with the override)
	s.activities.AssertCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
}

// TestPipelineWorkflow_EmptyOverrides tests that default behavior is unchanged with empty overrides.
// This test verifies that providing no overrides (empty string, zero duration) works as before.
func (s *PipelineOverridesTestSuite) TestPipelineWorkflow_EmptyOverrides() {
	input := PipelineInput{
		TenantID:        "tenant-1",
		SourceID:        400,
		ContentID:       "em-test04",
		JobID:           "job-400",
		ContentType:     "email",
		BodyText:        "Test without overrides",
		ModelOverride:   "", // Empty - use default
		TimeoutOverride: 0,  // Zero - use default
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test without overrides",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage should use default model
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b", // Default model
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5004), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("llama-3.2-1b", result.ModelUsed, "Should use default model when override is empty")
}

// TestPipelineWorkflow_RecordsOverrides tests that override parameters are recorded in pipeline_runs.
// This test will fail because the workflow doesn't call RecordOverrides yet.
func (s *PipelineOverridesTestSuite) TestPipelineWorkflow_RecordsOverrides() {
	// Note: This test focuses on workflow behavior. The actual RecordOverrides activity
	// would need to be mocked or tested separately in the activities package.

	input := PipelineInput{
		TenantID:        "tenant-1",
		SourceID:        500,
		ContentID:       "em-test05",
		JobID:           "job-500",
		ContentType:     "email",
		BodyText:        "Test recording overrides",
		ModelOverride:   "llama-3.2-3b",
		TimeoutOverride: 10 * time.Minute,
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test recording overrides",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-3b",
	}, nil)

	// RecordOverrides activity should be called with override parameters
	// This will fail because the activity doesn't exist or isn't called yet
	s.activities.On("RecordOverrides", mock.Anything, mock.MatchedBy(func(in RecordOverridesInput) bool {
		// Verify the override values are passed correctly
		return in.SourceID == 500 &&
			in.Overrides["model_override"] == "llama-3.2-3b" &&
			in.Overrides["timeout_override"] == "10m0s"
	})).Return(nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5005), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// Verify RecordOverrides was called
	s.activities.AssertCalled(s.T(), "RecordOverrides", mock.Anything, mock.Anything)
}

// TestPipelineWorkflow_BothOverrides tests that both model and timeout overrides work together.
func (s *PipelineOverridesTestSuite) TestPipelineWorkflow_BothOverrides() {
	input := PipelineInput{
		TenantID:        "tenant-1",
		SourceID:        600,
		ContentID:       "em-test06",
		JobID:           "job-600",
		ContentType:     "email",
		BodyText:        "Test both overrides",
		ModelOverride:   "gemini-2.0-flash-exp",
		TimeoutOverride: 20 * time.Minute,
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test both overrides",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage should use both overrides
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		// Verify model override is passed
		return in.ModelOverride == "gemini-2.0-flash-exp"
	})).Return(&TriageOutput{
		Category:   "RISK_ISSUE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "gemini-2.0-flash-exp",
	}, nil)

	// Stage 2-4: Deep processing with overrides
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		Risks: []string{"Critical risk"},
	}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary:   "Risk analysis",
		ModelUsed: "gemini-2.0-flash-exp",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5006), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("gemini-2.0-flash-exp", result.ModelUsed, "Should use override model")
}

func TestPipelineOverridesTestSuite(t *testing.T) {
	suite.Run(t, new(PipelineOverridesTestSuite))
}
