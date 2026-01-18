// Package main is the entry point for the Gmail Connector gRPC service.
// It provides endpoints for synchronizing and managing Gmail messages
// for the Penfold AI processing pipeline.
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

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/gmail/config"
	"github.com/otherjamesbrown/penfold/services/gmail/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	// Load configuration.
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger.
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.Level(cfg.Base.LogLevel),
		ServiceName: cfg.Base.ServiceName,
		Environment: cfg.Base.Environment.String(),
		JSONFormat:  cfg.Base.IsProduction(),
	})
	logging.SetGlobal(logger)

	logger.Info("starting Gmail Connector service",
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("environment", cfg.Base.Environment),
	)

	// Initialize metrics.
	m := metrics.NewMetrics(cfg.Base.ServiceName, "penfold")
	if err := m.RegisterMetrics(); err != nil {
		logger.Error("failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize health checker.
	healthChecker := health.NewChecker()

	// Register a basic health check for the gRPC server.
	healthChecker.RegisterCheck("grpc_server", func(ctx context.Context) error {
		// This check passes if the service is running.
		// More sophisticated checks can be added later (e.g., Gmail API connectivity).
		return nil
	}, health.Critical())

	// Create gRPC server with options.
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(m),
		),
	)

	// Create and register the Gmail service.
	gmailServer := server.NewGmailServer(cfg, logger)
	gmailv1.RegisterGmailConnectorServiceServer(grpcServer, gmailServer)

	// Enable gRPC reflection for debugging and tooling.
	reflection.Register(grpcServer)

	// Start HTTP server for health checks and metrics.
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

	// Start HTTP server in a goroutine.
	go func() {
		logger.Info("starting HTTP server for health and metrics",
			logging.F("address", cfg.HTTPAddress()),
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	// Start gRPC listener.
	listener, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		logger.Error("failed to create gRPC listener", logging.Err(err))
		os.Exit(1)
	}

	// Start gRPC server in a goroutine.
	go func() {
		logger.Info("starting gRPC server",
			logging.F("address", cfg.GRPCAddress()),
		)
		if err := grpcServer.Serve(listener); err != nil {
			logger.Error("gRPC server error", logging.Err(err))
		}
	}()

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("shutdown signal received, initiating graceful shutdown",
		logging.F("signal", sig.String()),
	)

	// Create a deadline for graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop accepting new gRPC connections and wait for existing ones to complete.
	grpcServer.GracefulStop()
	logger.Info("gRPC server stopped")

	// Shutdown HTTP server.
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	} else {
		logger.Info("HTTP server stopped")
	}

	logger.Info("Gmail Connector service shutdown complete")
}

// loggingInterceptor creates a gRPC unary interceptor for logging requests.
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
		logger.Debug("gRPC request completed",
			logging.F("method", info.FullMethod),
			logging.F("duration_ms", duration.Milliseconds()),
			logging.F("error", err),
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

		duration := time.Since(start)
		status := "ok"
		if err != nil {
			status = "error"
			m.RecordError("grpc_request")
		}

		m.RecordRequest("grpc", info.FullMethod, status, duration.Seconds())

		return resp, err
	}
}
