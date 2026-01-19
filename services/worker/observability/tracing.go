package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/workflow"
)

const (
	tracerName = "github.com/otherjamesbrown/penfold/services/worker/observability"
)

// TracingInterceptor provides OpenTelemetry tracing for Temporal workflows and activities.
type TracingInterceptor struct {
	interceptor.WorkerInterceptorBase
	tracer trace.Tracer
}

// NewTracingInterceptor creates a new tracing interceptor.
func NewTracingInterceptor() *TracingInterceptor {
	return &TracingInterceptor{
		tracer: otel.Tracer(tracerName),
	}
}

// InterceptActivity intercepts activity execution to add tracing.
func (t *TracingInterceptor) InterceptActivity(
	ctx context.Context,
	next interceptor.ActivityInboundInterceptor,
) interceptor.ActivityInboundInterceptor {
	return &tracingActivityInbound{
		ActivityInboundInterceptorBase: interceptor.ActivityInboundInterceptorBase{Next: next},
		tracer:                         t.tracer,
	}
}

// InterceptWorkflow intercepts workflow execution to add tracing.
func (t *TracingInterceptor) InterceptWorkflow(
	ctx workflow.Context,
	next interceptor.WorkflowInboundInterceptor,
) interceptor.WorkflowInboundInterceptor {
	return &tracingWorkflowInbound{
		WorkflowInboundInterceptorBase: interceptor.WorkflowInboundInterceptorBase{Next: next},
		tracer:                         t.tracer,
	}
}

// tracingActivityInbound wraps activity execution with tracing.
type tracingActivityInbound struct {
	interceptor.ActivityInboundInterceptorBase
	tracer trace.Tracer
}

// Init initializes the activity inbound interceptor.
func (t *tracingActivityInbound) Init(outbound interceptor.ActivityOutboundInterceptor) error {
	return t.Next.Init(outbound)
}

// ExecuteActivity wraps activity execution with a span.
func (t *tracingActivityInbound) ExecuteActivity(
	ctx context.Context,
	in *interceptor.ExecuteActivityInput,
) (interface{}, error) {
	// Get activity info from context
	info := activity.GetInfo(ctx)

	// Start a new span for this activity
	ctx, span := t.tracer.Start(ctx, "temporal.activity."+info.ActivityType.Name,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("temporal.activity.type", info.ActivityType.Name),
			attribute.String("temporal.activity.id", info.ActivityID),
			attribute.String("temporal.namespace", info.WorkflowNamespace),
			attribute.String("temporal.workflow.type", info.WorkflowType.Name),
			attribute.String("temporal.workflow.id", info.WorkflowExecution.ID),
			attribute.String("temporal.workflow.run_id", info.WorkflowExecution.RunID),
			attribute.String("temporal.task_queue", info.TaskQueue),
			attribute.Int("temporal.activity.attempt", int(info.Attempt)),
			attribute.Bool("temporal.activity.is_local", info.IsLocalActivity),
		),
	)
	defer span.End()

	// Execute the activity
	result, err := t.Next.ExecuteActivity(ctx, in)

	// Record the outcome
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("temporal.activity.error_type", ErrorType(err)))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return result, err
}

// tracingWorkflowInbound wraps workflow execution with tracing.
type tracingWorkflowInbound struct {
	interceptor.WorkflowInboundInterceptorBase
	tracer trace.Tracer
}

// Init initializes the workflow inbound interceptor.
func (t *tracingWorkflowInbound) Init(outbound interceptor.WorkflowOutboundInterceptor) error {
	return t.Next.Init(outbound)
}

// ExecuteWorkflow wraps workflow execution with a span.
func (t *tracingWorkflowInbound) ExecuteWorkflow(
	ctx workflow.Context,
	in *interceptor.ExecuteWorkflowInput,
) (interface{}, error) {
	// Note: workflow.Context is not a standard context.Context
	// We need to use workflow-safe operations

	info := workflow.GetInfo(ctx)

	// Create workflow-local span attributes
	// Note: We cannot create real OTel spans in workflow code (non-deterministic)
	// Instead, we inject trace info into workflow context for activities to use
	workflowTraceInfo := &WorkflowTraceInfo{
		WorkflowID:   info.WorkflowExecution.ID,
		RunID:        info.WorkflowExecution.RunID,
		WorkflowType: info.WorkflowType.Name,
		TaskQueue:    info.TaskQueueName,
		Namespace:    info.Namespace,
	}

	// Store trace info in workflow context for child activities
	ctx = workflow.WithValue(ctx, workflowTraceInfoKey, workflowTraceInfo)

	// Execute the workflow
	result, err := t.Next.ExecuteWorkflow(ctx, in)

	return result, err
}

// HandleSignal intercepts signal handling.
func (t *tracingWorkflowInbound) HandleSignal(
	ctx workflow.Context,
	in *interceptor.HandleSignalInput,
) error {
	// Add signal tracing context
	return t.Next.HandleSignal(ctx, in)
}

// HandleQuery intercepts query handling.
func (t *tracingWorkflowInbound) HandleQuery(
	ctx workflow.Context,
	in *interceptor.HandleQueryInput,
) (interface{}, error) {
	return t.Next.HandleQuery(ctx, in)
}

// WorkflowTraceInfo holds trace information for a workflow.
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

// GetWorkflowTraceInfo retrieves workflow trace info from a workflow context.
func GetWorkflowTraceInfo(ctx workflow.Context) *WorkflowTraceInfo {
	if info := ctx.Value(workflowTraceInfoKey); info != nil {
		if traceInfo, ok := info.(*WorkflowTraceInfo); ok {
			return traceInfo
		}
	}
	return nil
}

// InjectTraceContext injects trace context into Temporal headers.
func InjectTraceContext(ctx context.Context, header map[string][]byte) {
	if header == nil {
		return
	}

	carrier := &headerCarrier{fields: header}
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, carrier)
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
