package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockPipelineClient implements pipelinev1.PipelineServiceClient for testing.
// Only KickProcessing is functional; all other methods are stubs that return nil.
type mockPipelineClient struct {
	kickFunc func(ctx context.Context, req *pipelinev1.KickProcessingRequest, opts ...grpc.CallOption) (*pipelinev1.KickProcessingResponse, error)
}

func (m *mockPipelineClient) GetStats(ctx context.Context, in *pipelinev1.GetStatsRequest, opts ...grpc.CallOption) (*pipelinev1.GetStatsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetJob(ctx context.Context, in *pipelinev1.GetJobRequest, opts ...grpc.CallOption) (*pipelinev1.GetJobResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListJobs(ctx context.Context, in *pipelinev1.ListJobsRequest, opts ...grpc.CallOption) (*pipelinev1.ListJobsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) KickProcessing(ctx context.Context, in *pipelinev1.KickProcessingRequest, opts ...grpc.CallOption) (*pipelinev1.KickProcessingResponse, error) {
	if m.kickFunc != nil {
		return m.kickFunc(ctx, in, opts...)
	}
	return &pipelinev1.KickProcessingResponse{}, nil
}
func (m *mockPipelineClient) RetryFailed(ctx context.Context, in *pipelinev1.RetryFailedRequest, opts ...grpc.CallOption) (*pipelinev1.RetryFailedResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetQueueStatus(ctx context.Context, in *pipelinev1.GetQueueStatusRequest, opts ...grpc.CallOption) (*pipelinev1.GetQueueStatusResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetPipelineHealth(ctx context.Context, in *pipelinev1.GetPipelineHealthRequest, opts ...grpc.CallOption) (*pipelinev1.GetPipelineHealthResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetContentTrace(ctx context.Context, in *pipelinev1.GetContentTraceRequest, opts ...grpc.CallOption) (*pipelinev1.GetContentTraceResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListDeletedSources(ctx context.Context, in *pipelinev1.ListDeletedSourcesRequest, opts ...grpc.CallOption) (*pipelinev1.ListDeletedSourcesResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) UndeleteSource(ctx context.Context, in *pipelinev1.UndeleteSourceRequest, opts ...grpc.CallOption) (*pipelinev1.UndeleteSourceResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) DescribePipeline(ctx context.Context, in *pipelinev1.DescribePipelineRequest, opts ...grpc.CallOption) (*pipelinev1.DescribePipelineResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetPrompt(ctx context.Context, in *pipelinev1.GetPromptRequest, opts ...grpc.CallOption) (*pipelinev1.GetPromptResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListPromptVersions(ctx context.Context, in *pipelinev1.ListPromptVersionsRequest, opts ...grpc.CallOption) (*pipelinev1.ListPromptVersionsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) UpdatePrompt(ctx context.Context, in *pipelinev1.UpdatePromptRequest, opts ...grpc.CallOption) (*pipelinev1.UpdatePromptResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) RollbackPrompt(ctx context.Context, in *pipelinev1.RollbackPromptRequest, opts ...grpc.CallOption) (*pipelinev1.RollbackPromptResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ExportPrompt(ctx context.Context, in *pipelinev1.ExportPromptRequest, opts ...grpc.CallOption) (*pipelinev1.ExportPromptResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetSourceHistory(ctx context.Context, in *pipelinev1.GetSourceHistoryRequest, opts ...grpc.CallOption) (*pipelinev1.GetSourceHistoryResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ReprocessDryRun(ctx context.Context, in *pipelinev1.ReprocessDryRunRequest, opts ...grpc.CallOption) (*pipelinev1.ReprocessDryRunResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetTimeoutConfig(ctx context.Context, in *pipelinev1.GetTimeoutConfigRequest, opts ...grpc.CallOption) (*pipelinev1.GetTimeoutConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) UpdateTimeoutConfig(ctx context.Context, in *pipelinev1.UpdateTimeoutConfigRequest, opts ...grpc.CallOption) (*pipelinev1.UpdateTimeoutConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetPipelineErrors(ctx context.Context, in *pipelinev1.GetPipelineErrorsRequest, opts ...grpc.CallOption) (*pipelinev1.GetPipelineErrorsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) InspectStage(ctx context.Context, in *pipelinev1.InspectStageRequest, opts ...grpc.CallOption) (*pipelinev1.InspectStageResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) DiffStageRuns(ctx context.Context, in *pipelinev1.DiffStageRunsRequest, opts ...grpc.CallOption) (*pipelinev1.DiffStageRunsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) DiffPipelineRuns(ctx context.Context, in *pipelinev1.DiffPipelineRunsRequest, opts ...grpc.CallOption) (*pipelinev1.DiffPipelineRunsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetConcurrencyConfig(ctx context.Context, in *pipelinev1.GetConcurrencyConfigRequest, opts ...grpc.CallOption) (*pipelinev1.GetConcurrencyConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) SetConcurrencyConfig(ctx context.Context, in *pipelinev1.SetConcurrencyConfigRequest, opts ...grpc.CallOption) (*pipelinev1.SetConcurrencyConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListPendingSources(ctx context.Context, in *pipelinev1.ListPendingSourcesRequest, opts ...grpc.CallOption) (*pipelinev1.ListPendingSourcesResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetStageConfig(ctx context.Context, in *pipelinev1.GetStageConfigRequest, opts ...grpc.CallOption) (*pipelinev1.GetStageConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) StartBatchPipeline(ctx context.Context, in *pipelinev1.StartBatchPipelineRequest, opts ...grpc.CallOption) (*pipelinev1.StartBatchPipelineResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetBatchStatus(ctx context.Context, in *pipelinev1.GetBatchStatusRequest, opts ...grpc.CallOption) (*pipelinev1.GetBatchStatusResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListBatches(ctx context.Context, in *pipelinev1.ListBatchesRequest, opts ...grpc.CallOption) (*pipelinev1.ListBatchesResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) CancelBatch(ctx context.Context, in *pipelinev1.CancelBatchRequest, opts ...grpc.CallOption) (*pipelinev1.CancelBatchResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListModels(ctx context.Context, in *pipelinev1.ListModelsRequest, opts ...grpc.CallOption) (*pipelinev1.ListModelsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListClassificationRules(ctx context.Context, in *pipelinev1.ListClassificationRulesRequest, opts ...grpc.CallOption) (*pipelinev1.ListClassificationRulesResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetClassificationRule(ctx context.Context, in *pipelinev1.GetClassificationRuleRequest, opts ...grpc.CallOption) (*pipelinev1.GetClassificationRuleResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) TestClassificationRule(ctx context.Context, in *pipelinev1.TestClassificationRuleRequest, opts ...grpc.CallOption) (*pipelinev1.TestClassificationRuleResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListPipelineRoutes(ctx context.Context, in *pipelinev1.ListPipelineRoutesRequest, opts ...grpc.CallOption) (*pipelinev1.ListPipelineRoutesResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) TestPipelineRoute(ctx context.Context, in *pipelinev1.TestPipelineRouteRequest, opts ...grpc.CallOption) (*pipelinev1.TestPipelineRouteResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ListPipelineDefinitions(ctx context.Context, in *pipelinev1.ListPipelineDefinitionsRequest, opts ...grpc.CallOption) (*pipelinev1.ListPipelineDefinitionsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetPipelineDefinition(ctx context.Context, in *pipelinev1.GetPipelineDefinitionRequest, opts ...grpc.CallOption) (*pipelinev1.GetPipelineDefinitionResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) UpdatePipelineStageConfig(ctx context.Context, in *pipelinev1.UpdatePipelineStageConfigRequest, opts ...grpc.CallOption) (*pipelinev1.UpdatePipelineStageConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) CreatePipelineDefinition(ctx context.Context, in *pipelinev1.CreatePipelineDefinitionRequest, opts ...grpc.CallOption) (*pipelinev1.CreatePipelineDefinitionResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) AuditPipelineCompleteness(ctx context.Context, in *pipelinev1.AuditPipelineCompletenessRequest, opts ...grpc.CallOption) (*pipelinev1.AuditPipelineCompletenessResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) ComparePipelineRuns(ctx context.Context, in *pipelinev1.ComparePipelineRunsRequest, opts ...grpc.CallOption) (*pipelinev1.ComparePipelineRunsResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) GetOperationalConfig(ctx context.Context, in *pipelinev1.GetOperationalConfigRequest, opts ...grpc.CallOption) (*pipelinev1.GetOperationalConfigResponse, error) {
	return nil, nil
}
func (m *mockPipelineClient) SetOperationalConfig(ctx context.Context, in *pipelinev1.SetOperationalConfigRequest, opts ...grpc.CallOption) (*pipelinev1.SetOperationalConfigResponse, error) {
	return nil, nil
}

// Compile-time check that mockPipelineClient satisfies the interface.
var _ pipelinev1.PipelineServiceClient = (*mockPipelineClient)(nil)

// newTestPipelineActivities creates a PipelineActivities suitable for unit testing.
// pipelineRepo and baseRepo are nil — safe when only KickNextPending is exercised.
func newTestPipelineActivities(client pipelinev1.PipelineServiceClient) *PipelineActivities {
	logger := logging.NewNopLogger()
	return &PipelineActivities{
		logger:         logger.With(logging.F("component", "pipeline_activities")),
		pipelineClient: client,
	}
}

// TestKickNextPending_NilClient verifies that KickNextPending returns a zero result
// without error when the pipeline client is not configured (best-effort degradation).
func TestKickNextPending_NilClient(t *testing.T) {
	a := newTestPipelineActivities(nil)

	out, err := a.KickNextPending(context.Background(), workflows.KickNextPendingInput{
		TenantID: "tenant-1",
		Limit:    10,
	})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, int64(0), out.QueuedCount)
	require.Equal(t, "", out.Message)
}

// TestKickNextPending_Success verifies that KickNextPending calls KickProcessing
// with correct parameters and maps the response fields to the output type.
func TestKickNextPending_Success(t *testing.T) {
	mockClient := &mockPipelineClient{
		kickFunc: func(ctx context.Context, req *pipelinev1.KickProcessingRequest, opts ...grpc.CallOption) (*pipelinev1.KickProcessingResponse, error) {
			require.Equal(t, "tenant-abc", req.TenantId)
			require.Equal(t, int32(5), req.Limit)
			return &pipelinev1.KickProcessingResponse{
				QueuedCount: 3,
				Message:     "queued 3 items",
			}, nil
		},
	}

	a := newTestPipelineActivities(mockClient)

	out, err := a.KickNextPending(context.Background(), workflows.KickNextPendingInput{
		TenantID: "tenant-abc",
		Limit:    5,
	})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, int64(3), out.QueuedCount)
	require.Equal(t, "queued 3 items", out.Message)
}

// TestKickNextPending_RPCError verifies that a gRPC error from KickProcessing
// is wrapped in a temporal ApplicationError and propagated as a non-nil error.
func TestKickNextPending_RPCError(t *testing.T) {
	rpcErr := errors.New("connection refused")
	mockClient := &mockPipelineClient{
		kickFunc: func(ctx context.Context, req *pipelinev1.KickProcessingRequest, opts ...grpc.CallOption) (*pipelinev1.KickProcessingResponse, error) {
			return nil, rpcErr
		},
	}

	a := newTestPipelineActivities(mockClient)

	out, err := a.KickNextPending(context.Background(), workflows.KickNextPendingInput{
		TenantID: "tenant-xyz",
		Limit:    1,
	})

	require.Error(t, err)
	require.Nil(t, out)
	require.Contains(t, err.Error(), "KickProcessing RPC failed")
}

// TestKickNextPending_ZeroLimit verifies that a zero limit is passed through as-is
// (zero means no limit in the gateway's KickProcessing implementation).
func TestKickNextPending_ZeroLimit(t *testing.T) {
	var capturedLimit int32
	mockClient := &mockPipelineClient{
		kickFunc: func(ctx context.Context, req *pipelinev1.KickProcessingRequest, opts ...grpc.CallOption) (*pipelinev1.KickProcessingResponse, error) {
			capturedLimit = req.Limit
			return &pipelinev1.KickProcessingResponse{
				QueuedCount: 0,
				Message:     "nothing to queue",
			}, nil
		},
	}

	a := newTestPipelineActivities(mockClient)

	out, err := a.KickNextPending(context.Background(), workflows.KickNextPendingInput{
		TenantID: "tenant-1",
		Limit:    0, // zero = no limit
	})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, int32(0), capturedLimit)
	require.Equal(t, int64(0), out.QueuedCount)
}

// ---
// Tests for RecordSkippedStage (pf-e05c78)
// Bug: RecordSkippedStage silently swallowed DB errors (logger.Warn + return nil).
// Fix: propagate the first error encountered so callers (Temporal) can retry.
// ---

// mockFailingPipelineRepo is a PipelineRepository whose CreateRun always returns the
// configured error. RecordOverrides is a no-op stub.
type mockFailingPipelineRepo struct {
	err        error
	calledWith []PipelineRunInput
}

func (r *mockFailingPipelineRepo) CreateRun(_ context.Context, input PipelineRunInput) error {
	r.calledWith = append(r.calledWith, input)
	return r.err
}

func (r *mockFailingPipelineRepo) RecordOverrides(_ context.Context, _ int64, _ map[string]string) error {
	return nil
}

func (r *mockFailingPipelineRepo) GetContextProviders(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}

// newTestPipelineActivitiesWithRepo creates a PipelineActivities with a real pipelineRepo,
// enabling tests of activities that depend on the repo (e.g. RecordSkippedStage).
func newTestPipelineActivitiesWithRepo(repo PipelineRepository) *PipelineActivities {
	logger := logging.NewNopLogger()
	return &PipelineActivities{
		logger:       logger.With(logging.F("component", "pipeline_activities")),
		pipelineRepo: repo,
	}
}

// TestRecordSkippedStage_SurfacesErrors verifies that RecordSkippedStage returns an
// error when CreateRun fails, rather than silently swallowing it (pf-e05c78 bug 1).
func TestRecordSkippedStage_SurfacesErrors(t *testing.T) {
	dbErr := errors.New("pq: insert or update on table \"pipeline_runs\" violates foreign key constraint \"pipeline_runs_stage_fkey\"")

	repo := &mockFailingPipelineRepo{err: dbErr}
	a := newTestPipelineActivitiesWithRepo(repo)

	err := a.RecordSkippedStage(context.Background(), workflows.RecordSkippedStageInput{
		SourceID: 42,
		Stages: []workflows.SkippedStage{
			{Stage: "summarize", SkipReason: "contribution_gating:LOW"},
		},
		LangfuseTraceID: "trace-abc",
	})

	require.Error(t, err,
		"RecordSkippedStage must return an error when CreateRun fails (pf-e05c78: errors were silently swallowed)")
}

// TestRecordSkippedStage_StageMismatch_Summarize verifies that when CreateRun returns a
// FK violation for stage='summarize' (the stage name used by workflow code), the error
// is surfaced rather than swallowed (pf-e05c78 bug 2).
func TestRecordSkippedStage_StageMismatch_Summarize(t *testing.T) {
	fkErr := errors.New("pq: insert or update on table \"pipeline_runs\" violates foreign key constraint \"pipeline_runs_stage_fkey\": key (stage)=(summarize) is not present in table \"pipeline_stages\"")

	repo := &mockFailingPipelineRepo{err: fkErr}
	a := newTestPipelineActivitiesWithRepo(repo)

	// This is the exact call pattern from pipeline.go (the summarize skip path).
	err := a.RecordSkippedStage(context.Background(), workflows.RecordSkippedStageInput{
		SourceID: 99,
		Stages: []workflows.SkippedStage{
			{Stage: "summarize", SkipReason: "contribution_gating:NONE"},
		},
		LangfuseTraceID: "trace-xyz",
	})

	// Verify the repo received the call with stage="summarize".
	require.Len(t, repo.calledWith, 1,
		"CreateRun must have been called once for the single skipped stage")
	require.Equal(t, "summarize", repo.calledWith[0].Stage,
		"Stage name sent to CreateRun must be 'summarize' as used by the workflow")

	require.Error(t, err,
		"RecordSkippedStage must surface the FK violation instead of swallowing it (pf-e05c78)")
}

// TestRecordSkippedStage_PartialFailure_AllErrorsSurfaced verifies that when all
// CreateRun calls fail, RecordSkippedStage still calls each stage and returns an error.
// All stages must be attempted even when earlier ones fail.
func TestRecordSkippedStage_PartialFailure_AllErrorsSurfaced(t *testing.T) {
	dbErr := errors.New("pq: FK violation on pipeline_runs.stage")

	repo := &mockFailingPipelineRepo{err: dbErr}
	a := newTestPipelineActivitiesWithRepo(repo)

	err := a.RecordSkippedStage(context.Background(), workflows.RecordSkippedStageInput{
		SourceID: 10,
		Stages: []workflows.SkippedStage{
			{Stage: "summarize", SkipReason: "routing:no_pipeline:MEETING/TRANSCRIPT"},
			{Stage: "extract_ner", SkipReason: "routing:no_pipeline:MEETING/TRANSCRIPT"},
			{Stage: "analyze", SkipReason: "routing:no_pipeline:MEETING/TRANSCRIPT"},
		},
	})

	// All three CreateRun calls must have been attempted.
	require.Len(t, repo.calledWith, 3,
		"CreateRun must be called for every stage even when earlier stages fail")

	require.Error(t, err,
		"RecordSkippedStage must return an error when all CreateRun calls fail (pf-e05c78)")
}

// TestRecordSkippedStage_SuccessPath verifies that when CreateRun succeeds,
// RecordSkippedStage returns nil and calls CreateRun for each stage with correct fields.
func TestRecordSkippedStage_SuccessPath(t *testing.T) {
	repo := &mockFailingPipelineRepo{err: nil} // no error — success
	a := newTestPipelineActivitiesWithRepo(repo)

	err := a.RecordSkippedStage(context.Background(), workflows.RecordSkippedStageInput{
		SourceID: 7,
		Stages: []workflows.SkippedStage{
			{Stage: "summarize", SkipReason: "contribution_gating:LOW"},
			{Stage: "embed", SkipReason: "contribution_gating:LOW"},
		},
		LangfuseTraceID: "trace-success",
	})

	require.NoError(t, err, "RecordSkippedStage must return nil when all CreateRun calls succeed")
	require.Len(t, repo.calledWith, 2, "CreateRun must be called once per stage")
	require.Equal(t, "summarize", repo.calledWith[0].Stage)
	require.Equal(t, "skipped", repo.calledWith[0].Status)
	require.Equal(t, int64(7), repo.calledWith[0].SourceID)
	require.Equal(t, "contribution_gating:LOW", repo.calledWith[0].SkipReason)
	require.Equal(t, "trace-success", repo.calledWith[0].LangfuseTraceID)
}

// TestRecordSkippedStage_NoStages verifies that RecordSkippedStage returns nil
// when called with an empty stages list (early-exit path).
func TestRecordSkippedStage_NoStages(t *testing.T) {
	repo := &mockFailingPipelineRepo{err: nil}
	a := newTestPipelineActivitiesWithRepo(repo)

	err := a.RecordSkippedStage(context.Background(), workflows.RecordSkippedStageInput{
		SourceID: 1,
		Stages:   []workflows.SkippedStage{},
	})

	require.NoError(t, err, "RecordSkippedStage must return nil for empty stage list")
	require.Empty(t, repo.calledWith, "CreateRun must not be called when stages list is empty")
}

// ---
// Tests for FetchPipelineDefinition error paths (pf-85e95e)
// ---

// mockDefinitionRepo implements pipelineDefinitionRepo for testing.
type mockDefinitionRepo struct {
	stages []pipeline.StageDefinition
	err    error
}

func (r *mockDefinitionRepo) GetPipelineStages(_ context.Context, _, _ string) ([]pipeline.StageDefinition, error) {
	return r.stages, r.err
}

// newTestFetchPipelineDefinition creates a PipelineActivities wired with the given
// definition repo mock. Only definitionRepo and logger are populated.
func newTestFetchPipelineDefinition(repo pipelineDefinitionRepo) *PipelineActivities {
	logger := logging.NewNopLogger()
	return &PipelineActivities{
		logger:         logger.With(logging.F("component", "pipeline_activities")),
		definitionRepo: repo,
	}
}

// TestFetchPipelineDefinition_EmptyPipeline verifies that an empty pipeline name
// returns a NonRetryableApplicationError of type "ConfigurationError".
func TestFetchPipelineDefinition_EmptyPipeline(t *testing.T) {
	a := newTestFetchPipelineDefinition(&mockDefinitionRepo{})

	out, err := a.FetchPipelineDefinition(context.Background(), workflows.FetchPipelineDefinitionInput{
		TenantID: "tenant-1",
		Pipeline: "",
	})

	require.Nil(t, out)
	require.Error(t, err)

	var appErr *temporal.ApplicationError
	require.ErrorAs(t, err, &appErr, "error must be a temporal.ApplicationError")
	require.True(t, appErr.NonRetryable(), "empty pipeline name must produce a NonRetryableApplicationError")
	require.Equal(t, "ConfigurationError", appErr.Type())
	require.Contains(t, appErr.Message(), "pipeline definition not found")
}

// TestFetchPipelineDefinition_NoStages verifies that a successful DB query returning
// zero rows produces a NonRetryableApplicationError of type "ConfigurationError".
// This is a configuration error — the pipeline exists in code but has no DB definition.
func TestFetchPipelineDefinition_NoStages(t *testing.T) {
	a := newTestFetchPipelineDefinition(&mockDefinitionRepo{
		stages: []pipeline.StageDefinition{}, // query succeeded, 0 rows
		err:    nil,
	})

	out, err := a.FetchPipelineDefinition(context.Background(), workflows.FetchPipelineDefinitionInput{
		TenantID: "tenant-1",
		Pipeline: "standard",
	})

	require.Nil(t, out)
	require.Error(t, err)

	var appErr *temporal.ApplicationError
	require.ErrorAs(t, err, &appErr, "error must be a temporal.ApplicationError")
	require.True(t, appErr.NonRetryable(), "missing pipeline definition must produce a NonRetryableApplicationError")
	require.Equal(t, "ConfigurationError", appErr.Type())
	require.Contains(t, appErr.Message(), "pipeline definition not found")
	require.Contains(t, appErr.Message(), "pipeline=standard")
	require.Contains(t, appErr.Message(), "tenant=tenant-1")
}

// TestFetchPipelineDefinition_DBError verifies that a transient DB error produces a
// retryable ApplicationError (type "RepositoryError"), NOT a NonRetryableApplicationError.
// Temporal's retry policy will retry these automatically.
func TestFetchPipelineDefinition_DBError(t *testing.T) {
	dbErr := errors.New("pq: connection refused")
	a := newTestFetchPipelineDefinition(&mockDefinitionRepo{err: dbErr})

	out, err := a.FetchPipelineDefinition(context.Background(), workflows.FetchPipelineDefinitionInput{
		TenantID: "tenant-1",
		Pipeline: "standard",
	})

	require.Nil(t, out)
	require.Error(t, err)

	var appErr *temporal.ApplicationError
	require.ErrorAs(t, err, &appErr, "error must be a temporal.ApplicationError")
	require.False(t, appErr.NonRetryable(), "DB errors must remain retryable — they are transient failures, not configuration errors")
	require.Equal(t, "RepositoryError", appErr.Type())
}
