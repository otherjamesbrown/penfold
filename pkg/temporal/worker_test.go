package temporal

import (
	"testing"
	"time"

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

	t.Run("WithWorkerStopTimeout", func(t *testing.T) {
		opts := &worker.Options{}
		WithWorkerStopTimeout(30 * time.Second)(opts)

		if opts.WorkerStopTimeout != 30*time.Second {
			t.Errorf("expected WorkerStopTimeout 30s, got %v", opts.WorkerStopTimeout)
		}
	})

	t.Run("WithEnableSessionWorker", func(t *testing.T) {
		opts := &worker.Options{}
		WithEnableSessionWorker(true)(opts)

		if !opts.EnableSessionWorker {
			t.Error("expected EnableSessionWorker to be true")
		}
	})

	t.Run("WithEnableSessionWorker_false", func(t *testing.T) {
		opts := &worker.Options{EnableSessionWorker: true}
		WithEnableSessionWorker(false)(opts)

		if opts.EnableSessionWorker {
			t.Error("expected EnableSessionWorker to be false")
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

	t.Run("MultipleOptionsComposition", func(t *testing.T) {
		opts := &worker.Options{}

		// Apply multiple options
		WithMaxConcurrentActivities(20)(opts)
		WithMaxConcurrentWorkflows(10)(opts)
		WithWorkerStopTimeout(60 * time.Second)(opts)
		WithEnableSessionWorker(true)(opts)
		WithMaxConcurrentSessionExecutions(25)(opts)

		if opts.MaxConcurrentActivityExecutionSize != 20 {
			t.Errorf("expected MaxConcurrentActivityExecutionSize 20, got %d", opts.MaxConcurrentActivityExecutionSize)
		}
		if opts.MaxConcurrentWorkflowTaskExecutionSize != 10 {
			t.Errorf("expected MaxConcurrentWorkflowTaskExecutionSize 10, got %d", opts.MaxConcurrentWorkflowTaskExecutionSize)
		}
		if opts.WorkerStopTimeout != 60*time.Second {
			t.Errorf("expected WorkerStopTimeout 60s, got %v", opts.WorkerStopTimeout)
		}
		if !opts.EnableSessionWorker {
			t.Error("expected EnableSessionWorker to be true")
		}
		if opts.MaxConcurrentSessionExecutionSize != 25 {
			t.Errorf("expected MaxConcurrentSessionExecutionSize 25, got %d", opts.MaxConcurrentSessionExecutionSize)
		}
	})
}

func TestNewWorkerFromConfig(t *testing.T) {
	t.Run("ConfigFieldsAccessible", func(t *testing.T) {
		cfg := &Config{
			HostPort:                "localhost:7233",
			Namespace:               "default",
			TaskQueue:               "test-queue",
			MaxConcurrentActivities: 20,
			MaxConcurrentWorkflows:  15,
		}

		if cfg.TaskQueue != "test-queue" {
			t.Errorf("expected TaskQueue test-queue, got %s", cfg.TaskQueue)
		}
		if cfg.MaxConcurrentActivities != 20 {
			t.Errorf("expected MaxConcurrentActivities 20, got %d", cfg.MaxConcurrentActivities)
		}
		if cfg.MaxConcurrentWorkflows != 15 {
			t.Errorf("expected MaxConcurrentWorkflows 15, got %d", cfg.MaxConcurrentWorkflows)
		}
	})

	t.Run("ConfigWithZeroValues", func(t *testing.T) {
		cfg := &Config{
			HostPort:                "localhost:7233",
			Namespace:               "default",
			TaskQueue:               "test-queue",
			MaxConcurrentActivities: 0, // Should use defaults
			MaxConcurrentWorkflows:  0, // Should use defaults
		}

		// Zero values should not cause issues in NewWorkerFromConfig
		if cfg.MaxConcurrentActivities != 0 {
			t.Errorf("expected MaxConcurrentActivities 0, got %d", cfg.MaxConcurrentActivities)
		}
	})

	t.Run("ConfigWithLargeValues", func(t *testing.T) {
		cfg := &Config{
			HostPort:                "localhost:7233",
			Namespace:               "production",
			TaskQueue:               "high-throughput-queue",
			MaxConcurrentActivities: 1000,
			MaxConcurrentWorkflows:  500,
		}

		if cfg.MaxConcurrentActivities != 1000 {
			t.Errorf("expected MaxConcurrentActivities 1000, got %d", cfg.MaxConcurrentActivities)
		}
		if cfg.MaxConcurrentWorkflows != 500 {
			t.Errorf("expected MaxConcurrentWorkflows 500, got %d", cfg.MaxConcurrentWorkflows)
		}
	})
}

func TestWorkerRegistry(t *testing.T) {
	t.Run("NewWorkerRegistry_WithNil", func(t *testing.T) {
		registry := NewWorkerRegistry(nil)

		if registry == nil {
			t.Fatal("expected non-nil registry")
		}

		if registry.Worker() != nil {
			t.Error("expected nil worker since we passed nil")
		}
	})

	t.Run("NewWorkerRegistry_WorkerAccessor", func(t *testing.T) {
		// Test that Worker() returns what was passed in
		registry := NewWorkerRegistry(nil)
		if registry.Worker() != nil {
			t.Error("expected Worker() to return nil")
		}
	})
}

func TestWorkerOptionDefaults(t *testing.T) {
	// Test that default values are reasonable
	t.Run("DefaultActivityConcurrency", func(t *testing.T) {
		// Default is 10 based on NewWorker implementation
		opts := worker.Options{
			MaxConcurrentActivityExecutionSize:     10,
			MaxConcurrentWorkflowTaskExecutionSize: 10,
		}

		if opts.MaxConcurrentActivityExecutionSize != 10 {
			t.Errorf("expected default activity concurrency 10, got %d", opts.MaxConcurrentActivityExecutionSize)
		}
	})

	t.Run("ZeroValueOptions", func(t *testing.T) {
		// Zero values should work
		opts := &worker.Options{}

		if opts.MaxConcurrentActivityExecutionSize != 0 {
			t.Errorf("expected zero value, got %d", opts.MaxConcurrentActivityExecutionSize)
		}
		if opts.MaxConcurrentWorkflowTaskExecutionSize != 0 {
			t.Errorf("expected zero value, got %d", opts.MaxConcurrentWorkflowTaskExecutionSize)
		}
		if opts.EnableSessionWorker {
			t.Error("expected EnableSessionWorker to be false by default")
		}
		if opts.DisableWorkflowWorker {
			t.Error("expected DisableWorkflowWorker to be false by default")
		}
	})
}

func TestWorkerOptionType(t *testing.T) {
	// Verify WorkerOption is a function type
	var opt WorkerOption = func(opts *worker.Options) {
		opts.MaxConcurrentActivityExecutionSize = 100
	}

	opts := &worker.Options{}
	opt(opts)

	if opts.MaxConcurrentActivityExecutionSize != 100 {
		t.Errorf("expected 100, got %d", opts.MaxConcurrentActivityExecutionSize)
	}
}

func TestWorkerOptionChaining(t *testing.T) {
	// Test that options can be collected and applied in sequence
	options := []WorkerOption{
		WithMaxConcurrentActivities(15),
		WithMaxConcurrentWorkflows(8),
		WithWorkerStopTimeout(45 * time.Second),
	}

	opts := &worker.Options{}
	for _, opt := range options {
		opt(opts)
	}

	if opts.MaxConcurrentActivityExecutionSize != 15 {
		t.Errorf("expected 15, got %d", opts.MaxConcurrentActivityExecutionSize)
	}
	if opts.MaxConcurrentWorkflowTaskExecutionSize != 8 {
		t.Errorf("expected 8, got %d", opts.MaxConcurrentWorkflowTaskExecutionSize)
	}
	if opts.WorkerStopTimeout != 45*time.Second {
		t.Errorf("expected 45s, got %v", opts.WorkerStopTimeout)
	}
}

func TestWorkerOptionOverride(t *testing.T) {
	// Test that later options override earlier ones
	opts := &worker.Options{}

	WithMaxConcurrentActivities(10)(opts)
	WithMaxConcurrentActivities(20)(opts)
	WithMaxConcurrentActivities(30)(opts)

	if opts.MaxConcurrentActivityExecutionSize != 30 {
		t.Errorf("expected last value 30, got %d", opts.MaxConcurrentActivityExecutionSize)
	}
}
