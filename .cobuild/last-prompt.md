# Task: Fix: add digest-specific triage calibration for LOW importance

**Task ID:** pf-2193d8
**Agent:** 

## Task Content

## Parent Bug
pf-175ad3 — Dynamic Signal digest still triaged MEDIUM after prompt_override fix

## What to Change

### `tests/quality/helpers.go` — SeedClassificationRules pipeline definitions

The previous fix (pf-8f8510) applied prompt_override=3 to newsletter_digest triage, but v3 is an extraction prompt not a triage prompt.

This is related to pf-5fabbc (newsletter triage calibration). The newsletter_digest pipeline needs a triage prompt_override that:
1. Identifies the content as an automated digest
2. Defaults to LOW unless project-relevant content found

If pf-1d4347 (parent bug fix task) applies a newsletter triage prompt_override, verify it also handles the digest variant correctly. If the common newsletter prompt caps at MEDIUM, the digest variant may need a lower cap (LOW).

### Acceptance Criteria
- [ ] Dynamic Signal digest triaged as LOW
- [ ] `TestEval_Newsletter/004-dynamic-signal` triage check passes
- [ ] Other newsletter triage results not regressed

## Design Context (from pf-175ad3)

**Pipeline/Triage — Dynamic Signal digest still triaged MEDIUM after prompt_override fix**

## Problem

Dynamic Signal digest newsletter (004-dynamic-signal) is still triaged as MEDIUM importance despite the newsletter_digest pipeline now having triage prompt_override=3. The golden file expects LOW.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26 (after pf-8f8510 fix merged):

```
triage.importance: got "MEDIUM", expected one_of [LOW]
```

## Root Cause

The prompt_override=3 was added to the newsletter_digest pipeline triage stage (pf-8f8510 fix), but:
1. The v3 prompt may not have explicit calibration for digest newsletters
2. The Dynamic Signal content may contain enough project references that even a calibrated prompt rates it MEDIUM
3. Need to verify what prompt version 3 actually instructs for triage

## Investigation Steps

1. Check what triage prompt v3 contains — does it have digest-specific calibration?
2. Check the actual Dynamic Signal email content — is there legitimately MEDIUM-worthy content?
3. If v3 doesn't have digest calibration, either update v3 or create a v5 specifically for digest triage
4. Check if the golden file expectation of LOW is correct, or if MEDIUM is actually reasonable

## Acceptance Criteria

- [ ] Dynamic Signal digest newsletters triaged as LOW (or golden file updated if MEDIUM is correct)
- [ ] `TestEval_Newsletter/004-dynamic-signal` triage check passes

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-2193d8`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
