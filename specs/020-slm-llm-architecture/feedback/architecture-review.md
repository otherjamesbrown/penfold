# Architecture Review: SLM/LLM Architecture Design

**Reviewer:** Claude Opus 4.5 (agent-mycroft)
**Date:** 2026-02-04
**Scope:** Full document set (00-overview through implementation.md, plus design.md and all reference docs)

---

## Executive Summary

This is an unusually well-thought-out design. The documentation quality is high, the problem statement is clear, the constraints are honest, and the worked examples are grounded in real data. The human-AI collaboration model (the "radar model") is the strongest element and is genuinely differentiated from typical AI pipeline designs. The 6-stage pipeline is architecturally sound, and the decision to validate against 267 real emails and 18 transcripts before finalising the design is exactly the right instinct.

That said, there are real gaps. Some are design omissions that need addressing before implementation. Others are areas where the design is optimistic about what a 7B model can do, or where the operational complexity is higher than acknowledged.

This review is organised into three sections:
1. **What works well** (don't change these things)
2. **Significant concerns** (address before implementing)
3. **Gaps and missing pieces** (things the design doesn't cover)

---

## 1. What Works Well

### The Radar Model Is the Right Abstraction

The human-AI collaboration model is the strongest part of this design and the hardest to get right. Most AI systems either over-automate (the AI decides what matters) or under-automate (the AI waits to be asked). The radar model threads the needle: the AI tracks everything, the human focuses the spotlight, and the system bridges the gap with bidirectional prompting.

The key insight — that human signals (trust, gut feel, offline context) are *inputs*, not *overrides* — is correct and important. Encoding this into the data model (trust_level, seniority_tier, watch list annotations) rather than just describing it as a philosophy makes it concrete and implementable.

### The Pipeline Decomposition Is Sound

Splitting the single monolithic LLM call into 6 stages with appropriate model assignment per stage is the right architectural move. The principle "if the answer is already in the text, use the SLM; if it requires reasoning, use the LLM" is a good heuristic. The progression from cheap/local (Stages 0-3) to expensive/remote (Stage 4) is well-reasoned.

### Progressive Availability Is a Force Multiplier

Content becoming searchable after Stage 2 (embeddings) rather than waiting for Stage 4 (remote LLM) is an excellent design decision. It means the system degrades gracefully when Gemini is down, and it means batch processing doesn't block interactive use. This should be a hard architectural constraint, not just a nice-to-have.

### The Assertion Lifecycle and Golden Thread

The VxLAN worked example is convincing. The addition of `assertion_root_id` and `lifecycle_event` to the assertions table, plus the `assertion_references` table, is the right approach. The distinction between "primary" and "passing" significance solves the real problem of finding the 4 meetings that matter out of 17 that mentioned a risk.

### Validation Against Real Data

Analysing 267 emails and 18 transcripts before finalising the design is best practice. The finding that file size is misleading (5.7MB email = 18K chars of text) is exactly the kind of insight you only get from real data. The conclusion that 73% of emails fit in a single SLM call is well-grounded.

### The Cost Model Is Honest

The cost analysis is realistic. $0.35-1.60 per 100 emails with the hybrid pipeline vs $3-10 for everything-to-Gemini-Pro is a compelling argument. The acknowledgment that local SLM is "free but not free" (it consumes hardware resources) is a good nuance.

### The Session Bootstrap Design Is Practical

`penf context morning` returning a structured JSON briefing that Claude reads at session start is a clean solution to the amnesia problem. The separation of personality (CLAUDE.md) from memory (database via penf) is the right abstraction. The on-demand depth queries (`penf assertion briefing --root-id 101`) avoid overloading the context window at startup.

### No Graph Database

The argument against a graph database is correct. The relationships in Penfold are bounded-depth foreign key joins, not unbounded graph traversals. PostgreSQL with proper indexing handles this well. The materialised view / affinity score approach for "who works with whom" is the right trade-off.

---

## 2. Significant Concerns

### 2.1 The Stage 2 Extraction Prompt Contradicts the Prompt Engineering Rules

`prompt-engineering.md` states: **"One task per prompt. Never ask a 7B model to do two things at once."**

The Stage 2 extraction prompt in `design.md` asks the 7B to extract *seven* things simultaneously: people, dates, projects, organisations, action items, decisions, and risks. This is the exact anti-pattern the prompt engineering doc warns against.

**The risk:** The 7B will do some of these well (people, dates) and some poorly (action items with ambiguous assignees, risks stated indirectly). You won't know which outputs to trust because they're all in one response.

**Recommendation:** Either:
- Split Stage 2 into sub-stages (2a: people + dates + projects, 2b: action items, 2c: risks/decisions) — three focused SLM calls instead of one sprawling one. This triples the SLM call count but each call is more reliable.
- Or accept the quality trade-off and add explicit validation. If the extraction returns zero risks from content triaged as RISK_ISSUE, that's a quality signal — re-extract with a focused risk-only prompt.

The design already has the Langfuse infrastructure to track this. Instrument it from day one.

### 2.2 Context Package Size Is Unbounded

Stage 4 receives: clean text + extracted entities + resolved context + background knowledge (active risks, open actions, recent decisions, product events, glossary terms). For a meeting about a mature project with 20 active risks, 15 open actions, 10 recent decisions, and 6 months of product events, this context package could easily exceed 15-20K tokens *before* the actual content.

With Gemini 2.0 Flash at ~1M tokens context, this isn't a hard limit. But with Gemini Pro or smaller models, it could be. More importantly, attention quality degrades in long contexts even when the window technically fits.

**What's missing:** A context budget. The design should specify:
- Maximum context package size per content type (e.g., 8K tokens for emails, 15K for transcripts)
- Priority ordering for context items (current risks > recent decisions > historical events)
- Truncation strategy (most recent first? Highest severity first?)
- A mechanism to summarise context when it exceeds the budget (e.g., "15 open actions" rather than listing all 15 in full)

### 2.3 Assertion Deduplication Is the Hardest Unsolved Problem

The design correctly identifies this as "the hardest part of Stage 4.5" and then proposes: embedding similarity to find candidates, pass to LLM for confirmation. But the precision/recall trade-off here is critical and under-explored:

- **False positive match** (merging different risks): You lose information. Two distinct risks become one, and the history of the second is absorbed into the first. This is data corruption.
- **False negative match** (duplicating a risk): You get noise. Two versions of the same risk exist independently. This is annoying but recoverable.

The asymmetry means you should bias heavily toward false negatives (creating duplicates) and let the human merge them during review. The design doesn't state this explicitly.

**Additional concerns:**
- Embedding similarity between "VxLAN injection vulnerability" and "VxLAN configuration issue" might be high, but these could be different risks. The LLM confirmation step helps, but the LLM's context is limited to what Stage 3 assembled.
- The design proposes matching by project + keyword overlap + embedding distance. What about assertions that span projects? A risk raised in the CLIC context that affects OSL would need cross-project matching.
- There's no discussion of the human review path for assertion matches. When the system creates a "possible match" with 0.7 confidence, who reviews it? How?

**Recommendation:** Add a `match_confidence` field to assertion references. For matches above 0.9, auto-link. For 0.6-0.9, create a review queue item. Below 0.6, create a new assertion. Make the human the final arbiter for ambiguous cases — this is exactly the kind of judgment call the radar model is designed for.

### 2.4 Trust and Seniority Live on the Wrong Table

The data model adds `trust_level`, `trust_note`, and `trust_domains` directly to the `people` table. But the README states: "Trust is personal and subjective" and "Trust is the human's private signal."

In a multi-tenant system, the `people` table is shared. If User A trusts Sarah at level 5 and User B trusts Sarah at level 2, where does that go? The current schema doesn't support per-user trust.

**Recommendation:** Move trust fields to a separate `user_trust_signals` table:

```sql
CREATE TABLE user_trust_signals (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    user_id BIGINT NOT NULL,  -- or session/persona identifier
    person_id BIGINT NOT NULL REFERENCES people(id),
    trust_level SMALLINT CHECK (trust_level BETWEEN 0 AND 5),
    trust_note TEXT,
    trust_domains TEXT[],
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, user_id, person_id)
);
```

Seniority is different — it's organisational fact, not personal opinion. Keeping `seniority_tier` on the `people` table is correct. But it should be tenant-scoped (a person might be a VP at one company and a contractor at another).

### 2.5 The "Kill Path" for Low-Value Content May Be Too Aggressive

The triage stage routes PERSONAL/LOW and INTERNAL_COMMS/LOW to "store metadata only" or "store with basic metadata." These items get embeddings but skip extraction entirely.

This conflicts with the "track everything" principle. Consider: a casual email between colleagues that says "btw I heard the VxLAN fix might slip to February" is exactly the kind of signal the radar model should catch. But if the triage SLM reads the first 500 chars (which might be "Hey, how was your weekend?"), it classifies as PERSONAL/LOW and the risk signal in the body is lost.

**Recommendation:** Don't skip extraction entirely for any content. Instead, implement a "light extraction" path:
- Full extraction for HIGH/MEDIUM content (current design)
- Keyword-trigger extraction for LOW content: if the body contains any glossary term or known project name, run extraction despite the LOW triage. This is a fast regex/string check, not an SLM call.
- Metadata-only for content that fails both triage and keyword checks

This catches the "casual mention of something important" case without processing every lunch invitation through the full pipeline.

### 2.6 Error Recovery Between Stages Is Unspecified

The design principles say stages should be idempotent and intermediate results should be stored. But what happens when:

- Stage 3 partially fails (3 of 5 entities resolved, 2 failed due to DB timeout). Does Stage 4 proceed with partial context? Wait for retry?
- Stage 4 returns a malformed response. Retry with the same context? Different model? Fall back to Flash from Pro?
- The MLX server crashes mid-batch. 50 of 100 emails have completed Stage 1-2. When it restarts, does it pick up from item 51, or re-process everything?

The Temporal workflow engine handles some of this (saga pattern, compensation stack), but the design doesn't specify the retry/recovery policy per stage. Each stage has different failure characteristics:
- Stage 0 (parsing): deterministic, always succeeds or always fails for the same input
- Stage 1-2 (SLM): may timeout, may produce invalid JSON, may produce low-quality output
- Stage 3 (DB lookups): may partially succeed
- Stage 4 (remote LLM): may timeout, may be rate-limited, may be down entirely
- Stage 5 (embeddings): may timeout on the MLX server

**Recommendation:** Add a retry/recovery policy table to the design. For each stage: max retries, backoff strategy, fallback model (if applicable), what constitutes "partial success" and how to handle it, and when to escalate to the dead letter queue.

---

## 3. Gaps and Missing Pieces

### 3.1 No Quality Measurement Framework

`prompt-engineering.md` mentions tracking quality via Langfuse and "periodically review a sample." For a production system, this is insufficient.

**What's needed:**
- **Triage accuracy:** When Stage 4 analysis contradicts Stage 1 triage (e.g., LLM identifies a risk in content triaged as LOW), log this as a triage miss. Track the miss rate over time. If it exceeds a threshold, the triage prompts need adjustment.
- **Extraction recall:** When the LLM (Stage 4) identifies entities or action items that the SLM (Stage 2) missed, that's an extraction gap. Track per-field recall (people recall, action item recall, risk recall).
- **Resolution accuracy:** When a human corrects an auto-resolution in the review queue, that's a Stage 3 error. Track false positive and false negative resolution rates.
- **Assertion match accuracy:** When a human merges or splits assertions that Stage 4.5 linked or separated, that's a deduplication error. Track these.

Build a `quality_metrics` view that aggregates these signals. Include it in the morning briefing for the operator (not the end user, the person tuning the system).

### 3.2 Incremental Processing for Updated Content

What happens when:
- A new reply arrives in an already-processed email thread?
- A meeting transcript is re-uploaded with corrections?
- A Slack thread that was triaged as LOW gets a new message that makes it HIGH?

The design describes processing content as if each item is new. But in practice, most content arrives incrementally. The pipeline needs an incremental processing model:

- **For new thread replies:** Process only the new message through Stages 1-3. Re-run Stage 4 on the thread with updated context (previous analysis + new message extraction). Don't re-process old messages.
- **For corrected transcripts:** Detect what changed (diff), re-process affected segments only.
- **For Slack:** Re-triage the thread when new messages arrive. If the triage changes (LOW -> HIGH), backfill Stages 2-4 on the entire thread.

### 3.3 Concurrency Model and Throughput

The design says "LLM inference is sequential" on the MLX server. The cost model says "200 SLM calls for 100 emails takes 5-15 minutes." But this assumes sequential processing.

**Questions not addressed:**
- Can the MLX server handle concurrent requests? The MLX-LM server does support concurrent inference (with request queuing), but throughput depends on batch size and model memory usage.
- Should Stage 1 (triage) and Stage 5 (embeddings) use different MLX instances? The embedding model and the LLM are separate models. Can they serve concurrently?
- What's the throughput for mixed workloads? If a batch of 100 emails is being triaged while a user asks an interactive question, what's the latency impact?

**Recommendation:** Add a concurrency section. Define the expected throughput for each stage, the queuing strategy (priority queue with interactive requests jumping the batch queue?), and the impact of batch processing on interactive latency. The embedding model and LLM model can run concurrently on the MLX server — this is worth confirming and documenting.

### 3.4 Monitoring and Operational Visibility

For a system that processes emails and meetings, the operational concerns are significant:

- **Pipeline health:** How do you know if the pipeline is processing content? If 50 emails arrived overnight and only 30 were processed, where did the other 20 get stuck?
- **SLM health:** If the Qwen 2.5 7B starts producing lower-quality output (model degradation, server issues), how do you detect it?
- **Cost tracking:** The design mentions Langfuse for LLM tracing. But is there a cost dashboard? "We spent $X on Gemini this month" with breakdown by content type and stage?
- **Latency monitoring:** Average and p95 latency per stage, per content type. If triage starts taking 10 seconds instead of 2, that's an early warning.

The Langfuse integration provides the raw data. What's missing is the aggregation layer and alerting strategy.

### 3.5 The Session Bootstrap Doesn't Scale

`penf context morning` returns watch list, recent changes, active projects, and trusted people. With 3 watched items and 2 projects, this is a compact JSON payload that fits comfortably in Claude's context.

What about a power user tracking 30 risks across 8 projects with 15 trusted people? The morning briefing could be 10K+ tokens, consuming a significant chunk of Claude's working memory before the conversation even starts.

**Recommendation:** Add a pagination/summarisation layer:
- The morning briefing returns a summary view (counts, highlights, top 5 changes)
- Claude can drill into any section on demand (`penf context risks --project CLIC`)
- The briefing payload has a configurable max size, with the most important items included first and a "N more items available" indicator

### 3.6 Seniority Bootstrapping

The design adds `seniority_tier` (1-7) to the people table but doesn't discuss how to populate it. The `people` table already has `title` and `department` fields. For existing records:

- A person with title "VP Engineering" should auto-populate seniority_tier = 6
- A person with title "Senior Staff Engineer" should be ~4
- The mapping is fuzzy (titles vary across companies) but a rule-based first pass from existing title data gets you 80% there

**Recommendation:** Add a title-to-seniority mapping function as part of the seniority migration. Don't require the human to manually set seniority for 200+ people records. Auto-populate from titles, flag low-confidence mappings for review.

### 3.7 What Happens When the Remote LLM Disagrees With the SLM?

Stage 1 (SLM) classifies an email as PROJECT_UPDATE/MEDIUM. Stage 4 (LLM) analyses it and discovers a critical undisclosed risk that makes it RISK_ISSUE/CRITICAL.

Does Stage 4 retroactively update the triage? Does the content get re-tagged? If the daily review sorts by triage category, will this email appear under PROJECT_UPDATE (wrong) or RISK_ISSUE (correct)?

**Recommendation:** Stage 4 should be able to override triage results. Store the original triage and the LLM's revised classification separately (`triage_category` vs `analysis_category`). Use the LLM's classification for all downstream operations (daily review, alerting, watch list matching). Track override frequency as a quality metric (see 3.1).

### 3.8 No Discussion of Data Retention or Archival

The assertions table grows monotonically (old versions are superseded but never deleted). The assertion_references table grows with every piece of content that mentions a tracked risk. Over 2-3 years with thousands of emails and hundreds of meetings:

- How large does the assertions table get?
- Do superseded assertion versions need to stay in the hot table, or can they move to an archive?
- Do "passing" references in assertion_references need to persist forever?
- What's the storage cost for multi-level embeddings across thousands of content items?

This doesn't need a detailed answer now, but the design should acknowledge it as a future concern and ensure the schema supports eventual archival.

### 3.9 Privacy Model for Remote LLM Calls

The design says only Stage 4 content goes to remote APIs. But the Stage 4 context package includes:
- Extracted person names and resolved IDs
- Project names and product names
- Active risk descriptions
- Action items with assignees
- Organisational structure signals (seniority, team membership)

This is sensitive organisational data being sent to Google's Gemini API. The privacy section of the design is too brief on this. What specifically gets sent? Can the context package be scrubbed of identifying information before sending? Is there a data processing agreement with Google that covers this use case?

**Recommendation:** Add a "data sent to remote APIs" audit table that logs exactly what was included in each Stage 4 request. This serves both privacy compliance and debugging. Also specify what *never* gets sent (trust notes, human annotations, watch list contents — these are private signals that should stay local).

### 3.10 Slack Processing Is Underspecified

The email and transcript pipelines have detailed worked examples. Slack gets a high-level pipeline description and a few paragraphs about thread grouping. Given that Slack messages are listed as a planned content type and that they present unique challenges (high volume, threading, context dependence), this section needs more depth.

Specific gaps:
- How is a Slack workspace export ingested? Full export? Incremental API sync?
- Channel-to-project mapping: who maintains it? Auto-detected or manual?
- Reaction/emoji handling: "+1" and emoji reactions carry signal (agreement, urgency). Are they processed?
- Cross-channel references: a message in #general says "see the thread in #mtc-risks." Is this followed?
- Volume management: a channel with 500 messages/day needs aggressive filtering. What's the strategy beyond "triage per thread"?

---

## Summary of Recommendations

| # | Priority | Recommendation |
|---|----------|---------------|
| 2.1 | High | Split Stage 2 extraction into focused sub-prompts or add validation gates |
| 2.2 | High | Define context budget limits and priority ordering for Stage 4 |
| 2.3 | High | Bias deduplication toward false negatives; add confidence-based review queue |
| 2.4 | High | Move trust fields to a per-user table for multi-tenant support |
| 2.5 | Medium | Add keyword-trigger extraction for LOW-triaged content |
| 2.6 | Medium | Define per-stage retry/recovery policies |
| 3.1 | High | Build quality measurement framework with automated triage/extraction metrics |
| 3.2 | Medium | Design incremental processing for thread updates and corrections |
| 3.3 | Medium | Document concurrency model and interactive vs batch priority |
| 3.4 | Medium | Add monitoring dashboards and alerting strategy |
| 3.5 | Low | Add pagination/summarisation to session bootstrap |
| 3.6 | Low | Auto-populate seniority from existing title data |
| 3.7 | Medium | Allow Stage 4 to override Stage 1 triage with tracking |
| 3.8 | Low | Acknowledge data retention as a future concern |
| 3.9 | Medium | Audit what data is sent to remote APIs; define privacy boundaries |
| 3.10 | Low | Flesh out Slack processing with worked examples |

---

## Overall Assessment

This design is ready for implementation with targeted revisions. The core architecture (6-stage pipeline, human-AI collaboration model, assertion lifecycle, session bootstrap) is sound and well-motivated. The main risks are operational: SLM quality management, context budget control, assertion deduplication accuracy, and the gap between the "track everything" principle and the triage kill path.

The documentation quality is a significant asset. The fact that this spec folder is self-contained and readable by an external advisor without codebase access is unusual and valuable. Maintain this standard as the design evolves.

**Strongest elements:** Radar model, progressive availability, golden thread assertion tracking, real-data validation.

**Highest-risk elements:** Stage 2 extraction quality from a 7B model, assertion deduplication precision, context package size management.
