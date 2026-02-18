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

func (m *PipelineMockActivities) GroupEmailThread(ctx context.Context, input GroupEmailThreadInput) (*GroupEmailThreadOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GroupEmailThreadOutput), args.Error(1)
}

func (m *PipelineMockActivities) CreateEnrichmentRecord(ctx context.Context, input CreateEnrichmentRecordInput) (*CreateEnrichmentRecordOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CreateEnrichmentRecordOutput), args.Error(1)
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
	s.env.RegisterActivityWithOptions(s.activities.GroupEmailThread, activity.RegisterOptions{Name: "GroupEmailThread"})
	s.env.RegisterActivityWithOptions(s.activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: "CreateEnrichmentRecord"})

	// Default mock expectations for enrichment/threading activities (blocking since pf-67502c fix).
	// Individual tests can override these with more specific expectations.
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
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

func TestExtractSignaturesPerSender(t *testing.T) {
	tests := []struct {
		name     string
		bodyText string
		people   []ResolvedPerson
		expected map[string]string
	}{
		{
			name: "Threaded email with two senders",
			bodyText: `From: John Smith <john@example.com>
Subject: Re: Project Update

Thanks for the update. I'll review the roadmap this week.

Best,
John Smith
VP Engineering
Example Corp

On Mon, Feb 10, 2026, Sarah Chen wrote:

Here's the latest project update. Please review.

Regards,
Sarah Chen
Product Manager
Example Corp`,
			people: []ResolvedPerson{
				{Name: "John Smith"},
				{Name: "Sarah Chen"},
			},
			expected: map[string]string{
				"John Smith": "Best,\nJohn Smith\nVP Engineering\nExample Corp",
				"Sarah Chen": "Regards,\nSarah Chen\nProduct Manager\nExample Corp",
			},
		},
		{
			name: "Single message - returns empty map",
			bodyText: `Just a simple email with one signature.

Best regards,
Alice Smith
Engineer`,
			people: []ResolvedPerson{
				{Name: "Alice Smith"},
			},
			expected: nil,
		},
		{
			name: "Three-way thread with Original Message separator",
			bodyText: `From: Alice <alice@example.com>

I agree with both proposals.

Best,
Alice Smith
CTO

-----Original Message-----
From: Bob Jones

Sounds good to me.

Regards,
Bob Jones
Director

On Tue, Carol wrote:

Here's my proposal.

Cheers,
Carol Lee
Manager`,
			people: []ResolvedPerson{
				{Name: "Alice Smith"},
				{Name: "Bob Jones"},
				{Name: "Carol Lee"},
			},
			expected: map[string]string{
				"Alice Smith": "Best,\nAlice Smith\nCTO",
				"Bob Jones":   "Regards,\nBob Jones\nDirector",
				"Carol Lee":   "Cheers,\nCarol Lee\nManager",
			},
		},
		{
			name:     "Empty body - returns nil",
			bodyText: "",
			people: []ResolvedPerson{
				{Name: "John Smith"},
			},
			expected: nil,
		},
		{
			name: "No signature markers in thread - returns empty map",
			bodyText: `From: John <john@example.com>

Plain text.

On Mon, Sarah wrote:

More plain text.`,
			people: []ResolvedPerson{
				{Name: "John Smith"},
				{Name: "Sarah Chen"},
			},
			expected: map[string]string{},
		},
		{
			name: "Person not found in any block - missing from result",
			bodyText: `From: Alice <alice@example.com>

My update.

Best,
Alice Smith

On Mon, Bob wrote:

His update.

Regards,
Bob Jones`,
			people: []ResolvedPerson{
				{Name: "Alice Smith"},
				{Name: "Charlie Brown"}, // Not in thread
			},
			expected: map[string]string{
				"Alice Smith": "Best,\nAlice Smith",
			},
		},
		{
			name: "Cross-contamination bug: name in quoted text causes wrong signature match",
			bodyText: `From: Sarah Chen <sarah@example.com>
Subject: Re: Q3 Roadmap

I agree with John Smith's proposal. Let's proceed with the timeline.

Thanks,
Sarah Chen
Product Manager
Example Corp

On Mon, Feb 10, 2026, John Smith wrote:

Here's my proposal for Q3. Please review and let me know.

Best regards,
John Smith
VP Engineering
Example Corp`,
			people: []ResolvedPerson{
				{Name: "John Smith"},
				{Name: "Sarah Chen"},
			},
			expected: map[string]string{
				"John Smith":  "Best regards,\nJohn Smith\nVP Engineering\nExample Corp",
				"Sarah Chen":  "Thanks,\nSarah Chen\nProduct Manager\nExample Corp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSignaturesPerSender(tt.bodyText, tt.people)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestSLMPipeline_ThreadingRunsForPersonalEmails tests that email threading happens
// even for PERSONAL emails (SkipDeep=true). This test reproduces bug pf-68d631.
// Bug: ThreadGrouper (Stage 2.5) is inside the SkipDeep conditional block, so
// PERSONAL/INTERNAL_COMMS+LOW emails skip threading entirely.
// Expected: Threading should run for ALL emails because it only needs TenantID+SourceID.
func (s *SLMPipelineTestSuite) TestSLMPipeline_ThreadingRunsForPersonalEmails() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    1200,
		ContentID:   "em-test12",
		JobID:       "job-1200",
		ContentType: "email",
		ContentHash: "hash1200",
		BodyText:    "Hey, lunch at noon?",
		Subject:     "Lunch",
		SenderEmail: "friend@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 1200
	})).Return(&ParseEmailOutput{
		CleanBody:  "Hey, lunch at noon?",
		NewContent: "Hey, lunch at noon?",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — PERSONAL (SkipDeep=true)
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 1200
	})).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// CRITICAL ASSERTION: Stage 2.5 ThreadGrouper SHOULD be called even when SkipDeep=true
	// BUG pf-68d631: This expectation will FAIL because threading is inside the SkipDeep block

	// Stage 5: Embed (SkipDeep means stages 2-4.5 are skipped)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 1200
	})).Return(int64(5012), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// Execute workflow
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
	s.Equal("PERSONAL", result.Category)
	s.True(result.SkipDeep)

	// Verify deep processing activities were NOT called (expected behavior)
	s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "BuildContextPackage", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "PersistFindings", mock.Anything, mock.Anything)

	// CRITICAL ASSERTION: Threading SHOULD have been called
	// BUG pf-68d631: ThreadGrouper should run for ALL emails, even personal with SkipDeep=true.
	s.activities.AssertCalled(s.T(), "GroupEmailThread", mock.Anything, mock.MatchedBy(func(in GroupEmailThreadInput) bool {
		return in.TenantID == "tenant-1" && in.SourceID == 1200
	}))
}

// TestSLMPipeline_CleanupRetryPolicy tests that the cleanup handler in the defer block
// has a bounded retry policy to prevent infinite retries.
// This test reproduces bug pf-b0e8e4: cleanup UpdateContentStatus has no RetryPolicy,
// causing unlimited retries when the activity fails, creating zombie workflows.
//
// The defer cleanup (lines 478-507 in pipeline.go) is triggered when the workflow terminates
// abnormally (timeout or cancellation) WITHOUT reaching a normal error-handling path.
// We simulate this by canceling the workflow after triage, then making cleanup fail.
func (s *SLMPipelineTestSuite) TestSLMPipeline_CleanupRetryPolicy() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    1300,
		ContentID:   "em-test13",
		JobID:       "job-1300",
		ContentType: "email",
		ContentHash: "hash1300",
		BodyText:    "Test email for cleanup retry policy.",
		Subject:     "Cleanup Test",
		SenderEmail: "test@example.com",
	}

	// Stage 0: Parse succeeds
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 1300
	})).Return(&ParseEmailOutput{
		CleanBody:  "Test email for cleanup retry policy.",
		NewContent: "Test email for cleanup retry policy.",
	}, nil)

	// UpdateContentStatus: parsed (normal path, not cleanup)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed" && in.SourceID == 1300
	})).Maybe().Return(nil)

	// Stage 1: Triage succeeds
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 1300
	})).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// UpdateContentStatus: triage metadata (normal path, not cleanup)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed" && in.TriageCategory == "PROJECT_UPDATE"
	})).Maybe().Return(nil)

	// Cancel the workflow after triage - this triggers the defer cleanup handler
	s.env.RegisterDelayedCallback(func() {
		s.env.CancelWorkflow()
	}, 0)

	// These activities may or may not be called before cancellation
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 100}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Maybe().Return(&SLMPipelineExtractEntitiesOutput{}, nil)
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Maybe().Return(0, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Maybe().Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Maybe().Return(&DeepAnalyzeOutput{}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Maybe().Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)

	// CRITICAL: Track how many times cleanup UpdateContentStatus is called
	// The defer cleanup handler should have bounded retries (MaximumAttempts: 3)
	cleanupCallCount := 0
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		// The defer cleanup sets status="failed" when workflow doesn't complete normally
		if in.Status == "failed" && in.SourceID == 1300 {
			cleanupCallCount++
			return true
		}
		return false
	})).Return(temporal.NewApplicationError("database connection failed", "DatabaseError"))

	// Execute workflow - it will be canceled, triggering defer cleanup
	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())

	// The workflow should complete even though cleanup failed
	// BUG pf-b0e8e4: Without a retry policy on the cleanup activity options (pipeline.go:488-490),
	// Temporal will retry the cleanup UpdateContentStatus infinitely.

	// With proper retry policy (e.g., MaximumAttempts: 3), cleanup should be called at most 3 times.
	// Without retry policy (current bug), Temporal uses default unlimited retries.
	// In production this creates zombie workflows. In test environment, it will retry many times.

	// This assertion will FAIL with current code because there's no retry bound.
	require.LessOrEqual(s.T(), cleanupCallCount, 3,
		"BUG pf-b0e8e4: Cleanup UpdateContentStatus should have bounded retries (MaximumAttempts: 3). "+
			"Current code at pipeline.go:488-490 creates ActivityOptions with NO RetryPolicy, "+
			"causing Temporal to use default unlimited retries. When cleanup fails (e.g., database down), "+
			"the workflow retries forever, creating zombie workflows. Expected at most 3 cleanup attempts, "+
			"but got %d attempts in test environment (production would retry infinitely).", cleanupCallCount)
}

// TestSLMPipeline_ContentContribution_NONE verifies that NONE contribution skips all deep stages.
// Acceptance criteria: When ContentContribution="NONE", stages 2-4.5 (extract, context, analyze, persist) are skipped.
func (s *SLMPipelineTestSuite) TestSLMPipeline_ContentContribution_NONE() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    200,
		ContentID:   "em-hjn05WWs",
		JobID:       "job-200",
		ContentType: "email",
		BodyText:    "Thanks",
		Subject:     "Re: Meeting",
		SenderEmail: "user@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 200
	})).Return(&ParseEmailOutput{
		CleanBody:  "Thanks",
		NewContent: "Thanks",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage - returns NONE contribution
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 200
	})).Return(&TriageOutput{
		Category:            "PERSONAL",
		Importance:          "LOW",
		Reason:              "Simple acknowledgement",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false, // Not category-based skip, but contribution-based
		ContentContribution: "NONE",
		ContributionReason:  "No actionable knowledge",
	}, nil)

	// Stage 5: Embed - should still run for NONE contribution
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 200
	})).Return(int64(5002), nil)

	// UpdateContentStatus: completed
	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// CRITICAL: ExtractEntitiesActivity, BuildContextPackage, DeepAnalyze, PersistFindings should NOT be called
	s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "BuildContextPackage", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "PersistFindings", mock.Anything, mock.Anything)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)
	// TODO: After implementation, verify total processing time <2s (acceptance criteria)
}

// TestSLMPipeline_ContentContribution_LOW verifies that LOW contribution skips only deep analysis.
// Acceptance criteria: When ContentContribution="LOW", only stage 4 (deep analysis) is skipped.
func (s *SLMPipelineTestSuite) TestSLMPipeline_ContentContribution_LOW() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    300,
		ContentID:   "em-low-contrib",
		JobID:       "job-300",
		ContentType: "email",
		BodyText:    "Company newsletter with generic updates.",
		Subject:     "Monthly Newsletter",
		SenderEmail: "hr@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 300
	})).Return(&ParseEmailOutput{
		CleanBody:  "Company newsletter with generic updates.",
		NewContent: "Company newsletter with generic updates.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage - returns LOW contribution
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 300
	})).Return(&TriageOutput{
		Category:            "INTERNAL_COMMS",
		Importance:          "LOW",
		Reason:              "Generic company newsletter",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "LOW",
		ContributionReason:  "Generic information, not actionable",
	}, nil)

	// Stage 2: Extract - SHOULD run for LOW contribution
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		return in.SourceID == 300
	})).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{},
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "extracted"
	})).Return(nil)

	// Stage 3: Context - SHOULD run for LOW contribution
	s.activities.On("BuildContextPackage", mock.Anything, mock.MatchedBy(func(in BuildContextInput) bool {
		return in.SourceID == 300
	})).Return(&BuildContextOutput{
		EntitiesResolved: 0,
	}, nil)

	// Stage 4.5: Persist - SHOULD run for LOW contribution (persist extraction results)
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 300 && in.Analysis == nil // No deep analysis
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 0,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 300
	})).Return(int64(5003), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// CRITICAL: DeepAnalyze should NOT be called for LOW contribution
	s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)
}

// TestSLMPipeline_ContentContribution_HIGH verifies that HIGH contribution runs all stages.
// Acceptance criteria: When ContentContribution="HIGH", all stages run (no skipping).
func (s *SLMPipelineTestSuite) TestSLMPipeline_ContentContribution_HIGH() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    400,
		ContentID:   "em-high-contrib",
		JobID:       "job-400",
		ContentType: "email",
		BodyText:    "I resign",
		Subject:     "Resignation",
		SenderEmail: "employee@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 400
	})).Return(&ParseEmailOutput{
		CleanBody:  "I resign",
		NewContent: "I resign",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage - returns HIGH contribution despite short body
	// Acceptance criteria: "I resign" test case with HIGH contribution gets full pipeline
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 400
	})).Return(&TriageOutput{
		Category:            "RISK_ISSUE",
		Importance:          "HIGH",
		Reason:              "Employee resignation",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "HIGH",
		ContributionReason:  "Critical HR event requiring action",
	}, nil)

	// Stage 2: Extract - SHOULD run
	personID := int64(55)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		return in.SourceID == 400
	})).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{{Name: "Employee", Role: "employee"}},
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "extracted"
	})).Return(nil)

	// Stage 3: Context - SHOULD run
	s.activities.On("BuildContextPackage", mock.Anything, mock.MatchedBy(func(in BuildContextInput) bool {
		return in.SourceID == 400
	})).Return(&BuildContextOutput{
		ResolvedPeople:   []ResolvedPerson{{Name: "Employee", PersonID: &personID, Confidence: 0.9, Source: "fuzzy_match"}},
		EntitiesResolved: 1,
	}, nil)

	// Stage 4: Deep Analysis - SHOULD run for HIGH contribution
	s.activities.On("DeepAnalyze", mock.Anything, mock.MatchedBy(func(in DeepAnalyzeInput) bool {
		return in.SourceID == 400 && in.TriageCategory == "RISK_ISSUE"
	})).Return(&DeepAnalyzeOutput{
		Summary:   "Employee resignation notification",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: Persist - SHOULD run
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 400 && in.Analysis != nil
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 1,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 400
	})).Return(int64(5004), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)
	// All stages should have executed
}

// TestSLMPipeline_ContentContribution_Default verifies fallback behavior when content_contribution is missing.
// Acceptance criteria: If triage fails to return content_contribution, assume HIGH (never skip).
func (s *SLMPipelineTestSuite) TestSLMPipeline_ContentContribution_DefaultToHigh() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    500,
		ContentID:   "em-no-contrib",
		JobID:       "job-500",
		ContentType: "email",
		BodyText:    "Project update with important details.",
		Subject:     "Project Alpha Update",
		SenderEmail: "pm@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 500
	})).Return(&ParseEmailOutput{
		CleanBody:  "Project update with important details.",
		NewContent: "Project update with important details.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage - returns NO ContentContribution field (backward compatibility test)
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 500
	})).Return(&TriageOutput{
		Category:            "PROJECT_UPDATE",
		Importance:          "MEDIUM",
		Reason:              "Project status update",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "", // Empty/missing - should default to HIGH behavior
	}, nil)

	// All stages SHOULD run (default to HIGH when missing)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		return in.SourceID == 500
	})).Return(&SLMPipelineExtractEntitiesOutput{
		Projects: []string{"Project Alpha"},
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "extracted"
	})).Return(nil)

	s.activities.On("BuildContextPackage", mock.Anything, mock.MatchedBy(func(in BuildContextInput) bool {
		return in.SourceID == 500
	})).Return(&BuildContextOutput{
		EntitiesResolved: 1,
	}, nil)

	s.activities.On("DeepAnalyze", mock.Anything, mock.MatchedBy(func(in DeepAnalyzeInput) bool {
		return in.SourceID == 500
	})).Return(&DeepAnalyzeOutput{
		Summary:   "Project update analysis",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 500
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 1,
	}, nil)

	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 500
	})).Return(int64(5005), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)
	// Verify all deep stages ran (default HIGH behavior)
}

// TestSLMPipeline_ContentContribution_InteractionWithSkipDeep verifies interaction between
// existing SkipDeep (category-based) and new ContentContribution gating.
// When both are present, the more restrictive skip logic should apply.
func (s *SLMPipelineTestSuite) TestSLMPipeline_ContentContribution_InteractionWithSkipDeep() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    600,
		ContentID:   "em-both-skip",
		JobID:       "job-600",
		ContentType: "email",
		BodyText:    "See you at lunch!",
		Subject:     "Lunch",
		SenderEmail: "friend@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 600
	})).Return(&ParseEmailOutput{
		CleanBody:  "See you at lunch!",
		NewContent: "See you at lunch!",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage - PERSONAL category (triggers SkipDeep=true) AND NONE contribution
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 600
	})).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		Reason:     "Personal lunch invitation",
		ModelUsed:  "llama-3.2-1b",
		SkipDeep:   true, // Existing category-based logic
		// ContentContribution: "NONE", // TODO: New contribution-based logic
	}, nil)

	// Stage 5: Embed - should still run
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 600
	})).Return(int64(5006), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	// Deep stages should NOT be called (both skip mechanisms agree)
	// TODO: After implementation, uncomment:
	// s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	// s.activities.AssertNotCalled(s.T(), "DeepAnalyze", mock.Anything, mock.Anything)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	require.Equal(s.T(), "completed", result.Status)
	require.True(s.T(), result.SkipDeep) // Verify skip flag is set
}

// TestSLMPipeline_StageSpanPropagation verifies that each AI-calling stage receives a
// stage-specific SpanID (not the root pipeline SpanID) as its PipelineSpanID.
//
// This is the acceptance test for pf-9c71dd: worker-side stage spans for Langfuse error visibility.
//
// Acceptance criteria verified:
// 1. Each AI stage (triage, extract_entities, deep_analyze, embedding) gets a wrapping stage span.
// 2. The stage span's SpanID is passed to the activity input as PipelineSpanID (not the root).
// 3. Each activity receives a UNIQUE SpanID (stage-specific, not the same root SpanID).
//
// The test fails pre-implementation because currently all activities receive the same root
// pipelineSpanID from StartPipelineTracing — there are no per-stage span wrappers.
func (s *SLMPipelineTestSuite) TestSLMPipeline_StageSpanPropagation() {
	const rootPipelineSpanID = "aabbccddeeff0011" // 16-char hex, simulates what StartPipelineTracing returns

	input := PipelineInput{
		TenantID:    "tenant-span",
		SourceID:    9900,
		ContentID:   "em-spantest",
		JobID:       "job-9900",
		ContentType: "email",
		ContentHash: "hashspan",
		BodyText:    "Project Alpha deadline has moved. This impacts the Q2 roadmap.",
		Subject:     "Deadline Change",
		SenderEmail: "pm@example.com",
	}

	// Capture the PipelineSpanID received by each AI activity.
	var (
		triageSpanID     string
		extractSpanID    string
		deepAnalyzeSpanID string
		embeddingSpanID  string
	)

	// Stage 0: Parse (non-AI, no span capture needed).
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9900
	})).Return(&ParseEmailOutput{
		CleanBody:  "Project Alpha deadline has moved. This impacts the Q2 roadmap.",
		NewContent: "Project Alpha deadline has moved. This impacts the Q2 roadmap.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — capture the SpanID passed to this AI activity.
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		if in.SourceID == 9900 {
			triageSpanID = in.PipelineSpanID
		}
		return in.SourceID == 9900
	})).Return(&TriageOutput{
		Category:   "RISK_ISSUE",
		Importance: "HIGH",
		Reason:     "Deadline slippage",
		ModelUsed:  "llama-3.2-1b",
		SkipDeep:   false,
	}, nil)

	// Stage 2: ExtractEntities — capture the SpanID.
	personID := int64(42)
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.MatchedBy(func(in SLMPipelineExtractEntitiesInput) bool {
		if in.SourceID == 9900 {
			extractSpanID = in.PipelineSpanID
		}
		return in.SourceID == 9900
	})).Return(&SLMPipelineExtractEntitiesOutput{
		People:   []PersonResult{{Name: "PM User", Role: "project_manager"}},
		Projects: []string{"Project Alpha"},
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "extracted"
	})).Return(nil)

	// Stage 3: BuildContextPackage (non-AI coordinator, uses resolved entities).
	s.activities.On("BuildContextPackage", mock.Anything, mock.MatchedBy(func(in BuildContextInput) bool {
		return in.SourceID == 9900
	})).Return(&BuildContextOutput{
		ResolvedPeople:   []ResolvedPerson{{Name: "PM User", PersonID: &personID, Confidence: 0.95, Source: "exact_match"}},
		EntitiesResolved: 1,
	}, nil)

	// Stage 4: DeepAnalyze — capture the SpanID.
	s.activities.On("DeepAnalyze", mock.Anything, mock.MatchedBy(func(in DeepAnalyzeInput) bool {
		if in.SourceID == 9900 {
			deepAnalyzeSpanID = in.PipelineSpanID
		}
		return in.SourceID == 9900
	})).Return(&DeepAnalyzeOutput{
		Summary:   "Deadline slippage for Project Alpha",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: PersistFindings (persistence, not AI caller).
	s.activities.On("PersistFindings", mock.Anything, mock.MatchedBy(func(in PersistFindingsInput) bool {
		return in.SourceID == 9900
	})).Return(&PersistFindingsOutput{
		AssertionsCreated: 1,
	}, nil)

	// Stage 5: GenerateContentEmbedding — capture the SpanID.
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		if in.SourceID == 9900 {
			embeddingSpanID = in.PipelineSpanID
		}
		return in.SourceID == 9900
	})).Return(int64(9001), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// CRITICAL ASSERTIONS: Each AI activity must receive a non-empty SpanID.
	// Pre-implementation: all of these may be empty or equal to rootPipelineSpanID.
	// Post-implementation: each must be a unique, non-empty stage span ID.
	s.NotEmpty(triageSpanID,
		"Triage must receive a non-empty PipelineSpanID (stage span, not root)")
	s.NotEmpty(extractSpanID,
		"ExtractEntities must receive a non-empty PipelineSpanID (stage span, not root)")
	s.NotEmpty(deepAnalyzeSpanID,
		"DeepAnalyze must receive a non-empty PipelineSpanID (stage span, not root)")
	s.NotEmpty(embeddingSpanID,
		"GenerateContentEmbedding must receive a non-empty PipelineSpanID (stage span, not root)")

	// Each stage span ID must be DIFFERENT from the root pipeline span ID.
	// This is the key invariant: activities get the stage span, not the root pipeline span.
	s.NotEqual(rootPipelineSpanID, triageSpanID,
		"Triage PipelineSpanID must be a stage-specific span, not the root pipeline span")
	s.NotEqual(rootPipelineSpanID, extractSpanID,
		"ExtractEntities PipelineSpanID must be a stage-specific span, not the root pipeline span")
	s.NotEqual(rootPipelineSpanID, deepAnalyzeSpanID,
		"DeepAnalyze PipelineSpanID must be a stage-specific span, not the root pipeline span")
	s.NotEqual(rootPipelineSpanID, embeddingSpanID,
		"GenerateContentEmbedding PipelineSpanID must be a stage-specific span, not the root pipeline span")

	// Each stage span ID must be UNIQUE (different from one another).
	// Each stage wraps a different activity so each gets its own span.
	allSpanIDs := []string{triageSpanID, extractSpanID, deepAnalyzeSpanID, embeddingSpanID}
	seen := make(map[string]string) // spanID -> stage name
	stageNames := []string{"triage", "extract_entities", "deep_analyze", "embedding"}
	for i, spanID := range allSpanIDs {
		if prior, exists := seen[spanID]; exists {
			s.Failf("duplicate stage span ID",
				"Stage %q received the same PipelineSpanID %q as stage %q — each stage must get a unique span",
				stageNames[i], spanID, prior)
		}
		seen[spanID] = stageNames[i]
	}
}

func TestSLMPipelineTestSuite(t *testing.T) {
	suite.Run(t, new(SLMPipelineTestSuite))
}
