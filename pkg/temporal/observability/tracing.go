package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
)

const (
	// TracerName is the default tracer name for Temporal observability.
	TracerName = "github.com/otherjamesbrown/penfold/pkg/temporal/observability"

	// Trace attribute keys for Temporal-specific data
	AttrWorkflowType  = "temporal.workflow.type"
	AttrWorkflowID    = "temporal.workflow.id"
	AttrWorkflowRunID = "temporal.workflow.run_id"
	AttrActivityType  = "temporal.activity.type"
	AttrActivityID    = "temporal.activity.id"
	AttrTaskQueue     = "temporal.task_queue"
	AttrNamespace     = "temporal.namespace"
	AttrAttempt       = "temporal.attempt"
	AttrErrorType     = "temporal.error_type"
	AttrIsLocal       = "temporal.activity.is_local"
)

// WorkflowTracer provides tracing capabilities for Temporal workflows.
type WorkflowTracer struct {
	tracer trace.Tracer
}

// NewWorkflowTracer creates a new workflow tracer.
func NewWorkflowTracer() *WorkflowTracer {
	return &WorkflowTracer{
		tracer: otel.Tracer(TracerName),
	}
}

// NewWorkflowTracerWithName creates a new workflow tracer with a custom name.
func NewWorkflowTracerWithName(name string) *WorkflowTracer {
	return &WorkflowTracer{
		tracer: otel.Tracer(name),
	}
}

// Tracer returns the underlying OpenTelemetry tracer.
func (t *WorkflowTracer) Tracer() trace.Tracer {
	return t.tracer
}

// ActivityTracer provides tracing capabilities for Temporal activities.
type ActivityTracer struct {
	tracer trace.Tracer
}

// NewActivityTracer creates a new activity tracer.
func NewActivityTracer() *ActivityTracer {
	return &ActivityTracer{
		tracer: otel.Tracer(TracerName),
	}
}

// NewActivityTracerWithName creates a new activity tracer with a custom name.
func NewActivityTracerWithName(name string) *ActivityTracer {
	return &ActivityTracer{
		tracer: otel.Tracer(name),
	}
}

// Tracer returns the underlying OpenTelemetry tracer.
func (t *ActivityTracer) Tracer() trace.Tracer {
	return t.tracer
}

// StartActivitySpan starts a new span for an activity execution.
// Returns the context with the span attached and the span itself.
func (t *ActivityTracer) StartActivitySpan(ctx context.Context) (context.Context, trace.Span) {
	info := activity.GetInfo(ctx)

	ctx, span := t.tracer.Start(ctx, "temporal.activity."+info.ActivityType.Name,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String(AttrActivityType, info.ActivityType.Name),
			attribute.String(AttrActivityID, info.ActivityID),
			attribute.String(AttrNamespace, info.WorkflowNamespace),
			attribute.String(AttrWorkflowType, info.WorkflowType.Name),
			attribute.String(AttrWorkflowID, info.WorkflowExecution.ID),
			attribute.String(AttrWorkflowRunID, info.WorkflowExecution.RunID),
			attribute.String(AttrTaskQueue, info.TaskQueue),
			attribute.Int(AttrAttempt, int(info.Attempt)),
			attribute.Bool(AttrIsLocal, info.IsLocalActivity),
		),
	)

	return ctx, span
}

// RecordActivitySuccess records a successful activity completion.
func (t *ActivityTracer) RecordActivitySuccess(span trace.Span) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetStatus(codes.Ok, "")
}

// RecordActivityError records an activity error.
func (t *ActivityTracer) RecordActivityError(span trace.Span, err error) {
	if span == nil || !span.IsRecording() || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(attribute.String(AttrErrorType, ErrorType(err)))
}

// WorkflowTraceInfo holds trace information for a workflow.
// This is stored in workflow context for child activities to access.
type WorkflowTraceInfo struct {
	WorkflowID   string
	RunID        string
	WorkflowType string
	TaskQueue    string
	Namespace    string
	TraceID      string
	SpanID       string
}

// workflowTraceInfoKeyType is the type for the workflow trace info context key.
type workflowTraceInfoKeyType struct{}

// workflowTraceInfoKey is the context key for workflow trace info.
var workflowTraceInfoKey = workflowTraceInfoKeyType{}

// SetWorkflowTraceInfo stores workflow trace info in the workflow context.
func SetWorkflowTraceInfo(ctx workflow.Context, info *WorkflowTraceInfo) workflow.Context {
	return workflow.WithValue(ctx, workflowTraceInfoKey, info)
}

// GetWorkflowTraceInfo retrieves workflow trace info from a workflow context.
func GetWorkflowTraceInfo(ctx workflow.Context) *WorkflowTraceInfo {
	if info := ctx.Value(workflowTraceInfoKey); info != nil {
		if traceInfo, ok := info.(*WorkflowTraceInfo); ok {
			return traceInfo
		}
	}
	return nil
}

// CreateWorkflowTraceInfo creates a WorkflowTraceInfo from workflow context.
func CreateWorkflowTraceInfo(ctx workflow.Context) *WorkflowTraceInfo {
	info := workflow.GetInfo(ctx)
	return &WorkflowTraceInfo{
		WorkflowID:   info.WorkflowExecution.ID,
		RunID:        info.WorkflowExecution.RunID,
		WorkflowType: info.WorkflowType.Name,
		TaskQueue:    info.TaskQueueName,
		Namespace:    info.Namespace,
	}
}

// PropagateContext propagates trace context to Temporal headers.
// Use this when starting child workflows or signaling external workflows.
type PropagateContext struct {
	propagator propagation.TextMapPropagator
}

// NewPropagateContext creates a new context propagator.
func NewPropagateContext() *PropagateContext {
	return &PropagateContext{
		propagator: otel.GetTextMapPropagator(),
	}
}

// Inject injects trace context into Temporal headers.
func (p *PropagateContext) Inject(ctx context.Context, headers map[string][]byte) {
	if headers == nil {
		return
	}
	carrier := &headerCarrier{fields: headers}
	p.propagator.Inject(ctx, carrier)
}

// Extract extracts trace context from Temporal headers.
func (p *PropagateContext) Extract(ctx context.Context, headers map[string][]byte) context.Context {
	if headers == nil {
		return ctx
	}
	carrier := &headerCarrier{fields: headers}
	return p.propagator.Extract(ctx, carrier)
}

// headerCarrier implements propagation.TextMapCarrier for Temporal headers.
type headerCarrier struct {
	fields map[string][]byte
}

// Get returns the value for a key.
func (c *headerCarrier) Get(key string) string {
	if c.fields == nil {
		return ""
	}
	if v, ok := c.fields[key]; ok {
		return string(v)
	}
	return ""
}

// Set sets a key-value pair.
func (c *headerCarrier) Set(key, value string) {
	if c.fields == nil {
		return
	}
	c.fields[key] = []byte(value)
}

// Keys returns all keys.
func (c *headerCarrier) Keys() []string {
	if c.fields == nil {
		return nil
	}
	keys := make([]string, 0, len(c.fields))
	for k := range c.fields {
		keys = append(keys, k)
	}
	return keys
}

// Ensure headerCarrier implements TextMapCarrier.
var _ propagation.TextMapCarrier = (*headerCarrier)(nil)

// TraceIDFromContext returns the trace ID from the context.
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanIDFromContext returns the span ID from the context.
func SpanIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}

// AddWorkflowAttributes adds workflow-specific attributes to a span.
func AddWorkflowAttributes(span trace.Span, info workflow.Info) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String(AttrWorkflowType, info.WorkflowType.Name),
		attribute.String(AttrWorkflowID, info.WorkflowExecution.ID),
		attribute.String(AttrWorkflowRunID, info.WorkflowExecution.RunID),
		attribute.String(AttrTaskQueue, info.TaskQueueName),
		attribute.String(AttrNamespace, info.Namespace),
		attribute.Int(AttrAttempt, int(info.Attempt)),
	)
}

// AddActivityAttributes adds activity-specific attributes to a span.
func AddActivityAttributes(span trace.Span, info activity.Info) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String(AttrActivityType, info.ActivityType.Name),
		attribute.String(AttrActivityID, info.ActivityID),
		attribute.String(AttrWorkflowType, info.WorkflowType.Name),
		attribute.String(AttrWorkflowID, info.WorkflowExecution.ID),
		attribute.String(AttrWorkflowRunID, info.WorkflowExecution.RunID),
		attribute.String(AttrTaskQueue, info.TaskQueue),
		attribute.String(AttrNamespace, info.WorkflowNamespace),
		attribute.Int(AttrAttempt, int(info.Attempt)),
		attribute.Bool(AttrIsLocal, info.IsLocalActivity),
	)
}

// AddErrorAttributes adds error-related attributes to a span.
func AddErrorAttributes(span trace.Span, err error) {
	if span == nil || !span.IsRecording() || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(attribute.String(AttrErrorType, ErrorType(err)))
}
