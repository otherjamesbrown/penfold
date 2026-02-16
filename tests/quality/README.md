# Quality Tests — Pipeline Extraction Accuracy

Tests that real emails produce correct knowledge extraction. Unlike unit/integration/e2e tests that verify plumbing, these verify **what the pipeline actually extracts** using golden files with semantic matching.

## Why

If we switch models (Gemini Flash to something else) or change prompts, nothing in the existing test suite catches extraction regressions. These tests answer: *"given this email, did the pipeline extract the right people, risks, decisions?"*

## How It Works

1. Each `golden/*.yaml` file defines expected output for one test email
2. The test runner ingests the email via CLI, kicks the pipeline, waits for completion
3. Queries the database for actual extraction results
4. Compares using semantic matchers (case-insensitive substring, not exact string)
5. Reports failures with full actual output for debugging

## Running

```bash
# Full suite (~7-10 min, ~$0.50 in API costs)
go test -tags=quality -v -timeout 30m ./tests/quality/

# Single email
go test -tags=quality -v -timeout 10m -run TestQuality/011-risk ./tests/quality/

# From project root
cd /path/to/penfold
go test -tags=quality -v -timeout 30m ./tests/quality/
```

**Requirements:**
- SSL certs in `~/.postgresql/` (same as E2E tests)
- Gateway and LLM service running on dev02
- Network access to dev02.brown.chat

## Golden File Schema

```yaml
email: "011-risk-escalation.eml"       # File in tests/fixtures/acme-corp/emails/
description: >                          # What this test covers (documentation)
  Risk escalation with 3 explicit risks...

last_verified: "2026-02-14"             # When expectations were last validated
model_at_verification: "gemini-2.0-flash"  # Model used when verified

pipeline:
  must_complete: [embed, extract_ner, extract_semantic]  # Required stages

triage:
  importance:
    one_of: [HIGH, CRITICAL]            # Acceptable values (non-deterministic)
  category:
    one_of: [RISK_ISSUE, PROJECT_UPDATE]

people:
  min_count: 4                          # At least this many people found
  must_find:
    - name_contains: "Sarah Chen"       # Case-insensitive substring
      role_contains: "Product Manager"  # Optional: from signature/context
    - name_contains: "John Smith"
  must_not_find:
    - name_contains: "Acme Corp"        # Org name, not a person

assertions:
  min_count: 2
  must_find:
    - type: risk                        # assertion_type enum value
      description_contains: "budget"    # Case-insensitive substring
    - type: risk
      description_contains: "timeline"
  must_not_find:
    - type: security_incident           # Should not be classified this way

projects:
  must_find:
    - name_contains: "Project Alpha"    # Linked via assertions.project_id
```

### Matching Types

| Matcher | Meaning |
|---------|---------|
| `must_find` | Each item must match at least one extracted item |
| `must_not_find` | None of these should appear (guards against false positives) |
| `min_count` / `max_count` | Bounds on total extracted items (absorbs LLM variation) |
| `one_of` | Value must be one of listed options (for non-deterministic categoricals) |
| `name_contains` | Case-insensitive substring match on name |
| `description_contains` | Case-insensitive substring match on description |
| `role_contains` | Case-insensitive substring match on person title/role |
| `confidence_min` | Optional minimum confidence threshold |

## Test Emails

| File | Scenarios Covered |
|------|-------------------|
| `011-risk-escalation` | People with signatures/titles, 3 risks, projects, glossary, HIGH triage |
| `013-thread-with-decisions` | 4 labeled decisions, thread context (In-Reply-To), cross-project refs |
| `002-incident-response` | P0 severity, technical metrics, SLO/SLA, DRI, root cause |
| `012-low-priority-fyi` | **Negative test** — LOW triage, no assertions, must_not_find guards |

## Handling Non-Determinism

LLMs produce slightly different output per run. The design absorbs this:

1. **Substring matching** — "35% budget overrun" and "Budget overrun of 35%" both match `description_contains: "budget"`
2. **`one_of`** for categoricals — triage importance may vary between runs
3. **`min_count`** not exact — extracting 4 assertions when we expect 2+ is fine
4. **Only high-confidence expectations in `must_find`** — items extracted 95%+ of the time
5. **Full actual output logged on failure** — shows whether regression or noise

## Maintenance Protocol

### Modify existing golden file when:
- Pipeline improved (extracts something it used to miss) — relax or add expectations
- Prompt change intentionally altered behaviour — update expectations to match
- Golden file had a wrong expectation

### Add new golden file when:
- Bug is in a category not covered by existing emails
- Need to test a specific edge case in isolation

### Never:
- Weaken assertions to make a flaky test pass without understanding why
- Remove `must_find` items because a model change stopped finding them — that's the test doing its job

## Architecture

- **Tenant ID:** `00000000-0000-0000-0000-000000000003` (distinct from E2E `001` and integration `002`)
- **Build tag:** `//go:build quality` on all Go files
- **Sequential execution** — one email at a time (LLM calls are slow)
- **Single setup** — fixtures loaded once, each email gets unique `source_tag`
- **Helpers duplicated from E2E** — can't import across build tags; benchmark tier set this precedent
