# SLM/LLM Architecture Review

**Reviewer:** agent-mycroft (Claude Opus 4.5)
**Date:** 2026-02-04
**Scope:** Full spec folder — 00-overview through design.md and all reference documents

---

## Overall Assessment

This is a thorough, well-grounded design. The writing is unusually clear for a spec of this scope — it anticipates objections, justifies decisions with evidence from real data, and avoids the trap of designing in theory without checking against reality. The test data validation alone puts this ahead of most architectural specs.

That said, there are real concerns worth addressing before implementation.

---

## What's Strong

**The SLM/LLM split is well-reasoned.** The "rule of thumb" — pattern matching to SLM, reasoning to LLM — is the right framing. The specific stage assignments follow logically from it, and the worked examples demonstrate it concretely.

**The feedback loop (Stage 4.5) is the most valuable part of the design.** Without it, Penfold is just a search engine with extra steps. The assertion lifecycle, versioning chain, and context assembly are what turn content processing into institutional memory. The cold start sequence is realistic and acknowledges the chicken-and-egg problem honestly.

**The test data validation grounds the design in reality.** Validating against 267 real emails and 18 transcripts — and discovering that file size is misleading, that 73% of emails are under 5K, that quoted reply stripping matters more than chunking — prevents the design from over-engineering for imaginary problems.

**The deduplication strategy is pragmatic.** "Bias toward new, let the human merge during review" is the right call. Confidence-based fuzzy matching for assertion deduplication would be a reliability nightmare. The idempotency key design is clean.

**Progressive availability is an important architectural property** that many designs miss. Content being keyword-searchable at T+0 and entity-searchable at T+10s, independent of Stage 4 completion, is the right resilience posture.

---

## Concerns and Gaps

### 1. Stage 2b asks too much of the SLM

The design assigns these tasks to the 7B model:
- Triage classification (Stage 1)
- NER extraction (Stage 2a)
- Semantic extraction — action items, decisions, risks (Stage 2b)
- Topic segmentation validation (Stage 0.5)
- Batch Slack triage

Stage 2b is the one to worry about. Extracting action items and risks from business text is not the same as extracting named entities. "Dan will follow up with the customer by Friday" is straightforward. But "We need to keep an eye on the staffing situation" — is that a risk? An action item? A passing observation? This requires judgment, not pattern matching. The design puts it on the SLM side of the line, but it straddles it.

The quality gate (Stage 1 says RISK_ISSUE but Stage 2b finds zero risks → retry) partially addresses this, but it only catches *complete misses*, not *misclassifications* (extracting a risk that isn't one, or describing a risk so vaguely it's useless).

**Recommendation:** Accept that Stage 2b quality will be lower than 2a, and lean on Stage 4 to correct and enrich the Stage 2b output rather than treating it as verified fact. The "Already Extracted (verified)" label in the Stage 4 prompt is misleading — Stage 2b output should be presented as "preliminary extraction, verify and refine" to the LLM.

---

### 2. The context assembly for Stage 4 could blow the context window

The design says Stage 4 receives: clean text + Stage 2 output + Stage 3 resolved context + relevant background (active risks, action items, decisions, product timeline, glossary, participant context). For a meeting transcript about a project with 15 active risks, 20 open actions, 10 recent decisions, and 15 product events — that's a lot of context *before* the actual content.

The design relies on project scoping to keep this bounded, but there's no explicit token budget or truncation strategy for the context package. What happens when a project accumulates 50 active risks? 100 open action items?

**Recommendation:** Add explicit limits to the context assembly queries (some already have `LIMIT 10/15`, but not all). Define a token budget for each context section. Prioritize: watched items first, then by severity/recency. This is a simple code concern but it's the kind of thing that works fine in month 1 and breaks in month 6.

---

### 3. Trust and seniority weighting is underspecified

The concept is well-articulated in `00-overview.md`, and the data model additions are clear (seniority_tier, trust_level, trust_domains on people). But the design doesn't specify *how* these weights actually affect pipeline behavior.

Specifically:
- How does seniority_tier affect assertion visibility in the daily review? Is a risk from a VP (tier 6) automatically shown before a risk from an IC (tier 2)?
- How does trust_level factor into Stage 4 context? Does the LLM receive trust scores? Does it change which assertions are shown?
- How does seniority change detection work algorithmically? "A VP just joined a previously junior discussion" — what query detects this? What's the threshold for alerting?

These are referenced repeatedly as key features but the implementation path is hand-wavy. The assertion lifecycle and golden thread get full SQL examples; trust/seniority get none.

**Recommendation:** Add a concrete example showing how trust and seniority flow through the pipeline and into the briefing. Define the peripheral change detection queries. This doesn't need to be complex — a simple "if max(seniority_tier) for participants in this content > max(seniority_tier) for previous content about this assertion, flag as seniority_escalation" would suffice.

---

### 4. The session bootstrap design has a single point of failure

The entire collaboration model depends on `penf context morning` working correctly. If it returns stale data, incomplete projects, or fails silently, Claude's entire session is compromised. The design treats this as a CLI command, but it's architecturally load-bearing — it's how Claude reconstructs its working memory.

There's no specification for:
- What happens if `penf context morning` fails
- How to validate the bootstrap data is current (staleness detection)
- Versioning of the bootstrap format (when you add new fields, old sessions break)
- Size constraints (how many projects before the bootstrap exceeds Claude's context window)

**Recommendation:** Specify the bootstrap response format, failure modes, and a health check. This command should be as well-specified as the pipeline stages.

---

### 5. The Slack design is premature

The Slack section in design.md is detailed (channel mapping, reaction handling, edit/delete semantics, conversation clustering) but Slack is explicitly a "future content source." This level of design detail for a v2 feature risks over-constraining implementation when you get there. The conversation clustering heuristics (time gaps, participant continuity, SLM verification) are reasonable in theory but may not survive contact with real Slack data.

**Recommendation:** Keep the Slack section as a sketch (Stage 0 grouping, treating threads like short emails) and cut the detailed handling of reactions, edits, deletes, and unthreaded message grouping. Revisit when you have real Slack export data to validate against — the same way the email design was validated against 267 real emails.

---

### 6. The assertion_references table could grow very large

Every passing mention of every risk in every meeting creates a row. With 50 active risks discussed weekly in 5 meetings, that's 250 rows per week of mostly "mention/passing" noise. Over a year, that's 13,000 rows for passing mentions alone. The indexes (`idx_assertion_refs_root`, `idx_assertion_refs_source`, `idx_assertion_refs_significance`) help with queries, but the table's signal-to-noise ratio will degrade.

**Recommendation:** Consider whether passing mentions need to be stored at all. The design says you can filter by significance, which is true — but if 90% of rows are `significance='passing'`, you're storing a lot of data to support a query nobody runs. An alternative: only store `primary` and `supporting` references. Track mention counts as a counter on the assertion itself (`mention_count INT`), incremented when a passing mention is detected. This preserves the signal ("mentioned in 12 meetings") without the storage cost.

---

### 7. The design doesn't address reprocessing

The cold start section mentions "re-run Stage 4 on the important content" with better context. But there's no specification for how reprocessing works generally:
- If you change the triage prompt, do all 500 emails get re-triaged?
- If the SLM model is upgraded (7B → 14B), are previous extractions invalidated?
- If an assertion is manually corrected, does that affect downstream assertions that referenced it?

The design says "store intermediate results" which is correct, but doesn't specify which results are stable across model changes and which aren't.

**Recommendation:** Define which pipeline stages are "stable" (idempotent regardless of model: Stage 0, Stage 3) and which are "model-dependent" (Stages 1, 2, 4). For model-dependent stages, decide whether reprocessing is manual, automatic, or unnecessary. This doesn't need to be built in v1 but should be acknowledged in the design.

---

### 8. Two data model concerns

**`assertion_root_id` self-referencing.** The first assertion in a chain has `assertion_root_id` pointing to itself (`id: 101, assertion_root_id: 101`). This is functional but means you can't distinguish "this is the root" from "root_id wasn't set" using NULL. Consider whether root assertions should have `assertion_root_id = NULL` to make the root explicit, or add a boolean `is_root`.

**The timeline query in design.md has a bug.** The COALESCE-based product timeline query (around line 1270) joins `product_events` with `assertions` and `people` in a way that will produce incorrect results — the LEFT JOIN on assertions uses `a.is_current = false` which only captures historical versions, missing the current one. And the final JOIN on people will drop rows where no person is associated. This is a design doc, not production code, but if someone implements from it they'll get wrong results.

---

## README Feedback

The README (`readme.md`) is effective as an onboarding document for external reviewers but has a structural issue:

**The reading order is wrong.** The README says "read them in order" but places all six reference docs (00-06) before `design.md`. A reviewer following this literally reads the data model, entity resolution pipeline, and hardware constraints before understanding what the feature actually is. `design.md` is the heart of the spec and should come second, immediately after `00-overview.md`. The numbered reference docs are supporting material to consult as needed.

**design.md at ~1,900 lines needs navigation guidance.** The README doesn't tell reviewers which sections of design.md are most relevant to each review question. A sentence like "Sections on the pipeline stages (Stages 0-5) and the feedback loop are most relevant to the SLM/LLM split question; the golden thread section covers the collaboration model" would help focus review effort.

**No mention of the feedback directory.** Reviewers aren't told where to put their feedback or what format to use.

---

## Summary

The design is sound. The SLM/LLM split is well-justified, the pipeline architecture is pragmatic, and the feedback loop is the right core insight. The main gaps are:

1. **Stage 2b quality** — present SLM extraction as preliminary, not verified
2. **Context window management** — add explicit token budgets for Stage 4 context
3. **Trust/seniority** — needs concrete implementation detail, not just philosophy
4. **Session bootstrap** — needs specification as a first-class component
5. **Slack design** — defer to when real data is available
6. **assertion_references growth** — consider dropping passing mentions
7. **Reprocessing strategy** — acknowledge model-dependent vs stable stages
8. **Data model bugs** — self-referencing root_id semantics, timeline query

The strongest parts — the golden thread model, the test data validation, the progressive availability, and the "bias toward new" deduplication strategy — reflect careful thinking about what goes wrong in practice, not just what works in theory.
