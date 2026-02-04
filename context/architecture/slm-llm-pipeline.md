# SLM/LLM Content Processing Pipeline

> Authoritative reference for Penfold's content processing pipeline.
> For full design rationale, see `specs/020-slm-llm-architecture/design.md`.

## Overview

Content flows through six stages, each matched to the right compute for the job:

```
Content arrives (email / transcript / slack)
       |
       v
   Stage 0: Parse (no AI - libraries only)
   Strip HTML, extract headers, detect format
       |
       v
   Stage 1: Triage (SLM - every piece of content)
   Classify, rate importance, detect relevance
       |
       |--- LOW importance, PERSONAL/INTERNAL_COMMS ---> Store and stop
       |
       v
   Stage 2: Extract (SLM - content that passes triage)
   Pull out entities, dates, action items, project names
       |
       v
   Stage 3: Enrich with Context (code logic - database lookups)
   Match extracted entities to known people, glossary, products
       |
       v
   Stage 4: Deep Analysis (Remote LLM - only when warranted)
   Sentiment, strategic insights, risk mapping, synthesis
       |
       v
   Stage 5: Embed and Index (SLM for embeddings, database for storage)
   Generate vector embeddings, update search index
```

## The SLM/LLM Split

> If the answer is already in the text and you just need to pull it out or label it, use the SLM.
> If the answer requires reasoning, connecting dots, or understanding subtext, use the LLM.

| SLM (local 7B, Apple Silicon) | LLM (remote, Gemini Pro/Flash) |
|-------------------------------|-------------------------------|
| Classification into known categories | Connecting information across contexts |
| Extraction of explicitly stated facts | Nuanced business sentiment |
| Short summarisation | Complex multi-step reasoning |
| Structured output from clear instructions | Cross-content synthesis |
| Embedding generation | Risk mapping against known issues |

## Pipeline Stages

### Stage 0: Parse (No AI)

Converts raw content into clean text and structured metadata. Deterministic, fast.

- **Emails**: Strip HTML, extract headers (From/To/CC/Subject/Date/Message-ID/References), detect thread structure, separate quoted replies from new content.
- **Transcripts**: Parse VTT/SRT format, extract speaker labels and timestamps, remove timing artefacts. Stage 0.5 segments by topic using a structural pre-pass (regex for transition phrases, pause detection) validated by the SLM.
- **Slack** (future): Parse JSON export, group by `thread_ts`, treat each thread as a short document.

**Implementation**: `services/worker/activities/parse_*.go`

### Stage 1: Triage (SLM)

Classifies content and decides how much processing it deserves. Uses only the subject line, sender, and first ~500 characters.

**Categories**: PROJECT_UPDATE, CUSTOMER, RISK_ISSUE, ACTION_REQUEST, DECISION, INTERNAL_COMMS, PERSONAL, OTHER

**Importance**: HIGH, MEDIUM, LOW

**Output**: `{"category": "...", "importance": "...", "reason": "one sentence"}`

**Implementation**: `services/worker/activities/triage_activity.go`

### Stage 2: Extract (SLM)

Pulls structured data explicitly stated in the content. Split into two focused passes:

- **Stage 2a (NER)**: People, dates, projects, organisations.
- **Stage 2b (Semantic)**: Action items, decisions, risks.

**Quality gate**: If Stage 1 triaged as RISK_ISSUE but 2b finds zero risks, re-run 2b with a focused risk-only prompt. At most one extra SLM call.

**Long content**: Uses existing `splitIntoChunks()` (1,500 char chunks with 200 char overlap) for content over ~6,000 characters. Results merged by code (deduplication via string matching), not AI.

**Implementation**: `services/worker/activities/extract_*.go`

### Stage 3: Enrich with Context (Code + Database)

Matches raw extracted data against Penfold's knowledge base. No AI required.

- **People resolution**: exact email match -> alias lookup (`person_aliases`) -> name similarity (>0.7 confidence). Flags 0.7-0.9 for review.
- **Glossary expansion**: acronyms and terms resolved via glossary service. "TER" -> "Technical Execution Review".
- **Product matching**: project/product names matched against product hierarchy and aliases.
- **Unknown entity detection**: unresolved terms flagged for the review queue.

**Implementation**: `services/worker/activities/context_builder.go`, `pkg/enrichment/entities/`, `pkg/enrichment/query/`

### Stage 4: Deep Analysis (Remote LLM)

The reasoning stage. Receives clean text, extracted entities, resolved context, and background knowledge from the database. Produces sentiment, topic mapping, risk assessment, implicit action items, and strategic insights.

**Model selection by triage result**:

| Triage Result | Model | Rationale |
|---------------|-------|-----------|
| RISK_ISSUE (any) | Gemini Pro | Risk analysis needs best reasoning |
| CUSTOMER + HIGH | Gemini Pro | Customer content needs accuracy |
| PROJECT_UPDATE + HIGH | Gemini Pro | Major updates need good synthesis |
| PROJECT_UPDATE + MEDIUM | Gemini Flash | Standard updates, Flash sufficient |
| ACTION_REQUEST + MEDIUM | Gemini Flash | Action extraction is simpler |
| Anything + LOW | Gemini Flash | Lower stakes, save cost |

**Implementation**: `services/worker/activities/deep_analysis_activity.go`, `services/ai/router/`

### Stage 4.5: Persist Findings

Validates and stores Stage 4 output back into the knowledge base.

**Validation rules** (applied before any database write):
1. **Mandatory `context_excerpt`** -- every assertion must include a direct quote from the source. No quote = rejected (anti-hallucination defence).
2. **Allowlisted `lifecycle_event` values** -- only: raised, updated, escalated, de_escalated, assigned, decided, deferred, resolved, reopened.
3. **Allowlisted `reference_type` values** -- only: origination, escalation, decision, discussion, resolution, mention.
4. **Entity ID verification** -- referenced `person_id` / `product_id` must exist in the database.

**Outputs**: assertions table (risks, decisions, actions, commitments), product_events, mention_patterns, review queue items for unknown entities.

### Stage 5: Embed and Index (SLM)

Generates vector embeddings for semantic search. Uses local `mxbai-embed-large-v1` (1024 dimensions) on Apple Silicon.

**Multi-level embeddings per content item**:
1. Raw text (general semantic search)
2. Stage 4 summary (high-level search queries)
3. Extracted action items ("what did I need to do?" queries)

All embeddings stored with `embedding_model` and `model_version` for versioning. On model change, batch re-embed all content.

**Implementation**: `services/worker/activities/embed_*.go`

## Triage Gates and Skip Logic

| Category | Importance | Next Stage |
|----------|-----------|------------|
| PERSONAL | any | Store metadata only. Stop. |
| INTERNAL_COMMS | LOW | Store with basic metadata. Embed for search. Stop. |
| INTERNAL_COMMS | MEDIUM/HIGH | Stage 2 (extract action items like "complete training by Friday") |
| PROJECT_UPDATE | any | Stage 2 + Stage 4 |
| CUSTOMER | any | Stage 2 + Stage 4 |
| RISK_ISSUE | any | Stage 2 + Stage 4 (always Gemini Pro) |
| ACTION_REQUEST | any | Stage 2 + Stage 4 |
| DECISION | any | Stage 2 + Stage 4 |

Roughly **50-70% of incoming content never goes past the SLM**. That is the cost and latency saving.

## Context Building for Stage 4

Stage 3 assembles a context package from the knowledge base, scoped to the resolved project. No cross-project bleed.

**Token budgets by content type**:

| Content Type | Context Budget | Content Budget |
|-------------|---------------|---------------|
| Meeting transcript | ~3,000 tokens | ~4,000 tokens (segment summaries) |
| Email | ~2,000 tokens | ~2,000 tokens (full text or thread summaries) |
| Slack thread | ~1,000 tokens | ~1,000 tokens |

**Priority order within each section** (e.g., for assertions): watched items first, then by severity, then by recency. If a section exceeds its budget, tail items are dropped. If aggregate exceeds total, sections truncate in reverse order (glossary first, then timeline, then assertions). Content is never truncated to make room for context.

**Meeting context package** (~3,000 tokens):
1. Meeting series info + previous summary (~200 tokens)
2. Active assertions: watched risks first, then severity (max 10 risks, 10 actions, 5 decisions) (~1,200 tokens)
3. Product timeline events, last 60 days (max 10) (~400 tokens)
4. Participant context: role, seniority, action items (max 15 people) (~600 tokens)
5. Glossary terms relevant to the project (max 20) (~400 tokens)

## Assertion Lifecycle (Golden Thread)

Assertions (risks, decisions, actions, commitments) are tracked in the `assertions` table with two key additions:

- **`assertion_root_id`** -- points to the first assertion in the chain. All versions of the same risk/decision share this ID. Query history with `WHERE assertion_root_id = :root_id ORDER BY created_at`.
- **`lifecycle_event`** -- classifies each version: raised, updated, escalated, de_escalated, assigned, decided, deferred, resolved, reopened.

**Versioning**: `is_current` / `superseded_by` chain. When a risk is updated, old version is marked `is_current=false`, new version takes over.

**Multi-content references**: The `assertion_references` table links assertions to every content item that discussed them, with `reference_type` (origination, escalation, decision, discussion, resolution, mention) and `significance` (primary, supporting, passing).

**Deduplication**: Binary match, bias toward new. Stage 4 receives existing assertions as context and decides: same issue or new? When in doubt, create new. False negatives (duplicates) are cheaper than false positives (merging distinct risks). Daily briefing surfaces potential duplicates naturally.

**Idempotency**: Each assertion gets a deduplication key of `(source_id, assertion_type, extracted_text_hash)` to prevent Temporal retries from creating duplicates.

## Progressive Availability

Content becomes useful before the full pipeline completes:

| Timing | Stage Complete | What Works |
|--------|---------------|------------|
| T+0s | Content ingested | Keyword/full-text search on raw text |
| T+2s | Stage 1 (Triage) | Category filters, faceted search ("show me RISK_ISSUE emails") |
| T+10s | Stage 2 (Extract) | Entity-based search ("emails mentioning Dan Spataro"), basic embedding, semantic search |
| T+15s | Stage 3 (Enrich) | Relationship-based search ("emails about CLIC project") |
| T+30-60s | Stage 4 (Analyze) | Deep analysis, sentiment, insights, summary embeddings |

**Resilience**: Stages 0-3 run locally. If Gemini is down, search, entity extraction, and classification all work. Stage 4 queues via Temporal and processes when the API recovers.

## Trust and Seniority Weighting

Two separate axes, never combined into a single weight:

**Seniority** (organisational hierarchy, `seniority_tier` 1-7 on `people` table):
- Drives escalation detection. A VP joining a previously junior conversation triggers a peripheral alert.
- Affects the seniority profile of participants -- when max seniority increases for a topic, the system alerts.

**Trust** (personal, human-assigned, `trust_level` 0-5 plus optional `trust_domains[]`):
- A staff engineer you trust may carry more weight than an unfamiliar VP.
- Trust can be domain-specific: "I trust Sarah on technical risks but not on timeline estimates."
- Only the human sets trust. The pipeline never modifies it.

**Briefing ordering** (priority tiers, not numeric scores):

| Tier | Criteria |
|------|----------|
| 1 | Watched items (explicit human attention -- always first) |
| 2 | Trusted source assertions (trust_level >= 3) |
| 3 | Senior source assertions (seniority_tier >= 5) |
| 4 | Everything else (by recency) |

Within each tier: sort by severity, then recency.

## Session Bootstrap and Radar Model

**Core philosophy**: AI tracks everything (completeness). Human focuses the spotlight (judgment). Neither is sufficient alone.

**Claude's memory model**:

| Layer | Source | Contents | Changes |
|-------|--------|----------|---------|
| Personality | CLAUDE.md | Behaviour, system design, when to prompt the human | When system design changes |
| Memory | `penf context morning` | Project index, watch lists, last session summary | Every day/session |
| Depth | `penf` queries on demand | Full assertion history, golden thread, complete briefings | Real-time |

**Session flow**: `penf context morning` returns a project index with change counts ("MTC: 2 risk updates, 1 new action. CLIC: quiet."). User picks a project. Claude loads that project's full context. Drill-down via `penf assertion briefing --root-id 101` for any specific risk's golden thread.

**Bidirectional prompting**: Claude prompts the human ("3 new risks -- what are your thoughts?", "VxLAN hasn't been mentioned in 2 weeks. Resolved or forgotten?"). Human adds context the AI can't see (offline conversations, gut feel, trust signals).

**Project-scoped**: Everything is project-first. Morning briefing is a project index. Drill-down is project-scoped. Person-pivot ("what's Dan involved in?") is the exception that cuts across projects.

## Pipeline Introspection

**`penf pipeline describe`** -- show pipeline configuration, current models, prompt versions, and version skew across processed sources.

**Prompt versioning** -- prompt templates stored versioned in `prompt_templates` table. One active version per stage. Changes take effect without restart (short TTL cache). CLI: `penf pipeline prompt show|history|diff|update|rollback|export <stage>`.

**Provenance tracking** -- every stage execution recorded in `pipeline_runs` with `source_id`, `stage`, `model_id`, `prompt_version`, `config_hash`, `status`, and `duration_ms`. Previous runs marked `superseded` on reprocess.

**Targeted reprocessing** -- manual and selective, driven by conversation between user and Claude:
- `penf pipeline reprocess --stage <stage> --dry-run` shows impact analysis (affected sources, downstream cascade, estimated cost)
- Model upgrades apply to new content only by default
- Embedding model changes are the exception: all embeddings must be regenerated
- Prompt fixes are targeted to affected subsets

**Session integration** -- pipeline health is included in `penf context morning` output, so Claude starts every session aware of version skew.

## Key Implementation Files

| Component | Location |
|-----------|----------|
| Parse activities | `services/worker/activities/parse_*.go` |
| Triage activity | `services/worker/activities/triage_activity.go` |
| Extract activities | `services/worker/activities/extract_*.go` |
| Context builder | `services/worker/activities/context_builder.go` |
| Deep analysis | `services/worker/activities/deep_analysis_activity.go` |
| Embed activities | `services/worker/activities/embed_*.go` |
| Entity resolution | `pkg/enrichment/entities/` |
| Context queries | `pkg/enrichment/query/` |
| Workflow definitions | `services/worker/workflows/` |
| AI routing | `services/ai/router/` |
| AI backends (MLX) | `services/ai/backend/mlx.go` |
| Circuit breaker | `services/ai/router/circuit.go` |
| Activity interfaces | `services/worker/activities/interfaces.go` |
| Temporal presets | `pkg/temporal/options.go` |
