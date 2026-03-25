# Task: Fix: skip embed stage on empty content instead of failing workflow

**Task ID:** pf-f060ec
**Agent:** 

## Task Content

## Parent Bug
pf-c7896e — Pipeline/Embed: empty content fails workflow instead of skipping embed stage

## What to Change

### 1. `services/worker/activities/embedding.go` — `GenerateEmbedding` method

Replace the empty content validation (lines 122-128):
```go
if input.Content == "" {
    return 0, temporal.NewApplicationError("content is empty", "ValidationError")
}
```

With a skip path:
```go
if input.Content == "" {
    logger.Info("Skipping embedding — content is empty")
    // Record skip in pipeline_runs for observability
    if a.pipelineRepo != nil {
        inputJSON, _ := json.Marshal(map[string]interface{}{
            "content_length": 0,
            "tenant_id":      input.TenantID,
        })
        _ = a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
            SourceID:        input.SourceID,
            Stage:           "embed",
            Status:          "skipped",
            SkipReason:      "content_empty",
            InputData:       inputJSON,
            LangfuseTraceID: input.LangfuseTraceID,
        })
    }
    return 0, nil
}
```

### 2. `services/worker/activities/multilevel_embedding.go` — `GenerateMultiLevelEmbeddings` method

Replace the empty content validation (lines 100-106) with same skip pattern, returning a successful empty `MultiLevelEmbeddingOutput{}`.

### 3. Unit Tests

Add tests for the empty-content skip path in both activities:
- `GenerateEmbedding` with empty content returns `(0, nil)` and records a skipped pipeline run
- `GenerateMultiLevelEmbeddings` with empty content returns empty output with no error

### Acceptance Criteria
- [ ] Reprocessing `em-LsFClKnE` completes successfully with embed stage skipped
- [ ] Skip reason `content_empty` is recorded in pipeline_runs and visible via `penf content show`
- [ ] No regression — items with actual content still embed normally
- [ ] Unit tests pass for empty-content path

## Design Context (from pf-c7896e)

**Pipeline/Embed - empty content fails workflow instead of skipping embed stage**

# Bug: Embedding stage fails hard on empty content instead of skipping

## Problem

When content items have empty body text (e.g. calendar cancellation emails), the `GenerateContentEmbedding` Temporal activity fails with:

```
embedding_failed: activity error (type: GenerateContentEmbedding, scheduledEventID: 138, startedEventID: 139, identity: 60657@dev01.brown.chat@): content is empty (type: ValidationError, retryable: true)
```

This marks the entire content item as `failed` (state 4, failure_category: `processing_error`). The item should instead complete with the embed stage skipped and a reason recorded.

## Evidence

Two items failed in the feb-bulk-test batch (1,282 total, only 2 failures):

| Content ID | Subject | Langfuse Trace |
|-----------|---------|----------------|
| `em-LsFClKnE` | Canceled: MTC: Observability Planning (bi-weekly) | `55fa74a1-fdd4-4f7f-a07b-e61d246e3840` |
| `em-LqJc50sC` | Canceled: MTC: Observability Planning (bi-weekly) | `3288530e-b4fe-4c4f-a149-bad1513f34a2` |

Both are calendar cancellation emails with no meaningful body — just HTML boilerplate and a cancellation notice divider.

## Expected Behavior

When `GenerateContentEmbedding` receives empty content:
1. Skip the embedding stage (do not fail the activity)
2. Record skip reason: `"content_empty"` (or similar)
3. Allow the content item to complete successfully — all prior stages (parse, triage, extract) ran fine
4. The item should end in `completed` state, not `failed`

## Root Cause

The `GenerateContentEmbedding` activity in the worker validates that content is non-empty and returns a `ValidationError` if it is. This error propagates up and fails the entire workflow. Instead, empty content should be a skip condition, not an error.

## No Automated Test

No e2e test provided — this is an error-handling path in the Temporal activity. The fix is to change the validation error to a skip result. Mycroft can add a unit test for the empty-content path in the embedding activity.

## Acceptance Criteria

- [ ] Reprocessing `em-LsFClKnE` completes successfully with embed stage skipped
- [ ] Skip reason is recorded and visible via `penf content show`
- [ ] No regression — items with actual content still embed normally


---
*Appended by agent-penfold at 2026-03-25 18:42 UTC*

## Investigation Report

**Investigated by:** Claude (agent-mycroft)
**Date:** 2026-03-25

### Root Cause

The `GenerateEmbedding` activity in `services/worker/activities/embedding.go` (line 123) validates that `input.Content` is non-empty and returns a hard `ValidationError` via `temporal.NewApplicationError`:

```go
if input.Content == "" {
    return 0, temporal.NewApplicationError(
        "content is empty",
        "ValidationError",
    )
}
```

This validation was introduced in commit `5f25f36` ("Reapply feat: Complete Go Migration") on 2026-01-19 — part of the original Go migration, not a deliberate design choice for empty content handling.

The same pattern exists in `multilevel_embedding.go` (line 101) for `GenerateMultiLevelEmbeddings`.

### Error Propagation Path

1. `GenerateEmbedding` returns `ValidationError` (Temporal ApplicationError)
2. In `pipeline.go:3172`, the embed stage checks `if err != nil` and treats ALL errors as fatal
3. Pipeline sets `state.result.Status = "failed"` and `FailureCategory = processing_error`
4. The item is permanently marked failed — no retry will help since content is genuinely empty

### Existing Skip Mechanism

The codebase already has a well-established skip pattern:
- `SkippedStage` struct in `pipeline.go:795` with `Stage` + `SkipReason` fields
- `PipelineRunInput.SkipReason` field in `interfaces.go:443` for recording skips to `pipeline_runs`
- `ActivityRecordSkippedStage` activity used for contribution gating, insufficient content, etc.
- Multiple examples: `"contribution_gating:NONE"`, `"insufficient_content:N_words"`, `"stage_not_in_pipeline"`

### The Fix

**Location 1:** `services/worker/activities/embedding.go`, `GenerateEmbedding` method (line 122-128)
- Change the empty content check from returning a `ValidationError` to returning `(0, nil)` — a successful zero result
- Log a warning with skip reason `"content_empty"`
- Record a `pipeline_runs` entry with `Status: "skipped"` and `SkipReason: "content_empty"` (using existing `pipelineRepo.CreateRun`)

**Location 2:** `services/worker/activities/multilevel_embedding.go`, `GenerateMultiLevelEmbeddings` method (line 100-106)
- Same change: return a successful empty `MultiLevelEmbeddingOutput` instead of a `ValidationError`

**Location 3:** `services/worker/workflows/pipeline.go` (line 3097-3203, embed stage)
- No change needed in the workflow — returning `(0, nil)` from the activity means `err == nil`, so the pipeline continues. The embed stage already handles `embeddingID == 0` gracefully (it just means no embedding was stored).

**Location 4 (content.go legacy path):** `services/worker/workflows/content.go` (line 382-397)
- Already handles embedding failure gracefully (`embeddingFailed = true` but continues). No change needed.

### Unit Test

Add a test in `embedding.go` (or a new `embedding_empty_content_test.go`) that calls `GenerateEmbedding` with empty content and asserts:
- No error returned
- Return value is 0
- `pipelineRepo.CreateRun` was called with `Status: "skipped"` and `SkipReason: "content_empty"`

### Risk Assessment

**Low risk.** The fix changes error→skip for a single validation check. Items with actual content are completely unaffected (the `input.Content == ""` guard only fires on truly empty strings). The existing skip/pipeline_run infrastructure handles all the recording.

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-f060ec`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
