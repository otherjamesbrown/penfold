package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// MetricsInterceptor provides Prometheus metrics collection for Temporal workflows and activities.
type MetricsInterceptor struct {
	interceptor.WorkerInterceptorBase
	metrics *Metrics
}

// NewMetricsInterceptor creates a new metrics interceptor.
func NewMetricsInterceptor(metrics *Metrics) *MetricsInterceptor {
	return &MetricsInterceptor{
		metrics: metrics,
	}
}

// InterceptActivity intercepts activity execution to collect metrics.
func (m *MetricsInterceptor) InterceptActivity(
	ctx context.Context,
	next interceptor.ActivityInboundInterceptor,
) interceptor.ActivityInboundInterceptor {
	return &metricsActivityInbound{
		ActivityInboundInterceptorBase: interceptor.ActivityInboundInterceptorBase{Next: next},
		metrics:                        m.metrics,
	}
}

// InterceptWorkflow intercepts workflow execution to collect metrics.
func (m *MetricsInterceptor) InterceptWorkflow(
	ctx workflow.Context,
	next interceptor.WorkflowInboundInterceptor,
) interceptor.WorkflowInboundInterceptor {
	return &metricsWorkflowInbound{
		WorkflowInboundInterceptorBase: interceptor.WorkflowInboundInterceptorBase{Next: next},
		metrics:                        m.metrics,
	}
}

// metricsActivityInbound wraps activity execution with metrics.
type metricsActivityInbound struct {
	interceptor.ActivityInboundInterceptorBase
	metrics *Metrics
}

// Init initializes the activity inbound interceptor.
func (m *metricsActivityInbound) Init(outbound interceptor.ActivityOutboundInterceptor) error {
	return m.Next.Init(outbound)
}

// ExecuteActivity wraps activity execution with metrics collection.
func (m *metricsActivityInbound) ExecuteActivity(
	ctx context.Context,
	in *interceptor.ExecuteActivityInput,
) (interface{}, error) {
	info := activity.GetInfo(ctx)
	startTime := time.Now()

	m.metrics.RecordActivityStart(info.ActivityType.Name, info.WorkflowType.Name, info.TaskQueue)

	result, err := m.Next.ExecuteActivity(ctx, in)

	duration := time.Since(startTime)

	if err != nil {
		m.metrics.RecordActivityFail(
			info.ActivityType.Name,
			info.WorkflowType.Name,
			info.TaskQueue,
			ErrorType(err),
			duration,
		)
	} else {
		m.metrics.RecordActivityComplete(
			info.ActivityType.Name,
			info.WorkflowType.Name,
			info.TaskQueue,
			duration,
		)
	}

	return result, err
}

// metricsWorkflowInbound wraps workflow execution with metrics.
type metricsWorkflowInbound struct {
	interceptor.WorkflowInboundInterceptorBase
	metrics *Metrics
}

// Init initializes the workflow inbound interceptor.
func (m *metricsWorkflowInbound) Init(outbound interceptor.WorkflowOutboundInterceptor) error {
	return m.Next.Init(outbound)
}

// ExecuteWorkflow wraps workflow execution with metrics collection.
func (m *metricsWorkflowInbound) ExecuteWorkflow(
	ctx workflow.Context,
	in *interceptor.ExecuteWorkflowInput,
) (interface{}, error) {
	info := workflow.GetInfo(ctx)

	// Record start using SideEffect for determinism
	workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		m.metrics.RecordWorkflowStart(info.WorkflowType.Name, info.TaskQueueName)
		return nil
	})

	startTime := workflow.Now(ctx)

	result, err := m.Next.ExecuteWorkflow(ctx, in)

	duration := workflow.Now(ctx).Sub(startTime)

	// Record completion using SideEffect for determinism
	workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		if err != nil {
			m.metrics.RecordWorkflowFail(info.WorkflowType.Name, info.TaskQueueName, ErrorType(err), duration)
		} else {
			m.metrics.RecordWorkflowComplete(info.WorkflowType.Name, info.TaskQueueName, duration)
		}
		return nil
	})

	return result, err
}

// TracingInterceptor provides OpenTelemetry tracing for Temporal workflows and activities.
type TracingInterceptor struct {
	interceptor.WorkerInterceptorBase
	tracer trace.Tracer
}

// NewTracingInterceptor creates a new tracing interceptor.
func NewTracingInterceptor() *TracingInterceptor {
	return &TracingInterceptor{
		tracer: otel.Tracer(TracerName),
	}
}

// NewTracingInterceptorWithName creates a new tracing interceptor with a custom tracer name.
func NewTracingInterceptorWithName(name string) *TracingInterceptor {
	return &TracingInterceptor{
		tracer: otel.Tracer(name),
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
	defer span.End()

	result, err := t.Next.ExecuteActivity(ctx, in)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String(AttrErrorType, ErrorType(err)))
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

// ExecuteWorkflow wraps workflow execution with trace context propagation.
func (t *tracingWorkflowInbound) ExecuteWorkflow(
	ctx workflow.Context,
	in *interceptor.ExecuteWorkflowInput,
) (interface{}, error) {
	// Note: workflow.Context is not a standard context.Context
	// We cannot create real OTel spans in workflow code (non-deterministic)
	// Instead, we inject trace info into workflow context for activities to use

	info := workflow.GetInfo(ctx)

	workflowTraceInfo := &WorkflowTraceInfo{
		WorkflowID:   info.WorkflowExecution.ID,
		RunID:        info.WorkflowExecution.RunID,
		WorkflowType: info.WorkflowType.Name,
		TaskQueue:    info.TaskQueueName,
		Namespace:    info.Namespace,
	}

	ctx = SetWorkflowTraceInfo(ctx, workflowTraceInfo)

	return t.Next.ExecuteWorkflow(ctx, in)
}

// HandleSignal intercepts signal handling.
func (t *tracingWorkflowInbound) HandleSignal(
	ctx workflow.Context,
	in *interceptor.HandleSignalInput,
) error {
	return t.Next.HandleSignal(ctx, in)
}

// HandleQuery intercepts query handling.
func (t *tracingWorkflowInbound) HandleQuery(
	ctx workflow.Context,
	in *interceptor.HandleQueryInput,
) (interface{}, error) {
	return t.Next.HandleQuery(ctx, in)
}

// LoggingInterceptor provides structured logging for Temporal workflows and activities.
type LoggingInterceptor struct {
	interceptor.WorkerInterceptorBase
	logger         logging.Logger
	metrics        *Metrics
	activityLogger *ActivityLogger
	workflowLogger *WorkflowLogger
}

// NewLoggingInterceptor creates a new logging interceptor.
func NewLoggingInterceptor(logger logging.Logger, metrics *Metrics) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger:         logger.With(logging.F("component", "temporal")),
		metrics:        metrics,
		activityLogger: NewActivityLoggerWithMetrics(logger, metrics),
		workflowLogger: NewWorkflowLoggerWithMetrics(logger, metrics),
	}
}

// InterceptActivity intercepts activity execution to add logging.
func (l *LoggingInterceptor) InterceptActivity(
	ctx context.Context,
	next interceptor.ActivityInboundInterceptor,
) interceptor.ActivityInboundInterceptor {
	return &loggingActivityInbound{
		ActivityInboundInterceptorBase: interceptor.ActivityInboundInterceptorBase{Next: next},
		activityLogger:                 l.activityLogger,
	}
}

// InterceptWorkflow intercepts workflow execution to add logging.
func (l *LoggingInterceptor) InterceptWorkflow(
	ctx workflow.Context,
	next interceptor.WorkflowInboundInterceptor,
) interceptor.WorkflowInboundInterceptor {
	return &loggingWorkflowInbound{
		WorkflowInboundInterceptorBase: interceptor.WorkflowInboundInterceptorBase{Next: next},
		workflowLogger:                 l.workflowLogger,
	}
}

// loggingActivityInbound wraps activity execution with logging.
type loggingActivityInbound struct {
	interceptor.ActivityInboundInterceptorBase
	activityLogger *ActivityLogger
}

// Init initializes the activity inbound interceptor.
func (l *loggingActivityInbound) Init(outbound interceptor.ActivityOutboundInterceptor) error {
	return l.Next.Init(outbound)
}

// ExecuteActivity wraps activity execution with logging.
func (l *loggingActivityInbound) ExecuteActivity(
	ctx context.Context,
	in *interceptor.ExecuteActivityInput,
) (interface{}, error) {
	startTime := time.Now()

	l.activityLogger.LogActivityStart(ctx)

	result, err := l.Next.ExecuteActivity(ctx, in)

	duration := time.Since(startTime)

	if err != nil {
		l.activityLogger.LogActivityFail(ctx, err, duration)
	} else {
		l.activityLogger.LogActivityComplete(ctx, duration)
	}

	return result, err
}

// loggingWorkflowInbound wraps workflow execution with logging.
type loggingWorkflowInbound struct {
	interceptor.WorkflowInboundInterceptorBase
	workflowLogger *WorkflowLogger
}

// Init initializes the workflow inbound interceptor.
func (l *loggingWorkflowInbound) Init(outbound interceptor.WorkflowOutboundInterceptor) error {
	return l.Next.Init(outbound)
}

// ExecuteWorkflow wraps workflow execution with logging.
func (l *loggingWorkflowInbound) ExecuteWorkflow(
	ctx workflow.Context,
	in *interceptor.ExecuteWorkflowInput,
) (interface{}, error) {
	info := workflow.GetInfo(ctx)

	// Use Temporal's replay-safe logger
	workflowLog := workflow.GetLogger(ctx)
	workflowLog.Info("Workflow started",
		"workflow_type", info.WorkflowType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"run_id", info.WorkflowExecution.RunID,
		"task_queue", info.TaskQueueName,
		"attempt", info.Attempt,
	)

	// Record start using SideEffect for determinism
	workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		l.workflowLogger.LogWorkflowStart(*info)
		return nil
	})

	startTime := workflow.Now(ctx)

	result, err := l.Next.ExecuteWorkflow(ctx, in)

	duration := workflow.Now(ctx).Sub(startTime)

	// Record completion using SideEffect for determinism
	workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		if err != nil {
			l.workflowLogger.LogWorkflowFail(*info, err, duration)
		} else {
			l.workflowLogger.LogWorkflowComplete(*info, duration)
		}
		return nil
	})

	if err != nil {
		workflowLog.Error("Workflow failed",
			"workflow_type", info.WorkflowType.Name,
			"workflow_id", info.WorkflowExecution.ID,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
	} else {
		workflowLog.Info("Workflow completed",
			"workflow_type", info.WorkflowType.Name,
			"workflow_id", info.WorkflowExecution.ID,
			"duration_ms", duration.Milliseconds(),
		)
	}

	return result, err
}

// HandleSignal intercepts signal handling with logging.
func (l *loggingWorkflowInbound) HandleSignal(
	ctx workflow.Context,
	in *interceptor.HandleSignalInput,
) error {
	info := workflow.GetInfo(ctx)
	workflowLog := workflow.GetLogger(ctx)

	workflowLog.Info("Signal received",
		"workflow_type", info.WorkflowType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"signal_name", in.SignalName,
	)

	// Record signal using SideEffect for determinism
	workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		l.workflowLogger.LogSignal(*info, in.SignalName)
		return nil
	})

	return l.Next.HandleSignal(ctx, in)
}

// HandleQuery intercepts query handling with logging.
func (l *loggingWorkflowInbound) HandleQuery(
	ctx workflow.Context,
	in *interceptor.HandleQueryInput,
) (interface{}, error) {
	info := workflow.GetInfo(ctx)
	workflowLog := workflow.GetLogger(ctx)

	workflowLog.Debug("Query received",
		"workflow_type", info.WorkflowType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"query_type", in.QueryType,
	)

	result, err := l.Next.HandleQuery(ctx, in)

	if err != nil {
		workflowLog.Error("Query failed",
			"query_type", in.QueryType,
			"error", err.Error(),
		)
	}

	return result, err
}

// ChainedInterceptor combines multiple interceptors into one.
type ChainedInterceptor struct {
	interceptors []interceptor.WorkerInterceptor
}

// NewChainedInterceptor creates a new chained interceptor.
func NewChainedInterceptor(interceptors ...interceptor.WorkerInterceptor) *ChainedInterceptor {
	return &ChainedInterceptor{
		interceptors: interceptors,
	}
}

// Interceptors returns the underlying interceptors.
func (c *ChainedInterceptor) Interceptors() []interceptor.WorkerInterceptor {
	return c.interceptors
}

// InterceptorOptions holds configuration for creating observability interceptors.
type InterceptorOptions struct {
	// Logger is the structured logger to use. Required for logging interceptor.
	Logger logging.Logger

	// Metrics is the metrics collector. Required for metrics interceptor.
	Metrics *Metrics

	// EnableTracing enables OpenTelemetry tracing. Default: true.
	EnableTracing bool

	// EnableLogging enables structured logging. Default: true.
	EnableLogging bool

	// EnableMetrics enables Prometheus metrics. Default: true.
	EnableMetrics bool

	// TracerName is the custom tracer name. If empty, uses default.
	TracerName string
}

// DefaultInterceptorOptions returns default options with all observability enabled.
func DefaultInterceptorOptions(logger logging.Logger) *InterceptorOptions {
	return &InterceptorOptions{
		Logger:        logger,
		Metrics:       nil, // Must be provided separately
		EnableTracing: true,
		EnableLogging: true,
		EnableMetrics: true,
	}
}

// NewInterceptors creates a slice of worker interceptors for observability.
// Use this with worker.Options.Interceptors.
func NewInterceptors(opts *InterceptorOptions) []interceptor.WorkerInterceptor {
	if opts == nil {
		opts = &InterceptorOptions{
			EnableTracing: true,
			EnableLogging: false,
			EnableMetrics: false,
		}
	}

	var interceptors []interceptor.WorkerInterceptor

	// Add tracing interceptor
	if opts.EnableTracing {
		if opts.TracerName != "" {
			interceptors = append(interceptors, NewTracingInterceptorWithName(opts.TracerName))
		} else {
			interceptors = append(interceptors, NewTracingInterceptor())
		}
	}

	// Add metrics interceptor
	if opts.EnableMetrics && opts.Metrics != nil {
		interceptors = append(interceptors, NewMetricsInterceptor(opts.Metrics))
	}

	// Add logging interceptor
	if opts.EnableLogging && opts.Logger != nil {
		interceptors = append(interceptors, NewLoggingInterceptor(opts.Logger, opts.Metrics))
	}

	return interceptors
}

// WorkerOption returns a worker option that adds observability interceptors.
func WorkerOption(opts *InterceptorOptions) func(*worker.Options) {
	interceptors := NewInterceptors(opts)
	return func(workerOpts *worker.Options) {
		workerOpts.Interceptors = append(workerOpts.Interceptors, interceptors...)
	}
}

// WithObservability adds observability interceptors to an existing worker options.
func WithObservability(workerOpts *worker.Options, interceptorOpts *InterceptorOptions) *worker.Options {
	interceptors := NewInterceptors(interceptorOpts)
	workerOpts.Interceptors = append(workerOpts.Interceptors, interceptors...)
	return workerOpts
}

// InterceptorChain provides a fluent interface for building interceptor chains.
type InterceptorChain struct {
	interceptors []interceptor.WorkerInterceptor
	logger       logging.Logger
	metrics      *Metrics
	tracerName   string
}

// NewInterceptorChain creates a new empty interceptor chain.
func NewInterceptorChain() *InterceptorChain {
	return &InterceptorChain{
		interceptors: make([]interceptor.WorkerInterceptor, 0),
	}
}

// WithLogger adds a logger to the chain.
func (c *InterceptorChain) WithLogger(logger logging.Logger) *InterceptorChain {
	c.logger = logger
	return c
}

// WithMetrics adds metrics to the chain.
func (c *InterceptorChain) WithMetrics(metrics *Metrics) *InterceptorChain {
	c.metrics = metrics
	return c
}

// WithTracerName sets a custom tracer name.
func (c *InterceptorChain) WithTracerName(name string) *InterceptorChain {
	c.tracerName = name
	return c
}

// WithTracing adds the tracing interceptor to the chain.
func (c *InterceptorChain) WithTracing() *InterceptorChain {
	if c.tracerName != "" {
		c.interceptors = append(c.interceptors, NewTracingInterceptorWithName(c.tracerName))
	} else {
		c.interceptors = append(c.interceptors, NewTracingInterceptor())
	}
	return c
}

// WithMetricsInterceptor adds the metrics interceptor to the chain.
// Requires that WithMetrics has been called first.
func (c *InterceptorChain) WithMetricsInterceptor() *InterceptorChain {
	if c.metrics != nil {
		c.interceptors = append(c.interceptors, NewMetricsInterceptor(c.metrics))
	}
	return c
}

// WithLogging adds the logging interceptor to the chain.
// Requires that WithLogger has been called first.
func (c *InterceptorChain) WithLogging() *InterceptorChain {
	if c.logger != nil {
		c.interceptors = append(c.interceptors, NewLoggingInterceptor(c.logger, c.metrics))
	}
	return c
}

// WithCustom adds a custom interceptor to the chain.
func (c *InterceptorChain) WithCustom(i interceptor.WorkerInterceptor) *InterceptorChain {
	c.interceptors = append(c.interceptors, i)
	return c
}

// Build returns the interceptor slice for use with worker.Options.
func (c *InterceptorChain) Build() []interceptor.WorkerInterceptor {
	return c.interceptors
}

// ApplyTo applies the interceptor chain to worker options.
func (c *InterceptorChain) ApplyTo(opts *worker.Options) {
	opts.Interceptors = append(opts.Interceptors, c.interceptors...)
}
