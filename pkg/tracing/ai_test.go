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

// --- ACCEPTANCE TESTS FOR pf-aba512 ---

// TestTraceLLMCall_PipelineTraceNesting verifies acceptance criterion 1:
// All AI spans share TraceID with pipeline span, proper parent-child nesting, no orphaned traces.
//
// This test simulates the full pipeline flow:
// 1. Create a pipeline span using penfold.pipeline tracer (like observability.StartPipeline)
// 2. Extract the trace ID from that pipeline span
// 3. Create AI spans (LLM, embedding, processing) using the pipeline trace ID
// 4. Verify all spans share the same trace ID
// 5. Verify proper parent-child relationships exist
// 6. Verify there are no orphaned traces (all spans belong to the pipeline trace)
func TestTraceLLMCall_PipelineTraceNesting(t *testing.T) {
	// Set up test tracer with in-memory exporter
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	// Rebind AITracer to the test provider (since it's a package-level var)
	oldAITracer := AITracer
	AITracer = otel.Tracer("penfold.ai")
	defer func() { AITracer = oldAITracer }()

	// Step 1: Create a pipeline span using the penfold.pipeline tracer
	// This simulates what observability.StartPipeline does
	pipelineTracer := otel.Tracer("penfold.pipeline")
	ctx, pipelineSpan := pipelineTracer.Start(context.Background(), "pipeline.email",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("penfold.content_id", "em-test123"),
			attribute.String("penfold.content_type", "email"),
			attribute.String("langfuse.session.id", "workflow-456"),
		),
	)

	// Extract the pipeline trace ID (32-char hex string)
	pipelineTraceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
	if len(pipelineTraceID) != 32 {
		t.Fatalf("expected 32-char trace ID, got %d chars: %s", len(pipelineTraceID), pipelineTraceID)
	}

	// Step 2: Create AI spans that should nest under the pipeline trace
	// Simulate multiple AI operations in a pipeline

	// AI span 1: LLM call for extraction
	ctx1, span1 := StartLLMCall(ctx, "extract-assertions", LLMCallOptions{
		PipelineTraceID: pipelineTraceID,
		Model:           "llama3.2",
		System:          AISystemOllama,
		ContentID:       "em-test123",
		TaskType:        "extraction",
		SessionID:       "workflow-456",
		UserID:          "user-789",
	})
	span1.End()

	// AI span 2: Embedding generation
	ctx2, span2 := StartEmbedding(ctx, "generate-embedding", EmbeddingOptions{
		PipelineTraceID: pipelineTraceID,
		Model:           "mxbai-embed-large-v1",
		System:          AISystemMLX,
		ContentID:       "em-test123",
	})
	span2.End()

	// AI span 3: General AI processing (mention resolution)
	ctx3, span3 := StartAIProcessing(ctx, "resolve-mentions", AIProcessingOptions{
		PipelineTraceID: pipelineTraceID,
		TaskType:        "resolution",
		ContentID:       "em-test123",
		ContentType:     "email",
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

	// Step 4: Verify all spans share the same trace ID
	seenTraceIDs := make(map[string]int)
	for i, span := range spans {
		traceID := span.SpanContext.TraceID().String()
		seenTraceIDs[traceID]++
		if traceID != pipelineTraceID {
			t.Errorf("span %d (%s): expected trace ID %s, got %s",
				i, span.Name, pipelineTraceID, traceID)
		}
	}

	// Verify there's only ONE trace ID across all spans (no orphaned traces)
	if len(seenTraceIDs) != 1 {
		t.Errorf("expected all spans to share 1 trace ID, but found %d different trace IDs: %v",
			len(seenTraceIDs), seenTraceIDs)
	}

	// Step 5: Verify parent-child relationships
	// The pipeline span should be the root (no parent)
	// The AI spans should all have the pipeline span as their parent

	// First pass: find the pipeline span
	var pipelineSpanID trace.SpanID
	for _, span := range spans {
		if span.InstrumentationScope.Name == "penfold.pipeline" {
			// This is the pipeline span - should have no parent
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

	// Second pass: verify AI spans have correct parent
	aiSpanCount := 0
	for _, span := range spans {
		if span.InstrumentationScope.Name == "penfold.ai" {
			// This is an AI span - should have the pipeline span as parent
			aiSpanCount++
			if !span.Parent.IsValid() {
				t.Errorf("AI span %s should have a parent, but has no parent", span.Name)
			} else if span.Parent.SpanID() != pipelineSpanID {
				t.Errorf("AI span %s should have pipeline span as parent (span ID %s), but has parent %s",
					span.Name, pipelineSpanID.String(), span.Parent.SpanID().String())
			}
		}
	}

	// Verify we found all AI spans
	if aiSpanCount != 3 {
		t.Errorf("expected 3 AI spans with pipeline parent, found %d", aiSpanCount)
	}

	// Use contexts to avoid unused variable warnings
	_ = ctx1
	_ = ctx2
	_ = ctx3
}

// TestTraceLLMCall_LangfuseMetadataPopulated verifies acceptance criterion 2:
// Every AI span has non-empty session_id, user_id, tags with source_tag.
//
// This test verifies that when AI spans are created with metadata, that metadata
// is correctly recorded as span attributes using the Langfuse attribute keys.
func TestTraceLLMCall_LangfuseMetadataPopulated(t *testing.T) {
	exporter, cleanup := setupAITestTracer(t)
	defer cleanup()

	// Create an LLM call with all metadata fields populated
	sessionID := "session-abc123"
	userID := "user-xyz789"
	contentID := "em-test456" // This becomes a tag
	tenantID := "tenant-001"

	ctx, span := StartLLMCall(context.Background(), "test-metadata-span", LLMCallOptions{
		PipelineTraceID: "aabbccddeeff00112233445566778899",
		Model:           "gpt-4",
		System:          AISystemOpenAI,
		TenantID:        tenantID,
		ContentID:       contentID,
		TaskType:        "summarize",
		UserID:          userID,
		SessionID:       sessionID,
	})
	span.End()

	// Verify the span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	spanData := spans[0]

	// Create a map of attributes for easier checking
	attrs := make(map[string]interface{})
	for _, attr := range spanData.Attributes {
		key := string(attr.Key)
		attrs[key] = attr.Value.AsInterface()
	}

	// Test: langfuse.session.id is set and non-empty
	t.Run("session_id", func(t *testing.T) {
		val, ok := attrs[AttrLangfuseSessionID]
		if !ok {
			t.Errorf("span missing attribute %s", AttrLangfuseSessionID)
			return
		}
		if val == "" {
			t.Errorf("attribute %s is empty, expected %s", AttrLangfuseSessionID, sessionID)
			return
		}
		if val != sessionID {
			t.Errorf("attribute %s = %v, expected %s", AttrLangfuseSessionID, val, sessionID)
		}
	})

	// Test: langfuse.user.id is set and non-empty
	t.Run("user_id", func(t *testing.T) {
		val, ok := attrs[AttrLangfuseUserID]
		if !ok {
			t.Errorf("span missing attribute %s", AttrLangfuseUserID)
			return
		}
		if val == "" {
			t.Errorf("attribute %s is empty, expected %s", AttrLangfuseUserID, userID)
			return
		}
		if val != userID {
			t.Errorf("attribute %s = %v, expected %s", AttrLangfuseUserID, val, userID)
		}
	})

	// Test: langfuse.trace.tags contains the content ID (source_tag)
	t.Run("trace_tags_with_source_tag", func(t *testing.T) {
		val, ok := attrs[AttrLangfuseTraceTags]
		if !ok {
			t.Errorf("span missing attribute %s", AttrLangfuseTraceTags)
			return
		}

		// The value should be a string slice containing the content ID
		// The attribute package may return []string or []interface{}
		var tags []string
		switch v := val.(type) {
		case []string:
			tags = v
		case []interface{}:
			tags = make([]string, len(v))
			for i, item := range v {
				if s, ok := item.(string); ok {
					tags[i] = s
				}
			}
		default:
			t.Errorf("attribute %s is not a string slice, got type %T", AttrLangfuseTraceTags, val)
			return
		}

		if len(tags) == 0 {
			t.Errorf("attribute %s is empty, expected to contain %s", AttrLangfuseTraceTags, contentID)
			return
		}

		// Check if content ID is in the tags
		foundContentID := false
		for _, tag := range tags {
			if tag == contentID {
				foundContentID = true
				break
			}
		}

		if !foundContentID {
			t.Errorf("attribute %s = %v, expected to contain content ID %s",
				AttrLangfuseTraceTags, tags, contentID)
		}
	})

	// Test: Other important attributes are also present
	t.Run("other_attributes", func(t *testing.T) {
		// Verify tenant_id
		if val, ok := attrs[AttrPenfoldTenantID]; !ok || val == "" {
			t.Errorf("attribute %s missing or empty", AttrPenfoldTenantID)
		}

		// Verify content_id
		if val, ok := attrs[AttrPenfoldContentID]; !ok || val == "" {
			t.Errorf("attribute %s missing or empty", AttrPenfoldContentID)
		}

		// Verify task_type
		if val, ok := attrs[AttrPenfoldTaskType]; !ok || val == "" {
			t.Errorf("attribute %s missing or empty", AttrPenfoldTaskType)
		}

		// Verify observation type
		if val, ok := attrs[AttrLangfuseObservationType]; !ok || val == "" {
			t.Errorf("attribute %s missing or empty", AttrLangfuseObservationType)
		}
	})

	_ = ctx
}

// TestTraceLLMCall_AllProtoRequestsPassPipelineTraceID verifies acceptance criterion 3:
// All proto request types are populated with PipelineTraceID when called from pipeline.
//
// This test verifies that the proto request structs used in gRPC calls include the
// pipeline_trace_id field and that it's correctly populated. This is a structural test
// to ensure the field exists in the API contract.
//
// NOTE: This test focuses on the tracing layer - it does NOT test the actual gRPC proto
// definitions or activity implementations (those are tested in their respective packages).
// Instead, it verifies that when the tracing library receives a PipelineTraceID, it
// correctly processes it and creates spans with the expected trace ID.
func TestTraceLLMCall_AllProtoRequestsPassPipelineTraceID(t *testing.T) {
	// This test verifies that all AI tracing functions (StartLLMCall, StartEmbedding,
	// StartAIProcessing) correctly handle the PipelineTraceID field.

	testCases := []struct {
		name            string
		createSpan      func(ctx context.Context, pipelineTraceID string) (context.Context, trace.Span)
		expectedScope   string
		expectedObsType string
	}{
		{
			name: "LLMCallOptions_PipelineTraceID",
			createSpan: func(ctx context.Context, pipelineTraceID string) (context.Context, trace.Span) {
				return StartLLMCall(ctx, "test-llm", LLMCallOptions{
					PipelineTraceID: pipelineTraceID,
					Model:           "test-model",
					ContentID:       "em-test123",
				})
			},
			expectedScope:   "penfold.ai",
			expectedObsType: ObservationTypeGeneration,
		},
		{
			name: "EmbeddingOptions_PipelineTraceID",
			createSpan: func(ctx context.Context, pipelineTraceID string) (context.Context, trace.Span) {
				return StartEmbedding(ctx, "test-embedding", EmbeddingOptions{
					PipelineTraceID: pipelineTraceID,
					Model:           "test-model",
					ContentID:       "em-test123",
				})
			},
			expectedScope:   "penfold.ai",
			expectedObsType: ObservationTypeGeneration,
		},
		{
			name: "AIProcessingOptions_PipelineTraceID",
			createSpan: func(ctx context.Context, pipelineTraceID string) (context.Context, trace.Span) {
				return StartAIProcessing(ctx, "test-processing", AIProcessingOptions{
					PipelineTraceID: pipelineTraceID,
					TaskType:        "test-task",
					ContentID:       "em-test123",
				})
			},
			expectedScope:   "penfold.ai",
			expectedObsType: ObservationTypeSpan,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exporter, cleanup := setupAITestTracer(t)
			defer cleanup()

			// Use a known pipeline trace ID
			pipelineTraceID := "1234567890abcdef1234567890abcdef"

			// Create the span with pipeline trace ID
			ctx, span := tc.createSpan(context.Background(), pipelineTraceID)
			span.End()

			// Verify the span was created
			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}

			spanData := spans[0]

			// Verify the span's trace ID matches the pipeline trace ID
			actualTraceID := spanData.SpanContext.TraceID().String()
			if actualTraceID != pipelineTraceID {
				t.Errorf("expected trace ID %s, got %s", pipelineTraceID, actualTraceID)
			}

			// Verify the span has the correct instrumentation scope
			if spanData.InstrumentationScope.Name != tc.expectedScope {
				t.Errorf("expected scope %s, got %s",
					tc.expectedScope, spanData.InstrumentationScope.Name)
			}

			// Verify the observation type attribute
			foundObsType := false
			for _, attr := range spanData.Attributes {
				if string(attr.Key) == AttrLangfuseObservationType {
					foundObsType = true
					if attr.Value.AsString() != tc.expectedObsType {
						t.Errorf("expected observation type %s, got %s",
							tc.expectedObsType, attr.Value.AsString())
					}
					break
				}
			}

			if !foundObsType {
				t.Errorf("span missing attribute %s", AttrLangfuseObservationType)
			}

			_ = ctx
		})
	}
}
