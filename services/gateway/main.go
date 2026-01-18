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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/gateway/config"
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

	// Initialize metrics.
	m := metrics.NewMetrics(cfg.Base.ServiceName, "penfold")
	if err := m.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize health checker.
	healthChecker := health.NewChecker()

	// Register a self-check (always passes if we're running).
	healthChecker.RegisterCheck("self", func(ctx context.Context) error {
		return nil
	}, health.Critical())

	// Create the gateway server.
	gatewayServer := server.NewGatewayServer(cfg, logger, m)

	// Create gRPC server with options.
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor(logger, m)),
	)

	// Register gRPC reflection for development/debugging.
	if cfg.Base.IsDevelopment() {
		reflection.Register(grpcServer)
		logger.Debug("gRPC reflection enabled")
	}

	// Note: Proto-generated service registration will be added when proto
	// generation is set up. For now, the server is a skeleton.
	_ = gatewayServer // Suppress unused variable warning

	// Start HTTP server for health checks and metrics.
	httpMux := http.NewServeMux()
	httpMux.Handle("/health", healthChecker.Handler())
	httpMux.Handle("/ready", healthChecker.ReadyHandler())
	httpMux.Handle("/live", healthChecker.LiveHandler())
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
