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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/mentions/resolver"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
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

	// Initialize tracing with Langfuse if configured
	var tracingShutdown tracing.ShutdownFunc
	if lfConfig := tracing.LangfuseConfigFromEnv(); lfConfig != nil && lfConfig.Host != "" {
		tracingCfg := &tracing.Config{
			ServiceName: cfg.ServiceName,
			Environment: cfg.Environment,
			SampleRate:  1.0,
			Exporter:    tracing.ExporterLangfuse,
			Langfuse:    lfConfig,
		}
		var err error
		tracingShutdown, err = tracing.InitTracer(tracingCfg)
		if err != nil {
			logger.Error("Failed to initialize Langfuse tracing", logging.Err(err))
			// Continue without tracing - not fatal
		} else {
			logger.Info("Langfuse tracing initialized",
				logging.F("host", lfConfig.Host),
			)
		}
	} else {
		logger.Info("Langfuse not configured - tracing disabled (set LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY to enable)")
	}

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

	// Initialize database pool if configured
	var dbPool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		var err error
		dbPool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("Failed to create database pool", logging.Err(err))
			os.Exit(1)
		}
		defer dbPool.Close()

		// Verify connection
		if err := dbPool.Ping(context.Background()); err != nil {
			logger.Error("Failed to connect to database", logging.Err(err))
			os.Exit(1)
		}
		logger.Info("Connected to database")
	} else {
		logger.Warn("DATABASE_URL not configured - activities requiring database will fail")
	}

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
		pkgtemporal.WithLogger(logger),
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
	var activityImpl *activities.Activities
	if dbPool != nil {
		activityImpl = activities.NewActivitiesWithDB(zerologger, dbPool, cfg.AIServiceURL)
		logger.Info("Activities initialized with database connection",
			logging.F("ai_service_url", cfg.AIServiceURL),
		)
	} else {
		activityImpl = activities.NewActivities(zerologger)
		logger.Warn("Activities initialized without database - some activities will fail")
	}
	activityRegistrar := activities.NewRegistrar(activityImpl)

	// LLM configuration (used for mentions resolution and health checks)
	llmURL := os.Getenv("LLM_URL")
	if llmURL == "" {
		llmURL = "http://localhost:8080"
	}
	llmModel := os.Getenv("LLM_MODEL")
	if llmModel == "" {
		llmModel = "mlx-community/Qwen2.5-32B-Instruct-4bit"
	}

	// Initialize mentions activities if database is available
	if dbPool != nil {
		mentionsRepo := mentions.NewPostgresRepository(dbPool)

		llmConfig := resolver.LLMConfig{
			Provider:   "vllm",
			Model:      llmModel,
			BaseURL:    llmURL,
			Timeout:    120 * time.Second,
			MaxRetries: 2,
		}

		llmProvider := resolver.NewVLLMProvider(llmConfig)
		logger.Info("LLM provider initialized",
			logging.F("url", llmURL),
			logging.F("model", llmModel),
		)

		// Create resolver with provider
		resolverConfig := resolver.DefaultConfig()
		mentionResolver := resolver.NewResolver(
			resolverConfig,
			llmProvider,
			nil, // EntityLookup - can be nil, will use default matching
			mentionsRepo,
			nil, // Tracer - can be nil for now
		)

		mentionsActivities := activities.NewMentionsActivities(
			zerologger,
			dbPool,
			mentionResolver,
			mentionsRepo,
		)
		activityRegistrar.WithMentionsActivities(mentionsActivities)
		logger.Info("Mentions activities initialized with resolver")
	}

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

	// Register database health check if database is configured
	if dbPool != nil {
		healthChecker.RegisterCheck("database", func(ctx context.Context) error {
			return dbPool.Ping(ctx)
		}, health.Critical())
	}

	// Register embeddings service health check
	healthChecker.RegisterCheck("embeddings", func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.AIServiceURL+"/health", nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("embeddings service unreachable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("embeddings service returned %d", resp.StatusCode)
		}
		return nil
	}, health.Critical())

	// Register LLM service health check
	healthChecker.RegisterCheck("llm", func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, llmURL+"/v1/models", nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("LLM service unreachable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("LLM service returned %d", resp.StatusCode)
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

	// Shutdown tracing
	if tracingShutdown != nil {
		if err := tracingShutdown(shutdownCtx); err != nil {
			logger.Error("Tracing shutdown error", logging.Err(err))
		} else {
			logger.Info("Tracing shutdown complete")
		}
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
