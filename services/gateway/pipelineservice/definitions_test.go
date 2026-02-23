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

func TestGetPipelineDefinition_Validation(t *testing.T) {
	t.Run("missing pipeline name", func(t *testing.T) {
		svc := NewService(nil, testLogger(), nil, nil, "default")
		_, err := svc.GetPipelineDefinition(context.Background(), &pipelinev1.GetPipelineDefinitionRequest{
			TenantId: "test-tenant",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "pipeline name is required")
	})
}

func TestUpdatePipelineStageConfig_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing pipeline name", func(t *testing.T) {
		_, err := svc.UpdatePipelineStageConfig(context.Background(), &pipelinev1.UpdatePipelineStageConfigRequest{
			Stage: "triage",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "pipeline name is required")
	})

	t.Run("missing stage name", func(t *testing.T) {
		_, err := svc.UpdatePipelineStageConfig(context.Background(), &pipelinev1.UpdatePipelineStageConfigRequest{
			Pipeline: "standard",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "stage name is required")
	})
}

func TestCreatePipelineDefinition_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing pipeline name", func(t *testing.T) {
		_, err := svc.CreatePipelineDefinition(context.Background(), &pipelinev1.CreatePipelineDefinitionRequest{
			TenantId: "test-tenant",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "pipeline name is required")
	})
}

func TestComparePipelineRuns_Validation(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	t.Run("missing stage", func(t *testing.T) {
		_, err := svc.ComparePipelineRuns(context.Background(), &pipelinev1.ComparePipelineRunsRequest{
			TenantId: "test-tenant",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "stage is required")
	})
}

func TestAuditPipelineCompleteness_NoDB(t *testing.T) {
	svc := NewService(nil, testLogger(), nil, nil, "default")

	_, err := svc.AuditPipelineCompleteness(context.Background(), &pipelinev1.AuditPipelineCompletenessRequest{
		TenantId: "test-tenant",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestStageDefToProto(t *testing.T) {
	modelOverride := "qwen2.5:7b"
	promptOverride := 3

	sd := pipeline.StageDefinition{
		Stage:          "triage",
		StageOrder:     1,
		Enabled:        true,
		ModelOverride:  &modelOverride,
		PromptOverride: &promptOverride,
		SkipWhenLow:    false,
		Optional:       false,
		TimeoutSeconds: 60,
	}

	proto := stageDefToProto(sd)

	assert.Equal(t, "triage", proto.Stage)
	assert.Equal(t, int32(1), proto.StageOrder)
	assert.True(t, proto.Enabled)
	assert.Equal(t, "qwen2.5:7b", proto.ModelOverride)
	assert.Equal(t, int32(3), proto.PromptOverride)
	assert.False(t, proto.SkipWhenLow)
	assert.False(t, proto.Optional)
	assert.Equal(t, int32(60), proto.TimeoutSeconds)
}

func TestStageDefToProto_NilOverrides(t *testing.T) {
	sd := pipeline.StageDefinition{
		Stage:      "embed",
		StageOrder: 8,
		Enabled:    true,
	}

	proto := stageDefToProto(sd)

	assert.Equal(t, "", proto.ModelOverride)
	assert.Equal(t, int32(0), proto.PromptOverride)
}

func TestPipelineDefToProto(t *testing.T) {
	def := pipeline.PipelineDefinition{
		Pipeline: "standard",
		Stages: []pipeline.StageDefinition{
			{Stage: "parse", StageOrder: 0, Enabled: true, TimeoutSeconds: 30},
			{Stage: "triage", StageOrder: 1, Enabled: true, TimeoutSeconds: 60},
		},
	}

	proto := pipelineDefToProto(def)

	assert.Equal(t, "standard", proto.Pipeline)
	require.Len(t, proto.Stages, 2)
	assert.Equal(t, "parse", proto.Stages[0].Stage)
	assert.Equal(t, "triage", proto.Stages[1].Stage)
}
