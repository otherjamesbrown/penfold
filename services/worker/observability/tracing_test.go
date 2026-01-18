package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewTracingInterceptor(t *testing.T) {
	interceptor := NewTracingInterceptor()

	if interceptor == nil {
		t.Fatal("NewTracingInterceptor returned nil")
	}

	if interceptor.tracer == nil {
		t.Error("tracer is nil")
	}
}

func TestHeaderCarrier(t *testing.T) {
	t.Run("Get and Set", func(t *testing.T) {
		fields := make(map[string][]byte)
		carrier := &headerCarrier{fields: fields}

		// Test Set
		carrier.Set("traceparent", "00-trace-id-span-id-01")

		// Test Get
		value := carrier.Get("traceparent")
		if value != "00-trace-id-span-id-01" {
			t.Errorf("expected '00-trace-id-span-id-01', got '%s'", value)
		}

		// Test Get non-existent key
		value = carrier.Get("nonexistent")
		if value != "" {
			t.Errorf("expected empty string for non-existent key, got '%s'", value)
		}
	})

	t.Run("Keys", func(t *testing.T) {
		fields := map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
		}
		carrier := &headerCarrier{fields: fields}

		keys := carrier.Keys()
		if len(keys) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keys))
		}
	})

	t.Run("nil fields", func(t *testing.T) {
		carrier := &headerCarrier{fields: nil}

		// Should not panic
		value := carrier.Get("key")
		if value != "" {
			t.Errorf("expected empty string, got '%s'", value)
		}

		carrier.Set("key", "value") // Should not panic

		keys := carrier.Keys()
		if keys != nil {
			t.Errorf("expected nil keys, got %v", keys)
		}
	})
}

func TestInjectTraceContext(t *testing.T) {
	// Set up test tracer
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Create a context with a span
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	// Test injection
	header := make(map[string][]byte)
	InjectTraceContext(ctx, header)

	// Should have injected traceparent header
	if _, ok := header["traceparent"]; !ok {
		t.Error("expected traceparent header to be injected")
	}
}

func TestInjectTraceContext_NilHeader(t *testing.T) {
	// Should not panic with nil header
	InjectTraceContext(context.Background(), nil)
}

func TestTraceIDFromContext(t *testing.T) {
	t.Run("with valid span", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exporter),
		)

		tracer := tp.Tracer("test")
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		traceID := TraceIDFromContext(ctx)
		if traceID == "" {
			t.Error("expected non-empty trace ID")
		}
	})

	t.Run("without span", func(t *testing.T) {
		traceID := TraceIDFromContext(context.Background())
		if traceID != "" {
			t.Errorf("expected empty trace ID, got '%s'", traceID)
		}
	})
}

func TestSpanIDFromContext(t *testing.T) {
	t.Run("with valid span", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exporter),
		)

		tracer := tp.Tracer("test")
		ctx, span := tracer.Start(context.Background(), "test-span")
		defer span.End()

		spanID := SpanIDFromContext(ctx)
		if spanID == "" {
			t.Error("expected non-empty span ID")
		}
	})

	t.Run("without span", func(t *testing.T) {
		spanID := SpanIDFromContext(context.Background())
		if spanID != "" {
			t.Errorf("expected empty span ID, got '%s'", spanID)
		}
	})
}

func TestWorkflowTraceInfo(t *testing.T) {
	info := &WorkflowTraceInfo{
		WorkflowID:   "wf-123",
		RunID:        "run-456",
		WorkflowType: "TestWorkflow",
		TaskQueue:    "test-queue",
		Namespace:    "default",
		TraceID:      "trace-789",
		SpanID:       "span-abc",
	}

	if info.WorkflowID != "wf-123" {
		t.Errorf("expected WorkflowID 'wf-123', got '%s'", info.WorkflowID)
	}
	if info.RunID != "run-456" {
		t.Errorf("expected RunID 'run-456', got '%s'", info.RunID)
	}
	if info.WorkflowType != "TestWorkflow" {
		t.Errorf("expected WorkflowType 'TestWorkflow', got '%s'", info.WorkflowType)
	}
}
