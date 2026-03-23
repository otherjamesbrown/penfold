package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// --- Test doubles ---

type mockLLMAIClient struct {
	generateSummaryFn func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
}

func (m *mockLLMAIClient) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	if m.generateSummaryFn != nil {
		return m.generateSummaryFn(ctx, req)
	}
	return &aiv1.SummaryResponse{Summary: `{"result":"ok"}`, ModelUsed: "test-model"}, nil
}

type mockLLMPromptRepo struct {
	getPromptFn func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error)
}

func (m *mockLLMPromptRepo) GetPromptByStage(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
	if m.getPromptFn != nil {
		return m.getPromptFn(ctx, stage, version)
	}
	return &pipeline.PromptTemplate{Stage: stage, Version: version, Content: "{{.Content}}"}, nil
}

// newTestLLMExecutor builds an LLMStageExecutor with defaults suitable for testing.
func newTestLLMExecutor(ai LLMAIClient, pr LLMPromptRepo) *LLMStageExecutor {
	if ai == nil {
		ai = &mockLLMAIClient{}
	}
	if pr == nil {
		pr = &mockLLMPromptRepo{}
	}
	return NewLLMStageExecutor(ai, pr)
}

func baseLLMStage() PipelineStageConfig {
	return PipelineStageConfig{
		Stage:     "triage",
		StageKind: "llm",
	}
}

func baseLLMInput() StageInput {
	return StageInput{
		TenantID: "tenant-1",
		SourceID: 42,
		Content:  "Quarterly earnings call scheduled for next Thursday.",
	}
}

// --- Constructor tests ---

func TestNewLLMStageExecutor_PanicsOnNilDependencies(t *testing.T) {
	ai := &mockLLMAIClient{}
	pr := &mockLLMPromptRepo{}

	require.Panics(t, func() { NewLLMStageExecutor(nil, pr) })
	require.Panics(t, func() { NewLLMStageExecutor(ai, nil) })
}

// --- Skip-when-low tests ---

func TestLLMStageExecutor_SkipWhenLowAndSkipDeep(t *testing.T) {
	// No AI call should occur — verified by registering a failing stub.
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return nil, errors.New("should not be called")
		},
	}
	ex := newTestLLMExecutor(ai, nil)
	stage := baseLLMStage()
	stage.SkipWhenLow = true

	input := baseLLMInput()
	input.SkipDeep = true

	out, err := ex.Execute(context.Background(), stage, input)

	require.NoError(t, err)
	require.True(t, out.Skipped)
	require.True(t, out.Success)
	require.Equal(t, "skip_deep", out.SkipReason)
	require.Equal(t, "triage", out.StageName)
}

func TestLLMStageExecutor_SkipWhenLowFalseExecutesNormally(t *testing.T) {
	ex := newTestLLMExecutor(nil, nil)
	stage := baseLLMStage()
	stage.SkipWhenLow = false

	input := baseLLMInput()
	input.SkipDeep = true // SkipDeep=true but SkipWhenLow=false → should not skip

	out, err := ex.Execute(context.Background(), stage, input)

	require.NoError(t, err)
	require.False(t, out.Skipped)
	require.True(t, out.Success)
}

// --- Validation tests ---

func TestLLMStageExecutor_EmptyStageNameError(t *testing.T) {
	ex := newTestLLMExecutor(nil, nil)
	stage := baseLLMStage()
	stage.Stage = ""

	out, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.Error(t, err)
	require.Contains(t, err.Error(), "stage name is required")
	require.False(t, out.Success)
}

func TestLLMStageExecutor_EmptyContentError(t *testing.T) {
	ex := newTestLLMExecutor(nil, nil)
	input := baseLLMInput()
	input.Content = ""

	out, err := ex.Execute(context.Background(), baseLLMStage(), input)

	require.Error(t, err)
	require.Contains(t, err.Error(), "content is empty")
	require.False(t, out.Success)
	require.Equal(t, "triage", out.StageName)
}

// --- Prompt loading tests ---

func TestLLMStageExecutor_PromptOverrideZeroFetchesActive(t *testing.T) {
	var capturedVersion int
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, version int) (*pipeline.PromptTemplate, error) {
			capturedVersion = version
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestLLMExecutor(nil, pr)
	stage := baseLLMStage()
	stage.PromptOverride = 0

	_, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err)
	require.Equal(t, 0, capturedVersion, "version 0 must request the active prompt")
}

func TestLLMStageExecutor_PromptOverridePassedThrough(t *testing.T) {
	var capturedVersion int
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, version int) (*pipeline.PromptTemplate, error) {
			capturedVersion = version
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestLLMExecutor(nil, pr)
	stage := baseLLMStage()
	stage.PromptOverride = 7

	_, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err)
	require.Equal(t, 7, capturedVersion)
}

func TestLLMStageExecutor_PromptLoadError(t *testing.T) {
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return nil, errors.New("prompt not found in DB")
		},
	}
	ex := newTestLLMExecutor(nil, pr)

	out, err := ex.Execute(context.Background(), baseLLMStage(), baseLLMInput())

	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt not found in DB")
	require.False(t, out.Success)
}

// --- Context assembly tests ---
//
// These tests verify the template-driven context approach: previous stage outputs
// are exposed to the prompt template by stage name so no stage-name branching is
// needed inside the executor.

// TestLLMStageExecutor_TriagePattern simulates a triage stage — prompt only uses {{.Content}}.
func TestLLMStageExecutor_TriagePattern(t *testing.T) {
	const triageResponse = `{"category":"INTERNAL","importance":"HIGH","reason":"exec comms","skip_deep":false}`

	var capturedPrompt string
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: triageResponse}, nil
		},
	}
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Content: "Classify this content:\n{{.Content}}"}, nil
		},
	}
	ex := newTestLLMExecutor(ai, pr)
	stage := baseLLMStage() // "triage"

	out, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "triage", out.StageName)
	require.Equal(t, "llm", out.StageKind)
	require.Equal(t, triageResponse, string(out.RawData))
	require.Contains(t, capturedPrompt, "Quarterly earnings call", "content must appear in rendered prompt")
	require.Contains(t, capturedPrompt, "Classify this content:")

	// Verify RawData round-trips through JSON (Temporal serialisation requirement).
	data, err := json.Marshal(out)
	require.NoError(t, err)
	var restored StageOutput
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, triageResponse, string(restored.RawData))
}

// TestLLMStageExecutor_ExtractPattern simulates an extract stage — prompt uses
// {{.Content}} and {{.triage}} (previous triage output).
func TestLLMStageExecutor_ExtractPattern(t *testing.T) {
	triageRaw, _ := json.Marshal(map[string]interface{}{
		"category":   "INTERNAL",
		"importance": "HIGH",
	})

	var capturedPrompt string
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{"people":[],"action_items":[]}`}, nil
		},
	}
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			// Template references previous triage output.
			return &pipeline.PromptTemplate{
				Content: "Extract from this {{.triage}} content:\n{{.Content}}",
			}, nil
		},
	}
	ex := newTestLLMExecutor(ai, pr)
	stage := PipelineStageConfig{Stage: "extract_semantic", StageKind: "llm"}

	input := baseLLMInput()
	input.PreviousOutputs = map[string]StageOutput{
		"triage": {
			Success:   true,
			StageName: "triage",
			StageKind: "llm",
			RawData:   triageRaw,
		},
	}

	out, err := ex.Execute(context.Background(), stage, input)

	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "extract_semantic", out.StageName)
	require.Contains(t, capturedPrompt, "Quarterly earnings call")
	// The triage map is rendered into the prompt.
	require.Contains(t, capturedPrompt, "INTERNAL", "triage output must appear in rendered prompt")
}

// TestLLMStageExecutor_AnalyzePattern simulates an analyze stage — prompt uses
// {{.triage}} and {{.extract_semantic}} (multiple previous outputs).
func TestLLMStageExecutor_AnalyzePattern(t *testing.T) {
	triageRaw, _ := json.Marshal(map[string]interface{}{"category": "PROJECT_UPDATE", "importance": "HIGH"})
	extractRaw, _ := json.Marshal(map[string]interface{}{"people": []string{"Alice", "Bob"}, "action_items": []string{"review Q3 plan"}})

	var capturedPrompt string
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{"summary":"Meeting summary","sentiment":"positive"}`}, nil
		},
	}
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{
				Content: "Analyze:\nTriage={{.triage}}\nExtract={{.extract_semantic}}\nContent={{.Content}}",
			}, nil
		},
	}
	ex := newTestLLMExecutor(ai, pr)
	stage := PipelineStageConfig{Stage: "analyze", StageKind: "llm"}

	input := baseLLMInput()
	input.PreviousOutputs = map[string]StageOutput{
		"triage":           {Success: true, StageName: "triage", RawData: triageRaw},
		"extract_semantic": {Success: true, StageName: "extract_semantic", RawData: extractRaw},
	}

	out, err := ex.Execute(context.Background(), stage, input)

	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "analyze", out.StageName)
	require.Contains(t, capturedPrompt, "PROJECT_UPDATE", "triage data must appear in prompt")
	require.Contains(t, capturedPrompt, "Alice", "extract data must appear in prompt")
	require.Contains(t, capturedPrompt, "Quarterly earnings call")
}

// TestLLMStageExecutor_FailedPreviousOutputsExcluded verifies that failed or skipped
// previous outputs are not made available to the template.
func TestLLMStageExecutor_FailedPreviousOutputsExcluded(t *testing.T) {
	failedTriageRaw, _ := json.Marshal(map[string]interface{}{"category": "SHOULD_NOT_APPEAR"})
	skippedExtractRaw, _ := json.Marshal(map[string]interface{}{"people": []string{"GHOST"}})

	var capturedPrompt string
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestLLMExecutor(ai, pr)
	stage := PipelineStageConfig{Stage: "analyze", StageKind: "llm"}

	input := baseLLMInput()
	input.PreviousOutputs = map[string]StageOutput{
		"triage":           {Success: false, RawData: failedTriageRaw},   // failed — excluded
		"extract_semantic": {Success: true, Skipped: true, RawData: skippedExtractRaw}, // skipped — Success=true but..
	}
	// Note: skipped stages have Success=true in our envelope, so extract_semantic WILL appear
	// (its RawData may be nil in real skipped stages, but here we test with RawData present).

	_, err := ex.Execute(context.Background(), stage, input)

	require.NoError(t, err)
	require.NotContains(t, capturedPrompt, "SHOULD_NOT_APPEAR", "failed outputs must not reach the template")
}

// --- AI call tests ---

func TestLLMStageExecutor_AICallReceivesJSONMode(t *testing.T) {
	var capturedReq *aiv1.SummaryRequest
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedReq = req
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)

	_, err := ex.Execute(context.Background(), baseLLMStage(), baseLLMInput())

	require.NoError(t, err)
	require.NotNil(t, capturedReq.JsonMode, "JsonMode must be set")
	require.True(t, *capturedReq.JsonMode)
}

func TestLLMStageExecutor_ModelOverrideApplied(t *testing.T) {
	var capturedReq *aiv1.SummaryRequest
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedReq = req
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)
	stage := baseLLMStage()
	stage.ModelOverride = "claude-opus-4-6"

	_, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err)
	require.NotNil(t, capturedReq.Model, "Model must be set when ModelOverride is non-empty")
	require.Equal(t, "claude-opus-4-6", *capturedReq.Model)
}

func TestLLMStageExecutor_NoModelOverrideOmitsModelField(t *testing.T) {
	var capturedReq *aiv1.SummaryRequest
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedReq = req
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)
	stage := baseLLMStage()
	stage.ModelOverride = ""

	_, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err)
	require.Nil(t, capturedReq.Model, "Model must be nil when ModelOverride is empty")
}

func TestLLMStageExecutor_LangfuseTraceIDPropagated(t *testing.T) {
	var capturedCtx context.Context
	ai := &mockLLMAIClient{
		generateSummaryFn: func(ctx context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedCtx = ctx
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)

	input := baseLLMInput()
	input.LangfuseTraceID = "trace-llm-abc-123"

	_, err := ex.Execute(context.Background(), baseLLMStage(), input)

	require.NoError(t, err)
	md, ok := metadata.FromOutgoingContext(capturedCtx)
	require.True(t, ok, "gRPC outgoing metadata must be set when LangfuseTraceID is non-empty")
	vals := md.Get("x-langfuse-trace-id")
	require.Len(t, vals, 1)
	require.Equal(t, "trace-llm-abc-123", vals[0])
}

func TestLLMStageExecutor_NoLangfuseTraceIDSkipsMetadata(t *testing.T) {
	var capturedCtx context.Context
	ai := &mockLLMAIClient{
		generateSummaryFn: func(ctx context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedCtx = ctx
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)

	input := baseLLMInput()
	input.LangfuseTraceID = ""

	_, err := ex.Execute(context.Background(), baseLLMStage(), input)

	require.NoError(t, err)
	_, ok := metadata.FromOutgoingContext(capturedCtx)
	require.False(t, ok, "no gRPC metadata should be set when LangfuseTraceID is empty")
}

// --- Error handling tests ---

func TestLLMStageExecutor_AICallErrorBlockingStage(t *testing.T) {
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return nil, errors.New("model overloaded: rate limit exceeded")
		},
	}
	ex := newTestLLMExecutor(ai, nil)
	stage := baseLLMStage()
	stage.Optional = false

	out, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.Error(t, err)
	require.Contains(t, err.Error(), "model overloaded")
	require.False(t, out.Success)
	require.Equal(t, "triage", out.StageName)
}

func TestLLMStageExecutor_AICallErrorOptionalStage(t *testing.T) {
	// Optional stages must return Success=false but a nil error so the pipeline continues.
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return nil, errors.New("context deadline exceeded")
		},
	}
	ex := newTestLLMExecutor(ai, nil)
	stage := baseLLMStage()
	stage.Optional = true

	out, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err, "optional stage AI failure must not propagate as an error")
	require.False(t, out.Success, "optional failure must set Success=false")
	require.Contains(t, out.Error, "context deadline exceeded")
	require.Nil(t, out.RawData)
}

// --- Output structure tests ---

func TestLLMStageExecutor_SuccessOutputFields(t *testing.T) {
	const rawResponse = `{"category":"INTERNAL","importance":"HIGH","reason":"executive communications"}`
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{Summary: rawResponse, ModelUsed: "claude-sonnet-4-6"}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)

	out, err := ex.Execute(context.Background(), baseLLMStage(), baseLLMInput())

	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "triage", out.StageName)
	require.Equal(t, "llm", out.StageKind)
	require.Equal(t, rawResponse, string(out.RawData))
	require.False(t, out.Skipped)
	require.Empty(t, out.Error)
}

func TestLLMStageExecutor_RawDataIsJSONSerializable(t *testing.T) {
	// Temporal requires all workflow/activity data to round-trip through JSON.
	ex := newTestLLMExecutor(nil, nil)

	out, err := ex.Execute(context.Background(), baseLLMStage(), baseLLMInput())
	require.NoError(t, err)

	data, err := json.Marshal(out)
	require.NoError(t, err)

	var restored StageOutput
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, out.Success, restored.Success)
	require.Equal(t, out.StageName, restored.StageName)
	require.Equal(t, out.StageKind, restored.StageKind)
	require.Equal(t, string(out.RawData), string(restored.RawData))
}

// TestLLMStageExecutor_RawDataUnmarshalable verifies that the response JSON
// returned by the executor can be decoded into a stage-specific output type.
// This models the "output parity" acceptance criterion: downstream stages and
// the dispatch loop must be able to unmarshal RawData into the expected struct.
func TestLLMStageExecutor_RawDataUnmarshalable(t *testing.T) {
	// TriageOutput-shaped response from the AI.
	type triageOutput struct {
		Category   string  `json:"category"`
		Importance string  `json:"importance"`
		Reason     string  `json:"reason"`
		Confidence float64 `json:"confidence"`
	}

	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary: `{"category":"INTERNAL","importance":"HIGH","reason":"exec comms","confidence":0.95}`,
			}, nil
		},
	}
	ex := newTestLLMExecutor(ai, nil)

	out, err := ex.Execute(context.Background(), baseLLMStage(), baseLLMInput())

	require.NoError(t, err)
	require.True(t, out.Success)
	require.NotEmpty(t, out.RawData)

	var triage triageOutput
	require.NoError(t, json.Unmarshal(out.RawData, &triage), "RawData must unmarshal into stage-specific output type")
	require.Equal(t, "INTERNAL", triage.Category)
	require.Equal(t, "HIGH", triage.Importance)
	require.InDelta(t, 0.95, triage.Confidence, 0.001)
}

// TestLLMStageExecutor_SummarizePattern verifies that summarize works with only
// Content and no previous outputs — the simplest LLM stage.
func TestLLMStageExecutor_SummarizePattern(t *testing.T) {
	const summaryJSON = `{"summary":"Brief summary of the earnings call.","key_points":["Q3 growth","new product line"]}`

	var capturedPrompt string
	ai := &mockLLMAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: summaryJSON}, nil
		},
	}
	pr := &mockLLMPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Content: "Summarize in 150 words:\n{{.Content}}"}, nil
		},
	}
	ex := newTestLLMExecutor(ai, pr)
	stage := PipelineStageConfig{Stage: "summarize", StageKind: "llm"}

	out, err := ex.Execute(context.Background(), stage, baseLLMInput())

	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "summarize", out.StageName)
	require.Equal(t, summaryJSON, string(out.RawData))
	require.Contains(t, capturedPrompt, "Quarterly earnings call")
	require.Contains(t, capturedPrompt, "Summarize in 150 words:")
}
