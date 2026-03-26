# Task: Fix: change newsletter_ctg classification to NEWSLETTER_INTERNAL

**Task ID:** pf-c3ec78
**Agent:** 

## Task Content

## Parent Bug
pf-2c4f31 — CTG Post-Its classified NEWSLETTER instead of NEWSLETTER_INTERNAL

## What to Change

### `tests/quality/helpers.go` — SeedClassificationRules

Change the subtype for the `newsletter_ctg` rule from "NEWSLETTER" to "NEWSLETTER_INTERNAL":

```go
// Before:
{"newsletter_ctg", 8, "NEWSLETTER", nil, "from_address", "exact", "ctgcomms@akamai.com"},
// After:
{"newsletter_ctg", 8, "NEWSLETTER_INTERNAL", nil, "from_address", "exact", "ctgcomms@akamai.com"},
```

### Acceptance Criteria
- [ ] 001-ctg-post-its.eml classified as NEWSLETTER_INTERNAL
- [ ] Routed to newsletter_internal pipeline
- [ ] `go build -tags=quality ./tests/quality/...` compiles

## Design Context (from pf-2c4f31)

**Pipeline/Classification — CTG Post-Its classified as NEWSLETTER instead of NEWSLETTER_INTERNAL**

## Problem

001-ctg-post-its.eml is classified as NEWSLETTER but the golden file expects NEWSLETTER_INTERNAL. The `newsletter_internal_corporate` classification rule matches on subject containing "Post-Its" but the CTG newsletter is being matched first by `newsletter_ctg` (exact sender match on ctgcomms@akamai.com, priority 8) which routes to NEWSLETTER.

## Evidence

From `TestEval_Newsletter` run on 2026-03-26:

```
routing.content_subtype: expected "NEWSLETTER_INTERNAL", got "NEWSLETTER"
routing.pipeline: expected "newsletter_internal", inferred "newsletter" from stages
```

## Root Cause

Priority conflict between two classification rules:
- `newsletter_ctg` (priority 8, exact from_address match) → NEWSLETTER
- `newsletter_internal_corporate` (priority 80, subject contains "Post-Its") → NEWSLETTER_INTERNAL

Lower priority number wins, so `newsletter_ctg` (8) takes precedence over the subject-based rule (80). The CTG Post-Its newsletter should be classified as NEWSLETTER_INTERNAL since it's an internal corporate newsletter.

## Fix Options

1. Change `newsletter_ctg` rule to map to NEWSLETTER_INTERNAL instead of NEWSLETTER
2. Lower the priority number on `newsletter_internal_corporate` to below 8
3. Add a separate rule for ctgcomms@akamai.com → NEWSLETTER_INTERNAL

Option 1 is simplest — the CTG comms sender only sends the Post-Its internal newsletter.

## Files to Change

- `tests/quality/helpers.go` — SeedClassificationRules: change newsletter_ctg subtype from "NEWSLETTER" to "NEWSLETTER_INTERNAL"
- Verify production classification_rules table has the same fix

## Acceptance Criteria

- [ ] 001-ctg-post-its.eml classified as NEWSLETTER_INTERNAL
- [ ] Routed to newsletter_internal pipeline (with v3 prompt_override)
- [ ] `TestEval_Newsletter/001-ctg-post-its` routing checks pass

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-c3ec78`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
