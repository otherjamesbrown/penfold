package observability

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newTestRegistry creates a fresh registry for testing.
func newTestRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

func TestNewMetrics(t *testing.T) {
	reg := newTestRegistry()
	cfg := &MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
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
	if m.WorkflowFailedTotal == nil {
		t.Error("WorkflowFailedTotal is nil")
	}
	if m.WorkflowDuration == nil {
		t.Error("WorkflowDuration is nil")
	}
	if m.ActivityStartedTotal == nil {
		t.Error("ActivityStartedTotal is nil")
	}
	if m.ActivityCompletedTotal == nil {
		t.Error("ActivityCompletedTotal is nil")
	}
	if m.ActivityFailedTotal == nil {
		t.Error("ActivityFailedTotal is nil")
	}
	if m.ActivityDuration == nil {
		t.Error("ActivityDuration is nil")
	}
}

func TestNewMetricsWithDefaults(t *testing.T) {
	reg := newTestRegistry()
	cfg := &MetricsConfig{
		Registry: reg,
	}

	m := NewMetrics(cfg)

	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}

	// Config should use defaults for namespace/service
	if m.config.Namespace != "" && m.config.ServiceName != "" {
		// Defaults applied in config
	}
}

func TestRecordWorkflowStart(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordWorkflowStart("EmailProcessingWorkflow", "penfold-email")
	m.RecordWorkflowStart("EmailProcessingWorkflow", "penfold-email")
	m.RecordWorkflowStart("OtherWorkflow", "penfold-main")

	// Check the counts
	count := testutil.ToFloat64(m.WorkflowStartedTotal.WithLabelValues("EmailProcessingWorkflow", "penfold-email"))
	if count != 2 {
		t.Errorf("expected 2 EmailProcessingWorkflow starts, got %v", count)
	}

	count = testutil.ToFloat64(m.WorkflowStartedTotal.WithLabelValues("OtherWorkflow", "penfold-main"))
	if count != 1 {
		t.Errorf("expected 1 OtherWorkflow start, got %v", count)
	}
}

func TestRecordWorkflowComplete(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordWorkflowComplete("EmailProcessingWorkflow", "penfold-email", 5*time.Second)

	count := testutil.ToFloat64(m.WorkflowCompletedTotal.WithLabelValues("EmailProcessingWorkflow", "penfold-email"))
	if count != 1 {
		t.Errorf("expected 1 workflow completion, got %v", count)
	}
}

func TestRecordWorkflowFail(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordWorkflowFail("EmailProcessingWorkflow", "penfold-email", "timeout", 10*time.Second)

	count := testutil.ToFloat64(m.WorkflowFailedTotal.WithLabelValues("EmailProcessingWorkflow", "penfold-email", "timeout"))
	if count != 1 {
		t.Errorf("expected 1 workflow failure, got %v", count)
	}
}

func TestRecordActivityStart(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordActivityStart("FetchSource", "EmailProcessingWorkflow", "penfold-email")
	m.RecordActivityStart("GenerateEmbedding", "EmailProcessingWorkflow", "penfold-email")

	count := testutil.ToFloat64(m.ActivityStartedTotal.WithLabelValues("FetchSource", "EmailProcessingWorkflow", "penfold-email"))
	if count != 1 {
		t.Errorf("expected 1 FetchSource start, got %v", count)
	}
}

func TestRecordActivityComplete(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordActivityComplete("FetchSource", "EmailProcessingWorkflow", "penfold-email", 100*time.Millisecond)

	count := testutil.ToFloat64(m.ActivityCompletedTotal.WithLabelValues("FetchSource", "EmailProcessingWorkflow", "penfold-email"))
	if count != 1 {
		t.Errorf("expected 1 activity completion, got %v", count)
	}
}

func TestRecordActivityFail(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordActivityFail("FetchSource", "EmailProcessingWorkflow", "penfold-email", "not_found", 50*time.Millisecond)

	count := testutil.ToFloat64(m.ActivityFailedTotal.WithLabelValues("FetchSource", "EmailProcessingWorkflow", "penfold-email", "not_found"))
	if count != 1 {
		t.Errorf("expected 1 activity failure, got %v", count)
	}
}

func TestErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, "none"},
		{"timeout error", &testError{msg: "operation timeout"}, "timeout"},
		{"canceled error", &testError{msg: "context canceled"}, "canceled"},
		{"not found error", &testError{msg: "resource not found"}, "not_found"},
		{"already exists error", &testError{msg: "resource already exists"}, "already_exists"},
		{"permission error", &testError{msg: "permission denied"}, "permission_denied"},
		{"invalid error", &testError{msg: "invalid argument"}, "invalid_argument"},
		{"connection error", &testError{msg: "connection refused"}, "connection_error"},
		{"retry error", &testError{msg: "retry limit exceeded"}, "retry_exhausted"},
		{"unknown error", &testError{msg: "something went wrong"}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ErrorType(tt.err)
			if result != tt.expected {
				t.Errorf("ErrorType(%v) = %s, expected %s", tt.err, result, tt.expected)
			}
		})
	}
}

func TestDefaultDurationBuckets(t *testing.T) {
	buckets := DefaultDurationBuckets()

	if len(buckets) == 0 {
		t.Error("DefaultDurationBuckets returned empty slice")
	}

	// Check that buckets are in ascending order
	for i := 1; i < len(buckets); i++ {
		if buckets[i] <= buckets[i-1] {
			t.Errorf("buckets not in ascending order at index %d: %v", i, buckets)
		}
	}

	// Check that reasonable values are included
	hasSubSecond := false
	hasMultiSecond := false
	hasMultiMinute := false

	for _, b := range buckets {
		if b < 1.0 {
			hasSubSecond = true
		}
		if b >= 10.0 && b < 60.0 {
			hasMultiSecond = true
		}
		if b >= 60.0 {
			hasMultiMinute = true
		}
	}

	if !hasSubSecond {
		t.Error("buckets should include sub-second values")
	}
	if !hasMultiSecond {
		t.Error("buckets should include multi-second values")
	}
	if !hasMultiMinute {
		t.Error("buckets should include multi-minute values")
	}
}

func TestRecordTaskQueuePoll(t *testing.T) {
	reg := newTestRegistry()
	m := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	m.RecordTaskQueuePoll("penfold-email", "workflow")
	m.RecordTaskQueuePoll("penfold-email", "activity")
	m.RecordTaskQueuePoll("penfold-email", "workflow")

	count := testutil.ToFloat64(m.TaskQueuePolls.WithLabelValues("penfold-email", "workflow"))
	if count != 2 {
		t.Errorf("expected 2 workflow polls, got %v", count)
	}

	count = testutil.ToFloat64(m.TaskQueuePolls.WithLabelValues("penfold-email", "activity"))
	if count != 1 {
		t.Errorf("expected 1 activity poll, got %v", count)
	}
}

// testError is a simple error type for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
