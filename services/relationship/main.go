// Package main provides the entry point for the Relationship Discovery gRPC service.
package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/relationship/config"
	"github.com/otherjamesbrown/penfold/services/relationship/server"
)

const version = "0.1.0"

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Use a fallback logger for initial errors
		panic("failed to load configuration: " + err.Error())
	}

	// Initialize logger
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.Level(cfg.LogLevel),
		ServiceName: cfg.ServiceName,
		Environment: string(cfg.Environment),
		JSONFormat:  cfg.IsProduction(),
	})

	logger.Info("Starting Relationship Discovery service",
		logging.F("version", version),
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("environment", cfg.Environment),
	)

	// Initialize metrics
	serviceMetrics := metrics.NewMetrics(cfg.ServiceName, "penfold")
	if err := serviceMetrics.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", logging.Err(err))
	}

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Register basic health check (always healthy for now)
	healthChecker.RegisterCheck("service", func(ctx context.Context) error {
		return nil
	}, health.Critical())

	// FUTURE: Add graph database health check when graph storage is implemented.
	// healthChecker.RegisterCheck("graphdb", graphDBHealthCheck, health.Critical())

	// Create the relationship server
	relationshipServer := server.NewRelationshipServer(cfg, logger)

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(serviceMetrics),
		),
	)

	// Register the relationship service
	relationshipv1.RegisterRelationshipServiceServer(grpcServer, relationshipServer)

	// Enable reflection for debugging (disable in production if needed)
	reflection.Register(grpcServer)

	// Start gRPC server
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		logger.Error("Failed to listen on gRPC port", logging.Err(err))
		os.Exit(1)
	}

	go func() {
		logger.Info("gRPC server listening", logging.F("address", cfg.GRPCAddress()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("gRPC server error", logging.Err(err))
		}
	}()

	// Start HTTP server for health checks and metrics
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
	}

	go func() {
		logger.Info("HTTP server listening", logging.F("address", cfg.HTTPAddress()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received shutdown signal", logging.F("signal", sig.String()))

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Graceful shutdown of HTTP server
	logger.Info("Shutting down HTTP server")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	}

	// Graceful shutdown of gRPC server
	logger.Info("Shutting down gRPC server")
	grpcServer.GracefulStop()

	logger.Info("Relationship Discovery service shutdown complete")
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

// metricsInterceptor creates a gRPC unary interceptor for recording metrics.
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
			m.RecordError("grpc_error")
		}

		m.RecordRequest("GRPC", info.FullMethod, status, duration)

		return resp, err
	}
}
