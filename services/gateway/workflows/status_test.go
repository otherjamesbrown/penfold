package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/enums/v1"
)

func TestNewStatusService(t *testing.T) {
	logger := testLogger()

	t.Run("with default config", func(t *testing.T) {
		svc := NewStatusService(nil, nil, logger)
		require.NotNil(t, svc)
		assert.NotNil(t, svc.config)
		assert.Equal(t, DefaultTemporalHostPort, svc.config.TemporalHostPort)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.TemporalHostPort = "custom:7233"

		svc := NewStatusService(nil, cfg, logger)
		require.NotNil(t, svc)
		assert.Equal(t, "custom:7233", svc.config.TemporalHostPort)
	})

	t.Run("with nil logger", func(t *testing.T) {
		svc := NewStatusService(nil, nil, nil)
		require.NotNil(t, svc)
	})

	t.Run("metrics disabled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = false

		svc := NewStatusService(nil, cfg, logger)
		require.NotNil(t, svc)
		assert.Nil(t, svc.metrics)
	})

	t.Run("metrics enabled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = true

		svc := NewStatusService(nil, cfg, logger)
		require.NotNil(t, svc)
		assert.NotNil(t, svc.metrics)
	})
}

func TestStatusService_RegisterMetrics(t *testing.T) {
	logger := testLogger()

	t.Run("metrics disabled returns nil", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = false

		svc := NewStatusService(nil, cfg, logger)
		err := svc.RegisterMetrics()
		assert.NoError(t, err)
	})

	t.Run("metrics enabled registers successfully", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = true
		cfg.MetricsPrefix = "test_status_1"

		svc := NewStatusService(nil, cfg, logger)

		// Use custom registry to avoid conflicts
		reg := prometheus.NewRegistry()
		err := svc.RegisterMetricsWithRegistry(reg)
		assert.NoError(t, err)
	})

	t.Run("double registration returns no error", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = true
		cfg.MetricsPrefix = "test_status_2"

		svc := NewStatusService(nil, cfg, logger)

		reg := prometheus.NewRegistry()
		err := svc.RegisterMetricsWithRegistry(reg)
		assert.NoError(t, err)

		// Register again - should handle already registered error
		err = svc.RegisterMetricsWithRegistry(reg)
		assert.NoError(t, err)
	})
}

func TestStatusService_GetWorkflowStatus_Validation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	svc := NewStatusService(nil, cfg, logger)

	t.Run("empty workflow_id returns error", func(t *testing.T) {
		status, err := svc.GetWorkflowStatus(context.Background(), "")
		assert.Nil(t, status)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workflow_id is required")
	})
}

func TestStatusService_CancelWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	svc := NewStatusService(nil, cfg, logger)

	t.Run("empty workflow_id returns error", func(t *testing.T) {
		err := svc.CancelWorkflow(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workflow_id is required")
	})
}

func TestBuildWorkflowQuery(t *testing.T) {
	t.Run("empty filter returns empty string", func(t *testing.T) {
		query := buildWorkflowQuery(WorkflowFilter{})
		assert.Equal(t, "", query)
	})

	t.Run("workflow type only", func(t *testing.T) {
		query := buildWorkflowQuery(WorkflowFilter{
			WorkflowType: "IngestionWorkflow",
		})
		assert.Equal(t, "WorkflowType = 'IngestionWorkflow'", query)
	})

	t.Run("status only", func(t *testing.T) {
		query := buildWorkflowQuery(WorkflowFilter{
			Status: "Running",
		})
		assert.Equal(t, "ExecutionStatus = 'Running'", query)
	})

	t.Run("unknown status is ignored", func(t *testing.T) {
		query := buildWorkflowQuery(WorkflowFilter{
			Status: "InvalidStatus",
		})
		assert.Equal(t, "", query)
	})

	t.Run("start time min", func(t *testing.T) {
		startTime := time.Date(2024, 1, 18, 0, 0, 0, 0, time.UTC)
		query := buildWorkflowQuery(WorkflowFilter{
			StartTimeMin: startTime,
		})
		assert.Contains(t, query, "StartTime >=")
		assert.Contains(t, query, "2024-01-18")
	})

	t.Run("start time max", func(t *testing.T) {
		endTime := time.Date(2024, 1, 18, 23, 59, 59, 0, time.UTC)
		query := buildWorkflowQuery(WorkflowFilter{
			StartTimeMax: endTime,
		})
		assert.Contains(t, query, "StartTime <=")
		assert.Contains(t, query, "2024-01-18")
	})

	t.Run("multiple conditions combined with AND", func(t *testing.T) {
		startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		query := buildWorkflowQuery(WorkflowFilter{
			WorkflowType: "IngestionWorkflow",
			Status:       "Completed",
			StartTimeMin: startTime,
		})
		assert.Contains(t, query, "WorkflowType = 'IngestionWorkflow'")
		assert.Contains(t, query, "ExecutionStatus = 'Completed'")
		assert.Contains(t, query, " AND ")
	})
}

func TestMapStatusToEnum(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"Running", "Running"},
		{"Completed", "Completed"},
		{"Failed", "Failed"},
		{"Canceled", "Canceled"},
		{"Terminated", "Terminated"},
		{"TimedOut", "TimedOut"},
		{"ContinuedAsNew", "ContinuedAsNew"},
		{"Unknown", ""},
		{"", ""},
		{"InvalidStatus", ""},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := mapStatusToEnum(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapWorkflowStatus(t *testing.T) {
	tests := []struct {
		status   enums.WorkflowExecutionStatus
		expected string
	}{
		{enums.WORKFLOW_EXECUTION_STATUS_RUNNING, "Running"},
		{enums.WORKFLOW_EXECUTION_STATUS_COMPLETED, "Completed"},
		{enums.WORKFLOW_EXECUTION_STATUS_FAILED, "Failed"},
		{enums.WORKFLOW_EXECUTION_STATUS_CANCELED, "Canceled"},
		{enums.WORKFLOW_EXECUTION_STATUS_TERMINATED, "Terminated"},
		{enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW, "ContinuedAsNew"},
		{enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT, "TimedOut"},
		{enums.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED, "Unknown"},
		{enums.WorkflowExecutionStatus(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := mapWorkflowStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorkflowStatus_Fields(t *testing.T) {
	status := WorkflowStatus{
		WorkflowID:        "wf-123",
		RunID:             "run-456",
		WorkflowType:      "IngestionWorkflow",
		Status:            "Running",
		TaskQueue:         "ingestion-queue",
		StartTime:         time.Now(),
		PendingActivities: 3,
		HistoryLength:     100,
	}

	assert.Equal(t, "wf-123", status.WorkflowID)
	assert.Equal(t, "run-456", status.RunID)
	assert.Equal(t, "IngestionWorkflow", status.WorkflowType)
	assert.Equal(t, "Running", status.Status)
	assert.Equal(t, "ingestion-queue", status.TaskQueue)
	assert.Equal(t, 3, status.PendingActivities)
	assert.Equal(t, int64(100), status.HistoryLength)
}

func TestWorkflowInfo_Fields(t *testing.T) {
	info := WorkflowInfo{
		WorkflowID:   "wf-123",
		RunID:        "run-456",
		WorkflowType: "SyncWorkflow",
		Status:       "Completed",
		StartTime:    time.Now(),
	}

	assert.Equal(t, "wf-123", info.WorkflowID)
	assert.Equal(t, "run-456", info.RunID)
	assert.Equal(t, "SyncWorkflow", info.WorkflowType)
	assert.Equal(t, "Completed", info.Status)
}

func TestWorkflowFilter_Fields(t *testing.T) {
	filter := WorkflowFilter{
		WorkflowType:  "ReviewWorkflow",
		Status:        "Running",
		StartTimeMin:  time.Now().Add(-24 * time.Hour),
		StartTimeMax:  time.Now(),
		PageSize:      50,
		NextPageToken: []byte("token"),
	}

	assert.Equal(t, "ReviewWorkflow", filter.WorkflowType)
	assert.Equal(t, "Running", filter.Status)
	assert.Equal(t, 50, filter.PageSize)
	assert.Equal(t, []byte("token"), filter.NextPageToken)
}

func TestListWorkflowsResult_Fields(t *testing.T) {
	result := ListWorkflowsResult{
		Workflows: []WorkflowInfo{
			{WorkflowID: "wf-1"},
			{WorkflowID: "wf-2"},
		},
		NextPageToken: []byte("next-token"),
	}

	assert.Len(t, result.Workflows, 2)
	assert.Equal(t, "wf-1", result.Workflows[0].WorkflowID)
	assert.Equal(t, []byte("next-token"), result.NextPageToken)
}

func TestStatusMetrics_Creation(t *testing.T) {
	metrics := newStatusMetrics("test_prefix")

	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.statusQueries)
	assert.NotNil(t, metrics.statusQueryErrors)
	assert.NotNil(t, metrics.statusQueryLatency)
	assert.NotNil(t, metrics.listQueries)
	assert.NotNil(t, metrics.cancelRequests)
}

func TestStatusService_MetricsRecording(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MetricsEnabled = true
	cfg.MetricsPrefix = "test_record"

	svc := NewStatusService(nil, cfg, logger)

	// Test that methods don't panic when called
	// (actual metric recording requires a real client)
	t.Run("recordStatusSuccess", func(t *testing.T) {
		svc.recordStatusSuccess("Running", time.Now())
	})

	t.Run("recordStatusError", func(t *testing.T) {
		svc.recordStatusError("describe_error")
	})

	t.Run("recordListSuccess", func(t *testing.T) {
		svc.recordListSuccess(time.Now())
	})

	t.Run("recordListError", func(t *testing.T) {
		svc.recordListError("list_error")
	})

	t.Run("recordCancelSuccess", func(t *testing.T) {
		svc.recordCancelSuccess()
	})

	t.Run("recordCancelError", func(t *testing.T) {
		svc.recordCancelError()
	})
}

func TestStatusService_MetricsRecording_NilMetrics(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MetricsEnabled = false

	svc := NewStatusService(nil, cfg, logger)

	// These should not panic even when metrics are nil
	svc.recordStatusSuccess("Running", time.Now())
	svc.recordStatusError("error")
	svc.recordListSuccess(time.Now())
	svc.recordListError("error")
	svc.recordCancelSuccess()
	svc.recordCancelError()
}
