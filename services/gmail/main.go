// Package main is the entry point for the Gmail Connector gRPC service.
// It provides endpoints for synchronizing and managing Gmail messages
// for the Penfold AI processing pipeline.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmail/v1"
	"github.com/otherjamesbrown/penfold/pkg/health"
	storage "github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/gmail/config"
	"github.com/otherjamesbrown/penfold/services/gmail/oauth"
	"github.com/otherjamesbrown/penfold/services/gmail/privacy"
	"github.com/otherjamesbrown/penfold/services/gmail/server"
	gSync "github.com/otherjamesbrown/penfold/services/gmail/sync"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// googleCredentialsFile is the JSON structure for OAuth2 credentials.
// Supports both flat format {"client_id":...} and Google's installed app format
// {"installed":{"client_id":...}}.
type googleCredentialsFile struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	Installed    *struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RedirectURIs []string `json:"redirect_uris"`
	} `json:"installed"`
}

// loadOAuthCredentials reads OAuth2 credentials from the given file path.
func loadOAuthCredentials(path string) (*oauth.Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}

	var f googleCredentialsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing credentials file: %w", err)
	}

	// Support Google's nested "installed" format.
	if f.Installed != nil {
		creds := &oauth.Credentials{
			ClientID:     f.Installed.ClientID,
			ClientSecret: f.Installed.ClientSecret,
		}
		if len(f.Installed.RedirectURIs) > 0 {
			creds.RedirectURI = f.Installed.RedirectURIs[0]
		}
		return creds, nil
	}

	if f.ClientID == "" || f.ClientSecret == "" {
		return nil, fmt.Errorf("credentials file missing client_id or client_secret")
	}

	return &oauth.Credentials{
		ClientID:     f.ClientID,
		ClientSecret: f.ClientSecret,
		RedirectURI:  f.RedirectURI,
	}, nil
}

// buildServerDeps wires up the sync engine and its dependencies.
// Returns nil deps (and logs warnings) if required config is missing.
func buildServerDeps(ctx context.Context, cfg *config.Config, logger logging.Logger) (*server.ServerDeps, error) {
	if cfg.OAuthCredentialsPath == "" {
		logger.Warn("GMAIL_OAUTH_CREDENTIALS_PATH not set; sync engine disabled")
		return nil, nil
	}

	if len(cfg.TokenEncryptionKey) == 0 {
		return nil, fmt.Errorf("GMAIL_TOKEN_ENCRYPTION_KEY is required but not set")
	}

	// Connect to PostgreSQL.
	dbPool, err := pgxpool.New(ctx, cfg.Base.Database.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := dbPool.Ping(ctx); err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Create token storage.
	tokenStorage, err := oauth.NewPostgresTokenStorage(&oauth.PostgresTokenStorageConfig{
		Pool:          dbPool,
		EncryptionKey: cfg.TokenEncryptionKey,
	})
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("creating token storage: %w", err)
	}

	if err := tokenStorage.EnsureTable(ctx); err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("ensuring oauth_tokens table: %w", err)
	}

	// Create sync state storage.
	stateStorage, err := gSync.NewPostgresStateStorage(&gSync.PostgresStateStorageConfig{
		Pool: dbPool,
	})
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("creating state storage: %w", err)
	}

	if err := stateStorage.EnsureTable(ctx); err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("ensuring gmail_sync_state table: %w", err)
	}

	// Load OAuth credentials.
	creds, err := loadOAuthCredentials(cfg.OAuthCredentialsPath)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("loading oauth credentials: %w", err)
	}

	// Create OAuth2 manager.
	oauthManager, err := oauth.NewOAuth2Manager(&oauth.Config{
		Credentials: creds,
		Storage:     tokenStorage,
	})
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("creating oauth2 manager: %w", err)
	}

	// Create privacy filter (no allowlist = all senders pass).
	privacyFilter, err := privacy.NewPrivacyFilter(nil)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("creating privacy filter: %w", err)
	}

	// Create sync engine.
	engineCfg := gSync.DefaultEngineConfig()
	engineCfg.OAuth2Manager = oauthManager
	engineCfg.StateStorage = stateStorage
	engineCfg.PrivacyFilter = privacyFilter
	engineCfg.Logger = logger.With(logging.F("component", "sync_engine"))
	engineCfg.BatchSize = cfg.MaxSyncBatchSize

	engine, err := gSync.NewEngine(engineCfg)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("creating sync engine: %w", err)
	}

	// Create source repository for pipeline ingestion.
	sourceRepo := storage.NewRepository(dbPool, logger)

	var temporalCli client.Client
	temporalHostPort := os.Getenv("TEMPORAL_HOST_PORT")
	if temporalHostPort != "" {
		temporalCfg := &pkgtemporal.Config{
			HostPort:  temporalHostPort,
			Namespace: cfg.Base.Temporal.Namespace,
		}
		var err error
		temporalCli, err = pkgtemporal.NewClient(temporalCfg, pkgtemporal.WithLogger(logger))
		if err != nil {
			logger.Warn("failed to create Temporal client, workflow start disabled",
				logging.Err(err),
			)
		} else {
			logger.Info("Temporal client configured")
		}
	} else {
		logger.Info("TEMPORAL_HOST_PORT not set, workflow start disabled")
	}

	return &server.ServerDeps{
		Engine:         engine,
		OAuthManager:   oauthManager,
		StateStorage:   stateStorage,
		SourceRepo:     sourceRepo,
		TemporalClient: temporalCli,
	}, nil
}

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

	// Wire sync engine dependencies.
	ctx := context.Background()
	deps, err := buildServerDeps(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to build server dependencies", logging.Err(err))
		os.Exit(1)
	}

	// Create gRPC server with options.
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(logger),
			metricsInterceptor(m),
		),
	)

	// Create and register the Gmail service.
	gmailServer := server.NewGmailServer(cfg, logger, deps)
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop accepting new gRPC connections and wait for existing ones to complete.
	grpcServer.GracefulStop()
	logger.Info("gRPC server stopped")

	// Shutdown HTTP server.
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
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
