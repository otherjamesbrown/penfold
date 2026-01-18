// Package main is the entry point for the Content Processor gRPC service.
// The Content Processor handles AI-powered content processing pipeline operations
// including embedding generation, summarization, and entity extraction.
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/contentv1"
	pkgconfig "github.com/otherjamesbrown/penfold/pkg/config"
	pkghealth "github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/content/config"
	"github.com/otherjamesbrown/penfold/services/content/server"
)

const serviceName = "content-processor"

func main() {
	// Set default service name if not provided via env.
	if os.Getenv("PENFOLD_SERVICE_NAME") == "" {
		os.Setenv("PENFOLD_SERVICE_NAME", serviceName)
	}

	// Load configuration.
	cfg, err := config.LoadServiceConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger.
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.Level(cfg.LogLevel),
		ServiceName: cfg.ServiceName,
		Environment: cfg.Environment.String(),
		JSONFormat:  cfg.Environment == pkgconfig.EnvProd,
	})
	logging.SetGlobal(logger)

	logger.Info("starting Content Processor service",
		logging.F("grpc_port", cfg.GRPCPort),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("environment", cfg.Environment),
	)

	// Initialize metrics.
	m := metrics.NewMetrics(serviceName, "penfold")
	if err := m.RegisterMetrics(); err != nil {
		logger.Error("failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize health checker.
	healthChecker := pkghealth.NewChecker()
	registerHealthChecks(healthChecker, cfg)

	// Create gRPC server.
	grpcServer := grpc.NewServer()

	// Register Content Processor service.
	contentServer := server.NewContentServer(cfg, logger, m)
	contentv1.RegisterContentProcessorServiceServer(grpcServer, contentServer)

	// Register gRPC health service.
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// Enable gRPC reflection for debugging.
	reflection.Register(grpcServer)

	// Start gRPC server.
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		logger.Error("failed to listen on gRPC port", logging.Err(err))
		os.Exit(1)
	}

	go func() {
		logger.Info("gRPC server starting", logging.F("address", cfg.GRPCAddress()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("gRPC server error", logging.Err(err))
		}
	}()

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
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("HTTP server starting", logging.F("address", cfg.HTTPAddress()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("shutdown signal received", logging.F("signal", sig.String()))

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Set health status to not serving.
	healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// Gracefully stop gRPC server.
	logger.Info("stopping gRPC server gracefully")
	grpcServer.GracefulStop()

	// Shutdown HTTP server.
	logger.Info("stopping HTTP server")
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	}

	logger.Info("Content Processor service stopped")
}

// registerHealthChecks registers all health checks for the service.
func registerHealthChecks(checker *pkghealth.Checker, cfg *config.ServiceConfig) {
	// Self-check (always healthy if running).
	checker.RegisterCheck("self", func(ctx context.Context) error {
		return nil
	})

	// Storage path check.
	checker.RegisterCheck("storage", func(ctx context.Context) error {
		// Check if storage path exists and is writable.
		info, err := os.Stat(cfg.StoragePath)
		if os.IsNotExist(err) {
			// Path doesn't exist, but that's okay in dev - we can create it.
			if cfg.Environment == pkgconfig.EnvDev {
				return nil
			}
			return fmt.Errorf("storage path does not exist: %s", cfg.StoragePath)
		}
		if err != nil {
			return fmt.Errorf("cannot access storage path: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("storage path is not a directory: %s", cfg.StoragePath)
		}
		return nil
	}, pkghealth.Critical())

	// TODO: Add database connectivity check when storage layer is implemented.
	// TODO: Add Redis connectivity check when event processing is implemented.
}
