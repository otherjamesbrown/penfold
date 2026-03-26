# Task: Fix: extend PreClassifyContent to detect newsletter senders for early pipeline resolution

**Task ID:** pf-7640c8
**Agent:** 

## Task Content

## Parent Bug
pf-a9a2ee — Pipeline/Triage prompt_override not applied for eval tenant newsletter triage

## Root Cause
The pipeline isn't known before triage for newsletter content, so the early pipeline definition fetch doesn't fire and prompt_override=0 is passed to triage. Notifications work because PreClassifyContent detects notification senders before triage.

## What to Change

### `services/worker/activities/triage_activities.go` — `PreClassifyContent` method

Extend the pre-classification to also check newsletter sender patterns against classification rules. Currently it only detects notification sources. It should also resolve newsletter classification rules so the pipeline is known before triage.

The classification rule engine already exists and is called inside triage. The fix is to move the rule engine call earlier — into PreClassifyContent — so the pipeline name is available for the early definition fetch.

### `services/worker/workflows/pipeline.go` — early pipeline fetch

Verify that `preclassifiedPipeline` is set from PreClassifyContent output when newsletter rules match. The existing code at line 1461-1464 should already handle this if PreClassifyContent returns the correct pipeline.

### Key constraints
- PreClassifyContent must be fast (no LLM calls) — classification rules are DB lookups only
- Must not break existing notification pre-classification
- Must handle the case where no rules match (fall through to triage as before)

### Acceptance Criteria
- [ ] Newsletter items have pipeline resolved before triage
- [ ] Triage prompt_override applied for newsletter/newsletter_internal/newsletter_digest
- [ ] `TestEval_Newsletter` triage checks pass for all 6 items
- [ ] Notification triage prompt_override=2 still works
- [ ] Standard email triage (no override) not affected
- [ ] `go test ./... && go vet ./...` passes

## Design Context (from pf-a9a2ee)

**Pipeline/Triage — prompt_override not applied for eval tenant newsletter triage**

## Problem

Newsletter triage prompt_override values (v2 for newsletter/newsletter_internal, v4 for newsletter_digest) are seeded in pipeline_definitions for the eval tenant but not applied during triage execution. Newsletters are still triaged as HIGH when they should be MEDIUM or LOW.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26 (after all seeding fixes merged):

- 001-ctg-post-its (NEWSLETTER_INTERNAL): triage=HIGH, expected MEDIUM or lower
- 002-akamai-wave (NEWSLETTER): triage=HIGH, expected MEDIUM or lower  
- 004-dynamic-signal (NEWSLETTER_DIGEST): triage=HIGH, expected LOW

003-emea-newsletter correctly returns MEDIUM, 006-spark-wellness correctly returns LOW — suggesting the LLM sometimes gets it right but the prompt_override is not being used.

Pipeline definitions confirm prompt_override is seeded:
```sql
SELECT pipeline, stage, prompt_override FROM pipeline_definitions 
WHERE tenant_id = '00000000-0000-0000-0000-000000000003' AND stage = 'triage';
-- newsletter: 2, newsletter_internal: 2, newsletter_digest: 4
```

## Root Cause Hypotheses

1. **Worker doesn't read prompt_override from pipeline_definitions during triage** — the triage activity may use a hardcoded prompt version or read from a different config source
2. **prompt_override lookup uses wrong tenant** — the worker may resolve the prompt using the default tenant instead of the eval tenant
3. **prompt_templates table missing the required version** — triage v2 or v4 may not exist in the prompt_templates table for the eval tenant
4. **Triage activity ignores prompt_override entirely** — the override may only be wired for extract stages, not triage

## Investigation Steps

1. Check `prompt_templates` table: `SELECT stage, version, is_active FROM prompt_templates WHERE stage = 'triage'`
2. Check worker triage activity code: how does it resolve which prompt to use? Does it read `prompt_override` from pipeline_definitions?
3. Check worker logs during eval run: does it log which prompt version is being used for triage?
4. Compare with notification pipeline: notification triage also has prompt_override=2 — does notification triage correctly use v2?

## Acceptance Criteria

- [ ] Triage activity reads and applies prompt_override from pipeline_definitions
- [ ] Newsletter triage uses the overridden prompt (v2 or v4 depending on pipeline variant)
- [ ] 001-ctg-post-its triaged MEDIUM or lower
- [ ] 002-akamai-wave triaged MEDIUM or lower
- [ ] 004-dynamic-signal triaged LOW
- [ ] No regression on existing triage behaviour for standard/notification pipelines

---
*Appended by agent-mycroft at 2026-03-26 18:48 UTC*

## Auto-completion evidence

Commit: a86bff48 [pf-a9a2ee] Auto-commit remaining changes
PR: https://github.com/otherjamesbrown/penfold/pull/79

### Files changed
```
.cobuild/last-prompt.md | 92 +++++++++++++++++++++++--------------------------
 1 file changed, 44 insertions(+), 48 deletions(-)
```

---
*Appended by agent-penfold at 2026-03-26 18:49 UTC*


---
*Investigation by agent-penfold at 2026-03-26 18:50 UTC*

## Investigation Report

### Root Cause — CONFIRMED

The triage `prompt_override` is only applied when the pipeline is known **before** triage runs. The early pipeline definition fetch (pipeline.go:1455-1486) loads stage configs including prompt_override, but only fires when `resolvedEarlyPipeline != ""`.

For newsletter items ingested via `penf ingest email` + `penf pipeline kick`, the pipeline isn't set until **after** triage classifies the content and resolves the routing. So `stageConfigMap` is empty during triage, and `promptOverrideForStage(stageConfigMap, "triage")` returns 0.

The `PreClassify`/`PreClassifyContent` activity runs before triage for some items (notification detection from sender patterns), but newsletter classification happens inside triage itself (via the rule engine), so the pipeline is a chicken-and-egg: the pipeline is determined by triage output, but the prompt_override needs to be set before triage runs.

### Affected Files
- `services/worker/workflows/pipeline.go` — lines 1455-1520 (early pipeline definition fetch + triage invocation)
- `services/worker/activities/triage_activities.go` — line 454-455 (PromptOverride usage)

### Fix Specification
Two options:

**Option A: Two-pass triage for newsletter content**
After triage routes to a newsletter pipeline, check if the pipeline definition has a triage prompt_override. If it does and it differs from what was used (0), re-run triage with the correct override. This is a retry, not a new stage.

**Option B: Pre-classify newsletters like notifications**
Extend the `PreClassifyContent` activity to also detect newsletter senders (using classification rules) and set the pipeline before triage. This makes newsletters work like notifications — pipeline known before triage, so early definition fetch works.

**Recommendation: Option B** — it's consistent with the notification pattern, avoids double LLM calls, and the classification rule engine already exists.

### Test Requirements
1. Eval test: `TestEval_Newsletter` triage checks pass for all 6 items
2. Notification triage regression: prompt_override=2 still applied correctly
3. Standard email: no triage prompt_override should not break

### Severity
MEDIUM — triage importance is wrong but pipeline stages and extraction work correctly.

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-7640c8`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.

**IMPORTANT RULES:**
- NEVER use raw `git merge` or `git push` to main — always use `cobuild complete` which creates a PR
- NEVER merge PRs yourself — the orchestrating agent handles merge via `cobuild merge` after review
- If a reviewer (Gemini, human) leaves a critical comment on your PR, you MUST address it before the PR can merge
- Check review comments: `gh pr view <pr-number> --comments`
