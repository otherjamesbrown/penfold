package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestDefaultInterceptorOptions(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})

	opts := DefaultInterceptorOptions(logger)

	if opts == nil {
		t.Fatal("DefaultInterceptorOptions returned nil")
	}

	if opts.Logger == nil {
		t.Error("Logger should not be nil")
	}

	if !opts.EnableTracing {
		t.Error("EnableTracing should be true by default")
	}

	if !opts.EnableLogging {
		t.Error("EnableLogging should be true by default")
	}

	if !opts.EnableMetrics {
		t.Error("EnableMetrics should be true by default")
	}
}

func TestNewInterceptors(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})

	t.Run("all enabled with custom metrics", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		metrics := NewMetrics(&MetricsConfig{
			Namespace:   "test",
			ServiceName: "worker",
			Registry:    reg,
		})

		opts := &InterceptorOptions{
			Logger:        logger,
			Metrics:       metrics,
			EnableTracing: true,
			EnableLogging: true,
			EnableMetrics: true,
		}

		interceptors := NewInterceptors(opts)

		// Should have tracing + logging interceptors
		if len(interceptors) != 2 {
			t.Errorf("expected 2 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("tracing only", func(t *testing.T) {
		opts := &InterceptorOptions{
			Logger:        logger,
			EnableTracing: true,
			EnableLogging: false,
			EnableMetrics: false,
		}

		interceptors := NewInterceptors(opts)

		if len(interceptors) != 1 {
			t.Errorf("expected 1 interceptor, got %d", len(interceptors))
		}
	})

	t.Run("logging only without metrics", func(t *testing.T) {
		opts := &InterceptorOptions{
			Logger:        logger,
			EnableTracing: false,
			EnableLogging: true,
			EnableMetrics: false,
		}

		interceptors := NewInterceptors(opts)

		if len(interceptors) != 1 {
			t.Errorf("expected 1 interceptor, got %d", len(interceptors))
		}
	})

	t.Run("none enabled", func(t *testing.T) {
		opts := &InterceptorOptions{
			Logger:        logger,
			EnableTracing: false,
			EnableLogging: false,
			EnableMetrics: false,
		}

		interceptors := NewInterceptors(opts)

		if len(interceptors) != 0 {
			t.Errorf("expected 0 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("nil options uses defaults", func(t *testing.T) {
		interceptors := NewInterceptors(nil)

		// Should have tracing only (no logger means no logging interceptor)
		if len(interceptors) != 1 {
			t.Errorf("expected 1 interceptor with nil opts, got %d", len(interceptors))
		}
	})
}

func TestInterceptorChain(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})

	t.Run("build chain", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithLogger(logger).
			WithTracing().
			WithLogging()

		interceptors := chain.Build()

		if len(interceptors) != 2 {
			t.Errorf("expected 2 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("with metrics", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		metrics := NewMetrics(&MetricsConfig{
			Namespace:   "test",
			ServiceName: "worker",
			Registry:    reg,
		})

		chain := NewInterceptorChain().
			WithLogger(logger).
			WithMetrics(metrics).
			WithTracing().
			WithLogging()

		interceptors := chain.Build()

		if len(interceptors) != 2 {
			t.Errorf("expected 2 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("logging without logger is no-op", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithTracing().
			WithLogging() // Should be skipped since no logger

		interceptors := chain.Build()

		if len(interceptors) != 1 {
			t.Errorf("expected 1 interceptor (tracing only), got %d", len(interceptors))
		}
	})

	t.Run("empty chain", func(t *testing.T) {
		chain := NewInterceptorChain()
		interceptors := chain.Build()

		if len(interceptors) != 0 {
			t.Errorf("expected 0 interceptors, got %d", len(interceptors))
		}
	})
}

func TestWorkerOption(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "worker",
		Registry:    reg,
	})

	opts := &InterceptorOptions{
		Logger:        logger,
		Metrics:       metrics,
		EnableTracing: true,
		EnableLogging: true,
		EnableMetrics: true,
	}

	// WorkerOption should return a function
	fn := WorkerOption(opts)
	if fn == nil {
		t.Fatal("WorkerOption returned nil")
	}
}
