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
	s.env.RegisterActivityWithOptions(s.activities.BuildContextPackage, activity.RegisterOptions{Name: "BuildContextPackage"})
	s.env.RegisterActivityWithOptions(s.activities.DeepAnalyze, activity.RegisterOptions{Name: "DeepAnalyze"})
	s.env.RegisterActivityWithOptions(s.activities.PersistFindings, activity.RegisterOptions{Name: "PersistFindings"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	s.env.RegisterActivityWithOptions(s.activities.DeleteEmbedding, activity.RegisterOptions{Name: "DeleteEmbedding"})
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

func TestSLMPipelineTestSuite(t *testing.T) {
	suite.Run(t, new(SLMPipelineTestSuite))
}
