// Package main is the entry point for the API Gateway service.
// It sets up and runs the gRPC server with health checks, metrics, and graceful shutdown.
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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	glossaryv1 "github.com/otherjamesbrown/penfold/api/proto/glossary/v1"
	mentionsv1 "github.com/otherjamesbrown/penfold/api/proto/mentions/v1"
	questionsv1 "github.com/otherjamesbrown/penfold/api/proto/questions/v1"
	"github.com/otherjamesbrown/penfold/pkg/auth"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/pkg/products"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
	"github.com/otherjamesbrown/penfold/pkg/sources"
	"github.com/otherjamesbrown/penfold/services/gateway/config"
	"github.com/otherjamesbrown/penfold/services/gateway/entityservice"
	"github.com/otherjamesbrown/penfold/services/gateway/glossaryservice"
	gatewayhealth "github.com/otherjamesbrown/penfold/services/gateway/health"
	"github.com/otherjamesbrown/penfold/services/gateway/mentionsservice"
	"github.com/otherjamesbrown/penfold/services/gateway/middleware"
	"github.com/otherjamesbrown/penfold/services/gateway/questionsservice"
	"github.com/otherjamesbrown/penfold/services/gateway/server"
)

func main() {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger.
	logCfg := &logging.Config{
		Level:       logging.Level(cfg.Base.LogLevel),
		ServiceName: cfg.Base.ServiceName,
		Environment: string(cfg.Base.Environment),
		JSONFormat:  cfg.Base.IsProduction(),
	}
	logger := logging.NewLogger(logCfg)
	logging.SetGlobal(logger)

	logger.Info("Starting API Gateway service",
		logging.F("version", server.Version),
		logging.F("environment", cfg.Base.Environment),
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
	)

	// Initialize database connection pool.
	dbPool, err := pgxpool.New(context.Background(), cfg.Base.Database.DSN())
	if err != nil {
		logger.Error("Failed to connect to database", logging.Err(err))
		os.Exit(1)
	}
	defer dbPool.Close()

	// Verify database connection.
	if err := dbPool.Ping(context.Background()); err != nil {
		logger.Error("Failed to ping database", logging.Err(err))
		os.Exit(1)
	}
	logger.Info("Connected to database",
		logging.F("host", cfg.Base.Database.Host),
		logging.F("database", cfg.Base.Database.Name),
	)

	// Initialize metrics.
	m := metrics.NewMetrics(cfg.Base.ServiceName, "penfold")
	if err := m.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize health aggregator for backend services.
	healthAggregator := gatewayhealth.NewAggregator(server.Version)
	healthAggregator.SetDefaultTimeout(5 * time.Second)

	// Register database health check.
	healthAggregator.RegisterService(gatewayhealth.ServiceConfig{
		Name:     "database",
		Client:   gatewayhealth.NewDatabaseHealthClient(dbPool.Ping),
		Critical: true,
		Timeout:  3 * time.Second,
	})

	// Register ML service health checks if URLs are configured.
	if cfg.EmbeddingsURL != "" {
		healthAggregator.RegisterService(gatewayhealth.ServiceConfig{
			Name:     "embeddings",
			Client:   gatewayhealth.NewHTTPHealthClient(cfg.EmbeddingsURL, "/health", 5*time.Second),
			Critical: false, // ML services are not critical for gateway operation
			Timeout:  5 * time.Second,
		})
		logger.Info("Registered embeddings health check", logging.F("url", cfg.EmbeddingsURL))
	}

	if cfg.LLMURL != "" {
		healthAggregator.RegisterService(gatewayhealth.ServiceConfig{
			Name:     "llm",
			Client:   gatewayhealth.NewHTTPHealthClient(cfg.LLMURL, "/v1/models", 5*time.Second),
			Critical: false,
			Timeout:  5 * time.Second,
		})
		logger.Info("Registered LLM health check", logging.F("url", cfg.LLMURL))
	}

	if cfg.WorkerHealthURL != "" {
		healthAggregator.RegisterService(gatewayhealth.ServiceConfig{
			Name:     "worker",
			Client:   gatewayhealth.NewHTTPHealthClient(cfg.WorkerHealthURL, "/health", 5*time.Second),
			Critical: false,
			Timeout:  5 * time.Second,
		})
		logger.Info("Registered worker health check", logging.F("url", cfg.WorkerHealthURL))
	}

	// Create the gateway server with health aggregator.
	gatewayServer := server.NewGatewayServer(cfg, logger, m, healthAggregator)

	// Build gRPC server options with interceptors.
	grpcOpts := buildGRPCServerOptions(cfg, logger, m)

	// Create gRPC server with options.
	grpcServer := grpc.NewServer(grpcOpts...)

	// Register gRPC reflection for development/debugging.
	if cfg.Base.IsDevelopment() {
		reflection.Register(grpcServer)
		logger.Debug("gRPC reflection enabled")
	}

	// Register gRPC services.
	_ = gatewayServer // Gateway server for future use

	// Register GlossaryService.
	glossaryRepo := glossary.NewRepository(dbPool)
	glossarySvc := glossaryservice.NewService(glossaryRepo, logger)
	glossaryv1.RegisterGlossaryServiceServer(grpcServer, glossarySvc)
	logger.Info("Registered GlossaryService")

	// Register QuestionsService.
	questionsRepo := reviewqueue.NewRepository(dbPool)
	sourcesRepo := sources.NewRepository(dbPool)
	questionsSvc := questionsservice.NewService(questionsRepo, glossaryRepo, sourcesRepo, logger)
	questionsv1.RegisterQuestionsServiceServer(grpcServer, questionsSvc)
	logger.Info("Registered QuestionsService")

	// Register MentionsService.
	mentionsRepo := mentions.NewPostgresRepository(dbPool)
	mentionsSvc := mentionsservice.NewService(mentionsRepo, logger)
	mentionsv1.RegisterMentionsServiceServer(grpcServer, mentionsSvc)
	logger.Info("Registered MentionsService")

	// Register EntityService for bulk entity seeding.
	entityRepo := entities.NewRepository(dbPool, logger)
	productRepo := products.NewRepository(dbPool, logger)
	entitySvc := entityservice.NewService(entityRepo, productRepo, logger)
	entityv1.RegisterEntityServiceServer(grpcServer, entitySvc)
	logger.Info("Registered EntityService")

	// Start HTTP server for health checks and metrics.
	httpMux := http.NewServeMux()
	httpMux.Handle("/health", healthAggregator.Handler())
	httpMux.Handle("/ready", healthAggregator.ReadyHandler())
	httpMux.Handle("/live", healthAggregator.LiveHandler())
	httpMux.Handle("/metrics", promhttp.Handler())

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddress(),
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Create error channel for server errors.
	errChan := make(chan error, 2)

	// Start HTTP server in goroutine.
	go func() {
		logger.Info("Starting HTTP server",
			logging.F("address", cfg.HTTPAddress()),
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start gRPC server in goroutine.
	go func() {
		lis, err := net.Listen("tcp", cfg.GRPCAddress())
		if err != nil {
			errChan <- fmt.Errorf("failed to listen on %s: %w", cfg.GRPCAddress(), err)
			return
		}

		logger.Info("Starting gRPC server",
			logging.F("address", cfg.GRPCAddress()),
		)
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal or error.
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal",
			logging.F("signal", sig.String()),
		)
	case err := <-errChan:
		logger.Error("Server error", logging.Err(err))
	}

	// Initiate graceful shutdown.
	logger.Info("Initiating graceful shutdown...")

	// Create shutdown context with timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server.
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	} else {
		logger.Info("HTTP server stopped gracefully")
	}

	// Stop gRPC server gracefully.
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info("gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("gRPC shutdown timeout, forcing stop")
		grpcServer.Stop()
	}

	logger.Info("Gateway service shutdown complete",
		logging.F("uptime", gatewayServer.Uptime().String()),
	)
}

// loggingInterceptor creates a gRPC unary interceptor for logging and metrics.
func loggingInterceptor(logger logging.Logger, m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Call the handler.
		resp, err := handler(ctx, req)

		// Calculate duration.
		duration := time.Since(start)

		// Determine status for metrics.
		statusCode := "OK"
		if err != nil {
			statusCode = "ERROR"
		}

		// Record metrics.
		m.RecordRequest("GRPC", info.FullMethod, statusCode, duration.Seconds())

		// Log the request.
		if err != nil {
			logger.Error("gRPC request failed",
				logging.F("method", info.FullMethod),
				logging.F("duration_ms", duration.Milliseconds()),
				logging.Err(err),
			)
		} else {
			logger.Debug("gRPC request completed",
				logging.F("method", info.FullMethod),
				logging.F("duration_ms", duration.Milliseconds()),
			)
		}

		return resp, err
	}
}

// buildGRPCServerOptions constructs gRPC server options including interceptors.
// When auth is enabled, it chains auth middleware with logging/metrics interceptors.
func buildGRPCServerOptions(cfg *config.GatewayConfig, logger logging.Logger, m *metrics.Metrics) []grpc.ServerOption {
	// Start with logging interceptor
	loggingInt := loggingInterceptor(logger, m)

	// If auth is not enabled, just use logging interceptor
	if !cfg.AuthEnabled {
		logger.Info("Authentication middleware disabled")
		return []grpc.ServerOption{
			grpc.UnaryInterceptor(loggingInt),
		}
	}

	// Build auth middleware configuration
	authCfg := &middleware.AuthConfig{
		Logger:        logger,
		RequireTenant: cfg.Auth.RequireTenant,
		SkipMethods:   cfg.Auth.SkipAuthMethods,
	}

	// Add default skip methods for health checks
	defaultSkipMethods := []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
	}
	authCfg.SkipMethods = append(authCfg.SkipMethods, defaultSkipMethods...)

	// Configure JWT validator if secret key is provided
	if cfg.Auth.JWTSecretKey != "" {
		var jwtOpts []auth.JWTValidatorOption
		if cfg.Auth.JWTIssuer != "" {
			jwtOpts = append(jwtOpts, auth.WithIssuer(cfg.Auth.JWTIssuer))
		}
		authCfg.JWTValidator = auth.NewJWTValidator(cfg.Auth.JWTSecretKey, jwtOpts...)
		logger.Info("JWT authentication enabled",
			logging.F("issuer", cfg.Auth.JWTIssuer),
		)
	}

	// Configure API key validator
	// In production, API keys would be loaded from a secure store.
	// For now, we create an empty validator that can be populated at runtime.
	authCfg.APIKeyValidator = auth.NewAPIKeyValidator()
	logger.Info("API key authentication enabled")

	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(authCfg)

	// Chain interceptors: logging runs first, then auth
	// This ensures all requests are logged, even failed auth attempts
	chainedInterceptor := middleware.ChainUnaryInterceptors(
		loggingInt,
		authMiddleware.UnaryInterceptor(),
	)

	logger.Info("Authentication middleware enabled",
		logging.F("require_tenant", cfg.Auth.RequireTenant),
		logging.F("skip_methods", authCfg.SkipMethods),
	)

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(chainedInterceptor),
		grpc.StreamInterceptor(authMiddleware.StreamInterceptor()),
	}
}
