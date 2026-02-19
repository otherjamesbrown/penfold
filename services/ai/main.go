// Package main provides the entry point for the AI Coordinator service.
// This service coordinates AI/ML operations including embeddings, summarization,
// assertion extraction, and content classification.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/buildinfo"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/pkg/models"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/services/ai/config"
	"github.com/otherjamesbrown/penfold/services/ai/server"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

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

	logger.Info("Starting AI Coordinator service",
		logging.F("version", buildinfo.Version),
		logging.F("commit", buildinfo.Commit),
		logging.F("build_time", buildinfo.BuildTime),
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("environment", cfg.Environment),
		logging.F("db_configured", cfg.DBURL != ""),
	)

	// Bind gRPC port FIRST - fail fast if port is unavailable
	// This prevents expensive cloud API initialization when the port is already in use
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr())
	if err != nil {
		logger.Error("Failed to create gRPC listener", logging.Err(err))
		os.Exit(1)
	}
	logger.Debug("gRPC port bound successfully", logging.F("address", cfg.GRPCAddr()))

	// Optionally initialize database connection for dynamic model config
	var dbPool *pgxpool.Pool
	var modelRepo *models.Repository
	if cfg.DBURL != "" {
		logger.Info("Initializing database connection for model config",
			logging.F("db_url", "<redacted>"),
		)

		poolConfig, err := pgxpool.ParseConfig(cfg.DBURL)
		if err != nil {
			logger.Error("Failed to parse database URL", logging.Err(err))
			os.Exit(1)
		}

		// Configure connection pool
		poolConfig.MaxConns = 5 // Small pool for config queries only
		poolConfig.MinConns = 1
		poolConfig.MaxConnLifetime = 1 * time.Hour
		poolConfig.MaxConnIdleTime = 10 * time.Minute

		dbPool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err != nil {
			logger.Error("Failed to create database pool", logging.Err(err))
			os.Exit(1)
		}

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := dbPool.Ping(ctx); err != nil {
			logger.Error("Failed to ping database", logging.Err(err))
			dbPool.Close()
			os.Exit(1)
		}

		modelRepo = models.NewRepository(dbPool, logger)
		logger.Info("Database connection established for model config")

		// Note: DBConfigResolver requires a tenant ID for config resolution.
		// In a multi-tenant system, this would come from request context.
		// For single-tenant deployment or global config, a fixed tenant ID is used.
		// The resolver is created but not yet wired to the server (future enhancement).
		_ = config.NewDBConfigResolver(modelRepo, cfg, uuid.Nil) // Placeholder for future server integration
	} else {
		logger.Info("Database not configured, using env var model config only")
	}

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

	// Initialize metrics
	svcMetrics := metrics.NewMetrics(cfg.ServiceName, "penfold")
	if err := svcMetrics.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Register gRPC health check
	healthChecker.RegisterCheck("grpc_server", func(ctx context.Context) error {
		// Basic check - the server is healthy if this runs
		return nil
	}, health.Critical())

	// Create MLX backend (used for embeddings)
	mlxBackend := backend.NewMLXBackend(&backend.MLXConfig{
		EmbeddingsURL:         cfg.MLXEmbeddingsURL,
		LLMURL:                cfg.MLXLLMURL,
		DefaultEmbeddingModel: cfg.DefaultEmbeddingModel,
		DefaultLLMModel:       cfg.DefaultLLMModel,
		EmbeddingDimensions:   cfg.EmbeddingDimensions,
		Timeout:               10 * time.Minute,
	})

	logger.Info("MLX backend configured (embeddings)",
		logging.F("embeddings_url", cfg.MLXEmbeddingsURL),
		logging.F("default_embedding_model", cfg.DefaultEmbeddingModel),
	)

	// Create Gemini backend (used for LLM chat/completion)
	geminiConfig := backend.DefaultGeminiConfig()
	if cfg.GeminiAPIKey != "" {
		geminiConfig.APIKey = cfg.GeminiAPIKey
	}
	if cfg.GeminiDefaultModel != "" {
		geminiConfig.DefaultLLMModel = cfg.GeminiDefaultModel
	}
	geminiBackend, err := backend.NewGeminiBackend(geminiConfig)
	if err != nil {
		logger.Error("Failed to create Gemini backend", logging.Err(err))
		os.Exit(1)
	}

	logger.Info("Gemini backend configured",
		logging.F("default_model", geminiConfig.DefaultLLMModel),
	)

	logger.Info("Ollama backend configured (LLM)",
		logging.F("url", cfg.MLXLLMURL),
		logging.F("default_model", cfg.DefaultLLMModel),
	)

	// Create composite backend: MLX for embeddings, MLX/Ollama for local LLM, Gemini for gemini-* models
	compositeBackend := backend.NewCompositeBackend(mlxBackend, mlxBackend, geminiBackend)
	defer func() { _ = compositeBackend.Close() }()

	// Register MLX embeddings health check (non-critical)
	healthChecker.RegisterCheck("mlx_embeddings", func(ctx context.Context) error {
		return mlxBackend.CheckEmbeddingsHealth(ctx)
	})

	// Register Gemini LLM health check (non-critical)
	healthChecker.RegisterCheck("gemini_llm", func(ctx context.Context) error {
		return geminiBackend.CheckLLMHealth(ctx)
	})

	// Create gRPC server.
	// The otelgrpc stats handler extracts traceparent from incoming gRPC metadata,
	// so the ai-coordinator's context inherits the worker's active span.
	// This allows applyPipelineTrace in pkg/tracing/ai.go to detect an active span
	// and use it as the parent instead of falling through to the manual trace ID path.
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(svcMetrics),
		),
	)

	// Register AI service
	aiServer := server.NewAIServer(cfg, logger, compositeBackend)
	aiv1.RegisterAICoordinatorServiceServer(grpcServer, aiServer)

	// Enable gRPC reflection for debugging
	if cfg.IsDevelopment() {
		reflection.Register(grpcServer)
		logger.Debug("gRPC reflection enabled")
	}

	// Start gRPC server (listener already bound earlier to fail fast)
	go func() {
		logger.Info("gRPC server listening", logging.F("address", cfg.GRPCAddr()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("gRPC server error", logging.Err(err))
		}
	}()

	// Start HTTP server for health and metrics
	httpMux := http.NewServeMux()
	httpMux.Handle("/health", healthChecker.Handler())
	httpMux.Handle("/ready", healthChecker.ReadyHandler())
	httpMux.Handle("/live", healthChecker.LiveHandler())
	httpMux.Handle("/metrics", metrics.Handler())
	httpMux.HandleFunc("/version", buildinfo.Handler("penfold-ai-coordinator"))

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

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received shutdown signal", logging.F("signal", sig.String()))

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.GracefulShutdownTimeout)*time.Second,
	)
	defer cancel()

	// Stop accepting new gRPC connections
	grpcServer.GracefulStop()
	logger.Info("gRPC server stopped")

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

	// Shutdown database pool
	if dbPool != nil {
		dbPool.Close()
		logger.Info("Database pool closed")
	}

	logger.Info("AI Coordinator service shutdown complete")
}

// loggingInterceptor creates a gRPC unary interceptor for request logging.
func loggingInterceptor(logger logging.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Call the handler
		resp, err := handler(ctx, req)

		// Log the request
		duration := time.Since(start)
		fields := []logging.Field{
			logging.F("method", info.FullMethod),
			logging.F("duration", duration),
		}

		if err != nil {
			logger.Error("gRPC request failed", append(fields, logging.Err(err))...)
		} else {
			logger.Debug("gRPC request completed", fields...)
		}

		return resp, err
	}
}

// metricsInterceptor creates a gRPC unary interceptor for metrics collection.
func metricsInterceptor(m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Call the handler
		resp, err := handler(ctx, req)

		// Record metrics
		duration := time.Since(start).Seconds()
		status := "ok"
		if err != nil {
			status = "error"
			m.RecordError("grpc_request")
		}

		m.RecordRequest("grpc", info.FullMethod, status, duration)

		return resp, err
	}
}
