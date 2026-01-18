# Temporal Workflow Orchestration - Integration Plan

**Status**: Ready for review
**Created**: 2026-01-18
**Scope**: Replace Redis pub/sub with Temporal for multi-step AI processing pipeline

## Overview

Integrate Temporal workflow orchestration into the Penfold Go pipeline to handle multi-step AI processing with proper backpressure, retry handling, and observability. This replaces the current Redis pub/sub approach which drops messages when the slow LLM processing can't keep up.

## Why Temporal?

| Problem with Redis Pub/Sub | Temporal Solution |
|---------------------------|-------------------|
| Messages dropped when buffer full | Task queue with persistence |
| No visibility into processing state | Full execution history in Web UI |
| Manual retry/circuit breaker logic | Built-in retry policies |
| No dead letter handling | Failed workflows visible & retryable |
| Difficult to add preprocessing steps | Workflows as code - easy to extend |

## Architecture

### Current State
```
Redis Pub/Sub → Go Pipeline → [Embedding] → [LLM Summary] → [LLM Assertions] → PostgreSQL
     ↓
 (messages dropped when LLM is slow)
```

### Target State
```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Temporal Server                                    │
│  ┌─────────────┐    ┌──────────────────┐    ┌─────────────────────────────┐ │
│  │ Task Queue  │───▶│ Email Processing │───▶│ Execution History           │ │
│  │ (persisted) │    │ Workflow         │    │ (full visibility)           │ │
│  └─────────────┘    └──────────────────┘    └─────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
         ▲                     │
         │                     ▼
┌────────┴───────┐    ┌───────────────────────────────────────────────────────┐
│ Workflow       │    │                    Activities                          │
│ Starter        │    │  ┌────────────┐  ┌────────────┐  ┌─────────────────┐  │
│ (Redis bridge  │    │  │ Fetch      │  │ Generate   │  │ Generate        │  │
│  or CLI)       │    │  │ Source     │  │ Embedding  │  │ Summary         │  │
└────────────────┘    │  └────────────┘  └────────────┘  └─────────────────┘  │
                      │  ┌────────────┐  ┌────────────┐  ┌─────────────────┐  │
                      │  │ Extract    │  │ Store      │  │ Update          │  │
                      │  │ Assertions │  │ Results    │  │ Status          │  │
                      │  └────────────┘  └────────────┘  └─────────────────┘  │
                      └───────────────────────────────────────────────────────┘
                                         │
                      ┌──────────────────┼──────────────────┐
                      ▼                  ▼                  ▼
               MLX Sidecar          vLLM-MLX          PostgreSQL
               (embeddings)         (LLM)             (storage)
               :8001                :8000
```

## Project Structure

```
penfold-go-pipeline/
├── cmd/
│   ├── pipeline/main.go          # Existing entry point (will become workflow starter)
│   └── worker/main.go            # NEW: Temporal worker process
├── internal/
│   ├── workflows/                # NEW: Workflow definitions
│   │   ├── email.go              # EmailProcessingWorkflow
│   │   └── content.go            # ContentProcessingWorkflow (future)
│   ├── activities/               # NEW: Activity implementations
│   │   ├── source.go             # FetchSource activity
│   │   ├── embedding.go          # GenerateEmbedding activity
│   │   ├── llm.go                # GenerateSummary, ExtractAssertions
│   │   └── storage.go            # StoreResults, UpdateStatus
│   ├── temporal/                 # NEW: Temporal client/config
│   │   ├── client.go             # Client factory
│   │   └── config.go             # Temporal configuration
│   ├── clients/                  # KEEP: AI service clients
│   │   ├── embeddings.go
│   │   └── llm.go
│   ├── storage/                  # KEEP: Database repositories
│   │   ├── postgres.go
│   │   ├── embeddings.go
│   │   ├── results.go
│   │   └── sources.go
│   ├── events/                   # KEEP (modified): Event schemas + bridge
│   │   ├── schemas.go            # Event types (unchanged)
│   │   ├── subscriber.go         # Redis subscriber (modified to trigger workflows)
│   │   └── router.go             # Route events to workflow starter
│   ├── config/config.go          # Add Temporal config
│   └── health/health.go          # Add Temporal health check
├── sidecar/app.py                # KEEP: MLX embeddings service
├── docker-compose.temporal.yml   # NEW: Temporal server for local dev
├── go.mod                        # Add temporal SDK dependency
└── Makefile                      # Add temporal targets
```

## Workflow Definition

### EmailProcessingWorkflow

```go
// internal/workflows/email.go

package workflows

import (
    "time"
    "go.temporal.io/sdk/workflow"
    "go.temporal.io/sdk/temporal"
)

type EmailProcessingInput struct {
    TenantID    string
    SourceID    int64
    MessageID   string
    FromEmail   string
    FromName    *string
    Subject     *string
    ToEmails    []string
    CcEmails    []string
    EmailDate   time.Time
    ContentHash string
    JobID       string
}

type EmailProcessingResult struct {
    SourceID       int64
    EmbeddingID    *int64
    SummaryID      *int64
    AssertionCount int
    Status         string
    Error          string
}

func EmailProcessingWorkflow(ctx workflow.Context, input EmailProcessingInput) (*EmailProcessingResult, error) {
    logger := workflow.GetLogger(ctx)
    logger.Info("Starting email processing", "source_id", input.SourceID)

    result := &EmailProcessingResult{SourceID: input.SourceID}

    // Activity options for fast operations (DB queries)
    fastOpts := workflow.ActivityOptions{
        StartToCloseTimeout: 30 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    3,
        },
    }

    // Activity options for embedding generation (1-5 seconds)
    embeddingOpts := workflow.ActivityOptions{
        StartToCloseTimeout: 30 * time.Second,
        HeartbeatTimeout:    10 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    2 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    3,
        },
    }

    // Activity options for LLM operations (30-60 seconds)
    llmOpts := workflow.ActivityOptions{
        StartToCloseTimeout:    2 * time.Minute,
        ScheduleToCloseTimeout: 5 * time.Minute,
        HeartbeatTimeout:       15 * time.Second,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    5 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    2, // Fewer retries for expensive ops
        },
    }

    // Step 1: Fetch source content
    var source SourceContent
    ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
    err := workflow.ExecuteActivity(ctx1, "FetchSource", input.TenantID, input.SourceID).Get(ctx, &source)
    if err != nil {
        result.Status = "failed"
        result.Error = "fetch_source: " + err.Error()
        return result, nil // Return result, not error (for visibility)
    }

    // Build email context for AI processing
    emailContext := buildEmailContext(input, source.ContentText)

    // Step 2: Generate embedding (can run in parallel with LLM if desired)
    var embeddingID int64
    ctx2 := workflow.WithActivityOptions(ctx, embeddingOpts)
    err = workflow.ExecuteActivity(ctx2, "GenerateEmbedding", GenerateEmbeddingInput{
        TenantID:    input.TenantID,
        SourceID:    input.SourceID,
        Content:     emailContext,
        ContentHash: input.ContentHash,
    }).Get(ctx, &embeddingID)
    if err != nil {
        logger.Warn("Embedding generation failed", "error", err)
        // Continue - embedding failure shouldn't block other processing
    } else {
        result.EmbeddingID = &embeddingID
    }

    // Step 3: Generate summary via LLM
    var summaryID int64
    ctx3 := workflow.WithActivityOptions(ctx, llmOpts)
    err = workflow.ExecuteActivity(ctx3, "GenerateSummary", GenerateSummaryInput{
        TenantID: input.TenantID,
        SourceID: input.SourceID,
        JobID:    input.JobID,
        Content:  emailContext,
    }).Get(ctx, &summaryID)
    if err != nil {
        logger.Warn("Summary generation failed", "error", err)
    } else {
        result.SummaryID = &summaryID
    }

    // Step 4: Extract assertions via LLM
    var assertionCount int
    ctx4 := workflow.WithActivityOptions(ctx, llmOpts)
    err = workflow.ExecuteActivity(ctx4, "ExtractAssertions", ExtractAssertionsInput{
        TenantID: input.TenantID,
        SourceID: input.SourceID,
        JobID:    input.JobID,
        Content:  emailContext,
    }).Get(ctx, &assertionCount)
    if err != nil {
        logger.Warn("Assertion extraction failed", "error", err)
    } else {
        result.AssertionCount = assertionCount
    }

    // Step 5: Update source status
    ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
    err = workflow.ExecuteActivity(ctx5, "UpdateSourceStatus", input.TenantID, input.SourceID, "completed").Get(ctx, nil)
    if err != nil {
        logger.Warn("Status update failed", "error", err)
    }

    result.Status = "completed"
    logger.Info("Email processing completed", "source_id", input.SourceID, "assertions", result.AssertionCount)

    return result, nil
}
```

## Activity Definitions

### Activity Struct with Dependencies

```go
// internal/activities/activities.go

package activities

import (
    "context"
    "github.com/otherjamesbrown/penfold-go-pipeline/internal/clients"
    "github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
    "github.com/rs/zerolog"
    "go.temporal.io/sdk/activity"
)

type Activities struct {
    sourceRepo    *storage.SourceRepository
    embeddingRepo *storage.EmbeddingRepository
    resultRepo    *storage.ProcessingResultRepository
    embedClient   *clients.EmbeddingsClient
    llmClient     *clients.LLMClient
    logger        zerolog.Logger
    config        *Config
}

func NewActivities(
    sourceRepo *storage.SourceRepository,
    embeddingRepo *storage.EmbeddingRepository,
    resultRepo *storage.ProcessingResultRepository,
    embedClient *clients.EmbeddingsClient,
    llmClient *clients.LLMClient,
    logger zerolog.Logger,
    config *Config,
) *Activities {
    return &Activities{
        sourceRepo:    sourceRepo,
        embeddingRepo: embeddingRepo,
        resultRepo:    resultRepo,
        embedClient:   embedClient,
        llmClient:     llmClient,
        logger:        logger,
        config:        config,
    }
}
```

### LLM Activity with Heartbeat

```go
// internal/activities/llm.go

func (a *Activities) GenerateSummary(ctx context.Context, input GenerateSummaryInput) (int64, error) {
    logger := activity.GetLogger(ctx)
    logger.Info("Generating summary", "source_id", input.SourceID)

    // Record heartbeat before long operation
    activity.RecordHeartbeat(ctx, "starting_llm_call")

    // Call LLM (30-60 seconds)
    summary, err := a.llmClient.GenerateSummary(ctx, input.Content)
    if err != nil {
        return 0, fmt.Errorf("llm call failed: %w", err)
    }

    activity.RecordHeartbeat(ctx, "llm_complete_storing_result")

    // Store result
    summaryData, _ := json.Marshal(map[string]interface{}{
        "summary":   summary,
        "source_id": input.SourceID,
    })

    result := &storage.ProcessingResult{
        TenantID:   input.TenantID,
        JobID:      input.JobID,
        ResultType: "brief_summary",
        ResultData: summaryData,
        ModelName:  &a.config.LLMModel,
    }

    if err := a.resultRepo.Store(ctx, result); err != nil {
        return 0, fmt.Errorf("store result failed: %w", err)
    }

    return result.ID, nil
}
```

## Worker Setup

```go
// cmd/worker/main.go

package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/rs/zerolog"
    "go.temporal.io/sdk/client"
    "go.temporal.io/sdk/worker"

    "github.com/otherjamesbrown/penfold-go-pipeline/internal/activities"
    "github.com/otherjamesbrown/penfold-go-pipeline/internal/clients"
    "github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
    "github.com/otherjamesbrown/penfold-go-pipeline/internal/storage"
    "github.com/otherjamesbrown/penfold-go-pipeline/internal/workflows"
)

const TaskQueue = "penfold-ai-processing"

func main() {
    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

    // Load config
    cfg, err := config.Load()
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to load config")
    }

    // Initialize Temporal client
    c, err := client.Dial(client.Options{
        HostPort: cfg.Temporal.HostPort, // localhost:7233
    })
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to create Temporal client")
    }
    defer c.Close()

    // Initialize dependencies (same as current pipeline)
    db, _ := storage.NewDB(context.Background(), cfg.Database, logger)
    defer db.Close()

    pool := db.Pool()
    sourceRepo := storage.NewSourceRepository(pool, logger)
    embeddingRepo := storage.NewEmbeddingRepository(pool, logger)
    resultRepo := storage.NewProcessingResultRepository(pool, logger)
    embedClient := clients.NewEmbeddingsClient(cfg.AI.Embeddings, logger)
    llmClient := clients.NewLLMClient(cfg.AI.LLM, logger)

    // Create activities instance with all dependencies
    acts := activities.NewActivities(
        sourceRepo, embeddingRepo, resultRepo,
        embedClient, llmClient,
        logger, cfg.Activities,
    )

    // Create worker
    w := worker.New(c, TaskQueue, worker.Options{
        MaxConcurrentActivityExecutionSize: 4,  // Limit concurrent LLM calls
    })

    // Register workflows
    w.RegisterWorkflow(workflows.EmailProcessingWorkflow)

    // Register activities
    w.RegisterActivity(acts.FetchSource)
    w.RegisterActivity(acts.GenerateEmbedding)
    w.RegisterActivity(acts.GenerateSummary)
    w.RegisterActivity(acts.ExtractAssertions)
    w.RegisterActivity(acts.StoreResults)
    w.RegisterActivity(acts.UpdateSourceStatus)

    // Run worker
    logger.Info().Str("task_queue", TaskQueue).Msg("Starting Temporal worker")

    go func() {
        if err := w.Run(worker.InterruptCh()); err != nil {
            logger.Fatal().Err(err).Msg("Worker failed")
        }
    }()

    // Wait for shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    logger.Info().Msg("Shutting down worker")
}
```

## Workflow Starter (Bridge from Redis)

```go
// cmd/pipeline/main.go (modified)

// Instead of processing inline, start a workflow
router.RegisterManualEmailHandler(func(ctx context.Context, event *events.ManualEmailIngestedEvent) error {
    logger.Info().Int64("source_id", event.SourceID).Msg("Starting workflow for email")

    workflowOptions := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("email-processing-%d", event.SourceID),
        TaskQueue: "penfold-ai-processing",
    }

    input := workflows.EmailProcessingInput{
        TenantID:    event.TenantID,
        SourceID:    event.SourceID,
        MessageID:   event.MessageID,
        FromEmail:   event.FromEmail,
        FromName:    event.FromName,
        Subject:     event.Subject,
        ToEmails:    event.ToEmails,
        CcEmails:    event.CcEmails,
        EmailDate:   event.EmailDate,
        ContentHash: event.ContentHash,
        JobID:       event.JobID,
    }

    we, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, workflows.EmailProcessingWorkflow, input)
    if err != nil {
        logger.Error().Err(err).Msg("Failed to start workflow")
        return err
    }

    logger.Info().
        Str("workflow_id", we.GetID()).
        Str("run_id", we.GetRunID()).
        Msg("Workflow started")

    return nil // Don't wait for completion - workflow runs async
})
```

## Temporal Server Setup

### Docker Compose for Local Development

```yaml
# docker-compose.temporal.yml

version: '3.8'

services:
  temporal:
    image: temporalio/auto-setup:1.24
    ports:
      - "7233:7233"   # gRPC API
    environment:
      - DB=postgresql
      - DB_PORT=5432
      - POSTGRES_USER=penfold
      - POSTGRES_PWD=${DB_PASSWORD}
      - POSTGRES_SEEDS=10.0.10.253
      - DYNAMIC_CONFIG_FILE_PATH=/etc/temporal/dynamic_config.yaml
    volumes:
      - ./temporal-config/dynamic_config.yaml:/etc/temporal/dynamic_config.yaml

  temporal-ui:
    image: temporalio/ui:2.26
    ports:
      - "8088:8080"   # Web UI (8088 to avoid conflict with LLM on 8000)
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
    depends_on:
      - temporal

  temporal-admin-tools:
    image: temporalio/admin-tools:1.24
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
    depends_on:
      - temporal
```

### Temporal Configuration

```yaml
# temporal-config/dynamic_config.yaml

# Increase workflow execution history size
frontend.historyMaxPageSize:
  - value: 10000
    constraints: {}

# Namespace-level settings
limit.maxIDLength:
  - value: 255
    constraints: {}
```

## Configuration Updates

```go
// internal/config/config.go (additions)

type TemporalConfig struct {
    HostPort  string        `env:"TEMPORAL_HOST_PORT" envDefault:"localhost:7233"`
    Namespace string        `env:"TEMPORAL_NAMESPACE" envDefault:"default"`
    TaskQueue string        `env:"TEMPORAL_TASK_QUEUE" envDefault:"penfold-ai-processing"`
}

type Config struct {
    // ... existing fields ...
    Temporal TemporalConfig
}
```

Environment variables:
```bash
# Temporal
TEMPORAL_HOST_PORT=localhost:7233
TEMPORAL_NAMESPACE=default
TEMPORAL_TASK_QUEUE=penfold-ai-processing
```

## New Dependencies

```
go get go.temporal.io/sdk@latest
```

Add to go.mod:
```
go.temporal.io/sdk v1.29.1
```

## Implementation Phases

### Phase 1: Temporal Infrastructure
- [ ] Add docker-compose.temporal.yml
- [ ] Add Temporal SDK dependency
- [ ] Add Temporal configuration to config.go
- [ ] Create temporal/client.go factory

### Phase 2: Workflow & Activities
- [ ] Create internal/workflows/email.go
- [ ] Create internal/activities/ package
- [ ] Implement FetchSource activity
- [ ] Implement GenerateEmbedding activity
- [ ] Implement GenerateSummary activity (with heartbeat)
- [ ] Implement ExtractAssertions activity (with heartbeat)
- [ ] Implement UpdateSourceStatus activity

### Phase 3: Worker Process
- [ ] Create cmd/worker/main.go
- [ ] Register workflows and activities
- [ ] Add graceful shutdown
- [ ] Add health check for Temporal connection

### Phase 4: Bridge Integration
- [ ] Modify cmd/pipeline/main.go to start workflows
- [ ] Update event handler to be workflow starter
- [ ] Add Temporal client initialization to main

### Phase 5: Testing & Validation
- [ ] Unit tests for workflows (Temporal test framework)
- [ ] Integration test with local Temporal
- [ ] Reprocess the 269 emails through Temporal
- [ ] Verify all results in database

## Verification Steps

1. **Start Temporal server:**
   ```bash
   docker-compose -f docker-compose.temporal.yml up -d
   ```

2. **Verify Temporal UI:**
   Open http://localhost:8088

3. **Start worker:**
   ```bash
   go run ./cmd/worker
   ```

4. **Start workflow starter (pipeline):**
   ```bash
   go run ./cmd/pipeline
   ```

5. **Trigger processing:**
   ```bash
   penf ingest email test.eml --source test
   ```

6. **Monitor in Temporal UI:**
   - See workflow execution
   - View activity history
   - Check retry attempts

7. **Verify results:**
   ```sql
   SELECT result_type, COUNT(*) FROM processing_results GROUP BY result_type;
   SELECT COUNT(*) FROM embeddings;
   ```

## Future Extensions

Once Temporal is working, the architecture supports:

1. **Additional preprocessing steps:**
   ```go
   // Just add activities to the workflow
   workflow.ExecuteActivity(ctx, "ExtractEntities", ...)
   workflow.ExecuteActivity(ctx, "ClassifyContent", ...)
   workflow.ExecuteActivity(ctx, "DetectLanguage", ...)
   ```

2. **Parallel processing:**
   ```go
   // Run embedding and summary in parallel
   embeddingFuture := workflow.ExecuteActivity(ctx, "GenerateEmbedding", ...)
   summaryFuture := workflow.ExecuteActivity(ctx, "GenerateSummary", ...)

   embeddingFuture.Get(ctx, &embeddingID)
   summaryFuture.Get(ctx, &summaryID)
   ```

3. **Conditional logic:**
   ```go
   if isComplexEmail(source) {
       workflow.ExecuteActivity(ctx, "GenerateDetailedSummary", ...)
   }
   ```

4. **Child workflows:**
   ```go
   // Spawn child workflow for attachment processing
   workflow.ExecuteChildWorkflow(ctx, AttachmentProcessingWorkflow, attachment)
   ```

5. **Saga pattern for compensation:**
   ```go
   // If LLM fails, clean up partial results
   defer func() {
       if err != nil {
           workflow.ExecuteActivity(ctx, "CleanupPartialResults", ...)
       }
   }()
   ```

## Files to Modify/Create

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Modify | Add Temporal SDK |
| `internal/config/config.go` | Modify | Add TemporalConfig |
| `internal/temporal/client.go` | Create | Temporal client factory |
| `internal/workflows/email.go` | Create | EmailProcessingWorkflow |
| `internal/activities/activities.go` | Create | Activities struct |
| `internal/activities/source.go` | Create | FetchSource activity |
| `internal/activities/embedding.go` | Create | GenerateEmbedding activity |
| `internal/activities/llm.go` | Create | LLM activities with heartbeat |
| `internal/activities/storage.go` | Create | Storage activities |
| `cmd/worker/main.go` | Create | Worker entry point |
| `cmd/pipeline/main.go` | Modify | Workflow starter |
| `docker-compose.temporal.yml` | Create | Temporal server |
| `temporal-config/dynamic_config.yaml` | Create | Temporal config |
| `Makefile` | Modify | Add temporal targets |

## Resource Requirements

| Component | Memory | CPU | Notes |
|-----------|--------|-----|-------|
| Temporal Server | ~500MB | Low | PostgreSQL backend |
| Temporal UI | ~100MB | Low | Web interface |
| Worker Process | ~50MB | Low | Polls task queue |
| Workflow Starter | ~20MB | Low | Existing pipeline |

Total additional: ~670MB (fits well on 32GB Mac Mini)
