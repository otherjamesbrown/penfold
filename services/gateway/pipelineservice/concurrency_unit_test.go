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

// TestGetConcurrencyConfig_NilDB tests that nil DB returns error.
func TestGetConcurrencyConfig_NilDB(t *testing.T) {
	ctx := context.Background()

	// Service with nil DB
	svc := NewService(nil, testLogger(), nil, nil, "default")

	req := &pipelinev1.GetConcurrencyConfigRequest{}
	resp, err := svc.GetConcurrencyConfig(ctx, req)

	// Expected: Internal error due to nil DB
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// TestSetConcurrencyConfig_ValidatesMaxConcurrent tests validation of max_concurrent.
func TestSetConcurrencyConfig_ValidatesMaxConcurrent(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	testCases := []struct {
		name          string
		maxConcurrent int32
		updatedBy     string
		expectedError string
	}{
		{
			name:          "zero_max_concurrent",
			maxConcurrent: 0,
			updatedBy:     "test",
			expectedError: "max_concurrent must be >= 1",
		},
		{
			name:          "negative_max_concurrent",
			maxConcurrent: -1,
			updatedBy:     "test",
			expectedError: "max_concurrent must be >= 1",
		},
		{
			name:          "empty_updated_by",
			maxConcurrent: 5,
			updatedBy:     "",
			expectedError: "updated_by is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &pipelinev1.SetConcurrencyConfigRequest{
				MaxConcurrent: tc.maxConcurrent,
				UpdatedBy:     tc.updatedBy,
			}

			resp, err := svc.SetConcurrencyConfig(ctx, req)

			require.Error(t, err)
			assert.Nil(t, resp)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
			assert.Contains(t, st.Message(), tc.expectedError)
		})
	}
}

// TestSetConcurrencyConfig_ValidInput tests that valid input is accepted.
func TestSetConcurrencyConfig_ValidInput(t *testing.T) {
	ctx := context.Background()

	// Service with nil DB will fail at DB access, but validation should pass
	svc := NewService(nil, testLogger(), nil, nil, "default")

	req := &pipelinev1.SetConcurrencyConfigRequest{
		MaxConcurrent: 5,
		UpdatedBy:     "test-user",
	}

	resp, err := svc.SetConcurrencyConfig(ctx, req)

	// Expected: Error due to nil DB, but after validation passes
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	// Should be Unavailable (DB not available), not InvalidArgument (validation failed)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestListPendingSources_NilDB tests that nil DB returns error.
func TestListPendingSources_NilDB(t *testing.T) {
	ctx := context.Background()

	// Service with nil DB
	svc := NewService(nil, testLogger(), nil, nil, "default")

	req := &pipelinev1.ListPendingSourcesRequest{
		Limit: 10,
	}

	resp, err := svc.ListPendingSources(ctx, req)

	// Expected: Unavailable error due to nil DB
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestListPendingSources_RequestStructure tests that request proto has expected fields.
func TestListPendingSources_RequestStructure(t *testing.T) {
	req := &pipelinev1.ListPendingSourcesRequest{
		Limit:     50,
		Offset:    10,
		SourceTag: "gmail-backup",
	}

	// Verify proto fields exist and are accessible
	assert.Equal(t, int32(50), req.Limit)
	assert.Equal(t, int32(10), req.Offset)
	assert.Equal(t, "gmail-backup", req.SourceTag)
}

// TestListPendingSources_ResponseStructure tests that response proto has expected fields.
func TestListPendingSources_ResponseStructure(t *testing.T) {
	resp := &pipelinev1.ListPendingSourcesResponse{
		Sources: []*pipelinev1.PendingSourceSummary{
			{
				Id:          123,
				ContentType: "email",
				SourceTag:   "gmail-backup",
				SizeBytes:   4096,
				ExternalId:  "msg-123",
			},
		},
		TotalCount: 100,
	}

	// Verify proto fields exist and are accessible
	assert.Len(t, resp.Sources, 1)
	assert.Equal(t, int64(123), resp.Sources[0].Id)
	assert.Equal(t, "email", resp.Sources[0].ContentType)
	assert.Equal(t, "gmail-backup", resp.Sources[0].SourceTag)
	assert.Equal(t, int64(4096), resp.Sources[0].SizeBytes)
	assert.Equal(t, "msg-123", resp.Sources[0].ExternalId)
	assert.Equal(t, int64(100), resp.TotalCount)
}

// TestGetConcurrencyConfig_ResponseStructure tests that response proto has expected fields.
func TestGetConcurrencyConfig_ResponseStructure(t *testing.T) {
	resp := &pipelinev1.GetConcurrencyConfigResponse{
		MaxConcurrent: 5,
		InFlightCount: 2,
		PendingCount:  10,
	}

	// Verify proto fields exist and are accessible
	assert.Equal(t, int32(5), resp.MaxConcurrent)
	assert.Equal(t, int32(2), resp.InFlightCount)
	assert.Equal(t, int64(10), resp.PendingCount)
}

// TestSetConcurrencyConfig_ResponseStructure tests that response proto has expected fields.
func TestSetConcurrencyConfig_ResponseStructure(t *testing.T) {
	resp := &pipelinev1.SetConcurrencyConfigResponse{
		MaxConcurrent: 10,
		PreviousValue: 5,
		Message:       "Updated max_concurrent from 5 to 10",
	}

	// Verify proto fields exist and are accessible
	assert.Equal(t, int32(10), resp.MaxConcurrent)
	assert.Equal(t, int32(5), resp.PreviousValue)
	assert.Equal(t, "Updated max_concurrent from 5 to 10", resp.Message)
}

// TestKickProcessing_ResponseHasNewFields tests that KickProcessingResponse has new concurrency fields.
func TestKickProcessing_ResponseHasNewFields(t *testing.T) {
	resp := &pipelinev1.KickProcessingResponse{
		QueuedCount:   3,
		Message:       "Started 3 workflows",
		InFlightCount: 2,
		PendingCount:  10,
		MaxConcurrent: 5,
	}

	// Verify all fields are accessible
	assert.Equal(t, int64(3), resp.QueuedCount)
	assert.Equal(t, "Started 3 workflows", resp.Message)
	assert.Equal(t, int32(2), resp.InFlightCount)
	assert.Equal(t, int64(10), resp.PendingCount)
	assert.Equal(t, int32(5), resp.MaxConcurrent)
}

// TestKickProcessing_NilTemporalClient tests that nil Temporal client returns error.
func TestKickProcessing_NilTemporalClient(t *testing.T) {
	ctx := context.Background()

	// Service with nil Temporal client
	svc := NewService(nil, testLogger(), nil, nil, "default")

	req := &pipelinev1.KickProcessingRequest{
		Limit: 10,
	}

	resp, err := svc.KickProcessing(ctx, req)

	// Expected: Unavailable error due to nil Temporal client
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Contains(t, st.Message(), "Temporal")
}
