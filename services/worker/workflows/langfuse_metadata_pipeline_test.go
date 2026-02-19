// Package workflows provides acceptance tests for pf-37ebe8:
// Remove all ReportLangfuseGeneration calls from the pipeline workflow,
// and pass traceID + phaseID via gRPC metadata on activity inputs instead.
//
// These tests are INTENTIONALLY FAILING until the feature is implemented.
package workflows

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// LangfuseMetadataPipelineTestSuite tests that the pipeline workflow no longer
// calls ReportLangfuseGeneration after pf-37ebe8 is implemented.
type LangfuseMetadataPipelineTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities

	// reportLangfuseGenerationCallCount tracks how many times the
	// ActivityReportLangfuseGeneration activity is scheduled by the workflow.
	// After pf-37ebe8: this must be 0.
	reportLangfuseGenerationCallCount atomic.Int32
}

func (s *LangfuseMetadataPipelineTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &PipelineMockActivities{}
	s.reportLangfuseGenerationCallCount.Store(0)

	// Register standard pipeline activities (same as SLMPipelineTestSuite).
	s.env.RegisterActivityWithOptions(s.activities.ParseEmail, activity.RegisterOptions{Name: pkgtemporal.ActivityParseEmail})
	s.env.RegisterActivityWithOptions(s.activities.ParseTranscript, activity.RegisterOptions{Name: pkgtemporal.ActivityParseTranscript})
	s.env.RegisterActivityWithOptions(s.activities.Triage, activity.RegisterOptions{Name: pkgtemporal.ActivityTriage})
	s.env.RegisterActivityWithOptions(s.activities.ExtractEntitiesActivity, activity.RegisterOptions{Name: pkgtemporal.ActivityExtractEntitiesActivity})
	s.env.RegisterActivityWithOptions(s.activities.ExtractAssertions, activity.RegisterOptions{Name: pkgtemporal.ActivityExtractAssertions})
	s.env.RegisterActivityWithOptions(s.activities.BuildContextPackage, activity.RegisterOptions{Name: pkgtemporal.ActivityBuildContextPackage})
	s.env.RegisterActivityWithOptions(s.activities.DeepAnalyze, activity.RegisterOptions{Name: pkgtemporal.ActivityDeepAnalyze})
	s.env.RegisterActivityWithOptions(s.activities.PersistFindings, activity.RegisterOptions{Name: pkgtemporal.ActivityPersistFindings})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentEmbedding})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{Name: pkgtemporal.ActivityUpdateContentStatus})
	s.env.RegisterActivityWithOptions(s.activities.DeleteEmbedding, activity.RegisterOptions{Name: pkgtemporal.ActivityDeleteEmbedding})
	s.env.RegisterActivityWithOptions(s.activities.RecordOverrides, activity.RegisterOptions{Name: pkgtemporal.ActivityRecordOverrides})
	s.env.RegisterActivityWithOptions(s.activities.FetchSource, activity.RegisterOptions{Name: pkgtemporal.ActivityFetchContent})
	s.env.RegisterActivityWithOptions(s.activities.GroupEmailThread, activity.RegisterOptions{Name: pkgtemporal.ActivityGroupEmailThread})
	s.env.RegisterActivityWithOptions(s.activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: pkgtemporal.ActivityCreateEnrichmentRecord})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{Name: pkgtemporal.ActivityGenerateContentSummary})

	// Register Langfuse activities that are STILL VALID post-feature:
	// CreateLangfuseTrace, ReportLangfusePhase, FinishLangfuseTrace.
	// These should still be called by the workflow.
	noopLangfuseTrace := func(ctx context.Context, input CreateLangfuseTraceInput) (*CreateLangfuseTraceOutput, error) {
		return &CreateLangfuseTraceOutput{TraceID: input.TraceID}, nil
	}
	noopLangfusePhase := func(ctx context.Context, input ReportLangfusePhaseInput) error {
		return nil
	}
	noopFinishTrace := func(ctx context.Context, input FinishLangfuseTraceInput) error {
		return nil
	}
	s.env.RegisterActivityWithOptions(noopLangfuseTrace, activity.RegisterOptions{Name: pkgtemporal.ActivityCreateLangfuseTrace})
	s.env.RegisterActivityWithOptions(noopLangfusePhase, activity.RegisterOptions{Name: pkgtemporal.ActivityReportLangfusePhase})
	s.env.RegisterActivityWithOptions(noopFinishTrace, activity.RegisterOptions{Name: pkgtemporal.ActivityFinishLangfuseTrace})

	// Register ActivityReportLangfuseGeneration as a call-counting activity.
	// This MUST NOT be called after pf-37ebe8 is implemented.
	counter := &s.reportLangfuseGenerationCallCount
	reportGeneration := func(ctx context.Context, input ReportLangfuseGenerationInput) error {
		counter.Add(1)
		return nil
	}
	s.env.RegisterActivityWithOptions(reportGeneration, activity.RegisterOptions{Name: pkgtemporal.ActivityReportLangfuseGeneration})

	// Default mock expectations for enrichment/threading activities.
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	// Stage 1.5 Summarize is non-blocking; default to success.
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
}

func (s *LangfuseMetadataPipelineTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestPipeline_NoReportLangfuseGeneration_FullPipeline verifies that the
// SLMPipelineWorkflow does NOT schedule any ActivityReportLangfuseGeneration
// activities during a complete full-pipeline run.
//
// CURRENTLY FAILING: The pipeline schedules ActivityReportLangfuseGeneration
// 6 times (triage, summarize, extract_entities, extract_assertions,
// deep_analyze, embedding). After pf-37ebe8 is implemented, all 6 calls
// must be removed and this test must pass with count == 0.
func (s *LangfuseMetadataPipelineTestSuite) TestPipeline_NoReportLangfuseGeneration_FullPipeline() {
	input := PipelineInput{
		TenantID:    "tenant-langfuse-meta",
		SourceID:    9001,
		ContentID:   "em-lf0001",
		JobID:       "job-lf-001",
		ContentType: "email",
		ContentHash: "hashLF001",
		BodyText:    "Project Gamma update: all milestones on track.",
		Subject:     "Project Status",
		SenderEmail: "pm@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody:  "Project Gamma update: all milestones on track.",
		NewContent: "Project Gamma update: all milestones on track.",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		Reason:     "Status update",
		ModelUsed:  "llama-3.2-1b",
		SkipDeep:   false,
	}, nil)

	// Stage 2: Extract
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{
		Projects: []string{"Project Gamma"},
	}, nil)

	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(0, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "extracted"
	})).Return(nil)

	// Stage 3: Context
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{
		EntitiesResolved: 0,
		TokensUsed:       100,
		TokenBudget:      2000,
	}, nil)

	// Stage 4: Analyze
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary:   "Project Gamma on track.",
		ModelUsed: "gemini-2.0-flash",
	}, nil)

	// Stage 4.5: Persist
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{
		AssertionsCreated: 0,
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(9001), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	// ACCEPTANCE CRITERION: ActivityReportLangfuseGeneration must NEVER be called.
	// Currently this will be 6 (one per AI stage). After pf-37ebe8 it must be 0.
	actualCallCount := s.reportLangfuseGenerationCallCount.Load()
	require.Equal(s.T(), int32(0), actualCallCount,
		"pf-37ebe8: ActivityReportLangfuseGeneration must not be called by the pipeline workflow. "+
			"Got %d calls — remove all ReportLangfuseGenerationInput usages from pipeline.go.",
		actualCallCount)
}

// TestPipeline_NoReportLangfuseGeneration_SkipDeep verifies that even on a
// skip-deep (PERSONAL) path, no ActivityReportLangfuseGeneration is scheduled.
//
// CURRENTLY FAILING: The pipeline schedules ActivityReportLangfuseGeneration
// at triage and embedding stages even on the short path.
func (s *LangfuseMetadataPipelineTestSuite) TestPipeline_NoReportLangfuseGeneration_SkipDeep() {
	input := PipelineInput{
		TenantID:    "tenant-langfuse-meta",
		SourceID:    9002,
		ContentID:   "em-lf0002",
		JobID:       "job-lf-002",
		ContentType: "email",
		ContentHash: "hashLF002",
		BodyText:    "Hey, catch you later!",
		Subject:     "Quick chat",
		SenderEmail: "friend@example.com",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Hey, catch you later!",
	}, nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "parsed"
	})).Return(nil)

	// Stage 1: Triage — PERSONAL (triggers skip_deep path)
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 5: Embed only (stages 2-4.5 skipped)
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(9002), nil)

	s.activities.On("UpdateContentStatus", mock.Anything, mock.MatchedBy(func(in UpdateContentStatusInput) bool {
		return in.Status == "completed"
	})).Return(nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	actualCallCount := s.reportLangfuseGenerationCallCount.Load()
	require.Equal(s.T(), int32(0), actualCallCount,
		"pf-37ebe8: ActivityReportLangfuseGeneration must not be called even on skip-deep path. "+
			"Got %d calls.", actualCallCount)
}

// TestReportLangfuseGenerationConstant_Removed documents that the
// ActivityReportLangfuseGeneration constant must be removed from
// pkg/temporal/activities.go after pf-37ebe8.
//
// This test uses a compile-time approach: the constant still exists in the
// codebase right now. Once the feature is implemented, the constant is removed,
// causing this test file to fail to compile — which is caught by the activity
// tests in services/worker/activities/langfuse_metadata_test.go (the
// LangfuseTraceID field references there are the primary compile gate).
//
// Runtime aspect: This test verifies the constant is NOT present in the list
// of activities that should remain after pf-37ebe8.
func TestReportLangfuseGenerationConstant_Removed(t *testing.T) {
	// Post-feature: ActivityReportLangfuseGeneration should not appear in
	// AllMainQueueActivities. This asserts the intended final state.
	mainActivities := pkgtemporal.AllMainQueueActivities()
	for _, name := range mainActivities {
		require.NotEqual(t, pkgtemporal.ActivityReportLangfuseGeneration, name,
			"pf-37ebe8: ActivityReportLangfuseGeneration must be removed from AllMainQueueActivities "+
				"after feature implementation. Found it still registered: %q", name)
	}
}

// TestLangfuseMetadataPipelineSuite runs the LangfuseMetadataPipelineTestSuite.
func TestLangfuseMetadataPipelineSuite(t *testing.T) {
	suite.Run(t, new(LangfuseMetadataPipelineTestSuite))
}
