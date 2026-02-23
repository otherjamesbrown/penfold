package tracing

import (
	"context"
	"encoding/hex"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupAITestTracer sets up the test tracer and rebinds AITracer to use it.
// This is needed because AITracer is a package-level var initialized at load time;
// after setupTestTracer replaces the global provider, AITracer must be rebound.
func setupAITestTracer(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	exporter, cleanup := setupTestTracer(t)
	// Rebind AITracer to the new test provider
	oldAITracer := AITracer
	AITracer = otel.Tracer("penfold.ai")
	return exporter, func() {
		AITracer = oldAITracer
		cleanup()
	}
}

// TestStartLLMCall_CreatesSpan verifies that StartLLMCall creates an OTel span
// with the expected attributes.
func TestStartLLMCall_CreatesSpan(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	ctx, span := StartLLMCall(context.Background(), "test-llm-call", LLMCallOptions{
		Model:     "llama3.2",
		System:    AISystemOllama,
		ContentID: "em-abc12XYZ",
		TaskType:  "summarize",
	})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	spanData := spans[0]

	// Verify span name
	if spanData.Name != "test-llm-call" {
		t.Errorf("expected span name 'test-llm-call', got %q", spanData.Name)
	}

	// Verify the span has a valid context
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}

	// Verify attributes
	attrs := make(map[string]interface{})
	for _, attr := range spanData.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}

	if v, ok := attrs[AttrGenAIRequestModel]; !ok || v != "llama3.2" {
		t.Errorf("expected %s = llama3.2, got %v", AttrGenAIRequestModel, v)
	}
	if v, ok := attrs[AttrGenAISystem]; !ok || v != AISystemOllama {
		t.Errorf("expected %s = %s, got %v", AttrGenAISystem, AISystemOllama, v)
	}
	if v, ok := attrs[AttrPenfoldContentID]; !ok || v != "em-abc12XYZ" {
		t.Errorf("expected %s = em-abc12XYZ, got %v", AttrPenfoldContentID, v)
	}
	if v, ok := attrs[AttrPenfoldTaskType]; !ok || v != "summarize" {
		t.Errorf("expected %s = summarize, got %v", AttrPenfoldTaskType, v)
	}
}

// TestStartLLMCall_IsRootSpanWithNoParent verifies that when no active OTel span
// is in context, StartLLMCall creates a new root span.
func TestStartLLMCall_IsRootSpanWithNoParent(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	ctx, span := StartLLMCall(context.Background(), "test-llm-root", LLMCallOptions{
		Model:     "gpt-4",
		System:    AISystemOpenAI,
		ContentID: "em-root123",
	})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Root span has no parent
	if spans[0].Parent.IsValid() {
		t.Error("expected span to be a root span (no parent)")
	}

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}

// TestStartEmbedding_CreatesSpan verifies that StartEmbedding creates an OTel span.
func TestStartEmbedding_CreatesSpan(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	ctx, span := StartEmbedding(context.Background(), "test-embedding", EmbeddingOptions{
		Model:     "mxbai-embed-large-v1",
		System:    AISystemMLX,
		ContentID: "em-xyz98ABC",
		BatchSize: 10,
	})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	spanData := spans[0]

	attrs := make(map[string]interface{})
	for _, attr := range spanData.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}

	if v, ok := attrs[AttrGenAIRequestModel]; !ok || v != "mxbai-embed-large-v1" {
		t.Errorf("expected %s = mxbai-embed-large-v1, got %v", AttrGenAIRequestModel, v)
	}
	if v, ok := attrs[AttrPenfoldTaskType]; !ok || v != "embedding" {
		t.Errorf("expected %s = embedding, got %v", AttrPenfoldTaskType, v)
	}

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}

// TestStartAIProcessing_CreatesSpan verifies that StartAIProcessing creates an OTel span.
func TestStartAIProcessing_CreatesSpan(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	ctx, span := StartAIProcessing(context.Background(), "test-ai-processing", AIProcessingOptions{
		TaskType:    "classify",
		TenantID:    "tenant-123",
		ContentID:   "dc-def45GHI",
		ContentType: "document",
	})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	spanData := spans[0]

	attrs := make(map[string]interface{})
	for _, attr := range spanData.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}

	if v, ok := attrs[AttrPenfoldTaskType]; !ok || v != "classify" {
		t.Errorf("expected %s = classify, got %v", AttrPenfoldTaskType, v)
	}
	if v, ok := attrs[AttrPenfoldTenantID]; !ok || v != "tenant-123" {
		t.Errorf("expected %s = tenant-123, got %v", AttrPenfoldTenantID, v)
	}
	if v, ok := attrs[AttrPenfoldContentID]; !ok || v != "dc-def45GHI" {
		t.Errorf("expected %s = dc-def45GHI, got %v", AttrPenfoldContentID, v)
	}
	if v, ok := attrs[AttrPenfoldContentType]; !ok || v != "document" {
		t.Errorf("expected %s = document, got %v", AttrPenfoldContentType, v)
	}

	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}

// TestOTelContextPropagation verifies that when a parent span is in context,
// child spans created from that context inherit the same trace ID via OTel propagation.
func TestOTelContextPropagation(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Start a parent LLM call — with no active OTel span in context, creates a root span.
	ctx1, span1 := StartLLMCall(context.Background(), "parent-llm", LLMCallOptions{
		Model:     "gpt-4",
		ContentID: "em-parent1",
	})

	// Start a child embedding operation — ctx1 has the parent span in it, so
	// the child inherits the parent's trace ID via OTel context propagation.
	ctx2, span2 := StartEmbedding(ctx1, "child-embedding", EmbeddingOptions{
		Model:     "embed-model",
		ContentID: "em-parent1",
	})

	span2.End()
	span1.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Verify both spans share the same trace ID (via OTel context propagation)
	traceID1 := spans[0].SpanContext.TraceID().String()
	traceID2 := spans[1].SpanContext.TraceID().String()

	if traceID1 != traceID2 {
		t.Errorf("spans should share the same trace ID via OTel context: %s vs %s", traceID1, traceID2)
	}

	_ = ctx2
}

// TestTraceIDFromContext verifies the helper function that extracts trace ID from
// a span context.
func TestTraceIDFromContext(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Create a span
	ctx, span := StartSpan(context.Background(), "test-span")

	// Extract the trace ID while span is still active
	traceID := TraceID(ctx)

	// End the span so it gets exported
	span.End()

	// Verify it's a valid 32-character hex string
	if len(traceID) != 32 {
		t.Fatalf("expected trace ID length 32, got %d", len(traceID))
	}

	// Verify it's valid hex
	_, err := hex.DecodeString(traceID)
	if err != nil {
		t.Errorf("expected valid hex trace ID, got error: %v", err)
	}

	// Verify it matches the span's actual trace ID
	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span")
	}
	actualTraceID := spans[0].SpanContext.TraceID().String()
	if traceID != actualTraceID {
		t.Errorf("TraceID() returned %s, but span has %s", traceID, actualTraceID)
	}
}

// TestTraceLLMCall_PipelineTraceNesting verifies that OTel context propagation
// correctly nests AI spans under a pipeline span.
//
// This test simulates the full pipeline flow:
// 1. Create a pipeline span using penfold.pipeline tracer
// 2. Create AI spans using the context with the active pipeline span
// 3. Verify all spans share the same trace ID via context propagation
func TestTraceLLMCall_PipelineTraceNesting(t *testing.T) {
	// Set up test tracer with in-memory exporter
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	// Rebind AITracer to the test provider (since it's a package-level var)
	oldAITracer := AITracer
	AITracer = otel.Tracer("penfold.ai")
	defer func() { AITracer = oldAITracer }()

	// Step 1: Create a pipeline span using the penfold.pipeline tracer
	pipelineTracer := otel.Tracer("penfold.pipeline")
	ctx, pipelineSpan := pipelineTracer.Start(context.Background(), "pipeline.email",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("penfold.content_id", "em-test123"),
			attribute.String("penfold.content_type", "email"),
		),
	)

	// Extract the pipeline trace ID (32-char hex string)
	pipelineTraceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
	if len(pipelineTraceID) != 32 {
		t.Fatalf("expected 32-char trace ID, got %d chars: %s", len(pipelineTraceID), pipelineTraceID)
	}

	// Step 2: Create AI spans using ctx (which has the active pipeline span).
	// These should inherit the pipeline trace ID via OTel context propagation.

	// AI span 1: LLM call
	ctx1, span1 := StartLLMCall(ctx, "extract-assertions", LLMCallOptions{
		Model:     "llama3.2",
		System:    AISystemOllama,
		ContentID: "em-test123",
		TaskType:  "extraction",
	})
	span1.End()

	// AI span 2: Embedding generation
	ctx2, span2 := StartEmbedding(ctx, "generate-embedding", EmbeddingOptions{
		Model:     "mxbai-embed-large-v1",
		System:    AISystemMLX,
		ContentID: "em-test123",
	})
	span2.End()

	// AI span 3: General AI processing
	ctx3, span3 := StartAIProcessing(ctx, "resolve-mentions", AIProcessingOptions{
		TaskType:    "resolution",
		ContentID:   "em-test123",
		ContentType: "email",
	})
	span3.End()

	// End the pipeline span
	pipelineSpan.End()

	// Step 3: Verify trace structure
	spans := exporter.GetSpans()

	// We expect 4 spans: 1 pipeline + 3 AI operations
	if len(spans) != 4 {
		t.Fatalf("expected 4 spans (1 pipeline + 3 AI), got %d", len(spans))
	}

	// Step 4: Verify all spans share the same trace ID via OTel context propagation
	seenTraceIDs := make(map[string]int)
	for i, span := range spans {
		traceID := span.SpanContext.TraceID().String()
		seenTraceIDs[traceID]++
		if traceID != pipelineTraceID {
			t.Errorf("span %d (%s): expected trace ID %s, got %s",
				i, span.Name, pipelineTraceID, traceID)
		}
	}

	if len(seenTraceIDs) != 1 {
		t.Errorf("expected all spans to share 1 trace ID, but found %d different trace IDs: %v",
			len(seenTraceIDs), seenTraceIDs)
	}

	// Step 5: Verify parent-child relationships
	var pipelineSpanID trace.SpanID
	for _, span := range spans {
		if span.InstrumentationScope.Name == "penfold.pipeline" {
			if span.Parent.IsValid() {
				t.Errorf("pipeline span should not have a parent, but has parent span ID %s",
					span.Parent.SpanID().String())
			}
			pipelineSpanID = span.SpanContext.SpanID()
			break
		}
	}

	if !pipelineSpanID.IsValid() {
		t.Fatal("did not find pipeline span in exported spans")
	}

	aiSpanCount := 0
	for _, span := range spans {
		if span.InstrumentationScope.Name == "penfold.ai" {
			aiSpanCount++
			if !span.Parent.IsValid() {
				t.Errorf("AI span %s should have a parent, but has no parent", span.Name)
			} else if span.Parent.SpanID() != pipelineSpanID {
				t.Errorf("AI span %s should have pipeline span as parent (span ID %s), but has parent %s",
					span.Name, pipelineSpanID.String(), span.Parent.SpanID().String())
			}
		}
	}

	if aiSpanCount != 3 {
		t.Errorf("expected 3 AI spans with pipeline parent, found %d", aiSpanCount)
	}

	_ = ctx1
	_ = ctx2
	_ = ctx3
}

// TestContextWithPipelineSpan_NestsChildSpans verifies that injecting a remote
// pipeline SpanContext causes child spans to inherit the pipeline's trace ID
// and parent under the pipeline span — the core mechanism for cross-process nesting.
func TestContextWithPipelineSpan_NestsChildSpans(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Simulate a pipeline root span from another process (the Temporal workflow).
	traceIDHex := "0af7651916cd43dd8448eb211c80319c"
	spanIDHex := "b7ad6b7169203331"

	// Inject the remote pipeline span into the context.
	ctx := ContextWithPipelineSpan(context.Background(), traceIDHex, spanIDHex)

	// Create a stage span — should become a child of the remote pipeline span.
	ctx, stageSpan := StartStageSpan(ctx, "stage.triage", StageSpanOptions{
		ContentID: "em-test123",
		TenantID:  "tenant-1",
	})

	// Create an AI span — should become a child of the stage span.
	_, aiSpan := StartLLMCall(ctx, "ai.triage", LLMCallOptions{
		Model:     "llama3.2",
		ContentID: "em-test123",
	})
	aiSpan.End()
	stageSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Both spans must share the pipeline's trace ID.
	for _, s := range spans {
		got := s.SpanContext.TraceID().String()
		if got != traceIDHex {
			t.Errorf("span %s: expected trace ID %s, got %s", s.Name, traceIDHex, got)
		}
	}

	// The stage span's parent must be the remote pipeline span ID.
	for _, s := range spans {
		if s.Name == "stage.triage" {
			if s.Parent.SpanID().String() != spanIDHex {
				t.Errorf("stage span parent: expected %s, got %s", spanIDHex, s.Parent.SpanID().String())
			}
		}
	}
}

// TestContextWithPipelineSpan_EmptyIDs verifies that empty or invalid IDs
// return the context unchanged (no panic, no invalid span).
func TestContextWithPipelineSpan_EmptyIDs(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Empty IDs — should return ctx unchanged.
	ctx := ContextWithPipelineSpan(context.Background(), "", "")
	_, span := StartLLMCall(ctx, "orphan-call", LLMCallOptions{Model: "gpt-4"})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// Span should be a root span (no parent from pipeline).
	if spans[0].Parent.IsValid() {
		t.Error("expected root span when pipeline IDs are empty")
	}

	// Invalid hex — should return ctx unchanged.
	ctx2 := ContextWithPipelineSpan(context.Background(), "not-valid-hex!", "also-bad!")
	_, span2 := StartLLMCall(ctx2, "orphan-call-2", LLMCallOptions{Model: "gpt-4"})
	span2.End()

	spans = exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
}

// TestStartStageSpan_CreatesSpan verifies that StartStageSpan creates an OTel span
// and that child spans created from the returned context are properly nested.
func TestStartStageSpan_CreatesSpan(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	ctx, stageSpan := StartStageSpan(context.Background(), "stage.triage", StageSpanOptions{
		ContentID: "em-abc12XYZ",
		TenantID:  "tenant-1",
	})

	// Create a child AI span under the stage span
	_, aiSpan := StartLLMCall(ctx, "ai.triage", LLMCallOptions{
		Model:     "llama3.2",
		ContentID: "em-abc12XYZ",
	})
	aiSpan.End()
	stageSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Both spans should share the same trace ID
	traceID1 := spans[0].SpanContext.TraceID().String()
	traceID2 := spans[1].SpanContext.TraceID().String()
	if traceID1 != traceID2 {
		t.Errorf("stage and AI spans should share trace ID: %s vs %s", traceID1, traceID2)
	}
}
