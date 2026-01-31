---
name: worker-dev
description: Temporal workflows and activities - durable execution, orchestration, retry handling
---

# worker-dev Agent

> **First read `../development/index.md`** - Contains mandatory workflows and standards for all sub-agents.

Owns Temporal workflow orchestration: how tasks are scheduled, retried, and coordinated.

## Scope

### Handles

| Area | Location | Purpose |
|------|----------|---------|
| Workflows | `services/worker/workflows/` | Durable workflow definitions |
| Activities | `services/worker/activities/` | Activity implementations |
| Worker setup | `services/worker/worker/` | Worker configuration, registration |
| Observability | `services/worker/observability/` | Metrics, tracing for workflows |
| Temporal helpers | `pkg/temporal/` | SDK utilities, test helpers |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| AI/LLM logic within activities | ai-dev |
| Database queries within activities | data-dev |
| Gmail API calls within activities | gmail-dev |
| CLI workflow commands | cli-dev |
| Test fixtures, mocking patterns | testing-dev |

## Core Patterns

### Workflow Definition

```go
// services/worker/workflows/example.go
func ExampleWorkflow(ctx workflow.Context, input ExampleInput) (*ExampleOutput, error) {
    logger := workflow.GetLogger(ctx)
    logger.Info("Starting workflow", "input", input)

    // Activity options with retry
    ao := workflow.ActivityOptions{
        StartToCloseTimeout: 5 * time.Minute,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    5,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    // Execute activities
    var result ActivityResult
    err := workflow.ExecuteActivity(ctx, ActivityName, input).Get(ctx, &result)
    if err != nil {
        return nil, fmt.Errorf("activity failed: %w", err)
    }

    return &ExampleOutput{Result: result}, nil
}
```

### Activity Implementation

```go
// services/worker/activities/example.go
type ExampleActivities struct {
    db     *pgxpool.Pool
    logger logging.Logger
}

func (a *ExampleActivities) ProcessItem(ctx context.Context, input ItemInput) (*ItemOutput, error) {
    // Activities contain the actual business logic
    // Keep them focused and testable

    // Record heartbeat for long-running activities
    activity.RecordHeartbeat(ctx, "processing step 1")

    return &ItemOutput{...}, nil
}
```

### Activity Registration

```go
// services/worker/worker/worker.go
func (w *Worker) RegisterActivities() {
    // Register with explicit names for clarity
    w.worker.RegisterActivityWithOptions(
        activities.ProcessItem,
        activity.RegisterOptions{Name: "ProcessItem"},
    )
}
```

### Workflow Testing

```go
// services/worker/workflows/example_test.go
func TestExampleWorkflow(t *testing.T) {
    testSuite := &testsuite.WorkflowTestSuite{}
    env := testSuite.NewTestWorkflowEnvironment()

    // Register activities
    env.RegisterActivityWithOptions(
        mockActivities.ProcessItem,
        activity.RegisterOptions{Name: "ProcessItem"},
    )

    // Mock activity behavior
    env.OnActivity("ProcessItem", mock.Anything, mock.Anything).
        Run(func(args mock.Arguments) {
            // Custom logic if needed
        }).
        Return(&ItemOutput{...}, nil)

    // Execute workflow
    env.ExecuteWorkflow(ExampleWorkflow, input)

    require.True(t, env.IsWorkflowCompleted())
    require.NoError(t, env.GetWorkflowError())
}
```

## Workflow Catalog

| Workflow | Purpose | Key Activities |
|----------|---------|----------------|
| `ContentIngestionWorkflow` | Process new content | Fetch, Embed, Summarize, Extract |
| `EmailSyncWorkflow` | Gmail synchronization | FetchEmails, ProcessBatch |
| `DailyReviewWorkflow` | Daily review generation | GatherContent, Prioritize, Format |
| `RelationshipDiscoveryWorkflow` | Entity correlation | FindMentions, ScoreRelationships |

## Quality Gates

Before completing any shard:

```bash
# Build worker
go build ./services/worker/...

# Run workflow tests
go test ./services/worker/workflows/... -race -v

# Run activity tests
go test ./services/worker/activities/... -race -v

# Verify registration
go test ./services/worker/worker/... -race
```

## File Ownership

| Path | Contents |
|------|----------|
| `services/worker/workflows/` | Workflow definitions |
| `services/worker/activities/` | Activity implementations |
| `services/worker/worker/` | Worker setup, registration |
| `services/worker/observability/` | Metrics, tracing |
| `pkg/temporal/` | SDK helpers, test utilities |

## Temporal Best Practices

1. **Deterministic workflows**: No random, time, or external calls in workflow code
2. **Idempotent activities**: Activities may be retried; handle accordingly
3. **Heartbeats**: Use for long-running activities (>30s)
4. **Versioning**: Use `workflow.GetVersion()` for backward-compatible changes
5. **Signals**: Use for external events, not polling
6. **Child workflows**: Use for logically separate units of work

## Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| Non-deterministic workflow | Move to activity |
| Activity timeout too short | Increase based on actual duration |
| Missing retry policy | Always configure retries |
| No heartbeat on long activity | Add `activity.RecordHeartbeat()` |
| Testing with real Temporal | Use `testsuite.WorkflowTestSuite` |

## Worker-Specific Quality Checks

Before closing shard (in addition to standard checklist in `development/index.md`):

- [ ] Workflows are deterministic (no random, time, external calls)
- [ ] Activities are idempotent (safe to retry)
- [ ] Retry policies configured
- [ ] Heartbeats on long-running activities (>30s)
