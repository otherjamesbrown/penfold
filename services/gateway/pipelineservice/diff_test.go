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

// TestDiffPipelineRuns_NoSourceID tests that missing source_id returns InvalidArgument.
// This test will FAIL because the handler is not implemented yet.
func TestDiffPipelineRuns_NoSourceID(t *testing.T) {
	ctx := context.Background()

	svc := NewService(nil, testLogger(), nil, nil, "default")

	req := &pipelinev1.DiffPipelineRunsRequest{
		SourceId: 0,
	}

	resp, err := svc.DiffPipelineRuns(ctx, req)

	// Expected: InvalidArgument error for missing source_id
	// Current: Will fail because DiffPipelineRuns is not implemented
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	// After implementation, this should be InvalidArgument
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "source_id")
}

// TestDiffPipelineRuns_RequestStructure tests that request proto has expected fields.
func TestDiffPipelineRuns_RequestStructure(t *testing.T) {
	runA := int64(100)
	runB := int64(101)

	req := &pipelinev1.DiffPipelineRunsRequest{
		SourceId: int64(123),
		RunIdA:   &runA,
		RunIdB:   &runB,
	}

	// Verify proto fields exist and are accessible
	assert.Equal(t, int64(123), req.SourceId)
	assert.NotNil(t, req.RunIdA)
	assert.Equal(t, int64(100), *req.RunIdA)
	assert.NotNil(t, req.RunIdB)
	assert.Equal(t, int64(101), *req.RunIdB)
}

// TestDiffPipelineRuns_ResponseStructure tests that response proto has expected fields.
func TestDiffPipelineRuns_ResponseStructure(t *testing.T) {
	resp := &pipelinev1.DiffPipelineRunsResponse{
		Diffs: []*pipelinev1.StageDiff{
			{
				Stage:      "extract_ner",
				Field:      "entities",
				OldValue:   "old",
				NewValue:   "new",
				ChangeType: pipelinev1.ChangeType_CHANGE_TYPE_MODIFIED,
			},
		},
	}

	// Verify proto fields exist and are accessible
	assert.Len(t, resp.Diffs, 1)
	assert.Equal(t, "extract_ner", resp.Diffs[0].Stage)
	assert.Equal(t, "entities", resp.Diffs[0].Field)
	assert.Equal(t, "old", resp.Diffs[0].OldValue)
	assert.Equal(t, "new", resp.Diffs[0].NewValue)
	assert.Equal(t, pipelinev1.ChangeType_CHANGE_TYPE_MODIFIED, resp.Diffs[0].ChangeType)
}

// TestStageDiff_ChangeTypes tests that all change type values are available.
func TestStageDiff_ChangeTypes(t *testing.T) {
	testCases := []struct {
		name      string
		changeType pipelinev1.ChangeType
	}{
		{"Unspecified", pipelinev1.ChangeType_CHANGE_TYPE_UNSPECIFIED},
		{"Added", pipelinev1.ChangeType_CHANGE_TYPE_ADDED},
		{"Removed", pipelinev1.ChangeType_CHANGE_TYPE_REMOVED},
		{"Modified", pipelinev1.ChangeType_CHANGE_TYPE_MODIFIED},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			diff := &pipelinev1.StageDiff{
				Stage:      "test_stage",
				Field:      "test_field",
				OldValue:   "old",
				NewValue:   "new",
				ChangeType: tc.changeType,
			}

			assert.Equal(t, tc.changeType, diff.ChangeType)
		})
	}
}

// TestDiffPipelineRuns_NilRepo tests that nil repository returns error.
// This documents the expected behavior when dependencies are not configured.
func TestDiffPipelineRuns_NilRepo(t *testing.T) {
	ctx := context.Background()

	// Service with nil repository
	svc := NewService(nil, testLogger(), nil, nil, "default")

	runA := int64(100)
	runB := int64(101)
	req := &pipelinev1.DiffPipelineRunsRequest{
		SourceId: int64(123),
		RunIdA:   &runA,
		RunIdB:   &runB,
	}

	resp, err := svc.DiffPipelineRuns(ctx, req)

	// Expected: Some error (Unavailable or Internal) due to nil repository
	// Current: Will fail because DiffPipelineRuns is not implemented (returns Unimplemented)
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	// After implementation with nil repo, should return Unavailable or Internal
	// For now, we just verify an error is returned
	assert.NotEqual(t, codes.OK, st.Code())
}

// TestDiffPipelineRuns_DefaultRunIDs tests behavior when run IDs not specified.
// This documents that the handler should use default logic to select runs.
func TestDiffPipelineRuns_DefaultRunIDs(t *testing.T) {
	ctx := context.Background()

	svc := NewService(nil, testLogger(), nil, nil, "default")

	req := &pipelinev1.DiffPipelineRunsRequest{
		SourceId: int64(123),
		// RunIdA and RunIdB not set - should use defaults (latest 2 runs)
	}

	resp, err := svc.DiffPipelineRuns(ctx, req)

	// Expected: Handler should call GetLatestRunIDs to get default runs
	// Current: Will fail because DiffPipelineRuns is not implemented
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	// After implementation, with nil repo should return Unavailable
	assert.NotEqual(t, codes.OK, st.Code())
}
