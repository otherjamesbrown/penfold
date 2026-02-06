# ADR: Evaluate tempts for Temporal Workflow Type Safety

**Status:** Decided - Do Not Adopt
**Date:** 2026-02-06
**Shard:** pf-11cd17

## Context

Penfold uses the standard Temporal Go SDK (v1.29.1) for all workflow orchestration. Activity names are string-based, and workflow-activity type contracts are enforced only at runtime via JSON serialization. The `tempts` library (github.com/vikstrous/tempts) wraps the Temporal SDK with Go generics to provide compile-time type safety.

This ADR evaluates whether tempts is suitable for new and/or existing Penfold workflows.

## tempts Overview

tempts provides generic wrappers around core Temporal primitives:

- `Activity[Param, Return]` - type-safe activity declaration and execution
- `Workflow[Param, Return]` - type-safe workflow declaration and execution
- `WorkflowSignal[WP, WR, SP]` - type-safe signals scoped to a workflow
- `QueryHandler[Param, Return]` - type-safe query handlers
- `Queue` - static queue declaration that validates activity/workflow registration
- `Worker` - worker that validates all declared activities/workflows have implementations

## Evaluation

### Pattern 1: Per-Stage Activity Options

**Current approach:**
```go
fastOpts := pkgtemporal.FastActivityOptions()       // 30s timeout
embeddingOpts := pkgtemporal.EmbeddingActivityOptions() // 30s + heartbeat
llmOpts := pkgtemporal.LLMActivityOptions()          // 10min + heartbeat

ctxTriage := workflow.WithActivityOptions(ctx, embeddingOpts)
err := workflow.ExecuteActivity(ctxTriage, "Triage", triageInput).Get(ctx, &triageOutput)
```

**With tempts:**
```go
// tempts hardcodes a 10s default timeout in WithImplementation() and
// always forces the task queue via workflow.WithTaskQueue(). There is
// no way to pass custom ActivityOptions per-call. The Activity.Run()
// method calls workflow.ExecuteActivity with only the task queue set.
ret, err := triageActivity.Run(ctx, triageInput)
// No way to specify embeddingOpts here ^
```

**Verdict: BLOCKER.** Penfold uses three distinct timeout tiers (Fast/Embedding/LLM) with different StartToClose, HeartbeatTimeout, and RetryPolicy settings per activity invocation. tempts has no mechanism for per-call ActivityOptions. The `WithImplementation()` wrapper hardcodes `StartToCloseTimeout: 10 * time.Second` and there is no override path. We would have to fork tempts or fall back to raw `workflow.ExecuteActivity` calls, negating the type-safety benefit.

### Pattern 2: Saga Compensation

**Current approach:**
```go
var compensations []func(workflow.Context) error

// After embedding generation
compensations = append(compensations, func(ctx workflow.Context) error {
    return workflow.ExecuteActivity(
        workflow.WithActivityOptions(ctx, fastOpts),
        "DeleteEmbedding", embeddingID,
    ).Get(ctx, nil)
})

// On failure
for i := len(compensations) - 1; i >= 0; i-- {
    compensations[i](ctx)
}
```

**With tempts:** The compensation pattern works with tempts activities since the compensation functions just call `activity.Run()`. However, compensation activities need their own ActivityOptions (fast timeout for cleanup), which brings us back to the Pattern 1 blocker.

**Verdict: Partially works, but blocked by Pattern 1.**

### Pattern 3: Workflow Signals

**Current approach:**
```go
priorityChan := workflow.GetSignalChannel(ctx, PipelinePrioritySignal)
cancelChan := workflow.GetSignalChannel(ctx, PipelineCancelSignal)

selector := workflow.NewSelector(ctx)
selector.AddReceive(priorityChan, func(c workflow.ReceiveChannel, more bool) {
    var signal pkgtemporal.PriorityUpdateSignal
    c.Receive(ctx, &signal)
    // handle
})
selector.AddReceive(cancelChan, func(c workflow.ReceiveChannel, more bool) {
    var signal pkgtemporal.CancelWithCompensationSignal
    c.Receive(ctx, &signal)
    state.cancelRequested = true
})

// Non-blocking drain
for selector.HasPending() {
    selector.Select(ctx)
}
```

**With tempts:**
```go
var prioritySignal = tempts.NewWorkflowSignal[PriorityUpdateSignal](
    &pipelineWorkflow, "pipeline_priority")
var cancelSignal = tempts.NewWorkflowSignal[CancelWithCompensationSignal](
    &pipelineWorkflow, "pipeline_cancel")

// Selector-based
selector := workflow.NewSelector(ctx)
prioritySignal.AddToSelector(ctx, selector, func(sig PriorityUpdateSignal) {
    // handle
})
cancelSignal.AddToSelector(ctx, selector, func(sig CancelWithCompensationSignal) {
    state.cancelRequested = true
})
```

**Verdict: GOOD.** tempts signals are well-designed. `AddToSelector`, `Receive`, and `TryReceive` cover all our usage patterns. Type safety on signal payloads would catch mismatches at compile time. The non-blocking drain pattern (`HasPending` + `Select`) still works since `AddToSelector` returns the same `workflow.Selector`.

### Pattern 4: Progress Queries

**Current approach:**
```go
workflow.SetQueryHandler(ctx, PipelineStatusQuery, func() (PipelineStatus, error) {
    return state.status, nil
})
```

**With tempts:**
```go
var statusQuery = tempts.NewQueryHandler[struct{}, PipelineStatus]("pipeline_status")

// In workflow
statusQuery.SetHandler(ctx, func(_ struct{}) (PipelineStatus, error) {
    return state.status, nil
})
```

**Verdict: GOOD.** Clean API. Note: tempts forces `Param` to be a struct, so our current zero-arg query `func() (PipelineStatus, error)` would need to become `func(struct{}) (PipelineStatus, error)`. Minor but changes the query signature.

### Pattern 5: Conditional Stage Execution

**Current approach:**
```go
if !triageOutput.SkipDeep {
    ctxExtract := workflow.WithActivityOptions(ctx, embeddingOpts)
    err = workflow.ExecuteActivity(ctxExtract, "ExtractEntitiesActivity", input).Get(ctx, &output)
    // ... stages 3, 4, 4.5 ...
}
```

**With tempts:** Conditional execution works fine since it's just Go control flow. The blocker remains that each conditional stage needs different ActivityOptions.

**Verdict: Works, but blocked by Pattern 1.**

### Pattern 6: Multi-Queue Worker Architecture

**Current approach:**
```go
// Register different activities per queue
switch taskQueue {
case config.MainTaskQueue:
    r.registerMainQueueActivities(w)
case config.AITaskQueue:
    r.registerAIQueueActivities(w)
case config.EmailTaskQueue:
    r.registerEmailQueueActivities(w)
}
```

**With tempts:**
```go
var mainQueue = tempts.NewQueue("penfold-main")
var aiQueue = tempts.NewQueue("penfold-ai")
var emailQueue = tempts.NewQueue("penfold-email")

// Each queue gets its own worker with validated registrations
mainWorker, _ := tempts.NewWorker(mainQueue, mainRegisterables)
aiWorker, _ := tempts.NewWorker(aiQueue, aiRegisterables)
emailWorker, _ := tempts.NewWorker(emailQueue, emailRegisterables)
```

**Verdict: GOOD.** tempts validates at startup that every declared activity/workflow has an implementation registered, and vice versa. This catches missing registrations that currently only surface at runtime. The multi-queue pattern maps cleanly.

## Additional Concerns

### SDK Version Mismatch

tempts depends on Temporal SDK **v1.25.1**. Penfold uses **v1.29.1**. Go modules would resolve this via minimum version selection (using v1.29.1), but tempts was only tested against v1.25.1. There are no tagged releases, and the author has not verified compatibility with newer SDK versions. Breaking changes in the SDK's `worker.Registry`, `client.Client`, or `workflow.Context` interfaces would surface as compile errors, but subtle behavioral differences would not.

### API Stability

The README states: *"This library can change without notice while I respond to feedback and improve the API. I'll remove this warning when I'm happy with the API."*

- **No tagged releases** - only pseudo-versions from commit hashes
- **Last commit:** 2026-02-03 (active development)
- **Single maintainer** (vikstrous)
- **No CHANGELOG, no semver commitment**

Adopting an unstable library for core workflow infrastructure carries significant maintenance risk.

### Default Timeout Injection

`Workflow.WithImplementation()` injects a default `StartToCloseTimeout: 10 * time.Second` via `workflow.WithActivityOptions()`. This means **any activity executed without explicit options inherits a 10s timeout**, overriding Temporal's server-side defaults. For our LLM activities (10min expected), this would cause silent timeout failures.

### Struct Requirement

All Param types must be structs (enforced by `panicIfNotStruct`). Some of our simpler activities take a single `int64` (e.g., `DeleteEmbedding(ctx, embeddingID int64)`). These would need wrapper structs, adding boilerplate.

## Side-by-Side: SLM Pipeline (Abbreviated)

### Current
```go
func SLMPipelineWorkflow(ctx workflow.Context, input SLMPipelineInput) (SLMPipelineResult, error) {
    fastOpts := pkgtemporal.FastActivityOptions()
    llmOpts := pkgtemporal.LLMActivityOptions()

    // Stage 1: Triage (embedding timeout)
    ctxTriage := workflow.WithActivityOptions(ctx, embeddingOpts)
    err := workflow.ExecuteActivity(ctxTriage, "Triage", triageInput).Get(ctx, &triageOutput)

    // Stage 2: Extract (LLM timeout)
    ctxExtract := workflow.WithActivityOptions(ctx, llmOpts)
    err = workflow.ExecuteActivity(ctxExtract, "ExtractEntitiesActivity", extractInput).Get(ctx, &extractOutput)

    // Stage 5: Embed (embedding timeout)
    ctxEmbed := workflow.WithActivityOptions(ctx, embeddingOpts)
    err = workflow.ExecuteActivity(ctxEmbed, "GenerateContentEmbedding", embedInput).Get(ctx, &embedOutput)
}
```

### With tempts (hypothetical, working around limitations)
```go
// Declarations (global)
var mainQueue = tempts.NewQueue("penfold-main")
var triageActivity = tempts.NewActivity[TriageInput, TriageOutput](mainQueue, "Triage")
var extractActivity = tempts.NewActivity[ExtractInput, ExtractOutput](mainQueue, "ExtractEntitiesActivity")
var embedActivity = tempts.NewActivity[EmbedInput, EmbedOutput](mainQueue, "GenerateContentEmbedding")

func SLMPipelineWorkflow(ctx workflow.Context, input SLMPipelineInput) (SLMPipelineResult, error) {
    // PROBLEM: Can't set per-activity options through tempts API.
    // Must fall back to raw SDK calls, losing type safety:
    ctxTriage := workflow.WithActivityOptions(ctx, embeddingOpts)
    err := workflow.ExecuteActivity(ctxTriage, triageActivity.Name, triageInput).Get(ctx, &triageOutput)
    // ^ This is just the current code with .Name instead of a string literal.
    // The generic type checking is bypassed entirely.
}
```

## Decision

**Do Not Adopt** - neither for new workflows nor existing ones.

### Rationale

1. **Critical blocker: No per-call ActivityOptions.** The inability to set timeouts, heartbeat intervals, and retry policies per activity execution is incompatible with Penfold's tiered timeout architecture. This is our most important Temporal pattern.

2. **Unstable API.** No tagged releases, explicit "can change without notice" warning, single maintainer. Not suitable for core infrastructure.

3. **SDK version lag.** Pinned to Temporal SDK v1.25.1 vs our v1.29.1. No guarantee of forward compatibility.

4. **Hidden default timeout.** The 10s default `StartToCloseTimeout` injected by `WithImplementation()` would cause subtle failures for long-running activities.

5. **Limited value after workarounds.** Once we work around the ActivityOptions blocker (by using raw `workflow.ExecuteActivity` with `activity.Name`), the type safety benefit is reduced to workflow-level input/output checking, which we can achieve more simply with our existing constants approach (pf-2b9442, pf-cca5b4).

### What We Gain From Our Current Approach Instead

The sibling shards in this epic (pf-2b9442 activity name constants, pf-cca5b4 contract tests) address the same type-mismatch risk without the tempts downsides:

- **Activity name constants** prevent typos at compile time
- **Contract tests** verify input/output serialization round-trips
- **No new dependency** on an unstable library
- **Full control** over ActivityOptions, timeouts, and retry policies

### If tempts Improves

If tempts adds:
1. Per-call `ActivityOptions` support (e.g., `activity.RunWithOptions(ctx, opts, param)`)
2. Tagged semantic versions
3. Compatibility testing with recent Temporal SDK versions

Then this decision should be revisited. The signals, queries, and worker validation APIs are well-designed and would provide genuine value.
