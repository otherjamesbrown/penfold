package workflows

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// BatchPipelineTestSuite tests the BatchPipelineWorkflow.
type BatchPipelineTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestBatchPipelineWorkflow(t *testing.T) {
	suite.Run(t, new(BatchPipelineTestSuite))
}

// newBatchEnv creates a test environment with BatchPipelineWorkflow and
// SLMPipelineWorkflow registered. SLMPipelineWorkflow must be registered so the
// test framework can resolve it when child workflows are executed; individual tests
// override its behaviour with env.OnWorkflow.
func (s *BatchPipelineTestSuite) newBatchEnv() *testsuite.TestWorkflowEnvironment {
	env := s.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(BatchPipelineWorkflow)
	env.RegisterWorkflowWithOptions(SLMPipelineWorkflow, workflow.RegisterOptions{
		Name: "SLMPipelineWorkflow",
	})
	return env
}

// TestBatchPipelineWorkflow_Basic verifies that all child workflows complete successfully.
func (s *BatchPipelineTestSuite) TestBatchPipelineWorkflow_Basic() {
	env := s.newBatchEnv()

	// Mock all child SLMPipelineWorkflow executions to succeed.
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil).
		Times(3)

	input := BatchPipelineInput{
		TenantID:      "tenant-1",
		ContentIDs:    []int64{101, 102, 103},
		MaxConcurrent: 2,
		BatchID:       "batch-basic-001",
	}

	env.ExecuteWorkflow(BatchPipelineWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result BatchPipelineResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), "batch-basic-001", result.BatchID)
	require.Equal(s.T(), 3, result.TotalItems)
	require.Equal(s.T(), 3, result.CompletedItems)
	require.Equal(s.T(), 0, result.FailedItems)
	require.Equal(s.T(), "completed", result.Status)
}

// TestBatchPipelineWorkflow_ChildFailure verifies that a child workflow failure is
// recorded but does not fail the overall batch — partial success is still "completed".
func (s *BatchPipelineTestSuite) TestBatchPipelineWorkflow_ChildFailure() {
	env := s.newBatchEnv()

	// Source 101 and 103 succeed; source 102 fails. Use WorkflowID matching to
	// differentiate, since the test framework routes by match order.
	// Two succeed:
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.MatchedBy(func(in PipelineInput) bool {
		return in.SourceID == 101 || in.SourceID == 103
	})).Return(&PipelineResult{Status: "completed"}, nil).Times(2)

	// One fails:
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.MatchedBy(func(in PipelineInput) bool {
		return in.SourceID == 102
	})).Return(nil, errors.New("simulated child failure")).Times(1)

	input := BatchPipelineInput{
		TenantID:      "tenant-1",
		ContentIDs:    []int64{101, 102, 103},
		MaxConcurrent: 3,
		BatchID:       "batch-fail-001",
	}

	env.ExecuteWorkflow(BatchPipelineWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result BatchPipelineResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), 3, result.TotalItems)
	require.Equal(s.T(), 2, result.CompletedItems)
	require.Equal(s.T(), 1, result.FailedItems)
	// Overall batch status should still be "completed" — failures are recorded but
	// do not fail the parent workflow.
	require.Equal(s.T(), "completed", result.Status)
}

// TestBatchPipelineWorkflow_CancelSignal verifies that sending a cancel signal stops
// the batch from starting new items while allowing active ones to finish.
func (s *BatchPipelineTestSuite) TestBatchPipelineWorkflow_CancelSignal() {
	env := s.newBatchEnv()

	// Each child completes successfully.
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil)

	input := BatchPipelineInput{
		TenantID:      "tenant-1",
		ContentIDs:    []int64{201, 202, 203, 204, 205},
		MaxConcurrent: 1,
		BatchID:       "batch-cancel-001",
	}

	// Send a cancel signal after the workflow has started executing.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(BatchCancelSignal, nil)
	}, 0) // Fire at the earliest opportunity after workflow start.

	env.ExecuteWorkflow(BatchPipelineWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result BatchPipelineResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	// After cancel: the batch should report "cancelled" status and must not have
	// processed all 5 items (cancellation stops new items from being started).
	require.Equal(s.T(), "batch-cancel-001", result.BatchID)
	require.Equal(s.T(), "cancelled", result.Status)
	require.Less(s.T(), result.CompletedItems+result.FailedItems, 5,
		"cancelled batch should not have processed all items")
}

// TestBatchPipelineWorkflow_ProgressQuery verifies the batch-progress query handler
// returns accurate counts during and after execution.
func (s *BatchPipelineTestSuite) TestBatchPipelineWorkflow_ProgressQuery() {
	env := s.newBatchEnv()

	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil).
		Times(2)

	input := BatchPipelineInput{
		TenantID:      "tenant-1",
		ContentIDs:    []int64{301, 302},
		MaxConcurrent: 2,
		BatchID:       "batch-query-001",
	}

	env.ExecuteWorkflow(BatchPipelineWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	// Query the progress state after workflow completion.
	var progress BatchProgress
	encodedVal, err := env.QueryWorkflow(BatchProgressQuery, nil)
	require.NoError(s.T(), err)
	require.NoError(s.T(), encodedVal.Get(&progress))

	require.Equal(s.T(), "batch-query-001", progress.BatchID)
	require.Equal(s.T(), 2, progress.Total)
	require.Equal(s.T(), 2, progress.Completed)
	require.Equal(s.T(), 0, progress.Failed)
	require.Equal(s.T(), 0, progress.Pending)
	require.Equal(s.T(), 0, progress.Active)
	require.Equal(s.T(), "completed", progress.Status)
}

// TestBatchPipelineWorkflow_EmptyBatch verifies that an empty batch immediately
// returns a completed result with zero counts.
func (s *BatchPipelineTestSuite) TestBatchPipelineWorkflow_EmptyBatch() {
	env := s.newBatchEnv()

	// No child workflows should be started.
	env.OnWorkflow("SLMPipelineWorkflow", mock.Anything, mock.Anything).
		Return(&PipelineResult{Status: "completed"}, nil).
		Maybe() // Should never be called.

	input := BatchPipelineInput{
		TenantID:      "tenant-1",
		ContentIDs:    []int64{},
		MaxConcurrent: 5,
		BatchID:       "batch-empty-001",
	}

	env.ExecuteWorkflow(BatchPipelineWorkflow, input)

	require.True(s.T(), env.IsWorkflowCompleted())
	require.NoError(s.T(), env.GetWorkflowError())

	var result BatchPipelineResult
	require.NoError(s.T(), env.GetWorkflowResult(&result))

	require.Equal(s.T(), "batch-empty-001", result.BatchID)
	require.Equal(s.T(), 0, result.TotalItems)
	require.Equal(s.T(), 0, result.CompletedItems)
	require.Equal(s.T(), 0, result.FailedItems)
	require.Equal(s.T(), "completed", result.Status)
}
