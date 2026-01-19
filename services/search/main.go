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

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/search/config"
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

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Register health checks
	registerHealthChecks(healthChecker, cfg, logger)

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(m),
		),
	)

	// Register search service
	searchServer := server.NewSearchServer(cfg, logger, m)
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
func registerHealthChecks(checker *health.Checker, cfg *config.Config, logger logging.Logger) {
	// Basic liveness check - always passes
	checker.RegisterCheck("liveness", func(ctx context.Context) error {
		return nil
	})

	// Vector database connectivity check (placeholder)
	checker.RegisterCheck("vectordb", func(ctx context.Context) error {
		// TODO: Implement actual vector database connectivity check
		// For now, return nil (healthy) to indicate the service is operational
		return nil
	}, health.Critical())

	// PostgreSQL connectivity check (placeholder)
	checker.RegisterCheck("postgres", func(ctx context.Context) error {
		// TODO: Implement actual PostgreSQL connectivity check
		// For now, return nil (healthy) to indicate the service is operational
		return nil
	}, health.Critical())

	logger.Debug("Health checks registered")
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
