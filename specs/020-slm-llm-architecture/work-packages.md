# SLM/LLM Pipeline — Work Packages

Parent shard: `pf-7f69f2`

## Execution Phases

```
Phase 1: WP1 (Schema) + WP2 (Parse)              — parallel, no overlap
Phase 2: WP3 (Triage)                             — after WP1
Phase 3: WP4 (Extract)                            — after WP3
Phase 4: WP5 (Context Builder)                    — after WP4
Phase 5: WP6 (Analysis) + WP10 (Bootstrap)        — parallel
Phase 6: WP7 (Persist) + WP11 (Watch/Trust)       — parallel
Phase 7: WP9 (Embeddings)                         — after WP7
Phase 8: WP8 (Orchestrator) + WP12 (Introspection) — parallel
Phase 9: WP13 (Documentation)                       — after WP8, WP10
```

## Dependency Graph

```
WP1 (Schema) ──────────┬──────────────────────────┐
      │                 │                           │
WP2 (Parse)             │                           │
      │                 │                           │
WP3 (Triage) ←─── WP1  │                           │
      │                 │                           │
WP4 (Extract) ←── WP3  │                           │
      │                 │                           │
WP5 (Context) ←── WP4 ─┘                           │
      │                                             │
WP6 (Analysis) ←─ WP5                              │
      │                                             │
WP7 (Persist) ←── WP6                              │
      │                                             │
WP9 (Embed) ←──── WP7                              │
      │                                             │
WP8 (Orchestrator) ← WP2,3,4,5,6,7,9              │
                                                    │
WP10 (Bootstrap) ←─── WP1,WP7 ────────────────────┤
WP11 (Watch/Trust) ←─ WP10 ───────────────────────┤
WP12 (Introspection) ← WP1,WP8 ──────────────────┘

WP13 (Documentation) ← WP8,WP10
```

## Shard Reference

| Shard | Work Package | Agent Type | Blocked By |
|-------|-------------|------------|------------|
| `pf-341e24` | WP1: Schema Foundations | data-dev | — |
| `pf-ac4ca3` | WP2: Stage 0 — Parse | worker-dev | — |
| `pf-39a879` | WP3: Stage 1 — Triage RPC | service-dev | WP1 |
| `pf-1eca7f` | WP4: Stage 2 — Extract RPC | ai-dev | WP1, WP2 |
| `pf-9816a7` | WP5: Stage 3 — Context Builder | service-dev | WP4, WP1 |
| `pf-964bd7` | WP6: Stage 4 — Deep Analysis | ai-dev | WP4, WP1 |
| `pf-67f20a` | WP7: Stage 4.5 — Persist Findings | worker-dev | WP6, WP1 |
| `pf-7c09c8` | WP8: Pipeline Orchestrator | worker-dev | WP2, WP3, WP4, WP5, WP6, WP1 |
| `pf-e0dbd7` | WP9: Stage 5 — Multi-Level Embeddings | ai-dev | WP7 |
| `pf-13d9dd` | WP10: Session Bootstrap | cli-dev | WP1, WP7 |
| `pf-d5b341` | WP11: Watch List + Trust/Seniority | cli-dev | WP10, WP1 |
| `pf-8313b5` | WP12: Pipeline Introspection | cli-dev | WP1, WP8 |
| `pf-2f4d11` | WP13: Documentation Update | general-purpose | WP8, WP10 |

## Completion Log

Record each work package completion here. Sessions should update this after committing.

| WP | Status | Commit | Date | Changes Summary |
|----|--------|--------|------|-----------------|
| WP1 | **done** | `0cdc532` | 2026-02-04 | 4 migrations (028–031): assertion lifecycle, pipeline registry, trust/seniority, watch list |
| WP2 | **done** | `52d1c8e` | 2026-02-04 | Email parse (HTML-to-text, quoted reply detection), transcript parse (SRT/VTT/TXT, speaker normalization), Temporal activities on Main/Email queues. 37 tests. |
| WP3 | **done** | `45d46b8` | 2026-02-04 | TriageContent gRPC RPC: proto definition, gateway proxy, AI server handler with triage prompt (8 categories, 3 importance levels), JSON validation, retry (max 2), Langfuse tracing, prompt template migration (032). 20+ tests. |
| WP4 | pending | — | — | — |
| WP5 | pending | — | — | — |
| WP6 | pending | — | — | — |
| WP7 | pending | — | — | — |
| WP8 | pending | — | — | — |
| WP9 | pending | — | — | — |
| WP10 | pending | — | — | — |
| WP11 | pending | — | — | — |
| WP12 | pending | — | — | — |
| WP13 | pending | — | — | — |

---

## WP1: Schema Foundations

**Shard:** `pf-341e24` · **Agent:** data-dev · **Depends on:** nothing

**Files:**
- `migrations/028_assertion_lifecycle.sql` — new
- `migrations/029_pipeline_registry.sql` — new
- `migrations/030_trust_seniority.sql` — new
- `migrations/031_watch_list.sql` — new

**Scope:**
- Assertion extensions: `assertion_root_id`, `lifecycle_event` columns on assertions table
- `assertion_references` table (`reference_type`, `significance`, `context_excerpt`)
- `pipeline_stages` table (stage registry with metadata)
- `pipeline_runs` table (provenance tracking)
- `prompt_templates` table (versioned prompts)
- `seniority_tier`, `trust_level`, `trust_domains` on people table
- `project_status` enum + status column on projects table
- `watch_list` table (`user_id`, `assertion_root_id`, `project_id`, `notes`)
- Indexes on all new columns/tables per design.md

**Acceptance criteria:**
- All migrations run cleanly on dev02
- Existing data preserved (ALTER, not recreate)
- Indexes exist for all query patterns in design.md
- `assertion_root_id` backfill: set to `id` for all existing assertions (they're all roots)

---

## WP2: Stage 0 — Parse ✓

**Shard:** `pf-ac4ca3` · **Agent:** worker-dev · **Depends on:** nothing · **Status:** done

**Files delivered:**
- `pkg/parse/email.go` — HTML-to-text (x/net/html tokenizer), quoted reply detection (Gmail/Outlook/standard), whitespace normalization
- `pkg/parse/email_test.go` — 18 tests
- `pkg/parse/transcript.go` — SRT parser, VTT/TXT delegation via pkg/ingest/meeting, speaker name normalization, format auto-detection, same-speaker merging
- `pkg/parse/transcript_test.go` — 10 test functions (19 sub-tests)
- `services/worker/activities/parse_activities.go` — ParseEmail + ParseTranscript Temporal activities
- `services/worker/activities/parse_activities_test.go` — 9 tests
- `services/worker/activities/register.go` — WithParseActivities builder, registered on Main + Email queues

**Implementation shards:** `pf-c439f2` (email), `pf-d8e2c6` (transcript), `pf-eb042d` (activities)

**Acceptance criteria (met):**
- Processes test emails: MIME to clean text extraction
- Quoted reply stripping reduces long threads to new content only (Gmail, Outlook, standard `>` quoting)
- VTT/SRT format overhead stripped (timestamps, entry numbers)
- Speaker names normalized ("Sara Weisman (she/her)" → "Sara Weisman")
- Tests cover: HTML stripping, quoted reply detection, VTT/SRT/TXT parsing, speaker normalization, format detection, edge cases

---

## WP3: Stage 1 — Triage RPC ✓

**Shard:** `pf-39a879` · **Agent:** service-dev, ai-dev · **Depends on:** WP1 · **Status:** done

**Files delivered:**
- `api/proto/ai/v1/ai.proto` — TriageContent RPC, TriageContentRequest/Response messages
- `api/proto/ai/v1/ai.pb.go`, `ai_grpc.pb.go` — generated proto code
- `pkg/ai/client.go` — TriageContent client method
- `pkg/ai/client_test.go` — TestClient_TriageContent (3 subtests)
- `services/gateway/modelservice/service.go` — TriageContent gateway proxy
- `services/ai/server/server.go` — TriageContent handler entry point with retry loop
- `services/ai/server/triage.go` — prompt builder, JSON parser, category/importance validation
- `services/ai/server/triage_test.go` — 20+ tests (valid, malformed JSON, unknown category, invalid importance, truncation)
- `migrations/032_seed_triage_prompt.sql` — seeds triage prompt template v1

**Implementation shards:** `pf-ccdeff` (proto + gateway + client), `pf-0360e1` (AI server + tests)

**Acceptance criteria (met):**
- TriageContent RPC callable via gateway
- Returns valid category + importance + reason
- Rejects malformed SLM output and retries (up to 2)
- Langfuse tracing records the call (StartLLMCall + SetLLMResult)
- Pipeline_runs provenance logging (DB insert deferred to workflow layer)
- Tests cover: valid classification, malformed JSON retry, unknown category rejection, invalid importance, content truncation to 500 chars

---

## WP4: Stage 2 — Extract RPC

**Shard:** `pf-1eca7f` · **Agent:** ai-dev · **Depends on:** WP3

**Files:**
- `api/proto/ai/v1/ai.proto` — add ExtractEntities RPC (2a NER + 2b semantic)
- `services/ai/server/server.go` — add handler
- `services/worker/activities/content_activities.go` — extend chunking for extraction
- Tests for all new code

**Scope:**
- Two-pass extraction: Stage 2a NER (people, dates, projects, orgs) + Stage 2b semantic (action items, decisions, risks)
- Chunking for content >3K chars using existing `splitIntoChunks()` with merge logic
- Quality gate: if triage=RISK_ISSUE but extraction returns 0 risks, re-run with focused prompt
- Output: structured JSON matching design.md schema
- Seed `pipeline_stages` rows for 'extract_ner' and 'extract_semantic'

**Acceptance criteria:**
- ExtractEntities RPC callable, returns merged NER + semantic results
- Chunking works for content up to 70K chars (CTG report case)
- Quality gate triggers and re-runs when triage/extraction disagree
- Merge logic deduplicates entities across chunks (code, not AI)
- Tests cover: short email extraction, chunked extraction, merge dedup, quality gate

---

## WP5: Stage 3 — Context Builder

**Shard:** `pf-9816a7` · **Agent:** service-dev · **Depends on:** WP4, WP1

**Files:**
- `pkg/enrichment/extraction/context.go` — extend existing context system
- `services/gateway/` — new context builder service or extend existing
- `pkg/enrichment/query/` — extend existing query builders

**Scope:**
- Takes Stage 2 output, resolves entities against people/glossary/products/teams tables
- Uses existing mention resolution pipeline (`services/gateway/mentionsservice/`)
- Builds context package for Stage 4 with token budgets per design.md:
  - Meeting: ~3,000 tokens (assertions, timeline, participants, glossary)
  - Email: ~2,000 tokens (thread history, assertions, sender, glossary)
  - Slack: ~1,000 tokens (channel context, assertions, participants)
- Queries: active risks, open actions, recent decisions, product timeline, glossary terms — all per design.md SQL
- Output: enriched entities with person_ids + context package JSON

**Acceptance criteria:**
- Resolves known people by exact email, alias, fuzzy name
- Glossary expansion works (TER -> Technical Execution Review)
- Context package respects token budgets, truncates from tail
- Unknown entities flagged for review queue
- Tests cover: person resolution, glossary expansion, token budget enforcement, unknown entity detection

---

## WP6: Stage 4 — Deep Analysis

**Shard:** `pf-964bd7` · **Agent:** ai-dev · **Depends on:** WP4, WP1

**Files:**
- `services/ai/server/server.go` — restructure analysis handler
- `services/ai/router/router.go` — extend routing to consider triage metadata
- `services/ai/registry/routing.go` — update model selection

**Scope:**
- Restructured Stage 4 prompt from design.md: verified entities + preliminary extraction + background context + untrusted content with `<untrusted_content>` delimiters
- Model selection based on triage category + importance (RISK_ISSUE -> Pro, PROJECT_UPDATE+HIGH -> Pro, MEDIUM -> Flash, etc.)
- Extend `DefaultModelSelector` to accept triage metadata in routing decision
- Output: verified assertions, sentiment, topic mapping, risk references with `lifecycle_change`, `context_excerpts`
- Replaces current `buildAnalysisPrompt()` / 8000-char truncation

**Acceptance criteria:**
- Analysis prompt includes resolved entities, SLM extraction, background context
- Model routing respects triage metadata (not just request type)
- Content is NOT truncated — full text or pre-processed summaries sent
- Output includes mandatory `context_excerpt` for every assertion
- Langfuse traces capture full prompt/completion
- Tests cover: prompt construction, model routing by triage, output schema validation

---

## WP7: Stage 4.5 — Persist Findings

**Shard:** `pf-67f20a` · **Agent:** worker-dev · **Depends on:** WP6, WP1

**Files:**
- `services/worker/activities/persist_activities.go` — new
- `services/worker/activities/persist_activities_test.go` — new
- `pkg/repository/` or equivalent — assertion write logic

**Scope:**
- Validates Stage 4 output: mandatory `context_excerpt`, allowlisted `lifecycle_event` and `reference_type` values, entity ID existence verification
- Writes to assertions table: new risks/decisions/actions with `assertion_root_id`, `lifecycle_event`
- Deduplication: binary match (LLM decides match or new), bias toward new
- Idempotency keys: `(source_id, assertion_type, extracted_text_hash)` for upsert
- Creates `assertion_references` rows with `reference_type` and `significance`
- Saga compensation: rollback partial writes on failure
- Updates `entity_project_affinity` scores
- Creates review queue items for unknown entities/acronyms

**Acceptance criteria:**
- New assertions created with `root_id`, `lifecycle_event`
- Existing assertions superseded correctly (`is_current=false`, `superseded_by` set)
- `assertion_references` created for every content-assertion link
- Validation rejects assertions without `context_excerpt`
- Idempotency: retry produces same result, no duplicates
- Saga compensation rolls back partial writes
- Tests cover: new assertion, supersede existing, validation rejection, idempotency, rollback

---

## WP8: Pipeline Orchestrator

**Shard:** `pf-7c09c8` · **Agent:** worker-dev · **Depends on:** WP2, WP3, WP4, WP5, WP6, WP1

**Files:**
- `services/worker/workflows/pipeline.go` — new (or extend content.go)
- `services/worker/workflows/pipeline_test.go` — new
- `services/worker/activities/register.go` — register new activities
- `pkg/temporal/options.go` — stage-to-preset mapping

**Scope:**
- Temporal workflow coordinating Stages 0 -> 1 -> 2 -> 3 -> 4 -> 4.5 -> 5
- Triage gates: LOW/PERSONAL skips to Stage 5 only; others continue
- Progressive availability: content keyword-searchable after Stage 0, entity-searchable after Stage 2
- Stage-to-preset mapping from design.md (parse=Fast, triage=Embedding, extract=Embedding per-chunk, analyze=LLM, etc.)
- Partial failure handling: per-chunk extraction failures merge available results; enrichment continues with unresolved entities
- Replaces current 8-stage workflow in `services/worker/workflows/content.go`

**Acceptance criteria:**
- Full pipeline executes for a test email (all stages)
- Triage correctly gates LOW content (skips Stages 2-4)
- Progressive availability: content searchable before Stage 4 completes
- Partial failure: extraction chunk failure doesn't block pipeline
- Stage 4 failure doesn't lose Stages 0-3 results
- Temporal retry policies correct per stage
- Tests cover: full pipeline, triage skip, partial failure, Stage 4 timeout

---

## WP9: Stage 5 — Multi-Level Embeddings

**Shard:** `pf-e0dbd7` · **Agent:** ai-dev · **Depends on:** WP7

**Files:**
- `services/worker/activities/embedding.go` — extend
- `services/worker/activities/embedding_repo.go` — extend
- Migration for embedding model versioning columns if not present

**Scope:**
- Embed multiple representations: raw text, summary, action items, risk assessments
- Embedding model versioning: `embedding_model` + `model_version` columns, index on both
- Segment-level embeddings for transcripts
- Glossary-expanded query support at index time
- Batch re-embedding capability for model changes

**Acceptance criteria:**
- Multiple embeddings generated per content item (raw, summary, action items)
- Model version tracked on every embedding
- Stale embedding detection query works
- Batch re-embedding for model change
- Tests cover: multi-level embedding generation, version tracking, stale detection

---

## WP10: Session Bootstrap

**Shard:** `pf-13d9dd` · **Agent:** cli-dev · **Depends on:** WP1, WP7

**Files:**
- `cmd/penf/cmd/context.go` — new or extend
- `cmd/penf/cmd/session.go` — extend
- `api/proto/gateway/v1/gateway.proto` — Context RPCs
- `services/gateway/` — context service handlers

**Scope:**
- `penf context morning` — playbook-based bootstrap per 07-session-bootstrap.md
- Playbook stored in Context-Palace, user-editable through conversation
- Per-step commands: `penf session resume --last-closed`, `penf pipeline status --since-last-session`, `penf meetings list`, `penf projects list --status active --sort activity`
- `penf context project --name X` — project drill-down
- `penf context session-end --summary "..."` — persist session summary
- `penf assertion briefing --root-id ID` — golden thread query
- All commands return JSON for Claude consumption

**Acceptance criteria:**
- `penf context morning` reads playbook, each step returns focused JSON
- Project drill-down returns watched risks, new risks, open actions, recent decisions
- Assertion briefing returns full lifecycle with golden thread
- Session end persists summary for next bootstrap
- Graceful degradation: failed step doesn't block others
- Tests cover: each command individually, playbook parsing

---

## WP11: Watch List + Trust/Seniority

**Shard:** `pf-d5b341` · **Agent:** cli-dev · **Depends on:** WP10, WP1

**Files:**
- `cmd/penf/cmd/watch.go` — new
- `cmd/penf/cmd/trust.go` — new (or extend people commands)
- `api/proto/gateway/v1/gateway.proto` — watch list + trust RPCs
- `services/gateway/` — handlers

**Scope:**
- Watch list CRUD: add/remove/annotate items per project
- Trust management: set `trust_level`, `trust_domains` on people
- Seniority management: set `seniority_tier` on people
- Briefing ordering by priority tiers: watched -> trusted -> senior -> other
- Seniority escalation detection query from design.md
- Peripheral change detection: `penf context changes --watched-only`

**Acceptance criteria:**
- Watch list add/remove/annotate works
- Trust level and domains settable on people
- Briefing ordering query returns correct tier ordering
- Seniority escalation detection identifies new senior participants
- Tests cover: CRUD operations, tier ordering, escalation detection

---

## WP12: Pipeline Introspection

**Shard:** `pf-8313b5` · **Agent:** cli-dev · **Depends on:** WP1, WP8

**Files:**
- `cmd/penf/cmd/pipeline.go` — extend with new subcommands
- `api/proto/pipeline/v1/pipeline.proto` — new RPCs
- `services/gateway/pipelineservice/service.go` — handlers

**Scope:**
- `penf pipeline describe [--stage X]` — stage registry with metadata, current model, prompt version
- `penf pipeline prompt show/history/diff/update/rollback/export` — prompt template management
- `penf pipeline reprocess --stage X --dry-run` — impact analysis with downstream cascade
- `penf pipeline reprocess --stage X [--all|--filter|--source]` — targeted reprocessing
- `penf pipeline history --source ID` — all stage runs for a source
- Version skew reporting in session bootstrap

**Acceptance criteria:**
- Pipeline describe shows all stages with current model/prompt versions
- Prompt show/update/rollback works with version history
- Reprocess dry-run shows affected sources and downstream impact
- Reprocess executes targeted re-runs
- Tests cover: describe output, prompt CRUD, dry-run impact calculation

---

## WP13: Documentation Update

**Shard:** `pf-2f4d11` · **Agent:** general-purpose · **Depends on:** WP8, WP10

**Files to update (currently wrong):**
- `context/ARCHITECTURE.md` — replace 8-stage pipeline with new 6-stage pipeline
- `context/architecture/core-patterns.md` — rewrite Phased Pipeline and Multi-Modal AI sections
- `context/agents/ai-dev.md` — replace "4-Step Content Pipeline" with new pipeline
- `context/agents/worker-dev.md` — update workflow catalog with new activities

**Files to update (currently incomplete):**
- `context/infrastructure.md` — LLM server purpose beyond mention resolution
- `context/architecture/observability-patterns.md` — add pipeline stage tracing
- `context/shared/use-cases.md` — enrich UC-2 with new capabilities

**New file:**
- `context/architecture/slm-llm-pipeline.md` — authoritative pipeline reference (distilled from `specs/020-slm-llm-architecture/design.md`)

**Scope:**

Document 7 new concepts currently missing from `/context/`:
1. SLM/LLM task split philosophy (if answer is in text use SLM, if requires reasoning use LLM)
2. Triage gates and skip logic (LOW/PERSONAL skip Stages 2-4)
3. Progressive availability (keyword-searchable after Stage 0, entity-searchable after Stage 2)
4. Assertion lifecycle and golden thread (`assertion_root_id`, `lifecycle_event`, `assertion_references`)
5. Trust and seniority weighting (two axes: organizational seniority vs personal trust)
6. Session bootstrap and radar model (`penf context morning`, AI tracks everything / human focuses spotlight)
7. Pipeline introspection (`penf pipeline describe`, prompt versioning, targeted reprocessing)

**Acceptance criteria:**
- No `context/` doc references the old 8-stage pipeline
- `ARCHITECTURE.md` accurately describes the new pipeline stages
- Agent guides (`ai-dev.md`, `worker-dev.md`) reflect new activities and patterns
- New `slm-llm-pipeline.md` exists as authoritative reference
- All 7 new concepts documented in at least one `context/` file
- Documentation is concise and actionable (not a copy of design.md)
