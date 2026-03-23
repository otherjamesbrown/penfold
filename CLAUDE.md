# Task: Integration test: notification category eval framework end-to-end

**Task ID:** pf-9c7494
**Agent:** agent-mycroft

## Task Content

## Scope
End-to-end validation that all Phase 2 notification eval components work together: types parse correctly, golden YAML files load, matchers execute against real pipeline output, Langfuse scores are recorded.

### Test Plan

1. **Type compilation check**: Verify all notification types compile and golden YAML files deserialize correctly into the extended GoldenExpectation struct.

2. **Golden file coverage check**: Verify all 9 notification golden files exist and cover all 4 handling modes:
   - triage_once: 3 files (Aha to-dos, Bitmovin, Internal A360)
   - daily_summary: 4 files (Aha digest x2, Jira, Google)
   - compliance_status: 1 file (Oracle Learning)
   - immediate_escalate: 1 file (GlobalSecOps)

3. **Matcher unit test validation**: Run notification_matchers_test.go and verify all table-driven tests pass.

4. **Full eval run**: Execute TestEval_Notification against real DB with all 9 golden files:
   - All L1 routing assertions execute (content_subtype=NOTIFICATION, correct pipeline, correct stages)
   - All L2 triage assertions execute (importance calibrated per source, MustNotBe enforced)
   - All L2 notification extraction assertions execute (handling mode validated per source)
   - EvalResults populated with MatchDetail records for each check

5. **Langfuse recording validation**: Verify that after eval run:
   - Dataset "eval-notification" exists in Langfuse
   - Run items created for each golden file
   - Scores recorded: routing_correct, triage_calibration, notification_extraction per item

6. **Cross-category compatibility**: Run both TestEval_Newsletter and TestEval_Notification together to verify no interference:
   go test -tags=quality ./tests/quality/... -run TestEval_ -v

### Acceptance Criteria
- [ ] All 9 notification golden files parse without error
- [ ] All handling modes covered by at least one golden file
- [ ] Matcher unit tests pass
- [ ] Full eval run completes for all 9 items
- [ ] Langfuse scores recorded for all items
- [ ] Newsletter and notification evals run together without conflict
- [ ] MustNotBe enforcement works (non-escalation items must_not_be HIGH/CRITICAL)

### Code Locations
- tests/quality/notification_eval_test.go — TestEval_Notification
- tests/quality/notification_matchers_test.go — unit tests
- tests/quality/golden/notification/*.yaml — 9 golden files

### Dependencies
- pf-20233c (types)
- pf-2e00e9 (golden files)
- pf-b8ed97 (matchers)
- pf-fd89f5 (test runner)

## Design Context (from pf-4d2288)

**Content Category Eval Framework — Langfuse-driven quality validation**

# Content Category Eval Framework

## Problem

We're in a reactive loop: fix one content category, discover issues in the next, fixes cascade. No systematic way to validate quality per content type or catch regressions over time. We need an eval framework that lets us define expected behaviour per content category, validate pipeline output against those expectations, and detect drift.

## Design Principles

- **One category at a time.** Each content category (newsletter, notification, human email, transcript) gets its own eval suite with James defining expected behaviour first.
- **Grade outputs, not paths.** We care about what was extracted, not which internal function was called (per Anthropic guidance).
- **Start from real content.** Eval cases come from actual processed content with James's judgment on what the output should be, not synthetic data.
- **Three eval levels.** Deterministic routing checks, LLM-as-judge quality scoring, digest contribution validation.
- **Build on what exists.** Extend golden YAML pattern + Langfuse Dataset API already in codebase.

## Architecture

### Eval Levels

| Level | What | How | Speed |
|-------|------|-----|-------|
| **L1: Routing** | Content classified correctly, enters right pipeline, runs expected stages | Deterministic assertions — content_subtype, pipeline name, stage completion list | Fast, no LLM |
| **L2: Stage Quality** | Triage importance, extraction completeness, summary accuracy, field population | LLM-as-judge via Langfuse evaluators + golden YAML semantic matchers | Slow, LLM cost |
| **L3: Digest Contribution** | Category data feeds into rollups with expected fields | Deterministic — expected fields present in digest gather output | Medium, no LLM |

### Components

```
┌─────────────────────────────────────────────────┐
│                 Eval Runner                      │
│  (Go test harness, extends quality test tier)    │
├─────────────────────────────────────────────────┤
│                                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │ Category │  │ Category │  │ Category │  ...  │
│  │Newsletter│  │Notif.    │  │Human     │      │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘      │
│       │              │              │            │
│  ┌────▼─────────────▼──────────────▼────┐       │
│  │        Golden YAML per category       │       │
│  │  (routing + stage quality + digest)   │       │
│  └────┬──────────────────────────────────┘       │
│       │                                          │
│  ┌────▼─────────────────────────────────┐       │
│  │     Langfuse Dataset + Scoring        │       │
│  │  - Dataset per category               │       │
│  │  - Run item per eval execution        │       │
│  │  - Scores per eval level              │       │
│  │  - Drift tracking over time           │       │
│  └───────────────────────────────────────┘       │
└─────────────────────────────────────────────────┘
```

### Golden YAML — Extended for Categories

Extend the existing golden YAML format (tests/quality/golden/) with category-specific fields. Each content category gets its own directory:

```
tests/quality/golden/
  newsletter/
    001-akamai-wave.yaml
    002-ctg-post-its.yaml
    003-emea-newsletter.yaml
  notification/
    001-jira-update.yaml
    002-aha-reminder.yaml
    003-github-pr.yaml
  standard/
    002-incident-response.yaml    # (existing)
    011-risk-escalation.yaml      # (existing)
  transcript/
    001-team-standup.yaml
```

#### Newsletter golden YAML example

```yaml
email: emails/newsletter-akamai-wave-nov2025.eml
description: "Akamai Wave — company newsletter with wellness, Q3 results, HR updates"
last_verified: "2026-03-18"
category: newsletter

# L1: Routing assertions
routing:
  content_subtype: NEWSLETTER
  pipeline: newsletter
  must_complete: [parse, triage, newsletter_extract, embed]
  must_not_run: [extract_semantic, extract_ner, extract_assertions]

# L2: Stage quality
triage:
  importance:
    one_of: [LOW, MEDIUM]
  category:
    one_of: [fyi, company_news]

newsletter_extract:
  executive_summary:
    min_length: 50
    must_mention: ["wellness", "Q3"]
  risks:
    max_count: 0
  action_items:
    min_count: 0
    max_count: 2
  key_announcements:
    min_count: 2
    must_find:
      - description_contains: "wellness"
  quality_gate_triggered: false

# L3: Digest contribution
digest:
  handling: standalone_summary    # or: group_with, discard
  fields_in_rollup:
    - key_announcements
    - action_items
  summary_min_length: 30

# Behaviour spec (James's intent)
intent:
  description: "Company newsletter — summarise key announcements, surface action items. Include in weekly digest as standalone entry."
  value: medium
```

#### Notification golden YAML example

```yaml
email: emails/notification-aha-daily-reminder.eml
description: "Aha! daily reminder — low-value system notification"
category: notification

routing:
  content_subtype: NOTIFICATION
  notification_source: aha
  pipeline: notification
  must_complete: [parse, triage, summarize, extract_ner, extract_semantic, embed]

triage:
  importance:
    one_of: [LOW, MEDIUM]
    must_not_be: [HIGH, CRITICAL]
  category:
    one_of: [notification, system_update]

extract_semantic:
  prompt_version: 2

intent:
  description: "System notification — batch into daily rollup, never surface as individual item."
  value: low
```

### Langfuse Integration

#### Datasets

One Langfuse dataset per content category:

```
Dataset: "eval-newsletter"
  Items:
    - input: {source: "newsletter-akamai-wave-nov2025.eml", golden: "001-akamai-wave.yaml"}
    - input: {source: "newsletter-ctg-post-its.eml", golden: "002-ctg-post-its.yaml"}

Dataset: "eval-notification"
  Items: ...
```

#### Experiment Runs

Each eval execution creates a Langfuse run linked to the pipeline trace:

```go
client.CreateDatasetRunItem(ctx, &CreateDatasetRunItemRequest{
    DatasetItemID: item.ID,
    TraceID:       contentTraceID,
    RunName:       "eval-newsletter-2026-03-18",
    Metadata: map[string]interface{}{
        "l1_routing_pass": true,
        "l2_quality_score": 0.85,
        "l3_digest_pass":   true,
        "worker_version":   commitSHA,
        "prompt_versions":  map[string]int{"newsletter_extract": 2, "triage": 3},
    },
})
```

#### Scoring

Extend Langfuse client with scores API to record per-level scores on traces:

```go
// L1: Binary pass/fail
client.CreateScore(ctx, traceID, "routing_correct", 1.0, "")

// L2: Quality score
client.CreateScore(ctx, traceID, "extraction_quality", 0.85,
    "Missing 1 of 3 expected key_announcements")

// L3: Digest contribution
client.CreateScore(ctx, traceID, "digest_contribution", 1.0, "")
```

Enables Langfuse dashboards showing quality trends per category over time.

### LLM-as-Judge Evaluators

Two approaches, used together:

**1. Semantic matchers (existing pattern, extended)**
- `must_mention` — case-insensitive substring
- `one_of` — value in set
- `min_count/max_count` — count bounds
- `description_contains` — substring match on structured fields

**2. LLM-as-judge (new, for subjective quality)**

```yaml
newsletter_extract:
  executive_summary:
    llm_judge:
      prompt: |
        Rate this newsletter summary on a 1-5 scale:
        - Does it capture the key topics?
        - Is it concise but complete?
        - Would an executive find it useful?
      min_score: 3
```

Use sparingly — most checks should be deterministic.

### Eval Runner

Extends existing quality test tier. New test function per category:

```go
func TestEval_Newsletter(t *testing.T) {
    goldenFiles := discoverGoldenFiles(t, "newsletter")
    for _, gf := range goldenFiles {
        t.Run(gf.Name, func(t *testing.T) {
            sourceID := ingestEmail(t, gf.EmailPath)
            waitForComplete(t, sourceID)
            results := &EvalResults{}

            // L1: Routing
            results.L1 = assertRouting(t, sourceID, gf.Routing)

            // L2: Stage quality
            results.L2 = assertStageQuality(t, sourceID, gf)

            // L3: Digest contribution
            results.L3 = assertDigestContribution(t, sourceID, gf.Digest)

            // Record to Langfuse
            recordEvalToLangfuse(t, sourceID, gf, results)
        })
    }
}
```

Run per category:
```bash
go test -tags=quality ./tests/quality/... -run TestEval_Newsletter -v
go test -tags=quality ./tests/quality/... -run TestEval_ -v
```

## Workflow: Adding a New Category

1. **James reviews real content** — 5-10 examples, defines intent for each (discard / group / standalone summary)
2. **Penfold drafts golden YAML** — from James's intent + actual pipeline output
3. **Run evals** — identifies gaps between intent and reality
4. **Fix pipeline/prompts** — address specific failures
5. **Re-run evals** — confirm fixes
6. **Record to Langfuse** — baseline scores for drift detection
7. **Move to next category**

## Implementation Plan

### Phase 1: Framework bootstrap (with newsletter)
- Extend golden YAML types for routing, newsletter_extract, digest fields
- Add newsletter golden directory + 3-5 newsletter .eml fixtures
- Add routing assertion matcher
- Add newsletter_extract assertion matcher
- Create Langfuse scoring client extension (CreateScore)
- Create eval-newsletter Langfuse dataset
- Wire eval runner for newsletter category
- **Depends on:** worker connectivity fixed, newsletter_extract actually runs

### Phase 2: Notification evals
- Add notification golden directory + fixtures
- Triage calibration assertions (importance per notification_source)
- Prompt version assertion (verify v2 applied)

### Phase 3: Standard email evals
- Migrate existing 4 golden files into new structure
- Extraction quality matchers for entities, risks, projects
- CLIC thread eval

### Phase 4: Transcript + digest + RAG evals
- Transcript-specific matchers
- Cross-category digest quality evals
- Search relevance evals

## Key Files to Create/Modify

**New (penfold repo):**
- `tests/quality/golden/newsletter/*.yaml`
- `tests/quality/golden/notification/*.yaml`
- `tests/quality/newsletter_eval_test.go`
- `tests/quality/notification_eval_test.go`
- `tests/quality/routing_matchers.go`
- `tests/quality/category_matchers.go`
- `tests/quality/digest_matchers.go`
- `tests/quality/langfuse_eval.go`
- `tests/fixtures/acme-corp/emails/newsletter-*.eml`

**Modify (penfold repo):**
- `tests/quality/types.go` — extend GoldenExpectation
- `tests/quality/matchers.go` — add MatchRouting, MatchNewsletterExtract
- `pkg/langfuse/client.go` — add CreateScore method

## What This Does NOT Cover

- **Content handling rules** (discard/group/standalone) — need James's input per item. The `intent` field captures this, but pipeline doesn't implement grouping yet.
- **Automated CI/CD gating** — v1 is manual `go test`. CI integration is future.
- **Production monitoring** — offline eval, not real-time alerting.

## Open Questions

1. **Newsletter grouping** — pipeline doesn't support grouping today. Eval grouping behaviour now, or defer?
2. **Real vs fixture content** — real James emails (anonymised) or synthetic Acme Corp? Real content tests classification rules more honestly.
3. **LLM-as-judge model** — same model as pipeline (gemini-2.5-flash) or independent (Claude)?
4. **Eval frequency** — every deploy? Weekly? On-demand for v1?


---
*Appended by agent-penfold at 2026-03-18 19:28 UTC*


---

## Newsletter Category — Intent Specification

Defined with James on 2026-03-18 by reviewing all 18 newsletter items across 8 distinct senders.

### Handling Modes

| Mode | Behaviour |
|------|-----------|
| **friday_summary** | Group into a single "Weekly Newsletter Summary" delivered Friday morning. Highlight any content matching James's projects/products. |
| **conditional** | Scan for project/product relevance. If relevant content found, include in Friday summary. If nothing relevant, discard entirely. |
| **discard** | Skip processing — no extraction, no summary, no digest contribution. |

### Sender → Handling Map

| Sender | Address | Type | Handling | Notes |
|--------|---------|------|----------|-------|
| Post-Its / CTG | ctgcomms@akamai.com | Division newsletter | friday_summary | Bi-weekly, 4 issues. Key updates, job postings, program wins. |
| Akamai Wave | AkamaiWave@akamai.com | Company newsletter | friday_summary | Monthly, 2 issues. CEO updates, product launches, HR/wellness. |
| EMEA Newsletter | EMEA_newsletter@akamai.com | Regional newsletter | friday_summary | Monthly, 2 issues. Events, spotlights, TA updates. |
| AkamaiSpark Quarterly | AkamaiSpark@akamai.com | Employee engagement | friday_summary | Quarterly. Wellness, volunteering, ERGs, benefits. |
| Talent Zone | TA-EMEA@akamai.com | Recruitment | friday_summary | Open roles, referral bonuses. |
| Spark / Wellness | AkamaiWellness@akamai.com | Internal comms | friday_summary | Edge case — Spark is internal comms, grouped as newsletter. |
| Dynamic Signal | dynamicsignal@akamai.com | Automated digest | conditional | Weekly automated social feed. Usually low-value noise. Only include if project/product relevant. |
| Eng Learning | EngLearn@akamai.com | Training | discard | Stub email with link to Aloha — no extractable content. |

### Extraction Requirements (for friday_summary items)

1. **Key announcements** — what's new or changing
2. **Action items** — deadlines, required actions (e.g. performance cycle, survey completion)
3. **Project/product relevance** — flag any mention of James's projects or products (matched against known entities)
4. **Executive summary** — 2-3 sentence overview of the newsletter

### Friday Weekly Newsletter Summary Output

The summary should:
- Combine all newsletters received that week into one narrative
- Lead with project/product-relevant highlights (if any)
- Group remaining content by theme (people, culture, business updates, action items)
- Be concise — one page max, not a wall of text

### Test Content Inventory (18 items)

| Source ID | Sender | Subject | Handling |
|-----------|--------|---------|----------|
| 3381 | EMEA_newsletter | EMEA Newsletter - December | friday_summary |
| 3382 | EngLearn | Engineering Learning Newsletter Q4 | discard |
| 3394 | ctgcomms | Post-Its Dec 17 | friday_summary |
| 3399 | AkamaiSpark | Quarterly Newsletter Dec | friday_summary |
| 3483 | AkamaiWave | Wave - Nov 2025 | friday_summary |
| 3795 | AkamaiWave | Wave - Year in Review Dec | friday_summary |
| 3796 | EMEA_newsletter | EMEA Newsletter - November | friday_summary |
| 3797 | ctgcomms | Post-Its Dec 3 | friday_summary |
| 3798 | AkamaiWellness | Mental Health / Spark | friday_summary |
| 3799 | ctgcomms | Post-Its Nov 19 | friday_summary |
| 3800 | ctgcomms | Post-Its Nov 5 | friday_summary |
| 3801 | TA-EMEA | Talent Zone Newsletter | friday_summary |
| 3802 | dynamicsignal | Dynamic Signal Digest Dec 15 | conditional |
| 3803 | dynamicsignal | Dynamic Signal Digest Dec 8 | conditional |
| 3804 | dynamicsignal | Dynamic Signal Digest Nov 3 | conditional |
| 3805 | dynamicsignal | Dynamic Signal Digest Nov 17 | conditional |
| 3806 | dynamicsignal | Dynamic Signal Digest Nov 10 | conditional |
| 3807 | dynamicsignal | See what you've been missing | conditional |

---
*Appended by agent-penfold at 2026-03-18 19:55 UTC*


---

## Notification Category — Intent Specification

Defined with James on 2026-03-18 by reviewing 10 notification items across 6 distinct sources.

### Handling Modes

| Mode | Behaviour |
|------|-----------|
| **immediate_escalate** | Surface immediately. Escalate to WhatsApp. For urgent/security content requiring immediate attention. |
| **triage_once** | Surface once when new for "mine or delegated?" decision. Once triaged as delegated → suppress individuals, roll into daily summary. |
| **daily_summary** | Never surface individually. Batch into end-of-day activity summary. |
| **compliance_status** | Group all related items into a single status view by person + deadline. Surface once when new, weekly reminder until completed. |

### Source → Handling Map

| Source | Address | Type | Handling | Notes |
|--------|---------|------|----------|-------|
| Aha! daily to-dos | *-akamai@iad-prod1.mailer.aha.io | Task management | triage_once | New items → surface for triage. Delegated → suppress, daily summary only. Same overdue items repeat daily — suppress after first triage. |
| Aha! hourly digest / updates | support@aha.io | Project activity | daily_summary | "3 updates on MTC outcomes today: Patrick updated X, Patti updated Y". Never individual. |
| Jira (TRACK-JIRA) | gsd-jira@akamai.com | Project activity | daily_summary | Same as Aha updates. Status changes, assignee changes, resolutions. |
| Oracle Learning | oracle-hcm-prod@akamai.com | Compliance training | compliance_status | Group into "Security and Compliance Training" view: who needs to do what by when. James + direct reports. Surface once when new, weekly reminder. |
| Google sign-in alerts | no-reply@accounts.google.com | Security/routine | daily_summary | Group with compliance/routine notifications into end-of-day summary. |
| Corporate security (GlobalSecOps) | globalsecops@akamai.com | Security/urgent | **immediate_escalate** | **Escalate to WhatsApp.** Malicious DNS, security incidents — require immediate attention. |
| Action Required (vendor/internal) | various | Task management | triage_once | Surface once for "mine or delegated?" Likely delegated → daily summary after triage. |

### Extraction Requirements

**For daily_summary items:**
- Who did what on which project/epic
- Status changes, date changes, resolutions
- Aggregate by project: "MTC Outcomes: 3 updates (Patrick, Patti, Ilan)"

**For triage_once items:**
- Task description, assignee, due date
- Is this new or a repeat reminder?
- Project context

**For compliance_status items:**
- Training course name
- Person assigned (James or direct report)
- Due date / overdue status
- Completion status

**For immediate_escalate items:**
- Threat description
- Machine/system affected
- Required response
- Urgency level

### Key Design Insight: Delegated Work Pattern

James is a project lead — many Aha/Jira items are assigned to him but executed by others. The system needs to:
1. Recognise first-time assignment → surface for triage
2. Accept "delegated" triage decision → suppress future individual notifications for that item
3. Continue tracking delegated items in daily summary (activity feed, not action items)

### Test Content Inventory

| Source ID | Source | Subject | Handling |
|-----------|--------|---------|----------|
| 3413 | Aha! to-dos | Daily to-dos reminder | triage_once |
| 3410 | Aha! digest | Recent changes to Compute Feedback Engine | daily_summary |
| 3412 | Aha! digest | Recent changes to Compute Feedback Engine | daily_summary |
| 3808 | Jira | TRACK-JIRA AHASUP-247 updates | daily_summary |
| 3810 | Oracle Learning | Anti-Bribery Training overdue | compliance_status |
| 3409 | Google | Security alert sign-in | daily_summary |
| 3812 | GlobalSecOps | Malicious DNS request | immediate_escalate |
| 3811 | Bitmovin | Action Required: New Player UI | triage_once |
| 3809 | Internal | ACTION REQUESTED: A360 Cleanup | triage_once |

---
*Appended by agent-penfold at 2026-03-23 20:20 UTC*

## Open Questions — Resolved (2026-03-23)

### Q1: Newsletter grouping — DEFER
Newsletter grouping (combining multiple newsletters into one digest) is a digest rollup concern, not an eval framework concern. The eval framework tests individual newsletter extraction quality. Grouping behaviour is tested at the digest level (Phase 4). Create a tracking shard if needed.

### Q2: Real vs fixture content — REAL CONTENT
Use real James emails. We already have 17 newsletter sources in the DB with known-good extraction results. Synthetic content doesn't test classification rules honestly. Anonymisation not needed for internal dev testing.

### Q3: LLM-as-judge model — DEFER
LLM-as-judge is out of scope for Phase 1. Phase 1 uses deterministic matchers (field presence, count bounds, substring matching). LLM-as-judge is Phase 3+ when we need to evaluate extraction quality subjectively (e.g. 'is this summary useful?'). When implemented, use an independent model (Claude) to avoid self-evaluation bias.

### Q4: Eval frequency — ON-DEMAND FOR V1
Run evals on-demand via `go test -tags=quality` for v1. Frequency automation (per-deploy, weekly) is a Phase 4 concern. The eval framework just needs to produce Langfuse scores — scheduling is separate.

## Scope Clarifications

### L3 digest contribution — DEFERRED
L3 (digest contribution validation) is out of scope for Phase 1. Phase 1 covers L1 (routing) and L2 (extraction quality). L3 requires the digest rollup to run, which is a separate pipeline. Track as Phase 4.

### LLM-as-judge — DEFERRED
Out of scope for Phase 1. Deterministic matchers only. Track as Phase 3.

### Phases 2-4 — DEFERRED WITH TRACKING
Phase 2 (notifications), Phase 3 (standard email + LLM-as-judge), Phase 4 (transcript + digest + scheduling) are defined in the design but not decomposed into tasks. They will be decomposed when Phase 1 is proven and the intent specifications for those categories are written.

---
*Appended by agent-mycroft at 2026-03-23 20:57 UTC*

## Decomposition Review — Phase 2 (Notification Category Evals)
*Reviewed by M at $(date -u '+%Y-%m-%d %H:%M UTC')*

### Verdict: APPROVED

5 tasks, clean dependency graph, sensible wave structure. One minor issue and two observations noted below.

---

### Per-Task Assessment

#### pf-20233c — Extend golden YAML types for notification category (Wave 1)
- **Right-sized?** Yes. Single file (types.go), type definitions only. Well-scoped for one session.
- **Acceptance criteria testable?** Yes. Compile check + backward compat + mode coverage are all verifiable.
- **Code locations identified?** Yes. Single file: tests/quality/types.go.
- **No missing decisions?** Minor issue: the spec says to define ActualNotificationExtraction "matching DB schema" but does not specify what the notification pipeline actually stores in content_enrichment. The agent will need to query the DB to discover the schema. This is acknowledged in the approach section ("Check the notification pipeline's actual DB schema") so it is workable, but it means the type definition is partially discovery work. Acceptable.
- **Dependencies correct?** None declared, none needed. Correct.

#### pf-2e00e9 — Notification golden YAML files and .eml fixtures (Wave 2)
- **Right-sized?** Borderline. 9 golden YAML files + 9 .eml fixtures = 18 files. However, these are data files following a template, not complex code. Acceptable for a single session.
- **Acceptance criteria testable?** Yes. File count, handling mode coverage, parsability are all concrete.
- **Code locations identified?** Yes. Two directories specified.
- **No missing decisions?** The golden YAML structure shows `must_complete: [parse, triage, summarize, extract_ner, extract_semantic, embed]` for notifications. But routing_matchers.go shows inferPipeline maps `notification_extract` stage to "notification" pipeline, and a legacy fallback maps `summarize` without `extract_semantic` to "notification". The golden YAML example has both `summarize` AND `extract_semantic` in must_complete, which would make inferPipeline return "standard" not "notification". The agent needs to verify actual pipeline stages from real data, not copy the example verbatim. The approach section says to do this ("Query DB for each source_id to get actual pipeline output"), so this should self-correct.
- **Dependencies correct?** Depends on pf-20233c. Correct — golden files need notification types to parse.

#### pf-b8ed97 — Notification triage calibration and handling mode matchers (Wave 2)
- **Right-sized?** Yes. 2 new files (matchers + tests), 1 small edit (matchOneOf MustNotBe). Parallel-safe with pf-2e00e9.
- **Acceptance criteria testable?** Yes. Compile, unit tests, MatchDetail output are all verifiable.
- **Code locations identified?** Yes. Three files specified.
- **No missing decisions?** The spec says "Each handling mode has at least 3 assertion checks" but the actual checks depend on what the notification pipeline extracts. This couples to pf-20233c's discovery of the DB schema. The agent implementing this task will need the types from pf-20233c to know what fields to assert on. Dependency is correctly declared.
- **Dependencies correct?** Depends on pf-20233c only. Correct. Does NOT depend on pf-2e00e9 (golden files), which is right — matchers don't need golden files to compile and unit test.

#### pf-fd89f5 — Notification eval test runner with Langfuse recording (Wave 3)
- **Right-sized?** Yes. 1 new file, possible small extension to helpers.go. Follows established TestEval_Newsletter pattern closely.
- **Acceptance criteria testable?** Yes. All 7 criteria are concrete and verifiable.
- **Code locations identified?** Yes. 3 files specified.
- **No missing decisions?** No. The pattern is well-established from Phase 1.
- **Dependencies correct?** Depends on pf-20233c, pf-2e00e9, pf-b8ed97. All three are needed. Correct.

#### pf-9c7494 — Integration test: end-to-end (Wave 4)
- **Right-sized?** This is not a separate implementation task — it is a test plan/checklist to run after all other tasks complete. No new code to write. This is appropriate as the final gate.
- **Acceptance criteria testable?** Yes. 7 concrete checks.
- **Code locations identified?** References files from prior tasks. Correct.
- **No missing decisions?** No.
- **Dependencies correct?** Depends on all 4 prior tasks. Correct.

---

### Overall Assessment

**6. Complete coverage?** Yes. The design's Phase 2 scope is: "Add notification golden directory + fixtures, triage calibration assertions, prompt version assertion." All three are covered. Note: prompt version assertion is not explicitly called out in any task, but it is implicitly covered by the golden YAML routing assertions (must_complete stages verify the pipeline ran correctly, and the extract_semantic prompt_version field from the design example can be included in golden YAML).

**7. Old code removal?** N/A. Phase 2 is additive.

**8. Ordering sensible?** Yes. Types first (Wave 1), then data + matchers in parallel (Wave 2), then test runner (Wave 3), then integration validation (Wave 4). This mirrors the Phase 1 pattern and is logically sound.

**9. Integration test covers all design success criteria?** Yes. pf-9c7494 covers type compilation, golden file coverage, matcher validation, full eval run, Langfuse recording, and cross-category compatibility.

**10. No gaps?** One observation: the design mentions "Prompt version assertion (verify v2 applied)" as a Phase 2 deliverable. None of the 5 tasks explicitly implement a prompt version checker. The golden YAML example in the design shows `extract_semantic: prompt_version: 2` but neither the types (pf-20233c) nor the matchers (pf-b8ed97) mention a prompt version field or matcher. This is a minor gap — it could be added to pf-b8ed97's scope or tracked separately. Not blocking since the notification pipeline may not yet record prompt versions in a queryable way.

### Summary

Clean decomposition. Well-structured dependency graph. Tasks follow established Phase 1 patterns, which reduces implementation risk. The prompt_version assertion gap is minor and can be addressed in-flight. Approved for implementation.

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `cd penfold-go-pipeline && go test ./... && cd penfold-go-pipeline && go vet ./...`
2. Build: `cd penfold-go-pipeline && go build ./...`
3. **Run `cxp task complete pf-9c7494`** — this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.


---

## Design Context (from pf-4d2288)

**Content Category Eval Framework — Langfuse-driven quality validation**

# Content Category Eval Framework

## Problem

We're in a reactive loop: fix one content category, discover issues in the next, fixes cascade. No systematic way to validate quality per content type or catch regressions over time. We need an eval framework that lets us define expected behaviour per content category, validate pipeline output against those expectations, and detect drift.

## Design Principles

- **One category at a time.** Each content category (newsletter, notification, human email, transcript) gets its own eval suite with James defining expected behaviour first.
- **Grade outputs, not paths.** We care about what was extracted, not which internal function was called (per Anthropic guidance).
- **Start from real content.** Eval cases come from actual processed content with James's judgment on what the output should be, not synthetic data.
- **Three eval levels.** Deterministic routing checks, LLM-as-judge quality scoring, digest contribution validation.
- **Build on what exists.** Extend golden YAML pattern + Langfuse Dataset API already in codebase.

## Architecture

### Eval Levels

| Level | What | How | Speed |
|-------|------|-----|-------|
| **L1: Routing** | Content classified correctly, enters right pipeline, runs expected stages | Deterministic assertions — content_subtype, pipeline name, stage completion list | Fast, no LLM |
| **L2: Stage Quality** | Triage importance, extraction completeness, summary accuracy, field population | LLM-as-judge via Langfuse evaluators + golden YAML semantic matchers | Slow, LLM cost |
| **L3: Digest Contribution** | Category data feeds into rollups with expected fields | Deterministic — expected fields present in digest gather output | Medium, no LLM |

### Components

```
┌─────────────────────────────────────────────────┐
│                 Eval Runner                      │
│  (Go test harness, extends quality test tier)    │
├─────────────────────────────────────────────────┤
│                                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │ Category │  │ Category │  │ Category │  ...  │
│  │Newsletter│  │Notif.    │  │Human     │      │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘      │
│       │              │              │            │
│  ┌────▼─────────────▼──────────────▼────┐       │
│  │        Golden YAML per category       │       │
│  │  (routing + stage quality + digest)   │       │
│  └────┬──────────────────────────────────┘       │
│       │                                          │
│  ┌────▼─────────────────────────────────┐       │
│  │     Langfuse Dataset + Scoring        │       │
│  │  - Dataset per category               │       │
│  │  - Run item per eval execution        │       │
│  │  - Scores per eval level              │       │
│  │  - Drift tracking over time           │       │
│  └───────────────────────────────────────┘       │
└─────────────────────────────────────────────────┘
```

### Golden YAML — Extended for Categories

Extend the existing golden YAML format (tests/quality/golden/) with category-specific fields. Each content category gets its own directory:

```
tests/quality/golden/
  newsletter/
    001-akamai-wave.yaml
    002-ctg-post-its.yaml
    003-emea-newsletter.yaml
  notification/
    001-jira-update.yaml
    002-aha-reminder.yaml
    003-github-pr.yaml
  standard/
    002-incident-response.yaml    # (existing)
    011-risk-escalation.yaml      # (existing)
  transcript/
    001-team-standup.yaml
```

#### Newsletter golden YAML example

```yaml
email: emails/newsletter-akamai-wave-nov2025.eml
description: "Akamai Wave — company newsletter with wellness, Q3 results, HR updates"
last_verified: "2026-03-18"
category: newsletter

# L1: Routing assertions
routing:
  content_subtype: NEWSLETTER
  pipeline: newsletter
  must_complete: [parse, triage, newsletter_extract, embed]
  must_not_run: [extract_semantic, extract_ner, extract_assertions]

# L2: Stage quality
triage:
  importance:
    one_of: [LOW, MEDIUM]
  category:
    one_of: [fyi, company_news]

newsletter_extract:
  executive_summary:
    min_length: 50
    must_mention: ["wellness", "Q3"]
  risks:
    max_count: 0
  action_items:
    min_count: 0
    max_count: 2
  key_announcements:
    min_count: 2
    must_find:
      - description_contains: "wellness"
  quality_gate_triggered: false

# L3: Digest contribution
digest:
  handling: standalone_summary    # or: group_with, discard
  fields_in_rollup:
    - key_announcements
    - action_items
  summary_min_length: 30

# Behaviour spec (James's intent)
intent:
  description: "Company newsletter — summarise key announcements, surface action items. Include in weekly digest as standalone entry."
  value: medium
```

#### Notification golden YAML example

```yaml
email: emails/notification-aha-daily-reminder.eml
description: "Aha! daily reminder — low-value system notification"
category: notification

routing:
  content_subtype: NOTIFICATION
  notification_source: aha
  pipeline: notification
  must_complete: [parse, triage, summarize, extract_ner, extract_semantic, embed]

triage:
  importance:
    one_of: [LOW, MEDIUM]
    must_not_be: [HIGH, CRITICAL]
  category:
    one_of: [notification, system_update]

extract_semantic:
  prompt_version: 2

intent:
  description: "System notification — batch into daily rollup, never surface as individual item."
  value: low
```

### Langfuse Integration

#### Datasets

One Langfuse dataset per content category:

```
Dataset: "eval-newsletter"
  Items:
    - input: {source: "newsletter-akamai-wave-nov2025.eml", golden: "001-akamai-wave.yaml"}
    - input: {source: "newsletter-ctg-post-its.eml", golden: "002-ctg-post-its.yaml"}

Dataset: "eval-notification"
  Items: ...
```

#### Experiment Runs

Each eval execution creates a Langfuse run linked to the pipeline trace:

```go
client.CreateDatasetRunItem(ctx, &CreateDatasetRunItemRequest{
    DatasetItemID: item.ID,
    TraceID:       contentTraceID,
    RunName:       "eval-newsletter-2026-03-18",
    Metadata: map[string]interface{}{
        "l1_routing_pass": true,
        "l2_quality_score": 0.85,
        "l3_digest_pass":   true,
        "worker_version":   commitSHA,
        "prompt_versions":  map[string]int{"newsletter_extract": 2, "triage": 3},
    },
})
```

#### Scoring

Extend Langfuse client with scores API to record per-level scores on traces:

```go
// L1: Binary pass/fail
client.CreateScore(ctx, traceID, "routing_correct", 1.0, "")

// L2: Quality score
client.CreateScore(ctx, traceID, "extraction_quality", 0.85,
    "Missing 1 of 3 expected key_announcements")

// L3: Digest contribution
client.CreateScore(ctx, traceID, "digest_contribution", 1.0, "")
```

Enables Langfuse dashboards showing quality trends per category over time.

### LLM-as-Judge Evaluators

Two approaches, used together:

**1. Semantic matchers (existing pattern, extended)**
- `must_mention` — case-insensitive substring
- `one_of` — value in set
- `min_count/max_count` — count bounds
- `description_contains` — substring match on structured fields

**2. LLM-as-judge (new, for subjective quality)**

```yaml
newsletter_extract:
  executive_summary:
    llm_judge:
      prompt: |
        Rate this newsletter summary on a 1-5 scale:
        - Does it capture the key topics?
        - Is it concise but complete?
        - Would an executive find it useful?
      min_score: 3
```

Use sparingly — most checks should be deterministic.

### Eval Runner

Extends existing quality test tier. New test function per category:

```go
func TestEval_Newsletter(t *testing.T) {
    goldenFiles := discoverGoldenFiles(t, "newsletter")
    for _, gf := range goldenFiles {
        t.Run(gf.Name, func(t *testing.T) {
            sourceID := ingestEmail(t, gf.EmailPath)
            waitForComplete(t, sourceID)
            results := &EvalResults{}

            // L1: Routing
            results.L1 = assertRouting(t, sourceID, gf.Routing)

            // L2: Stage quality
            results.L2 = assertStageQuality(t, sourceID, gf)

            // L3: Digest contribution
            results.L3 = assertDigestContribution(t, sourceID, gf.Digest)

            // Record to Langfuse
            recordEvalToLangfuse(t, sourceID, gf, results)
        })
    }
}
```

Run per category:
```bash
go test -tags=quality ./tests/quality/... -run TestEval_Newsletter -v
go test -tags=quality ./tests/quality/... -run TestEval_ -v
```

## Workflow: Adding a New Category

1. **James reviews real content** — 5-10 examples, defines intent for each (discard / group / standalone summary)
2. **Penfold drafts golden YAML** — from James's intent + actual pipeline output
3. **Run evals** — identifies gaps between intent and reality
4. **Fix pipeline/prompts** — address specific failures
5. **Re-run evals** — confirm fixes
6. **Record to Langfuse** — baseline scores for drift detection
7. **Move to next category**

## Implementation Plan

### Phase 1: Framework bootstrap (with newsletter)
- Extend golden YAML types for routing, newsletter_extract, digest fields
- Add newsletter golden directory + 3-5 newsletter .eml fixtures
- Add routing assertion matcher
- Add newsletter_extract assertion matcher
- Create Langfuse scoring client extension (CreateScore)
- Create eval-newsletter Langfuse dataset
- Wire eval runner for newsletter category
- **Depends on:** worker connectivity fixed, newsletter_extract actually runs

### Phase 2: Notification evals
- Add notification golden directory + fixtures
- Triage calibration assertions (importance per notification_source)
- Prompt version assertion (verify v2 applied)

### Phase 3: Standard email evals
- Migrate existing 4 golden files into new structure
- Extraction quality matchers for entities, risks, projects
- CLIC thread eval

### Phase 4: Transcript + digest + RAG evals
- Transcript-specific matchers
- Cross-category digest quality evals
- Search relevance evals

## Key Files to Create/Modify

**New (penfold repo):**
- `tests/quality/golden/newsletter/*.yaml`
- `tests/quality/golden/notification/*.yaml`
- `tests/quality/newsletter_eval_test.go`
- `tests/quality/notification_eval_test.go`
- `tests/quality/routing_matchers.go`
- `tests/quality/category_matchers.go`
- `tests/quality/digest_matchers.go`
- `tests/quality/langfuse_eval.go`
- `tests/fixtures/acme-corp/emails/newsletter-*.eml`

**Modify (penfold repo):**
- `tests/quality/types.go` — extend GoldenExpectation
- `tests/quality/matchers.go` — add MatchRouting, MatchNewsletterExtract
- `pkg/langfuse/client.go` — add CreateScore method

## What This Does NOT Cover

- **Content handling rules** (discard/group/standalone) — need James's input per item. The `intent` field captures this, but pipeline doesn't implement grouping yet.
- **Automated CI/CD gating** — v1 is manual `go test`. CI integration is future.
- **Production monitoring** — offline eval, not real-time alerting.

## Open Questions

1. **Newsletter grouping** — pipeline doesn't support grouping today. Eval grouping behaviour now, or defer?
2. **Real vs fixture content** — real James emails (anonymised) or synthetic Acme Corp? Real content tests classification rules more honestly.
3. **LLM-as-judge model** — same model as pipeline (gemini-2.5-flash) or independent (Claude)?
4. **Eval frequency** — every deploy? Weekly? On-demand for v1?


---
*Appended by agent-penfold at 2026-03-18 19:28 UTC*


---

## Newsletter Category — Intent Specification

Defined with James on 2026-03-18 by reviewing all 18 newsletter items across 8 distinct senders.

### Handling Modes

| Mode | Behaviour |
|------|-----------|
| **friday_summary** | Group into a single "Weekly Newsletter Summary" delivered Friday morning. Highlight any content matching James's projects/products. |
| **conditional** | Scan for project/product relevance. If relevant content found, include in Friday summary. If nothing relevant, discard entirely. |
| **discard** | Skip processing — no extraction, no summary, no digest contribution. |

### Sender → Handling Map

| Sender | Address | Type | Handling | Notes |
|--------|---------|------|----------|-------|
| Post-Its / CTG | ctgcomms@akamai.com | Division newsletter | friday_summary | Bi-weekly, 4 issues. Key updates, job postings, program wins. |
| Akamai Wave | AkamaiWave@akamai.com | Company newsletter | friday_summary | Monthly, 2 issues. CEO updates, product launches, HR/wellness. |
| EMEA Newsletter | EMEA_newsletter@akamai.com | Regional newsletter | friday_summary | Monthly, 2 issues. Events, spotlights, TA updates. |
| AkamaiSpark Quarterly | AkamaiSpark@akamai.com | Employee engagement | friday_summary | Quarterly. Wellness, volunteering, ERGs, benefits. |
| Talent Zone | TA-EMEA@akamai.com | Recruitment | friday_summary | Open roles, referral bonuses. |
| Spark / Wellness | AkamaiWellness@akamai.com | Internal comms | friday_summary | Edge case — Spark is internal comms, grouped as newsletter. |
| Dynamic Signal | dynamicsignal@akamai.com | Automated digest | conditional | Weekly automated social feed. Usually low-value noise. Only include if project/product relevant. |
| Eng Learning | EngLearn@akamai.com | Training | discard | Stub email with link to Aloha — no extractable content. |

### Extraction Requirements (for friday_summary items)

1. **Key announcements** — what's new or changing
2. **Action items** — deadlines, required actions (e.g. performance cycle, survey completion)
3. **Project/product relevance** — flag any mention of James's projects or products (matched against known entities)
4. **Executive summary** — 2-3 sentence overview of the newsletter

### Friday Weekly Newsletter Summary Output

The summary should:
- Combine all newsletters received that week into one narrative
- Lead with project/product-relevant highlights (if any)
- Group remaining content by theme (people, culture, business updates, action items)
- Be concise — one page max, not a wall of text

### Test Content Inventory (18 items)

| Source ID | Sender | Subject | Handling |
|-----------|--------|---------|----------|
| 3381 | EMEA_newsletter | EMEA Newsletter - December | friday_summary |
| 3382 | EngLearn | Engineering Learning Newsletter Q4 | discard |
| 3394 | ctgcomms | Post-Its Dec 17 | friday_summary |
| 3399 | AkamaiSpark | Quarterly Newsletter Dec | friday_summary |
| 3483 | AkamaiWave | Wave - Nov 2025 | friday_summary |
| 3795 | AkamaiWave | Wave - Year in Review Dec | friday_summary |
| 3796 | EMEA_newsletter | EMEA Newsletter - November | friday_summary |
| 3797 | ctgcomms | Post-Its Dec 3 | friday_summary |
| 3798 | AkamaiWellness | Mental Health / Spark | friday_summary |
| 3799 | ctgcomms | Post-Its Nov 19 | friday_summary |
| 3800 | ctgcomms | Post-Its Nov 5 | friday_summary |
| 3801 | TA-EMEA | Talent Zone Newsletter | friday_summary |
| 3802 | dynamicsignal | Dynamic Signal Digest Dec 15 | conditional |
| 3803 | dynamicsignal | Dynamic Signal Digest Dec 8 | conditional |
| 3804 | dynamicsignal | Dynamic Signal Digest Nov 3 | conditional |
| 3805 | dynamicsignal | Dynamic Signal Digest Nov 17 | conditional |
| 3806 | dynamicsignal | Dynamic Signal Digest Nov 10 | conditional |
| 3807 | dynamicsignal | See what you've been missing | conditional |

---
*Appended by agent-penfold at 2026-03-18 19:55 UTC*


---

## Notification Category — Intent Specification

Defined with James on 2026-03-18 by reviewing 10 notification items across 6 distinct sources.

### Handling Modes

| Mode | Behaviour |
|------|-----------|
| **immediate_escalate** | Surface immediately. Escalate to WhatsApp. For urgent/security content requiring immediate attention. |
| **triage_once** | Surface once when new for "mine or delegated?" decision. Once triaged as delegated → suppress individuals, roll into daily summary. |
| **daily_summary** | Never surface individually. Batch into end-of-day activity summary. |
| **compliance_status** | Group all related items into a single status view by person + deadline. Surface once when new, weekly reminder until completed. |

### Source → Handling Map

| Source | Address | Type | Handling | Notes |
|--------|---------|------|----------|-------|
| Aha! daily to-dos | *-akamai@iad-prod1.mailer.aha.io | Task management | triage_once | New items → surface for triage. Delegated → suppress, daily summary only. Same overdue items repeat daily — suppress after first triage. |
| Aha! hourly digest / updates | support@aha.io | Project activity | daily_summary | "3 updates on MTC outcomes today: Patrick updated X, Patti updated Y". Never individual. |
| Jira (TRACK-JIRA) | gsd-jira@akamai.com | Project activity | daily_summary | Same as Aha updates. Status changes, assignee changes, resolutions. |
| Oracle Learning | oracle-hcm-prod@akamai.com | Compliance training | compliance_status | Group into "Security and Compliance Training" view: who needs to do what by when. James + direct reports. Surface once when new, weekly reminder. |
| Google sign-in alerts | no-reply@accounts.google.com | Security/routine | daily_summary | Group with compliance/routine notifications into end-of-day summary. |
| Corporate security (GlobalSecOps) | globalsecops@akamai.com | Security/urgent | **immediate_escalate** | **Escalate to WhatsApp.** Malicious DNS, security incidents — require immediate attention. |
| Action Required (vendor/internal) | various | Task management | triage_once | Surface once for "mine or delegated?" Likely delegated → daily summary after triage. |

### Extraction Requirements

**For daily_summary items:**
- Who did what on which project/epic
- Status changes, date changes, resolutions
- Aggregate by project: "MTC Outcomes: 3 updates (Patrick, Patti, Ilan)"

**For triage_once items:**
- Task description, assignee, due date
- Is this new or a repeat reminder?
- Project context

**For compliance_status items:**
- Training course name
- Person assigned (James or direct report)
- Due date / overdue status
- Completion status

**For immediate_escalate items:**
- Threat description
- Machine/system affected
- Required response
- Urgency level

### Key Design Insight: Delegated Work Pattern

James is a project lead — many Aha/Jira items are assigned to him but executed by others. The system needs to:
1. Recognise first-time assignment → surface for triage
2. Accept "delegated" triage decision → suppress future individual notifications for that item
3. Continue tracking delegated items in daily summary (activity feed, not action items)

### Test Content Inventory

| Source ID | Source | Subject | Handling |
|-----------|--------|---------|----------|
| 3413 | Aha! to-dos | Daily to-dos reminder | triage_once |
| 3410 | Aha! digest | Recent changes to Compute Feedback Engine | daily_summary |
| 3412 | Aha! digest | Recent changes to Compute Feedback Engine | daily_summary |
| 3808 | Jira | TRACK-JIRA AHASUP-247 updates | daily_summary |
| 3810 | Oracle Learning | Anti-Bribery Training overdue | compliance_status |
| 3409 | Google | Security alert sign-in | daily_summary |
| 3812 | GlobalSecOps | Malicious DNS request | immediate_escalate |
| 3811 | Bitmovin | Action Required: New Player UI | triage_once |
| 3809 | Internal | ACTION REQUESTED: A360 Cleanup | triage_once |

---
*Appended by agent-penfold at 2026-03-23 20:20 UTC*

## Open Questions — Resolved (2026-03-23)

### Q1: Newsletter grouping — DEFER
Newsletter grouping (combining multiple newsletters into one digest) is a digest rollup concern, not an eval framework concern. The eval framework tests individual newsletter extraction quality. Grouping behaviour is tested at the digest level (Phase 4). Create a tracking shard if needed.

### Q2: Real vs fixture content — REAL CONTENT
Use real James emails. We already have 17 newsletter sources in the DB with known-good extraction results. Synthetic content doesn't test classification rules honestly. Anonymisation not needed for internal dev testing.

### Q3: LLM-as-judge model — DEFER
LLM-as-judge is out of scope for Phase 1. Phase 1 uses deterministic matchers (field presence, count bounds, substring matching). LLM-as-judge is Phase 3+ when we need to evaluate extraction quality subjectively (e.g. 'is this summary useful?'). When implemented, use an independent model (Claude) to avoid self-evaluation bias.

### Q4: Eval frequency — ON-DEMAND FOR V1
Run evals on-demand via `go test -tags=quality` for v1. Frequency automation (per-deploy, weekly) is a Phase 4 concern. The eval framework just needs to produce Langfuse scores — scheduling is separate.

## Scope Clarifications

### L3 digest contribution — DEFERRED
L3 (digest contribution validation) is out of scope for Phase 1. Phase 1 covers L1 (routing) and L2 (extraction quality). L3 requires the digest rollup to run, which is a separate pipeline. Track as Phase 4.

### LLM-as-judge — DEFERRED
Out of scope for Phase 1. Deterministic matchers only. Track as Phase 3.

### Phases 2-4 — DEFERRED WITH TRACKING
Phase 2 (notifications), Phase 3 (standard email + LLM-as-judge), Phase 4 (transcript + digest + scheduling) are defined in the design but not decomposed into tasks. They will be decomposed when Phase 1 is proven and the intent specifications for those categories are written.

---
*Appended by agent-mycroft at 2026-03-23 20:57 UTC*

## Decomposition Review — Phase 2 (Notification Category Evals)
*Reviewed by M at $(date -u '+%Y-%m-%d %H:%M UTC')*

### Verdict: APPROVED

5 tasks, clean dependency graph, sensible wave structure. One minor issue and two observations noted below.

---

### Per-Task Assessment

#### pf-20233c — Extend golden YAML types for notification category (Wave 1)
- **Right-sized?** Yes. Single file (types.go), type definitions only. Well-scoped for one session.
- **Acceptance criteria testable?** Yes. Compile check + backward compat + mode coverage are all verifiable.
- **Code locations identified?** Yes. Single file: tests/quality/types.go.
- **No missing decisions?** Minor issue: the spec says to define ActualNotificationExtraction "matching DB schema" but does not specify what the notification pipeline actually stores in content_enrichment. The agent will need to query the DB to discover the schema. This is acknowledged in the approach section ("Check the notification pipeline's actual DB schema") so it is workable, but it means the type definition is partially discovery work. Acceptable.
- **Dependencies correct?** None declared, none needed. Correct.

#### pf-2e00e9 — Notification golden YAML files and .eml fixtures (Wave 2)
- **Right-sized?** Borderline. 9 golden YAML files + 9 .eml fixtures = 18 files. However, these are data files following a template, not complex code. Acceptable for a single session.
- **Acceptance criteria testable?** Yes. File count, handling mode coverage, parsability are all concrete.
- **Code locations identified?** Yes. Two directories specified.
- **No missing decisions?** The golden YAML structure shows `must_complete: [parse, triage, summarize, extract_ner, extract_semantic, embed]` for notifications. But routing_matchers.go shows inferPipeline maps `notification_extract` stage to "notification" pipeline, and a legacy fallback maps `summarize` without `extract_semantic` to "notification". The golden YAML example has both `summarize` AND `extract_semantic` in must_complete, which would make inferPipeline return "standard" not "notification". The agent needs to verify actual pipeline stages from real data, not copy the example verbatim. The approach section says to do this ("Query DB for each source_id to get actual pipeline output"), so this should self-correct.
- **Dependencies correct?** Depends on pf-20233c. Correct — golden files need notification types to parse.

#### pf-b8ed97 — Notification triage calibration and handling mode matchers (Wave 2)
- **Right-sized?** Yes. 2 new files (matchers + tests), 1 small edit (matchOneOf MustNotBe). Parallel-safe with pf-2e00e9.
- **Acceptance criteria testable?** Yes. Compile, unit tests, MatchDetail output are all verifiable.
- **Code locations identified?** Yes. Three files specified.
- **No missing decisions?** The spec says "Each handling mode has at least 3 assertion checks" but the actual checks depend on what the notification pipeline extracts. This couples to pf-20233c's discovery of the DB schema. The agent implementing this task will need the types from pf-20233c to know what fields to assert on. Dependency is correctly declared.
- **Dependencies correct?** Depends on pf-20233c only. Correct. Does NOT depend on pf-2e00e9 (golden files), which is right — matchers don't need golden files to compile and unit test.

#### pf-fd89f5 — Notification eval test runner with Langfuse recording (Wave 3)
- **Right-sized?** Yes. 1 new file, possible small extension to helpers.go. Follows established TestEval_Newsletter pattern closely.
- **Acceptance criteria testable?** Yes. All 7 criteria are concrete and verifiable.
- **Code locations identified?** Yes. 3 files specified.
- **No missing decisions?** No. The pattern is well-established from Phase 1.
- **Dependencies correct?** Depends on pf-20233c, pf-2e00e9, pf-b8ed97. All three are needed. Correct.

#### pf-9c7494 — Integration test: end-to-end (Wave 4)
- **Right-sized?** This is not a separate implementation task — it is a test plan/checklist to run after all other tasks complete. No new code to write. This is appropriate as the final gate.
- **Acceptance criteria testable?** Yes. 7 concrete checks.
- **Code locations identified?** References files from prior tasks. Correct.
- **No missing decisions?** No.
- **Dependencies correct?** Depends on all 4 prior tasks. Correct.

---

### Overall Assessment

**6. Complete coverage?** Yes. The design's Phase 2 scope is: "Add notification golden directory + fixtures, triage calibration assertions, prompt version assertion." All three are covered. Note: prompt version assertion is not explicitly called out in any task, but it is implicitly covered by the golden YAML routing assertions (must_complete stages verify the pipeline ran correctly, and the extract_semantic prompt_version field from the design example can be included in golden YAML).

**7. Old code removal?** N/A. Phase 2 is additive.

**8. Ordering sensible?** Yes. Types first (Wave 1), then data + matchers in parallel (Wave 2), then test runner (Wave 3), then integration validation (Wave 4). This mirrors the Phase 1 pattern and is logically sound.

**9. Integration test covers all design success criteria?** Yes. pf-9c7494 covers type compilation, golden file coverage, matcher validation, full eval run, Langfuse recording, and cross-category compatibility.

**10. No gaps?** One observation: the design mentions "Prompt version assertion (verify v2 applied)" as a Phase 2 deliverable. None of the 5 tasks explicitly implement a prompt version checker. The golden YAML example in the design shows `extract_semantic: prompt_version: 2` but neither the types (pf-20233c) nor the matchers (pf-b8ed97) mention a prompt version field or matcher. This is a minor gap — it could be added to pf-b8ed97's scope or tracked separately. Not blocking since the notification pipeline may not yet record prompt versions in a queryable way.

### Summary

Clean decomposition. Well-structured dependency graph. Tasks follow established Phase 1 patterns, which reduces implementation risk. The prompt_version assertion gap is minor and can be addressed in-flight. Approved for implementation.