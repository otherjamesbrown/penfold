// Package main provides the entry point for the Penfold Temporal worker.
// This worker processes workflows and activities across multiple task queues:
//   - penfold-main: general workflows
//   - penfold-ai: AI-intensive workflows (embeddings, LLM calls)
//   - penfold-email: email processing workflows
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/worker/activities"
	"github.com/otherjamesbrown/penfold/services/worker/config"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

const version = "0.1.0"

// workerManager manages multiple Temporal workers for different task queues.
type workerManager struct {
	client  client.Client
	workers map[string]worker.Worker
	logger  logging.Logger
	mu      sync.RWMutex
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logCfg := &logging.Config{
		Level:       logging.Level(cfg.LogLevel),
		ServiceName: cfg.ServiceName,
		Environment: cfg.Environment,
		JSONFormat:  cfg.IsProduction(),
	}
	logger := logging.NewLogger(logCfg)
	logging.SetGlobal(logger)

	logger.Info("Starting Penfold Temporal worker",
		logging.F("version", version),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("temporal_host", cfg.TemporalHostPort),
		logging.F("temporal_namespace", cfg.TemporalNamespace),
		logging.F("task_queues", cfg.TaskQueues),
		logging.F("environment", cfg.Environment),
	)

	// Create zerolog logger for Temporal SDK and activities
	zerologLevel := zerolog.InfoLevel
	switch cfg.LogLevel {
	case "debug":
		zerologLevel = zerolog.DebugLevel
	case "warn":
		zerologLevel = zerolog.WarnLevel
	case "error":
		zerologLevel = zerolog.ErrorLevel
	}
	zerologger := zerolog.New(os.Stdout).
		Level(zerologLevel).
		With().
		Timestamp().
		Str("service_name", cfg.ServiceName).
		Str("environment", cfg.Environment).
		Logger()

	// Initialize metrics
	svcMetrics := metrics.NewMetrics(cfg.ServiceName, "penfold")
	if err := svcMetrics.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Create Temporal client configuration
	temporalCfg := &pkgtemporal.Config{
		HostPort:                cfg.TemporalHostPort,
		Namespace:               cfg.TemporalNamespace,
		MaxConcurrentActivities: cfg.MaxConcurrentActivities,
		MaxConcurrentWorkflows:  cfg.MaxConcurrentWorkflows,
	}

	// Create Temporal client
	temporalClient, err := pkgtemporal.NewClient(
		temporalCfg,
		pkgtemporal.WithLogger(zerologger),
	)
	if err != nil {
		logger.Error("Failed to create Temporal client", logging.Err(err))
		os.Exit(1)
	}
	defer temporalClient.Close()

	logger.Info("Connected to Temporal server",
		logging.F("host", cfg.TemporalHostPort),
		logging.F("namespace", cfg.TemporalNamespace),
	)

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Create worker manager
	wm := &workerManager{
		client:  temporalClient,
		workers: make(map[string]worker.Worker),
		logger:  logger,
	}

	// Create activity and workflow registrars
	activityImpl := activities.NewActivities(zerologger)
	activityRegistrar := activities.NewRegistrar(activityImpl)
	workflowRegistrar := workflows.NewRegistrar()

	// Create workers for each configured task queue
	for _, taskQueue := range cfg.TaskQueues {
		w := pkgtemporal.NewWorkerFromConfig(
			temporalClient,
			&pkgtemporal.Config{
				TaskQueue:               taskQueue,
				MaxConcurrentActivities: cfg.MaxConcurrentActivities,
				MaxConcurrentWorkflows:  cfg.MaxConcurrentWorkflows,
			},
			pkgtemporal.WithWorkerStopTimeout(time.Duration(cfg.GracefulShutdownTimeout)*time.Second),
		)

		// Register workflows and activities for this queue
		workflowRegistrar.RegisterAll(w, taskQueue)
		activityRegistrar.RegisterAll(w, taskQueue)

		wm.workers[taskQueue] = w

		logger.Info("Created worker for task queue",
			logging.F("task_queue", taskQueue),
			logging.F("workflow_count", workflowRegistrar.WorkflowCount(taskQueue)),
			logging.F("activity_count", activityRegistrar.ActivityCount(taskQueue)),
		)

		// Register health check for this worker
		healthChecker.RegisterCheck(
			fmt.Sprintf("worker_%s", taskQueue),
			wm.createWorkerHealthCheck(taskQueue),
			health.Critical(),
		)
	}

	// Register Temporal connection health check
	healthChecker.RegisterCheck("temporal_connection", func(ctx context.Context) error {
		// Check if we can reach Temporal by describing the namespace
		_, err := temporalClient.WorkflowService().DescribeNamespace(ctx, nil)
		if err != nil {
			return fmt.Errorf("temporal connection check failed: %w", err)
		}
		return nil
	}, health.Critical())

	// Start HTTP server for health and metrics
	httpMux := http.NewServeMux()
	httpMux.Handle("/health", healthChecker.Handler())
	httpMux.Handle("/ready", healthChecker.ReadyHandler())
	httpMux.Handle("/live", healthChecker.LiveHandler())
	httpMux.Handle("/metrics", metrics.Handler())

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddr(),
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("HTTP server listening", logging.F("address", cfg.HTTPAddr()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	// Start all workers
	errCh := make(chan error, len(wm.workers))
	for taskQueue, w := range wm.workers {
		go func(tq string, wrk worker.Worker) {
			logger.Info("Starting worker", logging.F("task_queue", tq))
			if err := wrk.Run(worker.InterruptCh()); err != nil {
				logger.Error("Worker error", logging.F("task_queue", tq), logging.Err(err))
				errCh <- fmt.Errorf("worker %s failed: %w", tq, err)
			}
		}(taskQueue, w)
	}

	// Wait for shutdown signal or worker error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", logging.F("signal", sig.String()))
	case err := <-errCh:
		logger.Error("Worker failed, initiating shutdown", logging.Err(err))
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.GracefulShutdownTimeout)*time.Second,
	)
	defer cancel()

	// Stop all workers
	var wg sync.WaitGroup
	for taskQueue, w := range wm.workers {
		wg.Add(1)
		go func(tq string, wrk worker.Worker) {
			defer wg.Done()
			logger.Info("Stopping worker", logging.F("task_queue", tq))
			wrk.Stop()
			logger.Info("Worker stopped", logging.F("task_queue", tq))
		}(taskQueue, w)
	}

	// Wait for workers to stop with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("All workers stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("Worker shutdown timed out")
	}

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	} else {
		logger.Info("HTTP server stopped")
	}

	logger.Info("Penfold Temporal worker shutdown complete")
}

// createWorkerHealthCheck creates a health check function for a specific worker.
func (wm *workerManager) createWorkerHealthCheck(taskQueue string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		wm.mu.RLock()
		defer wm.mu.RUnlock()

		w, exists := wm.workers[taskQueue]
		if !exists {
			return fmt.Errorf("worker for queue %q not found", taskQueue)
		}

		// Check if worker is running (not nil)
		if w == nil {
			return fmt.Errorf("worker for queue %q is nil", taskQueue)
		}

		// The worker is considered healthy if it exists and hasn't errored
		// More sophisticated health checks can be added here
		return nil
	}
}
