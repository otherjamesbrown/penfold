# Task: Add LLM-as-judge score reading to newsletter and notification eval tests

**Task ID:** pf-2cb3e7
**Agent:** 

## Task Content

## Wave 3 — depends on pf-056eaf (GetScores)

Update the existing newsletter and notification eval tests to read LLM-as-judge scores from Langfuse evaluators after pipeline processing. This gives LLM-as-judge coverage across all 3 content categories.

## What to add

In both `tests/quality/newsletter_eval_test.go` and `tests/quality/notification_eval_test.go`, after the existing `RecordResult` call:

```go
// LLM-as-judge: read Langfuse evaluator scores
if lfEval != nil {
    time.Sleep(10 * time.Second)
    scores, err := lfEval.GetScores(ctx, traceID)
    if err == nil {
        for _, score := range scores {
            t.Logf("  langfuse.%s: %.1f", score.Name, score.Value)
            if score.Value < 3.0 {
                t.Errorf("langfuse.%s: score %.1f below threshold 3.0", score.Name, score.Value)
            }
        }
    }
}
```

Note: Soft-assert pattern — if no scores returned (evaluators not yet configured in Langfuse UI), log a warning but don't fail. This avoids breaking the test suite before evaluators are configured.

## Code locations
- `tests/quality/newsletter_eval_test.go` — ~12 lines added after RecordResult
- `tests/quality/notification_eval_test.go` — same change

## Acceptance criteria
- [ ] Both test files compile with the new GetScores call
- [ ] LLM-as-judge scores are logged when present
- [ ] Tests don't fail when Langfuse evaluators are not configured (empty scores list)
- [ ] Scores below 3.0 are flagged as test errors when evaluators are configured
- [ ] `go build ./tests/quality/...` passes

## Design Context (from pf-71f660)

**Eval Framework Phase 3 — standard email evals + LLM-as-judge**

# Eval Framework Phase 3 — Standard Email Evals + LLM-as-Judge

## Problem

Standard human emails (the largest content category) have no eval coverage in the new framework. The legacy `TestQuality_ExtractionAccuracy` test exists but doesn't follow the eval framework pattern — no L1 routing checks, no Langfuse recording, no `EvalResults`, and it can't run alongside the category-specific eval tests (newsletter, notification) because it uses a different setup/teardown pattern.

Additionally, Phases 1-2 only use deterministic matchers (substring, count bounds). For subjective quality questions like "is this summary useful?" or "did the extraction capture the key business meaning?", we need LLM-as-judge evaluation. This should cover all content categories (standard, newsletter, notification), not just standard emails.

## What Exists

### Golden files (4, root of golden/ directory)
- `002-incident-response.yaml` — P0 incident, HIGH importance, issues + people
- `011-risk-escalation.yaml` — 3 explicit risks with people and projects
- `012-low-priority-fyi.yaml` — Negative test: LOW importance, no assertions expected
- `013-thread-with-decisions.yaml` — 4 labeled decisions with thread context

### Fixtures (13 .eml files, only 4 have golden files)
001-project-update, 002-incident-response, 003-meeting-invite, 004-code-review, 005-project-kickoff, 006-sales-update, 007-documentation, 008-security-review, 009-mobile-update, 010-postmortem, 011-risk-escalation, 012-low-priority-fyi, 013-thread-with-decisions

### Matchers (in matchers.go)
- `MatchTriage()` — importance + category (calls t.Error, no MatchDetail)
- `MatchPeople()` — min/max count, must_find/must_not_find by name
- `MatchAssertions()` — by type + description_contains
- `MatchProjects()` — by name
- `MatchPipelineStages()` — stage completion

### Standard pipeline (15 stages in prod)
parse → triage → summarize → extract_ner → extract_assertions → attribute_project → instruction_evaluate → extract_semantic → resolve → enrich_entities → analyze → persist → embed

Key difference from newsletter/notification: standard emails extract to the `assertions` and `people` tables, not to `content_enrichment.extracted_data` JSON.

### Langfuse infrastructure
- Self-hosted Langfuse 3 on dev02:3000
- `CreateScore()` API implemented in `pkg/langfuse/client.go`
- `langfuse_eval.go` records L1/L2/L3 scores on pipeline traces
- Datasets per category (`eval-newsletter`, `eval-notification`)

## Design

### Part A: Migrate standard email evals to new framework

#### A1. Move golden files into `golden/standard/` subdirectory

Relocate existing 4 files and add `routing:` section + `category: standard`:

```yaml
email: emails/002-incident-response.eml
description: "P0 incident report — API Gateway degradation affecting 15% of requests"
last_verified: "2026-03-26"
category: standard

routing:
  content_subtype: HUMAN
  pipeline: standard
  must_complete: [parse, triage, extract_ner, extract_assertions, extract_semantic, embed]
  must_not_run: [newsletter_extract]

triage:
  importance:
    one_of: [HIGH, CRITICAL]
  category:
    one_of: [RISK_ISSUE, INCIDENT, OPERATIONAL]

people:
  min_count: 2
  must_find:
    - name_contains: "Daniel"

assertions:
  min_count: 1
  must_find:
    - type: issue
      description_contains: "API"
```

#### A2. Add 4 new golden files (8 total)

**001-project-update.yaml** — Project Alpha MVP status, Q1 deadline risk
- Triage: MEDIUM, PROJECT_UPDATE
- People: John, Sarah (min 2)
- Assertions: risk (Q1 deadline), action (sync meeting)

**005-project-kickoff.yaml** — ML Tiger Team formation, executive-approved
- Triage: HIGH, PROJECT_UPDATE/DECISION
- People: Brandon, Jessica, Lisa (min 3)
- Assertions: decision (Tiger Team approved), action (kickoff meeting)

**008-security-review.yaml** — Annual security audit, 2 P1 critical items
- Triage: HIGH/CRITICAL, RISK_ISSUE/SECURITY
- People: Daniel, Emily, Rob (min 3)
- Assertions: risk (P1 auth), risk (P1 CI/CD), action (ADR). Must NOT flag K8s as issue (passed audit).

**010-postmortem.yaml** — P0 API Gateway incident, 3 enterprise SLA breaches
- Triage: HIGH/CRITICAL, INCIDENT
- People: Robert, Dan, Steph (min 3)
- Assertions: issue (SLA breach), issue (autoscaling root cause), action (runbook update), action (gradual rollout)

#### A3. Create `standard_eval_test.go`

Same pattern as newsletter/notification:
1. Setup: EnsureTenantExists, CleanupTestTenant, SeedClassificationRules
2. Discover golden files from `golden/standard/`
3. For each: ingest → kick → wait → L1 routing → L2 quality → Langfuse recording

#### A4. Refactor existing matchers to return `MatchDetail`

Wrap `MatchTriage`, `MatchPeople`, `MatchAssertions`, `MatchProjects` to produce structured results for Langfuse scoring. Create `MatchStandardExtract()` that orchestrates all four and returns `[]MatchDetail`.

#### A5. Seed standard pipeline for eval tenant

Add to `SeedClassificationRules`:
- Pipeline routing: `HUMAN` → `standard`
- Pipeline definitions: 15 stages matching prod (stage_order, stage_kind, timeouts)

Note: HUMAN is the default subtype when no classification rules match, so no classification rules needed — just routing and definitions.

#### A6. Retire legacy test runner

Remove `TestQuality_ExtractionAccuracy` from `quality_test.go` once `TestEval_Standard` covers all 8 golden files.

### Part B: LLM-as-Judge via Langfuse Evaluators

#### Approach: Langfuse built-in evaluators

Use Langfuse's native evaluator feature rather than calling Claude directly from test code. This means:

1. **Define evaluator templates in Langfuse UI** (dev02:3000) — each template specifies a prompt, scoring rubric, and target model
2. **Langfuse runs evaluators automatically** on matching traces — when a pipeline trace is created, Langfuse evaluates it and records scores
3. **Our test code reads scores** via the existing `GetTraces()` API — no LLM calls from Go code
4. **Evaluator prompts are tunable from the UI** — no code changes needed to adjust evaluation criteria

#### Model: Claude Sonnet

Use `claude-sonnet-4-6` for LLM-as-judge. Sonnet provides better judgment quality than Haiku for nuanced extraction evaluation, and the cost is acceptable since evaluators only run on eval traces (not production volume).

Langfuse needs an Anthropic API key configured as an LLM provider to call Claude. Check if this is already set up in the Langfuse instance.

#### Evaluator definitions (configure in Langfuse UI)

**Evaluator 1: Extraction Completeness** (all categories)
```
Name: extraction_completeness
Model: claude-sonnet-4-6
Score: 1-5 scale
Trigger: traces with tag "eval-*"

Prompt:
You are evaluating an email processing pipeline's extraction quality.

## Email Content
{{input}}

## Extracted Data
{{output}}

Rate the extraction completeness on a 1-5 scale:
1 - Major items missing, extraction is not useful
2 - Several important items missing
3 - Key items captured but some gaps
4 - Nearly complete, minor omissions only
5 - All important items captured accurately

Score only the COMPLETENESS — whether important facts from the email
appear in the extraction. Do not penalise for extra information.

Return your score as a single integer (1-5) on the first line,
followed by a brief explanation.
```

**Evaluator 2: Triage Accuracy** (all categories)
```
Name: triage_accuracy
Model: claude-sonnet-4-6
Score: 1-5 scale
Trigger: traces with tag "eval-*"

Prompt:
You are evaluating whether an email was triaged correctly.

## Email Content
{{input}}

## Triage Result
Importance: {{importance}}
Category: {{category}}

Rate the triage accuracy on a 1-5 scale:
1 - Completely wrong importance AND category
2 - One dimension (importance or category) is significantly wrong
3 - Roughly correct but could be better calibrated
4 - Good classification, minor quibble at most
5 - Perfect classification

Return your score as a single integer (1-5) on the first line,
followed by a brief explanation.
```

**Evaluator 3: Summary Usefulness** (newsletter + standard)
```
Name: summary_usefulness
Model: claude-sonnet-4-6
Score: 1-5 scale
Trigger: traces with tag "eval-newsletter-*" or "eval-standard-*"

Prompt:
You are evaluating whether a content summary would be useful to a
VP of Products who manages multiple teams and projects.

## Original Content
{{input}}

## Generated Summary
{{summary}}

Rate usefulness on a 1-5 scale:
1 - Summary is misleading or useless
2 - Summary exists but misses the key points
3 - Captures main topic but lacks actionable detail
4 - Good summary, highlights what matters to an executive
5 - Excellent — concise, actionable, highlights risks and decisions

Return your score as a single integer (1-5) on the first line,
followed by a brief explanation.
```

#### Integration with test harness

The Go test code doesn't call the LLM — it reads Langfuse scores after processing:

```go
// After pipeline completes and deterministic checks run:
if lfEval != nil {
    // Wait briefly for Langfuse evaluators to run
    time.Sleep(10 * time.Second)

    // Read LLM-as-judge scores from Langfuse
    scores, err := lfEval.GetScores(ctx, traceID)
    if err == nil {
        for _, score := range scores {
            t.Logf("  langfuse.%s: %.1f", score.Name, score.Value)
            if score.Value < 3.0 {
                t.Errorf("langfuse.%s: score %.1f below threshold 3.0", score.Name, score.Value)
            }
        }
    }
}
```

This requires adding `GetScores()` to the Langfuse client — a simple GET on `/api/public/scores?traceId={id}`.

#### Coverage: all categories

LLM-as-judge evaluators run on ALL eval traces (newsletter, notification, standard) via the `eval-*` tag trigger. Category-specific evaluators (like summary_usefulness) use more specific tag patterns.

### Part C: Implementation order

| Wave | What | Depends on |
|------|------|------------|
| 1 | Move golden files to `golden/standard/`, add routing sections | Nothing |
| 1 | Add 4 new golden files | Nothing |
| 2 | Refactor matchers to return MatchDetail | Wave 1 |
| 2 | Seed standard pipeline definitions | Nothing |
| 2 | Configure Langfuse LLM provider (Anthropic API key) | Nothing |
| 3 | Create `standard_eval_test.go` | Waves 1-2 |
| 3 | Define Langfuse evaluator templates in UI | Wave 2 (LLM provider) |
| 3 | Add `GetScores()` to Langfuse client | Nothing |
| 4 | Integrate LLM-as-judge scores into eval tests | Wave 3 |
| 4 | Retire legacy `quality_test.go` | Wave 3 |
| 5 | Integration test: full run all categories | Waves 1-4 |

## Dependencies

- Phase 1/2 infrastructure — done
- Worker running with standard pipeline — blocked by pf-b36282 (PreClassify duplicate, filed)
- Anthropic API key in Langfuse — needs verification/setup
- Standard pipeline routing for eval tenant — Part A5

## Success Criteria

- [ ] 8 standard email golden files with routing + extraction expectations
- [ ] `TestEval_Standard` runs all golden files with L1 + L2 + Langfuse recording
- [ ] Existing matchers produce `MatchDetail` for scoring
- [ ] Standard pipeline definitions seeded for eval tenant
- [ ] Langfuse evaluators configured: extraction_completeness, triage_accuracy, summary_usefulness
- [ ] LLM-as-judge scores (from Langfuse evaluators) read and asserted in eval tests
- [ ] LLM-as-judge covers all 3 categories (standard, newsletter, notification)
- [ ] Legacy `quality_test.go` retired

## Scope Boundaries

**Not included:**
- CLIC thread evaluation (cross-email assertion rollup) — future phase
- Digest contribution (L3) — Phase 4
- Automated scheduling/CI — Phase 4
- Real James email fixtures (staying with Acme Corp synthetic for standard emails)
- Langfuse evaluator config API in Go SDK (configure via UI instead)


---
*Appended by agent-mycroft at 2026-03-26 18:48 UTC*

## Auto-completion evidence

Commit: 438d2839 [pf-71f660] Auto-commit remaining changes
PR: https://github.com/otherjamesbrown/penfold/pull/80

### Files changed
```
.cobuild/last-prompt.md | 907 +++++++++++++++++++++++++++++++++++++++++++++---
 1 file changed, 868 insertions(+), 39 deletions(-)
```

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-2cb3e7`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.

**IMPORTANT RULES:**
- NEVER use raw `git merge` or `git push` to main — always use `cobuild complete` which creates a PR
- NEVER merge PRs yourself — the orchestrating agent handles merge via `cobuild merge` after review
- If a reviewer (Gemini, human) leaves a critical comment on your PR, you MUST address it before the PR can merge
- Check review comments: `gh pr view <pr-number> --comments`
