// Package main provides the entry point for the Penfold Temporal worker.
// This worker processes workflows and activities across multiple task queues:
//   - penfold-main: general workflows
//   - penfold-ai: AI-intensive workflows (embeddings, LLM calls)
//   - penfold-email: email processing workflows
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/otherjamesbrown/penfold/pkg/buildinfo"
	pkgconfig "github.com/otherjamesbrown/penfold/pkg/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/routing"
	enrichmentconfig "github.com/otherjamesbrown/penfold/pkg/enrichment/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/ledger"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/pkg/health"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/logs"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/mentions/resolver"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/pkg/topics"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	pkgobs "github.com/otherjamesbrown/penfold/pkg/temporal/observability"
	"github.com/otherjamesbrown/penfold/pkg/timeout"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	airegistry "github.com/otherjamesbrown/penfold/services/ai/registry"
	"github.com/otherjamesbrown/penfold/services/worker/activities"
	"github.com/otherjamesbrown/penfold/services/worker/config"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
	temporalotel "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
)

// logWriterAdapter adapts logs.Repository to logging.LogWriter.
type logWriterAdapter struct {
	repo *logs.Repository
}

func (a *logWriterAdapter) WriteBatch(ctx context.Context, entries []logging.LogEntry) error {
	inputs := make([]logs.EntryInput, len(entries))
	for i, entry := range entries {
		inputs[i] = logs.EntryInput{
			TenantID:  entry.TenantID,
			Timestamp: entry.Timestamp,
			Level:     logs.Level(entry.Level),
			Service:   entry.Service,
			Message:   entry.Message,
			Fields:    entry.Fields,
			TraceID:   entry.TraceID,
			Caller:    entry.Caller,
		}
	}
	_, err := a.repo.CreateBatch(ctx, inputs)
	return err
}

// projectTaggingRepositoryAdapter adapts entity and mentions repositories for project tagging.
type projectTaggingRepositoryAdapter struct {
	entityRepo   *entities.Repository
	mentionsRepo *mentions.PostgresRepository
	db           *pgxpool.Pool
}

func (a *projectTaggingRepositoryAdapter) GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error) {
	return a.entityRepo.GetProjectsWithKeywords(ctx, tenantID)
}

func (a *projectTaggingRepositoryAdapter) GetContentText(ctx context.Context, tenantID string, contentID int64) (string, error) {
	// Query sources table for raw_content
	query := `SELECT raw_content FROM sources WHERE id = $1 AND tenant_id = $2`
	var contentText string
	err := a.db.QueryRow(ctx, query, contentID, tenantID).Scan(&contentText)
	if err != nil {
		return "", fmt.Errorf("failed to get content text: %w", err)
	}
	return contentText, nil
}

func (a *projectTaggingRepositoryAdapter) CreateContentMention(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error {
	// Check for existing mention to avoid duplicates on reprocessing
	var exists bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM content_mentions
			WHERE tenant_id = $1 AND content_id = $2 AND entity_type = $3
			AND LOWER(mentioned_text) = LOWER($4) AND resolved_entity_id = $5
		)
	`, tenantID, contentID, entityType, mentionedText, resolvedEntityID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check existing mention: %w", err)
	}
	if exists {
		return nil // Already exists, skip
	}

	// Create an already-resolved mention directly via SQL
	query := `
		INSERT INTO content_mentions (
			tenant_id, content_id, entity_type, mentioned_text,
			resolved_entity_id, resolution_confidence, resolution_source,
			status, resolved_at, candidates
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), '[]'::jsonb
		)
	`

	_, err = a.db.Exec(ctx, query,
		tenantID,
		contentID,
		entityType,
		mentionedText,
		resolvedEntityID,
		1.0, // resolution_confidence - exact keyword match
		string(mentions.ResolutionSourceExactMatch),
		string(mentions.MentionStatusAutoResolved),
	)
	if err != nil {
		return fmt.Errorf("failed to create resolved mention: %w", err)
	}

	return nil
}

// workerManager manages multiple Temporal workers for different task queues.
type workerManager struct {
	client  client.Client
	workers map[string]worker.Worker
	logger  logging.Logger
	mu      sync.RWMutex
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logCfg := &logging.Config{
		Level:       logging.Level(cfg.LogLevel),
		ServiceName: cfg.ServiceName,
		Environment: cfg.Environment,
		JSONFormat:  cfg.IsProduction(),
	}
	logger := logging.NewLogger(logCfg)
	logging.SetGlobal(logger)

	logger.Info("Starting Penfold Temporal worker",
		logging.F("version", buildinfo.Version),
		logging.F("commit", buildinfo.Commit),
		logging.F("build_time", buildinfo.BuildTime),
		logging.F("http_port", cfg.HTTPPort),
		logging.F("temporal_host", cfg.TemporalHostPort),
		logging.F("temporal_namespace", cfg.TemporalNamespace),
		logging.F("task_queues", cfg.TaskQueues),
		logging.F("environment", cfg.Environment),
	)

	// Initialize tracing (no-op exporter; OTel context propagation is handled via gRPC interceptors)
	var tracingShutdown tracing.ShutdownFunc
	{
		tracingCfg := &tracing.Config{
			ServiceName: cfg.ServiceName,
			Environment: cfg.Environment,
			SampleRate:  1.0,
			Exporter:    tracing.ExporterNone,
		}
		var err error
		tracingShutdown, err = tracing.InitTracer(tracingCfg)
		if err != nil {
			logger.Error("Failed to initialize tracing", logging.Err(err))
			// Continue without tracing - not fatal
		} else {
			logger.Info("Tracing initialized (no-op exporter)")
		}
	}

	// Initialize Langfuse direct ingestion client for pipeline trace reporting.
	// This is separate from the OTel-based tracing above and uses the Langfuse batch API directly.
	// If env vars are not set, langfuseIngestion will be nil and all activities will be no-ops.
	var langfuseIngestion *langfuse.Ingestion
	if lfIngestionCfg := langfuse.ConfigFromEnv(); lfIngestionCfg != nil {
		langfuseClient, lfErr := langfuse.NewClient(lfIngestionCfg)
		if lfErr != nil {
			logger.Warn("Failed to create Langfuse ingestion client (pipeline tracing disabled)",
				logging.F("error", lfErr),
			)
		} else {
			langfuseIngestion = langfuse.NewIngestion(langfuseClient)
			logger.Info("Langfuse direct ingestion initialized for pipeline phase tracking",
				logging.F("host", lfIngestionCfg.Host),
			)
		}
	} else {
		logger.Info("Langfuse ingestion not configured — pipeline phase tracking disabled")
	}

	// Initialize database pool if configured
	var dbPool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		var err error
		dbPool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("Failed to create database pool", logging.Err(err))
			os.Exit(1)
		}
		defer dbPool.Close()

		// Verify connection
		if err := dbPool.Ping(context.Background()); err != nil {
			logger.Error("Failed to connect to database", logging.Err(err))
			os.Exit(1)
		}
		logger.Info("Connected to database")

		// Initialize DB log sink for async log persistence.
		logsRepo := logs.NewRepository(dbPool)
		logWriter := &logWriterAdapter{repo: logsRepo}
		dbSink := logging.NewDBSink(logging.DBSinkConfig{
			Writer:        logWriter,
			BufferSize:    1000,
			BatchSize:     100,
			FlushInterval: 2 * time.Second,
		})
		defer dbSink.Close()

		// Recreate logger with DB sink attached.
		logCfg.Sinks = []logging.Sink{dbSink}
		logger = logging.NewLogger(logCfg)
		logging.SetGlobal(logger)
		logger.Info("DB log sink initialized")
	} else {
		logger.Warn("DATABASE_URL not configured - activities requiring database will fail")
	}

	// Initialize timeout configuration
	ctx := context.Background()
	// Guard against nil-pointer-in-interface: a nil *pgxpool.Pool passed as
	// timeout.DB creates a non-nil interface wrapping a nil concrete value,
	// which bypasses the db==nil check in Config.Refresh and panics.
	var timeoutDB timeout.DB
	if dbPool != nil {
		timeoutDB = dbPool
	}
	// Use the single known tenant ID — pipeline_operational_config is tenant-scoped.
	timeoutCfg := timeout.New(timeoutDB, pkgconfig.DefaultTenantID().String())

	// Load timeout config from database (non-fatal if fails — uses defaults)
	if err := timeoutCfg.Refresh(ctx); err != nil {
		logger.Warn("Failed to load timeout config from DB, using defaults", logging.F("error", err))
	}

	// Start background refresh loop to pick up runtime changes
	timeoutCfg.StartRefreshLoop(ctx, 30*time.Second)

	// Log configuration changes for observability
	timeoutCfg.OnChange(func(key string, oldVal, newVal time.Duration) {
		logger.Info("Timeout config changed",
			logging.F("key", key),
			logging.F("old", oldVal.String()),
			logging.F("new", newVal.String()),
		)
	})

	// Log all active timeout values at startup for verification
	logger.Info("Timeout configuration loaded",
		logging.F("ai_client.request", timeoutCfg.Get("timeout.ai_client.request").String()),
		logging.F("http.backend.gemini", timeoutCfg.Get("timeout.http.backend.gemini").String()),
		logging.F("http.backend.mlx", timeoutCfg.Get("timeout.http.backend.mlx").String()),
		logging.F("schedule_to_close.default", timeoutCfg.Get("timeout.schedule_to_close.default").String()),
	)

	// Initialize metrics
	svcMetrics := metrics.NewMetrics(cfg.ServiceName, "penfold")
	if err := svcMetrics.RegisterMetrics(); err != nil {
		logger.Error("Failed to register metrics", logging.Err(err))
		os.Exit(1)
	}

	// Initialize Temporal observability metrics
	temporalMetrics := pkgobs.NewMetrics(&pkgobs.MetricsConfig{
		Namespace:   "penfold",
		ServiceName: cfg.ServiceName,
	})

	// Create observability interceptors
	obsInterceptors := pkgobs.NewInterceptors(&pkgobs.InterceptorOptions{
		Logger:        logger,
		Metrics:       temporalMetrics,
		EnableTracing: true,
		EnableMetrics: true,
		EnableLogging: true,
	})
	logger.Info("Temporal observability initialized",
		logging.F("tracing", true),
		logging.F("metrics", true),
		logging.F("logging", true),
	)

	// Create Temporal OTel tracing interceptor for automatic trace context propagation.
	// This propagates the OTel trace context from workflow to activities, enabling
	// stage spans created in activities to be children of the workflow trace.
	temporalTracingInterceptor, err := temporalotel.NewTracingInterceptor(temporalotel.TracerOptions{})
	if err != nil {
		logger.Error("Failed to create Temporal OTel tracing interceptor", logging.Err(err))
		os.Exit(1)
	}
	// Combine: OTel tracing interceptor first, then existing metrics/logging interceptors.
	allInterceptors := append([]interceptor.WorkerInterceptor{temporalTracingInterceptor}, obsInterceptors...)
	logger.Info("Temporal OTel tracing interceptor initialized")

	// Create Temporal client configuration
	temporalCfg := &pkgtemporal.Config{
		HostPort:                cfg.TemporalHostPort,
		Namespace:               cfg.TemporalNamespace,
		MaxConcurrentActivities: cfg.MaxConcurrentActivities,
		MaxConcurrentWorkflows:  cfg.MaxConcurrentWorkflows,
	}

	// Create Temporal client
	temporalClient, err := pkgtemporal.NewClient(
		temporalCfg,
		pkgtemporal.WithLogger(logger),
	)
	if err != nil {
		logger.Error("Failed to create Temporal client", logging.Err(err))
		os.Exit(1)
	}
	defer temporalClient.Close()

	logger.Info("Connected to Temporal server",
		logging.F("host", cfg.TemporalHostPort),
		logging.F("namespace", cfg.TemporalNamespace),
	)

	// Initialize health checker
	healthChecker := health.NewChecker()

	// Create worker manager
	wm := &workerManager{
		client:  temporalClient,
		workers: make(map[string]worker.Worker),
		logger:  logger,
	}

	// Create AI client for gRPC communication with AI Coordinator service
	var aiClient *ai.Client
	if cfg.AIServiceAddr != "" {
		var err error
		aiClient, err = ai.NewClient(cfg.AIServiceAddr,
			ai.WithInsecure(),
			ai.WithRequestTimeout(timeoutCfg.Get("timeout.ai_client.request")),
			ai.WithMaxRetries(3),
		)
		if err != nil {
			logger.Error("Failed to create AI client", logging.Err(err))
			// Continue without AI client - activities will fail gracefully
		} else {
			defer func() {
				if err := aiClient.Close(); err != nil {
					logger.Error("Failed to close AI client", logging.Err(err))
				}
			}()
			logger.Info("AI client created",
				logging.F("ai_service_addr", cfg.AIServiceAddr),
				logging.F("request_timeout", timeoutCfg.Get("timeout.ai_client.request").String()),
			)
		}
	}

	// Create gateway pipeline client for KickNextPending auto-drain
	var pipelineClient pipelinev1.PipelineServiceClient
	if cfg.GatewayAddr != "" {
		gwConn, err := grpc.NewClient(cfg.GatewayAddr,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
		)
		if err != nil {
			logger.Error("Failed to create gateway pipeline client", logging.Err(err))
		} else {
			defer gwConn.Close()
			pipelineClient = pipelinev1.NewPipelineServiceClient(gwConn)
			logger.Info("Gateway pipeline client created",
				logging.F("gateway_addr", cfg.GatewayAddr),
			)
		}
	}

	// Create activity and workflow registrars
	var activityImpl *activities.Activities
	if dbPool != nil {
		activityImpl = activities.NewActivitiesWithDB(logger, dbPool, cfg.AIServiceURL)
		logger.Info("Activities initialized with database connection",
			logging.F("ai_service_url", cfg.AIServiceURL),
		)
	} else {
		activityImpl = activities.NewActivities(logger)
		logger.Warn("Activities initialized without database - some activities will fail")
	}
	activityRegistrar := activities.NewRegistrar(activityImpl)

	// Create pipeline repository if database is available (for run provenance tracking)
	var pipelineRepo activities.PipelineRepository
	if dbPool != nil {
		pipelineRepo = activities.NewPipelineRepository(dbPool)
		logger.Info("Pipeline repository initialized for provenance tracking")
	}

	// Create pipeline activities if pipeline repository is available
	if pipelineRepo != nil && dbPool != nil {
		baseRepo := pipeline.NewRepository(dbPool)
		pipelineActivities := activities.NewPipelineActivities(logger, pipelineRepo, baseRepo)
		if pipelineClient != nil {
			pipelineActivities.WithPipelineClient(pipelineClient)
		}
		activityRegistrar.WithPipelineActivities(pipelineActivities)
		logger.Info("Pipeline activities initialized (RecordOverrides, KickNextPending)")
	}

	// Create parse activities (Stage 0, deterministic - no DB or AI needed)
	// pipelineRepo is passed for pipeline_runs recording; it may be nil if DB is unavailable.
	parseActivities := activities.NewParseActivities(logger, pipelineRepo)
	activityRegistrar.WithParseActivities(parseActivities)
	logger.Info("Parse activities initialized (Stage 0)")

	// Initialize AIClient-based activities if AI client is available
	if aiClient != nil {
		// Create embedding repository if database is available
		var embeddingRepo activities.EmbeddingRepository
		if dbPool != nil {
			embeddingRepo = activities.NewPostgresEmbeddingRepository(dbPool, logger)
			logger.Info("Embedding repository initialized with database connection")
		}

		// Create embedding activities
		embeddingActivities := activities.NewEmbeddingActivities(logger, aiClient, embeddingRepo, pipelineRepo)
		activityRegistrar.WithEmbeddingActivities(embeddingActivities)
		logger.Info("Embedding activities initialized with AI client")

		// Create summary repository if database is available
		var summaryRepo activities.SummaryRepository
		if dbPool != nil {
			summaryRepo = activities.NewPostgresSummaryRepository(dbPool, logger)
			logger.Info("Summary repository initialized with database connection")
		}

		// Create summarization activities
		summarizationActivities := activities.NewSummarizationActivities(logger, aiClient, summaryRepo)
		activityRegistrar.WithSummarizationActivities(summarizationActivities)
		logger.Info("Summarization activities initialized with AI client")

		// Create assertion and entity repositories if database is available
		var assertionRepo activities.AssertionRepository
		var entityRepo activities.EntityRepository
		if dbPool != nil {
			assertionRepo = activities.NewPostgresAssertionRepository(dbPool, logger)
			entityRepo = activities.NewPostgresEntityRepository(dbPool, logger)
			logger.Info("Assertion and entity repositories initialized with database connection")
		}

		// Create extraction activities
		extractionActivities := activities.NewExtractionActivities(logger, aiClient, assertionRepo, entityRepo, pipelineRepo)
		// Wire person lookup for NER header enrichment (pf-2059f5)
		if dbPool != nil {
			entityRepoFull := entities.NewRepository(dbPool, logger)
			extractionActivities.WithPersonLookup(activities.NewPersonLookupAdapter(entityRepoFull))
		}
		// Wire DB-backed model info for context window resolution (pf-b158ef)
		if dbPool != nil {
			modelInfoRepo := airegistry.NewPostgresRepository(dbPool, logger)
			extractionActivities.WithModelInfo(modelInfoRepo)
		}
		activityRegistrar.WithExtractionActivities(extractionActivities)
		logger.Info("Extraction activities initialized with AI client")

		// Create enrichment repository if database is available
		var enrichmentRepo activities.EnrichmentRepository
		if dbPool != nil {
			enrichmentRepo = activities.NewPostgresEnrichmentRepository(dbPool, logger)
			logger.Info("Enrichment repository initialized for content subtype classification")
		}

		// Create classification repository if database is available
		var classificationRepo activities.ClassificationRepository
		if dbPool != nil {
			classificationRepo = classification.NewRepository(dbPool, logger)
			logger.Info("Classification repository initialized for rule engine")
		}

		// Create routing repository if database is available
		var routingRepo activities.RoutingRepository
		if dbPool != nil {
			routingRepo = routing.NewRepository(dbPool)
			logger.Info("Routing repository initialized for pipeline routing")
		}

		// Create triage activities (Stage 1)
		triageActivities := activities.NewTriageActivities(logger, aiClient, pipelineRepo, enrichmentRepo, classificationRepo)
		if routingRepo != nil {
			triageActivities.WithRoutingRepo(routingRepo)
		}
		activityRegistrar.WithTriageActivities(triageActivities)
		logger.Info("Triage activities initialized with AI client (Stage 1)")

		// Create analysis activities (Stage 4)
		analysisActivities := activities.NewAnalysisActivities(logger, aiClient, pipelineRepo)
		activityRegistrar.WithAnalysisActivities(analysisActivities)
		logger.Info("Analysis activities initialized with AI client (Stage 4)")
	}

	// Initialize context builder activities if database is available (Stage 3)
	var entitiesRepo *entities.Repository
	if dbPool != nil {
		// Create entity repository for both resolution and lookups
		entitiesRepo = entities.NewRepository(dbPool, logger)
		entityResolver := entities.NewResolver(entitiesRepo)

		// Create config resolver for tenant-specific patterns
		configRepo := enrichmentconfig.NewConfigRepository(dbPool)
		configResolver := enrichmentconfig.NewConfigResolver(configRepo)

		// Create context package repository
		contextRepo := activities.NewContextPackageRepo(dbPool, logger)

		// Create topic repository and adapter for context enrichment
		topicRepo := topics.NewRepository(dbPool, logger)
		topicAdapter := activities.NewTopicLookupAdapter(topicRepo)

		// Create context builder activities
		contextBuilderActivities := activities.NewContextBuilderActivities(
			logger,
			entityResolver,
			entitiesRepo, // entitiesRepo implements EntityLookupInterface
			contextRepo,
			topicAdapter,
			pipelineRepo,
			configResolver,
		)
		activityRegistrar.WithContextBuilderActivities(contextBuilderActivities)
		logger.Info("Context builder activities initialized with database (Stage 3)")

		// Create person enrichment activities (Stage 3.5)
		// entityRepo satisfies PersonRepository interface (GetPersonByID, UpdatePerson, GetPeopleByDomain)
		// Internal domains loaded from tenant enrichment config
		var internalDomains []string
		tenantCfg, cfgErr := configResolver.GetConfig(context.Background(), "default")
		if cfgErr != nil {
			logger.Warn("Failed to load tenant config for internal domains, enrichment will skip is_internal",
				logging.F("error", cfgErr.Error()),
			)
		} else if tenantCfg != nil {
			internalDomains = tenantCfg.InternalDomains
		}
		domainCompanyRepo := activities.NewDomainCompanyRepository(dbPool, logger)
		personEnrichmentActivities := activities.NewPersonEnrichmentActivities(logger, entitiesRepo, domainCompanyRepo, internalDomains)
		activityRegistrar.WithPersonEnrichmentActivities(personEnrichmentActivities)
		logger.Info("Person enrichment activities initialized with database (Stage 3.5)",
			logging.F("internal_domains_count", len(internalDomains)),
		)
	}

	// Initialize persist activities if database is available (Stage 4.5)
	if dbPool != nil {
		// Create persist repository with review queue and glossary for acronym detection
		reviewQueueRepo := reviewqueue.NewRepository(dbPool)
		glossaryRepo := glossary.NewRepository(dbPool)
		persistRepo := activities.NewPersistRepo(dbPool, logger,
			activities.WithReviewQueue(reviewQueueRepo),
			activities.WithGlossary(glossaryRepo),
		)

		// Create persist activities
		persistActivities := activities.NewPersistActivities(logger, persistRepo, pipelineRepo)
		activityRegistrar.WithPersistActivities(persistActivities)
		logger.Info("Persist activities initialized with database (Stage 4.5)")
	}

	// Initialize mentions activities if database and AI client are available
	var mentionsRepo *mentions.PostgresRepository
	if dbPool != nil && aiClient != nil {
		mentionsRepo = mentions.NewPostgresRepository(dbPool)

		llmConfig := resolver.LLMConfig{
			Provider:   "ai-service",
			Timeout:    5 * time.Minute,
			MaxRetries: 2,
		}

		llmProvider := resolver.NewAIProvider(aiClient, llmConfig)
		logger.Info("Mentions LLM provider initialized via AI service",
			logging.F("ai_service_addr", cfg.AIServiceAddr),
		)

		// Create resolver with provider
		resolverConfig := resolver.DefaultConfig()
		mentionResolver := resolver.NewResolver(
			resolverConfig,
			llmProvider,
			nil, // EntityLookup - can be nil, will use default matching
			mentionsRepo,
			nil, // Tracer - can be nil for now
		)
		mentionResolver.SetPromptStore(pipeline.NewRepository(dbPool))

		mentionsActivities := activities.NewMentionsActivities(
			logger,
			dbPool,
			mentionResolver,
			mentionsRepo,
		)
		activityRegistrar.WithMentionsActivities(mentionsActivities)
		logger.Info("Mentions activities initialized with AI service resolver")
	} else if dbPool != nil {
		logger.Warn("Mentions activities not initialized: AI client not available")
	}

	// Initialize Project Tagging Activities
	if dbPool != nil && entitiesRepo != nil && mentionsRepo != nil {
		projectTaggingRepo := &projectTaggingRepositoryAdapter{
			entityRepo:   entitiesRepo,
			mentionsRepo: mentionsRepo,
			db:           dbPool,
		}
		projectTaggingActivities := activities.NewProjectTaggingActivities(logger, projectTaggingRepo)
		activityRegistrar.WithProjectTaggingActivities(projectTaggingActivities)
		logger.Info("Project tagging activities initialized")
	}

	// Initialize Threading Activities (Stage 2.5: email threading)
	if dbPool != nil {
		sourceRepo := activities.NewPostgresSourceRepository(dbPool, logger)
		threadRepo := activities.NewPostgresThreadRepository(dbPool, logger)
		threadActivities := activities.NewThreadActivities(logger, sourceRepo, threadRepo)
		activityRegistrar.WithThreadActivities(threadActivities)
		logger.Info("Thread activities initialized with database (Stage 2.5)")
	}

	// Initialize Enrichment Record Activities (creates content_enrichment rows after triage)
	if dbPool != nil {
		enrichmentRepo := enrichment.NewRepository(dbPool, logger)
		enrichmentActivities := activities.NewEnrichmentActivities(logger, enrichmentRepo)
		activityRegistrar.WithEnrichmentActivities(enrichmentActivities)
		logger.Info("Enrichment record activities initialized with database")
	}

	// Conversation activities for auto-linking (Stage 2.5)
	if dbPool != nil {
		sourceRepo := activities.NewPostgresSourceRepository(dbPool, logger)
		convRepo := activities.NewPostgresConversationRepository(dbPool, logger)
		// Guard against nil-pointer-in-interface: pass properly nil interface when aiClient is nil.
		var convAIClient activities.AIClient
		if aiClient != nil {
			convAIClient = aiClient
		}
		conversationActivities := activities.NewConversationActivities(logger, sourceRepo, convRepo, convAIClient)
		activityRegistrar.WithConversationActivities(conversationActivities)
		logger.Info("Conversation activities initialized with database (auto-linking)")
	}

	// Session ledger consolidation activities (daily consolidation workflow)
	if dbPool != nil && aiClient != nil {
		ledgerRepo := ledger.NewRepository(dbPool, logger)
		promptRepo := pipeline.NewRepository(dbPool)
		consolidationActivities := activities.NewConsolidationActivities(logger, ledgerRepo, promptRepo, aiClient)
		activityRegistrar.WithConsolidationActivities(consolidationActivities)
		logger.Info("Consolidation activities initialized with database and AI client")
	}

	// Langfuse activities for pipeline trace/phase reporting.
	// langfuseIngestion may be nil if Langfuse is not configured; activities will be no-ops.
	langfuseActivities := activities.NewLangfuseActivities(langfuseIngestion, logger)
	langfuseActivities.WithDB(dbPool)
	activityRegistrar.WithLangfuseActivities(langfuseActivities)
	if langfuseIngestion != nil {
		logger.Info("Langfuse pipeline activities initialized (CreateTrace, ReportPhase, ReportGeneration, FinishTrace)")
	} else {
		logger.Info("Langfuse pipeline activities registered as no-ops (Langfuse not configured)")
	}

	workflowRegistrar := workflows.NewRegistrar()

	// Create workers for each configured task queue
	for _, taskQueue := range cfg.TaskQueues {
		w := pkgtemporal.NewWorkerFromConfig(
			temporalClient,
			&pkgtemporal.Config{
				TaskQueue:               taskQueue,
				MaxConcurrentActivities: cfg.MaxConcurrentActivities,
				MaxConcurrentWorkflows:  cfg.MaxConcurrentWorkflows,
			},
			pkgtemporal.WithWorkerStopTimeout(time.Duration(cfg.GracefulShutdownTimeout)*time.Second),
			pkgtemporal.WithInterceptors(allInterceptors...),
		)

		// Register workflows and activities for this queue
		workflowRegistrar.RegisterAll(w, taskQueue)
		activityRegistrar.RegisterAll(w, taskQueue)

		wm.workers[taskQueue] = w

		logger.Info("Created worker for task queue",
			logging.F("task_queue", taskQueue),
			logging.F("workflow_count", workflowRegistrar.WorkflowCount(taskQueue)),
			logging.F("activity_count", activityRegistrar.ActivityCount(taskQueue)),
		)

		// Register health check for this worker
		healthChecker.RegisterCheck(
			fmt.Sprintf("worker_%s", taskQueue),
			wm.createWorkerHealthCheck(taskQueue),
			health.Critical(),
		)
	}

	// Validate pipeline stage registry against pipeline_definitions (warnings only)
	if dbPool != nil {
		defRepo := pipeline.NewRepository(dbPool)
		definedStages, err := defRepo.ListAllDefinedStages(context.Background())
		if err != nil {
			logger.Warn("failed to load pipeline definitions for registry validation", logging.Err(err))
		} else {
			pkgtemporal.ValidateStageRegistry(logger, definedStages, pkgtemporal.AllMainQueueActivities())
		}
	}

	// Register Temporal connection health check
	healthChecker.RegisterCheck("temporal_connection", func(ctx context.Context) error {
		// Check if we can reach Temporal by describing the namespace
		_, err := temporalClient.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
			Namespace: cfg.TemporalNamespace,
		})
		if err != nil {
			return fmt.Errorf("temporal connection check failed: %w", err)
		}
		return nil
	}, health.Critical())

	// Register database health check if database is configured
	if dbPool != nil {
		healthChecker.RegisterCheck("database", func(ctx context.Context) error {
			return dbPool.Ping(ctx)
		}, health.Critical())
	}

	// Register AI service health check (gRPC-based)
	if aiClient != nil {
		healthChecker.RegisterCheck("ai_service", func(ctx context.Context) error {
			return aiClient.HealthCheck(ctx)
		}, health.Critical())
	}

	// Register Ollama embeddings health check
	healthChecker.RegisterCheck("embeddings", func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.AIServiceURL+"/", nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("embeddings service unreachable: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("embeddings service returned %d", resp.StatusCode)
		}
		return nil
	}, health.Critical())

	// LLM health is now covered by the AI service health check above
	// (AI Coordinator routes LLM calls to Gemini Flash)

	// Start HTTP server for health and metrics
	httpMux := http.NewServeMux()
	httpMux.Handle("/health", healthChecker.Handler())
	httpMux.Handle("/ready", healthChecker.ReadyHandler())
	httpMux.Handle("/live", healthChecker.LiveHandler())
	httpMux.Handle("/metrics", metrics.Handler())
	httpMux.HandleFunc("/version", buildinfo.Handler("penfold-worker"))

	httpServer := &http.Server{
		Addr:         cfg.HTTPAddr(),
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("HTTP server listening", logging.F("address", cfg.HTTPAddr()))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", logging.Err(err))
		}
	}()

	// Start all workers
	errCh := make(chan error, len(wm.workers))
	for taskQueue, w := range wm.workers {
		go func(tq string, wrk worker.Worker) {
			logger.Info("Starting worker", logging.F("task_queue", tq))
			if err := wrk.Run(worker.InterruptCh()); err != nil {
				logger.Error("Worker error", logging.F("task_queue", tq), logging.Err(err))
				errCh <- fmt.Errorf("worker %s failed: %w", tq, err)
			}
		}(taskQueue, w)
	}

	// Ensure Temporal schedules are registered (idempotent)
	if cfg.HasTaskQueue(config.MainTaskQueue) {
		schedCtx, schedCancel := context.WithTimeout(context.Background(), 30*time.Second)
		ensureSchedules(schedCtx, temporalClient, logger)
		schedCancel()
	}

	// Wait for shutdown signal or worker error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", logging.F("signal", sig.String()))
	case err := <-errCh:
		logger.Error("Worker failed, initiating shutdown", logging.Err(err))
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.GracefulShutdownTimeout)*time.Second,
	)
	defer cancel()

	// Stop all workers
	var wg sync.WaitGroup
	for taskQueue, w := range wm.workers {
		wg.Add(1)
		go func(tq string, wrk worker.Worker) {
			defer wg.Done()
			logger.Info("Stopping worker", logging.F("task_queue", tq))
			wrk.Stop()
			logger.Info("Worker stopped", logging.F("task_queue", tq))
		}(taskQueue, w)
	}

	// Wait for workers to stop with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("All workers stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("Worker shutdown timed out")
	}

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", logging.Err(err))
	} else {
		logger.Info("HTTP server stopped")
	}

	// Shutdown tracing
	if tracingShutdown != nil {
		if err := tracingShutdown(shutdownCtx); err != nil {
			logger.Error("Tracing shutdown error", logging.Err(err))
		} else {
			logger.Info("Tracing shutdown complete")
		}
	}

	logger.Info("Penfold Temporal worker shutdown complete")
}

// createWorkerHealthCheck creates a health check function for a specific worker.
func (wm *workerManager) createWorkerHealthCheck(taskQueue string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		wm.mu.RLock()
		defer wm.mu.RUnlock()

		w, exists := wm.workers[taskQueue]
		if !exists {
			return fmt.Errorf("worker for queue %q not found", taskQueue)
		}

		// Check if worker is running (not nil)
		if w == nil {
			return fmt.Errorf("worker for queue %q is nil", taskQueue)
		}

		// The worker is considered healthy if it exists and hasn't errored
		// More sophisticated health checks can be added here
		return nil
	}
}
