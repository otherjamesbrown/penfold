# Task: Fix: align newsletter pipeline stage_order with production

**Task ID:** pf-438fb6
**Agent:** 

## Task Content

## Parent Bug
pf-1a8731 — newsletter_extract stage not running for classified newsletters

## What to Change

### `tests/quality/helpers.go` — SeedClassificationRules pipeline definitions

The eval tenant newsletter pipeline definitions use stage_order values (0, 1, 2, 3) that don't match production (0, 10, 20, 30+). The worker's pipeline executor may skip stages when ordering doesn't match expectations.

Update the newsletter pipeline definitions to match production stage ordering:

```go
// Base newsletter pipeline — match prod stage ordering
{"newsletter", "parse", 0, 60, nil, "code_only", nil},
{"newsletter", "triage", 10, 120, nil, "llm", nil},
{"newsletter", "newsletter_extract", 20, 120, nil, "structured_extract", &newsletterKey},
{"newsletter", "embed", 30, 60, nil, "embedding", nil},
```

Apply same fix for newsletter_internal and newsletter_digest variants.

### Investigation
Check production pipeline_definitions for the exact stage_order values:
```sql
SELECT pipeline, stage, stage_order FROM pipeline_definitions 
WHERE pipeline LIKE 'newsletter%' AND tenant_id = '<james_tenant>'
ORDER BY pipeline, stage_order;
```

### Acceptance Criteria
- [ ] Eval tenant pipeline_definitions stage_order matches production
- [ ] newsletter_extract runs and completes for all 6 newsletter test items
- [ ] `pipeline_runs` shows newsletter_extract with status=completed
- [ ] `go test -tags=quality ./tests/quality/... -run TestEval_Newsletter` — all routing.must_complete checks pass

## Design Context (from pf-1a8731)

**Pipeline/Newsletter — newsletter_extract stage not running for classified newsletters**

## Problem

Several newsletter emails are classified correctly as NEWSLETTER but the `newsletter_extract` stage does not run. Items complete with only [parse, triage, embed] — skipping the extraction stage that produces structured newsletter data.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26:

005-eng-learning (correctly classified NEWSLETTER):
```
routing.completed_stages: [parse triage embed]
routing.must_complete: stage "newsletter_extract" not completed
```

004-dynamic-signal (classified NEWSLETTER_DIGEST):
```
routing.completed_stages: [parse triage embed]
routing.must_complete: stage "newsletter_extract" not completed
```

Note: newsletter_extract data IS present in the DB for 004 (queried separately), suggesting it may have run but wasn't recorded in pipeline_runs, or it ran on a previous processing attempt.

## Root Cause Hypotheses

1. **Pipeline definition mismatch**: The newsletter pipeline definitions for the eval test tenant may not include `newsletter_extract` stage, or it's disabled.
2. **Stage ordering**: `newsletter_extract` stage_order in test seed (2) vs prod stage_order — may cause the stage to be skipped.
3. **Worker stage dispatch**: The worker may not be dispatching `newsletter_extract` for items that have already been partially processed.

## Investigation Steps

1. Check `pipeline_definitions` for tenant `00000000-0000-0000-0000-000000000003` — does `newsletter_extract` exist with `enabled=true`?
2. Check `pipeline_runs` for affected source IDs — is there a `newsletter_extract` row with status != 'completed'?
3. Compare eval tenant pipeline_definitions with production tenant definitions.
4. Check worker logs during eval run for any newsletter_extract errors.

## Acceptance Criteria

- [ ] All correctly classified NEWSLETTER items run newsletter_extract stage
- [ ] `pipeline_runs` shows newsletter_extract with status=completed
- [ ] `TestEval_Newsletter` routing.must_complete checks pass for all 6 items


## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-438fb6`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
