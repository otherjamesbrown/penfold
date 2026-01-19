package observability

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// LoggingInterceptor provides structured logging for Temporal workflows and activities.
type LoggingInterceptor struct {
	interceptor.WorkerInterceptorBase
	logger  logging.Logger
	metrics *Metrics
}

// NewLoggingInterceptor creates a new logging interceptor.
func NewLoggingInterceptor(logger logging.Logger, metrics *Metrics) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger:  logger.With(logging.F("component", "temporal")),
		metrics: metrics,
	}
}

// InterceptActivity intercepts activity execution to add logging.
func (l *LoggingInterceptor) InterceptActivity(
	ctx context.Context,
	next interceptor.ActivityInboundInterceptor,
) interceptor.ActivityInboundInterceptor {
	return &loggingActivityInbound{
		ActivityInboundInterceptorBase: interceptor.ActivityInboundInterceptorBase{Next: next},
		logger:                         l.logger,
		metrics:                        l.metrics,
	}
}

// InterceptWorkflow intercepts workflow execution to add logging.
func (l *LoggingInterceptor) InterceptWorkflow(
	ctx workflow.Context,
	next interceptor.WorkflowInboundInterceptor,
) interceptor.WorkflowInboundInterceptor {
	return &loggingWorkflowInbound{
		WorkflowInboundInterceptorBase: interceptor.WorkflowInboundInterceptorBase{Next: next},
		logger:                         l.logger,
		metrics:                        l.metrics,
	}
}

// loggingActivityInbound wraps activity execution with logging.
type loggingActivityInbound struct {
	interceptor.ActivityInboundInterceptorBase
	logger  logging.Logger
	metrics *Metrics
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
	// Get activity info from context
	info := activity.GetInfo(ctx)

	// Create activity-scoped logger
	activityLogger := l.logger.With(
		logging.F("activity_type", info.ActivityType.Name),
		logging.F("activity_id", info.ActivityID),
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("run_id", info.WorkflowExecution.RunID),
		logging.F("task_queue", info.TaskQueue),
		logging.F("attempt", info.Attempt),
	)

	// Add trace ID if available
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		activityLogger = activityLogger.With(logging.F("trace_id", traceID))
	}

	// Log activity start
	activityLogger.Info("Activity started",
		logging.F("scheduled_time", info.ScheduledTime),
		logging.F("heartbeat_timeout", info.HeartbeatTimeout),
	)

	// Record metrics
	if l.metrics != nil {
		l.metrics.RecordActivityStart(info.ActivityType.Name, info.WorkflowType.Name, info.TaskQueue)
	}

	startTime := time.Now()

	// Execute the activity
	result, err := l.Next.ExecuteActivity(ctx, in)

	duration := time.Since(startTime)

	// Log and record outcome
	if err != nil {
		activityLogger.Error("Activity failed",
			logging.F("duration_ms", duration.Milliseconds()),
			logging.F("error_type", ErrorType(err)),
			logging.Err(err),
		)
		if l.metrics != nil {
			l.metrics.RecordActivityFail(
				info.ActivityType.Name,
				info.WorkflowType.Name,
				info.TaskQueue,
				ErrorType(err),
				duration,
			)
		}
	} else {
		activityLogger.Info("Activity completed",
			logging.F("duration_ms", duration.Milliseconds()),
		)
		if l.metrics != nil {
			l.metrics.RecordActivityComplete(
				info.ActivityType.Name,
				info.WorkflowType.Name,
				info.TaskQueue,
				duration,
			)
		}
	}

	return result, err
}

// loggingWorkflowInbound wraps workflow execution with logging.
type loggingWorkflowInbound struct {
	interceptor.WorkflowInboundInterceptorBase
	logger  logging.Logger
	metrics *Metrics
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
	// Get workflow info
	info := workflow.GetInfo(ctx)

	// Create workflow-scoped logger
	// Note: We cannot use l.logger directly as workflow code must be deterministic
	// Instead, we use workflow.GetLogger which integrates with Temporal's replay-safe logging
	workflowLogger := workflow.GetLogger(ctx)

	// Log workflow start using Temporal's replay-safe logger
	workflowLogger.Info("Workflow started",
		"workflow_type", info.WorkflowType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"run_id", info.WorkflowExecution.RunID,
		"task_queue", info.TaskQueueName,
		"attempt", info.Attempt,
	)

	// Record start metric using a side effect (metrics are non-deterministic)
	if l.metrics != nil {
		workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
			l.metrics.RecordWorkflowStart(info.WorkflowType.Name, info.TaskQueueName)
			return nil
		})
	}

	startTime := workflow.Now(ctx)

	// Execute the workflow
	result, err := l.Next.ExecuteWorkflow(ctx, in)

	endTime := workflow.Now(ctx)
	duration := endTime.Sub(startTime)

	// Log and record outcome
	if err != nil {
		workflowLogger.Error("Workflow failed",
			"workflow_type", info.WorkflowType.Name,
			"workflow_id", info.WorkflowExecution.ID,
			"duration_ms", duration.Milliseconds(),
			"error_type", ErrorType(err),
			"error", err.Error(),
		)
		if l.metrics != nil {
			workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
				l.metrics.RecordWorkflowFail(
					info.WorkflowType.Name,
					info.TaskQueueName,
					ErrorType(err),
					duration,
				)
				return nil
			})
		}
	} else {
		workflowLogger.Info("Workflow completed",
			"workflow_type", info.WorkflowType.Name,
			"workflow_id", info.WorkflowExecution.ID,
			"duration_ms", duration.Milliseconds(),
		)
		if l.metrics != nil {
			workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
				l.metrics.RecordWorkflowComplete(
					info.WorkflowType.Name,
					info.TaskQueueName,
					duration,
				)
				return nil
			})
		}
	}

	return result, err
}

// HandleSignal intercepts signal handling with logging.
func (l *loggingWorkflowInbound) HandleSignal(
	ctx workflow.Context,
	in *interceptor.HandleSignalInput,
) error {
	info := workflow.GetInfo(ctx)
	workflowLogger := workflow.GetLogger(ctx)

	workflowLogger.Info("Signal received",
		"workflow_type", info.WorkflowType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"signal_name", in.SignalName,
	)

	return l.Next.HandleSignal(ctx, in)
}

// HandleQuery intercepts query handling with logging.
func (l *loggingWorkflowInbound) HandleQuery(
	ctx workflow.Context,
	in *interceptor.HandleQueryInput,
) (interface{}, error) {
	info := workflow.GetInfo(ctx)
	workflowLogger := workflow.GetLogger(ctx)

	workflowLogger.Debug("Query received",
		"workflow_type", info.WorkflowType.Name,
		"workflow_id", info.WorkflowExecution.ID,
		"query_type", in.QueryType,
	)

	result, err := l.Next.HandleQuery(ctx, in)

	if err != nil {
		workflowLogger.Error("Query failed",
			"query_type", in.QueryType,
			"error", err.Error(),
		)
	}

	return result, err
}
