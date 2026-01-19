// Package main provides the entry point for the Review service.
// The Review service manages daily review items and user review actions,
// surfacing content that requires human review, approval, or action.
package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/review/config"
	"github.com/otherjamesbrown/penfold/services/review/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const serviceName = "review-service"

func main() {
	// Load configuration.
	cfg, err := config.LoadServiceConfig()
	if err != nil {
		// Log to stderr before logger is initialized.
		os.Stderr.WriteString("failed to load configuration: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Ensure service name is set.
	if cfg.ServiceName == "" {
		cfg.ServiceName = serviceName
	}

	// Initialize structured logger.
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.Level(cfg.LogLevel),
		ServiceName: cfg.ServiceName,
		Environment: string(cfg.Environment),
		JSONFormat:  cfg.IsProduction(),
	})
	logging.SetGlobal(logger)

	logger.Info("starting review service",
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("environment", cfg.Environment),
	)

	// Initialize metrics.
	m := metrics.NewMetrics(cfg.ServiceName, "penfold")
	if err := m.RegisterMetrics(); err != nil {
		logger.Error("failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize health checker.
	healthChecker := health.NewChecker()

	// Register a basic liveness check.
	healthChecker.RegisterCheck("grpc_server", func(ctx context.Context) error {
		// Placeholder: will check gRPC server health once implemented.
		return nil
	}, health.Critical())

	// Create gRPC server with options.
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(m),
		),
	)

	// Create and register the Review service server.
	reviewServer := server.NewReviewServer(logger)
	reviewv1.RegisterReviewServiceServer(grpcServer, reviewServer)

	// Enable server reflection for debugging tools like grpcurl.
	reflection.Register(grpcServer)

	// Start gRPC listener.
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		logger.Error("failed to create gRPC listener",
			logging.Err(err),
			logging.F("address", cfg.GRPCAddress()),
		)
		os.Exit(1)
	}

	// Create HTTP server for health checks and metrics.
	httpMux := http.NewServeMux()
	httpMux.Handle("/health", healthChecker.Handler())
	httpMux.Handle("/ready", healthChecker.ReadyHandler())
	httpMux.Handle("/live", healthChecker.LiveHandler())
	httpMux.Handle("/metrics", metrics.Handler())

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddress(),
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to signal shutdown.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start gRPC server in goroutine.
	go func() {
		logger.Info("gRPC server listening", logging.F("address", cfg.GRPCAddress()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("gRPC server error", logging.Err(err))
		}
	}()

	// Start HTTP server in goroutine.
	go func() {
		logger.Info("HTTP server listening", logging.F("address", cfg.HTTPAddress()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	// Wait for shutdown signal.
	sig := <-shutdown
	logger.Info("received shutdown signal", logging.F("signal", sig.String()))

	// Create shutdown context with timeout.
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.ShutdownTimeoutSeconds)*time.Second,
	)
	defer cancel()

	// Gracefully stop gRPC server.
	logger.Info("stopping gRPC server gracefully")
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info("gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("gRPC graceful shutdown timed out, forcing stop")
		grpcServer.Stop()
	}

	// Shutdown HTTP server.
	logger.Info("stopping HTTP server gracefully")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	} else {
		logger.Info("HTTP server stopped gracefully")
	}

	logger.Info("review service shutdown complete")
}

// loggingInterceptor logs gRPC requests.
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
		logger.Info("gRPC request",
			logging.F("method", info.FullMethod),
			logging.F("duration_ms", duration.Milliseconds()),
			logging.F("error", err != nil),
		)

		return resp, err
	}
}

// metricsInterceptor records gRPC metrics.
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
		status := "ok"
		if err != nil {
			status = "error"
		}

		m.RecordRequest("grpc", info.FullMethod, status, duration)

		return resp, err
	}
}
