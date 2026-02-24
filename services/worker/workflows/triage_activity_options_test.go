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

// TriageActivityOptionsTestSuite tests that triage stage uses LLM activity options.
// This is a regression test for pf-c31214 where triage used embedding options
// (10s heartbeat, 30s start-to-close, NO ScheduleToClose) instead of LLM options,
// causing infinite heartbeat-triggered rescheduling when Ollama takes >10s.
type TriageActivityOptionsTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env        *testsuite.TestWorkflowEnvironment
	activities *PipelineMockActivities
}

func (s *TriageActivityOptionsTestSuite) SetupTest() {
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
	s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{Name: "GenerateContentSummary"})

	// Default mock expectations for enrichment/threading activities
	s.activities.On("CreateEnrichmentRecord", mock.Anything, mock.Anything).Maybe().Return(&CreateEnrichmentRecordOutput{EnrichmentID: 1}, nil)
	s.activities.On("GroupEmailThread", mock.Anything, mock.Anything).Maybe().Return(&GroupEmailThreadOutput{}, nil)
	s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Maybe().Return(int64(0), nil)
}

func (s *TriageActivityOptionsTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

// TestTriageUsesLLMActivityOptions verifies that triage stage uses LLM activity options.
//
// Requirements (from pf-c31214):
// - Triage stage must use LLM activity options, not embedding options
// - StartToCloseTimeout must be >= 5 minutes (LLM default is 10min)
// - ScheduleToCloseTimeout must be set (non-zero)
//
// This test WILL FAIL before the fix because triage currently uses embeddingOpts
// (line 850 in pipeline.go: triageOpts := stageOpts("triage", embeddingOpts))
//
// Expected behavior after fix:
// - Line 850 changes from embeddingOpts to llmOpts
// - Triage will have StartToCloseTimeout=10min, ScheduleToCloseTimeout=15min
func (s *TriageActivityOptionsTestSuite) TestTriageUsesLLMActivityOptions() {
	// Set up listener to capture activity options when Triage is called
	var capturedStartToClose time.Duration
	var capturedScheduleToClose time.Duration
	var capturedHeartbeat time.Duration

	s.env.SetOnActivityStartedListener(func(activityInfo *activity.Info, ctx context.Context, args converter.EncodedValues) {
		if activityInfo.ActivityType.Name == "Triage" {
			// Capture the activity options used for triage
			capturedStartToClose = activityInfo.StartToCloseTimeout
			capturedScheduleToClose = activityInfo.ScheduleToCloseTimeout
			capturedHeartbeat = activityInfo.HeartbeatTimeout
		}
	})

	input := PipelineInput{
		TenantID:    "tenant-1",
		SourceID:    100,
		ContentID:   "em-test-triage",
		JobID:       "job-100",
		ContentType: "email",
		BodyText:    "Test triage activity options",
	}

	// Stage 0: Parse
	s.activities.On("ParseEmail", mock.Anything, mock.Anything).Return(&ParseEmailOutput{
		CleanBody: "Test triage activity options",
	}, nil)
	s.activities.On("UpdateContentStatus", mock.Anything, mock.Anything).Return(nil)

	// Stage 1: Triage
	s.activities.On("Triage", mock.Anything, mock.Anything).Return(&TriageOutput{
		Category:   "PERSONAL",
		Importance: "LOW",
		SkipDeep:   true, // Skip deep processing to make test faster
		ModelUsed:  "llama-3.2-1b",
	}, nil)

	// Stage 5: Embed
	s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(int64(5001), nil)

	s.env.ExecuteWorkflow(SLMPipelineWorkflow, input)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())

	// Verify triage activity was called
	s.activities.AssertCalled(s.T(), "Triage", mock.Anything, mock.Anything)

	// CRITICAL ASSERTIONS: Verify triage uses LLM activity options
	require.NotZero(s.T(), capturedStartToClose, "should have captured StartToCloseTimeout for Triage")

	// LLM options have StartToCloseTimeout >= 5 minutes (actual is 10min)
	require.GreaterOrEqual(s.T(), capturedStartToClose, 5*time.Minute,
		"triage should use LLM activity options with StartToCloseTimeout >= 5min, got %v", capturedStartToClose)

	// LLM options MUST have ScheduleToCloseTimeout set (embedding options have 0)
	require.NotZero(s.T(), capturedScheduleToClose,
		"triage must have ScheduleToCloseTimeout set (LLM default is 15min), got %v", capturedScheduleToClose)

	// LLM options have HeartbeatTimeout of 5 minutes (embedding options have 10s)
	// This is informational but helps confirm we're using LLM options
	require.GreaterOrEqual(s.T(), capturedHeartbeat, 1*time.Minute,
		"triage should use LLM heartbeat timeout (5min), not embedding (10s), got %v", capturedHeartbeat)
}

func TestTriageActivityOptionsTestSuite(t *testing.T) {
	suite.Run(t, new(TriageActivityOptionsTestSuite))
}
