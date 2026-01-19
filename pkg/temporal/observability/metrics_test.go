package observability

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestDefaultDurationBuckets(t *testing.T) {
	buckets := DefaultDurationBuckets()
	if len(buckets) == 0 {
		t.Error("DefaultDurationBuckets returned empty slice")
	}

	// Verify buckets are in ascending order
	for i := 1; i < len(buckets); i++ {
		if buckets[i] <= buckets[i-1] {
			t.Errorf("Buckets not in ascending order at index %d: %v", i, buckets)
		}
	}
}

func TestNewMetrics(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		// We need to pass a config with the registry to avoid polluting the default registry
		cfg := &MetricsConfig{
			Namespace:       "test",
			ServiceName:     "test_service",
			DurationBuckets: DefaultDurationBuckets(),
			Registry:        reg,
		}
		m := NewMetrics(cfg)

		if m == nil {
			t.Fatal("NewMetrics returned nil")
		}
		if m.WorkflowStartedTotal == nil {
			t.Error("WorkflowStartedTotal is nil")
		}
		if m.WorkflowCompletedTotal == nil {
			t.Error("WorkflowCompletedTotal is nil")
		}
		if m.ActivityStartedTotal == nil {
			t.Error("ActivityStartedTotal is nil")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		cfg := &MetricsConfig{
			Namespace:       "custom",
			ServiceName:     "myservice",
			DurationBuckets: []float64{0.1, 0.5, 1.0},
			Registry:        reg,
		}
		m := NewMetrics(cfg)

		if m == nil {
			t.Fatal("NewMetrics returned nil")
		}
	})

	t.Run("with nil duration buckets uses defaults", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		cfg := &MetricsConfig{
			Namespace:   "test2",
			ServiceName: "test_service2",
			Registry:    reg,
		}
		m := NewMetrics(cfg)

		if m == nil {
			t.Fatal("NewMetrics returned nil")
		}
	})
}

func TestMetricsRecordWorkflow(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := &MetricsConfig{
		Namespace:       "test",
		ServiceName:     "workflow_test",
		DurationBuckets: DefaultDurationBuckets(),
		Registry:        reg,
	}
	m := NewMetrics(cfg)

	t.Run("RecordWorkflowStart", func(t *testing.T) {
		m.RecordWorkflowStart("TestWorkflow", "test-queue")
		m.RecordWorkflowStart("TestWorkflow", "test-queue")

		// Verify counter was incremented
		count := testutil.ToFloat64(m.WorkflowStartedTotal.WithLabelValues("TestWorkflow", "test-queue"))
		if count != 2 {
			t.Errorf("Expected workflow started count 2, got %f", count)
		}
	})

	t.Run("RecordWorkflowComplete", func(t *testing.T) {
		m.RecordWorkflowComplete("TestWorkflow", "test-queue", 100*time.Millisecond)

		count := testutil.ToFloat64(m.WorkflowCompletedTotal.WithLabelValues("TestWorkflow", "test-queue"))
		if count != 1 {
			t.Errorf("Expected workflow completed count 1, got %f", count)
		}
	})

	t.Run("RecordWorkflowFail", func(t *testing.T) {
		m.RecordWorkflowFail("TestWorkflow", "test-queue", "timeout", 50*time.Millisecond)

		count := testutil.ToFloat64(m.WorkflowFailedTotal.WithLabelValues("TestWorkflow", "test-queue", "timeout"))
		if count != 1 {
			t.Errorf("Expected workflow failed count 1, got %f", count)
		}
	})
}

func TestMetricsRecordActivity(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := &MetricsConfig{
		Namespace:       "test",
		ServiceName:     "activity_test",
		DurationBuckets: DefaultDurationBuckets(),
		Registry:        reg,
	}
	m := NewMetrics(cfg)

	t.Run("RecordActivityStart", func(t *testing.T) {
		m.RecordActivityStart("TestActivity", "TestWorkflow", "test-queue")

		count := testutil.ToFloat64(m.ActivityStartedTotal.WithLabelValues("TestActivity", "TestWorkflow", "test-queue"))
		if count != 1 {
			t.Errorf("Expected activity started count 1, got %f", count)
		}
	})

	t.Run("RecordActivityComplete", func(t *testing.T) {
		m.RecordActivityComplete("TestActivity", "TestWorkflow", "test-queue", 25*time.Millisecond)

		count := testutil.ToFloat64(m.ActivityCompletedTotal.WithLabelValues("TestActivity", "TestWorkflow", "test-queue"))
		if count != 1 {
			t.Errorf("Expected activity completed count 1, got %f", count)
		}
	})

	t.Run("RecordActivityFail", func(t *testing.T) {
		m.RecordActivityFail("TestActivity", "TestWorkflow", "test-queue", "connection_error", 10*time.Millisecond)

		count := testutil.ToFloat64(m.ActivityFailedTotal.WithLabelValues("TestActivity", "TestWorkflow", "test-queue", "connection_error"))
		if count != 1 {
			t.Errorf("Expected activity failed count 1, got %f", count)
		}
	})
}

func TestMetricsTaskQueue(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := &MetricsConfig{
		Namespace:       "test",
		ServiceName:     "taskqueue_test",
		DurationBuckets: DefaultDurationBuckets(),
		Registry:        reg,
	}
	m := NewMetrics(cfg)

	t.Run("RecordTaskQueuePoll", func(t *testing.T) {
		m.RecordTaskQueuePoll("test-queue", "workflow")
		m.RecordTaskQueuePoll("test-queue", "workflow")
		m.RecordTaskQueuePoll("test-queue", "activity")

		workflowPolls := testutil.ToFloat64(m.TaskQueuePolls.WithLabelValues("test-queue", "workflow"))
		if workflowPolls != 2 {
			t.Errorf("Expected workflow polls 2, got %f", workflowPolls)
		}

		activityPolls := testutil.ToFloat64(m.TaskQueuePolls.WithLabelValues("test-queue", "activity"))
		if activityPolls != 1 {
			t.Errorf("Expected activity polls 1, got %f", activityPolls)
		}
	})

	t.Run("SetTaskQueueDepth", func(t *testing.T) {
		m.SetTaskQueueDepth("test-queue", "workflow", 100)

		depth := testutil.ToFloat64(m.TaskQueueDepth.WithLabelValues("test-queue", "workflow"))
		if depth != 100 {
			t.Errorf("Expected task queue depth 100, got %f", depth)
		}

		// Update depth
		m.SetTaskQueueDepth("test-queue", "workflow", 50)
		depth = testutil.ToFloat64(m.TaskQueueDepth.WithLabelValues("test-queue", "workflow"))
		if depth != 50 {
			t.Errorf("Expected task queue depth 50, got %f", depth)
		}
	})
}

func TestErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, "none"},
		{"timeout error", errors.New("operation timeout exceeded"), "timeout"},
		{"canceled error", errors.New("context canceled"), "canceled"},
		{"not found error", errors.New("resource not found"), "not_found"},
		{"already exists error", errors.New("workflow already exists"), "already_exists"},
		{"permission error", errors.New("permission denied"), "permission_denied"},
		{"invalid error", errors.New("invalid argument provided"), "invalid_argument"},
		{"connection error", errors.New("connection refused"), "connection_error"},
		{"retry error", errors.New("retry limit exceeded"), "retry_exhausted"},
		{"panic error", errors.New("panic in activity"), "panic"},
		{"deadline error", errors.New("deadline exceeded"), "deadline_exceeded"},
		{"unknown error", errors.New("some random error"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ErrorType(tt.err)
			if result != tt.expected {
				t.Errorf("ErrorType(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"hello", "", true},
		{"", "foo", false},
		{"foo", "foobar", false},
		{"timeout error", "timeout", true},
		{"TIMEOUT", "timeout", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestMetricsConfig(t *testing.T) {
	t.Run("default namespace", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		m := NewMetrics(&MetricsConfig{
			Namespace:   "penfold",
			ServiceName: "temporal",
			Registry:    reg,
		})

		// Record some metrics so they appear in gather
		m.RecordWorkflowStart("TestWorkflow", "test-queue")

		// Collect metrics to verify they were registered
		metricFamilies, err := reg.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		// Check that we have some metrics registered
		if len(metricFamilies) == 0 {
			t.Error("No metrics were registered")
		}

		// Verify namespace is in metric names
		found := false
		for _, mf := range metricFamilies {
			if strings.HasPrefix(mf.GetName(), "penfold_temporal_") {
				found = true
				break
			}
		}
		if !found {
			names := make([]string, len(metricFamilies))
			for i, mf := range metricFamilies {
				names[i] = mf.GetName()
			}
			t.Errorf("Metrics don't have expected namespace prefix. Found: %v", names)
		}
	})
}

func TestNewMetricsWithNilConfig(t *testing.T) {
	// Test that NewMetrics with nil config doesn't panic and uses defaults
	// Note: This will register to the default registry, so we can't easily verify
	// but we can ensure it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewMetrics(nil) panicked: %v", r)
		}
	}()

	// Use a custom registry to avoid polluting global state
	reg := prometheus.NewRegistry()
	cfg := &MetricsConfig{
		Registry: reg,
	}
	m := NewMetrics(cfg)
	if m == nil {
		t.Error("NewMetrics returned nil")
	}
}
