package observability

import (
	"bytes"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestNewMetricsInterceptor(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("creates interceptor", func(t *testing.T) {
		mi := NewMetricsInterceptor(metrics)
		if mi == nil {
			t.Fatal("NewMetricsInterceptor returned nil")
		}
		if mi.metrics == nil {
			t.Error("metrics is nil")
		}
	})
}

func TestNewTracingInterceptor(t *testing.T) {
	t.Run("creates interceptor with default name", func(t *testing.T) {
		ti := NewTracingInterceptor()
		if ti == nil {
			t.Fatal("NewTracingInterceptor returned nil")
		}
		if ti.tracer == nil {
			t.Error("tracer is nil")
		}
	})

	t.Run("creates interceptor with custom name", func(t *testing.T) {
		ti := NewTracingInterceptorWithName("custom-tracer")
		if ti == nil {
			t.Fatal("NewTracingInterceptorWithName returned nil")
		}
		if ti.tracer == nil {
			t.Error("tracer is nil")
		}
	})
}

func TestNewLoggingInterceptor(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("creates interceptor", func(t *testing.T) {
		li := NewLoggingInterceptor(logger, metrics)
		if li == nil {
			t.Fatal("NewLoggingInterceptor returned nil")
		}
		if li.logger == nil {
			t.Error("logger is nil")
		}
		if li.activityLogger == nil {
			t.Error("activityLogger is nil")
		}
		if li.workflowLogger == nil {
			t.Error("workflowLogger is nil")
		}
	})

	t.Run("with nil metrics", func(t *testing.T) {
		li := NewLoggingInterceptor(logger, nil)
		if li == nil {
			t.Fatal("NewLoggingInterceptor returned nil")
		}
		if li.metrics != nil {
			t.Error("metrics should be nil")
		}
	})
}

func TestNewChainedInterceptor(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})
	mi := NewMetricsInterceptor(metrics)
	ti := NewTracingInterceptor()

	t.Run("creates chained interceptor", func(t *testing.T) {
		ci := NewChainedInterceptor(mi, ti)
		if ci == nil {
			t.Fatal("NewChainedInterceptor returned nil")
		}
	})

	t.Run("Interceptors returns underlying list", func(t *testing.T) {
		ci := NewChainedInterceptor(mi, ti)
		interceptors := ci.Interceptors()
		if len(interceptors) != 2 {
			t.Errorf("len(Interceptors) = %d, want 2", len(interceptors))
		}
	})

	t.Run("with empty list", func(t *testing.T) {
		ci := NewChainedInterceptor()
		if ci == nil {
			t.Fatal("NewChainedInterceptor returned nil")
		}
		if len(ci.Interceptors()) != 0 {
			t.Error("Expected empty interceptor list")
		}
	})
}

func TestInterceptorOptions(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})

	t.Run("DefaultInterceptorOptions", func(t *testing.T) {
		opts := DefaultInterceptorOptions(logger)
		if opts == nil {
			t.Fatal("DefaultInterceptorOptions returned nil")
		}
		if opts.Logger == nil {
			t.Error("Logger is nil")
		}
		if !opts.EnableTracing {
			t.Error("EnableTracing should be true")
		}
		if !opts.EnableLogging {
			t.Error("EnableLogging should be true")
		}
		if !opts.EnableMetrics {
			t.Error("EnableMetrics should be true")
		}
	})
}

func TestNewInterceptors(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("with nil options", func(t *testing.T) {
		interceptors := NewInterceptors(nil)
		if interceptors == nil {
			t.Fatal("NewInterceptors returned nil")
		}
		// With nil options, should have tracing enabled by default
		if len(interceptors) != 1 {
			t.Errorf("Expected 1 interceptor (tracing only), got %d", len(interceptors))
		}
	})

	t.Run("with all options enabled", func(t *testing.T) {
		opts := &InterceptorOptions{
			Logger:        logger,
			Metrics:       metrics,
			EnableTracing: true,
			EnableLogging: true,
			EnableMetrics: true,
		}
		interceptors := NewInterceptors(opts)
		if len(interceptors) != 3 {
			t.Errorf("Expected 3 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("with tracing only", func(t *testing.T) {
		opts := &InterceptorOptions{
			EnableTracing: true,
			EnableLogging: false,
			EnableMetrics: false,
		}
		interceptors := NewInterceptors(opts)
		if len(interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(interceptors))
		}
	})

	t.Run("with custom tracer name", func(t *testing.T) {
		opts := &InterceptorOptions{
			EnableTracing: true,
			TracerName:    "custom-tracer",
		}
		interceptors := NewInterceptors(opts)
		if len(interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(interceptors))
		}
	})

	t.Run("with logging but no logger", func(t *testing.T) {
		opts := &InterceptorOptions{
			Logger:        nil,
			EnableLogging: true,
			EnableTracing: false,
		}
		interceptors := NewInterceptors(opts)
		// Should not add logging interceptor without logger
		if len(interceptors) != 0 {
			t.Errorf("Expected 0 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("with metrics but no metrics instance", func(t *testing.T) {
		opts := &InterceptorOptions{
			Metrics:       nil,
			EnableMetrics: true,
			EnableTracing: false,
		}
		interceptors := NewInterceptors(opts)
		// Should not add metrics interceptor without metrics
		if len(interceptors) != 0 {
			t.Errorf("Expected 0 interceptors, got %d", len(interceptors))
		}
	})
}

func TestWorkerOption(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})

	opts := &InterceptorOptions{
		Logger:        logger,
		EnableTracing: true,
		EnableLogging: true,
	}

	t.Run("returns function", func(t *testing.T) {
		fn := WorkerOption(opts)
		if fn == nil {
			t.Fatal("WorkerOption returned nil")
		}
	})

	t.Run("function modifies worker options", func(t *testing.T) {
		fn := WorkerOption(opts)
		workerOpts := &worker.Options{}

		fn(workerOpts)

		if len(workerOpts.Interceptors) == 0 {
			t.Error("Worker options should have interceptors")
		}
	})
}

func TestWithObservability(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("adds interceptors to worker options", func(t *testing.T) {
		workerOpts := &worker.Options{}
		interceptorOpts := &InterceptorOptions{
			Logger:        logger,
			Metrics:       metrics,
			EnableTracing: true,
			EnableLogging: true,
			EnableMetrics: true,
		}

		result := WithObservability(workerOpts, interceptorOpts)

		if result == nil {
			t.Fatal("WithObservability returned nil")
		}
		if len(result.Interceptors) != 3 {
			t.Errorf("Expected 3 interceptors, got %d", len(result.Interceptors))
		}
	})

	t.Run("preserves existing interceptors", func(t *testing.T) {
		existingInterceptor := NewTracingInterceptor()
		workerOpts := &worker.Options{
			Interceptors: []interceptor.WorkerInterceptor{existingInterceptor},
		}
		interceptorOpts := &InterceptorOptions{
			EnableTracing: true,
		}

		result := WithObservability(workerOpts, interceptorOpts)

		if len(result.Interceptors) != 2 {
			t.Errorf("Expected 2 interceptors (1 existing + 1 new), got %d", len(result.Interceptors))
		}
	})
}

func TestInterceptorChain(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(&MetricsConfig{
		Namespace:   "test",
		ServiceName: "test",
		Registry:    reg,
	})

	t.Run("NewInterceptorChain creates empty chain", func(t *testing.T) {
		chain := NewInterceptorChain()
		if chain == nil {
			t.Fatal("NewInterceptorChain returned nil")
		}
		if len(chain.interceptors) != 0 {
			t.Error("Chain should start empty")
		}
	})

	t.Run("WithLogger", func(t *testing.T) {
		chain := NewInterceptorChain().WithLogger(logger)
		if chain.logger == nil {
			t.Error("logger not set")
		}
	})

	t.Run("WithMetrics", func(t *testing.T) {
		chain := NewInterceptorChain().WithMetrics(metrics)
		if chain.metrics == nil {
			t.Error("metrics not set")
		}
	})

	t.Run("WithTracerName", func(t *testing.T) {
		chain := NewInterceptorChain().WithTracerName("custom")
		if chain.tracerName != "custom" {
			t.Error("tracerName not set")
		}
	})

	t.Run("WithTracing adds tracing interceptor", func(t *testing.T) {
		chain := NewInterceptorChain().WithTracing()
		if len(chain.interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(chain.interceptors))
		}
	})

	t.Run("WithTracing uses custom name", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithTracerName("custom-tracer").
			WithTracing()
		if len(chain.interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(chain.interceptors))
		}
	})

	t.Run("WithMetricsInterceptor adds metrics interceptor", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithMetrics(metrics).
			WithMetricsInterceptor()
		if len(chain.interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(chain.interceptors))
		}
	})

	t.Run("WithMetricsInterceptor without metrics does nothing", func(t *testing.T) {
		chain := NewInterceptorChain().WithMetricsInterceptor()
		if len(chain.interceptors) != 0 {
			t.Errorf("Expected 0 interceptors, got %d", len(chain.interceptors))
		}
	})

	t.Run("WithLogging adds logging interceptor", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithLogger(logger).
			WithLogging()
		if len(chain.interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(chain.interceptors))
		}
	})

	t.Run("WithLogging without logger does nothing", func(t *testing.T) {
		chain := NewInterceptorChain().WithLogging()
		if len(chain.interceptors) != 0 {
			t.Errorf("Expected 0 interceptors, got %d", len(chain.interceptors))
		}
	})

	t.Run("WithCustom adds custom interceptor", func(t *testing.T) {
		custom := NewTracingInterceptor()
		chain := NewInterceptorChain().WithCustom(custom)
		if len(chain.interceptors) != 1 {
			t.Errorf("Expected 1 interceptor, got %d", len(chain.interceptors))
		}
	})

	t.Run("Build returns interceptor slice", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithTracing().
			WithLogger(logger).
			WithLogging()
		interceptors := chain.Build()
		if len(interceptors) != 2 {
			t.Errorf("Expected 2 interceptors, got %d", len(interceptors))
		}
	})

	t.Run("ApplyTo modifies worker options", func(t *testing.T) {
		workerOpts := &worker.Options{}
		chain := NewInterceptorChain().
			WithTracing().
			WithMetrics(metrics).
			WithMetricsInterceptor()

		chain.ApplyTo(workerOpts)

		if len(workerOpts.Interceptors) != 2 {
			t.Errorf("Expected 2 interceptors, got %d", len(workerOpts.Interceptors))
		}
	})

	t.Run("fluent interface chain", func(t *testing.T) {
		chain := NewInterceptorChain().
			WithLogger(logger).
			WithMetrics(metrics).
			WithTracerName("my-tracer").
			WithTracing().
			WithMetricsInterceptor().
			WithLogging()

		interceptors := chain.Build()
		if len(interceptors) != 3 {
			t.Errorf("Expected 3 interceptors, got %d", len(interceptors))
		}
	})
}
