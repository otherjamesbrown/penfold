# Task: Wire semaphore acquire/release into AI coordinator RPC handlers

**Task ID:** pf-5984d0
**Agent:** 

## Task Content

## Phase 3b: Model-aware semaphores — wire into RPC handlers

### Description
Wire the semaphore acquire/release (built in Phase 3a) into the ChatCompletion path in composite.go. Every LLM call through the AI coordinator must acquire a semaphore slot for its backend before executing and release it after.

### Changes

**1. Wire into CompositeBackend.ChatCompletion()**

File: `services/ai/backend/composite.go`

In the `ChatCompletion` method, after `extractProvider(opts.Model)` resolves the provider string:

```go
func (c *CompositeBackend) ChatCompletion(ctx context.Context, opts ChatOptions) (*ChatResponse, error) {
    provider := extractProvider(opts.Model)
    
    // Acquire semaphore for this provider
    acquireStart := time.Now()
    if err := c.acquireSemaphore(ctx, provider); err != nil {
        return nil, fmt.Errorf("semaphore acquire for %s: %w", provider, err)
    }
    defer c.releaseSemaphore(provider)
    
    semaphoreWaitMs := time.Since(acquireStart).Milliseconds()
    // Log semaphore wait time for observability
    
    // ...existing routing switch...
}
```

**2. Langfuse observability metadata**

Add semaphore wait time and concurrency state to the request metadata:
- `semaphore_wait_ms` — time spent waiting for a semaphore slot
- `backend_concurrent` — current occupancy of the semaphore at acquire time
- `backend_max` — configured max for this backend

These should be logged via the existing structured logging (zerolog), and if Langfuse metadata is available on the context, set there too.

**3. Graceful shutdown**

When the AI coordinator shuts down, in-flight semaphore holders should be allowed to complete (context cancellation handles this naturally via the select in acquireSemaphore). No special shutdown logic needed beyond existing graceful shutdown.

### Acceptance Criteria
- [ ] Every ChatCompletion call acquires/releases semaphore for its resolved provider
- [ ] Semaphore wait time is logged per request
- [ ] With ollama concurrency=3, a 4th concurrent ollama request blocks until one completes
- [ ] Gemini requests are not blocked by ollama semaphore (independent semaphores)
- [ ] Context cancellation (e.g. Temporal activity timeout) causes blocked acquire to return error
- [ ] Integration test: concurrent requests to same provider are limited

### Files Changed (max 3)
1. `services/ai/backend/composite.go` (edit — wire acquire/release into ChatCompletion)
2. `services/ai/backend/composite_test.go` (edit — integration/concurrency tests)

### Dependencies
- Task pf-88e86e (Phase 3a — semaphore infrastructure must exist first)

### Target
penfold repo

---
*Appended by agent-penfold at 2026-03-25 15:35 UTC*

## Scope Update (post Phase 3a review)

Phase 3a (pf-88e86e) already delivered:
- ✅ Semaphore acquire/release wired into ChatCompletion
- ✅ Context cancellation support (select on ctx.Done)
- ✅ Graceful shutdown (Close stops reload goroutine)

**Remaining work for this task is Langfuse observability only:**

1. Record `semaphore_wait_ms` — time between acquire attempt and slot granted
2. Record `backend_concurrent` — current semaphore occupancy at acquire time
3. Record `backend_max` — configured max for this provider
4. Log via structured logging (zerolog) per request
5. If Langfuse metadata is on context, set as generation metadata

### Updated files (max 2)
1. `services/ai/backend/composite.go` — add timing around acquireSemaphore, expose semaphore len/cap
2. `services/ai/backend/composite_test.go` — verify metadata is populated

## Design Context (from pf-6e38e9)

**Rate limit review and proposal — model-aware concurrency with queue backpressure**

# Rate Limit Review and Proposal — model-aware concurrency with queue backpressure

## 1. Problem

The pipeline has 16 separate rate limiting, concurrency, and throttling mechanisms layered on top of each other. They were designed to protect a local Ollama/qwen3:8b model (~30-40s per call) but now most LLM stages use Gemini Flash (~2-5s per call). The result:

- **Bulk ingest of 1,075 emails processed at ~10 concurrent** despite `max_concurrent=30`, because Temporal worker limits (`MaxConcurrentWorkflows=10`) and `KickNextPending Limit=1` (hardcoded in `pipeline.go:1002,1564,3316`) bottleneck throughput
- **Timeouts designed for Ollama** — `LLMActivityOptions` has 10-minute StartToClose (`pkg/temporal/options.go`) when Gemini returns in 2-5s
- **Queue backup causes cascading timeouts** — when 200+ items enter the pipeline, Temporal workflows queue up. The 10-minute LLM timeouts mean a workflow can hold a slot for 10 minutes even if the LLM call finished in 3s. Meanwhile queued workflows hit their ScheduleToClose timeout (15min) waiting for a slot
- **No model awareness** — the same concurrency limit applies whether the stage calls qwen3:8b (30-40s, local, single-threaded) or Gemini Flash (2-5s, cloud, high concurrency). Flooding 30 concurrent requests to qwen overwhelms Ollama; 30 to Gemini is well within limits

### Current limits inventory

| # | Limit | Value | Location | Configurable | Originally for |
|---|-------|-------|----------|---|---|
| 1 | `pipeline.max_concurrent` | 30 (was 5, default 1) | `pipeline_operational_config` DB | Yes (live) | Ollama |
| 2 | `KickNextPending Limit` | 0 (was 1) | `pipeline_operational_config` key `pipeline.kick_next_limit` | Yes (live) | Sequential processing |
| 3 | `MaxConcurrentActivities` | 50 (was 10) | Worker env var | Yes (restart, SDK constraint) | Temporal throttle |
| 4 | `MaxConcurrentWorkflows` | 30 (was 10) | Worker env var | Yes (restart, SDK constraint) | Temporal throttle |
| 5 | `LLMActivityOptions` timeout | 10min/15min | `pkg/temporal/options.go` | No (hardcoded) | Slow Ollama calls |
| 6 | `EmbeddingActivityOptions` timeout | 30s/5min | `pkg/temporal/options.go` | No (hardcoded) | Local MLX embeddings |
| 7 | `ClaimReprocessSlot` | = max_concurrent | `pkg/pipeline/repository.go:314` | Yes (same as #1) | Ollama |
| 8 | `MaxConcurrentModels` | 5 | `services/ai/ensemble/orchestration.go:38` | No (hardcoded) | Ensemble operations |
| 9 | Contribution gating | per-stage skip_when_low | `pipeline_definitions` DB | Yes | Reduce LLM calls |

### The core tension

Qwen3:8b on Ollama: ~30-40s per call, single model instance, CPU-bound on dev01. Can handle ~2-3 concurrent calls before quality degrades.

Gemini Flash via API: ~2-5s per call, cloud-hosted, supports 100+ RPM. Can handle 30-50 concurrent calls easily.

Both are called by the same pipeline stages. The system needs per-model concurrency limits, not a single global gate.

## 2. User / Consumer

- **James** — bulk ingest of hundreds of emails should complete in minutes, not hours
- **Pipeline operators** — adding a new model backend (e.g. Claude, GPT-4) shouldn't require tuning hardcoded limits
- **The system itself** — queue backup from bulk operations should not cascade into timeout failures

## 3. Success Criteria

1. Bulk ingest of 1,000 emails completes pipeline processing within 30 minutes (currently takes 3+ hours)
2. Qwen3:8b (Ollama) is never sent more than 3 concurrent requests, regardless of pipeline concurrency
3. Gemini Flash can receive up to 50 concurrent requests when pipeline load demands it
4. Adding a new model backend requires only a DB config row, not code changes to concurrency limits
5. When the pending queue exceeds 100 items, no workflow hits ScheduleToClose timeout due to queuing delays

## 4. Scope Boundaries

**In scope:**
- Model-aware concurrency limits (per-model max concurrent)
- KickNextPending limit change (kick to fill available slots, not just 1)
- Timeout adjustment (model-aware or reduced to match actual latency)
- Queue backpressure (extend ScheduleToClose when queue is deep)
- DB-driven configuration for all limits

**Out of scope:**
- Temporal cluster scaling (single worker is sufficient for current volume)
- Cost optimization / model routing (separate concern — which model to use is already handled by AI coordinator routing rules)
- Ensemble concurrency (item #8 — only used for experimental multi-model, not production)
- Contribution gating changes (already addressed by pf-bb24a8)

## 5. Technical Approach

### A. Model-aware concurrency via AI coordinator semaphores

The AI coordinator (`services/ai/server/server.go`) is the single point where all LLM calls are made. It already knows which model/backend handles each request. Add per-backend semaphores:

```go
// services/ai/server/server.go
type AIServer struct {
    // ...existing fields...
    backendSemaphores map[string]chan struct{} // backend_name → buffered channel
}
```

Configuration in `pipeline_operational_config`:
```sql
INSERT INTO pipeline_operational_config (tenant_id, key, value) VALUES
  ('<TENANT>', 'model.concurrency.ollama', '3'),
  ('<TENANT>', 'model.concurrency.gemini', '50'),
  ('<TENANT>', 'model.concurrency.default', '10');
```

When the AI coordinator receives a request:
1. Determine backend (ollama, gemini, mlx)
2. Acquire semaphore slot for that backend (block if full)
3. Execute LLM call
4. Release semaphore slot

If the semaphore is full, the gRPC call blocks (Temporal activity heartbeats keep the workflow alive). This is simpler than returning ResourceExhausted and requiring callers to retry.

**Why this approach:** The AI coordinator already centralises model routing. Adding semaphores here means the pipeline workflow doesn't need to know about model limits — it just makes calls, and the coordinator manages contention. No changes to pipeline.go or the workflow.

### B. KickNextPending — fill available slots

Change `KickNextPending` from `Limit: 1` to `Limit: 0` (meaning "fill all available slots"):

**File:** `services/worker/workflows/pipeline.go` lines 1002, 1564, 3316

```go
// Before:
Limit: 1,
// After:
Limit: 0, // 0 = fill available slots (gateway calculates from max_concurrent - in_flight)
```

**File:** `services/gateway/pipelineservice/service.go` line 176-178 — already handles `Limit: 0` by using `availableSlots` as the limit.

Wait — checking the code, `req.Limit > 0` is the guard at line 177. If Limit=0, it uses `availableSlots` directly. So this already works; we just need to change the hardcoded 1 to 0 in the workflow.

### C. Model-aware timeouts

Replace the hardcoded `LLMActivityOptions` with per-stage timeouts from `pipeline_definitions.timeout_seconds`:

The stage-kind executor framework (pf-ccbd1b, already implemented) reads `stage.TimeoutSeconds` and overrides `StartToCloseTimeout` if > 0. The fix is to set appropriate timeout_seconds in pipeline_definitions:

```sql
-- Gemini stages: 60s is plenty
UPDATE pipeline_definitions SET timeout_seconds = 60
WHERE stage IN ('triage', 'extract_ner', 'extract_semantic', 'extract_assertions')
  AND tenant_id = '<TENANT>';

-- Ollama stages (newsletter_extract via qwen): 120s
UPDATE pipeline_definitions SET timeout_seconds = 120
WHERE stage = 'newsletter_extract'
  AND tenant_id = '<TENANT>';

-- Analyze (may use gemini-pro, slower): 180s
UPDATE pipeline_definitions SET timeout_seconds = 180
WHERE stage = 'analyze'
  AND tenant_id = '<TENANT>';
```

**No code change needed** — the executor framework already supports this. Just needs DB values populated.

### D. Queue backpressure — dynamic ScheduleToClose

When the pending queue is deep (>50 items), extend the ScheduleToClose timeout for new workflows. This prevents queued workflows from timing out while waiting for a slot.

**File:** `services/gateway/pipelineservice/service.go` — in the `startWorkflow` call, read pending count and adjust:

```go
scheduleToClose := 1 * time.Hour // default
if pendingCount > 50 {
    scheduleToClose = 4 * time.Hour // deep queue — give it time
}
```

This is a simple heuristic. The proper fix is the model-aware semaphores (A) which prevent queue buildup in the first place.

### Code locations summary

| Change | File | Lines | Type |
|--------|------|-------|------|
| Per-backend semaphores | `services/ai/server/server.go` | New field + acquire/release in GenerateSummary, Triage, ExtractEntities | Code change |
| Semaphore config | `pipeline_operational_config` | New rows | DB migration |
| KickNextPending limit | `services/worker/workflows/pipeline.go` | 1002, 1564, 3316 | Read from `pipeline_operational_config` key `pipeline.kick_next_limit` (default: 0 = fill slots) |
| Stage timeouts | `pipeline_definitions` | Existing column | DB update |
| Queue backpressure | `services/gateway/pipelineservice/service.go` | startWorkflow area | Code change |
| Temporal worker limits | `/etc/penfold/worker.env` | WORKER_MAX_CONCURRENT_* | Env config |

### Data model

No new tables. Extends existing:
- `pipeline_operational_config`: new keys `model.concurrency.<backend>`
- `pipeline_definitions`: populate existing `timeout_seconds` column

## 6. Migration / Rollout

**Phase 1 — Quick wins (DB + env only, no code):**
1. Set `timeout_seconds` in `pipeline_definitions` for all stages
2. Increase `WORKER_MAX_CONCURRENT_ACTIVITIES=50` and `WORKER_MAX_CONCURRENT_WORKFLOWS=30` in worker.env
3. Restart worker

**Phase 2 — KickNextPending fix (small code change):**
1. Change `Limit: 1` to `Limit: 0` in 3 places in pipeline.go
2. Deploy worker

**Phase 3 — Model-aware semaphores (main code change):**
1. Add semaphore map to AIServer
2. Load config from `pipeline_operational_config` at startup
3. Acquire/release in each RPC handler
4. Deploy AI coordinator
5. Set `model.concurrency.ollama=3`, `model.concurrency.gemini=50`

**Phase 4 — Queue backpressure (if needed after Phase 1-3):**
1. Dynamic ScheduleToClose based on pending count
2. Only needed if queue backup still causes timeouts after semaphores

Each phase is independently deployable and backward-compatible. Phase 1 can be done immediately.

## Edge Cases / Error Handling

- **Semaphore full + heartbeat timeout:** If a workflow is blocked waiting for a semaphore slot and the heartbeat timeout expires, the activity will be retried by Temporal. The semaphore should use a context-aware acquire (select on ctx.Done and semaphore) to fail fast if the activity is cancelled.
- **Model config missing:** If `model.concurrency.<backend>` is not in the DB, fall back to `model.concurrency.default` (10). If that's also missing, raise a startup validation error (fail visibly, don't silently fall back to a hardcoded value).
- **Backend identification:** The AI coordinator already resolves which backend handles a request via routing rules. The backend name (ollama, gemini, mlx) is known before the LLM call.
- **Hot config reload:** Semaphore limits should be reloaded periodically (every 60s) so changes take effect without restart. Resize semaphore by creating a new channel and draining the old one.

## Langfuse Observability

- Log semaphore wait time per-request: `langfuse.meta.semaphore_wait_ms`
- Log backend concurrency at time of request: `langfuse.meta.backend_concurrent`, `langfuse.meta.backend_max`
- Existing per-stage duration tracking shows total time (includes semaphore wait) — the new `semaphore_wait_ms` separates queue time from execution time


### HIGH Fix 1: KickNextPending — DB-driven limit

The `KickNextPending Limit` value (currently hardcoded as `1` in three places in `pipeline.go`) will be read from `pipeline_operational_config` instead of being a code constant:

**New config key:** `pipeline.kick_next_limit`
- Default seed value: `0` (meaning "fill all available slots" — gateway already handles this at `service.go:177`)
- Setting to `1` restores current sequential behaviour
- Setting to any positive N caps how many items are kicked per cycle

```sql
INSERT INTO pipeline_operational_config (tenant_id, key, value) VALUES
  ('<TENANT>', 'pipeline.kick_next_limit', '0');
```

The workflow reads this value via `GetOperationalConfig` (already available in the workflow context) before each `KickNextPending` call. This replaces the three hardcoded `Limit: 1` values at `pipeline.go:1002,1564,3316`.

### HIGH Fix 2: Temporal worker env vars — acknowledged SDK constraint

`WORKER_MAX_CONCURRENT_ACTIVITIES` and `WORKER_MAX_CONCURRENT_WORKFLOWS` are **Temporal SDK construction-time parameters** — they are passed to `worker.New()` at process startup and cannot be changed at runtime. This is a Temporal SDK limitation, not an application design choice.

These values are explicitly excluded from the "all config in DB" principle because:
1. The Temporal Go SDK `worker.Options` struct is set once at `worker.New()` — there is no API to change these after construction
2. They control Temporal's internal slot management, not application-level concurrency
3. The actual model-aware concurrency is handled by the AI coordinator semaphores (Section 5A), which ARE DB-driven and live-reloadable

The env vars set the upper bound of what Temporal will schedule. The semaphores (DB-driven) set the actual per-model limits within that bound. Restarting the worker to change Temporal's ceiling is acceptable because it's an infrastructure tuning operation, not a behavioural change.

### HIGH Fix 3: Backend name contract — exact provider strings

The AI coordinator resolves the backend provider via `extractProvider(opts.Model)` in `services/ai/backend/composite.go:36-52`. The `Model` field uses the format `"provider/model-name"` (e.g. `"gemini/gemini-2.5-flash"`, `"ollama/qwen3:8b"`).

The `extractProvider()` function splits on `/` and returns the prefix. The `CompositeBackend.ChatCompletion()` switch statement routes on these exact strings:

| Provider string | Routes to | DB constraint |
|----------------|-----------|---------------|
| `"gemini"` | `b.gemini.ChatCompletion()` | `ai_models.provider IN ('ollama','gemini','openai','anthropic','mlx')` |
| `"anthropic"` | `b.anthropic.ChatCompletion()` | same |
| `"ollama"` | `b.ollama.ChatCompletion()` (default) | same |
| `"mlx"` | `b.ollama.ChatCompletion()` (default) | same |
| `""` (no prefix) | Sniff for "gemini" in model name, else ollama | backward compat |

The semaphore config keys use the same provider strings as suffixes, matching the existing `pipeline_operational_config` dot-notation convention (e.g. `timeout.http.backend.gemini`):

```sql
INSERT INTO pipeline_operational_config (tenant_id, key, value) VALUES
  ('<TENANT>', 'model.concurrency.ollama', '3'),
  ('<TENANT>', 'model.concurrency.gemini', '50'),
  ('<TENANT>', 'model.concurrency.anthropic', '20'),
  ('<TENANT>', 'model.concurrency.default', '10');
```

The semaphore map key is the return value of `extractProvider(opts.Model)`. If no `model.concurrency.<provider>` key exists, it falls back to `model.concurrency.default`. If that's also missing, a startup validation error is raised (not a silent fallback to a hardcoded value).


---
*Appended by agent-mycroft at 2026-03-25 15:32 UTC*

## Retrospective Note — Hardcoded Config Values

The Phase 4 implementation (pf-aa6fcb) shipped with hardcoded backpressure thresholds (50, 100) instead of reading from pipeline_operational_config. The design review flagged this as LOW but it should have been HIGH — it violates pf-eeb256 ("all config in database").

**Root cause:** The design-review skill's severity classification for "hardcoded values that should be configurable" was too lenient. The penfold architectural principles treat this as a hard constraint, not a suggestion.

**Action:** 
1. Bug filed: pf-3f7157 (dispatched to fix)
2. Design review skill should classify hardcoded config as HIGH when the project has a "config in DB" principle
3. The gate-readiness-review skill should cross-reference project-specific constraints at the correct severity level

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. **Run `cobuild complete pf-5984d0`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
