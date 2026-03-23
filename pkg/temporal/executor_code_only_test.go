package temporal

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// mockActivityOutput is a simple activity output for testing.
type mockActivityOutput struct {
	Result string `json:"result"`
	Count  int    `json:"count"`
}

// CodeOnlyExecutorTestSuite tests CodeOnlyExecutor using Temporal's workflow test suite.
type CodeOnlyExecutorTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *CodeOnlyExecutorTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *CodeOnlyExecutorTestSuite) TearDownTest() {
	s.env.AssertExpectations(s.T())
}

// runExecutor runs CodeOnlyExecutor.Execute inside a test workflow and returns the result.
func (s *CodeOnlyExecutorTestSuite) runExecutor(input StageInput) (StageOutput, error) {
	s.env.ExecuteWorkflow(func(ctx workflow.Context) (StageOutput, error) {
		return (&CodeOnlyExecutor{}).Execute(ctx, input)
	})
	s.Require().True(s.env.IsWorkflowCompleted())
	var result StageOutput
	err := s.env.GetWorkflowResult(&result)
	return result, err
}

// TestSuccessfulExecution verifies that a code_only stage dispatches the activity
// and returns Success=true with RawData populated from the activity output.
func (s *CodeOnlyExecutorTestSuite) TestSuccessfulExecution() {
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (*mockActivityOutput, error) {
			return &mockActivityOutput{Result: "persisted", Count: 5}, nil
		},
		activity.RegisterOptions{Name: ActivityPersistFindings},
	)

	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:     "persist",
			StageKind: "code_only",
			Enabled:   true,
		},
		Payload: map[string]interface{}{"tenant_id": "t1", "source_id": 42},
	})

	s.Require().NoError(err)
	s.True(result.Success)
	s.False(result.Skipped)
	s.Empty(result.Error)
	s.NotEmpty(result.RawData)

	var decoded mockActivityOutput
	s.Require().NoError(json.Unmarshal(result.RawData, &decoded))
	s.Equal("persisted", decoded.Result)
	s.Equal(5, decoded.Count)
}

// TestSkipWhenLow verifies that stages marked skip_when_low are bypassed when SkipDeep is true.
// No activity is called; the executor returns Skipped=true immediately.
func (s *CodeOnlyExecutorTestSuite) TestSkipWhenLow() {
	// No activity registered — ensures none is called.
	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:       "enrich_entities",
			StageKind:   "code_only",
			Enabled:     true,
			SkipWhenLow: true,
		},
		SkipDeep: true,
		Payload:  map[string]interface{}{"tenant_id": "t1"},
	})

	s.Require().NoError(err)
	s.True(result.Success)
	s.True(result.Skipped)
	s.Equal("skip_deep", result.SkipReason)
	s.Nil(result.RawData)
}

// TestSkipWhenLowNotTriggered verifies that skip_when_low stages execute normally
// when SkipDeep is false.
func (s *CodeOnlyExecutorTestSuite) TestSkipWhenLowNotTriggered() {
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (*mockActivityOutput, error) {
			return &mockActivityOutput{Result: "enriched"}, nil
		},
		activity.RegisterOptions{Name: ActivityEnrichEntities},
	)

	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:       "enrich_entities",
			StageKind:   "code_only",
			Enabled:     true,
			SkipWhenLow: true,
		},
		SkipDeep: false,
		Payload:  map[string]interface{}{"tenant_id": "t1"},
	})

	s.Require().NoError(err)
	s.True(result.Success)
	s.False(result.Skipped)
	s.NotEmpty(result.RawData)
}

// TestOptionalStageFailure verifies that optional stages return Success=true
// when the activity fails, capturing the error in StageOutput.Error.
func (s *CodeOnlyExecutorTestSuite) TestOptionalStageFailure() {
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (*mockActivityOutput, error) {
			return nil, &enrichmentError{"entity enrichment unavailable"}
		},
		activity.RegisterOptions{Name: ActivityEnrichEntities},
	)

	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:     "enrich_entities",
			StageKind: "code_only",
			Enabled:   true,
			Optional:  true,
		},
		Payload: map[string]interface{}{"tenant_id": "t1"},
	})

	// Workflow-level error is nil — optional stages don't block.
	s.Require().NoError(err)
	s.True(result.Success)
	s.NotEmpty(result.Error)
	s.Nil(result.RawData)
}

// TestNonOptionalStageFailure verifies that non-optional stage failures propagate
// as workflow errors.
func (s *CodeOnlyExecutorTestSuite) TestNonOptionalStageFailure() {
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (*mockActivityOutput, error) {
			return nil, &enrichmentError{"persist failed"}
		},
		activity.RegisterOptions{Name: ActivityPersistFindings},
	)

	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:     "persist",
			StageKind: "code_only",
			Enabled:   true,
			Optional:  false,
		},
		Payload: map[string]interface{}{"tenant_id": "t1"},
	})

	s.Require().Error(err)
	s.False(result.Success)
}

// TestTimeoutOverride verifies that TimeoutSeconds from StageConfig overrides
// the FastActivityOptions default and the activity completes successfully.
func (s *CodeOnlyExecutorTestSuite) TestTimeoutOverride() {
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, _ interface{}) (*mockActivityOutput, error) {
			return &mockActivityOutput{Result: "ok"}, nil
		},
		activity.RegisterOptions{Name: ActivityPersistFindings},
	)

	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:          "persist",
			StageKind:      "code_only",
			Enabled:        true,
			TimeoutSeconds: 120,
		},
		Payload: map[string]interface{}{"tenant_id": "t1"},
	})

	s.Require().NoError(err)
	s.True(result.Success)
}

// TestUnknownStageName verifies that unmapped stage names return an error without
// attempting any activity dispatch.
func (s *CodeOnlyExecutorTestSuite) TestUnknownStageName() {
	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:     "nonexistent_stage",
			StageKind: "code_only",
		},
	})

	s.Require().Error(err)
	s.False(result.Success)
}

// TestMultiActivityStageRejected verifies that stages mapping to multiple activities
// are rejected — they require a different executor (e.g. StructuredExtractExecutor).
func (s *CodeOnlyExecutorTestSuite) TestMultiActivityStageRejected() {
	// newsletter_extract maps to [ActivityStructuredExtract, ActivityPersistExtractedData].
	result, err := s.runExecutor(StageInput{
		Config: StageConfig{
			Stage:     "newsletter_extract",
			StageKind: "code_only", // wrong kind, but executor validates on activity count
		},
	})

	s.Require().Error(err)
	s.False(result.Success)
}

func TestCodeOnlyExecutor(t *testing.T) {
	suite.Run(t, new(CodeOnlyExecutorTestSuite))
}

// enrichmentError is a simple error type for tests.
type enrichmentError struct{ msg string }

func (e *enrichmentError) Error() string { return e.msg }
