// Package temporal provides Temporal worker creation and configuration for Penfold services.
package temporal

import (
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// WorkerOption is a function that configures worker options.
type WorkerOption func(*worker.Options)

// WithMaxConcurrentActivities sets the maximum concurrent activity executions.
func WithMaxConcurrentActivities(max int) WorkerOption {
	return func(opts *worker.Options) {
		opts.MaxConcurrentActivityExecutionSize = max
	}
}

// WithMaxConcurrentWorkflows sets the maximum concurrent workflow task executions.
func WithMaxConcurrentWorkflows(max int) WorkerOption {
	return func(opts *worker.Options) {
		opts.MaxConcurrentWorkflowTaskExecutionSize = max
	}
}

// WithWorkerStopTimeout sets the graceful stop timeout.
func WithWorkerStopTimeout(timeout time.Duration) WorkerOption {
	return func(opts *worker.Options) {
		opts.WorkerStopTimeout = timeout
	}
}

// WithEnableSessionWorker enables session worker for sticky activities.
func WithEnableSessionWorker(enable bool) WorkerOption {
	return func(opts *worker.Options) {
		opts.EnableSessionWorker = enable
	}
}

// WithMaxConcurrentSessionExecutions sets max concurrent session executions.
func WithMaxConcurrentSessionExecutions(max int) WorkerOption {
	return func(opts *worker.Options) {
		opts.MaxConcurrentSessionExecutionSize = max
	}
}

// WithDisableWorkflowWorker disables workflow execution on this worker.
// Use when you want a dedicated activity worker.
func WithDisableWorkflowWorker(disable bool) WorkerOption {
	return func(opts *worker.Options) {
		opts.DisableWorkflowWorker = disable
	}
}

// WithDisableActivityWorker disables activity execution on this worker.
// Use when you want a dedicated workflow worker.
func WithDisableActivityWorker(disable bool) WorkerOption {
	return func(opts *worker.Options) {
		opts.DisableEagerActivities = disable
	}
}

// NewWorker creates a new Temporal worker with the given configuration.
func NewWorker(c client.Client, taskQueue string, options ...WorkerOption) worker.Worker {
	opts := worker.Options{
		MaxConcurrentActivityExecutionSize:     10,
		MaxConcurrentWorkflowTaskExecutionSize: 10,
	}

	// Apply options
	for _, opt := range options {
		opt(&opts)
	}

	return worker.New(c, taskQueue, opts)
}

// NewWorkerFromConfig creates a new Temporal worker using the Config.
func NewWorkerFromConfig(c client.Client, cfg *Config, options ...WorkerOption) worker.Worker {
	// Apply config defaults first
	defaults := []WorkerOption{}
	if cfg.MaxConcurrentActivities > 0 {
		defaults = append(defaults, WithMaxConcurrentActivities(cfg.MaxConcurrentActivities))
	}
	if cfg.MaxConcurrentWorkflows > 0 {
		defaults = append(defaults, WithMaxConcurrentWorkflows(cfg.MaxConcurrentWorkflows))
	}

	// Merge defaults with provided options (provided options take precedence)
	allOptions := append(defaults, options...)

	return NewWorker(c, cfg.TaskQueue, allOptions...)
}

// WorkerRegistry provides a fluent interface for registering workflows and activities.
type WorkerRegistry struct {
	worker worker.Worker
}

// NewWorkerRegistry creates a new registry wrapper around a worker.
func NewWorkerRegistry(w worker.Worker) *WorkerRegistry {
	return &WorkerRegistry{worker: w}
}

// RegisterWorkflow registers a workflow function with the worker.
func (r *WorkerRegistry) RegisterWorkflow(wf interface{}) *WorkerRegistry {
	r.worker.RegisterWorkflow(wf)
	return r
}

// RegisterWorkflowWithOptions registers a workflow function with custom options.
func (r *WorkerRegistry) RegisterWorkflowWithOptions(wf interface{}, opts workflow.RegisterOptions) *WorkerRegistry {
	r.worker.RegisterWorkflowWithOptions(wf, opts)
	return r
}

// RegisterActivity registers an activity function or struct with the worker.
func (r *WorkerRegistry) RegisterActivity(act interface{}) *WorkerRegistry {
	r.worker.RegisterActivity(act)
	return r
}

// RegisterActivityWithOptions registers an activity with custom options.
func (r *WorkerRegistry) RegisterActivityWithOptions(act interface{}, opts activity.RegisterOptions) *WorkerRegistry {
	r.worker.RegisterActivityWithOptions(act, opts)
	return r
}

// Worker returns the underlying worker for running.
func (r *WorkerRegistry) Worker() worker.Worker {
	return r.worker
}

// Run starts the worker and blocks until interrupted.
func (r *WorkerRegistry) Run(interruptCh <-chan interface{}) error {
	return r.worker.Run(interruptCh)
}
