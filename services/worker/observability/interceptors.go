package observability

import (
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// InterceptorOptions holds configuration for creating observability interceptors.
type InterceptorOptions struct {
	// Logger is the structured logger to use. Required.
	Logger logging.Logger

	// Metrics is the metrics collector. If nil, metrics are disabled.
	Metrics *Metrics

	// EnableTracing enables OpenTelemetry tracing. Default: true.
	EnableTracing bool

	// EnableLogging enables structured logging. Default: true.
	EnableLogging bool

	// EnableMetrics enables Prometheus metrics. Default: true.
	EnableMetrics bool
}

// DefaultInterceptorOptions returns default options with all observability enabled.
func DefaultInterceptorOptions(logger logging.Logger) *InterceptorOptions {
	return &InterceptorOptions{
		Logger:        logger,
		Metrics:       nil, // Will be created if not provided
		EnableTracing: true,
		EnableLogging: true,
		EnableMetrics: true,
	}
}

// NewInterceptors creates a slice of worker interceptors for observability.
// Use this with worker.Options.Interceptors.
//
// Note: If EnableMetrics is true but Metrics is nil, metrics will not be auto-created.
// You must explicitly provide a Metrics instance if you want metrics recording.
func NewInterceptors(opts *InterceptorOptions) []interceptor.WorkerInterceptor {
	if opts == nil {
		opts = &InterceptorOptions{
			EnableTracing: true,
			EnableLogging: false, // No logger provided, so disable logging
			EnableMetrics: false, // No metrics provided, so disable metrics
		}
	}

	var interceptors []interceptor.WorkerInterceptor

	// Use provided metrics if available
	var metrics *Metrics
	if opts.EnableMetrics && opts.Metrics != nil {
		metrics = opts.Metrics
	}

	// Add tracing interceptor
	if opts.EnableTracing {
		interceptors = append(interceptors, NewTracingInterceptor())
	}

	// Add logging interceptor (includes metrics recording)
	if opts.EnableLogging && opts.Logger != nil {
		interceptors = append(interceptors, NewLoggingInterceptor(opts.Logger, metrics))
	}

	return interceptors
}

// WorkerOption returns a worker option that adds observability interceptors.
// This is a convenience function for adding interceptors to worker.Options.
func WorkerOption(opts *InterceptorOptions) func(*worker.Options) {
	interceptors := NewInterceptors(opts)
	return func(workerOpts *worker.Options) {
		workerOpts.Interceptors = append(workerOpts.Interceptors, interceptors...)
	}
}

// WithObservability adds observability interceptors to an existing worker options.
// This modifies the provided options in place and returns them for chaining.
func WithObservability(workerOpts *worker.Options, interceptorOpts *InterceptorOptions) *worker.Options {
	interceptors := NewInterceptors(interceptorOpts)
	workerOpts.Interceptors = append(workerOpts.Interceptors, interceptors...)
	return workerOpts
}

// InterceptorChain represents a chain of interceptors that can be added to a worker.
type InterceptorChain struct {
	interceptors []interceptor.WorkerInterceptor
	logger       logging.Logger
	metrics      *Metrics
}

// NewInterceptorChain creates a new empty interceptor chain.
func NewInterceptorChain() *InterceptorChain {
	return &InterceptorChain{
		interceptors: make([]interceptor.WorkerInterceptor, 0),
	}
}

// WithLogger adds a logger to the chain.
func (c *InterceptorChain) WithLogger(logger logging.Logger) *InterceptorChain {
	c.logger = logger
	return c
}

// WithMetrics adds metrics to the chain.
func (c *InterceptorChain) WithMetrics(metrics *Metrics) *InterceptorChain {
	c.metrics = metrics
	return c
}

// WithTracing adds the tracing interceptor to the chain.
func (c *InterceptorChain) WithTracing() *InterceptorChain {
	c.interceptors = append(c.interceptors, NewTracingInterceptor())
	return c
}

// WithLogging adds the logging interceptor to the chain.
// Requires that WithLogger has been called first.
func (c *InterceptorChain) WithLogging() *InterceptorChain {
	if c.logger != nil {
		c.interceptors = append(c.interceptors, NewLoggingInterceptor(c.logger, c.metrics))
	}
	return c
}

// WithCustom adds a custom interceptor to the chain.
func (c *InterceptorChain) WithCustom(i interceptor.WorkerInterceptor) *InterceptorChain {
	c.interceptors = append(c.interceptors, i)
	return c
}

// Build returns the interceptor slice for use with worker.Options.
func (c *InterceptorChain) Build() []interceptor.WorkerInterceptor {
	return c.interceptors
}

// ApplyTo applies the interceptor chain to worker options.
func (c *InterceptorChain) ApplyTo(opts *worker.Options) {
	opts.Interceptors = append(opts.Interceptors, c.interceptors...)
}
