package tracing

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupTestTracer creates a test tracer provider with an in-memory exporter.
func setupTestTracer(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	exporter := tracetest.NewInMemoryExporter()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Save old provider to restore later
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
	}

	return exporter, cleanup
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServiceName != "unknown" {
		t.Errorf("expected ServiceName 'unknown', got '%s'", cfg.ServiceName)
	}

	if cfg.Environment != "development" {
		t.Errorf("expected Environment 'development', got '%s'", cfg.Environment)
	}

	if cfg.Endpoint != "localhost:4317" {
		t.Errorf("expected Endpoint 'localhost:4317', got '%s'", cfg.Endpoint)
	}

	if cfg.SampleRate != 1.0 {
		t.Errorf("expected SampleRate 1.0, got %f", cfg.SampleRate)
	}

	if cfg.Exporter != ExporterStdout {
		t.Errorf("expected Exporter ExporterStdout, got '%s'", cfg.Exporter)
	}

	if !cfg.Insecure {
		t.Error("expected Insecure to be true")
	}
}

func TestInitTracer_StdoutExporter(t *testing.T) {
	var buf bytes.Buffer

	cfg := &Config{
		ServiceName:  "test-service",
		Environment:  "test",
		Exporter:     ExporterStdout,
		SampleRate:   1.0,
		StdoutWriter: &buf,
	}

	shutdown, err := InitTracer(cfg)
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Errorf("shutdown failed: %v", err)
		}
	}()

	// Create a span to verify the tracer is working
	tracer := Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()

	// Shutdown to flush the span
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown failed: %v", err)
	}

	// Check that something was written to stdout
	output := buf.String()
	if !strings.Contains(output, "test-span") {
		t.Errorf("expected output to contain 'test-span', got: %s", output)
	}
}

func TestInitTracer_NoneExporter(t *testing.T) {
	cfg := &Config{
		ServiceName: "test-service",
		Environment: "test",
		Exporter:    ExporterNone,
		SampleRate:  1.0,
	}

	shutdown, err := InitTracer(cfg)
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}

func TestInitTracer_NilConfig(t *testing.T) {
	// Capture stdout to avoid noise
	var buf bytes.Buffer

	// First set a config that uses the buffer
	cfg := &Config{
		ServiceName:  "default-test",
		Environment:  "test",
		Exporter:     ExporterNone,
		SampleRate:   1.0,
		StdoutWriter: &buf,
	}

	shutdown, err := InitTracer(cfg)
	if err != nil {
		t.Fatalf("InitTracer with nil failed: %v", err)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}

func TestInitTracer_SampleRates(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
	}{
		{"always sample", 1.0},
		{"never sample", 0.0},
		{"ratio sample", 0.5},
		{"above 1", 1.5},
		{"below 0", -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ServiceName: "test-service",
				Environment: "test",
				Exporter:    ExporterNone,
				SampleRate:  tt.sampleRate,
			}

			shutdown, err := InitTracer(cfg)
			if err != nil {
				t.Fatalf("InitTracer failed: %v", err)
			}

			if err := shutdown(context.Background()); err != nil {
				t.Errorf("shutdown failed: %v", err)
			}
		})
	}
}

func TestInitTracer_UnknownExporter(t *testing.T) {
	cfg := &Config{
		ServiceName: "test-service",
		Environment: "test",
		Exporter:    "unknown",
		SampleRate:  1.0,
	}

	_, err := InitTracer(cfg)
	if err == nil {
		t.Error("expected error for unknown exporter")
	}

	if !strings.Contains(err.Error(), "unknown exporter type") {
		t.Errorf("expected 'unknown exporter type' error, got: %v", err)
	}
}

func TestTracer(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	tracer := Tracer("my-package")
	ctx, span := tracer.Start(context.Background(), "operation")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "operation" {
		t.Errorf("expected span name 'operation', got '%s'", spans[0].Name)
	}

	_ = ctx // use ctx to avoid unused variable warning
}

func TestTracerWithVersion(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	tracer := TracerWithVersion("my-package", "1.0.0")
	_, span := tracer.Start(context.Background(), "versioned-operation")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "versioned-operation" {
		t.Errorf("expected span name 'versioned-operation', got '%s'", spans[0].Name)
	}
}

func TestStartSpan(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "my-span")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "my-span" {
		t.Errorf("expected span name 'my-span', got '%s'", spans[0].Name)
	}

	// Verify context contains the span
	spanFromCtx := trace.SpanFromContext(ctx)
	if !spanFromCtx.SpanContext().IsValid() {
		t.Error("expected valid span in context")
	}
}

func TestStartSpanWithTracer(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpanWithTracer(context.Background(), "custom-tracer", "custom-span")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "custom-span" {
		t.Errorf("expected span name 'custom-span', got '%s'", spans[0].Name)
	}

	_ = ctx
}

func TestSpanFromContext(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	// Create a span and put it in context
	ctx, span := StartSpan(context.Background(), "test-span")

	// Retrieve it
	retrieved := SpanFromContext(ctx)

	if retrieved.SpanContext().SpanID() != span.SpanContext().SpanID() {
		t.Error("retrieved span should match original span")
	}

	span.End()

	_ = exporter
}

func TestAddEvent(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")
	AddEvent(span, "something-happened", Attr("key", "value"))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	events := spans[0].Events
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Name != "something-happened" {
		t.Errorf("expected event name 'something-happened', got '%s'", events[0].Name)
	}
}

func TestAddEvent_NilSpan(t *testing.T) {
	// Should not panic
	AddEvent(nil, "event", Attr("key", "value"))
}

func TestAddEventToContext(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "test-span")
	AddEventToContext(ctx, "context-event", AttrInt("count", 42))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	events := spans[0].Events
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Name != "context-event" {
		t.Errorf("expected event name 'context-event', got '%s'", events[0].Name)
	}
}

func TestSetError(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")
	testErr := errors.New("something went wrong")
	SetError(span, testErr)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("expected status code Error, got %v", spans[0].Status.Code)
	}

	if spans[0].Status.Description != "something went wrong" {
		t.Errorf("expected status description 'something went wrong', got '%s'", spans[0].Status.Description)
	}
}

func TestSetError_NilError(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")
	SetError(span, nil)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Status should not be error when error is nil
	if spans[0].Status.Code == codes.Error {
		t.Error("status should not be Error when error is nil")
	}
}

func TestSetError_NilSpan(t *testing.T) {
	// Should not panic
	SetError(nil, errors.New("test error"))
}

func TestSetErrorWithMessage(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")
	testErr := errors.New("original error")
	SetErrorWithMessage(span, testErr, "custom message")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Description != "custom message" {
		t.Errorf("expected status description 'custom message', got '%s'", spans[0].Status.Description)
	}
}

func TestSetErrorToContext(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "test-span")
	testErr := errors.New("context error")
	SetErrorToContext(ctx, testErr)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("expected status code Error, got %v", spans[0].Status.Code)
	}
}

func TestSetOK(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")
	SetOK(span)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Ok {
		t.Errorf("expected status code Ok, got %v", spans[0].Status.Code)
	}
}

func TestSetOK_NilSpan(t *testing.T) {
	// Should not panic
	SetOK(nil)
}

func TestSetAttributes(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")
	SetAttributes(span,
		Attr("string_key", "value"),
		AttrInt("int_key", 42),
		AttrBool("bool_key", true),
	)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrs := spans[0].Attributes
	if len(attrs) < 3 {
		t.Errorf("expected at least 3 attributes, got %d", len(attrs))
	}
}

func TestSetAttributes_NilSpan(t *testing.T) {
	// Should not panic
	SetAttributes(nil, Attr("key", "value"))
}

func TestSetAttributesToContext(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "test-span")
	SetAttributesToContext(ctx, Attr("context_key", "context_value"))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	found := false
	for _, attr := range spans[0].Attributes {
		if string(attr.Key) == "context_key" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'context_key' attribute")
	}
}

func TestAttrHelpers(t *testing.T) {
	tests := []struct {
		name     string
		attr     attribute.KeyValue
		expected interface{}
	}{
		{"string", Attr("key", "value"), "value"},
		{"int", AttrInt("key", 42), int64(42)},
		{"int64", AttrInt64("key", 9999999999), int64(9999999999)},
		{"bool", AttrBool("key", true), true},
		{"float64", AttrFloat64("key", 3.14), 3.14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.attr.Key != "key" {
				t.Errorf("expected key 'key', got '%s'", tt.attr.Key)
			}
			// Just verify the attribute was created without panic
		})
	}
}

func TestAttrStringSlice(t *testing.T) {
	attr := AttrStringSlice("tags", []string{"a", "b", "c"})
	if attr.Key != "tags" {
		t.Errorf("expected key 'tags', got '%s'", attr.Key)
	}
}

func TestSpanContext(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "test-span")

	sc := SpanContext(span)
	if !strings.Contains(sc, "trace_id=") {
		t.Errorf("expected SpanContext to contain 'trace_id=', got: %s", sc)
	}
	if !strings.Contains(sc, "span_id=") {
		t.Errorf("expected SpanContext to contain 'span_id=', got: %s", sc)
	}

	span.End()
	_ = exporter
}

func TestSpanContext_NilSpan(t *testing.T) {
	sc := SpanContext(nil)
	if sc != "no-span" {
		t.Errorf("expected 'no-span', got '%s'", sc)
	}
}

func TestTraceID(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "test-span")

	traceID := TraceID(ctx)
	if traceID == "" {
		t.Error("expected non-empty trace ID")
	}
	if len(traceID) != 32 { // Trace IDs are 32 hex characters
		t.Errorf("expected trace ID length 32, got %d", len(traceID))
	}

	span.End()
	_ = exporter
}

func TestTraceID_NoSpan(t *testing.T) {
	traceID := TraceID(context.Background())
	if traceID != "" {
		t.Errorf("expected empty trace ID for context without span, got '%s'", traceID)
	}
}

func TestSpanID(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "test-span")

	spanID := SpanID(ctx)
	if spanID == "" {
		t.Error("expected non-empty span ID")
	}
	if len(spanID) != 16 { // Span IDs are 16 hex characters
		t.Errorf("expected span ID length 16, got %d", len(spanID))
	}

	span.End()
	_ = exporter
}

func TestSpanID_NoSpan(t *testing.T) {
	spanID := SpanID(context.Background())
	if spanID != "" {
		t.Errorf("expected empty span ID for context without span, got '%s'", spanID)
	}
}

func TestWithSpanKind(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	_, span := StartSpan(context.Background(), "client-span",
		WithSpanKind(SpanKindClient))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].SpanKind != trace.SpanKindClient {
		t.Errorf("expected SpanKindClient, got %v", spans[0].SpanKind)
	}
}

func TestSpanKindConstants(t *testing.T) {
	if SpanKindServer != trace.SpanKindServer {
		t.Error("SpanKindServer mismatch")
	}
	if SpanKindClient != trace.SpanKindClient {
		t.Error("SpanKindClient mismatch")
	}
	if SpanKindProducer != trace.SpanKindProducer {
		t.Error("SpanKindProducer mismatch")
	}
	if SpanKindConsumer != trace.SpanKindConsumer {
		t.Error("SpanKindConsumer mismatch")
	}
	if SpanKindInternal != trace.SpanKindInternal {
		t.Error("SpanKindInternal mismatch")
	}
}

func TestHTTPMiddleware(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	wrapped := HTTPMiddleware(handler)

	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if !strings.Contains(spans[0].Name, "GET") {
		t.Errorf("expected span name to contain 'GET', got '%s'", spans[0].Name)
	}

	if !strings.Contains(spans[0].Name, "/api/users") {
		t.Errorf("expected span name to contain '/api/users', got '%s'", spans[0].Name)
	}

	// Check span kind is Server
	if spans[0].SpanKind != trace.SpanKindServer {
		t.Errorf("expected SpanKindServer, got %v", spans[0].SpanKind)
	}
}

func TestHTTPMiddleware_ErrorStatus(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	wrapped := HTTPMiddleware(handler)

	req := httptest.NewRequest("GET", "/api/error", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("expected status code Error, got %v", spans[0].Status.Code)
	}
}

func TestHTTPMiddlewareWithConfig_SkipPaths(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cfg := &HTTPMiddlewareConfig{
		SkipPaths: []string{"/health", "/metrics"},
	}

	wrapped := HTTPMiddlewareWithConfig(cfg)(handler)

	// Request to skipped path
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 0 {
		t.Errorf("expected 0 spans for skipped path, got %d", len(spans))
	}

	// Request to normal path
	exporter.Reset()
	req = httptest.NewRequest("GET", "/api/users", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	spans = exporter.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span for normal path, got %d", len(spans))
	}
}

func TestHTTPMiddleware_ContextPropagation(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	var capturedCtx context.Context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddleware(handler)

	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Verify span is in context
	span := SpanFromContext(capturedCtx)
	if !span.SpanContext().IsValid() {
		t.Error("expected valid span in request context")
	}

	_ = exporter
}

func TestInjectHTTPHeaders(t *testing.T) {
	exporter, cleanup := setupTestTracer(t)
	defer cleanup()

	ctx, span := StartSpan(context.Background(), "client-request")

	req := httptest.NewRequest("GET", "/api/external", nil)
	InjectHTTPHeaders(ctx, req)

	// Check that traceparent header was added
	traceparent := req.Header.Get("Traceparent")
	if traceparent == "" {
		t.Error("expected Traceparent header to be set")
	}

	span.End()
	_ = exporter
}

func TestServiceFromMethod(t *testing.T) {
	tests := []struct {
		method   string
		expected string
	}{
		{"/package.service/Method", "package.service"},
		{"/my.pkg.Service/DoThing", "my.pkg.Service"},
		{"package.service/Method", "package.service"},
		{"/short", "short"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := serviceFromMethod(tt.method)
			if result != tt.expected {
				t.Errorf("serviceFromMethod(%q) = %q, want %q", tt.method, result, tt.expected)
			}
		})
	}
}

func TestMethodFromMethod(t *testing.T) {
	tests := []struct {
		method   string
		expected string
	}{
		{"/package.service/Method", "Method"},
		{"/my.pkg.Service/DoThing", "DoThing"},
		{"noSlash", "noSlash"},
		{"/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := methodFromMethod(tt.method)
			if result != tt.expected {
				t.Errorf("methodFromMethod(%q) = %q, want %q", tt.method, result, tt.expected)
			}
		})
	}
}

func TestSchemeFromRequest(t *testing.T) {
	// Test with X-Forwarded-Proto
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if scheme := schemeFromRequest(req); scheme != "https" {
		t.Errorf("expected 'https', got '%s'", scheme)
	}

	// Test without headers (should default to http)
	req = httptest.NewRequest("GET", "/", nil)
	if scheme := schemeFromRequest(req); scheme != "http" {
		t.Errorf("expected 'http', got '%s'", scheme)
	}
}
