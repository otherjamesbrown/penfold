# Task: Fix: add triage prompt_override for newsletter pipelines to cap at MEDIUM

**Task ID:** pf-1d4347
**Agent:** 

## Task Content

## Parent Bug
pf-5fabbc — newsletters triaged as HIGH instead of MEDIUM/LOW

## What to Change

### `tests/quality/helpers.go` — SeedClassificationRules pipeline definitions

Add prompt_override to the triage stage for all three newsletter pipeline variants. Use a prompt version that instructs triage to cap newsletter importance at MEDIUM.

Currently the newsletter triage has no prompt_override:
```go
{"newsletter", "triage", 10, 120, nil, "llm", nil},
```

Need to determine the correct prompt version. Check existing prompt versions:
```sql
SELECT DISTINCT prompt_override FROM pipeline_definitions WHERE stage = 'triage' AND prompt_override IS NOT NULL;
```

If no existing version handles newsletter calibration, may need to coordinate with the prompt management system.

Interim fix: Use prompt_override=2 (notification triage prompt) which is already calibrated for non-human content, then verify results.

### Acceptance Criteria
- [ ] Newsletter triage importance is MEDIUM or lower for 001-ctg-post-its and 002-akamai-wave
- [ ] `go build -tags=quality ./tests/quality/...` compiles
- [ ] No regression on other newsletter triage results (003-emea should stay MEDIUM)

## Design Context (from pf-5fabbc)

**Pipeline/Triage — newsletters triaged as HIGH instead of MEDIUM/LOW**

## Problem

Multiple newsletters are triaged as HIGH importance when they should be MEDIUM or LOW. Newsletters are informational content that rarely require immediate action.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26:

**001-ctg-post-its**: `triage.importance: got "HIGH"` — internal corporate newsletter, expected MEDIUM at most
**002-akamai-wave**: `triage.importance: got "HIGH"` — external digest newsletter, expected MEDIUM

Only 003-emea-newsletter correctly triaged as MEDIUM.

## Root Cause

The triage prompt doesn't have sufficient calibration for newsletter content subtypes. Newsletters should generally be triaged MEDIUM (informational) or LOW (automated digests), not HIGH. The triage stage doesn't receive context about the content_subtype being NEWSLETTER, so it judges importance based on content alone — which can trigger HIGH for newsletters that mention projects or deadlines.

## Fix Approach

Either:
1. Add a triage prompt_override for the newsletter pipeline that calibrates importance downward for informational content
2. Pass content_subtype as context to the triage stage so the prompt can adjust calibration
3. Post-triage calibration rule: cap newsletter importance at MEDIUM unless content contains genuine escalation keywords

## Acceptance Criteria

- [ ] Newsletter triage importance is MEDIUM or lower for standard newsletters
- [ ] Only newsletters with genuine escalation content (security alerts, P0 incidents forwarded via newsletter) should be HIGH
- [ ] `TestEval_Newsletter` triage checks pass for 002-akamai-wave and 001-ctg-post-its

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-1d4347`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
