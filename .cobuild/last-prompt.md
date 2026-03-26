# Task: Fix: NULL timeout_seconds crash in pipeline_definitions scan

**Task ID:** pf-af6c7c
**Agent:** 

## Task Content

## Context

Parent bug: pf-57bf1d
Worker crash-loops on startup because pipeline_definitions rows with NULL timeout_seconds cannot be scanned into Go int field. 7,172 of 21,488 rows have NULL.

## Fix Specification

### Part 1: Code — COALESCE in SQL queries (preferred approach)

In `pkg/pipeline/definitions.go`, change all 4 SELECT queries to use `COALESCE(timeout_seconds, 120)` instead of bare `timeout_seconds`:

1. **ListPipelines** (line 78): `COALESCE(timeout_seconds, 120)` in SELECT
2. **GetPipelineStages** (line 130): same
3. **UpdateStageConfig** RETURNING clause (line 230): same
4. **getStageDefinition** (line 329): same

The struct field `TimeoutSeconds int` stays as-is. No downstream changes needed.

### Part 2: Migration — backfill NULLs + NOT NULL constraint

Create migration (next available number):

```sql
-- +goose Up
UPDATE pipeline_definitions SET timeout_seconds = 120 WHERE timeout_seconds IS NULL;
ALTER TABLE pipeline_definitions ALTER COLUMN timeout_seconds SET NOT NULL;

-- +goose Down
ALTER TABLE pipeline_definitions ALTER COLUMN timeout_seconds DROP NOT NULL;
```

Default timeout of 120s matches the original column DEFAULT from migration 074.

### Verification

After applying:
1. `go build ./...` — compiles
2. `go test ./pkg/pipeline/... ./services/worker/...` — tests pass
3. Worker starts without crash-looping
4. `SELECT COUNT(*) FROM pipeline_definitions WHERE timeout_seconds IS NULL` returns 0

### Test to add

In `pkg/pipeline/definitions_test.go` or similar, add a test that scans a row where timeout_seconds would be NULL to verify COALESCE works correctly. This prevents regression if someone removes the COALESCE.

## Design Context (from pf-57bf1d)

**Worker crash: pipeline_definitions.timeout_seconds NULL scan fails startup validation**

## Problem

Worker crash-loops on startup with:

```
Pipeline definition validation failed — exiting
loading pipeline definitions for tenant c3170310-78bd-409c-b186-126f40bfa6ad: scanning pipeline definition: can't scan into dest[13] (col: timeout_seconds): cannot scan NULL into *int
```

## Root Cause

Migration 151 (pf-bc6fa5) set `timeout_seconds` for 6 specific stages (triage, extract_ner, extract_semantic, extract_assertions, newsletter_extract, analyze) but left all other pipeline_definitions rows with NULL. The Go struct scans `timeout_seconds` into `*int` which can't accept NULL.

## Impact

Worker is down. No pipeline processing.

## Fix Needed

Two-part fix:

1. **DB:** Set a default timeout_seconds for all rows where it's currently NULL (e.g. 300s as a safe default)
2. **Code:** Change the scan target from `*int` to `*sql.NullInt64` or `**int` so NULL is handled gracefully — a missing timeout should mean "use activity default", not crash

## Acceptance Criteria

- [ ] Worker starts successfully with NULL timeout_seconds rows in pipeline_definitions
- [ ] Existing non-NULL timeout_seconds values are still respected
- [ ] Migration seeds a default for all NULL rows

---
*Appended by agent-penfold at 2026-03-26 07:31 UTC*

## Investigation Report

### Symptom

Worker crash-loops on startup at pipeline definition validation. The exact error:

```
loading pipeline definitions for tenant c3170310-78bd-409c-b186-126f40bfa6ad: scanning pipeline definition: can't scan into dest[13] (col: timeout_seconds): cannot scan NULL into *int
```

The crash occurs at `services/worker/main.go:436-438` where `ValidateAllDefinitions` calls `ListPipelines`, which scans all `pipeline_definitions` rows. Any row with NULL `timeout_seconds` causes the scan to fail because the Go struct field is `int` (not `*int`).

### Root Cause

**Direct cause:** `StageDefinition.TimeoutSeconds` is declared as `int` (line 26 of `pkg/pipeline/definitions.go`), which cannot accept SQL NULL values. The pgx driver returns a scan error when it encounters NULL.

**Contributing cause — migration 151 (pf-bc6fa5, commit 05f882a):** This migration only updated 6 specific stages (`triage`, `extract_ner`, `extract_semantic`, `extract_assertions`, `newsletter_extract`, `analyze`) but **set them to specific values rather than fixing the NULL problem**. The migration's WHERE clauses match by stage name only, so it should have applied to all tenants — but DB inspection shows **all** of those stages still have NULL timeout_seconds (7,172 NULL rows out of 21,488 total).

**Why NULLs exist despite column DEFAULT:** The column was created with `DEFAULT 120` (migration 074) and is nullable (`IS NULL: YES`). Any row inserted without specifying `timeout_seconds` gets 120. However, the NULLs exist in exactly the 6 stages migration 151 targeted. This suggests migration 151 either: (a) never ran successfully against this database, or (b) ran but was subsequently reverted (the DOWN migration explicitly sets those stages back to NULL). The goose_db_version table is empty, so migration state tracking is unavailable.

**Design issue:** The `timeout_seconds` column allows NULL but the Go struct uses a non-nullable type. This mismatch means any NULL in this column will crash the scan. Other nullable columns in the same struct (`ContentType`, `ModelOverride`, `PromptOverride`, `Temperature`, `MaxTokens`, `MaxRetries`) are correctly declared as pointer types.

### Affected Files

1. `pkg/pipeline/definitions.go:26` — `TimeoutSeconds int` struct field (should handle NULL)
2. `pkg/pipeline/definitions.go:95` — Scan call in `ListPipelines` (crash site)
3. `pkg/pipeline/definitions.go:146` — Scan call in `GetPipelineStages`
4. `pkg/pipeline/definitions.go:237` — Scan call in `UpdateStageConfig`
5. `pkg/pipeline/definitions.go:334` — Scan call in `getStageDefinition`
6. `pkg/temporal/stage_executor.go:20` — `PipelineStageConfig.TimeoutSeconds int` (downstream consumer)
7. `pkg/temporal/stage_executor.go:37` — `StageConfig.TimeoutSeconds int` (downstream consumer)
8. `services/worker/workflows/pipeline.go:835` — `localStageConfig.TimeoutSeconds int` (downstream consumer)
9. `services/gateway/pipelineservice/definitions.go:363` — Proto conversion reads `sd.TimeoutSeconds`
10. `services/gateway/pipelineservice/service.go:1956-1957` — Timeout map builder checks `> 0`

### Related Issues

- Migration 151 (pf-bc6fa5) was the intended fix but appears to have not persisted. The goose migration tracking table is empty.
- The same NULL-vs-non-pointer pattern could theoretically affect any future column added as nullable in SQL but non-pointer in Go. However, all other nullable columns in StageDefinition are already pointer types — this is the only mismatch.

### Fragility Assessment

- **Coupling:** The scan target type in Go must match the SQL column nullability. Four separate scan sites all share the same bug — any NULL in timeout_seconds crashes all of them.
- **Test coverage:** `ValidateAllDefinitions` tests use mock listers that never return scan errors. There is no integration test that scans real NULL-containing rows. The `pipeline_definitions_test.go` test fixtures always set TimeoutSeconds to non-zero values.
- **Change frequency:** `pipeline_definitions` schema is actively evolving (15+ migrations since creation). Each migration that adds rows or columns increases the risk of NULL mismatches.

### Fix Specification

Two-part fix (code + data):

**Part 1: Code — handle NULL timeout_seconds gracefully**

1. File: `pkg/pipeline/definitions.go:26` — Change `TimeoutSeconds int` to `TimeoutSeconds *int`
2. File: `pkg/pipeline/definitions.go` — All 4 scan calls already use `&sd.TimeoutSeconds`, which will work with `*int`
3. File: `pkg/temporal/stage_executor.go:20,37` — Both `PipelineStageConfig` and `StageConfig` use `TimeoutSeconds int`. The mapping from `StageDefinition` to these types (in `services/worker/activities/pipeline_activities.go:334` and `services/worker/workflows/pipeline.go:3123`) must dereference the pointer with a default (e.g., 120).
4. File: `services/gateway/pipelineservice/definitions.go:363` — Proto conversion must handle nil pointer
5. All downstream consumers that check `cfg.TimeoutSeconds > 0` already handle zero as "use default", so passing 0 for NULL is safe

**Alternative (simpler, preferred): Use COALESCE in SQL**

Instead of changing the struct to `*int`, wrap the column in `COALESCE(timeout_seconds, 120)` in all 4 SELECT queries. This avoids touching any downstream code. The struct stays `int`, NULL becomes 120.

**Part 2: Migration — backfill NULLs and prevent recurrence**

1. New migration: `UPDATE pipeline_definitions SET timeout_seconds = 120 WHERE timeout_seconds IS NULL;`
2. Same migration: `ALTER TABLE pipeline_definitions ALTER COLUMN timeout_seconds SET NOT NULL;`
3. The column already has `DEFAULT 120`, so future inserts without timeout_seconds will get 120.
4. Adding NOT NULL prevents this class of bug from recurring.

### Test Requirements

1. Test: Unit test scanning a row with NULL timeout_seconds — Verifies: scan does not panic/error
2. Test: Integration test loading pipeline definitions from DB with mixed NULL/non-NULL timeout_seconds — Verifies: all rows load correctly with default applied
3. Test: Verify NOT NULL constraint prevents NULL insertion after migration — Verifies: data integrity

### Severity

**CRITICAL** — Worker is completely down. No pipeline processing is happening. Every startup attempt crash-loops at validation.

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-af6c7c`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
