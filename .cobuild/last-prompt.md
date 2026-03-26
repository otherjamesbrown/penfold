# Task: Fix: add AkamaiWellness classification rule for Spark newsletter

**Task ID:** pf-03fc5c
**Agent:** 

## Task Content

## Parent Bug
pf-66100d — Spark wellness newsletter misclassified as HUMAN

## What to Change

### `tests/quality/helpers.go` — SeedClassificationRules

Add a classification rule for the AkamaiWellness sender:

```go
{"newsletter_akamai_wellness", 8, "NEWSLETTER", nil, "from_address", "exact", "AkamaiWellness@akamai.com"},
```

Add it alongside the existing `newsletter_akamai_spark` rule in the newsletter rules section.

### Acceptance Criteria
- [ ] `006-spark-wellness.eml` classified as NEWSLETTER
- [ ] Newsletter pipeline stages run (parse, triage, newsletter_extract, embed)
- [ ] `go test -tags=quality ./tests/quality/... -run TestEval_Newsletter/006-spark-wellness` passes routing checks

## Design Context (from pf-66100d)

**Pipeline/Classification — Spark wellness newsletter misclassified as HUMAN**

## Problem

Spark/Wellness newsletter (006-spark-wellness.eml) from `AkamaiSpark@akamai.com` is classified as HUMAN instead of NEWSLETTER. The eval framework golden file expects `content_subtype: NEWSLETTER`.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26:

```
routing.content_subtype: expected "NEWSLETTER", got "HUMAN"
routing.completed_stages: [triage parse]
routing.must_complete: stage "newsletter_extract" not completed
routing.must_complete: stage "embed" not completed
```

No newsletter extraction ran — the item was treated as a regular human email.

## Root Cause

Classification rule `newsletter_akamai_spark` uses exact match on `AkamaiSpark@akamai.com` but the actual From header may have a different case or format. The rule is seeded in `tests/quality/helpers.go` SeedClassificationRules. Need to verify:

1. Does the From address in `006-spark-wellness.eml` exactly match `AkamaiSpark@akamai.com`?
2. Is the classification rule engine case-sensitive on exact matches?
3. Check the production classification_rules table for the pattern used there.

## Acceptance Criteria

- [ ] 006-spark-wellness.eml classified as NEWSLETTER (content_subtype)
- [ ] Newsletter pipeline stages run (parse, triage, newsletter_extract, embed)
- [ ] `TestEval_Newsletter/006-spark-wellness` routing checks pass


## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-03fc5c`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
