# Penfold Observability Framework

The Observability Framework provides comprehensive monitoring, tracing, and debugging capabilities for Penfold's Go microservices.

## Overview

Penfold's observability system enables:
- **Structured Logging**: Consistent JSON/human-readable logging with context propagation
- **Distributed Tracing**: OpenTelemetry-based tracing with Langfuse integration for AI operations
- **Prometheus Metrics**: Standard request/error/duration metrics with HTTP and gRPC middleware
- **Health Checks**: Composable health checks with Kubernetes probe support

## Quick Start

### Prerequisites
- Go 1.22+
- PostgreSQL 16+ (for application data)
- Langfuse (optional, for AI tracing)

### Installation

The observability packages are part of the Penfold codebase:

```go
import (
    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/metrics"
    "github.com/penfold/pkg/tracing"
    "github.com/penfold/pkg/health"
)
```

### Basic Usage

```go
package main

import (
    "context"
    "net/http"

    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/metrics"
    "github.com/penfold/pkg/tracing"
    "github.com/penfold/pkg/health"
)

func main() {
    ctx := context.Background()

    // Initialize logging
    logger := logging.NewLogger(&logging.Config{
        Level:       logging.LevelInfo,
        ServiceName: "my-service",
        Environment: "development",
        JSONFormat:  false,
    })
    logging.SetGlobal(logger)

    // Initialize tracing
    shutdown, err := tracing.InitTracer(&tracing.Config{
        ServiceName: "my-service",
        Environment: "development",
        Exporter:    tracing.ExporterStdout,
    })
    if err != nil {
        logger.Error("failed to init tracer", logging.Err(err))
    }
    defer shutdown(ctx)

    // Initialize metrics
    m := metrics.NewMetrics("my-service", "penfold")
    m.RegisterMetrics()

    // Initialize health checker
    checker := health.NewChecker()
    checker.RegisterCheck("database", health.DatabaseCheck(dbPool))

    // Log with context
    logger.Info("service started",
        logging.F("port", 8080),
        logging.F("version", "1.0.0"),
    )
}
```

## Components

### Structured Logging (`pkg/logging`)

Provides consistent structured logging using zerolog with support for JSON and human-readable formats.

```go
import "github.com/penfold/pkg/logging"

// Create a logger
logger := logging.NewLogger(&logging.Config{
    Level:       logging.LevelInfo,
    ServiceName: "gateway",
    Environment: "production",
    JSONFormat:  true,
})

// Log with fields
logger.Info("processing request",
    logging.F("request_id", requestID),
    logging.F("user_id", userID),
)

// Log errors
logger.Error("failed to process",
    logging.Err(err),
    logging.F("content_id", contentID),
)

// Add context (trace IDs, etc.)
ctxLogger := logger.WithContext(ctx)
ctxLogger.Info("traced operation complete")

// Create child logger with persistent fields
serviceLogger := logger.With(
    logging.F("component", "email-processor"),
)
```

**Source**: `pkg/logging/logger.go`

### Distributed Tracing (`pkg/tracing`)

OpenTelemetry-based distributed tracing with multiple exporter options.

```go
import "github.com/penfold/pkg/tracing"

// Initialize tracer with OTLP exporter
shutdown, err := tracing.InitTracer(&tracing.Config{
    ServiceName: "worker",
    Environment: "production",
    Endpoint:    "localhost:4317",
    Exporter:    tracing.ExporterOTLP,
    SampleRate:  1.0,
    Insecure:    true,
})
defer shutdown(ctx)

// Start a span
ctx, span := tracing.StartSpan(ctx, "process-email")
defer span.End()

// Add attributes
tracing.SetAttributes(span,
    tracing.Attr("email_id", emailID),
    tracing.AttrInt("attachment_count", len(attachments)),
)

// Record errors
if err != nil {
    tracing.SetError(span, err)
}

// Add events
tracing.AddEvent(span, "validation-complete",
    tracing.Attr("status", "success"),
)
```

**AI-Specific Tracing** with Langfuse integration:

```go
import "github.com/penfold/pkg/tracing"

// Configure Langfuse exporter
shutdown, err := tracing.InitTracer(&tracing.Config{
    ServiceName: "worker",
    Environment: "production",
    Exporter:    tracing.ExporterLangfuse,
    Langfuse: &tracing.LangfuseConfig{
        Host:      "http://langfuse:3000",
        PublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
        SecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
    },
})

// Trace an LLM call
ctx, span := tracing.StartLLMCall(ctx, "mention-resolution", tracing.LLMCallOptions{
    Model:    "llama3.2",
    System:   tracing.AISystemOllama,
    TaskType: "extraction",
    TenantID: tenantID,
})
defer span.End()

// Record the result
tracing.SetLLMResult(span, tracing.LLMResult{
    InputTokens:  150,
    OutputTokens: 42,
    Model:        "llama3.2:8b",
    LatencyMs:    234,
})

// Trace embedding operations
ctx, span := tracing.StartEmbedding(ctx, "generate-embedding", tracing.EmbeddingOptions{
    Model:  "mxbai-embed-large-v1",
    System: tracing.AISystemMLX,
})
defer span.End()
```

**Source**: `pkg/tracing/tracing.go`, `pkg/tracing/ai.go`, `pkg/tracing/helpers.go`

### Prometheus Metrics (`pkg/metrics`)

Standard Prometheus metrics with automatic HTTP and gRPC instrumentation.

```go
import "github.com/penfold/pkg/metrics"

// Create metrics
m := metrics.NewMetrics("gateway", "penfold")
m.RegisterMetrics()

// Use HTTP middleware for automatic instrumentation
mux := http.NewServeMux()
handler := metrics.HTTPMiddleware(m)(mux)

// Or with configuration
handler := metrics.HTTPMiddlewareWithConfig(&metrics.MiddlewareConfig{
    Metrics:   m,
    SkipPaths: []string{"/healthz", "/metrics"},
})(mux)

// Expose metrics endpoint
mux.Handle("/metrics", metrics.Handler())

// Manual metric recording
m.RecordRequest("POST", "/api/v1/ingest", "200", 0.150)
m.RecordError("database_connection")
m.IncrementConnections()
m.DecrementConnections()

// gRPC interceptors
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(metrics.UnaryServerInterceptor(m)),
    grpc.StreamInterceptor(metrics.StreamServerInterceptor(m)),
)
```

**Available Metrics**:
- `penfold_requests_total` - Counter by method, path, status
- `penfold_request_duration_seconds` - Histogram by method, path
- `penfold_active_connections` - Gauge of active connections
- `penfold_errors_total` - Counter by error type

**Source**: `pkg/metrics/metrics.go`, `pkg/metrics/middleware.go`

### Health Checks (`pkg/health`)

Composable health checks with Kubernetes probe support.

```go
import "github.com/penfold/pkg/health"

// Create health checker
checker := health.NewChecker()

// Register checks
checker.RegisterCheck("database", health.DatabaseCheck(dbPool), health.Critical())
checker.RegisterCheck("redis", health.RedisCheck(redisClient))
checker.RegisterCheck("temporal", health.TemporalCheck(temporalClient))

// Custom check
checker.RegisterCheck("external-api", health.CustomCheck(func() error {
    resp, err := http.Get("https://api.example.com/health")
    if err != nil {
        return err
    }
    if resp.StatusCode != 200 {
        return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
    }
    return nil
}))

// Register HTTP handlers
mux.Handle("/healthz", checker.Handler())      // Full health status
mux.Handle("/readyz", checker.ReadyHandler())  // Readiness probe
mux.Handle("/livez", checker.LiveHandler())    // Liveness probe
```

**Health Status Levels**:
- `healthy` - All checks passed
- `degraded` - Non-critical checks failed
- `unhealthy` - Critical checks failed

**Source**: `pkg/health/health.go`, `pkg/health/checks.go`

## Middleware Integration

### HTTP Server with Full Observability

```go
import (
    "github.com/penfold/pkg/logging"
    "github.com/penfold/pkg/metrics"
    "github.com/penfold/pkg/tracing"
    "github.com/penfold/pkg/health"
)

func setupServer() http.Handler {
    m := metrics.NewMetrics("gateway", "penfold")
    m.RegisterMetrics()

    checker := health.NewChecker()
    checker.RegisterCheck("database", health.DatabaseCheck(dbPool))

    mux := http.NewServeMux()

    // Health and metrics endpoints
    mux.Handle("/healthz", checker.Handler())
    mux.Handle("/readyz", checker.ReadyHandler())
    mux.Handle("/livez", checker.LiveHandler())
    mux.Handle("/metrics", metrics.Handler())

    // Application routes
    mux.HandleFunc("/api/v1/search", handleSearch)

    // Apply middleware (order matters: tracing first, then metrics)
    handler := tracing.HTTPMiddlewareWithConfig(&tracing.HTTPMiddlewareConfig{
        TracerName: "gateway",
        SkipPaths:  []string{"/healthz", "/readyz", "/livez", "/metrics"},
    })(mux)

    handler = metrics.HTTPMiddlewareWithConfig(&metrics.MiddlewareConfig{
        Metrics:   m,
        SkipPaths: []string{"/healthz", "/readyz", "/livez", "/metrics"},
    })(handler)

    return handler
}
```

### gRPC Server with Full Observability

```go
import (
    "github.com/penfold/pkg/metrics"
    "github.com/penfold/pkg/tracing"
)

func setupGRPCServer() *grpc.Server {
    m := metrics.NewMetrics("worker", "penfold")
    m.RegisterMetrics()

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

    return server
}
```

## Configuration

### Environment Variables

```bash
# Logging
LOG_LEVEL=info              # debug, info, warn, error
LOG_FORMAT=json             # json or console

# Tracing
OTEL_EXPORTER_ENDPOINT=localhost:4317
OTEL_SAMPLE_RATE=1.0
OTEL_INSECURE=true

# Langfuse (AI tracing)
LANGFUSE_HOST=http://langfuse:3000
LANGFUSE_PUBLIC_KEY=pk-lf-xxx
LANGFUSE_SECRET_KEY=sk-lf-xxx
```

### Service Configuration

```go
type ServiceConfig struct {
    // Logging
    LogLevel  string `env:"LOG_LEVEL" default:"info"`
    LogFormat string `env:"LOG_FORMAT" default:"json"`

    // Tracing
    TracingEndpoint   string  `env:"OTEL_EXPORTER_ENDPOINT" default:"localhost:4317"`
    TracingSampleRate float64 `env:"OTEL_SAMPLE_RATE" default:"1.0"`
    TracingInsecure   bool    `env:"OTEL_INSECURE" default:"true"`

    // Langfuse
    LangfuseHost      string `env:"LANGFUSE_HOST"`
    LangfusePublicKey string `env:"LANGFUSE_PUBLIC_KEY"`
    LangfuseSecretKey string `env:"LANGFUSE_SECRET_KEY"`
}
```

## Architecture

The observability framework uses:
- **zerolog**: High-performance structured logging
- **OpenTelemetry**: Distributed tracing with W3C Trace Context propagation
- **Prometheus**: Metrics collection and exposition
- **Langfuse**: AI/LLM operation tracing (optional)

```
+----------------+     +------------------+     +------------------+
| HTTP Request   | --> | Tracing          | --> | Metrics          |
|                |     | Middleware       |     | Middleware       |
+----------------+     +------------------+     +------------------+
                              |                        |
                              v                        v
                       +-------------+          +--------------+
                       | OTLP/       |          | Prometheus   |
                       | Langfuse    |          | /metrics     |
                       +-------------+          +--------------+
                              |
                              v
                       +-------------+
                       | Jaeger/     |
                       | Langfuse UI |
                       +-------------+
```

## Package Structure

```
pkg/
  logging/
    logger.go          # Logger interface and implementation
    logger_test.go     # Tests
  metrics/
    metrics.go         # Prometheus metrics definitions
    middleware.go      # HTTP and gRPC middleware
    metrics_test.go
    middleware_test.go
  tracing/
    tracing.go         # OpenTelemetry tracer initialization
    middleware.go      # HTTP and gRPC middleware
    helpers.go         # Span helper functions
    ai.go              # AI/LLM-specific tracing for Langfuse
    tracing_test.go
    langfuse_test.go
  health/
    health.go          # Health checker and HTTP handlers
    checks.go          # Built-in check implementations
    health_test.go
```

## Related Documentation

- [Quickstart Guide](quickstart.md) - Detailed setup instructions
- [Architecture Patterns](../../context/ARCHITECTURE.md) - Implementation patterns
- [Infrastructure](../../context/infrastructure.md) - Service deployment details
