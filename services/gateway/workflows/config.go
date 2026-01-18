// Package workflows provides a bridge between the Gateway API and Temporal workflows.
// It handles workflow lifecycle management including starting, status tracking, and cancellation.
package workflows

import (
	"os"
	"strconv"
	"time"
)

// Config holds configuration for the workflow starter bridge.
type Config struct {
	// Temporal connection settings
	TemporalHostPort string
	TemporalNamespace string

	// Task queue configuration
	DefaultTaskQueue    string
	IngestionTaskQueue  string
	SyncTaskQueue       string
	ReviewTaskQueue     string
	AnalysisTaskQueue   string

	// Timeout settings
	DefaultStartTimeout      time.Duration
	DefaultExecutionTimeout  time.Duration
	DefaultTaskTimeout       time.Duration

	// Retry settings
	MaxRetryAttempts int
	RetryBackoff     time.Duration

	// Metrics settings
	MetricsEnabled bool
	MetricsPrefix  string
}

// Default configuration values.
const (
	DefaultTemporalHostPort = "localhost:7233"
	DefaultTemporalNamespace = "default"

	DefaultDefaultTaskQueue   = "penfold-gateway"
	DefaultIngestionTaskQueue = "penfold-ingestion"
	DefaultSyncTaskQueue      = "penfold-sync"
	DefaultReviewTaskQueue    = "penfold-review"
	DefaultAnalysisTaskQueue  = "penfold-analysis"

	DefaultStartTimeout     = 10 * time.Second
	DefaultExecutionTimeout = 24 * time.Hour
	DefaultTaskTimeout      = 10 * time.Minute

	DefaultMaxRetryAttempts = 3
	DefaultRetryBackoff     = 1 * time.Second

	DefaultMetricsEnabled = true
	DefaultMetricsPrefix  = "penfold_gateway_workflow"
)

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		TemporalHostPort:  DefaultTemporalHostPort,
		TemporalNamespace: DefaultTemporalNamespace,

		DefaultTaskQueue:   DefaultDefaultTaskQueue,
		IngestionTaskQueue: DefaultIngestionTaskQueue,
		SyncTaskQueue:      DefaultSyncTaskQueue,
		ReviewTaskQueue:    DefaultReviewTaskQueue,
		AnalysisTaskQueue:  DefaultAnalysisTaskQueue,

		DefaultStartTimeout:     DefaultStartTimeout,
		DefaultExecutionTimeout: DefaultExecutionTimeout,
		DefaultTaskTimeout:      DefaultTaskTimeout,

		MaxRetryAttempts: DefaultMaxRetryAttempts,
		RetryBackoff:     DefaultRetryBackoff,

		MetricsEnabled: DefaultMetricsEnabled,
		MetricsPrefix:  DefaultMetricsPrefix,
	}
}

// LoadConfigFromEnv loads configuration from environment variables.
// Environment variables:
//   - TEMPORAL_HOST_PORT: Temporal server address (default: localhost:7233)
//   - TEMPORAL_NAMESPACE: Temporal namespace (default: default)
//   - WORKFLOW_DEFAULT_TASK_QUEUE: Default task queue (default: penfold-gateway)
//   - WORKFLOW_INGESTION_TASK_QUEUE: Ingestion workflow task queue
//   - WORKFLOW_SYNC_TASK_QUEUE: Sync workflow task queue
//   - WORKFLOW_REVIEW_TASK_QUEUE: Review workflow task queue
//   - WORKFLOW_ANALYSIS_TASK_QUEUE: Analysis workflow task queue
//   - WORKFLOW_START_TIMEOUT_SECONDS: Timeout for starting workflows
//   - WORKFLOW_EXECUTION_TIMEOUT_HOURS: Maximum workflow execution time
//   - WORKFLOW_TASK_TIMEOUT_MINUTES: Task timeout
//   - WORKFLOW_MAX_RETRY_ATTEMPTS: Maximum retry attempts
//   - WORKFLOW_RETRY_BACKOFF_SECONDS: Retry backoff duration
//   - WORKFLOW_METRICS_ENABLED: Enable Prometheus metrics (default: true)
//   - WORKFLOW_METRICS_PREFIX: Prometheus metrics prefix
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("TEMPORAL_HOST_PORT"); v != "" {
		cfg.TemporalHostPort = v
	}

	if v := os.Getenv("TEMPORAL_NAMESPACE"); v != "" {
		cfg.TemporalNamespace = v
	}

	if v := os.Getenv("WORKFLOW_DEFAULT_TASK_QUEUE"); v != "" {
		cfg.DefaultTaskQueue = v
	}

	if v := os.Getenv("WORKFLOW_INGESTION_TASK_QUEUE"); v != "" {
		cfg.IngestionTaskQueue = v
	}

	if v := os.Getenv("WORKFLOW_SYNC_TASK_QUEUE"); v != "" {
		cfg.SyncTaskQueue = v
	}

	if v := os.Getenv("WORKFLOW_REVIEW_TASK_QUEUE"); v != "" {
		cfg.ReviewTaskQueue = v
	}

	if v := os.Getenv("WORKFLOW_ANALYSIS_TASK_QUEUE"); v != "" {
		cfg.AnalysisTaskQueue = v
	}

	if v := os.Getenv("WORKFLOW_START_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.DefaultStartTimeout = time.Duration(secs) * time.Second
		}
	}

	if v := os.Getenv("WORKFLOW_EXECUTION_TIMEOUT_HOURS"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 {
			cfg.DefaultExecutionTimeout = time.Duration(hours) * time.Hour
		}
	}

	if v := os.Getenv("WORKFLOW_TASK_TIMEOUT_MINUTES"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil && mins > 0 {
			cfg.DefaultTaskTimeout = time.Duration(mins) * time.Minute
		}
	}

	if v := os.Getenv("WORKFLOW_MAX_RETRY_ATTEMPTS"); v != "" {
		if attempts, err := strconv.Atoi(v); err == nil && attempts > 0 {
			cfg.MaxRetryAttempts = attempts
		}
	}

	if v := os.Getenv("WORKFLOW_RETRY_BACKOFF_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.RetryBackoff = time.Duration(secs) * time.Second
		}
	}

	if v := os.Getenv("WORKFLOW_METRICS_ENABLED"); v != "" {
		cfg.MetricsEnabled = v == "true" || v == "1"
	}

	if v := os.Getenv("WORKFLOW_METRICS_PREFIX"); v != "" {
		cfg.MetricsPrefix = v
	}

	return cfg
}

// TaskQueueForWorkflowType returns the appropriate task queue for a workflow type.
func (c *Config) TaskQueueForWorkflowType(workflowType string) string {
	switch workflowType {
	case WorkflowTypeIngestion:
		return c.IngestionTaskQueue
	case WorkflowTypeSync:
		return c.SyncTaskQueue
	case WorkflowTypeReview:
		return c.ReviewTaskQueue
	case WorkflowTypeAnalysis:
		return c.AnalysisTaskQueue
	default:
		return c.DefaultTaskQueue
	}
}
