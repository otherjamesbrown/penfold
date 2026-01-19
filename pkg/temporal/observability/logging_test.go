package observability

import (
	"bytes"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func newTestLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})
}

func TestNewWorkflowLogger(t *testing.T) {
	logger := newTestLogger()

	t.Run("creates logger", func(t *testing.T) {
		wl := NewWorkflowLogger(logger)
		if wl == nil {
			t.Fatal("NewWorkflowLogger returned nil")
		}
		if wl.logger == nil {
			t.Error("logger is nil")
		}
		if wl.metrics != nil {
			t.Error("metrics should be nil when not provided")
		}
	})
}

func TestNewWorkflowLoggerWithMetrics(t *testing.T) {
	logger := newTestLogger()
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("creates logger with metrics", func(t *testing.T) {
		wl := NewWorkflowLoggerWithMetrics(logger, metrics)
		if wl == nil {
			t.Fatal("NewWorkflowLoggerWithMetrics returned nil")
		}
		if wl.logger == nil {
			t.Error("logger is nil")
		}
		if wl.metrics == nil {
			t.Error("metrics is nil")
		}
	})

	t.Run("creates logger with nil metrics", func(t *testing.T) {
		wl := NewWorkflowLoggerWithMetrics(logger, nil)
		if wl == nil {
			t.Fatal("NewWorkflowLoggerWithMetrics returned nil")
		}
		if wl.metrics != nil {
			t.Error("metrics should be nil")
		}
	})
}

func TestNewActivityLogger(t *testing.T) {
	logger := newTestLogger()

	t.Run("creates logger", func(t *testing.T) {
		al := NewActivityLogger(logger)
		if al == nil {
			t.Fatal("NewActivityLogger returned nil")
		}
		if al.logger == nil {
			t.Error("logger is nil")
		}
		if al.metrics != nil {
			t.Error("metrics should be nil when not provided")
		}
	})
}

func TestNewActivityLoggerWithMetrics(t *testing.T) {
	logger := newTestLogger()
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("creates logger with metrics", func(t *testing.T) {
		al := NewActivityLoggerWithMetrics(logger, metrics)
		if al == nil {
			t.Fatal("NewActivityLoggerWithMetrics returned nil")
		}
		if al.logger == nil {
			t.Error("logger is nil")
		}
		if al.metrics == nil {
			t.Error("metrics is nil")
		}
	})

	t.Run("creates logger with nil metrics", func(t *testing.T) {
		al := NewActivityLoggerWithMetrics(logger, nil)
		if al == nil {
			t.Fatal("NewActivityLoggerWithMetrics returned nil")
		}
		if al.metrics != nil {
			t.Error("metrics should be nil")
		}
	})
}

func TestNewTemporalLogAdapter(t *testing.T) {
	logger := newTestLogger()
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("creates adapter", func(t *testing.T) {
		adapter := NewTemporalLogAdapter(logger, metrics)
		if adapter == nil {
			t.Fatal("NewTemporalLogAdapter returned nil")
		}
	})

	t.Run("WorkflowLogger returns logger", func(t *testing.T) {
		adapter := NewTemporalLogAdapter(logger, metrics)
		wl := adapter.WorkflowLogger()
		if wl == nil {
			t.Error("WorkflowLogger returned nil")
		}
	})

	t.Run("ActivityLogger returns logger", func(t *testing.T) {
		adapter := NewTemporalLogAdapter(logger, metrics)
		al := adapter.ActivityLogger()
		if al == nil {
			t.Error("ActivityLogger returned nil")
		}
	})

	t.Run("with nil metrics", func(t *testing.T) {
		adapter := NewTemporalLogAdapter(logger, nil)
		if adapter == nil {
			t.Fatal("NewTemporalLogAdapter returned nil")
		}
		if adapter.WorkflowLogger() == nil {
			t.Error("WorkflowLogger returned nil")
		}
		if adapter.ActivityLogger() == nil {
			t.Error("ActivityLogger returned nil")
		}
	})
}
