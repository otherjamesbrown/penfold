package workflows

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// PipelineMockActivities provides mock implementations for pipeline activities.
type PipelineMockActivities struct {
	mock.Mock
}

func (m *PipelineMockActivities) ParseEmail(ctx context.Context, input ParseEmailInput) (*ParseEmailOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ParseEmailOutput), args.Error(1)
}

func (m *PipelineMockActivities) ParseTranscript(ctx context.Context, input ParseTranscriptInput) (*ParseTranscriptOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ParseTranscriptOutput), args.Error(1)
}

func (m *PipelineMockActivities) Triage(ctx context.Context, input TriageInput) (*TriageOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*TriageOutput), args.Error(1)
}

func (m *PipelineMockActivities) ExtractEntitiesActivity(ctx context.Context, input SLMPipelineExtractEntitiesInput) (*SLMPipelineExtractEntitiesOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SLMPipelineExtractEntitiesOutput), args.Error(1)
}

func (m *PipelineMockActivities) ExtractAssertions(ctx context.Context, input interface{}) (int, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int), args.Error(1)
}

func (m *PipelineMockActivities) BuildContextPackage(ctx context.Context, input BuildContextInput) (*BuildContextOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BuildContextOutput), args.Error(1)
}

func (m *PipelineMockActivities) DeepAnalyze(ctx context.Context, input DeepAnalyzeInput) (*DeepAnalyzeOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DeepAnalyzeOutput), args.Error(1)
}

func (m *PipelineMockActivities) PersistFindings(ctx context.Context, input PersistFindingsInput) (*PersistFindingsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PersistFindingsOutput), args.Error(1)
}

func (m *PipelineMockActivities) GenerateContentEmbedding(ctx context.Context, input GenerateEmbeddingInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

func (m *PipelineMockActivities) UpdateContentStatus(ctx context.Context, input UpdateContentStatusInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *PipelineMockActivities) DeleteEmbedding(ctx context.Context, embeddingID int64) error {
	args := m.Called(ctx, embeddingID)
	return args.Error(0)
}

func (m *PipelineMockActivities) RecordOverrides(ctx context.Context, input RecordOverridesInput) error {
	args := m.Called(ctx, input)
	return args.Error(0)
}

func (m *PipelineMockActivities) FetchSource(ctx context.Context, input FetchSourceInput) (*FetchSourceOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchSourceOutput), args.Error(1)
}

// SLMPipelineTestSuite tests the SLMPipelineWorkflow.
type SLMPipelineTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *SLMPipelineTestSuite) SetupTest() {
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
}

func (s *SLMPipelineTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestSLMPipeline_FullPipeline tests the full pipeline for a high-importance email.
func (s *SLMPipelineTestSuite) TestSLMPipeline_FullPipeline() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    100,
		ContentID:   "em-test01",
		JobID:       "job-100",
		ContentType: "email",
		ContentHash: "hash100",
		BodyText:    "Project Alpha is at risk. The timeline has slipped by 2 weeks.",
		Subject:     "Risk Update",
		SenderEmail: "pm@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 100
	})).Return(&ParseEmailOutput{
		CleanBody:  "Project Alpha is at risk. The timeline has slipped by 2 weeks.",
		NewContent: "Project Alpha is at risk. The timeline has slipped by 2 weeks.",
	}, nil)

	// UpdateContentStatus: parsed
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 100 && in.ContentType == "email"
	})).Return(&TriageOutput{
		Category:   "RISK_ISSUE",
		Importance: "HIGH",
		Reason:     "Risk escalation",
		ModelUsed:  "llama-3.2-1b",
		SkipDeep:   false,
	}, nil)

	// Stage 2: Extract
	personID := int64(42)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		return in.SourceID == 100
	})).Return(&SLMPipelineExtractEntitiesOutput{
		People:   []PersonResult{{Name: "PM User", Role: "project_manager"}},
		Projects: []string{"Project Alpha"},
		Risks:    []string{"Timeline slipped by 2 weeks"},
	}, nil)

	// UpdateContentStatus: extracted
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "extracted"
	})).Return(nil)

	// Stage 3: Context
	s.activities.On("BuildContextPackage", mock.Anything, mock.MatchedBy(func(in BuildContextInput) bool {
		return in.SourceID == 100
	})).Return(&BuildContextOutput{
		ResolvedPeople:   []ResolvedPerson{{Name: "PM User", PersonID: &personID, Confidence: 0.95, Source: "exact_match"}},
		EntitiesResolved: 1,
		TokensUsed:       500,
		TokenBudget:      2000,
	}, nil)

	// Stage 4: Analyze
	s.activities.On("DeepAnalyze", mock.Anything, mock.MatchedBy(func(in DeepAnalyzeInput) bool {
		return in.SourceID == 100 && in.TriageCategory == "RISK_ISSUE"
	})).Return(&DeepAnalyzeOutput{
		Summary:   "Risk escalation for Project Alpha",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: Persist
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 100 && in.Analysis != nil
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 2,
		ReferencesCreated: 1,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 100
	})).Return(int64(5001), nil)

	// UpdateContentStatus: completed
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// Execute
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("RISK_ISSUE", result.Category)
	s.Equal("HIGH", result.Importance)
	s.False(result.SkipDeep)
	s.NotNil(result.EmbeddingID)
	s.Equal(int64(5001), *result.EmbeddingID)
}

// TestSLMPipeline_TriageSkip tests that PERSONAL content skips deep processing.
func (s *SLMPipelineTestSuite) TestSLMPipeline_TriageSkip() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    200,
		ContentID:   "em-test02",
		JobID:       "job-200",
		ContentType: "email",
		ContentHash: "hash200",
		BodyText:    "Hey, lunch at noon?",
		Subject:     "Lunch",
		SenderEmail: "friend@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Hey, lunch at noon?",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — PERSONAL
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 5: Embed (skip Stages 2-4.5)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5002), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("PERSONAL", result.Category)
	s.True(result.SkipDeep)
	s.NotNil(result.EmbeddingID)

	// Verify deep processing activities were NOT called
	s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "BuildContextPackage", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "PersistFindings", mock.Anything, mock.Anything)
}

// TestSLMPipeline_Stage4Failure tests that Stage 4 failure doesn't block the pipeline.
func (s *SLMPipelineTestSuite) TestSLMPipeline_Stage4Failure() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    300,
		ContentID:   "em-test03",
		JobID:       "job-300",
		ContentType: "email",
		ContentHash: "hash300",
		BodyText:    "Project update: Phase 2 started.",
		Subject:     "Update",
		SenderEmail: "pm@example.com",
	}

	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Project update: Phase 2 started.",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed" || in.Status == "extracted" || in.Status == "completed"
	})).Return(nil)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category: "PROJECT_UPDATE", Importance: "MEDIUM", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		Projects: []string{"Project X"},
	}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)

	// Stage 4 FAILS
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("LLM timeout", "TimeoutError"),
	)

	// Stage 5 still runs
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5003), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)

	// PersistFindings should NOT be called since analysis failed
	s.activities.AssertNotCalled(s.T(), "PersistFindings", mock.Anything, mock.Anything)
}

// TestSLMPipeline_EmbeddingFailure tests that embedding failure fails the pipeline.
func (s *SLMPipelineTestSuite) TestSLMPipeline_EmbeddingFailure() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    400,
		ContentID:   "em-test04",
		JobID:       "job-400",
		ContentType: "email",
		ContentHash: "hash400",
		BodyText:    "Quick question about the budget.",
		Subject:     "Budget",
		SenderEmail: "cfo@example.com",
	}

	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Quick question about the budget.",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category: "INTERNAL_COMMS", Importance: "MEDIUM", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Budget inquiry", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)

	// Stage 5 FAILS
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(
		int64(0), temporal.NewApplicationError("embedding service down", "ServiceUnavailable"),
	)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("failed", result.Status)
	s.Contains(result.Error, "embedding_failed")
	s.Nil(result.EmbeddingID)
}

// TestSLMPipeline_MeetingTranscript tests the pipeline with a meeting transcript.
func (s *SLMPipelineTestSuite) TestSLMPipeline_MeetingTranscript() {
	input := PipelineInput{
		TenantID:          "tenant-1",
		SourceID:          500,
		ContentID:         "mt-test05",
		JobID:             "job-500",
		ContentType:       "meeting",
		ContentHash:       "hash500",
		TranscriptContent: "00:00:01.000 --> 00:00:05.000\nSara: Let's review the risks.\n",
		TranscriptFormat:  "vtt",
	}

	// Stage 0: ParseTranscript
	s.activities.On("ParseTranscript", mock.Anything, mock.MatchedBy(func(in ParseTranscriptInput) bool {
		return in.SourceID == 500 && in.Format == "vtt"
	})).Return(&ParseTranscriptOutput{
		CleanText:  "Sara: Let's review the risks.",
		Speakers:   []string{"Sara"},
		DurationMs: 5000,
		Format:     "vtt",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.ContentType == "meeting"
	})).Return(&TriageOutput{
		Category: "PROJECT_UPDATE", Importance: "HIGH", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)

	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{{Name: "Sara"}},
	}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Risk review meeting", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{
		AssertionsCreated: 1,
	}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5005), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("PROJECT_UPDATE", result.Category)
}

// TestSLMPipeline_Stage4Timeout tests that Stage 4 timeout is handled gracefully.
func (s *SLMPipelineTestSuite) TestSLMPipeline_Stage4Timeout() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    600,
		ContentID:   "em-test06",
		JobID:       "job-600",
		ContentType: "email",
		ContentHash: "hash600",
		BodyText:    "Important customer feedback received.",
		Subject:     "Feedback",
		SenderEmail: "customer@example.com",
	}

	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Important customer feedback received.",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category: "CUSTOMER", Importance: "HIGH", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)

	// Stage 4 times out
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(
		nil, temporal.NewApplicationError("activity timeout", "TimeoutError"),
	)

	// Pipeline continues to embedding
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5006), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.NotNil(result.EmbeddingID)
	s.activities.AssertNotCalled(s.T(), "PersistFindings", mock.Anything, mock.Anything)
}

// TestSLMPipeline_QueryStatus tests the status query handler.
func (s *SLMPipelineTestSuite) TestSLMPipeline_QueryStatus() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    700,
		ContentID:   "em-test07",
		JobID:       "job-700",
		ContentType: "email",
		ContentHash: "hash700",
		BodyText:    "Status check content.",
		Subject:     "Status",
		SenderEmail: "user@example.com",
	}

	// Minimal mocks for quick completion (PERSONAL skip)
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Status check content.",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category: "PERSONAL", Importance: "LOW", SkipDeep: true, ModelUsed: "llama-3.2-1b",
	}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5007), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())

	// Query the final status
	queryResult, err := s.env.QueryWorkflow(PipelineStatusQuery)
	require.NoError(s.T(), err)

	var status PipelineStatus
	require.NoError(s.T(), queryResult.Get(&status))
	s.Equal("completed", status.Stage)
	s.Equal(3, status.StepsCompleted) // parse, triage, embed
	s.Equal(3, status.TotalSteps)
}

// TestSLMPipeline_Cancellation tests cancellation signal handling.
func (s *SLMPipelineTestSuite) TestSLMPipeline_Cancellation() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    800,
		ContentID:   "em-test08",
		JobID:       "job-800",
		ContentType: "email",
		ContentHash: "hash800",
		BodyText:    "Content to be cancelled.",
		Subject:     "Cancel Me",
		SenderEmail: "user@example.com",
	}

	// Stage 0 succeeds
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Content to be cancelled.",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Stage 1 succeeds
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category: "PROJECT_UPDATE", Importance: "HIGH", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)

	// Send cancel signal after triage
	s.env.RegisterDelayedCallback(func() {
		s.env.SignalWorkflow(PipelineCancelSignal, pkgtemporal.CancelWithCompensationSignal{
			Reason: "User requested cancellation",
		})
	}, 0)

	// These may or may not be called depending on timing
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Maybe().Return(&SLMPipelineExtractEntitiesOutput{}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Maybe().Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Maybe().Return(&DeepAnalyzeOutput{}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Maybe().Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("cancelled", result.Status)
	s.Contains(result.Error, "User requested cancellation")
}

// TestSLMPipeline_TriageValidationFailure tests that triage validation errors
// (e.g., empty content) properly reject the source instead of leaving it pending.
func (s *SLMPipelineTestSuite) TestSLMPipeline_TriageValidationFailure() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    800,
		ContentID:   "em-test08",
		JobID:       "job-800",
		ContentType: "email",
		ContentHash: "hash800",
		BodyText:    "\r\n",
		Subject:     "Calendar Placeholder",
		SenderEmail: "calendar@example.com",
	}

	// Stage 0: Parse returns nearly empty content
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 800
	})).Return(&ParseEmailOutput{
		CleanBody:  "",
		NewContent: "",
	}, nil)

	// UpdateContentStatus: parsed
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage FAILS with validation error (empty content)
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 800
	})).Return(
		nil, temporal.NewApplicationError("content is empty after parsing", "ValidationError"),
	)

	// UpdateContentStatus: rejected with empty_content category
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "rejected" && in.FailureCategory == "empty_content"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("rejected", result.Status)
	s.Contains(result.Error, "empty_content")

	// Verify no further stages were executed
	s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "BuildContextPackage", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "GenerateContentEmbedding", mock.Anything, mock.Anything)
}

// TestSLMPipeline_MeetingReprocess_FetchSourceFallback tests the FetchSource fallback
// path for meeting reprocessing via minimal SLMPipelineInput (pf-0065d5).
// This test SHOULD FAIL because the FetchSource fallback sets BodyText but NOT
// TranscriptContent, causing ParseTranscript to receive empty Content.
func (s *SLMPipelineTestSuite) TestSLMPipeline_MeetingReprocess_FetchSourceFallback() {
	// Minimal input (SLMPipelineInput contract) - no ContentType, triggers FetchSource fallback
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    900,
		ContentID:   "mt-test09",
		JobID:       "job-900",
		ContentType: "", // EMPTY - triggers FetchSource fallback path
		ContentHash: "hash900",
	}

	// FetchSource returns meeting content type with transcript in ContentText
	s.activities.On("FetchSource", mock.Anything, mock.MatchedBy(func(in FetchSourceInput) bool {
		return in.SourceID == 900
	})).Return(&FetchSourceOutput{
		ContentType: "meeting",
		ContentText: "Speaker 1: Hello, this is a test meeting transcript.",
		Subject:     "Test Meeting",
	}, nil)

	// Stage 0: ParseTranscript should receive NON-EMPTY Content
	// BUG: It will receive EMPTY Content because FetchSource sets BodyText, not TranscriptContent
	s.activities.On("ParseTranscript", mock.Anything, mock.MatchedBy(func(in ParseTranscriptInput) bool {
		// This assertion SHOULD pass but WILL FAIL due to the bug
		if in.Content == "" {
			s.T().Errorf("BUG REPRODUCED: ParseTranscript received empty Content (expected transcript from FetchSource)")
		}
		return in.SourceID == 900
	})).Return(&ParseTranscriptOutput{
		CleanText:  "Speaker 1: Hello, this is a test meeting transcript.",
		Speakers:   []string{"Speaker 1"},
		DurationMs: 3000,
		Format:     "plain",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.ContentType == "meeting"
	})).Return(&TriageOutput{
		Category: "MEETING", Importance: "MEDIUM", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)

	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{{Name: "Speaker 1"}},
	}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Test meeting summary", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5009), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

// TestSLMPipeline_EmailReprocess_FetchSourceBodyHTMLFallback tests the FetchSource fallback
// path for email reprocessing via minimal SLMPipelineInput (pf-dfbc24).
// This test SHOULD FAIL because the FetchSource fallback sets BodyText but NOT
// BodyHTML, and ParseEmail should receive both fields for proper HTML parsing.
func (s *SLMPipelineTestSuite) TestSLMPipeline_EmailReprocess_FetchSourceBodyHTMLFallback() {
	// Minimal input (SLMPipelineInput contract) - no ContentType, triggers FetchSource fallback
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    1000,
		ContentID:   "em-test10",
		JobID:       "job-1000",
		ContentType: "", // EMPTY - triggers FetchSource fallback path
		ContentHash: "hash1000",
	}

	// FetchSource returns email content type with HTML in body_html metadata
	// In the real system, body_html would be in ingestion_metadata and should be returned
	s.activities.On("FetchSource", mock.Anything, mock.MatchedBy(func(in FetchSourceInput) bool {
		return in.SourceID == 1000
	})).Return(&FetchSourceOutput{
		ContentType: "email",
		ContentText: "Plain text version of the email body.",
		Subject:     "Test Email with HTML",
		SenderEmail: "sender@example.com",
		SenderName:  "Test Sender",
		BodyHTML:    "<html><body>HTML version of the email body.</body></html>", // pf-dfbc24: FIX - return BodyHTML from FetchSource
	}, nil)

	// Stage 0: ParseEmail should receive BOTH BodyText AND BodyHTML
	// BUG: It will receive empty BodyHTML because FetchSource doesn't populate it
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		// This assertion tracks whether BodyHTML is populated from FetchSource
		if in.BodyHTML == "" {
			s.T().Errorf("BUG REPRODUCED: ParseEmail received empty BodyHTML (expected HTML content from FetchSource)")
		}
		// The test expects BOTH BodyText and BodyHTML to be set from FetchSource
		if in.BodyText == "" {
			s.T().Errorf("ParseEmail received empty BodyText (regression)")
		}
		return in.SourceID == 1000
	})).Return(&ParseEmailOutput{
		CleanBody:  "Plain text version of the email body.",
		NewContent: "Plain text version of the email body.",
		IsReply:    false,
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.ContentType == "email"
	})).Return(&TriageOutput{
		Category: "INTERNAL_COMMS", Importance: "MEDIUM", SkipDeep: false, ModelUsed: "llama-3.2-1b",
	}, nil)

	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{{Name: "Test Sender"}},
	}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Test email summary", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5010), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

// TestSLMPipeline_CleanupOnCancellation tests that workflow cancellation
// triggers cleanup and updates status to 'failed'.
// This test reproduces bug pf-caed79: workflow timeout/cancellation leaves status='pending'.
// The bug: There's no defer block with disconnected context to update status on cancellation.
// Expected behavior: UpdateContentStatus activity should be called with status='failed'.
func (s *SLMPipelineTestSuite) TestSLMPipeline_CleanupOnCancellation() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    900,
		ContentID:   "em-test09",
		JobID:       "job-900",
		ContentType: "email",
		ContentHash: "hash900",
		BodyText:    "Project update email content.",
		Subject:     "Project Status",
		SenderEmail: "pm@example.com",
	}

	// Stage 0: Parse succeeds
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 900
	})).Return(&ParseEmailOutput{
		CleanBody:  "Project update email content.",
		NewContent: "Project update email content.",
	}, nil)

	// UpdateContentStatus: parsed (before cancellation)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Maybe().Return(nil)

	// Stage 1: Triage succeeds
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 900
	})).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// UpdateContentStatus: triage metadata (before cancellation)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed" && in.TriageCategory == "PROJECT_UPDATE"
	})).Maybe().Return(nil)

	// Cancel the workflow via context cancellation (simulates timeout)
	// This differs from signal-based cancellation in TestSLMPipeline_Cancellation
	s.env.RegisterDelayedCallback(func() {
		s.env.CancelWorkflow()
	}, 0)

	// These activities may or may not be called depending on timing
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Maybe().Return(&SLMPipelineExtractEntitiesOutput{}, nil)
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Maybe().Return(0, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Maybe().Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Maybe().Return(&DeepAnalyzeOutput{}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Maybe().Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)

	// CRITICAL ASSERTION: UpdateContentStatus should be called with status='failed' on cancellation
	// This is the cleanup mechanism that should exist in a defer block with disconnected context
	cleanupCalled := false
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		if in.Status == "failed" && in.SourceID == 900 {
			cleanupCalled = true
			return true
		}
		return false
	})).Maybe().Return(nil)

	// Execute workflow
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())

	// The test expectation: cleanup should have been called
	// BUG pf-caed79: This assertion will FAIL because there's no defer block cleanup
	require.True(s.T(), cleanupCalled,
		"BUG pf-caed79: Workflow cancellation should trigger cleanup via UpdateContentStatus with status='failed'. "+
			"Currently there's no defer block with disconnected context to handle timeout/cancellation cleanup.")
}

// TestSLMPipeline_PersistFindingsReceivesParsedContent tests that PersistFindings
// receives PARSED content (from ParseEmail), not RAW content (from FetchSource).
// This test reproduces bug pf-fc4b48: acronym detection receives RAW email content.
func (s *SLMPipelineTestSuite) TestSLMPipeline_PersistFindingsReceivesParsedContent() {
	rawBodyWithEmailHeaders := `From: sender@example.com
To: recipient@example.com
Subject: Project Update
Message-ID: <abc123@mail.example.com>
X-Routing-ID: routing-xyz-789

CLIC is our project framework for managing initiatives.
We need to review the CLIC workflow next week.

--- Original Message ---
On Jan 1, 2026, John wrote:
> Let's discuss CLIC implementation.`

	parsedCleanBody := `CLIC is our project framework for managing initiatives.
We need to review the CLIC workflow next week.`

	// Minimal input (SLMPipelineInput contract) - triggers FetchSource fallback
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    1100,
		ContentID:   "em-test11",
		JobID:       "job-1100",
		ContentType: "", // EMPTY - triggers FetchSource fallback path
		ContentHash: "hash1100",
	}

	// FetchSource returns RAW content with email headers
	s.activities.On("FetchSource", mock.Anything, mock.MatchedBy(func(in FetchSourceInput) bool {
		return in.SourceID == 1100
	})).Return(&FetchSourceOutput{
		ContentType: "email",
		ContentText: rawBodyWithEmailHeaders,
		Subject:     "Project Update",
		SenderEmail: "sender@example.com",
		SenderName:  "Sender Name",
	}, nil)

	// Stage 0: ParseEmail cleans the content and returns only NEW content
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 1100 && in.BodyText == rawBodyWithEmailHeaders
	})).Return(&ParseEmailOutput{
		CleanBody:     parsedCleanBody,
		NewContent:    parsedCleanBody,
		QuotedContent: "On Jan 1, 2026, John wrote:\n> Let's discuss CLIC implementation.",
		IsReply:       true,
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.ContentType == "email"
	})).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 2: Extract
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		Projects: []string{"CLIC"},
		People:   []PersonResult{{Name: "Sender Name"}},
	}, nil)

	// Stage 3: Context
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)

	// Stage 4: Analyze
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary:   "Project update about CLIC framework",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: PersistFindings - THIS IS THE KEY ASSERTION
	// We capture what BodyText is actually passed to PersistFindings
	var capturedBodyText string
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		capturedBodyText = in.BodyText
		return in.SourceID == 1100
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 2,
		ReferencesCreated: 1,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5011), nil)

	// Execute workflow
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// CRITICAL ASSERTION: PersistFindings should receive PARSED content, not RAW content
	// BUG pf-fc4b48: Currently it receives rawBodyWithEmailHeaders instead of parsedCleanBody
	s.Equal(parsedCleanBody, capturedBodyText,
		"PersistFindings should receive PARSED content from ParseEmail, not RAW content from FetchSource. "+
			"Bug pf-fc4b48: Acronym detection receives RAW email content with headers and quoted replies.")
}

func TestExtractSignature(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "RFC 3676 separator with signature",
			body:     "Email body text here.\n\n-- \nJohn Doe\nSenior Engineer",
			expected: "-- \nJohn Doe\nSenior Engineer",
		},
		{
			name:     "Best regards sign-off",
			body:     "Email body text here.\n\nBest regards,\nJane Smith\nProduct Manager",
			expected: "Best regards,\nJane Smith\nProduct Manager",
		},
		{
			name:     "Kind regards sign-off",
			body:     "Email content.\n\nKind regards,\nBob",
			expected: "Kind regards,\nBob",
		},
		{
			name:     "Simple Thanks sign-off",
			body:     "Quick message.\n\nThanks,\nAlice",
			expected: "Thanks,\nAlice",
		},
		{
			name:     "No signature markers",
			body:     "Just a plain email with no signature markers at all",
			expected: "",
		},
		{
			name:     "Empty body",
			body:     "",
			expected: "",
		},
		{
			name: "Multiple markers - uses last one",
			body: "Email starts here.\n\nThanks for that.\n\nBest regards,\nJohn",
			expected: "Best regards,\nJohn",
		},
		{
			name:     "Sincerely sign-off",
			body:     "Formal email.\n\nSincerely,\nDr. Smith",
			expected: "Sincerely,\nDr. Smith",
		},
		{
			name:     "Cheers sign-off",
			body:     "Casual email.\n\nCheers,\nMike",
			expected: "Cheers,\nMike",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSignature(tt.body)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSLMPipelineTestSuite(t *testing.T) {
	suite.Run(t, new(SLMPipelineTestSuite))
}
