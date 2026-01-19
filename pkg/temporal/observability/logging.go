package observability

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// WorkflowLogger provides structured logging for Temporal workflows.
// It integrates with the penfold logging package for consistent log output.
type WorkflowLogger struct {
	logger  logging.Logger
	metrics *Metrics
}

// NewWorkflowLogger creates a new workflow logger.
func NewWorkflowLogger(logger logging.Logger) *WorkflowLogger {
	return &WorkflowLogger{
		logger: logger.With(logging.F("component", "temporal.workflow")),
	}
}

// NewWorkflowLoggerWithMetrics creates a new workflow logger with metrics integration.
func NewWorkflowLoggerWithMetrics(logger logging.Logger, metrics *Metrics) *WorkflowLogger {
	return &WorkflowLogger{
		logger:  logger.With(logging.F("component", "temporal.workflow")),
		metrics: metrics,
	}
}

// LogWorkflowStart logs a workflow start event.
// Note: This should be called from a SideEffect in workflow code for determinism.
func (l *WorkflowLogger) LogWorkflowStart(info workflow.Info) {
	l.logger.Info("Workflow started",
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("run_id", info.WorkflowExecution.RunID),
		logging.F("task_queue", info.TaskQueueName),
		logging.F("namespace", info.Namespace),
		logging.F("attempt", info.Attempt),
	)

	if l.metrics != nil {
		l.metrics.RecordWorkflowStart(info.WorkflowType.Name, info.TaskQueueName)
	}
}

// LogWorkflowComplete logs a workflow completion event.
func (l *WorkflowLogger) LogWorkflowComplete(info workflow.Info, duration time.Duration) {
	l.logger.Info("Workflow completed",
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("run_id", info.WorkflowExecution.RunID),
		logging.F("duration_ms", duration.Milliseconds()),
	)

	if l.metrics != nil {
		l.metrics.RecordWorkflowComplete(info.WorkflowType.Name, info.TaskQueueName, duration)
	}
}

// LogWorkflowFail logs a workflow failure event.
func (l *WorkflowLogger) LogWorkflowFail(info workflow.Info, err error, duration time.Duration) {
	errType := ErrorType(err)
	l.logger.Error("Workflow failed",
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("run_id", info.WorkflowExecution.RunID),
		logging.F("duration_ms", duration.Milliseconds()),
		logging.F("error_type", errType),
		logging.Err(err),
	)

	if l.metrics != nil {
		l.metrics.RecordWorkflowFail(info.WorkflowType.Name, info.TaskQueueName, errType, duration)
	}
}

// LogSignal logs a signal received event.
func (l *WorkflowLogger) LogSignal(info workflow.Info, signalName string) {
	l.logger.Info("Signal received",
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("signal_name", signalName),
	)
}

// LogQuery logs a query received event.
func (l *WorkflowLogger) LogQuery(info workflow.Info, queryType string) {
	l.logger.Debug("Query received",
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("query_type", queryType),
	)
}

// WithWorkflow returns a logger with workflow context fields.
func (l *WorkflowLogger) WithWorkflow(info workflow.Info) logging.Logger {
	return l.logger.With(
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("run_id", info.WorkflowExecution.RunID),
		logging.F("task_queue", info.TaskQueueName),
	)
}

// ActivityLogger provides structured logging for Temporal activities.
type ActivityLogger struct {
	logger  logging.Logger
	metrics *Metrics
}

// NewActivityLogger creates a new activity logger.
func NewActivityLogger(logger logging.Logger) *ActivityLogger {
	return &ActivityLogger{
		logger: logger.With(logging.F("component", "temporal.activity")),
	}
}

// NewActivityLoggerWithMetrics creates a new activity logger with metrics integration.
func NewActivityLoggerWithMetrics(logger logging.Logger, metrics *Metrics) *ActivityLogger {
	return &ActivityLogger{
		logger:  logger.With(logging.F("component", "temporal.activity")),
		metrics: metrics,
	}
}

// LogActivityStart logs an activity start event.
func (l *ActivityLogger) LogActivityStart(ctx context.Context) {
	info := activity.GetInfo(ctx)

	activityLogger := l.withActivityContext(info)
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		activityLogger = activityLogger.With(logging.F("trace_id", traceID))
	}

	activityLogger.Info("Activity started",
		logging.F("scheduled_time", info.ScheduledTime),
		logging.F("heartbeat_timeout", info.HeartbeatTimeout),
	)

	if l.metrics != nil {
		l.metrics.RecordActivityStart(info.ActivityType.Name, info.WorkflowType.Name, info.TaskQueue)
	}
}

// LogActivityComplete logs an activity completion event.
func (l *ActivityLogger) LogActivityComplete(ctx context.Context, duration time.Duration) {
	info := activity.GetInfo(ctx)

	activityLogger := l.withActivityContext(info)
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		activityLogger = activityLogger.With(logging.F("trace_id", traceID))
	}

	activityLogger.Info("Activity completed",
		logging.F("duration_ms", duration.Milliseconds()),
	)

	if l.metrics != nil {
		l.metrics.RecordActivityComplete(info.ActivityType.Name, info.WorkflowType.Name, info.TaskQueue, duration)
	}
}

// LogActivityFail logs an activity failure event.
func (l *ActivityLogger) LogActivityFail(ctx context.Context, err error, duration time.Duration) {
	info := activity.GetInfo(ctx)
	errType := ErrorType(err)

	activityLogger := l.withActivityContext(info)
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		activityLogger = activityLogger.With(logging.F("trace_id", traceID))
	}

	activityLogger.Error("Activity failed",
		logging.F("duration_ms", duration.Milliseconds()),
		logging.F("error_type", errType),
		logging.Err(err),
	)

	if l.metrics != nil {
		l.metrics.RecordActivityFail(info.ActivityType.Name, info.WorkflowType.Name, info.TaskQueue, errType, duration)
	}
}

// LogHeartbeat logs an activity heartbeat event.
func (l *ActivityLogger) LogHeartbeat(ctx context.Context, details string) {
	info := activity.GetInfo(ctx)
	l.withActivityContext(info).Debug("Activity heartbeat",
		logging.F("details", details),
	)
}

// WithActivity returns a logger with activity context fields.
func (l *ActivityLogger) WithActivity(ctx context.Context) logging.Logger {
	info := activity.GetInfo(ctx)
	activityLogger := l.withActivityContext(info)
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		activityLogger = activityLogger.With(logging.F("trace_id", traceID))
	}
	return activityLogger
}

// withActivityContext adds activity info fields to the logger.
func (l *ActivityLogger) withActivityContext(info activity.Info) logging.Logger {
	return l.logger.With(
		logging.F("activity_type", info.ActivityType.Name),
		logging.F("activity_id", info.ActivityID),
		logging.F("workflow_type", info.WorkflowType.Name),
		logging.F("workflow_id", info.WorkflowExecution.ID),
		logging.F("run_id", info.WorkflowExecution.RunID),
		logging.F("task_queue", info.TaskQueue),
		logging.F("attempt", info.Attempt),
	)
}

// TemporalLogAdapter adapts the WorkflowLogger/ActivityLogger to work with Temporal's
// replay-safe logging within workflow code.
type TemporalLogAdapter struct {
	workflowLogger *WorkflowLogger
	activityLogger *ActivityLogger
}

// NewTemporalLogAdapter creates a new Temporal log adapter.
func NewTemporalLogAdapter(logger logging.Logger, metrics *Metrics) *TemporalLogAdapter {
	return &TemporalLogAdapter{
		workflowLogger: NewWorkflowLoggerWithMetrics(logger, metrics),
		activityLogger: NewActivityLoggerWithMetrics(logger, metrics),
	}
}

// WorkflowLogger returns the workflow logger.
func (a *TemporalLogAdapter) WorkflowLogger() *WorkflowLogger {
	return a.workflowLogger
}

// ActivityLogger returns the activity logger.
func (a *TemporalLogAdapter) ActivityLogger() *ActivityLogger {
	return a.activityLogger
}
