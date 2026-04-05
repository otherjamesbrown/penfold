package workflows

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, DefaultTemporalHostPort, cfg.TemporalHostPort)
	assert.Equal(t, DefaultTemporalNamespace, cfg.TemporalNamespace)
	assert.Equal(t, DefaultDefaultTaskQueue, cfg.DefaultTaskQueue)
	assert.Equal(t, DefaultIngestionTaskQueue, cfg.IngestionTaskQueue)
	assert.Equal(t, DefaultSyncTaskQueue, cfg.SyncTaskQueue)
	assert.Equal(t, DefaultReviewTaskQueue, cfg.ReviewTaskQueue)
	assert.Equal(t, DefaultAnalysisTaskQueue, cfg.AnalysisTaskQueue)
	assert.Equal(t, DefaultStartTimeout, cfg.DefaultStartTimeout)
	assert.Equal(t, DefaultExecutionTimeout, cfg.DefaultExecutionTimeout)
	assert.Equal(t, DefaultTaskTimeout, cfg.DefaultTaskTimeout)
	assert.Equal(t, DefaultMaxRetryAttempts, cfg.MaxRetryAttempts)
	assert.Equal(t, DefaultRetryBackoff, cfg.RetryBackoff)
	assert.Equal(t, DefaultMetricsEnabled, cfg.MetricsEnabled)
	assert.Equal(t, DefaultMetricsPrefix, cfg.MetricsPrefix)
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Save and restore environment
	envVars := []string{
		"TEMPORAL_HOST_PORT",
		"TEMPORAL_NAMESPACE",
		"WORKFLOW_DEFAULT_TASK_QUEUE",
		"WORKFLOW_INGESTION_TASK_QUEUE",
		"WORKFLOW_SYNC_TASK_QUEUE",
		"WORKFLOW_REVIEW_TASK_QUEUE",
		"WORKFLOW_ANALYSIS_TASK_QUEUE",
		"WORKFLOW_START_TIMEOUT_SECONDS",
		"WORKFLOW_EXECUTION_TIMEOUT_HOURS",
		"WORKFLOW_TASK_TIMEOUT_MINUTES",
		"WORKFLOW_MAX_RETRY_ATTEMPTS",
		"WORKFLOW_RETRY_BACKOFF_SECONDS",
		"WORKFLOW_METRICS_ENABLED",
		"WORKFLOW_METRICS_PREFIX",
	}

	savedEnv := make(map[string]string)
	for _, key := range envVars {
		savedEnv[key] = os.Getenv(key)
		_ = os.Unsetenv(key)
	}
	defer func() {
		for key, value := range savedEnv {
			if value != "" {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}()

	t.Run("default values", func(t *testing.T) {
		cfg := LoadConfigFromEnv()
		assert.Equal(t, DefaultTemporalHostPort, cfg.TemporalHostPort)
		assert.Equal(t, DefaultTemporalNamespace, cfg.TemporalNamespace)
	})

	t.Run("custom values from env", func(t *testing.T) {
		_ = os.Setenv("TEMPORAL_HOST_PORT", "temporal.example.com:7233")
		_ = os.Setenv("TEMPORAL_NAMESPACE", "custom-ns")
		_ = os.Setenv("WORKFLOW_DEFAULT_TASK_QUEUE", "custom-queue")
		_ = os.Setenv("WORKFLOW_INGESTION_TASK_QUEUE", "ingest-queue")
		_ = os.Setenv("WORKFLOW_SYNC_TASK_QUEUE", "sync-queue")
		_ = os.Setenv("WORKFLOW_REVIEW_TASK_QUEUE", "review-queue")
		_ = os.Setenv("WORKFLOW_ANALYSIS_TASK_QUEUE", "analysis-queue")
		_ = os.Setenv("WORKFLOW_START_TIMEOUT_SECONDS", "30")
		_ = os.Setenv("WORKFLOW_EXECUTION_TIMEOUT_HOURS", "48")
		_ = os.Setenv("WORKFLOW_TASK_TIMEOUT_MINUTES", "20")
		_ = os.Setenv("WORKFLOW_MAX_RETRY_ATTEMPTS", "5")
		_ = os.Setenv("WORKFLOW_RETRY_BACKOFF_SECONDS", "3")
		_ = os.Setenv("WORKFLOW_METRICS_ENABLED", "false")
		_ = os.Setenv("WORKFLOW_METRICS_PREFIX", "custom_prefix")

		cfg := LoadConfigFromEnv()

		assert.Equal(t, "temporal.example.com:7233", cfg.TemporalHostPort)
		assert.Equal(t, "custom-ns", cfg.TemporalNamespace)
		assert.Equal(t, "custom-queue", cfg.DefaultTaskQueue)
		assert.Equal(t, "ingest-queue", cfg.IngestionTaskQueue)
		assert.Equal(t, "sync-queue", cfg.SyncTaskQueue)
		assert.Equal(t, "review-queue", cfg.ReviewTaskQueue)
		assert.Equal(t, "analysis-queue", cfg.AnalysisTaskQueue)
		assert.Equal(t, 30*time.Second, cfg.DefaultStartTimeout)
		assert.Equal(t, 48*time.Hour, cfg.DefaultExecutionTimeout)
		assert.Equal(t, 20*time.Minute, cfg.DefaultTaskTimeout)
		assert.Equal(t, 5, cfg.MaxRetryAttempts)
		assert.Equal(t, 3*time.Second, cfg.RetryBackoff)
		assert.False(t, cfg.MetricsEnabled)
		assert.Equal(t, "custom_prefix", cfg.MetricsPrefix)
	})

	t.Run("invalid numeric values ignored", func(t *testing.T) {
		os.Setenv("WORKFLOW_START_TIMEOUT_SECONDS", "invalid")
		os.Setenv("WORKFLOW_MAX_RETRY_ATTEMPTS", "-5")

		cfg := LoadConfigFromEnv()

		// Should use defaults for invalid values
		assert.Equal(t, DefaultStartTimeout, cfg.DefaultStartTimeout)
		assert.Equal(t, DefaultMaxRetryAttempts, cfg.MaxRetryAttempts)
	})

	t.Run("metrics enabled with 1", func(t *testing.T) {
		os.Setenv("WORKFLOW_METRICS_ENABLED", "1")
		cfg := LoadConfigFromEnv()
		assert.True(t, cfg.MetricsEnabled)
	})
}

func TestTaskQueueForWorkflowType(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		workflowType string
		expected     string
	}{
		{WorkflowTypeIngestion, DefaultIngestionTaskQueue},
		{WorkflowTypeSync, DefaultSyncTaskQueue},
		{WorkflowTypeReview, DefaultReviewTaskQueue},
		{WorkflowTypeAnalysis, DefaultAnalysisTaskQueue},
		{"unknown", DefaultDefaultTaskQueue},
		{"", DefaultDefaultTaskQueue},
	}

	for _, tt := range tests {
		t.Run(tt.workflowType, func(t *testing.T) {
			result := cfg.TaskQueueForWorkflowType(tt.workflowType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigConstants(t *testing.T) {
	// Verify constants are sensible values
	require.NotEmpty(t, DefaultTemporalHostPort)
	require.NotEmpty(t, DefaultTemporalNamespace)
	require.NotEmpty(t, DefaultDefaultTaskQueue)

	require.Greater(t, DefaultStartTimeout, time.Duration(0))
	require.Greater(t, DefaultExecutionTimeout, time.Duration(0))
	require.Greater(t, DefaultTaskTimeout, time.Duration(0))

	require.Greater(t, DefaultMaxRetryAttempts, 0)
	require.Greater(t, DefaultRetryBackoff, time.Duration(0))
}
