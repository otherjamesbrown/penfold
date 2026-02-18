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

func TestGetTimeoutConfig_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("nil db returns error", func(t *testing.T) {
		_, err := svc.GetTimeoutConfig(context.Background(), &pipelinev1.GetTimeoutConfigRequest{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.GetTimeoutConfigRequest{Key: "timeout.activity"}
		assert.Equal(t, "timeout.activity", req.Key)
	})

	t.Run("valid request with empty key", func(t *testing.T) {
		req := &pipelinev1.GetTimeoutConfigRequest{}
		assert.Equal(t, "", req.Key)
	})
}

func TestUpdateTimeoutConfig_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing key", func(t *testing.T) {
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Value:     "30s",
			UpdatedBy: "test",
			Reason:    "test",
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("missing value", func(t *testing.T) {
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Key:       "timeout.ai_client.request",
			UpdatedBy: "test",
			Reason:    "test",
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("missing updated_by", func(t *testing.T) {
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Key:    "timeout.ai_client.request",
			Value:  "30s",
			Reason: "test",
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("missing reason", func(t *testing.T) {
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Key:       "timeout.ai_client.request",
			Value:     "30s",
			UpdatedBy: "test",
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("invalid duration value", func(t *testing.T) {
		// NOTE: With nil DB, this test now returns Unavailable (DB check happens before parsing).
		// This is correct behavior after fix for pf-881da1. To test invalid duration validation,
		// use an integration test with a real DB and a duration-type config entry.
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Key:       "timeout.ai_client.request",
			Value:     "invalid",
			UpdatedBy: "test",
			Reason:    "test",
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		// After pf-881da1 fix: DB check happens first, so nil DB returns Unavailable
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("nil db returns error", func(t *testing.T) {
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Key:       "timeout.ai_client.request",
			Value:     "30s",
			UpdatedBy: "test",
			Reason:    "test",
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("valid request structure", func(t *testing.T) {
		req := &pipelinev1.UpdateTimeoutConfigRequest{
			Key:       "timeout.ai_client.request",
			Value:     "180s",
			UpdatedBy: "test-user",
			Reason:    "Extended for longer requests",
		}
		assert.Equal(t, "timeout.ai_client.request", req.Key)
		assert.Equal(t, "180s", req.Value)
		assert.Equal(t, "test-user", req.UpdatedBy)
		assert.Equal(t, "Extended for longer requests", req.Reason)
	})

	// BUG REPRODUCTION TEST [pf-881da1]
	// This test reproduces the bug where UpdateTimeoutConfig unconditionally
	// parses all values as durations before checking value_type from the DB.
	// When the CLI sends a model config value like "qwen2.5:7b" via
	// UpdateTimeoutConfig with key "model.stage.triage", the handler rejects
	// it with "invalid duration value" because it tries to parse the model
	// name as a Go duration.
	//
	// Expected behavior: The handler should check value_type from the DB first,
	// and only parse as duration when value_type=="duration".
	//
	// This test SHOULD FAIL with current code (proving we catch the bug).
	t.Run("model config value should not be parsed as duration [pf-881da1]", func(t *testing.T) {
		// This test currently FAILS because the handler unconditionally parses
		// the value as a duration at line ~1197, before querying value_type.
		// The correct behavior would be to accept this model name value.
		_, err := svc.UpdateTimeoutConfig(context.Background(), &pipelinev1.UpdateTimeoutConfigRequest{
			Key:       "model.stage.triage",
			Value:     "qwen2.5:7b",
			UpdatedBy: "test",
			Reason:    "test model config",
		})

		// BUG: Currently fails with InvalidArgument "invalid duration value"
		// EXPECTED: Should fail with Unavailable (nil db) NOT InvalidArgument (duration parse)
		// Once fixed, this should pass the duration parse and fail on nil db check instead.
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)

		// BUG ASSERTION: Current code returns InvalidArgument because it tries
		// to parse "qwen2.5:7b" as a duration before checking value_type.
		// After the fix, this should return Unavailable (nil db) instead.
		//
		// This assertion SHOULD FAIL with current code, proving the bug exists.
		assert.Equal(t, codes.Unavailable, st.Code(),
			"Expected Unavailable (nil db), but got %v with message: %q. "+
			"This indicates the handler is incorrectly parsing the value as a duration "+
			"before checking value_type from the database. "+
			"The handler should check value_type first and only parse as duration "+
			"when value_type=='duration'.", st.Code(), st.Message())
	})
}
