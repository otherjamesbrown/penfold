# Task: Fix: add triage prompt_override for newsletter_digest pipeline

**Task ID:** pf-51303e
**Agent:** 

## Task Content

## Parent Bug
pf-8f8510 — Dynamic Signal digest triaged MEDIUM instead of LOW

## What to Change

### `tests/quality/helpers.go` — SeedClassificationRules pipeline definitions

The newsletter_digest pipeline triage stage has no prompt_override, so it uses the default triage prompt which doesn't know digest newsletters should typically be LOW importance.

Add prompt_override to the triage stage for newsletter_digest pipeline:

```go
// newsletter_digest pipeline — triage with digest-aware prompt
{"newsletter_digest", "triage", 10, 120, &promptV2, "llm", nil},
```

### Investigation Needed
1. Check what triage prompt version 2 does — is it appropriate for digest calibration?
2. Check if a new prompt version is needed specifically for digest triage
3. Query production to see if newsletter_digest has a triage prompt_override there:
   ```sql
   SELECT stage, prompt_override FROM pipeline_definitions 
   WHERE pipeline = 'newsletter_digest' AND stage = 'triage';
   ```

### Acceptance Criteria
- [ ] Dynamic Signal digest newsletters triaged as LOW importance
- [ ] `go test -tags=quality ./tests/quality/... -run TestEval_Newsletter/004-dynamic-signal` triage check passes

## Design Context (from pf-8f8510)

**Pipeline/Triage — Dynamic Signal digest triaged MEDIUM instead of LOW**

## Problem

Dynamic Signal digest newsletter (004-dynamic-signal.eml) is triaged as MEDIUM importance but the golden file expects LOW. Dynamic Signal digests are automated social feed compilations with typically low value.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26:

```
triage.importance: got "MEDIUM", expected one_of [LOW]
```

The newsletter intent specification (pf-4d2288) defines Dynamic Signal as `handling: conditional` with `value: low` — "Weekly automated social feed. Usually low-value noise."

## Root Cause

Triage calibration for NEWSLETTER_DIGEST subtype. The triage stage may not have sufficient context about newsletter digest subtypes to rate them as LOW importance. The `prompt_override=4` for the newsletter_digest pipeline variant may need adjustment, or the triage prompt itself needs calibration for digest-type newsletters.

## Acceptance Criteria

- [ ] Dynamic Signal digest newsletters triaged as LOW importance
- [ ] `TestEval_Newsletter/004-dynamic-signal` triage check passes


## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-51303e`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
