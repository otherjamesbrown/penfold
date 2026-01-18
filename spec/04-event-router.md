# Workflow Orchestrator Specification

## Overview

The Workflow Orchestrator is the central processing coordination service built on Temporal. It manages multi-step AI processing workflows with proper backpressure, retry handling, and observability. This replaces Redis pub/sub with Temporal's durable workflow execution.

## Responsibilities

1. **Workflow Execution**: Coordinate multi-step AI processing pipelines
2. **Activity Management**: Execute discrete processing steps with proper timeouts
3. **Retry Management**: Built-in retry policies with exponential backoff
4. **Heartbeat Monitoring**: Track long-running LLM operations
5. **Backpressure**: Task queue with persistence prevents message loss
6. **Observability**: Full execution history visible in Temporal UI
7. **Metrics**: Workflow throughput, latency, failure rates

## Architecture

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
│ (CLI/Gateway   │    │  │ Fetch      │  │ Generate   │  │ Generate        │  │
│  triggers)     │    │  │ Source     │  │ Embedding  │  │ Summary         │  │
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
               :8087                :8000
```

## Why Temporal over Redis Pub/Sub

| Problem with Redis Pub/Sub | Temporal Solution |
|---------------------------|-------------------|
| Messages dropped when buffer full | Task queue with persistence |
| No visibility into processing state | Full execution history in Web UI |
| Manual retry/circuit breaker logic | Built-in retry policies |
| No dead letter handling | Failed workflows visible & retryable |
| Difficult to add preprocessing steps | Workflows as code - easy to extend |

## Workflow Definitions

### Email Processing Workflow

```protobuf
// api/proto/workflow/v1/workflow.proto

syntax = "proto3";
package workflow.v1;

import "google/protobuf/timestamp.proto";

// Workflow input for email processing
message EmailProcessingInput {
  string tenant_id = 1;
  int64 source_id = 2;
  string message_id = 3;
  string thread_id = 4;
  string from_email = 5;
  optional string from_name = 6;
  optional string subject = 7;
  repeated string to_emails = 8;
  repeated string cc_emails = 9;
  google.protobuf.Timestamp email_date = 10;
  string content_hash = 11;
  string job_id = 12;
}

// Workflow result
message EmailProcessingResult {
  int64 source_id = 1;
  optional int64 embedding_id = 2;
  optional int64 summary_id = 3;
  int32 assertion_count = 4;
  string status = 5;  // completed, failed
  optional string error = 6;
}

// Content processing workflow input
message ContentProcessingInput {
  string tenant_id = 1;
  string source_id = 2;
  string source_type = 3;  // email, meeting, document
  string content = 4;
  map<string, string> metadata = 5;
}

// Relationship discovery workflow input
message RelationshipDiscoveryInput {
  string tenant_id = 1;
  string source_id = 2;
  repeated string entity_ids = 3;
}
```

### Activity Messages

```protobuf
// Activity inputs and outputs

message FetchSourceInput {
  string tenant_id = 1;
  int64 source_id = 2;
}

message FetchSourceOutput {
  string content_text = 1;
  map<string, string> metadata = 2;
}

message GenerateEmbeddingInput {
  string tenant_id = 1;
  int64 source_id = 2;
  string content = 3;
  string content_hash = 4;
}

message GenerateSummaryInput {
  string tenant_id = 1;
  int64 source_id = 2;
  string job_id = 3;
  string content = 4;
}

message ExtractAssertionsInput {
  string tenant_id = 1;
  int64 source_id = 2;
  string job_id = 3;
  string content = 4;
}
```

## Workflow Implementation

```go
// internal/workflows/email.go

package workflows

import (
    "time"
    "go.temporal.io/sdk/workflow"
    "go.temporal.io/sdk/temporal"
)

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

## Activity Implementations

```go
// internal/activities/activities.go

package activities

import (
    "context"
    "github.com/otherjamesbrown/penfold/pkg/db"
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

func (a *Activities) ExtractAssertions(ctx context.Context, input ExtractAssertionsInput) (int, error) {
    logger := activity.GetLogger(ctx)
    logger.Info("Extracting assertions", "source_id", input.SourceID)

    activity.RecordHeartbeat(ctx, "starting_assertion_extraction")

    assertions, err := a.llmClient.ExtractAssertions(ctx, input.Content)
    if err != nil {
        return 0, fmt.Errorf("assertion extraction failed: %w", err)
    }

    activity.RecordHeartbeat(ctx, "storing_assertions")

    // Store each assertion
    for _, assertion := range assertions {
        if err := a.resultRepo.StoreAssertion(ctx, input.TenantID, input.SourceID, assertion); err != nil {
            logger.Warn("Failed to store assertion", "error", err)
        }
    }

    return len(assertions), nil
}
```

## Temporal Worker

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

    "github.com/otherjamesbrown/penfold/internal/activities"
    "github.com/otherjamesbrown/penfold/internal/clients"
    "github.com/otherjamesbrown/penfold/internal/config"
    "github.com/otherjamesbrown/penfold/internal/storage"
    "github.com/otherjamesbrown/penfold/internal/workflows"
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

    // Initialize dependencies
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

    // Create worker with concurrency limit for LLM operations
    w := worker.New(c, TaskQueue, worker.Options{
        MaxConcurrentActivityExecutionSize: 4,  // Limit concurrent LLM calls
    })

    // Register workflows
    w.RegisterWorkflow(workflows.EmailProcessingWorkflow)
    w.RegisterWorkflow(workflows.ContentProcessingWorkflow)
    w.RegisterWorkflow(workflows.RelationshipDiscoveryWorkflow)

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

## Workflow Starter

```go
// internal/temporal/starter.go

package temporal

import (
    "context"
    "fmt"

    "go.temporal.io/sdk/client"
    "github.com/otherjamesbrown/penfold/internal/workflows"
)

const TaskQueue = "penfold-ai-processing"

type WorkflowStarter struct {
    client client.Client
}

func NewWorkflowStarter(c client.Client) *WorkflowStarter {
    return &WorkflowStarter{client: c}
}

// StartEmailProcessing triggers the email processing workflow
func (s *WorkflowStarter) StartEmailProcessing(ctx context.Context, input workflows.EmailProcessingInput) (string, error) {
    workflowOptions := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("email-processing-%d", input.SourceID),
        TaskQueue: TaskQueue,
    }

    we, err := s.client.ExecuteWorkflow(ctx, workflowOptions, workflows.EmailProcessingWorkflow, input)
    if err != nil {
        return "", fmt.Errorf("failed to start workflow: %w", err)
    }

    return we.GetID(), nil
}

// StartContentProcessing triggers the content processing workflow
func (s *WorkflowStarter) StartContentProcessing(ctx context.Context, input workflows.ContentProcessingInput) (string, error) {
    workflowOptions := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("content-processing-%s", input.SourceID),
        TaskQueue: TaskQueue,
    }

    we, err := s.client.ExecuteWorkflow(ctx, workflowOptions, workflows.ContentProcessingWorkflow, input)
    if err != nil {
        return "", fmt.Errorf("failed to start workflow: %w", err)
    }

    return we.GetID(), nil
}

// StartRelationshipDiscovery triggers the relationship discovery workflow
func (s *WorkflowStarter) StartRelationshipDiscovery(ctx context.Context, input workflows.RelationshipDiscoveryInput) (string, error) {
    workflowOptions := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("relationship-discovery-%s", input.SourceID),
        TaskQueue: TaskQueue,
    }

    we, err := s.client.ExecuteWorkflow(ctx, workflowOptions, workflows.RelationshipDiscoveryWorkflow, input)
    if err != nil {
        return "", fmt.Errorf("failed to start workflow: %w", err)
    }

    return we.GetID(), nil
}
```

## Failure Handling

Temporal provides built-in failure handling with full visibility:

### Retry Policies

```go
// Activity retry policies are defined per activity type

// Fast operations (DB queries): 3 retries, 1s initial, 2x backoff
fastOpts := workflow.ActivityOptions{
    StartToCloseTimeout: 30 * time.Second,
    RetryPolicy: &temporal.RetryPolicy{
        InitialInterval:    time.Second,
        BackoffCoefficient: 2.0,
        MaximumAttempts:    3,
    },
}

// LLM operations: 2 retries, 5s initial, 2x backoff (expensive, limit retries)
llmOpts := workflow.ActivityOptions{
    StartToCloseTimeout:    2 * time.Minute,
    ScheduleToCloseTimeout: 5 * time.Minute,
    HeartbeatTimeout:       15 * time.Second,
    RetryPolicy: &temporal.RetryPolicy{
        InitialInterval:    5 * time.Second,
        BackoffCoefficient: 2.0,
        MaximumAttempts:    2,
    },
}
```

### Failed Workflow Management

Failed workflows are visible in Temporal UI (http://localhost:8088):
- **Full execution history**: See exactly where each workflow failed
- **Retry from failure point**: Re-execute failed workflows
- **Query failed workflows**: Filter by status, time range, workflow type

```bash
# List failed workflows using temporal CLI
temporal workflow list --query "ExecutionStatus='Failed'"

# Retry a failed workflow
temporal workflow reset --workflow-id "email-processing-123" --reason "retry after fix"

# Terminate a stuck workflow
temporal workflow terminate --workflow-id "email-processing-123" --reason "stuck"
```

### Error Classification

```go
// Non-retryable errors (don't retry these)
var nonRetryableErrors = []string{
    "*NotFoundError",           // Source doesn't exist
    "*ValidationError",         // Invalid input
    "*AuthenticationError",     // Bad credentials
}

// Apply to activity options
llmOpts := workflow.ActivityOptions{
    // ...
    RetryPolicy: &temporal.RetryPolicy{
        // ...
        NonRetryableErrorTypes: nonRetryableErrors,
    },
}
```

## Configuration

```yaml
# config/workflow-orchestrator.yaml

server:
  grpc_port: 8090
  metrics_port: 9091

temporal:
  host_port: "localhost:7233"
  namespace: "default"
  task_queue: "penfold-ai-processing"

database:
  host: "home-01"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

worker:
  max_concurrent_activities: 4  # Limit concurrent LLM calls
  max_concurrent_workflows: 100

activities:
  fast:
    start_to_close_timeout: "30s"
    max_attempts: 3
  embedding:
    start_to_close_timeout: "30s"
    heartbeat_timeout: "10s"
    max_attempts: 3
  llm:
    start_to_close_timeout: "2m"
    schedule_to_close_timeout: "5m"
    heartbeat_timeout: "15s"
    max_attempts: 2

ai:
  embeddings:
    endpoint: "http://localhost:8087"
    model: "sentence-transformers/all-MiniLM-L6-v2"
  llm:
    endpoint: "http://localhost:8000"
    model: "mlx-community/Qwen2.5-7B-Instruct-4bit"

logging:
  level: "info"
  format: "json"
```

### Temporal Server Configuration

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
      - "8088:8080"   # Web UI
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
    depends_on:
      - temporal
```

## gRPC Service (for management)

```protobuf
// api/proto/orchestrator/v1/orchestrator.proto

syntax = "proto3";
package orchestrator.v1;

import "google/protobuf/timestamp.proto";

service WorkflowOrchestratorService {
  // Workflow management
  rpc StartWorkflow(StartWorkflowRequest) returns (StartWorkflowResponse);
  rpc GetWorkflow(GetWorkflowRequest) returns (WorkflowExecution);
  rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse);
  rpc CancelWorkflow(CancelWorkflowRequest) returns (CancelWorkflowResponse);
  rpc RetryWorkflow(RetryWorkflowRequest) returns (RetryWorkflowResponse);

  // Stats and health
  rpc GetStats(GetStatsRequest) returns (GetStatsResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
}

message StartWorkflowRequest {
  string workflow_type = 1;  // email_processing, content_processing, relationship_discovery
  string tenant_id = 2;
  bytes input = 3;  // JSON-encoded workflow input
}

message StartWorkflowResponse {
  string workflow_id = 1;
  string run_id = 2;
}

message WorkflowExecution {
  string workflow_id = 1;
  string run_id = 2;
  string workflow_type = 3;
  string status = 4;  // Running, Completed, Failed, Canceled, Terminated
  google.protobuf.Timestamp start_time = 5;
  google.protobuf.Timestamp close_time = 6;
  bytes result = 7;  // JSON-encoded result if completed
  string error = 8;  // Error message if failed
}

message GetStatsResponse {
  int64 workflows_running = 1;
  int64 workflows_completed_24h = 2;
  int64 workflows_failed_24h = 3;
  int64 activities_pending = 4;
  map<string, int64> workflows_by_type = 5;
  double avg_workflow_duration_ms = 6;
  double avg_activity_duration_ms = 7;
}
```

## Implementation Structure

```
services/workflow-orchestrator/
├── cmd/
│   ├── worker/
│   │   └── main.go           # Temporal worker process
│   └── orchestrator/
│       └── main.go           # gRPC management service
├── internal/
│   ├── workflows/
│   │   ├── email.go          # EmailProcessingWorkflow
│   │   ├── content.go        # ContentProcessingWorkflow
│   │   └── relationship.go   # RelationshipDiscoveryWorkflow
│   ├── activities/
│   │   ├── activities.go     # Activity struct with dependencies
│   │   ├── source.go         # FetchSource activity
│   │   ├── embedding.go      # GenerateEmbedding activity
│   │   ├── llm.go            # LLM activities with heartbeat
│   │   └── storage.go        # Storage activities
│   ├── temporal/
│   │   ├── client.go         # Temporal client factory
│   │   ├── starter.go        # Workflow starter
│   │   └── config.go         # Temporal configuration
│   ├── clients/
│   │   ├── embeddings.go     # Embedding service client
│   │   └── llm.go            # LLM service client
│   ├── storage/
│   │   ├── source.go         # Source repository
│   │   ├── embedding.go      # Embedding repository
│   │   └── result.go         # Processing result repository
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       ├── workflow/
│       │   └── v1/
│       │       └── workflow.proto    # Workflow input/output messages
│       └── orchestrator/
│           └── v1/
│               └── orchestrator.proto # Management gRPC service
├── docker-compose.temporal.yml
├── temporal-config/
│   └── dynamic_config.yaml
└── go.mod
```
