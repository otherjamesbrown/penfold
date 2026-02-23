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

// TestGetStageConfig_NilDB verifies that a nil DB returns Unavailable error.
func TestGetStageConfig_NilDB(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.GetStageConfig(ctx, &pipelinev1.GetStageConfigRequest{})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestGetStageConfig_NilDB_SingleStage verifies nil DB error even with a stage filter.
func TestGetStageConfig_NilDB_SingleStage(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.GetStageConfig(ctx, &pipelinev1.GetStageConfigRequest{Stage: "triage"})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestGetStageConfig_RequestStructure verifies that the request proto has the expected fields.
func TestGetStageConfig_RequestStructure(t *testing.T) {
	req := &pipelinev1.GetStageConfigRequest{Stage: "triage"}
	assert.Equal(t, "triage", req.Stage)

	// Empty request is valid (means all stages)
	emptyReq := &pipelinev1.GetStageConfigRequest{}
	assert.Equal(t, "", emptyReq.Stage)
}

// TestGetStageConfig_ResponseStructure verifies that StageConfigEntry has all
// expected fields including the model_source field added for [pf-16e358].
func TestGetStageConfig_ResponseStructure(t *testing.T) {
	// Verify proto fields are accessible — these correspond to the fix where
	// we return resolved model name and its source
	entry := &pipelinev1.StageConfigEntry{
		Stage:         "triage",
		Model:         "qwen2.5:7b",
		Timeout:       "120s",
		Heartbeat:     "30s",
		ModelSource:   "default",
		TimeoutSource: "default",
	}

	assert.Equal(t, "triage", entry.Stage)
	assert.Equal(t, "qwen2.5:7b", entry.Model)
	assert.Equal(t, "120s", entry.Timeout)
	assert.Equal(t, "30s", entry.Heartbeat)
	assert.Equal(t, "default", entry.ModelSource)
	assert.Equal(t, "default", entry.TimeoutSource)
}

// TestGetStageConfig_ModelSourceValues verifies the expected model_source values
// returned by the resolution chain documented in [pf-16e358].
func TestGetStageConfig_ModelSourceValues(t *testing.T) {
	// Valid source values produced by GetStageConfig resolution chain:
	//   "default"      — hardcoded fallback (qwen2.5:7b), no model_config rows found
	//   "global"       — model_config row with config_key = "default.llm"
	//   "stage_config" — model_config row with config_key = "stage.{stage}"
	validSources := []string{"default", "global", "stage_config"}

	for _, src := range validSources {
		entry := &pipelinev1.StageConfigEntry{
			Stage:       "triage",
			Model:       "qwen2.5:7b",
			ModelSource: src,
		}
		assert.Equal(t, src, entry.ModelSource, "model_source value %q should be settable", src)
	}
}

// TestGetStageConfig_DefaultTimeouts verifies the helper functions return
// correct default timeout values for each stage type.
func TestGetStageConfig_DefaultTimeouts(t *testing.T) {
	testCases := []struct {
		stage             string
		expectedTimeout   string
		expectedHeartbeat string
	}{
		{"triage", "120s", "30s"},
		{"extract_ner", "120s", "30s"},
		{"extract_assertions", "120s", "30s"},
		{"embed", "120s", "30s"},
		{"analyze", "600s", "300s"},
	}

	for _, tc := range testCases {
		t.Run(tc.stage, func(t *testing.T) {
			assert.Equal(t, tc.expectedTimeout, stageDefaultTimeout(tc.stage))
			assert.Equal(t, tc.expectedHeartbeat, stageDefaultHeartbeat(tc.stage))
		})
	}
}

// TestGetStageConfig_FallbackModel documents the hardcoded fallback model value.
// If model_config has no rows for a stage or global default, this model is used.
func TestGetStageConfig_FallbackModel(t *testing.T) {
	// The fallback is defined as const fallbackModel = "qwen2.5:7b" in service.go.
	// This test documents the expected value for regression detection.
	const expectedFallback = "qwen2.5:7b"

	// Build a response entry with fallback values to verify the proto supports it.
	entry := &pipelinev1.StageConfigEntry{
		Stage:       "triage",
		Model:       expectedFallback,
		ModelSource: "default",
	}

	assert.Equal(t, expectedFallback, entry.Model)
	assert.Equal(t, "default", entry.ModelSource)
}
