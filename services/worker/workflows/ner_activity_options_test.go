package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/testsuite"
)

// NERActivityOptionsTestSuite tests that extract_ner stage uses LLM activity options.
// This is a regression test for pf-d8b465 where extract_ner used embedding options
// (10s heartbeat, 30s start-to-close) instead of LLM options, causing timeouts
// on meeting transcripts (50K+ chars requiring chunked LLM extraction).
type NERActivityOptionsTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *NERActivityOptionsTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.activities = &PipelineMockActivities{}

	// Register all pipeline activities
	s.env.RegisterActivityWithOptions(s.activities.ParseEmail, activity.RegisterOptions{Name: "ParseEmail"})
	s.env.RegisterActivityWithOptions(s.activities.Triage, activity.RegisterOptions{Name: "Triage"})
	s.env.RegisterActivityWithOptions(s.activities.ExtractEntitiesActivity, activity.RegisterOptions{Name: "ExtractEntitiesActivity"})
	s.env.RegisterActivityWithOptions(s.activities.ExtractAssertions, activity.RegisterOptions{Name: "ExtractAssertions"})
	s.env.RegisterActivityWithOptions(s.activities.BuildContextPackage, activity.RegisterOptions{Name: "BuildContextPackage"})
	s.env.RegisterActivityWithOptions(s.activities.DeepAnalyze, activity.RegisterOptions{Name: "DeepAnalyze"})
	s.env.RegisterActivityWithOptions(s.activities.PersistFindings, activity.RegisterOptions{Name: "PersistFindings"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{Name: "GenerateContentEmbedding"})
	s.env.RegisterActivityWithOptions(s.activities.UpdateContentStatus, activity.RegisterOptions{Name: "UpdateContentStatus"})
	s.env.RegisterActivityWithOptions(s.activities.RecordOverrides, activity.RegisterOptions{Name: "RecordOverrides"})
	s.env.RegisterActivityWithOptions(s.activities.GroupEmailThread, activity.RegisterOptions{Name: "GroupEmailThread"})
	s.env.RegisterActivityWithOptions(s.activities.CreateEnrichmentRecord, activity.RegisterOptions{Name: "CreateEnrichmentRecord"})
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{Name: "GenerateContentSummary"})

	// Default mock expectations for enrichment/threading activities
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
}

func (s *NERActivityOptionsTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestExtractNERUsesLLMActivityOptions verifies that extract_ner stage uses LLM activity options.
//
// Requirements (from pf-d8b465):
// - ExtractEntitiesActivity must use LLM activity options, not embedding options
// - StartToCloseTimeout must be >= 5 minutes (LLM default is 10min)
// - HeartbeatTimeout must be >= 1 minute (LLM default is 5min)
//
// Before fix: extract_ner used embeddingOpts (30s start-to-close, 10s heartbeat)
// which caused timeouts on meeting transcripts (50K+ chars, chunked LLM extraction).
func (s *NERActivityOptionsTestSuite) TestExtractNERUsesLLMActivityOptions() {
	var capturedStartToClose time.Duration
	var capturedHeartbeat time.Duration

	s.env.SetOnActivityStartedListener(func(activityInfo *activity.Info, ctx context.Context, args converter.EncodedValues) {
		if activityInfo.ActivityType.Name == "ExtractEntitiesActivity" {
			capturedStartToClose = activityInfo.StartToCloseTimeout
			capturedHeartbeat = activityInfo.HeartbeatTimeout
		}
	})

	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    300,
		ContentID:   "em-test-ner-opts",
		JobID:       "job-300",
		ContentType: "email",
		BodyText:    "Test extract_ner activity options",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test extract_ner activity options",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage — SkipDeep=false so pipeline continues to stage 2
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "RISK_ISSUE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 2: ExtractEntitiesActivity — this is the activity under test
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)

	// Stage 2b: ExtractAssertions
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(0, nil)

	// Stage 3: BuildContextPackage
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)

	// Stage 4: DeepAnalyze
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{
		Summary: "Test summary",
	}, nil)

	// Stage 4.5: PersistFindings
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(7001), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	// Verify ExtractEntitiesActivity was called
	s.activities.AssertCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)

	// CRITICAL ASSERTIONS: Verify extract_ner uses LLM activity options
	require.NotZero(s.T(), capturedStartToClose, "should have captured StartToCloseTimeout for ExtractEntitiesActivity")

	// LLM options have StartToCloseTimeout >= 5 minutes (actual is 10min).
	// With the bug, embeddingOpts gives 30s — this assertion FAILS before the fix.
	require.GreaterOrEqual(s.T(), capturedStartToClose, 5*time.Minute,
		"extract_ner should use LLM activity options with StartToCloseTimeout >= 5min, got %v (bug: embeddingOpts gives 30s)", capturedStartToClose)

	// LLM options have HeartbeatTimeout >= 1 minute (LLM default is 5min).
	// With the bug, embeddingOpts gives 10s — this assertion FAILS before the fix.
	require.GreaterOrEqual(s.T(), capturedHeartbeat, 1*time.Minute,
		"extract_ner should use LLM heartbeat timeout (5min), not embedding (10s), got %v", capturedHeartbeat)
}

// TestExtractNER_StageConfigMapOverridesInputTimeouts verifies that pipeline-specific
// stageConfigMap timeouts from FetchPipelineDefinition are applied to activity options.
//
// Regression test for pf-3b5d5d / pf-7dc6d5: stageConfigMap (from FetchPipelineDefinition)
// is the sole source of per-pipeline timeouts. The transcript pipeline has
// timeout_seconds=600 for NER, overriding the default LLM opts.
func (s *NERActivityOptionsTestSuite) TestExtractNER_StageConfigMapOverridesInputTimeouts() {
	var capturedStartToClose time.Duration
	var capturedHeartbeat time.Duration

	s.env.SetOnActivityStartedListener(func(activityInfo *activity.Info, ctx context.Context, args converter.EncodedValues) {
		if activityInfo.ActivityType.Name == "ExtractEntitiesActivity" {
			capturedStartToClose = activityInfo.StartToCloseTimeout
			capturedHeartbeat = activityInfo.HeartbeatTimeout
		}
	})

	// Register FetchPipelineDefinition mock — returns transcript pipeline with 600s NER timeout
	s.env.RegisterActivityWithOptions(s.activities.FetchPipelineDefinition, activity.RegisterOptions{Name: "FetchPipelineDefinition"})
	s.activities.On("FetchPipelineDefinition", mock.Anything, mock.Anything).Return(&FetchPipelineDefinitionOutput{
		Found: true,
		Stages: []PipelineStageConfig{
			{Stage: "triage", StageOrder: 0, Enabled: true, TimeoutSeconds: 120},
			{Stage: "extract_ner", StageOrder: 1, Enabled: true, SkipWhenLow: false, TimeoutSeconds: 600},
			{Stage: "extract_semantic", StageOrder: 2, Enabled: true, SkipWhenLow: false, TimeoutSeconds: 600},
			{Stage: "analyze", StageOrder: 3, Enabled: true, TimeoutSeconds: 600},
			{Stage: "embed", StageOrder: 4, Enabled: true, TimeoutSeconds: 120},
		},
	}, nil)

	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    301,
		ContentID:   "mt-test-timeout-priority",
		JobID:       "job-301",
		ContentType: "email",
		BodyText:    "Test stageConfigMap applies pipeline-specific timeouts",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test stageConfigMap applies pipeline-specific timeouts",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 2: ExtractEntitiesActivity — activity under test
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)

	// Stage 2b: ExtractAssertions
	s.activities.On("ExtractAssertions", mock.Anything, mock.Anything).Return(0, nil)

	// Stage 3: BuildContextPackage
	s.activities.On("BuildContextPackage", mock.Anything, mock.Anything).Return(&BuildContextOutput{}, nil)

	// Stage 4: DeepAnalyze
	s.activities.On("DeepAnalyze", mock.Anything, mock.Anything).Return(&DeepAnalyzeOutput{Summary: "Test"}, nil)

	// Stage 4.5: PersistFindings
	s.activities.On("PersistFindings", mock.Anything, mock.Anything).Return(&PersistFindingsOutput{}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(7002), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	s.activities.AssertCalled(s.T(), "ExtractEntitiesActivity", mock.Anything, mock.Anything)

	// CRITICAL: stageConfigMap (600s from transcript pipeline via FetchPipelineDefinition)
	// must be applied as the pipeline-specific timeout.
	require.NotZero(s.T(), capturedStartToClose)
	require.Equal(s.T(), 600*time.Second, capturedStartToClose,
		"stageConfigMap timeout (600s) must be applied from pipeline_definitions; got %v", capturedStartToClose)
	require.Equal(s.T(), 300*time.Second, capturedHeartbeat,
		"stageConfigMap heartbeat (300s = timeout/2) must be applied; got %v", capturedHeartbeat)
}

func TestNERActivityOptionsTestSuite(t *testing.T) {
	suite.Run(t, new(NERActivityOptionsTestSuite))
}
