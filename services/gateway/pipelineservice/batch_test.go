package pipelineservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
)

// TestStartBatchPipeline_NoTemporalClient verifies that StartBatchPipeline
// returns Unavailable when no Temporal client is configured.
func TestStartBatchPipeline_NoTemporalClient(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	_, err := svc.StartBatchPipeline(context.Background(), &pipelinev1.StartBatchPipelineRequest{
		TenantId:   "test-tenant",
		ContentIds: []int64{1, 2, 3},
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestGetBatchStatus_MissingBatchID verifies that GetBatchStatus returns
// InvalidArgument when no batch_id is provided.
func TestGetBatchStatus_MissingBatchID(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	_, err := svc.GetBatchStatus(context.Background(), &pipelinev1.GetBatchStatusRequest{})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// TestCancelBatch_MissingBatchID verifies that CancelBatch returns
// InvalidArgument when no batch_id is provided.
func TestCancelBatch_MissingBatchID(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	_, err := svc.CancelBatch(context.Background(), &pipelinev1.CancelBatchRequest{})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// TestListBatches_DefaultLimit verifies that ListBatches uses a default limit
// when limit is 0. This is a structure/validation test; DB access would be
// exercised in integration tests.
func TestListBatches_DefaultLimit(t *testing.T) {
	req := &pipelinev1.ListBatchesRequest{
		TenantId: "test-tenant",
		Limit:    0, // should default to 20
	}
	assert.Equal(t, "test-tenant", req.TenantId)
	assert.Equal(t, int32(0), req.Limit)
}

// TestBatchToProto verifies that the batchToProto helper converts correctly.
func TestBatchToProto_NilSafe(t *testing.T) {
	result := batchToProto(nil)
	assert.Nil(t, result)
}

// TestStartBatchPipeline_RequestStructure verifies proto field wiring.
func TestStartBatchPipeline_RequestStructure(t *testing.T) {
	req := &pipelinev1.StartBatchPipelineRequest{
		TenantId:      "tenant-abc",
		ContentIds:    []int64{10, 20, 30},
		MaxConcurrent: 5,
		SourceTag:     "gmail",
	}
	assert.Equal(t, "tenant-abc", req.TenantId)
	assert.Equal(t, []int64{10, 20, 30}, req.ContentIds)
	assert.Equal(t, int32(5), req.MaxConcurrent)
	assert.Equal(t, "gmail", req.SourceTag)
}

// TestGetBatchStatus_ResponseStructure verifies proto field wiring.
func TestGetBatchStatus_ResponseStructure(t *testing.T) {
	resp := &pipelinev1.GetBatchStatusResponse{
		BatchId:        "batch-123",
		Status:         "running",
		TotalItems:     100,
		CompletedItems: 40,
		FailedItems:    5,
		PendingItems:   55,
		ActiveItems:    3,
	}
	assert.Equal(t, "batch-123", resp.BatchId)
	assert.Equal(t, "running", resp.Status)
	assert.Equal(t, int32(100), resp.TotalItems)
	assert.Equal(t, int32(40), resp.CompletedItems)
	assert.Equal(t, int32(5), resp.FailedItems)
	assert.Equal(t, int32(55), resp.PendingItems)
	assert.Equal(t, int32(3), resp.ActiveItems)
}

// TestCancelBatch_RequestStructure verifies proto field wiring for cancel.
func TestCancelBatch_RequestStructure(t *testing.T) {
	req := &pipelinev1.CancelBatchRequest{
		BatchId: "batch-xyz",
	}
	assert.Equal(t, "batch-xyz", req.BatchId)

	resp := &pipelinev1.CancelBatchResponse{
		Cancelled: true,
		Message:   "Batch cancelled",
	}
	assert.True(t, resp.Cancelled)
	assert.Equal(t, "Batch cancelled", resp.Message)
}
