// Package main provides the entry point for the Penfold Temporal worker.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
	"go.temporal.io/sdk/worker"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/activities"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/clients"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/temporal"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/workflows"
)

const version = "1.0.0"

func main() {
	// Initialize zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := zerolog.New(os.Stdout).With().
		Timestamp().
		Str("service", "penfold-worker").
		Str("version", version).
		Logger()

	// Set log level based on environment
	if os.Getenv("LOG_LEVEL") == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	logger.Info().Msg("Starting Penfold Temporal Worker")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load configuration")
	}

	logger.Info().
		Str("temporal_host", cfg.Temporal.HostPort).
		Str("temporal_namespace", cfg.Temporal.Namespace).
		Str("task_queue", cfg.Temporal.TaskQueue).
		Int("max_concurrent_activities", cfg.Temporal.MaxConcurrentActivities).
		Msg("Configuration loaded")

	// Create Temporal client
	temporalClient, err := temporal.NewClient(cfg.Temporal, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create Temporal client")
	}
	defer temporalClient.Close()

	// Initialize database
	dbConfig := &storage.Config{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		Database:        cfg.Database.Name,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		MaxConns:        int32(cfg.Database.MaxOpenConns),
		MinConns:        int32(cfg.Database.MaxIdleConns),
		MaxConnLifetime: cfg.Database.ConnMaxLifetime,
		MaxConnIdleTime: cfg.Database.ConnMaxIdleTime,
		RetryAttempts:   5,
		RetryDelay:      2 * time.Second,
	}

	db, err := storage.NewDB(context.Background(), dbConfig, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	defer db.Close()

	logger.Info().Msg("Connected to PostgreSQL")

	// Get connection pool for repositories
	pool := db.Pool()

	// Initialize repositories
	sourceRepo := storage.NewSourceRepository(pool, logger)
	embeddingRepo := storage.NewEmbeddingRepository(pool, logger)
	resultRepo := storage.NewProcessingResultRepository(pool, logger)

	// Initialize AI clients
	embeddingsConfig := clients.EmbeddingsConfig{
		BaseURL:    cfg.AI.EmbeddingsURL,
		Model:      cfg.AI.EmbeddingsModel,
		Timeout:    cfg.AI.EmbeddingsTimeout,
		MaxRetries: cfg.Processing.MaxRetries,
		CircuitBreakerSettings: gobreaker.Settings{
			Name:        "embeddings-client",
			MaxRequests: cfg.Processing.CircuitBreakerMaxRequests,
			Interval:    10 * time.Second,
			Timeout:     cfg.Processing.CircuitBreakerTimeout,
		},
	}
	embeddingsClient := clients.NewEmbeddingsClient(embeddingsConfig, logger)

	llmConfig := clients.LLMConfig{
		BaseURL:            cfg.AI.LLMURL,
		Model:              cfg.AI.LLMModel,
		DefaultTemperature: 0.7,
		DefaultMaxTokens:   1024,
		Timeout:            cfg.AI.LLMTimeout,
		MaxRetries:         cfg.Processing.MaxRetries,
		CircuitBreakerSettings: gobreaker.Settings{
			Name:        "llm-client",
			MaxRequests: cfg.Processing.CircuitBreakerMaxRequests,
			Interval:    10 * time.Second,
			Timeout:     cfg.Processing.CircuitBreakerTimeout,
		},
	}
	llmClient := clients.NewLLMClient(llmConfig, logger)

	// Check AI service health
	checkAIServices(context.Background(), embeddingsClient, llmClient, logger)

	// Create activities instance with all dependencies
	acts := activities.NewActivities(
		sourceRepo,
		embeddingRepo,
		resultRepo,
		embeddingsClient,
		llmClient,
		logger,
		&cfg.AI,
	)

	// Create Temporal worker
	w := worker.New(temporalClient, cfg.Temporal.TaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.Temporal.MaxConcurrentActivities,
		MaxConcurrentWorkflowTaskExecutionSize: 10,
	})

	// Register workflows
	w.RegisterWorkflow(workflows.EmailProcessingWorkflow)

	// Register activities with the struct instance
	// Temporal SDK uses method names by default
	w.RegisterActivity(acts)

	logger.Info().
		Str("task_queue", cfg.Temporal.TaskQueue).
		Msg("Temporal worker configured")

	// Start worker in a goroutine
	go func() {
		logger.Info().Msg("Starting Temporal worker")
		if err := w.Run(worker.InterruptCh()); err != nil {
			logger.Fatal().Err(err).Msg("Worker failed")
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Worker will gracefully shutdown via InterruptCh()
	logger.Info().Msg("Penfold Temporal Worker shutdown complete")
}

// checkAIServices verifies that AI services are available.
func checkAIServices(ctx context.Context, embeddings *clients.EmbeddingsClient, llm *clients.LLMClient, logger zerolog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check embeddings service
	if err := embeddings.HealthCheck(ctx); err != nil {
		logger.Warn().Err(err).Msg("Embeddings service not available")
	} else {
		logger.Info().Msg("Embeddings service available")
	}

	// Check LLM service
	if err := llm.HealthCheck(ctx); err != nil {
		logger.Warn().Err(err).Msg("LLM service not available")
	} else {
		logger.Info().Msg("LLM service available")
	}
}
