// Package main implements the Search service entry point.
// It sets up the gRPC server with health checks, metrics, and graceful shutdown.
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
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"github.com/otherjamesbrown/penfold/pkg/embeddings"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/search/cache"
	"github.com/otherjamesbrown/penfold/services/search/config"
	"github.com/otherjamesbrown/penfold/services/search/engine"
	"github.com/otherjamesbrown/penfold/services/search/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	serviceName      = "search-service"
	metricsNamespace = "penfold_search"
	shutdownTimeout  = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	// Override service name
	cfg.Base.ServiceName = serviceName

	// Initialize logger
	logCfg := &logging.Config{
		Level:       logging.Level(cfg.Base.LogLevel),
		ServiceName: serviceName,
		Environment: cfg.Base.Environment.String(),
		JSONFormat:  cfg.Base.IsProduction(),
	}
	logger := logging.NewLogger(logCfg)
	logging.SetGlobal(logger)

	logger.Info("Starting search service",
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("environment", cfg.Base.Environment),
	)

	// Initialize metrics
	m := metrics.NewMetrics(serviceName, metricsNamespace)
	if err := m.RegisterMetrics(); err != nil {
		return fmt.Errorf("registering metrics: %w", err)
	}

	// Initialize database connection pool
	var dbPool *pgxpool.Pool
	if cfg.Base.Database.Host != "" {
		var err error
		dbPool, err = pgxpool.New(context.Background(), cfg.Base.Database.DSN())
		if err != nil {
			logger.Warn("Failed to create database pool - search will be unavailable",
				logging.Err(err),
			)
		} else {
			defer dbPool.Close()
			if err := dbPool.Ping(context.Background()); err != nil {
				logger.Warn("Failed to connect to database - search will be unavailable",
					logging.Err(err),
				)
				dbPool = nil
			} else {
				logger.Info("Connected to database")
			}
		}
	} else {
		logger.Warn("Database not configured - search will be unavailable")
	}

	// Initialize embedding client
	var embeddingClient embeddings.Client
	embeddingCfg := embeddings.LoadFromEnv()
	// Override with search service config if set
	if cfg.Embedding.Model != "" {
		embeddingCfg.Model = cfg.Embedding.Model
	}
	if cfg.Embedding.Dimensions > 0 {
		embeddingCfg.Dimensions = cfg.Embedding.Dimensions
	}
	embeddingClient, err = embeddings.NewMLXClient(embeddingCfg, logger, nil)
	if err != nil {
		logger.Warn("Failed to create embedding client - semantic search will be unavailable",
			logging.Err(err),
		)
		embeddingClient = nil
	} else {
		logger.Info("Embedding client initialized",
			logging.F("url", embeddingCfg.ServerURL),
			logging.F("model", embeddingCfg.Model),
			logging.F("dimensions", embeddingCfg.Dimensions),
		)
	}

	// Initialize search cache
	searchCache := cache.New()
	defer searchCache.Close()

	// Initialize search engines
	var bm25Engine *engine.BM25Engine
	var vectorEngine *engine.VectorEngine
	var rrfEngine *engine.RRFEngine

	if dbPool != nil {
		// Initialize BM25 engine
		bm25Engine = engine.NewBM25Engine(dbPool, logger, nil)
		logger.Info("BM25 search engine initialized")

		// Initialize vector engine if embedding client is available
		if embeddingClient != nil {
			// Cast embeddings.Client to engine.EmbeddingClient (they have compatible interfaces)
			vectorEngine = engine.NewVectorEngine(dbPool, &embeddingAdapter{embeddingClient}, nil, logger)
			logger.Info("Vector search engine initialized")
		}

		// Initialize RRF hybrid engine if both engines are available
		if bm25Engine != nil && vectorEngine != nil {
			rrfCfg := engine.DefaultRRFConfig()
			rrfCfg.BM25Weight = cfg.Search.DefaultTextWeight
			rrfCfg.VectorWeight = cfg.Search.DefaultVectorWeight
			rrfEngine = engine.NewRRFEngine(bm25Engine, vectorEngine, rrfCfg, logger)
			logger.Info("Hybrid search engine initialized",
				logging.F("bm25_weight", cfg.Search.DefaultTextWeight),
				logging.F("vector_weight", cfg.Search.DefaultVectorWeight),
			)
		}
	}

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Register health checks
	registerHealthChecks(healthChecker, cfg, logger, dbPool)

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(m),
		),
	)

	// Build server options
	serverOpts := []server.ServerOption{}
	if dbPool != nil {
		serverOpts = append(serverOpts, server.WithDB(dbPool))
	}
	if embeddingClient != nil {
		serverOpts = append(serverOpts, server.WithEmbeddingClient(embeddingClient))
	}
	if bm25Engine != nil {
		serverOpts = append(serverOpts, server.WithBM25Engine(bm25Engine))
	}
	if vectorEngine != nil {
		serverOpts = append(serverOpts, server.WithVectorEngine(vectorEngine))
	}
	if rrfEngine != nil {
		serverOpts = append(serverOpts, server.WithRRFEngine(rrfEngine))
	}
	serverOpts = append(serverOpts, server.WithCache(searchCache))

	// Register search service
	searchServer := server.NewSearchServer(cfg, logger, m, serverOpts...)
	searchv1.RegisterSearchServiceServer(grpcServer, searchServer)

	// Enable reflection for debugging
	if cfg.Base.IsDevelopment() {
		reflection.Register(grpcServer)
		logger.Info("gRPC reflection enabled for development")
	}

	// Start HTTP server for health and metrics
	httpServer := startHTTPServer(cfg, healthChecker, logger)

	// Start gRPC server
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		return fmt.Errorf("creating gRPC listener: %w", err)
	}

	// Channel to receive server errors
	errChan := make(chan error, 1)

	go func() {
		logger.Info("gRPC server starting", logging.F("address", cfg.GRPCAddress()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", logging.F("signal", sig.String()))
	case err := <-errChan:
		return err
	}

	// Graceful shutdown
	return shutdown(grpcServer, httpServer, logger)
}

// registerHealthChecks adds health checks to the checker.
func registerHealthChecks(checker *health.Checker, cfg *config.Config, logger logging.Logger, dbPool *pgxpool.Pool) {
	// Basic liveness check - always passes
	checker.RegisterCheck("liveness", func(ctx context.Context) error {
		return nil
	})

	// PostgreSQL connectivity check
	checker.RegisterCheck("postgres", func(ctx context.Context) error {
		if dbPool == nil {
			return fmt.Errorf("database not configured")
		}
		return dbPool.Ping(ctx)
	}, health.Critical())

	logger.Debug("Health checks registered")
}

// embeddingAdapter adapts embeddings.Client to engine.EmbeddingClient.
type embeddingAdapter struct {
	client embeddings.Client
}

func (a *embeddingAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	return a.client.Embed(ctx, text)
}

func (a *embeddingAdapter) Dimensions() int {
	return a.client.Dimensions()
}

// startHTTPServer creates and starts the HTTP server for health and metrics endpoints.
func startHTTPServer(cfg *config.Config, checker *health.Checker, logger logging.Logger) *http.Server {
	mux := http.NewServeMux()

	// Health endpoints
	mux.Handle("/health", checker.Handler())
	mux.Handle("/ready", checker.ReadyHandler())
	mux.Handle("/live", checker.LiveHandler())

	// Metrics endpoint
	mux.Handle("/metrics", metrics.Handler())

	server := &http.Server{
		Addr:         cfg.HTTPAddress(),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("HTTP server starting", logging.F("address", cfg.HTTPAddress()))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	return server
}

// shutdown performs graceful shutdown of all servers.
func shutdown(grpcServer *grpc.Server, httpServer *http.Server, logger logging.Logger) error {
	logger.Info("Starting graceful shutdown", logging.F("timeout", shutdownTimeout.String()))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Stop accepting new gRPC requests and wait for existing ones to complete
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	// Wait for gRPC server to stop or timeout
	select {
	case <-stopped:
		logger.Info("gRPC server stopped gracefully")
	case <-ctx.Done():
		logger.Warn("gRPC shutdown timed out, forcing stop")
		grpcServer.Stop()
	}

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
		return fmt.Errorf("HTTP server shutdown: %w", err)
	}
	logger.Info("HTTP server stopped gracefully")

	logger.Info("Shutdown complete")
	return nil
}

// loggingInterceptor creates a gRPC interceptor for request logging.
func loggingInterceptor(logger logging.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
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

// metricsInterceptor creates a gRPC interceptor for metrics collection.
func metricsInterceptor(m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start).Seconds()
		status := "OK"
		if err != nil {
			status = "ERROR"
			m.RecordError("grpc_error")
		}

		m.RecordRequest("gRPC", info.FullMethod, status, duration)

		return resp, err
	}
}
