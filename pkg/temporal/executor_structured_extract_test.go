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

type mockSEAIClient struct {
	generateSummaryFn func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
}

func (m *mockSEAIClient) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	if m.generateSummaryFn != nil {
		return m.generateSummaryFn(ctx, req)
	}
	return &aiv1.SummaryResponse{Summary: `{}`, ModelUsed: "test-model"}, nil
}

type mockSEPromptRepo struct {
	getPromptFn func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error)
}

func (m *mockSEPromptRepo) GetPromptByStage(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
	if m.getPromptFn != nil {
		return m.getPromptFn(ctx, stage, version)
	}
	return &pipeline.PromptTemplate{Stage: stage, Version: version, Content: "{{.Content}}"}, nil
}

type mockSEPersister struct {
	persistFn func(ctx context.Context, tenantID string, sourceID int64, key string, data json.RawMessage) (bool, error)
}

func (m *mockSEPersister) PersistExtractedData(ctx context.Context, tenantID string, sourceID int64, key string, data json.RawMessage) (bool, error) {
	if m.persistFn != nil {
		return m.persistFn(ctx, tenantID, sourceID, key, data)
	}
	return true, nil
}

// newTestExecutor builds a StructuredExtractExecutor with sensible defaults for testing.
// Any of the three dependencies can be overridden by passing non-nil values.
func newTestExecutor(ai StructuredExtractAIClient, pr StructuredExtractPromptRepo, pe StructuredExtractPersister) *StructuredExtractExecutor {
	if ai == nil {
		ai = &mockSEAIClient{}
	}
	if pr == nil {
		pr = &mockSEPromptRepo{}
	}
	if pe == nil {
		pe = &mockSEPersister{}
	}
	return NewStructuredExtractExecutor(ai, pr, pe)
}

func baseStage() PipelineStageConfig {
	return PipelineStageConfig{
		Stage:     "newsletter_extract",
		StageKind: "structured_extract",
		PersistKey: "newsletter",
	}
}

func baseInput() StageInput {
	return StageInput{
		TenantID: "tenant-1",
		SourceID: 42,
		Content:  "This is a newsletter about Go programming.",
	}
}

// --- Validation tests ---

func TestStructuredExtractExecutor_EmptyContent(t *testing.T) {
	ex := newTestExecutor(nil, nil, nil)
	input := baseInput()
	input.Content = ""

	out, err := ex.Execute(context.Background(), baseStage(), input)

	require.Error(t, err)
	require.Contains(t, err.Error(), "content is empty")
	require.False(t, out.Success)
	require.Equal(t, "newsletter_extract", out.StageName)
}

func TestStructuredExtractExecutor_EmptyStageName(t *testing.T) {
	ex := newTestExecutor(nil, nil, nil)
	stage := baseStage()
	stage.Stage = ""

	out, err := ex.Execute(context.Background(), stage, baseInput())

	require.Error(t, err)
	require.Contains(t, err.Error(), "stage name is required")
	require.False(t, out.Success)
}

// --- Prompt loading tests ---

func TestStructuredExtractExecutor_PromptOverrideZeroFetchesActive(t *testing.T) {
	var capturedVersion int
	pr := &mockSEPromptRepo{
		getPromptFn: func(_ context.Context, _ string, version int) (*pipeline.PromptTemplate, error) {
			capturedVersion = version
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestExecutor(nil, pr, nil)
	stage := baseStage()
	stage.PromptOverride = 0

	_, err := ex.Execute(context.Background(), stage, baseInput())

	require.NoError(t, err)
	require.Equal(t, 0, capturedVersion, "version 0 should request the active prompt")
}

func TestStructuredExtractExecutor_PromptOverrideNonZeroPassedThrough(t *testing.T) {
	var capturedVersion int
	pr := &mockSEPromptRepo{
		getPromptFn: func(_ context.Context, _ string, version int) (*pipeline.PromptTemplate, error) {
			capturedVersion = version
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestExecutor(nil, pr, nil)
	stage := baseStage()
	stage.PromptOverride = 5

	_, err := ex.Execute(context.Background(), stage, baseInput())

	require.NoError(t, err)
	require.Equal(t, 5, capturedVersion)
}

func TestStructuredExtractExecutor_PromptLoadError(t *testing.T) {
	pr := &mockSEPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return nil, errors.New("prompt not found")
		},
	}
	ex := newTestExecutor(nil, pr, nil)

	out, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt not found")
	require.False(t, out.Success)
}

// --- Context building tests ---

func TestStructuredExtractExecutor_BackgroundContextFromPreviousOutput(t *testing.T) {
	const bgCtx = "Glossary: Alpha=Project Alpha\nTopics: engineering"
	previousOutputRaw, _ := json.Marshal(backgroundContextCarrier{BackgroundContext: bgCtx})

	var capturedPrompt string
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	pr := &mockSEPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Content: "Context: {{.BackgroundContext}}\nContent: {{.Content}}"}, nil
		},
	}
	ex := newTestExecutor(ai, pr, nil)

	input := baseInput()
	input.PreviousOutputs = map[string]StageOutput{
		"build_extraction_context": {
			Success:   true,
			StageName: "build_extraction_context",
			StageKind: "code_only",
			RawData:   previousOutputRaw,
		},
	}

	_, err := ex.Execute(context.Background(), baseStage(), input)

	require.NoError(t, err)
	require.Contains(t, capturedPrompt, bgCtx, "rendered prompt must include background context from previous output")
	require.Contains(t, capturedPrompt, "This is a newsletter about Go programming.")
}

func TestStructuredExtractExecutor_NoPreviousOutputsUsesEmptyContext(t *testing.T) {
	var capturedPrompt string
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	pr := &mockSEPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestExecutor(ai, pr, nil)

	_, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.NoError(t, err)
	require.Equal(t, "This is a newsletter about Go programming.", capturedPrompt)
}

func TestStructuredExtractExecutor_FailedPreviousOutputIgnored(t *testing.T) {
	// A failed previous output should not contribute background context.
	failedRaw, _ := json.Marshal(backgroundContextCarrier{BackgroundContext: "should-not-appear"})
	var capturedPrompt string
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	pr := &mockSEPromptRepo{
		getPromptFn: func(_ context.Context, _ string, _ int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Content: "{{.Content}}"}, nil
		},
	}
	ex := newTestExecutor(ai, pr, nil)

	input := baseInput()
	input.PreviousOutputs = map[string]StageOutput{
		"build_extraction_context": {
			Success: false, // failed — must not contribute context
			RawData: failedRaw,
		},
	}

	_, err := ex.Execute(context.Background(), baseStage(), input)

	require.NoError(t, err)
	require.NotContains(t, capturedPrompt, "should-not-appear")
}

// --- AI call tests ---

func TestStructuredExtractExecutor_AICallReceivesJSONMode(t *testing.T) {
	var capturedReq *aiv1.SummaryRequest
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedReq = req
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestExecutor(ai, nil, nil)

	_, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.NoError(t, err)
	require.NotNil(t, capturedReq.JsonMode, "JsonMode must be set")
	require.True(t, *capturedReq.JsonMode, "JsonMode must be true")
}

func TestStructuredExtractExecutor_ModelOverrideApplied(t *testing.T) {
	var capturedReq *aiv1.SummaryRequest
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedReq = req
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestExecutor(ai, nil, nil)
	stage := baseStage()
	stage.ModelOverride = "claude-opus-4-6"

	_, err := ex.Execute(context.Background(), stage, baseInput())

	require.NoError(t, err)
	require.NotNil(t, capturedReq.Model, "Model must be set when ModelOverride is non-empty")
	require.Equal(t, "claude-opus-4-6", *capturedReq.Model)
}

func TestStructuredExtractExecutor_NoModelOverrideOmitsModelField(t *testing.T) {
	var capturedReq *aiv1.SummaryRequest
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedReq = req
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestExecutor(ai, nil, nil)
	stage := baseStage()
	stage.ModelOverride = ""

	_, err := ex.Execute(context.Background(), stage, baseInput())

	require.NoError(t, err)
	require.Nil(t, capturedReq.Model, "Model must be nil when ModelOverride is empty")
}

func TestStructuredExtractExecutor_LangfuseTraceIDPropagated(t *testing.T) {
	var capturedCtx context.Context
	ai := &mockSEAIClient{
		generateSummaryFn: func(ctx context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedCtx = ctx
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestExecutor(ai, nil, nil)

	input := baseInput()
	input.LangfuseTraceID = "trace-abc-123"

	_, err := ex.Execute(context.Background(), baseStage(), input)

	require.NoError(t, err)
	md, ok := metadata.FromOutgoingContext(capturedCtx)
	require.True(t, ok, "gRPC outgoing metadata must be set when LangfuseTraceID is non-empty")
	vals := md.Get("x-langfuse-trace-id")
	require.Len(t, vals, 1)
	require.Equal(t, "trace-abc-123", vals[0])
}

func TestStructuredExtractExecutor_NoLangfuseTraceIDSkipsMetadata(t *testing.T) {
	var capturedCtx context.Context
	ai := &mockSEAIClient{
		generateSummaryFn: func(ctx context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedCtx = ctx
			return &aiv1.SummaryResponse{Summary: `{}`}, nil
		},
	}
	ex := newTestExecutor(ai, nil, nil)

	input := baseInput()
	input.LangfuseTraceID = ""

	_, err := ex.Execute(context.Background(), baseStage(), input)

	require.NoError(t, err)
	_, ok := metadata.FromOutgoingContext(capturedCtx)
	require.False(t, ok, "no gRPC metadata should be set when LangfuseTraceID is empty")
}

func TestStructuredExtractExecutor_AICallError(t *testing.T) {
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return nil, errors.New("model overloaded")
		},
	}
	ex := newTestExecutor(ai, nil, nil)

	out, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.Error(t, err)
	require.Contains(t, err.Error(), "model overloaded")
	require.False(t, out.Success)
	require.Equal(t, "newsletter_extract", out.StageName)
}

// --- Persist tests ---

func TestStructuredExtractExecutor_PersistKeyFromConfig(t *testing.T) {
	var capturedKey string
	pe := &mockSEPersister{
		persistFn: func(_ context.Context, _ string, _ int64, key string, _ json.RawMessage) (bool, error) {
			capturedKey = key
			return true, nil
		},
	}
	ex := newTestExecutor(nil, nil, pe)
	stage := baseStage()
	stage.PersistKey = "newsletter"

	_, err := ex.Execute(context.Background(), stage, baseInput())

	require.NoError(t, err)
	require.Equal(t, "newsletter", capturedKey)
}

func TestStructuredExtractExecutor_PersistKeyFallback(t *testing.T) {
	var capturedKey string
	pe := &mockSEPersister{
		persistFn: func(_ context.Context, _ string, _ int64, key string, _ json.RawMessage) (bool, error) {
			capturedKey = key
			return true, nil
		},
	}
	ex := newTestExecutor(nil, nil, pe)
	stage := baseStage()
	stage.PersistKey = "" // empty — should fall back

	_, err := ex.Execute(context.Background(), stage, baseInput())

	require.NoError(t, err)
	require.Equal(t, "newsletter", capturedKey, "PersistKey fallback must strip _extract suffix")
}

func TestStructuredExtractExecutor_PersistReceivesTenantAndSourceID(t *testing.T) {
	var capturedTenantID string
	var capturedSourceID int64
	pe := &mockSEPersister{
		persistFn: func(_ context.Context, tenantID string, sourceID int64, _ string, _ json.RawMessage) (bool, error) {
			capturedTenantID = tenantID
			capturedSourceID = sourceID
			return true, nil
		},
	}
	ex := newTestExecutor(nil, nil, pe)

	_, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.NoError(t, err)
	require.Equal(t, "tenant-1", capturedTenantID)
	require.Equal(t, int64(42), capturedSourceID)
}

func TestStructuredExtractExecutor_PersistFailureIsNonBlocking(t *testing.T) {
	// Persist failure must not fail the stage — matches pipeline.go:2608 non-blocking pattern.
	pe := &mockSEPersister{
		persistFn: func(_ context.Context, _ string, _ int64, _ string, _ json.RawMessage) (bool, error) {
			return false, errors.New("DB connection lost")
		},
	}
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{Summary: `{"title":"Test"}`}, nil
		},
	}
	ex := newTestExecutor(ai, nil, pe)

	out, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.NoError(t, err, "persist failure must not return an error")
	require.True(t, out.Success, "stage must succeed even when persist fails")
	require.Equal(t, `{"title":"Test"}`, string(out.RawData), "RawData must contain extracted JSON")
}

// --- Success / output tests ---

func TestStructuredExtractExecutor_SuccessOutput(t *testing.T) {
	const rawResponse = `{"title":"Go Programming","items":["generics","testing"]}`
	ai := &mockSEAIClient{
		generateSummaryFn: func(_ context.Context, _ *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			return &aiv1.SummaryResponse{
				Summary:   rawResponse,
				ModelUsed: "claude-sonnet-4-6",
			}, nil
		},
	}
	ex := newTestExecutor(ai, nil, nil)

	out, err := ex.Execute(context.Background(), baseStage(), baseInput())

	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "newsletter_extract", out.StageName)
	require.Equal(t, "structured_extract", out.StageKind)
	require.Equal(t, rawResponse, string(out.RawData))
}

func TestStructuredExtractExecutor_OutputIsJSONSerializable(t *testing.T) {
	// Verify the returned StageOutput round-trips through JSON (Temporal serialization requirement).
	ex := newTestExecutor(nil, nil, nil)

	out, err := ex.Execute(context.Background(), baseStage(), baseInput())
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

// --- Constructor tests ---

func TestNewStructuredExtractExecutor_PanicsOnNilDependencies(t *testing.T) {
	ai := &mockSEAIClient{}
	pr := &mockSEPromptRepo{}
	pe := &mockSEPersister{}

	require.Panics(t, func() { NewStructuredExtractExecutor(nil, pr, pe) })
	require.Panics(t, func() { NewStructuredExtractExecutor(ai, nil, pe) })
	require.Panics(t, func() { NewStructuredExtractExecutor(ai, pr, nil) })
}
