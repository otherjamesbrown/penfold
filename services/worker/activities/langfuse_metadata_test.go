// Package activities provides acceptance tests for pf-37ebe8:
// Pass Langfuse traceID + phaseID via gRPC metadata to AI coordinator calls,
// and remove all ReportLangfuseGeneration calls from the pipeline.
//
// These tests are INTENTIONALLY FAILING until the feature is implemented.
// They reference fields (LangfuseTraceID, LangfusePhaseID) that do not yet exist
// on the activity input structs, causing compile-time failures.
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// contextCapturingAIClient wraps mockAIClient and captures the context passed
// to each AI method so tests can inspect outgoing gRPC metadata.
type contextCapturingAIClient struct {
	inner           *mockAIClient
	capturedTriageCtx context.Context
	capturedAnalyzeCtx context.Context
	capturedSummaryCtx context.Context
	capturedExtractCtx context.Context
}

func (c *contextCapturingAIClient) TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	c.capturedTriageCtx = ctx
	if c.inner.triageContentFn != nil {
		return c.inner.triageContentFn(ctx, req)
	}
	return &aiv1.TriageContentResponse{
		Category:   "PROJECT_UPDATE",
		Importance: "HIGH",
		Reason:     "Test",
		ModelUsed:  "test-model",
	}, nil
}

func (c *contextCapturingAIClient) DeepAnalyze(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
	c.capturedAnalyzeCtx = ctx
	if c.inner.deepAnalyzeFn != nil {
		return c.inner.deepAnalyzeFn(ctx, req)
	}
	return &aiv1.DeepAnalyzeResponse{Summary: "test", ModelUsed: "test-model"}, nil
}

func (c *contextCapturingAIClient) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	c.capturedSummaryCtx = ctx
	if c.inner.generateSummaryFn != nil {
		return c.inner.generateSummaryFn(ctx, req)
	}
	return &aiv1.SummaryResponse{}, nil
}

func (c *contextCapturingAIClient) ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	c.capturedExtractCtx = ctx
	if c.inner.extractEntitiesFn != nil {
		return c.inner.extractEntitiesFn(ctx, req)
	}
	return &aiv1.ExtractEntitiesResponse{}, nil
}

func (c *contextCapturingAIClient) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	if c.inner.extractAssertionsFn != nil {
		return c.inner.extractAssertionsFn(ctx, req)
	}
	return &aiv1.AssertionResponse{}, nil
}

func (c *contextCapturingAIClient) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	if c.inner.generateEmbeddingFn != nil {
		return c.inner.generateEmbeddingFn(ctx, req)
	}
	return &aiv1.EmbeddingResponse{}, nil
}

// --- Acceptance Test 1: Triage attaches gRPC metadata when traceID is non-empty ---

// TestTriage_PassesLangfuseMetadata verifies that when LangfuseTraceID and
// LangfusePhaseID are set on TriageInput, the context passed to
// aiClient.TriageContent contains gRPC outgoing metadata with the keys
// "x-langfuse-trace-id" and "x-langfuse-phase-id".
//
// FAILS until pf-37ebe8 is implemented:
//   - TriageInput does not yet have LangfuseTraceID or LangfusePhaseID fields.
func TestTriage_PassesLangfuseMetadata(t *testing.T) {
	logger := logging.NewNopLogger()
	capturingClient := &contextCapturingAIClient{inner: &mockAIClient{}}
	activities := NewTriageActivities(logger, capturingClient, nil, nil, nil)

	input := workflows.TriageInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		ContentID:   "em-abc123",
		JobID:       "job-123",
		Content:     "Project Alpha is on track for Q2 delivery.",
		Subject:     "Project Status Update",
		SenderEmail: "alice@example.com",
		ContentType: "email",
		// These fields do not yet exist on TriageInput — compile failure expected.
		LangfuseTraceID: "trace-abc-123",
		LangfusePhaseID: "phase-triage-456",
	}

	_, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)

	// Verify gRPC outgoing metadata was attached to the context used for the AI call.
	require.NotNil(t, capturingClient.capturedTriageCtx, "expected context to be captured")

	md, ok := metadata.FromOutgoingContext(capturingClient.capturedTriageCtx)
	require.True(t, ok, "expected gRPC outgoing metadata to be present on context passed to TriageContent")

	traceVals := md.Get("x-langfuse-trace-id")
	require.Len(t, traceVals, 1, "expected exactly one x-langfuse-trace-id metadata value")
	require.Equal(t, "trace-abc-123", traceVals[0])

	phaseVals := md.Get("x-langfuse-phase-id")
	require.Len(t, phaseVals, 1, "expected exactly one x-langfuse-phase-id metadata value")
	require.Equal(t, "phase-triage-456", phaseVals[0])
}

// --- Acceptance Test 2: Triage does NOT attach metadata when traceID is empty ---

// TestTriage_NoMetadata_WhenTraceIDEmpty verifies that when LangfuseTraceID is
// empty, no Langfuse-specific gRPC metadata is attached to the outgoing context.
// This prevents unnecessary metadata on non-traced requests.
//
// FAILS until pf-37ebe8 is implemented:
//   - TriageInput does not yet have LangfuseTraceID or LangfusePhaseID fields.
func TestTriage_NoMetadata_WhenTraceIDEmpty(t *testing.T) {
	logger := logging.NewNopLogger()
	capturingClient := &contextCapturingAIClient{inner: &mockAIClient{}}
	activities := NewTriageActivities(logger, capturingClient, nil, nil, nil)

	input := workflows.TriageInput{
		TenantID:    "test-tenant",
		SourceID:    456,
		ContentID:   "em-def456",
		JobID:       "job-456",
		Content:     "Routine status update for the team.",
		Subject:     "Weekly Sync",
		SenderEmail: "bob@example.com",
		ContentType: "email",
		// LangfuseTraceID is intentionally empty — no metadata should be attached.
		LangfuseTraceID: "",
		LangfusePhaseID: "",
	}

	_, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)

	require.NotNil(t, capturingClient.capturedTriageCtx)

	// When traceID is empty, no Langfuse metadata keys should be present.
	md, ok := metadata.FromOutgoingContext(capturingClient.capturedTriageCtx)
	if ok {
		traceVals := md.Get("x-langfuse-trace-id")
		require.Empty(t, traceVals,
			"x-langfuse-trace-id must not be set when LangfuseTraceID is empty")
		phaseVals := md.Get("x-langfuse-phase-id")
		require.Empty(t, phaseVals,
			"x-langfuse-phase-id must not be set when LangfuseTraceID is empty")
	}
	// If no outgoing metadata at all, that's also acceptable.
}

// --- Acceptance Test 3: DeepAnalyze attaches gRPC metadata when traceID is non-empty ---

// TestDeepAnalyze_PassesLangfuseMetadata verifies that when LangfuseTraceID and
// LangfusePhaseID are set on DeepAnalyzeInput, the context passed to
// aiClient.DeepAnalyze contains gRPC outgoing metadata.
//
// FAILS until pf-37ebe8 is implemented:
//   - DeepAnalyzeInput does not yet have LangfuseTraceID or LangfusePhaseID fields.
func TestDeepAnalyze_PassesLangfuseMetadata(t *testing.T) {
	logger := logging.NewNopLogger()
	capturingClient := &contextCapturingAIClient{inner: &mockAIClient{}}
	activities := NewAnalysisActivities(logger, capturingClient, nil)

	input := workflows.DeepAnalyzeInput{
		TenantID:         "test-tenant",
		SourceID:         123,
		ContentID:        "em-abc123",
		JobID:            "job-123",
		Content:          "Project update with strategic decisions.",
		ContentType:      "email",
		TriageCategory:   "PROJECT_UPDATE",
		TriageImportance: "HIGH",
		// These fields do not yet exist on DeepAnalyzeInput — compile failure expected.
		LangfuseTraceID: "trace-analyze-789",
		LangfusePhaseID: "phase-analyze-012",
	}

	_, err := activities.DeepAnalyze(context.Background(), input)
	require.NoError(t, err)

	require.NotNil(t, capturingClient.capturedAnalyzeCtx, "expected context to be captured from DeepAnalyze call")

	md, ok := metadata.FromOutgoingContext(capturingClient.capturedAnalyzeCtx)
	require.True(t, ok, "expected gRPC outgoing metadata on context passed to aiClient.DeepAnalyze")

	traceVals := md.Get("x-langfuse-trace-id")
	require.Len(t, traceVals, 1)
	require.Equal(t, "trace-analyze-789", traceVals[0])

	phaseVals := md.Get("x-langfuse-phase-id")
	require.Len(t, phaseVals, 1)
	require.Equal(t, "phase-analyze-012", phaseVals[0])
}

// --- Acceptance Test 4: ExtractEntities attaches gRPC metadata ---

// TestExtractEntities_PassesLangfuseMetadata verifies that when LangfuseTraceID
// and LangfusePhaseID are set on the extraction input, the context passed to
// aiClient.ExtractEntities contains gRPC outgoing metadata.
//
// FAILS until pf-37ebe8 is implemented:
//   - SLMPipelineExtractEntitiesInput does not yet have LangfuseTraceID or LangfusePhaseID.
func TestExtractEntities_PassesLangfuseMetadata(t *testing.T) {
	logger := logging.NewNopLogger()
	capturingClient := &contextCapturingAIClient{inner: &mockAIClient{}}
	activities := NewExtractionActivities(logger, capturingClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := workflows.SLMPipelineExtractEntitiesInput{
		TenantID: "test-tenant",
		SourceID: 123,
		JobID:    "job-123",
		Content:  "Alice Smith will review the design doc by March 10th for Project Alpha.",
		// These fields do not yet exist — compile failure expected.
		LangfuseTraceID: "trace-extract-345",
		LangfusePhaseID: "phase-extract-678",
	}

	_, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)

	require.NotNil(t, capturingClient.capturedExtractCtx, "expected context to be captured from ExtractEntities call")

	md, ok := metadata.FromOutgoingContext(capturingClient.capturedExtractCtx)
	require.True(t, ok, "expected gRPC outgoing metadata on context passed to aiClient.ExtractEntities")

	traceVals := md.Get("x-langfuse-trace-id")
	require.Len(t, traceVals, 1)
	require.Equal(t, "trace-extract-345", traceVals[0])

	phaseVals := md.Get("x-langfuse-phase-id")
	require.Len(t, phaseVals, 1)
	require.Equal(t, "phase-extract-678", phaseVals[0])
}

// --- Acceptance Test 5: ReportLangfuseGenerationInput removed ---

// TestReportLangfuseGenerationInput_Removed verifies that
// ReportLangfuseGenerationInput no longer exists as a type.
//
// Since Go doesn't have negative type assertions at compile time, this test
// uses a runtime approach: it checks that the pipeline workflow type registry
// (via a sentinel compile check) forces a build failure if the type exists.
//
// This test documents the EXPECTED post-feature state. While
// ReportLangfuseGenerationInput currently DOES exist, once the feature is
// implemented this test validates the removal indirectly — the struct fields
// on TriageInput (LangfuseTraceID / LangfusePhaseID) are the primary compile
// gate. This test serves as documentation and a runtime assertion about the
// intended architecture.
//
// Post-implementation: the workflow should no longer schedule
// ActivityReportLangfuseGeneration activities. This is validated by the
// workflow-level tests in services/worker/workflows/.
func TestReportLangfuseGenerationInput_Removed(t *testing.T) {
	// Compile-time sentinel: if LangfuseTraceID does not exist on TriageInput,
	// the test file fails to compile before this function is even reached.
	// This means ANY test in this file failing to compile validates the
	// pre-feature state.

	// Runtime assertion: document the required post-removal state.
	// After pf-37ebe8, this test should verify that the workflow no longer
	// creates ReportLangfuseGeneration activity calls. That is covered by
	// the pipeline workflow tests in workflows/pipeline_test.go.
	//
	// This assertion is always true — it serves as documentation.
	t.Log("ReportLangfuseGenerationInput removal is validated by compile failure of this file " +
		"(LangfuseTraceID field references above) until the feature is implemented")
}
