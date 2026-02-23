package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStageConfigMap_NilDef(t *testing.T) {
	result := buildStageConfigMap(nil)
	assert.Nil(t, result)
}

func TestBuildStageConfigMap_NotFound(t *testing.T) {
	def := &FetchPipelineDefinitionOutput{Found: false}
	result := buildStageConfigMap(def)
	assert.Nil(t, result)
}

func TestBuildStageConfigMap_EmptyStages(t *testing.T) {
	def := &FetchPipelineDefinitionOutput{Found: true, Stages: nil}
	result := buildStageConfigMap(def)
	assert.Nil(t, result)
}

func TestBuildStageConfigMap_ValidDef(t *testing.T) {
	def := &FetchPipelineDefinitionOutput{
		Found: true,
		Stages: []PipelineStageConfig{
			{Stage: "parse", StageOrder: 0, Enabled: true, SkipWhenLow: false, Optional: false, TimeoutSeconds: 30},
			{Stage: "triage", StageOrder: 1, Enabled: true, SkipWhenLow: false, Optional: false, TimeoutSeconds: 60},
			{Stage: "extract_ner", StageOrder: 2, Enabled: true, SkipWhenLow: true, Optional: false, TimeoutSeconds: 120},
			{Stage: "analyze", StageOrder: 6, Enabled: false, SkipWhenLow: true, Optional: true, TimeoutSeconds: 180},
		},
	}

	result := buildStageConfigMap(def)
	require.NotNil(t, result)
	assert.Len(t, result, 4)

	parse, ok := result["parse"]
	require.True(t, ok)
	assert.Equal(t, 0, parse.StageOrder)
	assert.True(t, parse.Enabled)
	assert.False(t, parse.SkipWhenLow)

	analyze, ok := result["analyze"]
	require.True(t, ok)
	assert.False(t, analyze.Enabled)
	assert.True(t, analyze.Optional)
}

func TestIsStageEnabled_NilMap(t *testing.T) {
	// Nil map = fallback mode, all stages enabled
	assert.True(t, isStageEnabled(nil, "parse"))
	assert.True(t, isStageEnabled(nil, "triage"))
	assert.True(t, isStageEnabled(nil, "nonexistent"))
}

func TestIsStageEnabled_StageMissing(t *testing.T) {
	m := map[string]PipelineStageConfig{
		"parse": {Stage: "parse", Enabled: true},
	}
	// Stage not in map = enabled by default
	assert.True(t, isStageEnabled(m, "unknown_stage"))
}

func TestIsStageEnabled_StageDisabled(t *testing.T) {
	m := map[string]PipelineStageConfig{
		"parse":   {Stage: "parse", Enabled: true},
		"analyze": {Stage: "analyze", Enabled: false},
	}

	assert.True(t, isStageEnabled(m, "parse"))
	assert.False(t, isStageEnabled(m, "analyze"))
}

func TestPipelineStageConfig_Struct(t *testing.T) {
	cfg := PipelineStageConfig{
		Stage:          "extract_ner",
		StageOrder:     2,
		Enabled:        true,
		SkipWhenLow:    true,
		Optional:       false,
		TimeoutSeconds: 120,
		ModelOverride:  "qwen2.5:7b",
	}

	assert.Equal(t, "extract_ner", cfg.Stage)
	assert.Equal(t, 2, cfg.StageOrder)
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.SkipWhenLow)
	assert.False(t, cfg.Optional)
	assert.Equal(t, 120, cfg.TimeoutSeconds)
	assert.Equal(t, "qwen2.5:7b", cfg.ModelOverride)
}

func TestFetchPipelineDefinitionInput_Struct(t *testing.T) {
	input := FetchPipelineDefinitionInput{
		TenantID: "tenant-1",
		Pipeline: "standard",
	}
	assert.Equal(t, "tenant-1", input.TenantID)
	assert.Equal(t, "standard", input.Pipeline)
}

func TestPipelineInput_HasPipelineField(t *testing.T) {
	input := PipelineInput{
		TenantID: "tenant-1",
		SourceID: 42,
		Pipeline: "transcript",
	}
	assert.Equal(t, "transcript", input.Pipeline)
}
