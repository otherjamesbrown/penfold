# Observability Framework Quickstart Guide

**Last Updated**: 2026-01-23
**Prerequisites**: Go 1.22+, PostgreSQL 16+ (optional for health checks), Langfuse (optional for AI tracing)

## Development Environment Setup

### 1. Core Dependencies

The observability packages are included in Penfold's Go modules. Key dependencies:

```go
// go.mod (excerpt)
require (
    github.com/rs/zerolog v1.32.0      // Structured logging
    github.com/prometheus/client_golang v1.19.0  // Prometheus metrics
    go.opentelemetry.io/otel v1.28.0   // OpenTelemetry tracing
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.28.0
    go.opentelemetry.io/otel/sdk v1.28.0
)
```

### 2. Package Imports

```go
import (
    "github.com/penfold/pkg/logging"  // Structured logging
    "github.com/penfold/pkg/metrics"  // Prometheus metrics
    "github.com/penfold/pkg/tracing"  // OpenTelemetry tracing
    "github.com/penfold/pkg/health"   // Health checks
)
```

## Quick Start: Complete Service Setup

### 1. Initialize All Observability Components

```go
package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/penfold/pkg/health"
    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/metrics"
    "github.com/penfold/pkg/tracing"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 1. Initialize Logging
    logger := logging.NewLogger(&logging.Config{
        Level:       logging.LevelInfo,
        ServiceName: "my-service",
        Environment: os.Getenv("ENVIRONMENT"),
        JSONFormat:  os.Getenv("ENVIRONMENT") == "production",
    })
    logging.SetGlobal(logger)

    logger.Info("starting service")

    // 2. Initialize Tracing
    tracingCfg := &tracing.Config{
        ServiceName: "my-service",
        Environment: os.Getenv("ENVIRONMENT"),
        SampleRate:  1.0,
    }

    // Use Langfuse for AI tracing if configured
    if lfCfg := tracing.LangfuseConfigFromEnv(); lfCfg != nil {
        tracingCfg.Exporter = tracing.ExporterLangfuse
        tracingCfg.Langfuse = lfCfg
        logger.Info("using Langfuse tracing exporter")
    } else if os.Getenv("OTEL_EXPORTER_ENDPOINT") != "" {
        tracingCfg.Exporter = tracing.ExporterOTLP
        tracingCfg.Endpoint = os.Getenv("OTEL_EXPORTER_ENDPOINT")
        tracingCfg.Insecure = os.Getenv("OTEL_INSECURE") == "true"
        logger.Info("using OTLP tracing exporter")
    } else {
        tracingCfg.Exporter = tracing.ExporterStdout
        logger.Info("using stdout tracing exporter")
    }

    shutdownTracing, err := tracing.InitTracer(tracingCfg)
    if err != nil {
        logger.Error("failed to initialize tracing", logging.Err(err))
    } else {
        defer shutdownTracing(ctx)
    }

    // 3. Initialize Metrics
    m := metrics.NewMetrics("my-service", "penfold")
    if err := m.RegisterMetrics(); err != nil {
        logger.Error("failed to register metrics", logging.Err(err))
    }

    // 4. Initialize Health Checker
    checker := health.NewChecker()

    // Register database health check (if using PostgreSQL)
    if dbPool != nil {
        checker.RegisterCheck("database", health.DatabaseCheck(dbPool), health.Critical())
    }

    // 5. Set up HTTP server with observability
    mux := http.NewServeMux()

    // Health and metrics endpoints
    mux.Handle("/healthz", checker.Handler())
    mux.Handle("/readyz", checker.ReadyHandler())
    mux.Handle("/livez", checker.LiveHandler())
    mux.Handle("/metrics", metrics.Handler())

    // Application routes
    mux.HandleFunc("/api/v1/hello", handleHello)

    // Apply middleware
    skipPaths := []string{"/healthz", "/readyz", "/livez", "/metrics"}

    handler := tracing.HTTPMiddlewareWithConfig(&tracing.HTTPMiddlewareConfig{
        TracerName: "my-service",
        SkipPaths:  skipPaths,
    })(mux)

    handler = metrics.HTTPMiddlewareWithConfig(&metrics.MiddlewareConfig{
        Metrics:   m,
        SkipPaths: skipPaths,
    })(handler)

    // 6. Start server
    server := &http.Server{
        Addr:    ":8080",
        Handler: handler,
    }

    go func() {
        logger.Info("server listening", logging.F("addr", ":8080"))
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            logger.Error("server error", logging.Err(err))
        }
    }()

    // 7. Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    logger.Info("shutting down")
    shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
    defer shutdownCancel()

    if err := server.Shutdown(shutdownCtx); err != nil {
        logger.Error("shutdown error", logging.Err(err))
    }
}

func handleHello(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := logging.Global().WithContext(ctx)

    // Start a child span
    ctx, span := tracing.StartSpan(ctx, "handle-hello")
    defer span.End()

    logger.Info("handling request")
    tracing.AddEvent(span, "processing-started")

    // Simulate work
    time.Sleep(10 * time.Millisecond)

    tracing.SetOK(span)
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Hello, World!"))
}
```

### 2. Test the Setup

```bash
# Build and run
go build -o myservice ./cmd/myservice
./myservice

# Test endpoints
curl http://localhost:8080/api/v1/hello
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

## Component-Specific Quickstarts

### Logging Quickstart

```go
package main

import (
    "context"
    "errors"

    "github.com/penfold/pkg/logging"
)

func main() {
    // Create logger with development settings
    logger := logging.NewLogger(&logging.Config{
        Level:       logging.LevelDebug,
        ServiceName: "example",
        Environment: "development",
        JSONFormat:  false, // Human-readable for development
    })

    // Set as global logger
    logging.SetGlobal(logger)

    // Basic logging
    logger.Debug("debug message")
    logger.Info("info message")
    logger.Warn("warning message")
    logger.Error("error message")

    // Logging with fields
    logger.Info("processing item",
        logging.F("item_id", "123"),
        logging.F("item_type", "email"),
        logging.F("size_bytes", 1024),
    )

    // Logging errors
    err := errors.New("something went wrong")
    logger.Error("operation failed",
        logging.Err(err),
        logging.F("operation", "fetch"),
    )

    // Creating a child logger with persistent fields
    emailLogger := logger.With(
        logging.F("component", "email-processor"),
        logging.F("version", "1.0.0"),
    )
    emailLogger.Info("processing email") // Includes component and version

    // Context-aware logging (extracts trace_id, request_id)
    ctx := context.Background()
    ctx = context.WithValue(ctx, logging.TraceIDKey, "abc123")
    ctxLogger := logger.WithContext(ctx)
    ctxLogger.Info("traced operation") // Includes trace_id
}
```

### Tracing Quickstart

```go
package main

import (
    "context"
    "time"

    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/tracing"
)

func main() {
    ctx := context.Background()
    logger := logging.MustGlobal()

    // Initialize tracer with stdout for development
    shutdown, err := tracing.InitTracer(&tracing.Config{
        ServiceName: "example",
        Environment: "development",
        Exporter:    tracing.ExporterStdout,
    })
    if err != nil {
        logger.Error("failed to init tracer", logging.Err(err))
        return
    }
    defer shutdown(ctx)

    // Create a parent span
    ctx, parentSpan := tracing.StartSpan(ctx, "parent-operation")
    defer parentSpan.End()

    // Add attributes
    tracing.SetAttributes(parentSpan,
        tracing.Attr("user_id", "user-123"),
        tracing.AttrInt("batch_size", 10),
    )

    // Create child spans
    for i := 0; i < 3; i++ {
        ctx, childSpan := tracing.StartSpan(ctx, "child-operation")
        time.Sleep(10 * time.Millisecond)

        // Add events
        tracing.AddEvent(childSpan, "item-processed",
            tracing.AttrInt("item_index", i),
        )

        childSpan.End()
    }

    // Get trace ID for correlation
    traceID := tracing.TraceID(ctx)
    logger.Info("operation complete", logging.F("trace_id", traceID))
}
```

### AI Tracing Quickstart (Langfuse)

```go
package main

import (
    "context"
    "os"
    "time"

    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/tracing"
)

func main() {
    ctx := context.Background()
    logger := logging.MustGlobal()

    // Initialize with Langfuse
    shutdown, err := tracing.InitTracer(&tracing.Config{
        ServiceName: "ai-worker",
        Environment: "development",
        Exporter:    tracing.ExporterLangfuse,
        Langfuse: &tracing.LangfuseConfig{
            Host:      os.Getenv("LANGFUSE_HOST"),
            PublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
            SecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
        },
    })
    if err != nil {
        logger.Error("failed to init tracer", logging.Err(err))
        return
    }
    defer shutdown(ctx)

    // Start an AI pipeline trace
    ctx, pipelineSpan := tracing.StartPipeline(ctx, "email-enrichment", "email-123", "email")
    defer pipelineSpan.End()

    // Trace an LLM call
    ctx, llmSpan := tracing.StartLLMCall(ctx, "extract-entities", tracing.LLMCallOptions{
        Model:    "llama3.2:8b",
        System:   tracing.AISystemOllama,
        TaskType: "extraction",
        TenantID: "tenant-123",
    })

    // Simulate LLM call
    start := time.Now()
    time.Sleep(100 * time.Millisecond) // Simulated latency

    // Record result
    tracing.SetLLMResult(llmSpan, tracing.LLMResult{
        InputTokens:  150,
        OutputTokens: 42,
        Model:        "llama3.2:8b",
        LatencyMs:    time.Since(start).Milliseconds(),
    })
    llmSpan.End()

    // Trace an embedding operation
    ctx, embedSpan := tracing.StartEmbedding(ctx, "generate-embedding", tracing.EmbeddingOptions{
        Model:  "mxbai-embed-large-v1",
        System: tracing.AISystemMLX,
    })

    time.Sleep(50 * time.Millisecond)

    tracing.SetEmbeddingResult(embedSpan, tracing.EmbeddingResult{
        Dimensions:  1024,
        InputTokens: 256,
        LatencyMs:   50,
        Cached:      false,
    })
    embedSpan.End()

    // Record AI decision
    tracing.SetDecision(pipelineSpan, "entity-match", 0.95, "High confidence match on email address")

    logger.Info("AI pipeline complete")
}
```

### Metrics Quickstart

```go
package main

import (
    "net/http"
    "time"

    "github.com/penfold/pkg/metrics"
)

func main() {
    // Create metrics with service name and namespace
    m := metrics.NewMetrics("example", "penfold")
    m.RegisterMetrics()

    mux := http.NewServeMux()

    // Expose metrics endpoint
    mux.Handle("/metrics", metrics.Handler())

    // Application handler with manual metrics
    mux.HandleFunc("/api/process", func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Track connection
        m.IncrementConnections()
        defer m.DecrementConnections()

        // Process request
        time.Sleep(50 * time.Millisecond)

        // Record metrics
        duration := time.Since(start).Seconds()
        m.RecordRequest(r.Method, "/api/process", "200", duration)

        w.WriteHeader(http.StatusOK)
    })

    // Apply automatic metrics middleware
    handler := metrics.HTTPMiddleware(m)(mux)

    http.ListenAndServe(":8080", handler)
}
```

### Health Checks Quickstart

```go
package main

import (
    "context"
    "fmt"
    "net/http"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/penfold/pkg/health"
)

func main() {
    checker := health.NewChecker()

    // Database check (critical - if it fails, service is unhealthy)
    dbPool, _ := pgxpool.New(context.Background(), "postgres://...")
    checker.RegisterCheck("database", health.DatabaseCheck(dbPool), health.Critical())

    // Custom check (non-critical - if it fails, service is degraded)
    checker.RegisterCheck("external-api", func(ctx context.Context) error {
        resp, err := http.Get("https://api.example.com/health")
        if err != nil {
            return err
        }
        defer resp.Body.Close()
        if resp.StatusCode != 200 {
            return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
        }
        return nil
    })

    // Context-aware check with timeout
    checker.RegisterCheck("slow-dependency", health.ContextualCheck(func(ctx context.Context) error {
        // This check respects context cancellation
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            // Perform check
            return nil
        }
    }))

    mux := http.NewServeMux()

    // Full health status (for monitoring dashboards)
    // Returns {"status": "healthy|degraded|unhealthy", "checks": {...}, "timestamp": "..."}
    mux.Handle("/healthz", checker.Handler())

    // Readiness probe (for Kubernetes)
    // Returns 503 only if critical checks fail
    mux.Handle("/readyz", checker.ReadyHandler())

    // Liveness probe (for Kubernetes)
    // Always returns 200 if process is running
    mux.Handle("/livez", checker.LiveHandler())

    http.ListenAndServe(":8080", mux)
}
```

## gRPC Service Setup

```go
package main

import (
    "context"
    "net"

    "google.golang.org/grpc"
    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/metrics"
    "github.com/penfold/pkg/tracing"
)

func main() {
    ctx := context.Background()

    // Initialize logging
    logger := logging.NewLogger(&logging.Config{
        Level:       logging.LevelInfo,
        ServiceName: "grpc-service",
        JSONFormat:  true,
    })
    logging.SetGlobal(logger)

    // Initialize tracing
    shutdown, _ := tracing.InitTracer(&tracing.Config{
        ServiceName: "grpc-service",
        Exporter:    tracing.ExporterOTLP,
        Endpoint:    "localhost:4317",
        Insecure:    true,
    })
    defer shutdown(ctx)

    // Initialize metrics
    m := metrics.NewMetrics("grpc-service", "penfold")
    m.RegisterMetrics()

    // Create gRPC server with interceptors
    server := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            tracing.UnaryServerInterceptor(),
            metrics.UnaryServerInterceptor(m),
        ),
        grpc.ChainStreamInterceptor(
            tracing.StreamServerInterceptor(),
            metrics.StreamServerInterceptor(m),
        ),
    )

    // Register your gRPC services here
    // pb.RegisterMyServiceServer(server, &myServiceImpl{})

    // Start server
    lis, _ := net.Listen("tcp", ":50051")
    logger.Info("starting gRPC server", logging.F("addr", ":50051"))
    server.Serve(lis)
}
```

## gRPC Client Setup

```go
package main

import (
    "context"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "github.com/penfold/pkg/tracing"
)

func main() {
    ctx := context.Background()

    // Initialize tracing
    shutdown, _ := tracing.InitTracer(&tracing.Config{
        ServiceName: "grpc-client",
        Exporter:    tracing.ExporterOTLP,
        Endpoint:    "localhost:4317",
        Insecure:    true,
    })
    defer shutdown(ctx)

    // Create gRPC client with tracing interceptors
    conn, _ := grpc.DialContext(ctx, "localhost:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor()),
        grpc.WithStreamInterceptor(tracing.StreamClientInterceptor()),
    )
    defer conn.Close()

    // Use client
    // client := pb.NewMyServiceClient(conn)
    // resp, err := client.MyMethod(ctx, req)
}
```

## Environment Configuration

### Development (.env.development)

```bash
# Service
SERVICE_NAME=my-service
ENVIRONMENT=development

# Logging
LOG_LEVEL=debug
LOG_FORMAT=console

# Tracing (stdout for development)
OTEL_EXPORTER=stdout
```

### Production (.env.production)

```bash
# Service
SERVICE_NAME=my-service
ENVIRONMENT=production

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Tracing (OTLP for production)
OTEL_EXPORTER=otlp
OTEL_EXPORTER_ENDPOINT=otel-collector:4317
OTEL_SAMPLE_RATE=0.1
OTEL_INSECURE=false

# Langfuse (AI tracing)
LANGFUSE_HOST=https://langfuse.example.com
LANGFUSE_PUBLIC_KEY=pk-lf-xxx
LANGFUSE_SECRET_KEY=sk-lf-xxx
```

## Testing Observability

### Test Logging

```go
func TestLogging(t *testing.T) {
    var buf bytes.Buffer
    logger := logging.NewLogger(&logging.Config{
        Level:       logging.LevelDebug,
        ServiceName: "test",
        JSONFormat:  true,
        Output:      &buf,
    })

    logger.Info("test message", logging.F("key", "value"))

    output := buf.String()
    assert.Contains(t, output, "test message")
    assert.Contains(t, output, `"key":"value"`)
}
```

### Test Tracing

```go
func TestTracing(t *testing.T) {
    var buf bytes.Buffer
    shutdown, err := tracing.InitTracer(&tracing.Config{
        ServiceName:  "test",
        Exporter:     tracing.ExporterStdout,
        StdoutWriter: &buf,
    })
    require.NoError(t, err)
    defer shutdown(context.Background())

    ctx, span := tracing.StartSpan(context.Background(), "test-span")
    tracing.SetAttributes(span, tracing.Attr("test", "value"))
    span.End()

    // Force flush
    shutdown(context.Background())

    output := buf.String()
    assert.Contains(t, output, "test-span")
}
```

### Test Health Checks

```go
func TestHealthCheck(t *testing.T) {
    checker := health.NewChecker()
    checker.RegisterCheck("test", func(ctx context.Context) error {
        return nil
    })

    status := checker.Check(context.Background())
    assert.Equal(t, health.StatusHealthy, status.Status)
}
```

## Troubleshooting

### Traces Not Appearing in Langfuse

1. Check environment variables are set correctly
2. Verify network connectivity to Langfuse host
3. Check for errors in logs during tracer initialization
4. Ensure spans are properly ended with `span.End()`

### High Cardinality Metrics

If you see memory issues, check for high-cardinality labels:

```go
// BAD: user_id creates too many label combinations
m.RequestsTotal.WithLabelValues(method, path, status, userID).Inc()

// GOOD: Use path patterns, not actual IDs
m.RequestsTotal.WithLabelValues(method, "/users/{id}", status).Inc()
```

### Missing Trace Context

Ensure trace context is propagated through all layers:

```go
// Always pass context to child functions
func parentHandler(ctx context.Context) {
    ctx, span := tracing.StartSpan(ctx, "parent")
    defer span.End()

    childFunction(ctx) // Pass context!
}

func childFunction(ctx context.Context) {
    ctx, span := tracing.StartSpan(ctx, "child")
    defer span.End()
    // Child span is now linked to parent
}
```

## Related Documentation

- [README](README.md) - Overview and API reference
- [Architecture](../../context/ARCHITECTURE.md) - System architecture patterns
- [Infrastructure](../../context/infrastructure.md) - Deployment and configuration
