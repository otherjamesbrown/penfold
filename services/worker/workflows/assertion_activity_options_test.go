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

// AssertionActivityOptionsTestSuite tests that extract_assertions stage uses LLM activity options.
// This is a reproduction test for pf-dc3add where extract_assertions used embedding options
// (10s heartbeat, 30s start-to-close) instead of LLM options, causing premature timeouts
// because the activity calls an LLM and needs the longer LLM-level timeouts.
type AssertionActivityOptionsTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *AssertionActivityOptionsTestSuite) SetupTest() {
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

func (s *AssertionActivityOptionsTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestExtractAssertionsUsesLLMActivityOptions verifies that extract_assertions stage uses LLM activity options.
//
// Requirements (from pf-dc3add):
// - ExtractAssertions stage must use LLM activity options, not embedding options
// - StartToCloseTimeout must be >= 5 minutes (LLM default is 10min)
// - HeartbeatTimeout must be >= 1 minute (LLM default is 5min)
//
// This test WILL FAIL before the fix because extract_assertions currently uses embeddingOpts
// (line 1260 in pipeline.go: assertionOpts := stageOpts("extract_assertions", embeddingOpts))
// causing StartToCloseTimeout=30s and HeartbeatTimeout=10s.
//
// Expected behavior after fix:
// - Line 1260 changes from embeddingOpts to llmOpts
// - ExtractAssertions will have StartToCloseTimeout=10min, HeartbeatTimeout=5min
func (s *AssertionActivityOptionsTestSuite) TestExtractAssertionsUsesLLMActivityOptions() {
	// Set up listener to capture activity options when ExtractAssertions is called
	var capturedStartToClose time.Duration
	var capturedHeartbeat time.Duration

	s.env.SetOnActivityStartedListener(func(activityInfo *activity.Info, ctx context.Context, args converter.EncodedValues) {
		if activityInfo.ActivityType.Name == "ExtractAssertions" {
			capturedStartToClose = activityInfo.StartToCloseTimeout
			capturedHeartbeat = activityInfo.HeartbeatTimeout
		}
	})

	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    200,
		ContentID:   "em-test-assertion-opts",
		JobID:       "job-200",
		ContentType: "email",
		BodyText:    "Test extract_assertions activity options",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test extract_assertions activity options",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage — SkipDeep=false and ContentContribution="" (defaults to HIGH)
	// so the pipeline continues into stages 2 and 2b.
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "RISK_ISSUE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 2: ExtractEntitiesActivity
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)

	// Stage 2b: ExtractAssertions — this is the activity under test
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
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(6001), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	// Verify ExtractAssertions activity was called
	s.activities.AssertCalled(s.T(), "ExtractAssertions", mock.Anything, mock.Anything)

	// CRITICAL ASSERTIONS: Verify extract_assertions uses LLM activity options
	require.NotZero(s.T(), capturedStartToClose, "should have captured StartToCloseTimeout for ExtractAssertions")

	// LLM options have StartToCloseTimeout >= 5 minutes (actual is 10min).
	// With the bug, embeddingOpts gives 30s — this assertion FAILS before the fix.
	require.GreaterOrEqual(s.T(), capturedStartToClose, 5*time.Minute,
		"extract_assertions should use LLM activity options with StartToCloseTimeout >= 5min, got %v (bug: embeddingOpts gives 30s)", capturedStartToClose)

	// LLM options have HeartbeatTimeout >= 1 minute (LLM default is 5min).
	// With the bug, embeddingOpts gives 10s — this assertion FAILS before the fix.
	require.GreaterOrEqual(s.T(), capturedHeartbeat, 1*time.Minute,
		"extract_assertions should use LLM heartbeat timeout (5min), not embedding (10s), got %v", capturedHeartbeat)
}

// TestExtractAssertionsUsesLLMActivityOptions_WithStageOverride verifies that per-stage timeout
// overrides still work when extract_assertions uses LLM options as the base.
func (s *AssertionActivityOptionsTestSuite) TestExtractAssertionsUsesLLMActivityOptions_WithStageOverride() {
	var capturedStartToClose time.Duration

	s.env.SetOnActivityStartedListener(func(activityInfo *activity.Info, ctx context.Context, args converter.EncodedValues) {
		if activityInfo.ActivityType.Name == "ExtractAssertions" {
			capturedStartToClose = activityInfo.StartToCloseTimeout
		}
	})

	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    201,
		ContentID:   "em-test-assertion-override",
		JobID:       "job-201",
		ContentType: "email",
		BodyText:    "Test extract_assertions with stage override",
		StageTimeouts: map[string]time.Duration{
			"extract_assertions": 3 * time.Minute, // Override StartToClose to 3min
		},
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test extract_assertions with stage override",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage — SkipDeep=false so pipeline continues to stage 2b
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "RISK_ISSUE",
		Importance: "HIGH",
		SkipDeep:   false,
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 2: ExtractEntitiesActivity
	s.activities.On("ExtractEntitiesActivity", mock.Anything, mock.Anything).Return(&SLMPipelineExtractEntitiesOutput{}, nil)

	// Stage 2b: ExtractAssertions — this is the activity under test
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
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(6002), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	// Verify stage override applied to StartToCloseTimeout
	require.Equal(s.T(), 3*time.Minute, capturedStartToClose,
		"stage timeout override should be applied to extract_assertions")
}

func TestAssertionActivityOptionsTestSuite(t *testing.T) {
	suite.Run(t, new(AssertionActivityOptionsTestSuite))
}
