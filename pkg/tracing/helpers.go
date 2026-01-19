package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SpanFromContext returns the span from the given context.
// If no span is found, returns a no-op span.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// StartSpan starts a new span with the given name.
// It returns the context with the span attached and the span itself.
// The caller is responsible for calling span.End() when the operation completes.
//
// Example:
//
//	ctx, span := tracing.StartSpan(ctx, "process-user")
//	defer span.End()
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer("").Start(ctx, name, opts...)
}

// StartSpanWithTracer starts a new span using a specific tracer.
// Use this when you want to attribute spans to a specific package or component.
//
// Example:
//
//	ctx, span := tracing.StartSpanWithTracer(ctx, "mypackage", "process-user")
//	defer span.End()
func StartSpanWithTracer(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, spanName, opts...)
}

// AddEvent adds an event to the current span.
// Events are time-stamped annotations that can include attributes.
//
// Example:
//
//	tracing.AddEvent(span, "user-validated", tracing.Attr("user_id", userID))
func AddEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// AddEventToContext adds an event to the span in the given context.
// This is a convenience function when you have a context but not a span reference.
func AddEventToContext(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	AddEvent(SpanFromContext(ctx), name, attrs...)
}

// SetError records an error on the span.
// It sets the span status to error and records the error as an attribute.
func SetError(span trace.Span, err error) {
	if span == nil || !span.IsRecording() || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// SetErrorWithMessage records an error on the span with a custom message.
// The message is used as the span status description.
func SetErrorWithMessage(span trace.Span, err error, message string) {
	if span == nil || !span.IsRecording() || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, message)
}

// SetErrorToContext records an error on the span in the given context.
func SetErrorToContext(ctx context.Context, err error) {
	SetError(SpanFromContext(ctx), err)
}

// SetOK sets the span status to OK.
func SetOK(span trace.Span) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetStatus(codes.Ok, "")
}

// SetAttributes adds attributes to a span.
func SetAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(attrs...)
}

// SetAttributesToContext adds attributes to the span in the given context.
func SetAttributesToContext(ctx context.Context, attrs ...attribute.KeyValue) {
	SetAttributes(SpanFromContext(ctx), attrs...)
}

// Attr creates a string attribute.
func Attr(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

// AttrInt creates an integer attribute.
func AttrInt(key string, value int) attribute.KeyValue {
	return attribute.Int(key, value)
}

// AttrInt64 creates an int64 attribute.
func AttrInt64(key string, value int64) attribute.KeyValue {
	return attribute.Int64(key, value)
}

// AttrBool creates a boolean attribute.
func AttrBool(key string, value bool) attribute.KeyValue {
	return attribute.Bool(key, value)
}

// AttrFloat64 creates a float64 attribute.
func AttrFloat64(key string, value float64) attribute.KeyValue {
	return attribute.Float64(key, value)
}

// AttrStringSlice creates a string slice attribute.
func AttrStringSlice(key string, value []string) attribute.KeyValue {
	return attribute.StringSlice(key, value)
}

// SpanContext returns a string representation of the span context.
// This is useful for logging trace/span IDs.
func SpanContext(span trace.Span) string {
	if span == nil {
		return "no-span"
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return "invalid-span"
	}
	return fmt.Sprintf("trace_id=%s span_id=%s", sc.TraceID().String(), sc.SpanID().String())
}

// TraceID returns the trace ID from the span context.
// Returns an empty string if the span is nil or invalid.
func TraceID(ctx context.Context) string {
	span := SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanID returns the span ID from the span context.
// Returns an empty string if the span is nil or invalid.
func SpanID(ctx context.Context) string {
	span := SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}

// WithSpanKind returns a SpanStartOption that sets the span kind.
func WithSpanKind(kind trace.SpanKind) trace.SpanStartOption {
	return trace.WithSpanKind(kind)
}

// SpanKindServer is the span kind for server-side operations.
var SpanKindServer = trace.SpanKindServer

// SpanKindClient is the span kind for client-side operations.
var SpanKindClient = trace.SpanKindClient

// SpanKindProducer is the span kind for message producers.
var SpanKindProducer = trace.SpanKindProducer

// SpanKindConsumer is the span kind for message consumers.
var SpanKindConsumer = trace.SpanKindConsumer

// SpanKindInternal is the span kind for internal operations.
var SpanKindInternal = trace.SpanKindInternal
