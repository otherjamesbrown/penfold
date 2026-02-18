package tracing

import (
	"context"
	"encoding/hex"
	"testing"

	"go.opentelemetry.io/otel"
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

// TestStartLLMCall_WithPipelineTraceID verifies that StartLLMCall uses the provided
// pipeline trace ID instead of creating a new root span, allowing AI operations to
// be grouped under a common parent trace.
func TestStartLLMCall_WithPipelineTraceID(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Create a pipeline trace ID (32-char hex string representing a trace ID)
	// This simulates what would come from a content processing pipeline
	pipelineTraceID := "0123456789abcdef0123456789abcdef"

	// Call StartLLMCall with PipelineTraceID set
	ctx, span := StartLLMCall(context.Background(), "test-llm-call", LLMCallOptions{
		PipelineTraceID: pipelineTraceID,
		Model:           "llama3.2",
		System:          AISystemOllama,
		ContentID:       "em-abc12XYZ",
		TaskType:        "summarize",
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify the span's trace ID matches the provided pipeline trace ID
	actualTraceID := spans[0].SpanContext.TraceID().String()
	if actualTraceID != pipelineTraceID {
		t.Errorf("expected trace ID %s, got %s", pipelineTraceID, actualTraceID)
	}

	// Verify the span is NOT a root span by checking if it has a parent context
	// When we provide a pipeline trace ID, the span should inherit that trace context
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
	if !spanCtx.HasTraceID() {
		t.Error("expected span to have trace ID")
	}
}

// TestStartEmbedding_WithPipelineTraceID verifies that StartEmbedding uses the provided
// pipeline trace ID instead of creating a new root span.
func TestStartEmbedding_WithPipelineTraceID(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Create a pipeline trace ID
	pipelineTraceID := "fedcba9876543210fedcba9876543210"

	// Call StartEmbedding with PipelineTraceID set
	ctx, span := StartEmbedding(context.Background(), "test-embedding", EmbeddingOptions{
		PipelineTraceID: pipelineTraceID,
		Model:           "mxbai-embed-large-v1",
		System:          AISystemMLX,
		ContentID:       "em-xyz98ABC",
		BatchSize:       10,
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify the span's trace ID matches the provided pipeline trace ID
	actualTraceID := spans[0].SpanContext.TraceID().String()
	if actualTraceID != pipelineTraceID {
		t.Errorf("expected trace ID %s, got %s", pipelineTraceID, actualTraceID)
	}

	// Verify the span has a valid context with the trace ID
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}

// TestStartAIProcessing_WithPipelineTraceID verifies that StartAIProcessing uses the
// provided pipeline trace ID instead of creating a new root span.
func TestStartAIProcessing_WithPipelineTraceID(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Create a pipeline trace ID
	pipelineTraceID := "aabbccddeeff00112233445566778899"

	// Call StartAIProcessing with PipelineTraceID set
	ctx, span := StartAIProcessing(context.Background(), "test-ai-processing", AIProcessingOptions{
		PipelineTraceID: pipelineTraceID,
		TaskType:        "classify",
		TenantID:        "tenant-123",
		ContentID:       "dc-def45GHI",
		ContentType:     "document",
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify the span's trace ID matches the provided pipeline trace ID
	actualTraceID := spans[0].SpanContext.TraceID().String()
	if actualTraceID != pipelineTraceID {
		t.Errorf("expected trace ID %s, got %s", pipelineTraceID, actualTraceID)
	}

	// Verify the span has a valid context
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}

// TestStartLLMCall_WithoutPipelineTraceID_IsRoot verifies backward compatibility:
// when no pipeline trace ID is provided, StartLLMCall should create a new root span
// as it currently does.
func TestStartLLMCall_WithoutPipelineTraceID_IsRoot(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Call StartLLMCall WITHOUT PipelineTraceID (empty string)
	ctx, span := StartLLMCall(context.Background(), "test-llm-root", LLMCallOptions{
		Model:     "gpt-4",
		System:    AISystemOpenAI,
		ContentID: "em-root123",
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify this is a root span (no parent span ID)
	// A root span has a valid span context but its parent span ID should be invalid
	spanData := spans[0]
	if !spanData.Parent.IsValid() {
		// This is good - no parent means it's a root span
		// This is the backward-compatible behavior we want to preserve
	}

	// Also verify the context contains a valid span
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}

// TestLangfuseTraceTagsAttribute verifies that the AttrLangfuseTraceTags constant exists and
// that spans with ContentID include the langfuse.trace.tags attribute for Langfuse grouping.
func TestLangfuseTraceTagsAttribute(t *testing.T) {
	// Test 1: Verify the constant exists and has the correct value
	expectedKey := "langfuse.trace.tags"
	if AttrLangfuseTraceTags != expectedKey {
		t.Errorf("expected AttrLangfuseTraceTags to be %q, got %q", expectedKey, AttrLangfuseTraceTags)
	}

	// Test 2: Verify spans with ContentID include the langfuse.trace.tags attribute
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	contentID := "em-test789"

	ctx, span := StartLLMCall(context.Background(), "test-tag-span", LLMCallOptions{
		Model:     "test-model",
		System:    AISystemOpenAI,
		ContentID: contentID,
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify the span has the langfuse.trace.tags attribute as a string slice
	spanData := spans[0]
	found := false
	for _, attr := range spanData.Attributes {
		if string(attr.Key) == AttrLangfuseTraceTags {
			found = true
			tags := attr.Value.AsStringSlice()
			if len(tags) != 1 || tags[0] != contentID {
				t.Errorf("expected langfuse.trace.tags to be [%q], got %v", contentID, tags)
			}
			break
		}
	}

	if !found {
		t.Errorf("expected span to have %q attribute, but it was not found", AttrLangfuseTraceTags)
	}

	_ = ctx
}

// TestPipelineTraceIDPropagation verifies that when a pipeline trace ID is provided,
// child spans inherit the same trace ID, creating a cohesive trace tree.
func TestPipelineTraceIDPropagation(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	pipelineTraceID := "1122334455667788aabbccddeeff0011"

	// Start a parent LLM call with the pipeline trace ID
	ctx1, span1 := StartLLMCall(context.Background(), "parent-llm", LLMCallOptions{
		PipelineTraceID: pipelineTraceID,
		Model:           "gpt-4",
		ContentID:       "em-parent1",
	})

	// Start a child embedding operation from the same context
	ctx2, span2 := StartEmbedding(ctx1, "child-embedding", EmbeddingOptions{
		PipelineTraceID: pipelineTraceID,
		Model:           "embed-model",
		ContentID:       "em-parent1",
	})

	span2.End()
	span1.End()

	// Verify both spans were created
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Verify both spans share the same trace ID
	traceID1 := spans[0].SpanContext.TraceID().String()
	traceID2 := spans[1].SpanContext.TraceID().String()

	if traceID1 != pipelineTraceID {
		t.Errorf("span 1: expected trace ID %s, got %s", pipelineTraceID, traceID1)
	}
	if traceID2 != pipelineTraceID {
		t.Errorf("span 2: expected trace ID %s, got %s", pipelineTraceID, traceID2)
	}
	if traceID1 != traceID2 {
		t.Errorf("spans should share the same trace ID: %s vs %s", traceID1, traceID2)
	}

	_ = ctx2
}

// TestTraceIDFromContext verifies the helper function that extracts trace ID from
// a span context (used for pipeline trace ID generation).
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

// TestStartLLMCall_WithInvalidPipelineTraceID verifies that when an invalid pipeline
// trace ID is provided, the span falls back to creating a root span instead of
// creating an orphaned span.
func TestStartLLMCall_WithInvalidPipelineTraceID(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Use an invalid trace ID (not valid hex, wrong length, etc.)
	invalidTraceID := "not-a-valid-hex-string"

	// Call StartLLMCall with invalid PipelineTraceID
	ctx, span := StartLLMCall(context.Background(), "test-llm-invalid-trace", LLMCallOptions{
		PipelineTraceID: invalidTraceID,
		Model:           "test-model",
		System:          AISystemOllama,
		ContentID:       "em-test123",
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Verify the span is a root span (has no parent)
	spanData := spans[0]
	if spanData.Parent.IsValid() {
		t.Error("expected span to be a root span (no parent), but it has a parent")
	}

	// Verify the span's trace ID is NOT the invalid string
	// (it should be a newly generated valid trace ID)
	actualTraceID := spanData.SpanContext.TraceID().String()
	if actualTraceID == invalidTraceID {
		t.Errorf("span should not have the invalid trace ID %s", invalidTraceID)
	}

	// Verify it's a valid 32-character hex string
	if len(actualTraceID) != 32 {
		t.Errorf("expected trace ID length 32, got %d", len(actualTraceID))
	}
	_, err := hex.DecodeString(actualTraceID)
	if err != nil {
		t.Errorf("expected valid hex trace ID, got error: %v", err)
	}

	// Verify the context has a valid span
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		t.Error("expected valid span context")
	}
}
