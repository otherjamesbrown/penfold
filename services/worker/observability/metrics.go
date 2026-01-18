// Package observability provides metrics, tracing, and logging interceptors for Temporal workflows.
package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsConfig holds configuration for Temporal metrics.
type MetricsConfig struct {
	// Namespace is the Prometheus metrics namespace (prefix).
	Namespace string

	// ServiceName identifies the service in metrics labels.
	ServiceName string

	// DurationBuckets defines histogram buckets for duration metrics.
	// If nil, default buckets are used.
	DurationBuckets []float64

	// Registry is the Prometheus registry to use. If nil, uses the default registry.
	Registry prometheus.Registerer
}

// DefaultDurationBuckets returns default histogram buckets for workflow/activity duration (in seconds).
func DefaultDurationBuckets() []float64 {
	return []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300}
}

// Metrics holds Prometheus metrics for Temporal workflows and activities.
type Metrics struct {
	// Workflow metrics
	WorkflowStartedTotal   *prometheus.CounterVec
	WorkflowCompletedTotal *prometheus.CounterVec
	WorkflowFailedTotal    *prometheus.CounterVec
	WorkflowDuration       *prometheus.HistogramVec

	// Activity metrics
	ActivityStartedTotal   *prometheus.CounterVec
	ActivityCompletedTotal *prometheus.CounterVec
	ActivityFailedTotal    *prometheus.CounterVec
	ActivityDuration       *prometheus.HistogramVec

	// Task queue metrics
	TaskQueuePolls *prometheus.CounterVec

	// Config
	config *MetricsConfig

	// registry holds the Prometheus registry for this metrics instance.
	registry prometheus.Registerer
}

// NewMetrics creates a new Metrics instance with Prometheus metrics for Temporal observability.
func NewMetrics(cfg *MetricsConfig) *Metrics {
	if cfg == nil {
		cfg = &MetricsConfig{
			Namespace:       "penfold",
			ServiceName:     "worker",
			DurationBuckets: DefaultDurationBuckets(),
		}
	}

	if cfg.DurationBuckets == nil {
		cfg.DurationBuckets = DefaultDurationBuckets()
	}

	registry := cfg.Registry
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		config:   cfg,
		registry: registry,
	}

	// Workflow metrics
	m.WorkflowStartedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "workflow_started_total",
			Help:      "Total number of workflows started",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"workflow_type", "task_queue"},
	)

	m.WorkflowCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "workflow_completed_total",
			Help:      "Total number of workflows completed successfully",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"workflow_type", "task_queue"},
	)

	m.WorkflowFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "workflow_failed_total",
			Help:      "Total number of workflows that failed",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"workflow_type", "task_queue", "error_type"},
	)

	m.WorkflowDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "workflow_duration_seconds",
			Help:      "Duration of workflow execution in seconds",
			Buckets:   cfg.DurationBuckets,
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"workflow_type", "task_queue", "status"},
	)

	// Activity metrics
	m.ActivityStartedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "activity_started_total",
			Help:      "Total number of activities started",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"activity_type", "workflow_type", "task_queue"},
	)

	m.ActivityCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "activity_completed_total",
			Help:      "Total number of activities completed successfully",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"activity_type", "workflow_type", "task_queue"},
	)

	m.ActivityFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "activity_failed_total",
			Help:      "Total number of activities that failed",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"activity_type", "workflow_type", "task_queue", "error_type"},
	)

	m.ActivityDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "activity_duration_seconds",
			Help:      "Duration of activity execution in seconds",
			Buckets:   cfg.DurationBuckets,
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"activity_type", "workflow_type", "task_queue", "status"},
	)

	// Task queue metrics
	m.TaskQueuePolls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "temporal",
			Name:      "task_queue_polls_total",
			Help:      "Total number of task queue poll operations",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"task_queue", "task_type"},
	)

	// Register all metrics
	m.mustRegister()

	return m
}

// mustRegister registers all metrics with the configured registry.
func (m *Metrics) mustRegister() {
	collectors := []prometheus.Collector{
		m.WorkflowStartedTotal,
		m.WorkflowCompletedTotal,
		m.WorkflowFailedTotal,
		m.WorkflowDuration,
		m.ActivityStartedTotal,
		m.ActivityCompletedTotal,
		m.ActivityFailedTotal,
		m.ActivityDuration,
		m.TaskQueuePolls,
	}

	for _, c := range collectors {
		// Use MustRegister - in production we want to fail fast on duplicate registration
		m.registry.MustRegister(c)
	}
}

// RecordWorkflowStart records a workflow start event.
func (m *Metrics) RecordWorkflowStart(workflowType, taskQueue string) {
	m.WorkflowStartedTotal.WithLabelValues(workflowType, taskQueue).Inc()
}

// RecordWorkflowComplete records a workflow completion event.
func (m *Metrics) RecordWorkflowComplete(workflowType, taskQueue string, duration time.Duration) {
	m.WorkflowCompletedTotal.WithLabelValues(workflowType, taskQueue).Inc()
	m.WorkflowDuration.WithLabelValues(workflowType, taskQueue, "completed").Observe(duration.Seconds())
}

// RecordWorkflowFail records a workflow failure event.
func (m *Metrics) RecordWorkflowFail(workflowType, taskQueue, errorType string, duration time.Duration) {
	m.WorkflowFailedTotal.WithLabelValues(workflowType, taskQueue, errorType).Inc()
	m.WorkflowDuration.WithLabelValues(workflowType, taskQueue, "failed").Observe(duration.Seconds())
}

// RecordActivityStart records an activity start event.
func (m *Metrics) RecordActivityStart(activityType, workflowType, taskQueue string) {
	m.ActivityStartedTotal.WithLabelValues(activityType, workflowType, taskQueue).Inc()
}

// RecordActivityComplete records an activity completion event.
func (m *Metrics) RecordActivityComplete(activityType, workflowType, taskQueue string, duration time.Duration) {
	m.ActivityCompletedTotal.WithLabelValues(activityType, workflowType, taskQueue).Inc()
	m.ActivityDuration.WithLabelValues(activityType, workflowType, taskQueue, "completed").Observe(duration.Seconds())
}

// RecordActivityFail records an activity failure event.
func (m *Metrics) RecordActivityFail(activityType, workflowType, taskQueue, errorType string, duration time.Duration) {
	m.ActivityFailedTotal.WithLabelValues(activityType, workflowType, taskQueue, errorType).Inc()
	m.ActivityDuration.WithLabelValues(activityType, workflowType, taskQueue, "failed").Observe(duration.Seconds())
}

// RecordTaskQueuePoll records a task queue poll event.
func (m *Metrics) RecordTaskQueuePoll(taskQueue, taskType string) {
	m.TaskQueuePolls.WithLabelValues(taskQueue, taskType).Inc()
}

// ErrorType extracts a categorized error type from an error.
func ErrorType(err error) string {
	if err == nil {
		return "none"
	}

	// Categorize common Temporal errors
	errStr := err.Error()

	switch {
	case contains(errStr, "timeout"):
		return "timeout"
	case contains(errStr, "canceled"):
		return "canceled"
	case contains(errStr, "not found"):
		return "not_found"
	case contains(errStr, "already exists"):
		return "already_exists"
	case contains(errStr, "permission"):
		return "permission_denied"
	case contains(errStr, "invalid"):
		return "invalid_argument"
	case contains(errStr, "connection"):
		return "connection_error"
	case contains(errStr, "retry"):
		return "retry_exhausted"
	default:
		return "unknown"
	}
}

// contains is a simple string contains check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
