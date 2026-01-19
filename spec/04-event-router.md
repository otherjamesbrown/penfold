# Event Router Specification

## Overview

The Event Router is the central event orchestration service. It subscribes to Redis pub-sub channels, manages event routing to appropriate handlers, and orchestrates the job processing state machine.

## Responsibilities

1. **Event Subscription**: Subscribe to all Redis event channels
2. **Event Routing**: Direct events to appropriate processing services
3. **Job Orchestration**: Manage processing job lifecycle
4. **Retry Management**: Handle failed jobs with exponential backoff
5. **Dead Letter Queue**: Capture permanently failed events
6. **Event Persistence**: PostgreSQL fallback for reliability
7. **Metrics**: Event throughput, latency, failure rates

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Event Router                                   │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                    Redis Subscriber                             │    │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │    │
│  │  │content.* │ │ email.*  │ │relation.*│ │  job.*   │          │    │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘          │    │
│  └───────┼────────────┼────────────┼────────────┼─────────────────┘    │
│          │            │            │            │                       │
│          └────────────┴────────────┴────────────┘                       │
│                              │                                          │
│                              ▼                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                    Event Dispatcher                             │    │
│  │                                                                 │    │
│  │   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐       │    │
│  │   │  Filter &   │───▶│   Route &   │───▶│   Dispatch  │       │    │
│  │   │  Validate   │    │   Enrich    │    │   to Service│       │    │
│  │   └─────────────┘    └─────────────┘    └─────────────┘       │    │
│  └────────────────────────────────────────────────────────────────┘    │
│                              │                                          │
│          ┌───────────────────┼───────────────────┐                     │
│          ▼                   ▼                   ▼                      │
│  ┌───────────────┐   ┌───────────────┐   ┌───────────────┐            │
│  │   Job Queue   │   │  Retry Queue  │   │  Dead Letter  │            │
│  │               │   │               │   │    Queue      │            │
│  └───────┬───────┘   └───────┬───────┘   └───────────────┘            │
│          │                   │                                          │
│          └───────────────────┘                                          │
│                    │                                                    │
│                    ▼                                                    │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                    Job Manager                                  │    │
│  │                                                                 │    │
│  │   State Machine: QUEUED → CLAIMED → IN_PROGRESS → COMPLETED    │    │
│  │                           ↓              ↓                      │    │
│  │                        FAILED ← ← ←  RETRYING                  │    │
│  └────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
            │                                      │
            ▼                                      ▼
    ┌───────────────┐                      ┌───────────────┐
    │  PostgreSQL   │                      │    Redis      │
    │  (jobs, logs) │                      │   (pub-sub)   │
    └───────────────┘                      └───────────────┘
```

## Event Types

### Content Events
```protobuf
// Events published by content ingestion

message ContentIngestedEvent {
  string event_id = 1;
  string tenant_id = 2;
  string source_id = 3;
  string source_type = 4;  // email, meeting, document
  string content = 5;
  map<string, string> metadata = 6;
  google.protobuf.Timestamp timestamp = 7;
}

message ContentProcessedEvent {
  string event_id = 1;
  string source_id = 2;
  ProcessingResult result = 3;
  repeated string job_ids = 4;
}
```

### Email Events
```protobuf
message EmailIngestedEvent {
  string event_id = 1;
  string tenant_id = 2;
  string message_id = 3;
  string thread_id = 4;
  string account_email = 5;
  string subject = 6;
  string snippet = 7;
  repeated string labels = 8;
  EmailParticipants participants = 9;
  google.protobuf.Timestamp received_at = 10;
}

message EmailThreadIngestedEvent {
  string event_id = 1;
  string tenant_id = 2;
  string thread_id = 3;
  repeated string message_ids = 4;
  int32 message_count = 5;
}
```

### Relationship Events
```protobuf
message RelationshipDiscoveredEvent {
  string event_id = 1;
  string tenant_id = 2;
  string relationship_id = 3;
  string source_entity_id = 4;
  string target_entity_id = 5;
  string relationship_type = 6;
  float confidence = 7;
  repeated string evidence_ids = 8;
}

message RelationshipValidatedEvent {
  string event_id = 1;
  string relationship_id = 2;
  bool confirmed = 3;
  string user_id = 4;
  string feedback = 5;
}
```

### Job Events
```protobuf
message JobCreatedEvent {
  string job_id = 1;
  string tenant_id = 2;
  string job_type = 3;
  string source_id = 4;
  JobPriority priority = 5;
}

message JobCompletedEvent {
  string job_id = 1;
  bool success = 2;
  string result = 3;
  int64 duration_ms = 4;
}

message JobFailedEvent {
  string job_id = 1;
  string error = 2;
  int32 retry_count = 3;
  bool will_retry = 4;
}
```

## Event Subscription

```go
// internal/subscriber/subscriber.go

type EventSubscriber struct {
    redis      *redis.Client
    dispatcher *Dispatcher
    channels   []string
}

var eventChannels = []string{
    "events:content.ingested",
    "events:content.processed",
    "events:email.ingested",
    "events:email.thread_ingested",
    "events:email.attachment_ingested",
    "events:manual_email.ingested",
    "events:relationship.discovered",
    "events:relationship.validated",
    "events:job.created",
    "events:job.completed",
    "events:job.failed",
    "events:sync.progress",
    "events:sync.completed",
}

func (s *EventSubscriber) Start(ctx context.Context) error {
    pubsub := s.redis.PSubscribe(ctx, "events:*")
    defer pubsub.Close()

    ch := pubsub.Channel()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case msg := <-ch:
            if err := s.handleMessage(ctx, msg); err != nil {
                slog.Error("failed to handle message",
                    "channel", msg.Channel,
                    "error", err,
                )
            }
        }
    }
}

func (s *EventSubscriber) handleMessage(ctx context.Context, msg *redis.Message) error {
    // Parse event type from channel
    eventType := strings.TrimPrefix(msg.Channel, "events:")

    // Parse event payload
    var event BaseEvent
    if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
        return fmt.Errorf("failed to unmarshal event: %w", err)
    }

    // Add event type
    event.Type = eventType

    // Persist to PostgreSQL for durability
    if err := s.persistEvent(ctx, &event); err != nil {
        slog.Warn("failed to persist event", "event_id", event.ID, "error", err)
    }

    // Dispatch to appropriate handler
    return s.dispatcher.Dispatch(ctx, &event)
}
```

## Event Dispatcher

```go
// internal/dispatcher/dispatcher.go

type Dispatcher struct {
    handlers map[string][]EventHandler
    jobMgr   *JobManager
}

type EventHandler interface {
    Handle(ctx context.Context, event *BaseEvent) error
}

func (d *Dispatcher) RegisterHandler(eventType string, handler EventHandler) {
    d.handlers[eventType] = append(d.handlers[eventType], handler)
}

func (d *Dispatcher) Dispatch(ctx context.Context, event *BaseEvent) error {
    handlers, ok := d.handlers[event.Type]
    if !ok {
        slog.Debug("no handlers for event type", "type", event.Type)
        return nil
    }

    // Fan-out to all registered handlers
    var wg sync.WaitGroup
    errors := make(chan error, len(handlers))

    for _, handler := range handlers {
        wg.Add(1)
        go func(h EventHandler) {
            defer wg.Done()
            if err := h.Handle(ctx, event); err != nil {
                errors <- fmt.Errorf("handler failed: %w", err)
            }
        }(handler)
    }

    wg.Wait()
    close(errors)

    // Collect errors
    var errs []error
    for err := range errors {
        errs = append(errs, err)
    }

    if len(errs) > 0 {
        return fmt.Errorf("dispatch errors: %v", errs)
    }

    return nil
}
```

## Job Manager

```go
// internal/jobs/manager.go

type JobManager struct {
    db           *pgxpool.Pool
    redis        *redis.Client
    workerPools  map[string]*WorkerPool
}

type JobState string

const (
    JobQueued     JobState = "QUEUED"
    JobClaimed    JobState = "CLAIMED"
    JobInProgress JobState = "IN_PROGRESS"
    JobCompleted  JobState = "COMPLETED"
    JobFailed     JobState = "FAILED"
    JobRetrying   JobState = "RETRYING"
    JobCancelled  JobState = "CANCELLED"
)

type Job struct {
    ID          string
    TenantID    string
    Type        string
    SourceID    string
    State       JobState
    Priority    int
    Payload     json.RawMessage
    RetryCount  int
    MaxRetries  int
    CreatedAt   time.Time
    UpdatedAt   time.Time
    CompletedAt *time.Time
    Error       *string
}

func (m *JobManager) CreateJob(ctx context.Context, job *Job) error {
    job.ID = uuid.New().String()
    job.State = JobQueued
    job.CreatedAt = time.Now()
    job.UpdatedAt = time.Now()
    job.MaxRetries = 5

    query := `
        INSERT INTO processing_jobs (
            id, tenant_id, job_type, source_id, state, priority,
            payload, retry_count, max_retries, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `

    _, err := m.db.Exec(ctx, query,
        job.ID, job.TenantID, job.Type, job.SourceID, job.State, job.Priority,
        job.Payload, job.RetryCount, job.MaxRetries, job.CreatedAt, job.UpdatedAt,
    )
    if err != nil {
        return fmt.Errorf("failed to create job: %w", err)
    }

    // Publish job available event
    event := JobCreatedEvent{
        JobID:    job.ID,
        TenantID: job.TenantID,
        JobType:  job.Type,
        SourceID: job.SourceID,
        Priority: int32(job.Priority),
    }
    return m.publishEvent(ctx, "job.created", &event)
}

func (m *JobManager) ClaimJob(ctx context.Context, workerID string, jobTypes []string) (*Job, error) {
    // Atomic claim using FOR UPDATE SKIP LOCKED
    query := `
        UPDATE processing_jobs
        SET state = $1, updated_at = $2, claimed_by = $3
        WHERE id = (
            SELECT id FROM processing_jobs
            WHERE state = $4 AND job_type = ANY($5)
            ORDER BY priority DESC, created_at ASC
            FOR UPDATE SKIP LOCKED
            LIMIT 1
        )
        RETURNING id, tenant_id, job_type, source_id, state, priority,
                  payload, retry_count, max_retries, created_at, updated_at
    `

    var job Job
    err := m.db.QueryRow(ctx, query,
        JobClaimed, time.Now(), workerID, JobQueued, jobTypes,
    ).Scan(
        &job.ID, &job.TenantID, &job.Type, &job.SourceID, &job.State, &job.Priority,
        &job.Payload, &job.RetryCount, &job.MaxRetries, &job.CreatedAt, &job.UpdatedAt,
    )

    if err == pgx.ErrNoRows {
        return nil, nil // No jobs available
    }
    if err != nil {
        return nil, fmt.Errorf("failed to claim job: %w", err)
    }

    return &job, nil
}

func (m *JobManager) CompleteJob(ctx context.Context, jobID string, result json.RawMessage) error {
    now := time.Now()
    query := `
        UPDATE processing_jobs
        SET state = $1, updated_at = $2, completed_at = $3, result = $4
        WHERE id = $5
    `

    _, err := m.db.Exec(ctx, query, JobCompleted, now, now, result, jobID)
    if err != nil {
        return fmt.Errorf("failed to complete job: %w", err)
    }

    // Publish completion event
    return m.publishEvent(ctx, "job.completed", &JobCompletedEvent{
        JobID:   jobID,
        Success: true,
        Result:  string(result),
    })
}

func (m *JobManager) FailJob(ctx context.Context, jobID string, err error) error {
    // Get current retry count
    var job Job
    query := `SELECT retry_count, max_retries FROM processing_jobs WHERE id = $1`
    if err := m.db.QueryRow(ctx, query, jobID).Scan(&job.RetryCount, &job.MaxRetries); err != nil {
        return fmt.Errorf("failed to get job: %w", err)
    }

    willRetry := job.RetryCount < job.MaxRetries
    newState := JobFailed
    if willRetry {
        newState = JobRetrying
    }

    errStr := err.Error()
    updateQuery := `
        UPDATE processing_jobs
        SET state = $1, updated_at = $2, error = $3, retry_count = retry_count + 1
        WHERE id = $4
    `
    if _, err := m.db.Exec(ctx, updateQuery, newState, time.Now(), errStr, jobID); err != nil {
        return fmt.Errorf("failed to update job: %w", err)
    }

    // Schedule retry with exponential backoff
    if willRetry {
        backoff := time.Duration(math.Pow(2, float64(job.RetryCount))) * time.Second
        m.scheduleRetry(ctx, jobID, backoff)
    }

    return m.publishEvent(ctx, "job.failed", &JobFailedEvent{
        JobID:      jobID,
        Error:      errStr,
        RetryCount: int32(job.RetryCount + 1),
        WillRetry:  willRetry,
    })
}
```

## Routing Configuration

```go
// internal/routing/routes.go

type Route struct {
    EventType   string
    ServiceAddr string
    JobType     string
    Priority    int
}

var routes = []Route{
    // Content processing
    {
        EventType:   "content.ingested",
        ServiceAddr: "localhost:8083",  // Content Processor
        JobType:     "content_process",
        Priority:    100,
    },
    {
        EventType:   "email.ingested",
        ServiceAddr: "localhost:8001",  // Embedding Pipeline
        JobType:     "embedding",
        Priority:    90,
    },
    {
        EventType:   "email.ingested",
        ServiceAddr: "localhost:8085",  // AI Coordinator
        JobType:     "entity_extraction",
        Priority:    80,
    },

    // Relationship discovery
    {
        EventType:   "content.processed",
        ServiceAddr: "localhost:8086",  // Relationship Discovery
        JobType:     "relationship_discovery",
        Priority:    70,
    },

    // Manual ingest
    {
        EventType:   "manual_email.ingested",
        ServiceAddr: "localhost:8001",  // Embedding Pipeline
        JobType:     "embedding",
        Priority:    90,
    },
    {
        EventType:   "manual_email.ingested",
        ServiceAddr: "localhost:8085",  // AI Coordinator
        JobType:     "entity_extraction",
        Priority:    80,
    },
}
```

## Dead Letter Queue

```go
// internal/dlq/dlq.go

type DeadLetterQueue struct {
    db *pgxpool.Pool
}

func (d *DeadLetterQueue) Push(ctx context.Context, job *Job, err error) error {
    query := `
        INSERT INTO dead_letter_queue (
            job_id, tenant_id, job_type, source_id, payload,
            error, retry_count, created_at, failed_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `

    _, dbErr := d.db.Exec(ctx, query,
        job.ID, job.TenantID, job.Type, job.SourceID, job.Payload,
        err.Error(), job.RetryCount, job.CreatedAt, time.Now(),
    )
    return dbErr
}

func (d *DeadLetterQueue) List(ctx context.Context, tenantID string, limit int) ([]*DeadJob, error) {
    query := `
        SELECT job_id, job_type, source_id, error, retry_count, failed_at
        FROM dead_letter_queue
        WHERE tenant_id = $1
        ORDER BY failed_at DESC
        LIMIT $2
    `

    rows, err := d.db.Query(ctx, query, tenantID, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var jobs []*DeadJob
    for rows.Next() {
        var job DeadJob
        if err := rows.Scan(&job.JobID, &job.JobType, &job.SourceID, &job.Error, &job.RetryCount, &job.FailedAt); err != nil {
            return nil, err
        }
        jobs = append(jobs, &job)
    }
    return jobs, nil
}

func (d *DeadLetterQueue) Replay(ctx context.Context, jobID string) error {
    // Move from DLQ back to processing_jobs with reset retry count
    query := `
        WITH moved AS (
            DELETE FROM dead_letter_queue WHERE job_id = $1
            RETURNING job_id, tenant_id, job_type, source_id, payload, created_at
        )
        INSERT INTO processing_jobs (id, tenant_id, job_type, source_id, payload, state, retry_count, created_at, updated_at)
        SELECT job_id, tenant_id, job_type, source_id, payload, 'QUEUED', 0, created_at, NOW()
        FROM moved
    `
    _, err := d.db.Exec(ctx, query, jobID)
    return err
}
```

## Configuration

```yaml
# config/event-router.yaml

server:
  grpc_port: 8090
  metrics_port: 9091

redis:
  address: "home-01:6379"
  pool_size: 10

database:
  host: "home-01"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

routing:
  default_priority: 50
  max_retries: 5
  retry_base_delay: "1s"
  retry_max_delay: "5m"

workers:
  embedding:
    concurrency: 4
    service_addr: "localhost:8001"
  entity_extraction:
    concurrency: 2
    service_addr: "localhost:8085"
  relationship:
    concurrency: 2
    service_addr: "localhost:8086"

dlq:
  enabled: true
  alert_threshold: 100  # Alert if DLQ size exceeds this

logging:
  level: "info"
  format: "json"
```

## gRPC Service (for management)

```protobuf
// api/proto/eventrouter/v1/eventrouter.proto

syntax = "proto3";
package eventrouter.v1;

service EventRouterService {
  // Job management
  rpc GetJob(GetJobRequest) returns (Job);
  rpc ListJobs(ListJobsRequest) returns (ListJobsResponse);
  rpc CancelJob(CancelJobRequest) returns (CancelJobResponse);
  rpc RetryJob(RetryJobRequest) returns (RetryJobResponse);

  // DLQ management
  rpc ListDeadLetterJobs(ListDeadLetterJobsRequest) returns (ListDeadLetterJobsResponse);
  rpc ReplayDeadLetterJob(ReplayDeadLetterJobRequest) returns (ReplayDeadLetterJobResponse);
  rpc PurgeDeadLetterQueue(PurgeDeadLetterQueueRequest) returns (PurgeDeadLetterQueueResponse);

  // Metrics
  rpc GetStats(GetStatsRequest) returns (GetStatsResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

message GetStatsResponse {
  int64 jobs_queued = 1;
  int64 jobs_in_progress = 2;
  int64 jobs_completed_24h = 3;
  int64 jobs_failed_24h = 4;
  int64 dlq_size = 5;
  map<string, int64> jobs_by_type = 6;
  double avg_processing_time_ms = 7;
}
```

## Implementation Structure

```
services/event-router/
├── cmd/
│   └── event-router/
│       └── main.go
├── internal/
│   ├── subscriber/
│   │   └── subscriber.go
│   ├── dispatcher/
│   │   └── dispatcher.go
│   ├── jobs/
│   │   ├── manager.go
│   │   └── state.go
│   ├── routing/
│   │   └── routes.go
│   ├── dlq/
│   │   └── dlq.go
│   ├── workers/
│   │   └── pool.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── eventrouter/
│           └── v1/
│               └── eventrouter.proto
└── go.mod
```
