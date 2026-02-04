package pipelineservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Tests for pipeline introspection handlers focus on validation logic.
// Database-dependent tests require integration test setup.

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error"
	return logging.NewLogger(cfg)
}

func TestDescribePipeline_Request(t *testing.T) {
	// Test that requests are properly structured
	req := &pipelinev1.DescribePipelineRequest{
		Stage: "triage",
	}
	assert.Equal(t, "triage", req.Stage)
}

func TestGetPrompt_Validation(t *testing.T) {
	t.Run("missing stage", func(t *testing.T) {
		svc := NewService(nil, testLogger(), nil, nil, "default")
		_, err := svc.GetPrompt(context.Background(), &pipelinev1.GetPromptRequest{})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.GetPromptRequest{Stage: "triage", Version: 2}
		assert.Equal(t, "triage", req.Stage)
		assert.Equal(t, int32(2), req.Version)
	})
}

func TestUpdatePrompt_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing stage", func(t *testing.T) {
		_, err := svc.UpdatePrompt(context.Background(), &pipelinev1.UpdatePromptRequest{Content: "test"})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("missing content", func(t *testing.T) {
		_, err := svc.UpdatePrompt(context.Background(), &pipelinev1.UpdatePromptRequest{Stage: "triage"})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.UpdatePromptRequest{
			Stage:       "triage",
			Content:     "new prompt",
			Description: "test change",
		}
		assert.Equal(t, "triage", req.Stage)
		assert.Equal(t, "new prompt", req.Content)
	})
}

func TestRollbackPrompt_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing stage", func(t *testing.T) {
		_, err := svc.RollbackPrompt(context.Background(), &pipelinev1.RollbackPromptRequest{Version: 1})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("missing version", func(t *testing.T) {
		_, err := svc.RollbackPrompt(context.Background(), &pipelinev1.RollbackPromptRequest{Stage: "triage"})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("negative version", func(t *testing.T) {
		_, err := svc.RollbackPrompt(context.Background(), &pipelinev1.RollbackPromptRequest{Stage: "triage", Version: -1})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.RollbackPromptRequest{Stage: "triage", Version: 2}
		assert.Equal(t, "triage", req.Stage)
		assert.Equal(t, int32(2), req.Version)
	})
}

func TestGetSourceHistory_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing source_id", func(t *testing.T) {
		_, err := svc.GetSourceHistory(context.Background(), &pipelinev1.GetSourceHistoryRequest{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("negative source_id", func(t *testing.T) {
		_, err := svc.GetSourceHistory(context.Background(), &pipelinev1.GetSourceHistoryRequest{SourceId: -1})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.GetSourceHistoryRequest{SourceId: 123, Stage: "triage"}
		assert.Equal(t, int64(123), req.SourceId)
		assert.Equal(t, "triage", req.Stage)
	})
}

func TestReprocessDryRun_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing stage", func(t *testing.T) {
		_, err := svc.ReprocessDryRun(context.Background(), &pipelinev1.ReprocessDryRunRequest{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.ReprocessDryRunRequest{
			Stage:     "triage",
			SourceTag: "test-tag",
			SourceIds: []int64{1, 2, 3},
		}
		assert.Equal(t, "triage", req.Stage)
		assert.Equal(t, "test-tag", req.SourceTag)
		assert.Len(t, req.SourceIds, 3)
	})
}
