// Package activities provides unit tests for the StructuredExtract activity.
package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// mockPromptRepository is a test double for PromptRepository.
type mockPromptRepository struct {
	getPromptFn func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error)
}

func (m *mockPromptRepository) GetPromptByStage(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
	if m.getPromptFn != nil {
		return m.getPromptFn(ctx, stage, version)
	}
	// Default: return a simple passthrough template so tests can control rendering
	content := "{{.Content}}"
	return &pipeline.PromptTemplate{
		Stage:   stage,
		Version: version,
		Content: content,
	}, nil
}

// newTestStructuredExtractActivities builds a StructuredExtractActivities for unit
// testing. The db field is deliberately nil because StructuredExtract does not use it;
// we bypass the constructor to avoid the nil-db panic.
func newTestStructuredExtractActivities(aiClient AIClient, promptRepo PromptRepository) *StructuredExtractActivities {
	logger := logging.NewNopLogger()
	return &StructuredExtractActivities{
		db:         nil, // not used in StructuredExtract
		aiClient:   aiClient,
		promptRepo: promptRepo,
		logger:     logger.With(logging.F("component", "structured_extract_activities")),
	}
}

// TestStructuredExtract_EmptyContent verifies that an empty Content field
// returns a ValidationError without calling the prompt repo or AI client.
func TestStructuredExtract_EmptyContent(t *testing.T) {
	called := false
	ai := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			called = true
			return nil, nil
		},
	}
	a := newTestStructuredExtractActivities(ai, &mockPromptRepository{})

	_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
		TenantID:  "tenant-1",
		SourceID:  1,
		StageName: "newsletter_extract",
		Content:   "",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "content is empty")
	require.False(t, called, "AI client must not be called for empty content")
}

// TestStructuredExtract_EmptyStageName verifies that an empty StageName field
// returns a ValidationError.
func TestStructuredExtract_EmptyStageName(t *testing.T) {
	a := newTestStructuredExtractActivities(&mockAIClient{}, &mockPromptRepository{})

	_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
		TenantID:  "tenant-1",
		SourceID:  1,
		StageName: "",
		Content:   "Some content here",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "stage_name is required")
}

// TestStructuredExtract_PromptOverrideSelection verifies that a PromptOverride of 0
// fetches the active prompt version and that a non-zero override passes through correctly.
func TestStructuredExtract_PromptOverrideSelection(t *testing.T) {
	t.Run("version 0 fetches active prompt", func(t *testing.T) {
		var capturedVersion int
		promptRepo := &mockPromptRepository{
			getPromptFn: func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
				capturedVersion = version
				return &pipeline.PromptTemplate{Stage: stage, Version: 0, Content: "{{.Content}}"}, nil
			},
		}
		ai := &mockAIClient{
			generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
				return &aiv1.SummaryResponse{Summary: `{}`, ModelUsed: "test-model"}, nil
			},
		}
		a := newTestStructuredExtractActivities(ai, promptRepo)

		_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
			TenantID:       "tenant-1",
			SourceID:       1,
			StageName:      "newsletter_extract",
			Content:        "some content",
			PromptOverride: 0,
		})
		require.NoError(t, err)
		require.Equal(t, 0, capturedVersion, "version 0 should be passed to GetPromptByStage for active lookup")
	})

	t.Run("specific version passed through", func(t *testing.T) {
		var capturedVersion int
		promptRepo := &mockPromptRepository{
			getPromptFn: func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
				capturedVersion = version
				return &pipeline.PromptTemplate{Stage: stage, Version: version, Content: "{{.Content}}"}, nil
			},
		}
		ai := &mockAIClient{
			generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
				return &aiv1.SummaryResponse{Summary: `{}`, ModelUsed: "test-model"}, nil
			},
		}
		a := newTestStructuredExtractActivities(ai, promptRepo)

		_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
			TenantID:       "tenant-1",
			SourceID:       1,
			StageName:      "newsletter_extract",
			Content:        "some content",
			PromptOverride: 3,
		})
		require.NoError(t, err)
		require.Equal(t, 3, capturedVersion, "non-zero PromptOverride should be passed to GetPromptByStage")
	})
}

// TestStructuredExtract_BackgroundContextRendering verifies that when BackgroundContext
// is set, the rendered prompt sent to the AI client includes the context text.
func TestStructuredExtract_BackgroundContextRendering(t *testing.T) {
	const promptTemplate = "Context: {{.BackgroundContext}}\nContent: {{.Content}}"
	promptRepo := &mockPromptRepository{
		getPromptFn: func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Stage: stage, Version: 0, Content: promptTemplate}, nil
		},
	}

	var capturedPrompt string
	ai := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{"key":"value"}`, ModelUsed: "test-model"}, nil
		},
	}
	a := newTestStructuredExtractActivities(ai, promptRepo)

	_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
		TenantID:          "tenant-1",
		SourceID:          1,
		StageName:         "newsletter_extract",
		Content:           "article body here",
		BackgroundContext: "Glossary: Alpha=Project Alpha",
	})
	require.NoError(t, err)
	require.Contains(t, capturedPrompt, "Glossary: Alpha=Project Alpha",
		"rendered prompt must include BackgroundContext when set")
	require.Contains(t, capturedPrompt, "article body here",
		"rendered prompt must include Content")
}

// TestStructuredExtract_NoBackgroundContext verifies that when BackgroundContext is
// empty, the template renders without it (no empty placeholder injected).
func TestStructuredExtract_NoBackgroundContext(t *testing.T) {
	const promptTemplate = "{{.Content}}"
	promptRepo := &mockPromptRepository{
		getPromptFn: func(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error) {
			return &pipeline.PromptTemplate{Stage: stage, Version: 0, Content: promptTemplate}, nil
		},
	}

	var capturedPrompt string
	ai := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedPrompt = req.Content
			return &aiv1.SummaryResponse{Summary: `{}`, ModelUsed: "test-model"}, nil
		},
	}
	a := newTestStructuredExtractActivities(ai, promptRepo)

	_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
		TenantID:          "tenant-1",
		SourceID:          1,
		StageName:         "newsletter_extract",
		Content:           "article body here",
		BackgroundContext: "",
	})
	require.NoError(t, err)
	require.Equal(t, "article body here", capturedPrompt,
		"rendered prompt should equal content only when BackgroundContext is empty")
	require.False(t, strings.Contains(capturedPrompt, "BackgroundContext"),
		"rendered prompt must not contain literal 'BackgroundContext' key when context is empty")
}

// TestStructuredExtract_LangfuseMetadataPropagation verifies that when LangfuseTraceID
// is set, the gRPC context passed to GenerateSummary carries the expected metadata headers.
func TestStructuredExtract_LangfuseMetadataPropagation(t *testing.T) {
	promptRepo := &mockPromptRepository{}

	var capturedCtx context.Context
	ai := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			capturedCtx = ctx
			return &aiv1.SummaryResponse{Summary: `{}`, ModelUsed: "test-model"}, nil
		},
	}
	a := newTestStructuredExtractActivities(ai, promptRepo)

	_, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
		TenantID:        "tenant-1",
		SourceID:        1,
		StageName:       "newsletter_extract",
		Content:         "some content",
		LangfuseTraceID: "trace-abc-123",
		LangfusePhaseID: "phase-xyz-456",
	})
	require.NoError(t, err)
	require.NotNil(t, capturedCtx, "AI client must receive a context")

	md, ok := metadata.FromOutgoingContext(capturedCtx)
	require.True(t, ok, "gRPC outgoing metadata must be set when LangfuseTraceID is non-empty")

	traceVals := md.Get("x-langfuse-trace-id")
	require.Len(t, traceVals, 1)
	require.Equal(t, "trace-abc-123", traceVals[0])

	phaseVals := md.Get("x-langfuse-phase-id")
	require.Len(t, phaseVals, 1)
	require.Equal(t, "phase-xyz-456", phaseVals[0])
}

// TestStructuredExtract_Success verifies the happy path returns the correct output fields.
func TestStructuredExtract_Success(t *testing.T) {
	inputTokens := int32(100)
	outputTokens := int32(50)

	promptRepo := &mockPromptRepository{}
	ai := &mockAIClient{
		generateSummaryFn: func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
			require.NotNil(t, req.JsonMode, "JSON mode must be enabled")
			require.True(t, *req.JsonMode, "JSON mode must be true")
			return &aiv1.SummaryResponse{
				Summary:      `{"title":"Test Newsletter","items":[]}`,
				ModelUsed:    "claude-3-5-sonnet",
				InputTokens:  &inputTokens,
				OutputTokens: &outputTokens,
			}, nil
		},
	}
	a := newTestStructuredExtractActivities(ai, promptRepo)

	out, err := a.StructuredExtract(context.Background(), StructuredExtractInput{
		TenantID:  "tenant-1",
		SourceID:  42,
		StageName: "newsletter_extract",
		Content:   "This is a newsletter about Go programming.",
	})

	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "newsletter_extract", out.StageName)
	require.Equal(t, "claude-3-5-sonnet", out.ModelUsed)
	require.Equal(t, 100, out.InputTokenCount)
	require.Equal(t, 50, out.OutputTokenCount)
	require.Equal(t, `{"title":"Test Newsletter","items":[]}`, string(out.RawJSON))
}
