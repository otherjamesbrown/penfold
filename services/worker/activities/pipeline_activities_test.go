package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
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
