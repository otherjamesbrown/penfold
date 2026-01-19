# Shared Go Libraries Specification

## Overview

Common Go packages shared across all Penfold microservices to ensure consistency in database access, event handling, configuration, logging, and observability.

## Package Structure

```
pkg/
├── db/                  # Database utilities
│   ├── pool.go         # Connection pooling
│   ├── tx.go           # Transaction helpers
│   ├── vector.go       # pgvector helpers
│   └── tenant.go       # Tenant-aware queries
├── temporal/           # Temporal workflow orchestration
│   ├── client.go       # Temporal client factory
│   ├── starter.go      # Workflow starter utilities
│   ├── activities.go   # Activity base patterns
│   ├── heartbeat.go    # Heartbeat helpers for LLM ops
│   └── config.go       # Temporal configuration
├── config/             # Configuration
│   ├── loader.go       # Config loading
│   └── secrets.go      # Secret management
├── logging/            # Structured logging
│   ├── logger.go       # slog wrapper
│   └── middleware.go   # gRPC/HTTP logging
├── metrics/            # Prometheus metrics
│   ├── registry.go     # Metric registration
│   └── middleware.go   # gRPC/HTTP metrics
├── tracing/            # OpenTelemetry tracing
│   ├── tracer.go       # Tracer setup
│   └── middleware.go   # gRPC/HTTP tracing
├── auth/               # Authentication
│   ├── token.go        # JWT validation
│   └── context.go      # Auth context helpers
├── health/             # Health checks
│   └── checker.go      # Health check utilities
└── proto/              # Shared protobuf definitions
    └── common/
        └── v1/
            └── common.proto
```

## Database Package (pkg/db)

### Connection Pool

```go
// pkg/db/pool.go

package db

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
    Host         string
    Port         int
    Database     string
    User         string
    Password     string
    PoolSize     int
    MaxIdleConns int
    MaxLifetime  time.Duration
}

func NewPool(ctx context.Context, cfg *Config) (*pgxpool.Pool, error) {
    connString := fmt.Sprintf(
        "postgres://%s:%s@%s:%d/%s?pool_max_conns=%d&pool_min_conns=%d",
        cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
        cfg.PoolSize, cfg.PoolSize/4,
    )

    poolCfg, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    poolCfg.MaxConnLifetime = cfg.MaxLifetime
    poolCfg.MaxConnIdleTime = cfg.MaxLifetime / 2

    pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
    if err != nil {
        return nil, fmt.Errorf("failed to create pool: %w", err)
    }

    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    return pool, nil
}
```

### Transaction Helpers

```go
// pkg/db/tx.go

package db

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type TxFunc func(ctx context.Context, tx pgx.Tx) error

func WithTransaction(ctx context.Context, pool *pgxpool.Pool, fn TxFunc) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }

    defer func() {
        if p := recover(); p != nil {
            tx.Rollback(ctx)
            panic(p)
        }
    }()

    if err := fn(ctx, tx); err != nil {
        if rbErr := tx.Rollback(ctx); rbErr != nil {
            return fmt.Errorf("tx failed: %w, rollback failed: %v", err, rbErr)
        }
        return err
    }

    if err := tx.Commit(ctx); err != nil {
        return fmt.Errorf("failed to commit: %w", err)
    }

    return nil
}
```

### Tenant-Aware Queries

```go
// pkg/db/tenant.go

package db

import (
    "context"
    "fmt"
)

type tenantKey struct{}

func ContextWithTenant(ctx context.Context, tenantID string) context.Context {
    return context.WithValue(ctx, tenantKey{}, tenantID)
}

func TenantFromContext(ctx context.Context) (string, error) {
    tenantID, ok := ctx.Value(tenantKey{}).(string)
    if !ok || tenantID == "" {
        return "", fmt.Errorf("tenant not found in context")
    }
    return tenantID, nil
}

// TenantQuery wraps a query to include tenant filtering
type TenantQuery struct {
    pool *pgxpool.Pool
}

func NewTenantQuery(pool *pgxpool.Pool) *TenantQuery {
    return &TenantQuery{pool: pool}
}

func (q *TenantQuery) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
    tenantID, err := TenantFromContext(ctx)
    if err != nil {
        return nil, err
    }

    // Prepend tenant_id to args
    tenantArgs := append([]interface{}{tenantID}, args...)

    return q.pool.Query(ctx, query, tenantArgs...)
}
```

### pgvector Helpers

```go
// pkg/db/vector.go

package db

import (
    "fmt"
    "strings"

    "github.com/pgvector/pgvector-go"
)

// VectorSearch performs similarity search using pgvector
func VectorSearch(ctx context.Context, pool *pgxpool.Pool, cfg VectorSearchConfig) ([]VectorResult, error) {
    tenantID, err := TenantFromContext(ctx)
    if err != nil {
        return nil, err
    }

    query := fmt.Sprintf(`
        SELECT
            e.entity_id,
            e.entity_type,
            1 - (e.embedding <=> $2::vector) as similarity
        FROM embeddings e
        JOIN %s s ON e.entity_id = s.id
        WHERE s.tenant_id = $1
          AND e.entity_type = $3
          AND 1 - (e.embedding <=> $2::vector) > $4
        ORDER BY e.embedding <=> $2::vector
        LIMIT $5
    `, cfg.JoinTable)

    rows, err := pool.Query(ctx, query,
        tenantID,
        pgvector.NewVector(cfg.QueryVector),
        cfg.EntityType,
        cfg.MinSimilarity,
        cfg.Limit,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []VectorResult
    for rows.Next() {
        var r VectorResult
        if err := rows.Scan(&r.EntityID, &r.EntityType, &r.Similarity); err != nil {
            return nil, err
        }
        results = append(results, r)
    }

    return results, nil
}

type VectorSearchConfig struct {
    QueryVector   []float32
    EntityType    string
    JoinTable     string
    MinSimilarity float32
    Limit         int
}

type VectorResult struct {
    EntityID   string
    EntityType string
    Similarity float32
}
```

## Temporal Package (pkg/temporal)

### Client Factory

```go
// pkg/temporal/client.go

package temporal

import (
    "fmt"

    "go.temporal.io/sdk/client"
)

type Config struct {
    HostPort  string
    Namespace string
    TaskQueue string
}

func NewClient(cfg *Config) (client.Client, error) {
    c, err := client.Dial(client.Options{
        HostPort:  cfg.HostPort,
        Namespace: cfg.Namespace,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create temporal client: %w", err)
    }
    return c, nil
}
```

### Workflow Starter

```go
// pkg/temporal/starter.go

package temporal

import (
    "context"
    "fmt"

    "go.temporal.io/sdk/client"
)

type WorkflowStarter struct {
    client    client.Client
    taskQueue string
}

func NewWorkflowStarter(c client.Client, taskQueue string) *WorkflowStarter {
    return &WorkflowStarter{
        client:    c,
        taskQueue: taskQueue,
    }
}

// StartWorkflow starts a workflow with the given ID and input
func (s *WorkflowStarter) StartWorkflow(
    ctx context.Context,
    workflowID string,
    workflow interface{},
    input interface{},
) (client.WorkflowRun, error) {
    options := client.StartWorkflowOptions{
        ID:        workflowID,
        TaskQueue: s.taskQueue,
    }

    we, err := s.client.ExecuteWorkflow(ctx, options, workflow, input)
    if err != nil {
        return nil, fmt.Errorf("failed to start workflow: %w", err)
    }

    return we, nil
}

// GetWorkflowResult waits for a workflow to complete and returns the result
func (s *WorkflowStarter) GetWorkflowResult(ctx context.Context, workflowID, runID string, result interface{}) error {
    we := s.client.GetWorkflow(ctx, workflowID, runID)
    return we.Get(ctx, result)
}
```

### Activity Helpers

```go
// pkg/temporal/activities.go

package temporal

import (
    "time"

    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

// ActivityOptions presets for different operation types

// FastActivityOptions for quick operations like DB queries
func FastActivityOptions() workflow.ActivityOptions {
    return workflow.ActivityOptions{
        StartToCloseTimeout: 30 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    3,
        },
    }
}

// EmbeddingActivityOptions for embedding generation (1-5 seconds)
func EmbeddingActivityOptions() workflow.ActivityOptions {
    return workflow.ActivityOptions{
        StartToCloseTimeout: 30 * time.Second,
        HeartbeatTimeout:    10 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    2 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    3,
        },
    }
}

// LLMActivityOptions for LLM operations (30-60 seconds)
func LLMActivityOptions() workflow.ActivityOptions {
    return workflow.ActivityOptions{
        StartToCloseTimeout:    2 * time.Minute,
        ScheduleToCloseTimeout: 5 * time.Minute,
        HeartbeatTimeout:       15 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    5 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    2, // Fewer retries for expensive ops
        },
    }
}
```

### Heartbeat Helpers

```go
// pkg/temporal/heartbeat.go

package temporal

import (
    "context"
    "time"

    "go.temporal.io/sdk/activity"
)

// HeartbeatLoop runs a heartbeat loop for long-running activities
// Returns a cancel function to stop the loop
func HeartbeatLoop(ctx context.Context, interval time.Duration, details func() interface{}) func() {
    done := make(chan struct{})

    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()

        for {
            select {
            case <-done:
                return
            case <-ctx.Done():
                return
            case <-ticker.C:
                activity.RecordHeartbeat(ctx, details())
            }
        }
    }()

    return func() { close(done) }
}

// WithHeartbeat wraps a long-running operation with automatic heartbeats
func WithHeartbeat[T any](ctx context.Context, interval time.Duration, fn func() (T, error)) (T, error) {
    cancel := HeartbeatLoop(ctx, interval, func() interface{} { return "processing" })
    defer cancel()

    return fn()
}
```

### Workflow Input Types

```go
// pkg/temporal/types.go

package temporal

import "time"

// EmailProcessingInput is the input for email processing workflows
type EmailProcessingInput struct {
    TenantID    string    `json:"tenant_id"`
    SourceID    int64     `json:"source_id"`
    MessageID   string    `json:"message_id"`
    ThreadID    string    `json:"thread_id"`
    FromEmail   string    `json:"from_email"`
    FromName    *string   `json:"from_name,omitempty"`
    Subject     *string   `json:"subject,omitempty"`
    ToEmails    []string  `json:"to_emails"`
    CcEmails    []string  `json:"cc_emails"`
    EmailDate   time.Time `json:"email_date"`
    ContentHash string    `json:"content_hash"`
    JobID       string    `json:"job_id"`
}

// EmailProcessingResult is the result of email processing workflows
type EmailProcessingResult struct {
    SourceID       int64   `json:"source_id"`
    EmbeddingID    *int64  `json:"embedding_id,omitempty"`
    SummaryID      *int64  `json:"summary_id,omitempty"`
    AssertionCount int     `json:"assertion_count"`
    Status         string  `json:"status"` // completed, failed
    Error          string  `json:"error,omitempty"`
}

// ContentProcessingInput is the input for content processing workflows
type ContentProcessingInput struct {
    TenantID   string            `json:"tenant_id"`
    SourceID   string            `json:"source_id"`
    SourceType string            `json:"source_type"`
    Content    string            `json:"content"`
    Metadata   map[string]string `json:"metadata"`
}

// RelationshipDiscoveryInput is the input for relationship discovery workflows
type RelationshipDiscoveryInput struct {
    TenantID  string   `json:"tenant_id"`
    SourceID  string   `json:"source_id"`
    EntityIDs []string `json:"entity_ids"`
}
```

## Logging Package (pkg/logging)

```go
// pkg/logging/logger.go

package logging

import (
    "context"
    "log/slog"
    "os"
)

type Config struct {
    Level  string // debug, info, warn, error
    Format string // json, text
}

func NewLogger(cfg *Config) *slog.Logger {
    var level slog.Level
    switch cfg.Level {
    case "debug":
        level = slog.LevelDebug
    case "warn":
        level = slog.LevelWarn
    case "error":
        level = slog.LevelError
    default:
        level = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{
        Level:     level,
        AddSource: true,
    }

    var handler slog.Handler
    if cfg.Format == "json" {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)
    }

    return slog.New(handler)
}

// Context helpers
type loggerKey struct{}

func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
    return context.WithValue(ctx, loggerKey{}, logger)
}

func LoggerFromContext(ctx context.Context) *slog.Logger {
    if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
        return logger
    }
    return slog.Default()
}

// Add request context
func WithRequestID(logger *slog.Logger, requestID string) *slog.Logger {
    return logger.With("request_id", requestID)
}

func WithTenant(logger *slog.Logger, tenantID string) *slog.Logger {
    return logger.With("tenant_id", tenantID)
}
```

### gRPC Logging Interceptor

```go
// pkg/logging/middleware.go

package logging

import (
    "context"
    "log/slog"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/status"
)

func UnaryServerInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler,
    ) (interface{}, error) {
        start := time.Now()

        resp, err := handler(ctx, req)

        duration := time.Since(start)
        st, _ := status.FromError(err)

        logger.Info("grpc request",
            "method", info.FullMethod,
            "duration_ms", duration.Milliseconds(),
            "status", st.Code().String(),
            "error", err != nil,
        )

        return resp, err
    }
}
```

## Metrics Package (pkg/metrics)

```go
// pkg/metrics/registry.go

package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Request metrics
    RequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "penfold",
            Name:      "requests_total",
            Help:      "Total number of requests",
        },
        []string{"service", "method", "status"},
    )

    RequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "penfold",
            Name:      "request_duration_seconds",
            Help:      "Request duration in seconds",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"service", "method"},
    )

    // Database metrics
    DBQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "penfold",
            Name:      "db_query_duration_seconds",
            Help:      "Database query duration",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"service", "query_type"},
    )

    DBPoolSize = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Namespace: "penfold",
            Name:      "db_pool_size",
            Help:      "Database connection pool size",
        },
        []string{"service"},
    )

    // Event metrics
    EventsPublished = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "penfold",
            Name:      "events_published_total",
            Help:      "Total events published",
        },
        []string{"service", "event_type"},
    )

    EventsProcessed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "penfold",
            Name:      "events_processed_total",
            Help:      "Total events processed",
        },
        []string{"service", "event_type", "status"},
    )

    // Job metrics
    JobsQueued = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Namespace: "penfold",
            Name:      "jobs_queued",
            Help:      "Number of queued jobs",
        },
        []string{"job_type"},
    )

    JobDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "penfold",
            Name:      "job_duration_seconds",
            Help:      "Job processing duration",
            Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
        },
        []string{"job_type"},
    )
)
```

## Health Package (pkg/health)

```go
// pkg/health/checker.go

package health

import (
    "context"
    "sync"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"
)

type Checker struct {
    checks map[string]Check
}

type Check func(ctx context.Context) error

type Status struct {
    Healthy  bool              `json:"healthy"`
    Checks   map[string]Result `json:"checks"`
    Duration time.Duration     `json:"duration"`
}

type Result struct {
    Healthy  bool          `json:"healthy"`
    Duration time.Duration `json:"duration"`
    Error    string        `json:"error,omitempty"`
}

func NewChecker() *Checker {
    return &Checker{
        checks: make(map[string]Check),
    }
}

func (c *Checker) Register(name string, check Check) {
    c.checks[name] = check
}

func (c *Checker) Run(ctx context.Context) *Status {
    start := time.Now()
    status := &Status{
        Healthy: true,
        Checks:  make(map[string]Result),
    }

    var wg sync.WaitGroup
    var mu sync.Mutex

    for name, check := range c.checks {
        wg.Add(1)
        go func(name string, check Check) {
            defer wg.Done()

            checkStart := time.Now()
            err := check(ctx)
            duration := time.Since(checkStart)

            result := Result{
                Healthy:  err == nil,
                Duration: duration,
            }
            if err != nil {
                result.Error = err.Error()
            }

            mu.Lock()
            status.Checks[name] = result
            if !result.Healthy {
                status.Healthy = false
            }
            mu.Unlock()
        }(name, check)
    }

    wg.Wait()
    status.Duration = time.Since(start)
    return status
}

// Common checks
func PostgresCheck(pool *pgxpool.Pool) Check {
    return func(ctx context.Context) error {
        return pool.Ping(ctx)
    }
}

func TemporalCheck(client client.Client) Check {
    return func(ctx context.Context) error {
        // Check Temporal connection by describing the system namespace
        _, err := client.CheckHealth(ctx, &client.CheckHealthRequest{})
        return err
    }
}
```

## Shared Protobuf Definitions

```protobuf
// pkg/proto/common/v1/common.proto

syntax = "proto3";
package common.v1;

import "google/protobuf/timestamp.proto";

// Standard entity reference
message EntityRef {
  string id = 1;
  string type = 2;  // person, project, source, etc.
  string name = 3;
}

// Pagination
message PaginationRequest {
  int32 page = 1;
  int32 page_size = 2;
}

message PaginationResponse {
  int32 page = 1;
  int32 page_size = 2;
  int32 total_count = 3;
  int32 total_pages = 4;
}

// Standard error
message Error {
  string code = 1;
  string message = 2;
  map<string, string> details = 3;
}

// Confidence score with reasoning
message ConfidenceScore {
  float score = 1;
  string reasoning = 2;
  map<string, float> components = 3;
}

// Audit info
message AuditInfo {
  string created_by = 1;
  google.protobuf.Timestamp created_at = 2;
  string updated_by = 3;
  google.protobuf.Timestamp updated_at = 4;
}
```

## Usage Example

```go
// Example service using shared libraries

package main

import (
    "context"
    "encoding/json"
    "log/slog"
    "net/http"
    "os"

    "github.com/otherjamesbrown/penfold/pkg/db"
    "github.com/otherjamesbrown/penfold/pkg/health"
    "github.com/otherjamesbrown/penfold/pkg/logging"
    temporalpkg "github.com/otherjamesbrown/penfold/pkg/temporal"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    ctx := context.Background()

    // Setup logging
    logger := logging.NewLogger(&logging.Config{
        Level:  "info",
        Format: "json",
    })
    slog.SetDefault(logger)

    // Setup database
    pool, err := db.NewPool(ctx, &db.Config{
        Host:     "home-01",
        Port:     5432,
        Database: "penfold",
        User:     "penfold",
        Password: os.Getenv("DB_PASSWORD"),
        PoolSize: 20,
    })
    if err != nil {
        slog.Error("failed to create db pool", "error", err)
        os.Exit(1)
    }
    defer pool.Close()

    // Setup Temporal client
    temporalClient, err := temporalpkg.NewClient(&temporalpkg.Config{
        HostPort:  "localhost:7233",
        Namespace: "default",
        TaskQueue: "penfold-ai-processing",
    })
    if err != nil {
        slog.Error("failed to create temporal client", "error", err)
        os.Exit(1)
    }
    defer temporalClient.Close()

    // Setup workflow starter
    starter := temporalpkg.NewWorkflowStarter(temporalClient, "penfold-ai-processing")

    // Setup health checker
    healthChecker := health.NewChecker()
    healthChecker.Register("postgres", health.PostgresCheck(pool))
    healthChecker.Register("temporal", health.TemporalCheck(temporalClient))

    // Metrics endpoint
    http.Handle("/metrics", promhttp.Handler())

    // Health endpoint
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        status := healthChecker.Run(r.Context())
        if !status.Healthy {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
        json.NewEncoder(w).Encode(status)
    })

    // Example: Start a workflow
    _ = starter // Use starter to trigger workflows

    // Start service...
}
```
