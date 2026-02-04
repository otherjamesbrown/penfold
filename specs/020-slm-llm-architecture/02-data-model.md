# Penfold: Data Model

## Overview

Penfold uses PostgreSQL 16+ with the pgvector extension for vector similarity search. The schema supports multi-tenant operation, a full content enrichment pipeline, entity resolution, AI extraction with audit trails, and a review queue for human-in-the-loop processing.

There are 27 migrations defining ~60 tables, ~30 enums, and extensive indexing.

## Schema Diagram (Logical Groupings)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           MULTI-TENANCY                                  │
│  tenants ──▶ tenant_domains, tenant_email_patterns,                     │
│              tenant_integrations, tenant_jira_mappings,                  │
│              tenant_processing_rules                                     │
└─────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        CONTENT & PIPELINE                                │
│  sources ──▶ content_enrichment ──▶ enrichment_stages                   │
│     │                                                                    │
│     ├──▶ email_threads ──▶ thread_messages                              │
│     ├──▶ email_attachments ──▶ attachment_enrichment                    │
│     ├──▶ source_attachments (parent↔child)                              │
│     └──▶ meetings ──▶ meeting_attendees, meeting_events,               │
│                       meeting_participants, meeting_mentions,            │
│                       meeting_series                                     │
└─────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       ENTITY RESOLUTION                                  │
│  people ──▶ person_aliases                                              │
│  teams ──▶ team_members (→ people)                                      │
│  projects ──▶ project_members (→ people, teams)                         │
│  products ──▶ product_aliases, product_teams, product_team_roles,       │
│              product_events ──▶ product_event_links                     │
│  glossary (with vector embeddings)                                      │
│  content_mentions ──▶ mention_patterns ──▶ entity_project_affinity     │
└─────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       AI EXTRACTION                                      │
│  assertions (risk, action, issue, decision, commitment, question)        │
│  content_sentiment                                                       │
│  prompt_templates ──▶ extraction_runs ──▶ extraction_feedback           │
│  extraction_experiments ──▶ extraction_experiment_results                │
│  content_insights ──▶ insight_type_registry                             │
└─────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     OPERATIONAL                                          │
│  review_queue (human-in-the-loop)                                       │
│  ingest_jobs ──▶ ingest_errors                                          │
│  dead_letter_items, queue_stats, processing_batches, workers            │
│  extracted_links ──▶ link_sources, link_enrichment                      │
│  jira_tickets ──▶ jira_ticket_changes                                   │
│  ai_models ──▶ ai_routing_rules, ai_model_health                       │
│  service_logs                                                            │
│  resolution_traces ──▶ resolution_trace_stages,                         │
│                        resolution_llm_calls, resolution_decisions        │
└─────────────────────────────────────────────────────────────────────────┘
```

## Key Tables

### Content & Enrichment Pipeline

**content_enrichment** — The central pipeline tracking table
| Column | Type | Purpose |
|--------|------|---------|
| source_id | BIGINT (FK) | Links to source content |
| content_type | ENUM | email, calendar, document, attachment |
| content_subtype | VARCHAR | More specific type |
| processing_profile | ENUM | full_ai, full_ai_chunked, metadata_only, state_tracking, structure_only, ocr_if_text |
| classification_confidence | REAL | How confident the classifier was |
| status | ENUM | pending → classifying → enriching → extracting → ai_processing → completed/failed/skipped |
| current_stage | VARCHAR | Which stage is currently running |
| participants | JSONB[] | Raw participant data |
| resolved_participants | JSONB[] | Resolved to known people |
| extracted_links | JSONB[] | Links found in content |
| thread_id | VARCHAR | Email thread grouping |
| project_id | BIGINT (FK) | Assigned project |
| extracted_data | JSONB | Flexible extraction results |
| ai_processed | BOOLEAN | Whether AI processing completed |

**enrichment_stages** — Per-stage tracking for debugging
- stage_name, processor_name, status, input_data, output_data, duration_ms

### People & Entity Resolution

**people** — Canonical person records
| Column | Type | Purpose |
|--------|------|---------|
| canonical_name | VARCHAR(500) | Display name |
| primary_email | VARCHAR(500) | Unique per tenant |
| account_type | ENUM | person, role, distribution, bot, external_service |
| is_internal | BOOLEAN | Internal vs external |
| confidence | REAL (0.0-1.0) | Resolution confidence |
| needs_review | BOOLEAN | Flagged for human review |
| auto_created | BOOLEAN | Created automatically vs manually |
| potential_duplicates | BIGINT[] | IDs of possible duplicates |
| title | VARCHAR(255) | Job title / role |
| department | VARCHAR(255) | Department within company |

**Proposed additions for seniority and trust** (see `00-overview.md` — collaboration philosophy):

| Column | Type | Purpose |
|--------|------|---------|
| seniority_tier | SMALLINT (1-7) | Organizational level: 1=IC → 3=senior/staff → 5=director → 7=C-level |
| trust_level | SMALLINT (0-5) | Human-assigned trust signal: 0=unset, 1=low → 5=high. Personal and subjective. |
| trust_note | TEXT | Optional: why you trust (or don't trust) this person's judgment |
| trust_domains | TEXT[] | Optional: specific areas of trusted expertise ("technical", "timelines", "risk assessment") |

Seniority and trust are **different axes** (see `00-overview.md`):
- **Seniority** is organizational fact — a VP attending a discussion that was previously junior-level is a signal regardless of trust
- **Trust** is human judgment — "when this person says it's an issue, I believe them"
- Both weight assertions during extraction and surface changes in peripheral monitoring

**person_aliases** — Multiple identifiers per person
- alias_type: email, slack_id, name, display_name
- alias_value, confidence, source (auto_created, gmail_api, manual)

### Assertions (RAID+)

**assertions** — Extracted structured findings from content
| Column | Type | Purpose |
|--------|------|---------|
| assertion_type | ENUM | risk, action, issue, decision, commitment, question |
| description | TEXT | What was asserted |
| source_quote | TEXT | Grounding quote from source |
| confidence | REAL | Extraction confidence |
| severity | ENUM | low, medium, high, critical |
| status | ENUM | open, in_progress, completed, cancelled |
| due_date | TIMESTAMPTZ | For actions/commitments |
| rationale | TEXT | For decisions |
| is_current | BOOLEAN | Latest version? |
| superseded_by | BIGINT (self-FK) | Version chain |
| owner_person_id | BIGINT (FK) | Who raised it |
| assignee_person_id | BIGINT (FK) | Who's responsible |
| target_person_id | BIGINT (FK) | Who it's about |
| decision_maker_person_id | BIGINT (FK) | Who decided |
| project_id | BIGINT (FK) | Related project |
| ticket_id | BIGINT (FK) | Linked Jira ticket |
| source_id | BIGINT | Content it was extracted from |
| thread_id | BIGINT (FK) | Email thread context |

The assertion versioning system uses `is_current` + `superseded_by` to track how assertions evolve across content items (e.g., a risk raised in meeting 1 gets escalated in meeting 5, decided in meeting 12).

### Glossary

**glossary** — Domain terminology with vector embeddings
| Column | Type | Purpose |
|--------|------|---------|
| term | VARCHAR(100) | The acronym/abbreviation |
| expansion | VARCHAR(500) | Full form |
| definition | TEXT | Longer explanation |
| context | JSONB[] | Tags for disambiguation |
| aliases | JSONB[] | Alternative spellings |
| expand_in_search | BOOLEAN | Whether to use for query expansion |
| linked_entity_type | VARCHAR | product, project, company |
| linked_entity_id | BIGINT | Which entity it links to |
| embedding | vector(1024) | For semantic matching |

### Products & Hierarchy

**products** — Business products with 3-level hierarchy
- product_type: product → sub_product → feature
- parent_id (self-FK) for hierarchy
- status: active, beta, sunset, deprecated

**product_events** — Timeline events for products
- event_type: decision, milestone, risk, release, competitor, org_change, market, note
- Links to source content via product_event_links

**product_team_roles** — Scoped role assignments
- role + scope (e.g., "DRI" for "PostgreSQL Support")
- Active date ranges for history

### Meetings

**meetings** — Canonical meeting records
- title, description, start_time, end_time, organizer_person_id
- Links to meeting_series for recurring meetings

**meeting_participants** — Resolved transcript participants
- display_name → person_id with match_type and confidence

**meeting_mentions** — People discussed (not just attending)
- Distinct from attendees: tracks who was talked about

### AI Infrastructure

**ai_models** — Model registry
| Column | Type | Purpose |
|--------|------|---------|
| id | TEXT | provider/model-name format |
| provider | TEXT | ollama, gemini, openai, anthropic, mlx |
| capabilities | TEXT[] | What the model can do |
| context_window | INTEGER | Token limit |
| input_cost_per_1k | DECIMAL | Cost tracking |
| is_local | BOOLEAN | Local vs remote |
| priority | INTEGER (0-100) | Selection priority |

**ai_routing_rules** — Task-based model routing
- task_type: embedding, summarization, extraction, classification
- preferred_models + fallback_models
- optimization_mode: latency, quality, cost, balanced
- require_local flag

**extraction_runs** — Full audit trail for AI calls
- template_id, model_id, model_version
- context_injected, full_prompt
- input_tokens, output_tokens, latency_ms
- raw_response, parsed_response, parse_errors

### Mention Resolution & Tracing

**content_mentions** — Entity mentions found in content
- entity_type: person, term, product, company, project
- mentioned_text, context_snippet, position
- resolution_source: exact_match, alias, fuzzy, project_context, prior_link, user_confirmed
- status: pending, auto_resolved, user_resolved, dismissed

**mention_patterns** — Learned resolution patterns
- pattern_text → resolved_entity_id
- times_seen, times_linked for confidence building

**resolution_traces** — Full debug traces for resolution process
- 4 stages: understanding → cross_mention → matching → verification
- Per-stage LLM calls logged with prompts and responses
- Individual decisions with reasoning and confidence

### Review Queue

**review_queue** — Human-in-the-loop processing
- question_type: acronym, person, entity, duplicate, other
- priority: high, medium, low
- status: pending, resolved, dismissed, deferred
- candidate_person_ids for disambiguation
- metadata JSONB for flexible context

## Key Enums

| Enum | Values | Used In |
|------|--------|---------|
| content_type | email, calendar, document, attachment | content_enrichment |
| processing_profile | full_ai, full_ai_chunked, metadata_only, state_tracking, structure_only, ocr_if_text | content_enrichment |
| assertion_type | risk, action, issue, decision, commitment, question | assertions |
| assertion_severity | low, medium, high, critical | assertions |
| action_status | open, in_progress, completed, cancelled | assertions |
| account_type | person, role, distribution, bot, external_service | people |
| mention_entity_type | person, term, product, company, project | content_mentions |
| mention_status | pending, auto_resolved, user_resolved, dismissed | content_mentions |
| product_type | product, sub_product, feature | products |

## Vector Search

Penfold uses pgvector for embedding-based similarity search:
- **Embedding dimension**: 1024 (mxbai-embed-large-v1)
- **Index type**: HNSW with cosine similarity
- **Indexed tables**: glossary (embedding column), sources (via search service)
- **Search modes**: Semantic only, keyword only, or hybrid (combines both with configurable weights)
