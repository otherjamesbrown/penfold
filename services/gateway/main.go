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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	glossaryv1 "github.com/otherjamesbrown/penfold/api/proto/glossary/v1"
	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	logsv1 "github.com/otherjamesbrown/penfold/api/proto/logs/v1"
	mentionsv1 "github.com/otherjamesbrown/penfold/api/proto/mentions/v1"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	productv1 "github.com/otherjamesbrown/penfold/api/proto/product/v1"
	projectv1 "github.com/otherjamesbrown/penfold/api/proto/project/v1"
	questionsv1 "github.com/otherjamesbrown/penfold/api/proto/questions/v1"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/review/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	teamsv1 "github.com/otherjamesbrown/penfold/api/proto/teams/v1"
	tenantv1 "github.com/otherjamesbrown/penfold/api/proto/tenant/v1"
	workflowv1 "github.com/otherjamesbrown/penfold/api/proto/workflow/v1"
	gatewaypb "github.com/otherjamesbrown/penfold/api/proto/core/v1/gatewaypb"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/otherjamesbrown/penfold/pkg/auth"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/logs"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/pkg/products"
	"github.com/otherjamesbrown/penfold/pkg/projects"
	"github.com/otherjamesbrown/penfold/pkg/relationships"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
	"github.com/otherjamesbrown/penfold/pkg/sources"
	"github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
	"github.com/otherjamesbrown/penfold/services/gateway/config"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/services/gateway/entityservice"
	"github.com/otherjamesbrown/penfold/services/gateway/glossaryservice"
	gatewayhealth "github.com/otherjamesbrown/penfold/services/gateway/health"
	"github.com/otherjamesbrown/penfold/services/gateway/ingestservice"
	"github.com/otherjamesbrown/penfold/services/gateway/logsservice"
	"github.com/otherjamesbrown/penfold/services/gateway/mentionsservice"
	"github.com/otherjamesbrown/penfold/services/gateway/middleware"
	"github.com/otherjamesbrown/penfold/services/gateway/modelservice"
	"github.com/otherjamesbrown/penfold/services/gateway/pipelineservice"
	"github.com/otherjamesbrown/penfold/services/gateway/productservice"
	"github.com/otherjamesbrown/penfold/services/gateway/projectservice"
	"github.com/otherjamesbrown/penfold/services/gateway/questionsservice"
	"github.com/otherjamesbrown/penfold/services/gateway/relationshipservice"
	"github.com/otherjamesbrown/penfold/services/gateway/reviewservice"
	"github.com/otherjamesbrown/penfold/services/gateway/searchservice"
	"github.com/otherjamesbrown/penfold/services/gateway/server"
	"github.com/otherjamesbrown/penfold/services/gateway/teamsservice"
	"github.com/otherjamesbrown/penfold/services/gateway/tenantservice"
	"github.com/otherjamesbrown/penfold/services/gateway/workflowservice"
)

func main() {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Load TLS configuration.
	tlsConfig, err := LoadTLSConfig(&cfg.TLS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load TLS config: %v\n", err)
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

	// Log TLS status.
	if tlsConfig != nil {
		logger.Info("TLS enabled",
			logging.F("cert", cfg.TLS.CertFile),
			logging.F("client_auth", cfg.TLS.ClientAuth),
		)
	} else {
		logger.Warn("TLS disabled - connections are unencrypted")
	}

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

	// Create AI service client (optional - gateway can work without it).
	var aiClient *ai.Client
	if cfg.AIServiceAddr != "" {
		var err error
		aiClient, err = ai.NewClient(cfg.AIServiceAddr, ai.WithInsecure())
		if err != nil {
			logger.Warn("Failed to connect to AI service, gateway will operate without AI capabilities",
				logging.F("addr", cfg.AIServiceAddr),
				logging.Err(err),
			)
			// Continue without AI - not fatal
		} else {
			logger.Info("Connected to AI service", logging.F("addr", cfg.AIServiceAddr))

			// Register AI service health check (non-critical).
			healthAggregator.RegisterService(gatewayhealth.ServiceConfig{
				Name:     "ai_service",
				Client:   gatewayhealth.NewAIHealthClient(aiClient.HealthCheck),
				Critical: false, // AI service is not critical for gateway operation
				Timeout:  5 * time.Second,
			})
			logger.Info("Registered AI service health check", logging.F("addr", cfg.AIServiceAddr))
		}
	}

	// Create the gateway server with health aggregator.
	gatewayServer := server.NewGatewayServer(cfg, logger, m, healthAggregator)

	// Build gRPC server options with interceptors.
	grpcOpts := buildGRPCServerOptions(cfg, logger, m)

	// Add TLS credentials if configured.
	if tlsConfig != nil {
		creds := credentials.NewTLS(tlsConfig)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}

	// Create gRPC server with options.
	grpcServer := grpc.NewServer(grpcOpts...)

	// Register gRPC reflection for development/debugging.
	if cfg.Base.IsDevelopment() {
		reflection.Register(grpcServer)
		logger.Debug("gRPC reflection enabled")
	}

	// Register gRPC services.
	gatewaypb.RegisterGatewayServiceServer(grpcServer, gatewayServer)
	logger.Info("Registered GatewayService")

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

	// Register ReviewService for review sessions and item management.
	reviewSvc := reviewservice.NewService(questionsRepo, logger)
	reviewv1.RegisterReviewServiceServer(grpcServer, reviewSvc)
	logger.Info("Registered ReviewService")

	// Register MentionsService.
	mentionsRepo := mentions.NewPostgresRepository(dbPool)
	mentionsSvc := mentionsservice.NewService(mentionsRepo, logger)
	mentionsv1.RegisterMentionsServiceServer(grpcServer, mentionsSvc)
	logger.Info("Registered MentionsService")

	// Register EntityService for bulk entity seeding.
	// Note: tenantRepo is created here to allow tenant resolution in EntityService and later services.
	tenantRepo := tenant.NewRepository(dbPool)
	entityRepo := entities.NewRepository(dbPool, logger)
	productRepo := products.NewRepository(dbPool, logger)
	entitySvc := entityservice.NewService(entityRepo, productRepo, logger)
	entityv1.RegisterEntityServiceServer(grpcServer, entitySvc)
	logger.Info("Registered EntityService")

	// Register PipelineService for pipeline stats and job tracking.
	pipelineRepo := pipeline.NewRepository(dbPool)
	pipelineSvc := pipelineservice.NewService(pipelineRepo, logger)
	pipelinev1.RegisterPipelineServiceServer(grpcServer, pipelineSvc)
	logger.Info("Registered PipelineService")

	// Register ProductService for product CRUD, hierarchy, aliases, and team management.
	productSvc := productservice.NewService(productRepo, entityRepo, logger)
	productv1.RegisterProductServiceServer(grpcServer, productSvc)
	logger.Info("Registered ProductService")

	// Register ProjectService for project CRUD and member management.
	// Uses tenantRepo (created above for EntityService) for tenant slug-to-UUID resolution.
	projectRepo := projects.NewRepository(dbPool, logger)
	projectSvc := projectservice.NewService(projectRepo, entityRepo, logger)
	projectv1.RegisterProjectServiceServer(grpcServer, projectSvc)
	logger.Info("Registered ProjectService")

	// Register TeamsService for team CRUD and member management.
	// Uses entityRepo (created above) for team operations and tenantRepo for tenant resolution.
	teamsSvc := teamsservice.NewService(entityRepo, tenantRepo, logger)
	teamsv1.RegisterTeamsServiceServer(grpcServer, teamsSvc)
	logger.Info("Registered TeamsService")

	// Register IngestService for email and meeting ingestion.
	// Uses tenantRepo (created above) for tenant slug-to-UUID resolution.
	ingestRepo := storage.NewRepository(dbPool, logger)
	ingestTenantAdapter := ingestservice.NewTenantRepoAdapter(func(ctx context.Context, ref string) (string, error) {
		t, err := tenantRepo.GetByRef(ctx, ref)
		if err != nil {
			return "", err
		}
		if t == nil {
			return "", nil
		}
		return t.ID, nil
	})
	ingestSvc := ingestservice.NewService(ingestRepo, ingestTenantAdapter, logger)
	ingestv1.RegisterIngestServiceServer(grpcServer, ingestSvc)
	logger.Info("Registered IngestService")

	// Register ModelService for AI model management and AI operations (Query, Summarize, Analyze).
	// This service works even when aiClient is nil - it returns Unavailable status.
	modelSvc := modelservice.NewService(aiClient, logger)
	modelSvc.SetSourcesRepo(sourcesRepo)
	modelSvc.SetDB(dbPool)
	// Note: Search client can be set later if search service connection is available
	aiv1.RegisterAICoordinatorServiceServer(grpcServer, modelSvc)
	if aiClient != nil {
		logger.Info("Registered ModelService (AI Coordinator proxy + AI Operations)")
	} else {
		logger.Warn("Registered ModelService (AI service not connected, AI operations will return Unavailable)")
	}

	// Register TenantService for multi-tenant management.
	// tenantRepo already created above for projectService
	tenantSvc := tenantservice.NewService(tenantRepo, logger)
	tenantv1.RegisterTenantServiceServer(grpcServer, tenantSvc)
	logger.Info("Registered TenantService")

	// Register RelationshipService for knowledge graph operations.
	// Uses tenantRepo (created above) for tenant slug-to-UUID resolution.
	relationshipRepo := relationships.NewRepository(dbPool, logger)
	relationshipSvc := relationshipservice.NewService(relationshipRepo, logger)
	relationshipv1.RegisterRelationshipServiceServer(grpcServer, relationshipSvc)
	logger.Info("Registered RelationshipService")

	// Register LogsService for centralized log viewing.
	logsRepo := logs.NewRepository(dbPool)
	logsSvc := logsservice.NewService(logsRepo, logger)
	logsv1.RegisterLogsServiceServer(grpcServer, logsSvc)
	logger.Info("Registered LogsService")

	// Register SearchService proxy to backend search service (optional).
	// This allows CLI to use gateway address for all operations.
	if cfg.SearchAddress != "" {
		searchSvc := searchservice.NewService(cfg.SearchAddress, logger)
		connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := searchSvc.Connect(connectCtx); err != nil {
			logger.Warn("Failed to connect to search backend, SearchService will return Unavailable",
				logging.F("search_addr", cfg.SearchAddress),
				logging.Err(err),
			)
		} else {
			logger.Info("Connected to search backend",
				logging.F("search_addr", cfg.SearchAddress),
			)
			defer searchSvc.Close()
		}
		connectCancel()
		searchv1.RegisterSearchServiceServer(grpcServer, searchSvc)
		logger.Info("Registered SearchService",
			logging.F("backend_addr", cfg.SearchAddress),
		)
	} else {
		logger.Info("SearchService disabled (GATEWAY_SEARCH_ADDRESS not set)")
	}

	// Register WorkflowService for Temporal workflow management (optional).
	if cfg.Temporal.Enabled {
		temporalCfg := &temporal.Config{
			HostPort:  cfg.Temporal.HostPort,
			Namespace: cfg.Temporal.Namespace,
		}
		temporalClient, err := temporal.NewClient(temporalCfg, temporal.WithLogger(logger))
		if err != nil {
			logger.Warn("Failed to connect to Temporal, WorkflowService will not be available",
				logging.F("host_port", cfg.Temporal.HostPort),
				logging.Err(err),
			)
		} else {
			workflowSvc := workflowservice.NewService(temporalClient, cfg.Temporal.Namespace, logger)
			workflowv1.RegisterWorkflowServiceServer(grpcServer, workflowSvc)
			logger.Info("Registered WorkflowService",
				logging.F("temporal_host", cfg.Temporal.HostPort),
				logging.F("namespace", cfg.Temporal.Namespace),
			)

			// Register Temporal health check (non-critical).
			healthAggregator.RegisterService(gatewayhealth.ServiceConfig{
				Name: "temporal",
				Client: gatewayhealth.NewTemporalHealthClient(func(ctx context.Context) error {
					// Simple health check - try to list namespaces
					_, err := temporalClient.WorkflowService().GetSystemInfo(ctx, nil)
					return err
				}),
				Critical: false,
				Timeout:  5 * time.Second,
			})
			logger.Info("Registered Temporal health check")

			// Ensure Temporal client is closed on shutdown
			defer func() {
				temporalClient.Close()
				logger.Info("Temporal client closed")
			}()
		}
	} else {
		logger.Info("WorkflowService disabled (TEMPORAL_HOST_PORT not set or GATEWAY_TEMPORAL_ENABLED=false)")
	}

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

	// Close AI client if it was created.
	if aiClient != nil {
		if err := aiClient.Close(); err != nil {
			logger.Error("AI client close error", logging.Err(err))
		} else {
			logger.Info("AI client closed")
		}
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
