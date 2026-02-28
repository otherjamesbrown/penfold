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

func TestBuildPipelineDefinitionMetadata(t *testing.T) {
	def := &FetchPipelineDefinitionOutput{
		Found: true,
		Stages: []PipelineStageConfig{
			{Stage: "parse", Enabled: true},
			{Stage: "triage", Enabled: true},
			{Stage: "extract_ner", Enabled: true, ModelOverride: "qwen2.5:14b"},
			{Stage: "analyze", Enabled: false, ModelOverride: "gemini-2.5-pro"},
			{Stage: "embed", Enabled: true},
		},
	}

	metadata := buildPipelineDefinitionMetadata("transcript", def)

	assert.Equal(t, "transcript", metadata["pipeline_name"])

	stages, ok := metadata["pipeline_stages"].([]string)
	require.True(t, ok, "pipeline_stages should be a string slice")
	assert.Equal(t, []string{"parse", "triage", "extract_ner", "embed"}, stages, "only enabled stages")

	overrides, ok := metadata["model_overrides"].(map[string]string)
	require.True(t, ok, "model_overrides should be a map")
	assert.Equal(t, "qwen2.5:14b", overrides["extract_ner"])
	assert.Equal(t, "gemini-2.5-pro", overrides["analyze"], "disabled stages with overrides still appear")
	assert.Len(t, overrides, 2)
}

func TestBuildPipelineDefinitionMetadata_NoOverrides(t *testing.T) {
	def := &FetchPipelineDefinitionOutput{
		Found: true,
		Stages: []PipelineStageConfig{
			{Stage: "parse", Enabled: true},
			{Stage: "triage", Enabled: true},
		},
	}

	metadata := buildPipelineDefinitionMetadata("standard", def)

	assert.Equal(t, "standard", metadata["pipeline_name"])
	_, hasOverrides := metadata["model_overrides"]
	assert.False(t, hasOverrides, "model_overrides should be omitted when empty")
}

func TestPipelineInput_HasPipelineField(t *testing.T) {
	input := PipelineInput{
		TenantID: "tenant-1",
		SourceID: 42,
		Pipeline: "transcript",
	}
	assert.Equal(t, "transcript", input.Pipeline)
}

func TestStageInPipeline_NilMap(t *testing.T) {
	assert.True(t, stageInPipeline(nil, "parse"))
	assert.True(t, stageInPipeline(nil, "anything"))
}

func TestStageInPipeline_StageMissing(t *testing.T) {
	m := map[string]PipelineStageConfig{
		"parse":  {Stage: "parse", Enabled: true},
		"triage": {Stage: "triage", Enabled: true},
	}
	assert.False(t, stageInPipeline(m, "analyze"), "stage not in pipeline should be false")
	assert.False(t, stageInPipeline(m, "extract_assertions"), "stage not in pipeline should be false")
}

func TestStageInPipeline_StagePresent(t *testing.T) {
	m := map[string]PipelineStageConfig{
		"parse":       {Stage: "parse", Enabled: true},
		"extract_ner": {Stage: "extract_ner", Enabled: true},
		"analyze":     {Stage: "analyze", Enabled: false},
	}
	assert.True(t, stageInPipeline(m, "parse"))
	assert.True(t, stageInPipeline(m, "extract_ner"))
	assert.False(t, stageInPipeline(m, "analyze"), "disabled stage should be false")
}

// TestExtractGate_SemanticOnlyPipeline verifies that the extraction block runs
// for pipelines with extract_semantic but not extract_ner (e.g. newsletter).
// Bug fix pf-9fd0fd: extraction gate was too narrow, only checking extract_ner.
func TestExtractGate_SemanticOnlyPipeline(t *testing.T) {
	// Newsletter pipeline: has extract_semantic but not extract_ner
	m := map[string]PipelineStageConfig{
		"parse":            {Stage: "parse", Enabled: true},
		"triage":           {Stage: "triage", Enabled: true},
		"extract_semantic": {Stage: "extract_semantic", Enabled: true},
		"persist":          {Stage: "persist", Enabled: true},
		"embed":            {Stage: "embed", Enabled: true},
	}

	// The extraction gate should open when either extract stage is present
	nerInPipeline := stageInPipeline(m, "extract_ner")
	semanticInPipeline := stageInPipeline(m, "extract_semantic")
	extractionShouldRun := nerInPipeline || semanticInPipeline

	assert.False(t, nerInPipeline, "extract_ner not in newsletter pipeline")
	assert.True(t, semanticInPipeline, "extract_semantic is in newsletter pipeline")
	assert.True(t, extractionShouldRun, "extraction block should run for semantic-only pipeline")
}

// TestExtractGate_NEROnlyPipeline verifies the extraction gate for
// pipelines with extract_ner but not extract_semantic (e.g. notification).
func TestExtractGate_NEROnlyPipeline(t *testing.T) {
	m := map[string]PipelineStageConfig{
		"parse":       {Stage: "parse", Enabled: true},
		"triage":      {Stage: "triage", Enabled: true},
		"extract_ner": {Stage: "extract_ner", Enabled: true},
		"persist":     {Stage: "persist", Enabled: true},
		"embed":       {Stage: "embed", Enabled: true},
	}

	nerInPipeline := stageInPipeline(m, "extract_ner")
	semanticInPipeline := stageInPipeline(m, "extract_semantic")
	extractionShouldRun := nerInPipeline || semanticInPipeline

	assert.True(t, nerInPipeline, "extract_ner is in notification pipeline")
	assert.False(t, semanticInPipeline, "extract_semantic not in notification pipeline")
	assert.True(t, extractionShouldRun, "extraction block should run for NER-only pipeline")
}

// TestPersistGate_IndependentOfAnalyze verifies that persist can run
// independently of the analyze stage for lightweight pipelines.
// Bug fix pf-9fd0fd: persist was nested inside analyze gate.
func TestPersistGate_IndependentOfAnalyze(t *testing.T) {
	// Notification pipeline: has persist but not analyze
	m := map[string]PipelineStageConfig{
		"parse":       {Stage: "parse", Enabled: true},
		"triage":      {Stage: "triage", Enabled: true},
		"extract_ner": {Stage: "extract_ner", Enabled: true},
		"persist":     {Stage: "persist", Enabled: true},
		"embed":       {Stage: "embed", Enabled: true},
	}

	assert.False(t, stageInPipeline(m, "analyze"), "analyze not in notification pipeline")
	assert.True(t, stageInPipeline(m, "persist"), "persist should be independently gatable")
}

// TestPersistGate_StandardPipeline verifies that persist still runs
// for standard pipelines that have both analyze and persist.
func TestPersistGate_StandardPipeline(t *testing.T) {
	m := map[string]PipelineStageConfig{
		"parse":               {Stage: "parse", Enabled: true},
		"triage":              {Stage: "triage", Enabled: true},
		"extract_ner":         {Stage: "extract_ner", Enabled: true},
		"extract_assertions":  {Stage: "extract_assertions", Enabled: true},
		"extract_semantic":    {Stage: "extract_semantic", Enabled: true},
		"resolve":             {Stage: "resolve", Enabled: true},
		"analyze":             {Stage: "analyze", Enabled: true},
		"persist":             {Stage: "persist", Enabled: true},
		"embed":               {Stage: "embed", Enabled: true},
	}

	assert.True(t, stageInPipeline(m, "analyze"), "analyze in standard pipeline")
	assert.True(t, stageInPipeline(m, "persist"), "persist in standard pipeline")
}
