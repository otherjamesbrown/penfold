package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// EmbeddingExecutorTestSuite tests EmbeddingExecutor using Temporal's workflow test suite.
// This is required because EmbeddingExecutor calls workflow.ExecuteActivity and must
// run inside workflow code.
type EmbeddingExecutorTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *EmbeddingExecutorTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *EmbeddingExecutorTestSuite) TearDownTest() {
	s.env.AssertExpectations(s.T())
}

// runExecutor executes EmbeddingExecutor.Execute inside a test workflow.
// The workflow.Context is bridged to context.Context via WithWorkflowContext.
func (s *EmbeddingExecutorTestSuite) runExecutor(stage PipelineStageConfig, input StageInput) (StageOutput, error) {
	s.env.ExecuteWorkflow(func(wfCtx workflow.Context) (StageOutput, error) {
		ctx := WithWorkflowContext(context.Background(), wfCtx)
		return (&EmbeddingExecutor{}).Execute(ctx, stage, input)
	})
	s.Require().True(s.env.IsWorkflowCompleted())
	var result StageOutput
	err := s.env.GetWorkflowResult(&result)
	return result, err
}

// registerMockEmbedding registers a mock GenerateContentEmbedding activity that
// returns the given embeddingID or error.
func (s *EmbeddingExecutorTestSuite) registerMockEmbedding(embeddingID int64, err error) {
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, _ embeddingActivityInput) (int64, error) {
			return embeddingID, err
		},
		activity.RegisterOptions{Name: ActivityGenerateContentEmbedding},
	)
}

// --- Tests ---

// TestSuccessfulEmbedding verifies the happy path: activity succeeds, StageOutput
// has Success=true and RawData containing the embedding ID.
func (s *EmbeddingExecutorTestSuite) TestSuccessfulEmbedding() {
	s.registerMockEmbedding(99, nil)

	result, err := s.runExecutor(
		PipelineStageConfig{Stage: "embed", StageKind: "embedding"},
		StageInput{TenantID: "t1", SourceID: 100, Content: "hello world"},
	)

	s.Require().NoError(err)
	s.True(result.Success)
	s.Equal("embed", result.StageName)
	s.Equal("embedding", result.StageKind)
	s.Empty(result.Error)
	s.NotEmpty(result.RawData)

	var payload EmbeddingResult
	s.Require().NoError(json.Unmarshal(result.RawData, &payload))
	s.Equal(int64(99), payload.EmbeddingID)
}

// TestActivityInputMapping verifies that StageInput fields are passed correctly
// to the embedding activity.
func (s *EmbeddingExecutorTestSuite) TestActivityInputMapping() {
	var capturedInput embeddingActivityInput
	s.env.RegisterActivityWithOptions(
		func(_ context.Context, in embeddingActivityInput) (int64, error) {
			capturedInput = in
			return 42, nil
		},
		activity.RegisterOptions{Name: ActivityGenerateContentEmbedding},
	)

	_, err := s.runExecutor(
		PipelineStageConfig{Stage: "embed", StageKind: "embedding"},
		StageInput{
			TenantID:        "tenant-abc",
			SourceID:        200,
			ContentID:       "em-xyz",
			Content:         "document text",
			LangfuseTraceID: "trace-123",
		},
	)

	s.Require().NoError(err)
	s.Equal("tenant-abc", capturedInput.TenantID)
	s.Equal(int64(200), capturedInput.SourceID)
	s.Equal("em-xyz", capturedInput.ContentID)
	s.Equal("document text", capturedInput.Content)
	s.Equal("trace-123", capturedInput.LangfuseTraceID)
}

// TestNonOptionalFailure verifies that when stage.Optional=false and the activity fails,
// Execute returns a non-nil error so the dispatch loop treats the stage as a pipeline failure.
// Note: Temporal does not populate the result struct when a workflow returns an error,
// so only the workflow-level error is asserted here.
func (s *EmbeddingExecutorTestSuite) TestNonOptionalFailure() {
	s.registerMockEmbedding(0, errors.New("embedding service unavailable"))

	result, err := s.runExecutor(
		PipelineStageConfig{Stage: "embed", StageKind: "embedding", Optional: false},
		StageInput{TenantID: "t1", SourceID: 1, Content: "text"},
	)

	s.Require().Error(err)
	s.False(result.Success)
}

// TestOptionalFailure verifies that when stage.Optional=true and the activity fails,
// Execute returns Success=false with a nil error (pipeline may continue).
func (s *EmbeddingExecutorTestSuite) TestOptionalFailure() {
	s.registerMockEmbedding(0, errors.New("embedding service unavailable"))

	result, err := s.runExecutor(
		PipelineStageConfig{Stage: "embed", StageKind: "embedding", Optional: true},
		StageInput{TenantID: "t1", SourceID: 1, Content: "text"},
	)

	s.Require().NoError(err)
	s.False(result.Success)
	s.NotEmpty(result.Error)
	s.Nil(result.RawData)
}

// TestTimeoutOverride verifies that TimeoutSeconds from stage config overrides the
// EmbeddingActivityOptions default. The test simply confirms the activity still runs.
func (s *EmbeddingExecutorTestSuite) TestTimeoutOverride() {
	s.registerMockEmbedding(77, nil)

	result, err := s.runExecutor(
		PipelineStageConfig{Stage: "embed", StageKind: "embedding", TimeoutSeconds: 120},
		StageInput{TenantID: "t1", SourceID: 1, Content: "text"},
	)

	s.Require().NoError(err)
	s.True(result.Success)

	var payload EmbeddingResult
	s.Require().NoError(json.Unmarshal(result.RawData, &payload))
	s.Equal(int64(77), payload.EmbeddingID)
}

// TestRegisteredInRegistry verifies that EmbeddingExecutor can be registered and
// retrieved from ExecutorRegistry under the "embedding" stage_kind.
func (s *EmbeddingExecutorTestSuite) TestRegisteredInRegistry() {
	reporter := &mockReporter{}
	registry := NewExecutorRegistry(reporter)
	registry.Register("embedding", &EmbeddingExecutor{})

	executor, err := registry.Get("embedding")
	s.Require().NoError(err)
	s.NotNil(executor)
}

// TestMissingWorkflowContext verifies that Execute returns an error when called
// without a workflow context (i.e. outside a workflow — should never happen in production).
func (s *EmbeddingExecutorTestSuite) TestMissingWorkflowContext() {
	executor := &EmbeddingExecutor{}
	_, err := executor.Execute(context.Background(), PipelineStageConfig{Stage: "embed"}, StageInput{})
	s.Require().Error(err)
	s.Contains(err.Error(), "workflow context not available")
}

func TestEmbeddingExecutor(t *testing.T) {
	suite.Run(t, new(EmbeddingExecutorTestSuite))
}
