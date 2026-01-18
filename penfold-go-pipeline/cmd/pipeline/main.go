// Package main provides the entry point for the penfold-go-pipeline service.
// This process acts as a bridge between Redis events and Temporal workflows.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"go.temporal.io/sdk/client"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/events"
	"github.com/otherjamesbrown/penfold-go-pipeline/internal/health"
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
		Str("service", "penfold-go-pipeline").
		Str("version", version).
		Logger()

	// Set log level based on environment
	if os.Getenv("LOG_LEVEL") == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	logger.Info().Msg("Starting Penfold Go Pipeline")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load configuration")
	}

	logger.Info().
		Str("db_host", cfg.Database.Host).
		Str("redis_host", cfg.Redis.Host).
		Str("temporal_host", cfg.Temporal.HostPort).
		Str("task_queue", cfg.Temporal.TaskQueue).
		Int("worker_count", cfg.Processing.WorkerCount).
		Msg("Configuration loaded")

	// Initialize storage
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

	// Initialize Temporal client
	temporalClient, err := temporal.NewClient(cfg.Temporal, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create Temporal client")
	}
	defer temporalClient.Close()

	logger.Info().
		Str("host_port", cfg.Temporal.HostPort).
		Str("namespace", cfg.Temporal.Namespace).
		Msg("Connected to Temporal")

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr(),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
		PoolSize:     cfg.Redis.PoolSize,
	})

	// Test Redis connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Warn().Err(err).Msg("Redis connection failed (will retry on first use)")
	} else {
		logger.Info().Msg("Connected to Redis")
	}
	defer redisClient.Close()

	// Initialize event router with handlers
	router := events.NewRouter(logger)

	// Register the email handler that starts a Temporal workflow
	router.RegisterManualEmailHandler(func(ctx context.Context, event *events.ManualEmailIngestedEvent) error {
		logger.Info().
			Int64("source_id", event.SourceID).
			Str("tenant_id", event.TenantID).
			Str("message_id", event.MessageID).
			Msg("Starting workflow for email")

		// Build workflow input from the event
		input := workflows.EmailProcessingInput{
			TenantID:    event.TenantID,
			SourceID:    event.SourceID,
			MessageID:   event.MessageID,
			FromEmail:   event.FromEmail,
			FromName:    event.FromName,
			Subject:     event.Subject,
			ToEmails:    event.ToEmails,
			CcEmails:    event.CcEmails,
			EmailDate:   event.EmailDate,
			ContentHash: event.ContentHash,
			JobID:       event.JobID,
		}

		// Start the workflow
		workflowOptions := client.StartWorkflowOptions{
			ID:        fmt.Sprintf("email-processing-%d", event.SourceID),
			TaskQueue: cfg.Temporal.TaskQueue,
		}

		we, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.EmailProcessingWorkflow, input)
		if err != nil {
			logger.Error().Err(err).
				Int64("source_id", event.SourceID).
				Msg("Failed to start workflow")
			return err
		}

		logger.Info().
			Str("workflow_id", we.GetID()).
			Str("run_id", we.GetRunID()).
			Int64("source_id", event.SourceID).
			Msg("Workflow started successfully")

		// Don't wait for completion - workflow runs async
		return nil
	})

	// Initialize Redis subscriber
	subscriberConfig := events.SubscriberConfig{
		Channels: []string{
			"events.manual_email.ingested",
			"events.content.ingested",
		},
		ReconnectDelay:    time.Second,
		MaxReconnectDelay: 30 * time.Second,
		ReconnectBackoff:  2.0,
		WorkerCount:       cfg.Processing.WorkerCount,
	}
	subscriber := events.NewSubscriber(redisClient, subscriberConfig, router.Route, logger)

	// Initialize health server
	healthServer := health.NewServer(&cfg.Server, logger, version)
	healthServer.AddChecker(health.NewRedisChecker(redisClient))
	healthServer.AddChecker(health.NewDBChecker(db.Health))
	// Note: Temporal health is checked by the worker; this process just starts workflows

	// Start health server
	go func() {
		if err := healthServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("Health server error")
		}
	}()

	logger.Info().Int("port", cfg.Server.HealthPort).Msg("Health server started")

	// Start event subscriber
	if err := subscriber.Start(context.Background()); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start event subscriber")
	}

	logger.Info().Strs("channels", subscriberConfig.Channels).Msg("Event subscriber started")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop subscriber
	if err := subscriber.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error stopping subscriber")
	}

	// Stop health server
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error stopping health server")
	}

	logger.Info().Msg("Penfold Go Pipeline shutdown complete")
}
