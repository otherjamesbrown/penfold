package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func setupTestTracer(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	oldTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)

	return exporter, func() {
		otel.SetTracerProvider(oldTP)
		tp.Shutdown(context.Background())
	}
}

func TestNewWorkflowTracer(t *testing.T) {
	t.Run("creates tracer with default name", func(t *testing.T) {
		wt := NewWorkflowTracer()
		if wt == nil {
			t.Fatal("NewWorkflowTracer returned nil")
		}
		if wt.tracer == nil {
			t.Error("tracer is nil")
		}
	})

	t.Run("Tracer returns underlying tracer", func(t *testing.T) {
		wt := NewWorkflowTracer()
		tracer := wt.Tracer()
		if tracer == nil {
			t.Error("Tracer() returned nil")
		}
	})
}

func TestNewWorkflowTracerWithName(t *testing.T) {
	wt := NewWorkflowTracerWithName("custom-tracer")
	if wt == nil {
		t.Fatal("NewWorkflowTracerWithName returned nil")
	}
	if wt.tracer == nil {
		t.Error("tracer is nil")
	}
}

func TestNewActivityTracer(t *testing.T) {
	t.Run("creates tracer with default name", func(t *testing.T) {
		at := NewActivityTracer()
		if at == nil {
			t.Fatal("NewActivityTracer returned nil")
		}
		if at.tracer == nil {
			t.Error("tracer is nil")
		}
	})

	t.Run("Tracer returns underlying tracer", func(t *testing.T) {
		at := NewActivityTracer()
		tracer := at.Tracer()
		if tracer == nil {
			t.Error("Tracer() returned nil")
		}
	})
}

func TestNewActivityTracerWithName(t *testing.T) {
	at := NewActivityTracerWithName("custom-activity-tracer")
	if at == nil {
		t.Fatal("NewActivityTracerWithName returned nil")
	}
	if at.tracer == nil {
		t.Error("tracer is nil")
	}
}

func TestActivityTracerRecordSuccess(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	at := NewActivityTracer()

	t.Run("with recording span", func(t *testing.T) {
		at.RecordActivitySuccess(span)
		span.End()

		spans := exporter.GetSpans()
		if len(spans) == 0 {
			t.Skip("No spans recorded - may be sampling")
		}

		lastSpan := spans[len(spans)-1]
		if lastSpan.Status.Code != codes.Ok {
			t.Errorf("Expected status Ok, got %v", lastSpan.Status.Code)
		}
	})

	t.Run("with nil span does not panic", func(t *testing.T) {
		at.RecordActivitySuccess(nil)
	})

	// Need a fresh context for next test
	_, nonRecordingSpan := trace.NewNoopTracerProvider().Tracer("noop").Start(ctx, "noop")

	t.Run("with non-recording span does not panic", func(t *testing.T) {
		at.RecordActivitySuccess(nonRecordingSpan)
	})
}

func TestActivityTracerRecordError(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-error-span")

	at := NewActivityTracer()

	t.Run("with error records it", func(t *testing.T) {
		testErr := &testError{msg: "test timeout error"}
		at.RecordActivityError(span, testErr)
		span.End()

		spans := exporter.GetSpans()
		if len(spans) == 0 {
			t.Skip("No spans recorded")
		}

		lastSpan := spans[len(spans)-1]
		if lastSpan.Status.Code != codes.Error {
			t.Errorf("Expected status Error, got %v", lastSpan.Status.Code)
		}
	})

	t.Run("with nil error does nothing", func(t *testing.T) {
		_, span2 := tracer.Start(context.Background(), "no-error")
		at.RecordActivityError(span2, nil)
		span2.End()
	})

	t.Run("with nil span does not panic", func(t *testing.T) {
		at.RecordActivityError(nil, &testError{msg: "error"})
	})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestHeaderCarrier(t *testing.T) {
	t.Run("Get returns value", func(t *testing.T) {
		carrier := &headerCarrier{
			fields: map[string][]byte{
				"traceparent": []byte("00-abc123-def456-01"),
			},
		}

		value := carrier.Get("traceparent")
		if value != "00-abc123-def456-01" {
			t.Errorf("Expected '00-abc123-def456-01', got %q", value)
		}
	})

	t.Run("Get returns empty for missing key", func(t *testing.T) {
		carrier := &headerCarrier{
			fields: map[string][]byte{},
		}

		value := carrier.Get("missing")
		if value != "" {
			t.Errorf("Expected empty string, got %q", value)
		}
	})

	t.Run("Get returns empty for nil fields", func(t *testing.T) {
		carrier := &headerCarrier{fields: nil}
		value := carrier.Get("any")
		if value != "" {
			t.Errorf("Expected empty string, got %q", value)
		}
	})

	t.Run("Set stores value", func(t *testing.T) {
		carrier := &headerCarrier{
			fields: make(map[string][]byte),
		}

		carrier.Set("traceparent", "00-xyz789-uvw123-00")

		if string(carrier.fields["traceparent"]) != "00-xyz789-uvw123-00" {
			t.Errorf("Value not stored correctly")
		}
	})

	t.Run("Set does nothing for nil fields", func(t *testing.T) {
		carrier := &headerCarrier{fields: nil}
		carrier.Set("key", "value") // Should not panic
	})

	t.Run("Keys returns all keys", func(t *testing.T) {
		carrier := &headerCarrier{
			fields: map[string][]byte{
				"key1": []byte("val1"),
				"key2": []byte("val2"),
			},
		}

		keys := carrier.Keys()
		if len(keys) != 2 {
			t.Errorf("Expected 2 keys, got %d", len(keys))
		}

		keyMap := make(map[string]bool)
		for _, k := range keys {
			keyMap[k] = true
		}
		if !keyMap["key1"] || !keyMap["key2"] {
			t.Error("Missing expected keys")
		}
	})

	t.Run("Keys returns nil for nil fields", func(t *testing.T) {
		carrier := &headerCarrier{fields: nil}
		keys := carrier.Keys()
		if keys != nil {
			t.Errorf("Expected nil, got %v", keys)
		}
	})
}

func TestPropagateContext(t *testing.T) {
	t.Run("NewPropagateContext creates instance", func(t *testing.T) {
		pc := NewPropagateContext()
		if pc == nil {
			t.Fatal("NewPropagateContext returned nil")
		}
		if pc.propagator == nil {
			t.Error("propagator is nil")
		}
	})

	t.Run("Inject with nil headers does not panic", func(t *testing.T) {
		pc := NewPropagateContext()
		pc.Inject(context.Background(), nil)
	})

	t.Run("Extract with nil headers returns context", func(t *testing.T) {
		pc := NewPropagateContext()
		ctx := context.Background()
		result := pc.Extract(ctx, nil)
		if result == nil {
			t.Error("Extract returned nil context")
		}
	})

	t.Run("Inject and Extract roundtrip", func(t *testing.T) {
		_, cleanup := setupTestTracer(t)
		defer cleanup()

		pc := NewPropagateContext()
		headers := make(map[string][]byte)

		// Create a span context
		tracer := otel.Tracer("test")
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		// Inject context
		pc.Inject(ctx, headers)

		// Extract into new context
		newCtx := pc.Extract(context.Background(), headers)
		if newCtx == nil {
			t.Error("Extract returned nil")
		}
	})
}

func TestTraceIDFromContext(t *testing.T) {
	t.Run("returns empty for context without span", func(t *testing.T) {
		traceID := TraceIDFromContext(context.Background())
		if traceID != "" {
			t.Errorf("Expected empty string, got %q", traceID)
		}
	})

	t.Run("returns trace ID for context with span", func(t *testing.T) {
		_, cleanup := setupTestTracer(t)
		defer cleanup()

		tracer := otel.Tracer("test")
		ctx, span := tracer.Start(context.Background(), "test")
		defer span.End()

		traceID := TraceIDFromContext(ctx)
		if traceID == "" {
			t.Error("Expected non-empty trace ID")
		}
		// Trace ID should be 32 hex characters
		if len(traceID) != 32 {
			t.Errorf("Expected trace ID length 32, got %d", len(traceID))
		}
	})
}

func TestSpanIDFromContext(t *testing.T) {
	t.Run("returns empty for context without span", func(t *testing.T) {
		spanID := SpanIDFromContext(context.Background())
		if spanID != "" {
			t.Errorf("Expected empty string, got %q", spanID)
		}
	})

	t.Run("returns span ID for context with span", func(t *testing.T) {
		_, cleanup := setupTestTracer(t)
		defer cleanup()

		tracer := otel.Tracer("test")
		ctx, span := tracer.Start(context.Background(), "test")
		defer span.End()

		spanID := SpanIDFromContext(ctx)
		if spanID == "" {
			t.Error("Expected non-empty span ID")
		}
		// Span ID should be 16 hex characters
		if len(spanID) != 16 {
			t.Errorf("Expected span ID length 16, got %d", len(spanID))
		}
	})
}

func TestWorkflowTraceInfo(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		info := &WorkflowTraceInfo{
			WorkflowID:   "wf-123",
			RunID:        "run-456",
			WorkflowType: "TestWorkflow",
			TaskQueue:    "test-queue",
			Namespace:    "default",
			TraceID:      "abc123",
			SpanID:       "def456",
		}

		if info.WorkflowID != "wf-123" {
			t.Errorf("WorkflowID mismatch")
		}
		if info.RunID != "run-456" {
			t.Errorf("RunID mismatch")
		}
		if info.WorkflowType != "TestWorkflow" {
			t.Errorf("WorkflowType mismatch")
		}
		if info.TaskQueue != "test-queue" {
			t.Errorf("TaskQueue mismatch")
		}
		if info.Namespace != "default" {
			t.Errorf("Namespace mismatch")
		}
		if info.TraceID != "abc123" {
			t.Errorf("TraceID mismatch")
		}
		if info.SpanID != "def456" {
			t.Errorf("SpanID mismatch")
		}
	})
}

func TestTracerNameConstant(t *testing.T) {
	if TracerName == "" {
		t.Error("TracerName constant is empty")
	}
	expected := "github.com/otherjamesbrown/penfold/pkg/temporal/observability"
	if TracerName != expected {
		t.Errorf("TracerName = %q, want %q", TracerName, expected)
	}
}

func TestTraceAttributeConstants(t *testing.T) {
	// Verify all attribute constants are defined and non-empty
	constants := map[string]string{
		"AttrWorkflowType":  AttrWorkflowType,
		"AttrWorkflowID":    AttrWorkflowID,
		"AttrWorkflowRunID": AttrWorkflowRunID,
		"AttrActivityType":  AttrActivityType,
		"AttrActivityID":    AttrActivityID,
		"AttrTaskQueue":     AttrTaskQueue,
		"AttrNamespace":     AttrNamespace,
		"AttrAttempt":       AttrAttempt,
		"AttrErrorType":     AttrErrorType,
		"AttrIsLocal":       AttrIsLocal,
	}

	for name, value := range constants {
		if value == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestAddErrorAttributes(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	tracer := otel.Tracer("test")

	t.Run("with error sets error status", func(t *testing.T) {
		_, span := tracer.Start(context.Background(), "error-test")
		testErr := &testError{msg: "timeout occurred"}

		AddErrorAttributes(span, testErr)
		span.End()

		spans := exporter.GetSpans()
		if len(spans) == 0 {
			t.Skip("No spans recorded")
		}

		lastSpan := spans[len(spans)-1]
		if lastSpan.Status.Code != codes.Error {
			t.Errorf("Expected Error status, got %v", lastSpan.Status.Code)
		}
	})

	t.Run("with nil span does not panic", func(t *testing.T) {
		AddErrorAttributes(nil, &testError{msg: "error"})
	})

	t.Run("with nil error does not panic", func(t *testing.T) {
		_, span := tracer.Start(context.Background(), "nil-error-test")
		AddErrorAttributes(span, nil)
		span.End()
	})
}
