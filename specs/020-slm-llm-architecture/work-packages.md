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
Phase 10: WP14 (E2E Tests)                            — after WP8
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

WP14 (E2E Tests) ←──── WP8
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
| `pf-26405d` | WP14: E2E Pipeline Tests | testing-dev | WP8 |

## Completion Log

Record each work package completion here. Sessions should update this after committing.

| WP | Status | Commit | Date | Changes Summary |
|----|--------|--------|------|-----------------|
| WP1 | **done** | `0cdc532` | 2026-02-04 | 4 migrations (028–031): assertion lifecycle, pipeline registry, trust/seniority, watch list |
| WP2 | **done** | `52d1c8e` | 2026-02-04 | Email parse (HTML-to-text, quoted reply detection), transcript parse (SRT/VTT/TXT, speaker normalization), Temporal activities on Main/Email queues. 37 tests. |
| WP3 | **done** | `45d46b8` | 2026-02-04 | TriageContent gRPC RPC: proto definition, gateway proxy, AI server handler with triage prompt (8 categories, 3 importance levels), JSON validation, retry (max 2), Langfuse tracing, prompt template migration (032). 20+ tests. |
| WP4 | **done** | `f72b4b6` | 2026-02-04 | Two-pass extraction (NER + semantic) with chunking, merge dedup, quality gate. ExtractEntities RPC on AI service + gateway proxy + worker activity. 30+ tests. |
| WP5 | **done** | `b67e52b` | 2026-02-04 | Context package repository (7 query methods: risks, actions, decisions, events, glossary, project resolution), context builder activity (person resolution via fuzzy name, project resolution via exact/keyword, token budgets per content type, tail truncation), EntityLookupInterface + EntityResolverInterface. 26 tests. |
| WP6 | **done** | `2908da3` | 2026-02-04 | DeepAnalyze RPC (proto + AI server handler + client), structured prompt with `<untrusted_content>` wrapping, model selection by triage category/importance (Pro vs Flash), context_excerpt validation, worker activity with proto conversion, context builder activity with token-budgeted assembly. 30+ tests. |
| WP7 | **done** | `5f2da36` | 2026-02-04 | PersistRepository (validation: context_excerpt, lifecycle_event/reference_type allowlists, entity ID verification; idempotency keys via SHA256; assertion creation with root_id; supersession with is_current/superseded_by; assertion_references for every content-assertion link; entity_project_affinity UPSERT; single pgx transaction for saga compensation), PersistFindings Temporal activity registered on Main queue. 21 tests. |
| WP8 | **done** | `95b64e4` | 2026-02-04 | SLMPipelineWorkflow orchestrating Stages 0→1→2→3→4→4.5→5, triage gates (PERSONAL/INTERNAL_COMMS+LOW skip deep), progressive availability (parsed/extracted status), Stage 4 optional (failure continues), embedding critical (failure = pipeline failure), saga compensation, signal/query handlers, triage activity wrapper. 15 tests (8 pipeline + 7 triage). |
| WP9 | **done** | `dad3519` | 2026-02-04 | Multi-level embedding repository (StoreMultiLevelEmbedding, GetEmbeddingsForSource, GetStaleEmbeddings, DeleteEmbeddingsForSource), GenerateMultiLevelEmbeddings activity (content + summary + action_item + decision + risk embeddings per source, removed-action filtering, non-fatal assertion failures), ReEmbedBatch activity (stale detection, batch re-embedding for model changes), migration 033 (representation_type column, embedding_vec vector(1024) with HNSW index, model_version index). 13 tests. |
| WP10 | **done** | `a49ff38` | 2026-02-04 | Session bootstrap CLI: `penf context morning` (playbook from Context-Palace with default fallback), `penf session resume --last-closed`, `penf reminders list --due`, meeting list filters (participant/since/has-changes) + `penf meeting recap`, project list filters (status/sort/limit/always-include) + `penf context project --name`, `penf pipeline status --since-last-session`. Proto: ListMeetings filters, GetMeetingRecap RPC, GetProjectContext RPC, ProjectFilter enhancements. Gateway handlers + repository methods. 30+ tests. |
| WP11 | pending | — | — | — |
| WP12 | pending | — | — | — |
| WP13 | **done** | `9decec7` | 2026-02-04 | Created `context/architecture/slm-llm-pipeline.md` (283 lines) as authoritative pipeline reference. Updated HIGH priority docs: ARCHITECTURE.md (pipeline section), core-patterns.md (Sections 1-2 rewritten), ai-dev.md (6-stage pipeline), worker-dev.md (workflow catalog). Updated MEDIUM priority docs: infrastructure.md (LLM description), observability-patterns.md (stage tracing + KPIs), use-cases.md (UC-2/UC-5 enriched). All 7 new concepts documented. No old pipeline references remain. |
| WP14 | **done** | *pending* | 2026-02-04 | E2E pipeline tests: 10 test scenarios (full email pipeline, meeting VTT/SRT, triage gate skip, high-importance risk routing, golden thread assertion lifecycle, batch ingestion, partial failure recovery, idempotency, entity resolution). PipelineE2EEnv with Temporal client, DB query helpers (assertStageCompleted/Skipped, getAssertionsForSource, getGoldenThread, countEmbeddingsForSource). 5 new fixtures (011-risk-escalation.eml, 012-low-priority-fyi.eml, 013-thread-with-decisions.eml, 002-project-review.vtt, 003-incident-retro.srt). Build tag: `//go:build e2e`. |

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

## WP4: Stage 2 — Extract RPC ✓

**Shard:** `pf-1eca7f` · **Agent:** ai-dev · **Depends on:** WP3 · **Status:** done

**Files delivered:**
- `services/ai/server/extract.go` — Two-pass extraction handler: Stage 2a NER (people, dates, projects, orgs) + Stage 2b semantic (action items, decisions, risks), quality gate with focused risk re-prompt, JSON parsing with retry
- `services/ai/server/extract_test.go` — NER/semantic/quality-gate response parsing tests, prompt construction tests
- `services/worker/activities/extraction.go` — ExtractEntities Temporal activity with chunking (6K rune threshold, 1500 rune chunks with 200 overlap), merge deduplication across all entity types (case-insensitive composite keys)
- `services/worker/activities/extraction_test.go` — Short email, chunked extraction, merge dedup, quality gate trigger/non-trigger tests
- `services/worker/activities/interfaces.go` — ExtractEntities method on AIClient interface
- `pkg/ai/client.go` — ExtractEntities client method
- `pkg/ai/client_test.go` — Client-side RPC tests (nil request, successful extraction, minimal content)
- `services/gateway/modelservice/service.go` — ExtractEntities gateway proxy

**Acceptance criteria (met):**
- ExtractEntities RPC callable, returns merged NER + semantic results
- Chunking works for content up to 70K chars (1500-rune chunks with sentence-boundary splitting)
- Quality gate triggers re-run when triage=RISK_ISSUE but extraction returns 0 risks
- Merge logic deduplicates entities across chunks using code-based case-insensitive maps (not AI)
- Tests cover: short email extraction, chunked extraction, merge dedup, quality gate

---

## WP5: Stage 3 — Context Builder ✓

**Shard:** `pf-9816a7` · **Agent:** service-dev · **Depends on:** WP4, WP1 · **Status:** done

**Files delivered:**
- `services/worker/activities/context_repo.go` — ContextPackageRepo implementation: 7 query methods (GetActiveRisks, GetOpenActions, GetRecentDecisions, GetProductEvents, GetGlossaryTerms, ResolveProjectByName, ResolveProjectByKeyword) using pgx/pgxpool with NULL handling
- `services/worker/activities/context_repo_test.go` — 18 tests covering all query methods, empty results, NULL handling
- `services/worker/activities/context_builder.go` — BuildContextPackage Temporal activity: person resolution (exact/fuzzy name via EntityLookupInterface), project resolution (exact name/keyword/glossary), context package assembly with per-content-type token budgets, tail truncation (glossary→events→decisions→actions→risks)
- `services/worker/activities/context_builder_test.go` — 8 test functions: person resolution (exact/fuzzy/unresolved), project resolution (exact/keyword/unresolved), token budgets (meeting=3000/email=2000/slack=1000), truncation, empty extraction, unknown entity detection
- `services/worker/activities/interfaces.go` — Added ContextPackageRepository interface, ContextAssertion/ContextProductEvent/ContextGlossaryTerm types, DeepAnalyze method on AIClient

**Implementation shards:** `pf-31762a` (context package repository), `pf-561b00` (context builder activity)

**Acceptance criteria (met):**
- Resolves known people by fuzzy name match via SearchPeopleByName
- Glossary expansion via GetGlossaryTerms query with product scoping
- Context package respects token budgets (meeting=3000, email=2000, slack=1000), truncates from tail
- Unknown entities flagged: EntitiesUnresolved count + UnresolvedTerms list
- Tests cover: person resolution (exact/fuzzy/unresolved), project resolution (exact/keyword/unresolved), token budget enforcement, truncation, empty extraction, unknown entity detection (26 tests total)

---

## WP6: Stage 4 — Deep Analysis ✓

**Shard:** `pf-964bd7` · **Agent:** ai-dev, service-dev, worker-dev · **Depends on:** WP4, WP1 · **Status:** done

**Files delivered:**
- `api/proto/ai/v1/ai.proto` — DeepAnalyze RPC definition, DeepAnalyzeRequest/Response messages, 10 new message types (DeepSentiment, TopicMapping, VerifiedActionItem, VerifiedDecision, RiskReference, ImplicitActionItem)
- `api/proto/ai/v1/ai.pb.go`, `ai_grpc.pb.go` — regenerated proto code
- `services/ai/server/analyze.go` — DeepAnalyze handler: structured prompt construction (4 sections with `<untrusted_content>` wrapping), model selection by triage category+importance, JSON response parsing with context_excerpt validation, retry logic (max 2), Langfuse tracing
- `services/ai/server/analyze_test.go` — prompt construction, model selection (14 combinations), response parsing (valid/malformed/missing excerpts)
- `pkg/ai/client.go` — DeepAnalyze client method with 60s extended timeout
- `pkg/ai/client_test.go` — 3 client test cases
- `services/worker/activities/analysis.go` — DeepAnalyze Temporal activity with domain types, proto conversion helpers
- `services/worker/activities/analysis_test.go` — 5 test cases (success, empty content, no client, client error, proto conversion)
- `services/worker/activities/interfaces.go` — Extended AIClient interface with DeepAnalyze method

**Implementation shards:** `pf-d50e75` (proto + AI server + client), `pf-c79afe` (worker activity)

**Acceptance criteria (met):**
- Analysis prompt includes resolved entities, SLM extraction, background context
- Model routing respects triage metadata: RISK_ISSUE→Pro, CUSTOMER+HIGH→Pro, PROJECT_UPDATE+HIGH→Pro, MEDIUM→Flash, LOW→Flash
- Content is NOT truncated — full text sent within `<untrusted_content>` delimiters
- Output includes mandatory `context_excerpt` for every assertion (parser validates)
- Langfuse traces capture full prompt/completion via tracing.StartLLMCall/SetLLMResult
- Tests cover: prompt construction, model routing by triage (14 cases), output schema validation, context_excerpt enforcement

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

## WP8: Pipeline Orchestrator ✓

**Shard:** `pf-7c09c8` · **Agent:** worker-dev · **Depends on:** WP2, WP3, WP4, WP5, WP6, WP1 · **Status:** done

**Files delivered:**
- `services/worker/workflows/pipeline.go` — SLMPipelineWorkflow: Stages 0→1→2→3→4→4.5→5, triage gates, progressive availability (parsed/extracted status updates), Stage 4 optional, embedding critical, saga compensation, signal/query handlers, pipeline-local mirror types for JSON-compatible activity I/O
- `services/worker/workflows/pipeline_test.go` — 8 tests: full pipeline, triage skip, Stage 4 failure, embedding failure, meeting transcript, Stage 4 timeout, query status, cancellation signal
- `services/worker/activities/triage_activities.go` — Stage 1 triage activity wrapper: TriageInput/Output, shouldSkipDeep logic, heartbeat/tracing, TriageContent RPC call
- `services/worker/activities/triage_activities_test.go` — 7 tests: success, skip_deep PERSONAL, skip_deep LOW INTERNAL_COMMS, no-skip HIGH INTERNAL_COMMS, empty content, nil AI client, AI client error

**Implementation shards:** `pf-cac6e6` (triage activity), `pf-e8600d` (pipeline orchestrator)

**Acceptance criteria (met):**
- Full pipeline executes for a test email (all 7 stages)
- Triage correctly gates PERSONAL/INTERNAL_COMMS+LOW content (skips Stages 2-4.5)
- Progressive availability: UpdateContentStatus called with "parsed" after Stage 0, "extracted" after Stage 2
- Stage 4 failure does not fail pipeline — continues to embedding
- Embedding failure fails the pipeline
- Stage-to-preset mapping: parse=Fast, triage=Embedding, extract=Embedding, context=Fast, analyze=LLM, persist=Fast, embed=Embedding
- Tests cover: full pipeline, triage skip, Stage 4 failure, embedding failure, meeting transcript, Stage 4 timeout, query status, cancellation signal (15 tests total)

---

## WP9: Stage 5 — Multi-Level Embeddings ✓

**Shard:** `pf-e0dbd7` · **Agent:** data-dev, worker-dev · **Depends on:** WP7 · **Status:** done

**Files delivered:**
- `services/worker/activities/interfaces.go` — Extended EmbeddingRepository with StoreMultiLevelEmbedding, GetEmbeddingsForSource, GetStaleEmbeddings, DeleteEmbeddingsForSource; added MultiLevelEmbeddingInput type; updated Embedding struct with EntityType, EntityID, RepresentationType, ModelVersion
- `services/worker/activities/embedding_repo.go` — Implemented all 4 new repository methods with validation, backward-compatible StoreEmbedding delegation
- `services/worker/activities/embedding_repo_test.go` — 5 repository tests (mock-based)
- `services/worker/activities/multilevel_embedding.go` — GenerateMultiLevelEmbeddings activity (content + summary + action_item + decision + risk embeddings, removed-action filtering, non-fatal assertion failures, 8192 char truncation), ReEmbedBatch activity (stale detection, batch re-embedding with source fetch)
- `services/worker/activities/multilevel_embedding_test.go` — 8 activity tests (full pipeline, empty analysis, skips removed, assertion failure continues, validation errors, batch stale processing, sources remaining, batch validation)
- `migrations/033_multi_level_embeddings.sql` — representation_type column, embedding_vec vector(1024) with HNSW index (m=16, ef_construction=64), model_version index, representation/entity indexes

**Implementation shards:** `pf-26ef5d` (repository + migration), `pf-6a9975` (activities + tests)

**Acceptance criteria (met):**
- Multiple embeddings generated per content item: content, summary, action_item, decision, risk
- Model version tracked on every embedding via ModelVersion field
- Stale embedding detection via GetStaleEmbeddings query (filters by model+version)
- Batch re-embedding via ReEmbedBatch activity (fetches stale sources, deletes old embeddings, generates new)
- Removed/filtered assertions skipped (status="removed")
- Individual assertion embedding failures don't fail the whole activity
- HNSW vector index for semantic search (pgvector, 1024 dimensions)
- Tests cover: multi-level embedding generation (6 types), version tracking, stale detection, batch re-embedding, validation, error resilience (13 tests total)

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

---

## WP14: E2E Pipeline Tests

**Shard:** `pf-26405d` · **Agent:** testing-dev · **Depends on:** WP8

**Files:**
- `tests/e2e/slm_pipeline_test.go` — new: full pipeline E2E tests
- `tests/e2e/slm_pipeline_helpers_test.go` — new: pipeline-specific helpers (stage polling, assertion verification)
- `tests/fixtures/acme-corp/emails/011-risk-escalation.eml` — new: high-importance risk email (triggers full Stage 2-4)
- `tests/fixtures/acme-corp/emails/012-low-priority-fyi.eml` — new: LOW/PERSONAL email (triggers triage skip)
- `tests/fixtures/acme-corp/emails/013-thread-with-decisions.eml` — new: multi-decision email for golden thread
- `tests/fixtures/acme-corp/meetings/002-project-review.vtt` — new: VTT transcript with risks, actions, decisions
- `tests/fixtures/acme-corp/meetings/003-incident-retro.srt` — new: SRT transcript for format coverage

**Scope:**

True end-to-end tests that send real content through the full SLM/LLM pipeline with **no mocks**. All tests use the `e2e` build tag and require real services: PostgreSQL (dev02), Temporal, AI service (local SLM), and Gateway.

### Test Scenarios

**1. Full email pipeline (happy path)**
- Ingest `001-project-update.eml` via CLI or Temporal signal
- Wait for pipeline workflow to complete (poll `pipeline_runs` table)
- Verify each stage executed: Stage 0 (parsed text stored), Stage 1 (triage category+importance), Stage 2 (entities extracted — people, projects, glossary terms), Stage 3 (context package built with resolved entities), Stage 4 (assertions created with `context_excerpt`), Stage 4.5 (assertions persisted, `assertion_references` created), Stage 5 (embeddings generated)
- Assert: source status = completed, pipeline_runs has entries for all stages

**2. Meeting transcript pipeline**
- Ingest `002-project-review.vtt` transcript
- Verify Stage 0 strips VTT timestamps and normalizes speakers
- Verify Stage 2 extracts people mentioned in transcript
- Verify Stage 3 resolves speakers against `people` table (fuzzy match)
- Verify Stage 4 produces assertions (risks, decisions, action items from meeting)
- Assert: assertions have correct `source_type = 'meeting'`

**3. Triage gate — LOW content skips Stages 2-4**
- Ingest `012-low-priority-fyi.eml` (content designed to triage as LOW or PERSONAL)
- Verify Stage 1 returns LOW or PERSONAL category
- Verify Stages 2-4 are NOT executed (no pipeline_runs entries)
- Verify Stage 5 still generates embeddings (content is searchable)
- Assert: source is searchable but has no assertions

**4. High-importance risk escalation**
- Ingest `011-risk-escalation.eml` (designed to triage as RISK_ISSUE + HIGH)
- Verify triage routes to RISK_ISSUE category
- Verify Stage 2 quality gate triggers (risk extraction re-run with focused prompt)
- Verify Stage 4 uses Pro model (not Flash)
- Verify assertions include `lifecycle_event` and `context_excerpt`
- Assert: at least one risk assertion persisted with `assertion_references`

**5. Golden thread — assertion lifecycle across documents**
- Ingest `001-project-update.eml` first (creates initial risk assertion)
- Then ingest `013-thread-with-decisions.eml` (references same risk)
- Verify second ingestion creates assertion with `assertion_root_id` pointing to original
- Verify `lifecycle_event` reflects progression (e.g., raised → mitigated)
- Assert: querying by `assertion_root_id` returns full lifecycle chain

**6. Entity resolution accuracy**
- Seed acme-corp fixtures (people, glossary, projects, teams)
- Ingest `001-project-update.eml` (mentions "John Smith", "Sarah Chen", "TER", "Project Alpha")
- Verify Stage 3 resolves: people → existing `people` rows (by email or fuzzy name), glossary → "TER" expands to "Technical Execution Review", projects → "Project Alpha" matched
- Verify unresolved entities flagged in context output
- Assert: resolved entity counts > 0, person_ids populated

**7. Batch ingestion — multiple emails**
- Ingest all 10 acme-corp emails in sequence
- Verify all 10 complete the pipeline (no hangs, no deadlocks)
- Verify cross-document entity resolution is consistent (same person gets same person_id)
- Assert: 10 sources completed, assertions deduplicated across documents

**8. Partial failure recovery**
- Requires: ability to inject failure (kill AI service mid-pipeline, or use a fixture that causes Stage 4 timeout)
- Ingest email, interrupt Stage 4
- Verify Stages 0-3 results are preserved (not rolled back)
- Verify source is marked as partially failed (not completed, not fully failed)
- Verify re-triggering pipeline resumes from failed stage
- Assert: `pipeline_runs` shows completed stages + failed stage

**9. Idempotency — duplicate ingestion**
- Ingest `001-project-update.eml` twice with same content
- Verify second ingestion is detected as duplicate (same `message_id` or `content_hash`)
- Assert: no duplicate assertions, no duplicate embeddings

**10. SRT transcript format**
- Ingest `003-incident-retro.srt` (SRT format)
- Verify Stage 0 correctly strips SRT timestamps and entry numbers
- Verify pipeline completes identically to VTT (format-agnostic after Stage 0)

### Environment Requirements

```
Services required:
  - PostgreSQL on dev02 (penfold_test_e2e database)
  - Temporal server (docker-compose or dev01)
  - AI service with local SLM (Qwen 7B or similar)
  - Gateway service (proxies AI RPCs)

Environment variables:
  - PENFOLD_DB_PASSWORD (required, skip if absent)
  - TEMPORAL_HOST (default: localhost:7233)
  - AI_SERVICE_URL (default: localhost:8080)
  - GATEWAY_URL (default: localhost:9090)
```

### Helpers

- `waitForPipelineComplete(sourceID, timeout)` — polls `pipeline_runs` until all expected stages complete or timeout
- `assertStageCompleted(sourceID, stageName)` — checks `pipeline_runs` for stage entry with status=completed
- `assertStageSkipped(sourceID, stageName)` — verifies no `pipeline_runs` entry exists for stage
- `getAssertionsForSource(sourceID)` — queries assertions table joined with `assertion_references`
- `getGoldenThread(rootID)` — queries assertion lifecycle chain by `assertion_root_id`
- `countEmbeddingsForSource(sourceID)` — counts embeddings generated for source

**Acceptance criteria:**
- All 10 test scenarios pass with real services (no mocks)
- Tests use `//go:build e2e` tag, skip gracefully when services unavailable
- Each test is independent (clean slate via `TruncateAllTables` + fixture reload)
- Tests complete within 5 minutes total (individual test timeout: 60s)
- Pipeline stage verification uses DB queries, not Temporal internal state
- At least 5 new fixture files added for targeted test scenarios
- Tests are runnable via `go test -tags=e2e -timeout 5m ./tests/e2e/... -run TestSLMPipeline`
