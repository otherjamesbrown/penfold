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
├── events/             # Event system
│   ├── publisher.go    # Redis event publishing
│   ├── subscriber.go   # Redis event subscription
│   ├── schemas.go      # Event type definitions
│   └── retry.go        # Retry logic
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

## Events Package (pkg/events)

### Publisher

```go
// pkg/events/publisher.go

package events

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
)

type Publisher struct {
    redis *redis.Client
}

func NewPublisher(redis *redis.Client) *Publisher {
    return &Publisher{redis: redis}
}

func (p *Publisher) Publish(ctx context.Context, eventType string, payload interface{}) error {
    event := Event{
        ID:        uuid.New().String(),
        Type:      eventType,
        Timestamp: time.Now(),
        Payload:   payload,
    }

    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    channel := fmt.Sprintf("events:%s", eventType)
    return p.redis.Publish(ctx, channel, data).Err()
}

type Event struct {
    ID        string      `json:"id"`
    Type      string      `json:"type"`
    Timestamp time.Time   `json:"timestamp"`
    Payload   interface{} `json:"payload"`
}
```

### Subscriber

```go
// pkg/events/subscriber.go

package events

import (
    "context"
    "encoding/json"
    "log/slog"

    "github.com/redis/go-redis/v9"
)

type Subscriber struct {
    redis    *redis.Client
    handlers map[string][]EventHandler
}

type EventHandler func(ctx context.Context, event *Event) error

func NewSubscriber(redis *redis.Client) *Subscriber {
    return &Subscriber{
        redis:    redis,
        handlers: make(map[string][]EventHandler),
    }
}

func (s *Subscriber) On(eventType string, handler EventHandler) {
    s.handlers[eventType] = append(s.handlers[eventType], handler)
}

func (s *Subscriber) Start(ctx context.Context) error {
    // Subscribe to all event channels
    patterns := make([]string, 0, len(s.handlers))
    for eventType := range s.handlers {
        patterns = append(patterns, "events:"+eventType)
    }

    pubsub := s.redis.PSubscribe(ctx, "events:*")
    defer pubsub.Close()

    ch := pubsub.Channel()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case msg := <-ch:
            var event Event
            if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
                slog.Error("failed to unmarshal event", "error", err)
                continue
            }

            handlers, ok := s.handlers[event.Type]
            if !ok {
                continue
            }

            for _, handler := range handlers {
                if err := handler(ctx, &event); err != nil {
                    slog.Error("handler failed",
                        "event_type", event.Type,
                        "event_id", event.ID,
                        "error", err,
                    )
                }
            }
        }
    }
}
```

### Event Schemas

```go
// pkg/events/schemas.go

package events

import "time"

// Content events
type ContentIngestedEvent struct {
    TenantID   string            `json:"tenant_id"`
    SourceID   string            `json:"source_id"`
    SourceType string            `json:"source_type"`
    Content    string            `json:"content"`
    Metadata   map[string]string `json:"metadata"`
}

type ContentProcessedEvent struct {
    SourceID string           `json:"source_id"`
    Result   ProcessingResult `json:"result"`
    JobIDs   []string         `json:"job_ids"`
}

type ProcessingResult struct {
    Summary    string            `json:"summary"`
    Entities   []Entity          `json:"entities"`
    Categories []string          `json:"categories"`
    Confidence float32           `json:"confidence"`
}

// Email events
type EmailIngestedEvent struct {
    TenantID     string    `json:"tenant_id"`
    MessageID    string    `json:"message_id"`
    ThreadID     string    `json:"thread_id"`
    AccountEmail string    `json:"account_email"`
    Subject      string    `json:"subject"`
    Snippet      string    `json:"snippet"`
    Labels       []string  `json:"labels"`
    ReceivedAt   time.Time `json:"received_at"`
}

// Job events
type JobCreatedEvent struct {
    JobID    string `json:"job_id"`
    TenantID string `json:"tenant_id"`
    JobType  string `json:"job_type"`
    SourceID string `json:"source_id"`
    Priority int    `json:"priority"`
}

type JobCompletedEvent struct {
    JobID      string `json:"job_id"`
    Success    bool   `json:"success"`
    Result     string `json:"result"`
    DurationMs int64  `json:"duration_ms"`
}

// Relationship events
type RelationshipDiscoveredEvent struct {
    TenantID        string   `json:"tenant_id"`
    RelationshipID  string   `json:"relationship_id"`
    SourceEntityID  string   `json:"source_entity_id"`
    TargetEntityID  string   `json:"target_entity_id"`
    Type            string   `json:"type"`
    Confidence      float32  `json:"confidence"`
    EvidenceIDs     []string `json:"evidence_ids"`
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

func RedisCheck(client *redis.Client) Check {
    return func(ctx context.Context) error {
        return client.Ping(ctx).Err()
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
    "log/slog"
    "net/http"

    "github.com/otherjamesbrown/penfold/pkg/db"
    "github.com/otherjamesbrown/penfold/pkg/events"
    "github.com/otherjamesbrown/penfold/pkg/health"
    "github.com/otherjamesbrown/penfold/pkg/logging"
    "github.com/otherjamesbrown/penfold/pkg/metrics"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/redis/go-redis/v9"
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

    // Setup Redis
    redisClient := redis.NewClient(&redis.Options{
        Addr: "home-01:6379",
    })

    // Setup event publisher
    publisher := events.NewPublisher(redisClient)

    // Setup health checker
    healthChecker := health.NewChecker()
    healthChecker.Register("postgres", health.PostgresCheck(pool))
    healthChecker.Register("redis", health.RedisCheck(redisClient))

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

    // Start service...
}
```
