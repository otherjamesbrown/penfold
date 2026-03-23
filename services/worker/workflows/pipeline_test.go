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

// standardTestPipelineDef returns a pipeline definition matching the standard SLM pipeline stages.
// Used as the default mock return for FetchPipelineDefinition in tests that don't need a specific definition.
func standardTestPipelineDef() *FetchPipelineDefinitionOutput {
	return &FetchPipelineDefinitionOutput{
		Found:       true,
		ContentType: "email",
		Stages: []PipelineStageConfig{
			{Stage: "parse", StageOrder: 0, Enabled: true},
			{Stage: "triage", StageOrder: 1, Enabled: true},
			{Stage: "extract_ner", StageOrder: 2, Enabled: true, SkipWhenLow: true},
			{Stage: "extract_assertions", StageOrder: 3, Enabled: true, SkipWhenLow: true},
			{Stage: "resolve", StageOrder: 4, Enabled: true, SkipWhenLow: true},
			{Stage: "enrich_entities", StageOrder: 5, Enabled: true, SkipWhenLow: true, Optional: true},
			{Stage: "analyze", StageOrder: 6, Enabled: true, SkipWhenLow: true, Optional: true},
			{Stage: "persist", StageOrder: 7, Enabled: true, SkipWhenLow: true},
			{Stage: "embed", StageKind: "embedding", StageOrder: 8, Enabled: true},
		},
	}
}

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

func (m *PipelineMockActivities) GenerateContentSummary(ctx context.Context, input GenerateSummaryInput) (int64, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(int64), args.Error(1)
}

func (m *PipelineMockActivities) DeleteAssertions(ctx context.Context, input DeleteAssertionsInput) (*DeleteAssertionsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DeleteAssertionsOutput), args.Error(1)
}

func (m *PipelineMockActivities) FetchPipelineDefinition(ctx context.Context, input FetchPipelineDefinitionInput) (*FetchPipelineDefinitionOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FetchPipelineDefinitionOutput), args.Error(1)
}

func (m *PipelineMockActivities) StructuredExtract(ctx context.Context, input StructuredExtractInput) (*StructuredExtractOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*StructuredExtractOutput), args.Error(1)
}

func (m *PipelineMockActivities) BuildExtractionContext(ctx context.Context, input BuildExtractionContextInput) (*BuildExtractionContextOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BuildExtractionContextOutput), args.Error(1)
}

func (m *PipelineMockActivities) BuildStageContext(ctx context.Context, input BuildStageContextInput) (string, error) {
	args := m.Called(ctx, input)
	return args.String(0), args.Error(1)
}

func (m *PipelineMockActivities) PersistExtractedData(ctx context.Context, input PersistExtractedDataInput) (*PersistExtractedDataOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*PersistExtractedDataOutput), args.Error(1)
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
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentSummary})
	s.env.RegisterActivityWithOptions(s.activities.DeleteAssertions, activity.RegisterOptions{Name: pkgtemporal.ActivityDeleteAssertions})
	s.env.RegisterActivityWithOptions(s.activities.FetchPipelineDefinition, activity.RegisterOptions{Name: pkgtemporal.ActivityFetchPipelineDefinition})
	s.env.RegisterActivityWithOptions(s.activities.StructuredExtract, activity.RegisterOptions{Name: pkgtemporal.ActivityStructuredExtract})
	s.env.RegisterActivityWithOptions(s.activities.BuildExtractionContext, activity.RegisterOptions{Name: pkgtemporal.ActivityBuildExtractionContext})
	// TODO(pf-6d9704): uncomment when BuildStageContext is fully implemented
	// s.env.RegisterActivityWithOptions(s.activities.BuildStageContext, activity.RegisterOptions{Name: pkgtemporal.ActivityBuildStageContext})
	s.env.RegisterActivityWithOptions(s.activities.PersistExtractedData, activity.RegisterOptions{Name: pkgtemporal.ActivityPersistExtractedData})

	// Default mock expectations for enrichment/threading activities (blocking since pf-67502c fix).
	// Individual tests can override these with more specific expectations.
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	// Stage 1.5 Summarize is non-blocking; default to success so tests don't log ActivityNotRegistered warnings.
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
	// pf-91b00d: DeleteAssertions is best-effort; default to no-op so tests that don't care about it pass.
	s.activities.On("DeleteAssertions", mock.Anything, mock.Anything).Maybe().Return(&DeleteAssertionsOutput{Deleted: 0}, nil)
	// FetchPipelineDefinition: default to standard pipeline definition.
	// Tests that need a specific pipeline definition must override this.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Maybe().Return(standardTestPipelineDef(), nil)
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
//
// Per-stage skip_when_low gating (pf-5f1a20): TotalSteps reflects only the stages that
// actually run. With SkipDeep=true, stages with skip_when_low=true are gated.
// standardTestPipelineDef has parse/triage/embed with skip_when_low=false → 3 steps.
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

// TestSLMPipeline_ContentContribution_LOW verifies that LOW contribution skips all stages
// where skip_when_low=true in the pipeline definition.
// Per-stage gating (pf-5f1a20): standardTestPipelineDef has extract_ner, extract_assertions,
// resolve, analyze, and persist all with skip_when_low=true. LOW contribution gates them all.
// Embed (skip_when_low=false) still runs.
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

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

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

	// Stage 5: Embed still runs (skip_when_low=false on embed stage)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 300
	})).Return(int64(5003), nil)

	// CRITICAL: All stages with skip_when_low=true must NOT be called for LOW contribution.
	// standardTestPipelineDef sets skip_when_low=true on: extract_ner, extract_assertions,
	// resolve, analyze, persist. Per-stage gating (pf-5f1a20) gates each independently.
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

// TestSLMPipeline_TeamsClassification verifies that Teams meeting content is classified
// with content_type=meeting and source_system=teams (not human_email) after the triage
// rule engine runs. pf-e494df + pf-43acf2.
func (s *SLMPipelineTestSuite) TestSLMPipeline_TeamsClassification() {
	// Minimal input - no ContentType set, so FetchSource will be called.
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    1100,
		ContentID:   "mt-teams01",
		JobID:       "job-1100",
		ContentType: "", // Empty triggers FetchSource fallback
		ContentHash: "hashTeams01",
	}

	// FetchSource returns meeting content type with Teams as the original source_system.
	s.activities.On("FetchSource", mock.Anything, mock.MatchedBy(func(in FetchSourceInput) bool {
		return in.SourceID == 1100
	})).Return(&FetchSourceOutput{
		ContentType:  "meeting",
		SourceSystem: "teams", // raw source_system from DB (pf-e494df)
		ContentText:  "User A: Hello everyone. User B: Let's start the standup.",
		Subject:      "Teams Standup Transcript",
	}, nil)

	// ParseTranscript for meeting content.
	s.activities.On("ParseTranscript", mock.Anything, mock.MatchedBy(func(in ParseTranscriptInput) bool {
		return in.SourceID == 1100
	})).Return(&ParseTranscriptOutput{
		CleanText:  "User A: Hello everyone. User B: Let's start the standup.",
		Speakers:   []string{"User A", "User B"},
		DurationMs: 5000,
		Format:     "plain",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Triage: the rule engine won't match Teams content and returns empty SourceSystem.
	// The pipeline should override this with the original "teams" source_system.
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 1100 && in.ContentType == "meeting"
	})).Return(&TriageOutput{
		Category:    "MEETING",
		Importance:  "MEDIUM",
		SkipDeep:    false,
		ModelUsed:   "llama-3.2-1b",
		SourceSystem: "", // rule engine returns empty for non-email content
	}, nil)

	// CreateEnrichmentRecord: verify content_type=meeting and source_system=teams.
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.MatchedBy(func(in CreateEnrichmentRecordInput) bool {
		if in.SourceID != 1100 {
			return false
		}
		s.Equal("meeting", string(in.ContentType), "expected content_type=meeting for Teams content")
		s.Equal("teams", string(in.SourceSystem), "expected source_system=teams (not human_email) for Teams content")
		s.Equal("transcript", string(in.ContentSubtype), "expected content_subtype=transcript for meeting content")
		return true
	})).Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1100}, nil)

	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		People: []PersonResult{{Name: "User A"}, {Name: "User B"}},
	}, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Teams standup discussion", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5100), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

// TestSLMPipeline_DeleteAssertions_CalledOnSkipExtract_pf91b00d verifies that the
// DeleteAssertions activity is invoked when skipExtract=true (contribution=NONE).
// This is Fix 2 for bug pf-91b00d: stale assertions from a prior run that extracted
// assertions (when the item had MEDIUM contribution) are cleaned up when the item is
// reprocessed and reclassified as contribution=NONE.
//
// Per-stage skip_when_low gating (pf-5f1a20): DeleteAssertions is called when extraction
// is skipped (all extract stages gated by skip_when_low=true and contribution=NONE/LOW).
func (s *SLMPipelineTestSuite) TestSLMPipeline_DeleteAssertions_CalledOnSkipExtract_pf91b00d() {
	input := PipelineInput{
		TenantID:    "tenant-fix",
		SourceID:    9100,
		ContentID:   "em-ooo-reprocess",
		JobID:       "job-9100",
		ContentType: "email",
		BodyText:    "I'm out of office until next Monday.",
		Subject:     "Automatic reply: Q4 planning",
		SenderEmail: "ooo@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9100
	})).Return(&ParseEmailOutput{
		CleanBody:  "I'm out of office until next Monday.",
		NewContent: "I'm out of office until next Monday.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Stage 1: Triage returns NONE contribution (auto-reply short-circuit or AI result).
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 9100
	})).Return(&TriageOutput{
		Category:            "INTERNAL_COMMS",
		Importance:          "LOW",
		Reason:              "Auto-reply OOO",
		ModelUsed:           "subject-classifier",
		SkipDeep:            true,
		ContentContribution: "NONE",
		ContributionReason:  "Auto-reply emails do not contribute meaningful content",
	}, nil)

	// pf-91b00d Fix 2: DeleteAssertions MUST be called when skipExtract=true.
	// Use Once() so this expectation takes precedence over the Maybe() default from SetupTest.
	s.activities.On("DeleteAssertions", mock.Anything, mock.MatchedBy(func(in DeleteAssertionsInput) bool {
		return in.TenantID == "tenant-fix" && in.SourceID == 9100
	})).Once().Return(&DeleteAssertionsOutput{Deleted: 3}, nil)

	// Stage 5: Embed still runs (only extraction/analysis are skipped)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 9100
	})).Return(int64(9100), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// Fix 2 assertion: DeleteAssertions must be called when skipExtract=true.
	// AssertCalled verifies the activity was invoked with the correct inputs.
	s.activities.AssertCalled(s.T(), "DeleteAssertions", mock.Anything, mock.MatchedBy(func(in DeleteAssertionsInput) bool {
		return in.TenantID == "tenant-fix" && in.SourceID == 9100
	}))

	// Verify extraction activities were NOT called (contribution=NONE gates them)
	s.activities.AssertNotCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)
	s.activities.AssertNotCalled(s.T(), "ExtractAssertions", mock.Anything, mock.Anything)
}

// TestSLMPipeline_DeleteAssertions_NotCalledOnFullPipeline_pf91b00d verifies that
// DeleteAssertions is NOT called when the full pipeline runs (contribution=HIGH).
// We must not delete assertions when they will be freshly written in the same run.
func (s *SLMPipelineTestSuite) TestSLMPipeline_DeleteAssertions_NotCalledOnFullPipeline_pf91b00d() {
	input := PipelineInput{
		TenantID:    "tenant-fix",
		SourceID:    9200,
		ContentID:   "em-high-contrib",
		JobID:       "job-9200",
		ContentType: "email",
		BodyText:    "Project Alpha is at risk due to delayed vendor delivery.",
		Subject:     "Risk Update",
		SenderEmail: "pm@example.com",
	}

	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9200
	})).Return(&ParseEmailOutput{
		CleanBody:  "Project Alpha is at risk due to delayed vendor delivery.",
		NewContent: "Project Alpha is at risk due to delayed vendor delivery.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Maybe().Return(nil)

	// Triage returns HIGH contribution — full pipeline runs
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 9200
	})).Return(&TriageOutput{
		Category:            "RISK_ISSUE",
		Importance:          "HIGH",
		ModelUsed:           "llama-3.2-1b",
		SkipDeep:            false,
		ContentContribution: "HIGH",
	}, nil)

	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		Risks: []string{"Delayed vendor delivery"},
	}, nil)
	// ExtractAssertions returns 0 (best-effort, non-blocking)
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(0, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Risk escalation", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(9200), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// DeleteAssertions must NOT be called when running the full pipeline.
	// Assertions are freshly written by ExtractAssertions in the same run.
	s.activities.AssertNotCalled(s.T(), "DeleteAssertions", mock.Anything, mock.MatchedBy(func(in DeleteAssertionsInput) bool {
		return in.SourceID == 9200
	}))
}

// TestSLMPipeline_ReprocessPrefersTriageRouting verifies that when reprocessing,
// fresh triage routing always wins over the stale pipeline carried in input.Pipeline (pf-cb63c1).
// Scenario: input.Pipeline="standard" (stale from previous run), triage returns Pipelines=["newsletter"].
// The post-triage FetchPipelineDefinition must be called with pipeline="newsletter", NOT "standard".
func (s *SLMPipelineTestSuite) TestSLMPipeline_ReprocessPrefersTriageRouting() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    9301,
		ContentID:   "em-reprocess01",
		JobID:       "job-9301",
		ContentType: "email",
		BodyText:    "Weekly newsletter content with lots of interesting articles and links.",
		Subject:     "Weekly Digest",
		SenderEmail: "newsletter@example.com",
		Pipeline:    "standard", // Stale pipeline from previous triage run
	}

	// Early fetch: called with "standard" because input.Pipeline="standard"
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "standard"
	})).Return(&FetchPipelineDefinitionOutput{Found: false}, nil)

	// ParseEmail
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9301
	})).Return(&ParseEmailOutput{
		CleanBody:  "Weekly newsletter content with lots of interesting articles and links.",
		NewContent: "Weekly newsletter content with lots of interesting articles and links.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Triage returns fresh routing to "newsletter" pipeline
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 9301
	})).Return(&TriageOutput{
		Category:           "NEWSLETTER",
		Importance:         "LOW",
		SkipDeep:           true,
		ModelUsed:          "llama-3.2-1b",
		RoutingContentType: "EMAIL",
		RoutingSubtype:     "NEWSLETTER",
		Pipelines:          []string{"newsletter"},
	}, nil)

	// Post-triage FetchPipelineDefinition MUST be called with "newsletter" (not "standard").
	// This is the key assertion: fresh triage routing wins.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "newsletter"
	})).Return(standardTestPipelineDef(), nil)

	// Downstream stages: SkipDeep=true so extract/analyze/persist are skipped; only embed runs.
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(9301), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// Verify FetchPipelineDefinition was called with "newsletter" (post-triage fetch).
	s.activities.AssertCalled(s.T(), "FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "newsletter"
	}))
}

// TestSLMPipeline_ReprocessFallsBackToInputPipeline verifies that when triage returns
// no pipeline routing, input.Pipeline is used as fallback (pf-cb63c1).
// Scenario: input.Pipeline="standard", triage returns empty Pipelines=[].
// The post-triage FetchPipelineDefinition must be called with pipeline="standard" (fallback).
func (s *SLMPipelineTestSuite) TestSLMPipeline_ReprocessFallsBackToInputPipeline() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    9302,
		ContentID:   "em-reprocess02",
		JobID:       "job-9302",
		ContentType: "email",
		BodyText:    "A regular email about project updates and next steps.",
		Subject:     "Project Update",
		SenderEmail: "colleague@example.com",
		Pipeline:    "standard", // input.Pipeline should be fallback when triage has no routing
	}

	// Early fetch: called with "standard" because input.Pipeline="standard"
	earlyFetchDef := standardTestPipelineDef()
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.MatchedBy(func(in FetchPipelineDefinitionInput) bool {
		return in.Pipeline == "standard"
	})).Return(earlyFetchDef, nil)

	// ParseEmail
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9302
	})).Return(&ParseEmailOutput{
		CleanBody:  "A regular email about project updates and next steps.",
		NewContent: "A regular email about project updates and next steps.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Triage returns empty Pipelines (no routing result)
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 9302
	})).Return(&TriageOutput{
		Category:   "INTERNAL_COMMS",
		Importance: "MEDIUM",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
		Pipelines:  []string{}, // Empty — no routing
	}, nil)

	// Post-triage: pipelineName falls back to input.Pipeline="standard".
	// earlyPipelineName="standard" and earlyPipelineDef is non-nil (Found=true above),
	// so the post-triage branch reuses earlyPipelineDef without a second fetch.

	// Downstream stages
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(0, nil)
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Project update summary", ModelUsed: "gemini-2.0-flash",
	}, nil)
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(9302), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)
}

// TestSLMPipeline_NewsletterExtract_UsesEnrichedContext verifies that when
// BuildStageContext succeeds, StructuredExtract receives the enriched
// BackgroundContext rather than the generic extraction context.
func (s *SLMPipelineTestSuite) TestSLMPipeline_NewsletterExtract_UsesEnrichedContext() {
	const newsletterContext = "### User Context\nAlice, PM"

	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    9500,
		ContentID:   "em-nl-test01",
		JobID:       "job-9500",
		ContentType: "email",
		ContentHash: "hash9500",
		BodyText:    "Weekly digest with industry updates and project news.",
		Subject:     "Weekly Digest",
		SenderEmail: "digest@newsletter.example.com",
		Pipeline:    "newsletter",
	}

	// Override the default Maybe FetchPipelineDefinition mock to return the newsletter pipeline.
	// Unset the default first so our specific return value takes priority.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Unset()
	newsletterPipelineDef := &FetchPipelineDefinitionOutput{
		Found:       true,
		ContentType: "email",
		Stages: []PipelineStageConfig{
			{
				Stage:      "newsletter_extract",
				StageKind:  "structured_extract",
				PersistKey: "newsletter",
				StageOrder: 1,
				Enabled:    true,
			},
		},
	}
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(newsletterPipelineDef, nil)

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9500
	})).Return(&ParseEmailOutput{
		CleanBody:  "Weekly digest with industry updates and project news.",
		NewContent: "Weekly digest with industry updates and project news.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage — routes to newsletter pipeline.
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 9500
	})).Return(&TriageOutput{
		Category:           "NEWSLETTER",
		Importance:         "HIGH",
		SkipDeep:           false,
		ModelUsed:          "llama-3.2-1b",
		RoutingContentType: "EMAIL",
		RoutingSubtype:     "NEWSLETTER",
		Pipelines:          []string{"newsletter"},
	}, nil)

	// BuildStageContext succeeds and returns enriched newsletter context.
	s.activities.On("BuildStageContext", mock.Anything, mock.MatchedBy(func(in BuildStageContextInput) bool {
		return in.TenantID == "tenant-1" && in.Pipeline == "newsletter" && in.Stage == "newsletter_extract"
	})).Return(newsletterContext, nil)

	// StructuredExtract: assert BackgroundContext is the enriched newsletter context.
	var capturedBackgroundContext string
	s.activities.On("StructuredExtract", mock.Anything, mock.MatchedBy(func(in StructuredExtractInput) bool {
		capturedBackgroundContext = in.BackgroundContext
		return in.SourceID == 9500 && in.StageName == "newsletter_extract"
	})).Return(&StructuredExtractOutput{
		ModelUsed: "gemini-2.0-flash",
		StageName: "newsletter_extract",
	}, nil)

	// PersistExtractedData: non-blocking persist of extracted JSON.
	s.activities.On("PersistExtractedData", mock.Anything, mock.MatchedBy(func(in PersistExtractedDataInput) bool {
		return in.SourceID == 9500 && in.Key == "newsletter"
	})).Return(&PersistExtractedDataOutput{Updated: true}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 9500
	})).Return(int64(9500), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// KEY ASSERTION: StructuredExtract must have received the enriched newsletter context.
	s.Equal(newsletterContext, capturedBackgroundContext,
		"StructuredExtract should receive the enriched newsletter context from BuildStageContext, not the generic extraction context")
}

// TestSLMPipeline_NewsletterExtract_FallbackOnContextError verifies that when
// BuildStageContext returns an error, StructuredExtract falls back to the
// generic extraction context and the pipeline completes successfully.
func (s *SLMPipelineTestSuite) TestSLMPipeline_NewsletterExtract_FallbackOnContextError() {
	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    9501,
		ContentID:   "em-nl-test02",
		JobID:       "job-9501",
		ContentType: "email",
		ContentHash: "hash9501",
		BodyText:    "Weekly digest with project updates and articles.",
		Subject:     "Weekly Digest",
		SenderEmail: "digest@newsletter.example.com",
		Pipeline:    "newsletter",
	}

	// Override the default Maybe FetchPipelineDefinition mock to return the newsletter pipeline.
	// Unset the default first so our specific return value takes priority.
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Unset()
	newsletterPipelineDef := &FetchPipelineDefinitionOutput{
		Found:       true,
		ContentType: "email",
		Stages: []PipelineStageConfig{
			{
				Stage:      "newsletter_extract",
				StageKind:  "structured_extract",
				PersistKey: "newsletter",
				StageOrder: 1,
				Enabled:    true,
			},
		},
	}
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(newsletterPipelineDef, nil)

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.MatchedBy(func(in ParseEmailInput) bool {
		return in.SourceID == 9501
	})).Return(&ParseEmailOutput{
		CleanBody:  "Weekly digest with project updates and articles.",
		NewContent: "Weekly digest with project updates and articles.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage — routes to newsletter pipeline.
	s.activities.On("Triage", mock.Anything, mock.MatchedBy(func(in TriageInput) bool {
		return in.SourceID == 9501
	})).Return(&TriageOutput{
		Category:           "NEWSLETTER",
		Importance:         "HIGH",
		SkipDeep:           false,
		ModelUsed:          "llama-3.2-1b",
		RoutingContentType: "EMAIL",
		RoutingSubtype:     "NEWSLETTER",
		Pipelines:          []string{"newsletter"},
	}, nil)

	// BuildExtractionContext succeeds and returns a known generic context.
	const genericContext = "### Glossary\n- ACME: Acme Corp\n### Topics\n- Q1 planning"
	s.activities.On("BuildExtractionContext", mock.Anything, mock.MatchedBy(func(in BuildExtractionContextInput) bool {
		return in.TenantID == "tenant-1"
	})).Return(&BuildExtractionContextOutput{
		BackgroundContext: genericContext,
		GlossaryCount:    1,
		TopicCount:        1,
	}, nil)

	// BuildStageContext fails — pipeline should fall back to generic context.
	s.activities.On("BuildStageContext", mock.Anything, mock.MatchedBy(func(in BuildStageContextInput) bool {
		return in.TenantID == "tenant-1" && in.Pipeline == "newsletter" && in.Stage == "newsletter_extract"
	})).Return("", temporal.NewApplicationError("context service unavailable", "ServiceUnavailable"))

	// StructuredExtract: assert BackgroundContext is the generic extraction context
	// (the value returned by BuildExtractionContext, NOT the newsletter-specific context).
	var capturedBackgroundContext string
	s.activities.On("StructuredExtract", mock.Anything, mock.MatchedBy(func(in StructuredExtractInput) bool {
		capturedBackgroundContext = in.BackgroundContext
		return in.SourceID == 9501 && in.StageName == "newsletter_extract"
	})).Return(&StructuredExtractOutput{
		ModelUsed: "gemini-2.0-flash",
		StageName: "newsletter_extract",
	}, nil)

	// PersistExtractedData: non-blocking persist of extracted JSON.
	s.activities.On("PersistExtractedData", mock.Anything, mock.MatchedBy(func(in PersistExtractedDataInput) bool {
		return in.SourceID == 9501 && in.Key == "newsletter"
	})).Return(&PersistExtractedDataOutput{Updated: true}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(in GenerateEmbeddingInput) bool {
		return in.SourceID == 9501
	})).Return(int64(9501), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	var result PipelineResult
	require.NoError(s.T(), s.env.GetWorkflowResult(&result))
	s.Equal("completed", result.Status)

	// KEY ASSERTION: StructuredExtract must have received the generic extraction context,
	// NOT the newsletter-specific context. After BuildStageContext failure, bgContext
	// falls back to extractionContext from BuildExtractionContext.
	s.Equal(genericContext, capturedBackgroundContext,
		"StructuredExtract should receive the generic extraction context when BuildStageContext fails")
}

func TestSLMPipelineTestSuite(t *testing.T) {
	suite.Run(t, new(SLMPipelineTestSuite))
}
