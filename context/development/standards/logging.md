# Logging Standards

Penfold uses a consistent logging wrapper (`pkg/logging`) across all Go services.

## Overview

The `logging` package wraps zerolog to provide a simple, consistent interface:
- `logging.Logger` interface for all components
- `logging.F(key, value)` for structured fields
- `logging.Err(err)` for error fields

## Three Logging Contexts

### 1. Service-Level Logging

For `main.go` and service initialization, create a logger with configuration:

```go
import "github.com/otherjamesbrown/penfold/pkg/logging"

logger := logging.NewLogger(&logging.Config{
    Level:       logging.Level(cfg.LogLevel),
    ServiceName: cfg.ServiceName,
    Environment: cfg.Environment,
    JSONFormat:  cfg.IsProduction(),
})
logging.SetGlobal(logger)

logger.Info("Service started",
    logging.F("version", version),
    logging.F("port", cfg.Port))
```

### 2. Activity Logging

For Temporal activities, accept `logging.Logger` in the constructor:

```go
import "github.com/otherjamesbrown/penfold/pkg/logging"

type MyActivities struct {
    logger logging.Logger
}

func NewMyActivities(logger logging.Logger) *MyActivities {
    return &MyActivities{
        logger: logger.With(logging.F("component", "my_activities")),
    }
}

func (a *MyActivities) ProcessItem(ctx context.Context, id int64) error {
    a.logger.Info("Processing item", logging.F("item_id", id))

    if err := doWork(); err != nil {
        a.logger.Error("Processing failed",
            logging.F("item_id", id),
            logging.Err(err))
        return err
    }

    a.logger.Info("Processing complete",
        logging.F("item_id", id),
        logging.F("duration", time.Since(start)))
    return nil
}
```

### 3. Workflow Logging

**IMPORTANT:** Workflows must use Temporal's logger for replay safety:

```go
import "go.temporal.io/sdk/workflow"

func MyWorkflow(ctx workflow.Context, input Input) error {
    logger := workflow.GetLogger(ctx)
    logger.Info("Workflow started", "input_key", input.Value)

    // ... workflow logic ...

    logger.Info("Workflow completed")
    return nil
}
```

**Never use `logging.Logger` in workflows** - it breaks deterministic replay.

## CLI Output

The CLI (`cmd/penf/`) uses `fmt.Printf` for user-facing output. This is intentional:
- Users expect formatted, human-readable output
- CLI output is not logs - it's the user interface
- Structured logging would be noise for CLI users

## API Reference

### Creating Fields

```go
logging.F("key", "string value")     // String field
logging.F("count", 42)               // Int field
logging.F("duration", time.Second)   // Duration field
logging.F("enabled", true)           // Bool field
logging.Err(err)                     // Error field (key is "error")
```

### Log Levels

```go
logger.Debug("message", fields...)   // Development details
logger.Info("message", fields...)    // Normal operations
logger.Warn("message", fields...)    // Potential issues
logger.Error("message", fields...)   // Errors requiring attention
```

### Derived Loggers

```go
// Add persistent fields for a component
componentLogger := logger.With(
    logging.F("component", "handler"),
    logging.F("tenant_id", tenantID),
)

// Add context (extracts trace_id, request_id if present)
ctxLogger := logger.WithContext(ctx)
```

## Don'ts

- **Don't use zerolog directly** in activities - use `logging.Logger`
- **Don't use `logging.Logger` in workflows** - use Temporal's logger
- **Don't log sensitive data** - passwords, tokens, PII, full request bodies
- **Don't log in hot paths** - avoid debug logging in tight loops

## Where Logs Go

### Production (JSON format)
Logs are written to stdout in JSON format. Access via:

| Service | Location | Command |
|---------|----------|---------|
| Gateway | dev02 | `journalctl -u penfold-gateway -f` |
| Worker | dev01 | `journalctl -u penfold-worker -f` |
| AI Service | dev01 | `journalctl -u penfold-ai -f` |

### Development (Console format)
Human-readable output to terminal with colors.

### Temporal Workflows
Workflow logs are separate from service logs:
```bash
# View workflow execution history
temporal workflow show -w <workflow-id> --namespace penfold

# List recent workflows
temporal workflow list --namespace penfold -q 'WorkflowType="ContentIngestionWorkflow"'
```

## Tracing with Langfuse

AI operations are traced to Langfuse for observability. See `pkg/tracing`:

```go
import "github.com/otherjamesbrown/penfold/pkg/tracing"

// Start an embedding trace
ctx, span := tracing.StartEmbedding(ctx, "ai.embedding", tracing.EmbeddingOptions{
    System:    tracing.AISystemMLX,
    TenantID:  tenantID,
    ContentID: contentID,
})
defer span.End()

// Record result
tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
    Dimensions: dimensions,
    LatencyMs:  latency,
    Error:      err,  // nil if success
})
```

Access traces at the Langfuse dashboard (requires LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY).

## Metrics

Prometheus metrics are exposed at `/metrics` on each service's HTTP port.

```go
import "github.com/otherjamesbrown/penfold/pkg/metrics"

// Create and register metrics
svcMetrics := metrics.NewMetrics(serviceName, "penfold")
svcMetrics.RegisterMetrics()

// Metrics handler
http.Handle("/metrics", metrics.Handler())
```

## Debugging Checklist

When investigating issues:

1. **Check service logs**: `journalctl -u penfold-<service> -f`
2. **Check Temporal UI**: workflow state, activity failures, retries
3. **Check Langfuse**: AI operation traces, latencies, errors
4. **Check Prometheus/Grafana**: resource metrics, error rates

See `context/agents/debugger.md` for full investigation workflow.

## Migration from zerolog

If you find code using zerolog directly:

| Old (zerolog) | New (logging) |
|---------------|---------------|
| `logger zerolog.Logger` | `logger logging.Logger` |
| `logger.Info().Msg("m")` | `logger.Info("m")` |
| `logger.Info().Str("k","v").Msg("m")` | `logger.Info("m", logging.F("k","v"))` |
| `logger.Error().Err(err).Msg("m")` | `logger.Error("m", logging.Err(err))` |
| `.With().Str("k","v").Logger()` | `.With(logging.F("k","v"))` |
