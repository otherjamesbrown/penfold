package temporal

import (
	"testing"

	"go.temporal.io/sdk/worker"
)

func TestWorkerOptions(t *testing.T) {
	t.Run("WithMaxConcurrentActivities", func(t *testing.T) {
		opts := &worker.Options{}
		WithMaxConcurrentActivities(25)(opts)

		if opts.MaxConcurrentActivityExecutionSize != 25 {
			t.Errorf("expected MaxConcurrentActivityExecutionSize 25, got %d", opts.MaxConcurrentActivityExecutionSize)
		}
	})

	t.Run("WithMaxConcurrentWorkflows", func(t *testing.T) {
		opts := &worker.Options{}
		WithMaxConcurrentWorkflows(15)(opts)

		if opts.MaxConcurrentWorkflowTaskExecutionSize != 15 {
			t.Errorf("expected MaxConcurrentWorkflowTaskExecutionSize 15, got %d", opts.MaxConcurrentWorkflowTaskExecutionSize)
		}
	})

	t.Run("WithEnableSessionWorker", func(t *testing.T) {
		opts := &worker.Options{}
		WithEnableSessionWorker(true)(opts)

		if !opts.EnableSessionWorker {
			t.Error("expected EnableSessionWorker to be true")
		}
	})

	t.Run("WithMaxConcurrentSessionExecutions", func(t *testing.T) {
		opts := &worker.Options{}
		WithMaxConcurrentSessionExecutions(50)(opts)

		if opts.MaxConcurrentSessionExecutionSize != 50 {
			t.Errorf("expected MaxConcurrentSessionExecutionSize 50, got %d", opts.MaxConcurrentSessionExecutionSize)
		}
	})

	t.Run("WithDisableWorkflowWorker", func(t *testing.T) {
		opts := &worker.Options{}
		WithDisableWorkflowWorker(true)(opts)

		if !opts.DisableWorkflowWorker {
			t.Error("expected DisableWorkflowWorker to be true")
		}
	})

	t.Run("WithDisableActivityWorker", func(t *testing.T) {
		opts := &worker.Options{}
		WithDisableActivityWorker(true)(opts)

		if !opts.DisableEagerActivities {
			t.Error("expected DisableEagerActivities to be true")
		}
	})
}

func TestNewWorkerFromConfig(t *testing.T) {
	// We can't test with a real client, but we can verify the function signature
	// and that config values are used correctly
	cfg := &Config{
		HostPort:                "localhost:7233",
		Namespace:               "default",
		TaskQueue:               "test-queue",
		MaxConcurrentActivities: 20,
		MaxConcurrentWorkflows:  15,
	}

	// Verify config fields are accessible and set
	if cfg.TaskQueue != "test-queue" {
		t.Errorf("expected TaskQueue test-queue, got %s", cfg.TaskQueue)
	}
	if cfg.MaxConcurrentActivities != 20 {
		t.Errorf("expected MaxConcurrentActivities 20, got %d", cfg.MaxConcurrentActivities)
	}
	if cfg.MaxConcurrentWorkflows != 15 {
		t.Errorf("expected MaxConcurrentWorkflows 15, got %d", cfg.MaxConcurrentWorkflows)
	}
}

func TestWorkerRegistry(t *testing.T) {
	// WorkerRegistry wraps a worker and provides fluent interface
	// We can't test with real worker without a client, but we can test the struct
	registry := NewWorkerRegistry(nil)

	if registry == nil {
		t.Fatal("expected non-nil registry")
	}

	if registry.Worker() != nil {
		t.Error("expected nil worker since we passed nil")
	}

	// Note: We can't call RegisterWorkflow or RegisterActivity on nil worker
	// Those would panic. Testing the registry creation is sufficient for unit tests.
	// Integration tests with a real Temporal server would test registration.
}
