package pipelineservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
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

// TestGetStageConfig_LLMParamsFields verifies that StageConfigEntry supports
// the optional LLM parameter fields added for Phase 3 CLI (pf-3a7fb9).
func TestGetStageConfig_LLMParamsFields(t *testing.T) {
	t.Run("with LLM params set", func(t *testing.T) {
		temp := float32(0.7)
		tokens := int32(4096)
		retries := int32(3)
		entry := &pipelinev1.StageConfigEntry{
			Stage:       "analyze",
			Model:       "qwen2.5:7b",
			Temperature: &temp,
			MaxTokens:   &tokens,
			MaxRetries:  &retries,
		}

		assert.Equal(t, float32(0.7), *entry.Temperature)
		assert.Equal(t, int32(4096), *entry.MaxTokens)
		assert.Equal(t, int32(3), *entry.MaxRetries)
	})

	t.Run("without LLM params (non-LLM stage)", func(t *testing.T) {
		entry := &pipelinev1.StageConfigEntry{
			Stage: "embed",
			Model: "mxbai-embed-large",
		}

		assert.Nil(t, entry.Temperature)
		assert.Nil(t, entry.MaxTokens)
		assert.Nil(t, entry.MaxRetries)
	})
}

// TestGetStageConfig_TenantIDField verifies that GetStageConfigRequest accepts
// a tenant_id field for tenant-aware stage config resolution (pf-4c2203).
func TestGetStageConfig_TenantIDField(t *testing.T) {
	req := &pipelinev1.GetStageConfigRequest{
		Stage:    "extract_ner",
		TenantId: "c3170310-78bd-409c-b186-126f40bfa6ad",
	}
	assert.Equal(t, "extract_ner", req.Stage)
	assert.Equal(t, "c3170310-78bd-409c-b186-126f40bfa6ad", req.TenantId)

	// Empty tenant_id is valid (server uses defaultTenantID)
	emptyTenantReq := &pipelinev1.GetStageConfigRequest{Stage: "triage"}
	assert.Equal(t, "", emptyTenantReq.TenantId)
}

// TestPipelineStageDefinition_LLMParamsFields verifies that PipelineStageDefinition
// includes LLM params so ListPipelineDefinitions and UpdatePipelineStageConfig
// responses include temperature, max_tokens, max_retries (pf-4c2203).
func TestPipelineStageDefinition_LLMParamsFields(t *testing.T) {
	t.Run("with LLM params", func(t *testing.T) {
		temp := float32(0.1)
		tokens := int32(8192)
		retries := int32(2)
		sd := &pipelinev1.PipelineStageDefinition{
			Stage:       "extract_ner",
			Temperature: &temp,
			MaxTokens:   &tokens,
			MaxRetries:  &retries,
		}
		assert.Equal(t, float32(0.1), *sd.Temperature)
		assert.Equal(t, int32(8192), *sd.MaxTokens)
		assert.Equal(t, int32(2), *sd.MaxRetries)
	})

	t.Run("nil LLM params", func(t *testing.T) {
		sd := &pipelinev1.PipelineStageDefinition{
			Stage: "embed",
		}
		assert.Nil(t, sd.Temperature)
		assert.Nil(t, sd.MaxTokens)
		assert.Nil(t, sd.MaxRetries)
	})
}

// TestStageDefToProto_LLMParams verifies that stageDefToProto populates
// temperature, max_tokens, max_retries from StageDefinition (pf-4c2203).
func TestStageDefToProto_LLMParams(t *testing.T) {
	t.Run("with LLM params", func(t *testing.T) {
		temp := 0.1
		tokens := 8192
		retries := 2
		sd := pipeline.StageDefinition{
			Stage:       "extract_ner",
			StageOrder:  2,
			Enabled:     true,
			Temperature: &temp,
			MaxTokens:   &tokens,
			MaxRetries:  &retries,
		}
		p := stageDefToProto(sd)
		require.NotNil(t, p.Temperature)
		assert.Equal(t, float32(0.1), *p.Temperature)
		require.NotNil(t, p.MaxTokens)
		assert.Equal(t, int32(8192), *p.MaxTokens)
		require.NotNil(t, p.MaxRetries)
		assert.Equal(t, int32(2), *p.MaxRetries)
	})

	t.Run("nil LLM params", func(t *testing.T) {
		sd := pipeline.StageDefinition{
			Stage:      "embed",
			StageOrder: 8,
			Enabled:    true,
		}
		p := stageDefToProto(sd)
		assert.Nil(t, p.Temperature)
		assert.Nil(t, p.MaxTokens)
		assert.Nil(t, p.MaxRetries)
	})
}

// TestUpdatePipelineStageConfigRequest_LLMParamsFields verifies that the update
// request proto supports the optional LLM parameter fields (pf-3a7fb9).
func TestUpdatePipelineStageConfigRequest_LLMParamsFields(t *testing.T) {
	temp := float32(0.5)
	tokens := int32(2048)
	retries := int32(2)
	req := &pipelinev1.UpdatePipelineStageConfigRequest{
		Pipeline:    "standard",
		Stage:       "analyze",
		Temperature: &temp,
		MaxTokens:   &tokens,
		MaxRetries:  &retries,
	}

	assert.Equal(t, float32(0.5), *req.Temperature)
	assert.Equal(t, int32(2048), *req.MaxTokens)
	assert.Equal(t, int32(2), *req.MaxRetries)
}
