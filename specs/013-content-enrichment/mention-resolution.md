# Unified Mention Resolution System

Part of [Content Enrichment Pipeline](spec.md)

**Beads**: pe-7i1s (Soft Corrections), pe-3m44 (Person Extraction), pe-3mn6 (Resolution Algorithm)

---

## Clarifications

### Session 2026-01-21

- Q: What is the acceptable LLM processing latency per content item? → A: <30 seconds default, must be configurable
- Q: What happens when LLM fails (timeout, parse error, unavailable)? → A: Retry locally (2x), then queue for human review
- Q: How long should trace data be retained? → A: 90 days full traces, 1 year decisions/corrections only

---

## Overview

A unified system for resolving mentioned text in content to canonical entities. Handles:

| Entity Type | Example | Resolution |
|-------------|---------|------------|
| **Person** | "Alan" | Allen Duet vs Alan Evans |
| **Term** | "VIP", "LKE" | Acronym expansion + linked entity |
| **Product** | "the database" | LKE Managed Databases vs DBaaS |
| **Company** | "the competitor" | Cloudflare vs AWS |
| **Project** | "the project" | MTC vs TikTok FY26 |

All entity types share:
- **Context-aware ranking**: Project context affects candidate ordering
- **Prior link history**: "You've linked this 5x before"
- **Soft vs permanent**: Content-level links vs glossary/entity definitions
- **Multi-candidate suggestions**: Primary + alternatives for ambiguous matches
- **Claude-native batch processing**: Full context for intelligent resolution

---

## Key Concepts

### Soft Links vs Permanent Links

| Type | Scope | Use Case |
|------|-------|----------|
| **Permanent** | Global | "LKE" always expands to "Linode Kubernetes Engine" |
| **Soft** | Single content item | "OKEE" → "LKE" in this transcript only (transcription error) |
| **Project-scoped** | Within project context | "VIP" → "Virtual IP" in networking projects |

### Term-Entity Linking

Terms (acronyms) can link to canonical entities:

```
LKE (term)
├── expansion: "Linode Kubernetes Engine"
└── linked_entity: Product:LKE (teams, timeline, decisions)

MTC (term)
├── expansion: "Major TikTok Contract"
└── linked_entity: Project:MTC (Jira, members, deadlines)
```

This enables:
- Search "LKE" → finds content AND product context
- AI can say "LKE is a product with these team members..."

---

## Data Model

### Glossary Terms (Extended)

```sql
-- Extend existing glossary_terms table
ALTER TABLE glossary_terms ADD COLUMN linked_entity_type TEXT;  -- product, project, company
ALTER TABLE glossary_terms ADD COLUMN linked_entity_id BIGINT;

-- Examples:
-- LKE: expansion="Linode Kubernetes Engine", linked_entity_type="product", linked_entity_id=12
-- MTC: expansion="Major TikTok Contract", linked_entity_type="project", linked_entity_id=5
```

### Content Mentions (Unified)

All mentions extracted from content, regardless of entity type:

```sql
CREATE TABLE content_mentions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    content_id BIGINT NOT NULL,

    -- What was mentioned
    entity_type TEXT NOT NULL,          -- person, term, product, company, project
    mentioned_text TEXT NOT NULL,       -- "Alan", "VIP", "the database"
    position INT,                       -- Character offset in content
    context_snippet TEXT,               -- Surrounding text for disambiguation

    -- Resolution
    resolved_entity_id BIGINT,          -- FK to appropriate table based on entity_type
    resolution_confidence DECIMAL(3,2),
    resolution_source TEXT,             -- exact_match, alias, project_context, prior_link, user_confirmed

    -- For terms: also track expansion used
    resolved_expansion TEXT,            -- "Linode Kubernetes Engine" (for term type)

    -- Candidates at extraction time
    candidates JSONB,                   -- [{entity_id, confidence, reasons}]

    -- Review status
    status TEXT DEFAULT 'pending',      -- pending, auto_resolved, user_resolved, dismissed
    resolved_at TIMESTAMPTZ,
    resolved_by TEXT,

    -- Project context at mention time
    project_context_id BIGINT,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT valid_entity_type CHECK (entity_type IN ('person', 'term', 'product', 'company', 'project'))
);

CREATE INDEX idx_content_mentions_content ON content_mentions(content_id);
CREATE INDEX idx_content_mentions_pending ON content_mentions(tenant_id, status) WHERE status = 'pending';
CREATE INDEX idx_content_mentions_entity ON content_mentions(tenant_id, entity_type, resolved_entity_id);
CREATE INDEX idx_content_mentions_text ON content_mentions(tenant_id, entity_type, lower(mentioned_text));
```

### Mention Patterns (Soft Links + History)

Tracks resolution patterns for suggestions:

```sql
CREATE TABLE mention_patterns (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,

    -- Pattern definition
    entity_type TEXT NOT NULL,
    pattern_text TEXT NOT NULL,         -- "Alan", "OKEE", "the database"

    -- Resolution target
    resolved_entity_id BIGINT,          -- NULL if pattern exists but unresolved
    resolved_expansion TEXT,            -- For terms without entity link

    -- Scope
    project_id BIGINT,                  -- NULL = global pattern
    is_permanent BOOLEAN DEFAULT false, -- true = glossary/entity alias, false = soft pattern

    -- Usage tracking
    times_seen INT DEFAULT 1,
    times_linked INT DEFAULT 0,
    last_seen_at TIMESTAMPTZ,
    last_linked_at TIMESTAMPTZ,

    -- Source tracking
    first_content_id BIGINT,            -- Where pattern was first seen
    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, entity_type, pattern_text, project_id)
);

CREATE INDEX idx_mention_patterns_lookup
    ON mention_patterns(tenant_id, entity_type, lower(pattern_text));
CREATE INDEX idx_mention_patterns_project
    ON mention_patterns(tenant_id, project_id) WHERE project_id IS NOT NULL;
```

### Entity Affinity (Project Context)

Tracks entity-project relationships for ranking:

```sql
CREATE TABLE entity_project_affinity (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,

    entity_type TEXT NOT NULL,
    entity_id BIGINT NOT NULL,
    project_id BIGINT NOT NULL,

    -- Affinity metrics
    mention_count INT DEFAULT 0,
    last_mentioned_at TIMESTAMPTZ,
    is_member BOOLEAN DEFAULT false,    -- Explicit project member (for persons)
    role TEXT,                          -- Role in project context

    -- Computed affinity score
    affinity_score DECIMAL(3,2) DEFAULT 0.5,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, entity_type, entity_id, project_id)
);

CREATE INDEX idx_entity_affinity_project ON entity_project_affinity(tenant_id, project_id);
CREATE INDEX idx_entity_affinity_entity ON entity_project_affinity(tenant_id, entity_type, entity_id);
```

---

## Resolution Algorithm

### Design Principle: LLM-Driven Resolution

Resolution decisions are made by an LLM, not rule-based code. Code handles context gathering and data preparation; the LLM analyzes and decides.

**Rationale:**
- Local LLM (MLX) means no cost constraint
- Async processing means no time constraint
- LLM handles nuance (phonetic variants, transcription errors, context) better than rules
- Cross-mention reasoning improves accuracy

### Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Content Ingestion                     │
└─────────────────────┬───────────────────────────────────┘
                      ▼
┌─────────────────────────────────────────────────────────┐
│  Stage 1: Extraction + Understanding (LLM)              │
│  - Extract mentions from content                         │
│  - Free-form understanding of each mention               │
│  - Flag likely transcription errors                      │
└─────────────────────┬───────────────────────────────────┘
                      ▼
┌─────────────────────────────────────────────────────────┐
│  Stage 2: Cross-Mention Reasoning (LLM)                 │
│  - Group mentions by content                             │
│  - Identify mention relationships                        │
│  - Build unified context understanding                   │
└─────────────────────┬───────────────────────────────────┘
                      ▼
┌─────────────────────────────────────────────────────────┐
│  Stage 3: Entity Matching (LLM + DB)                    │
│  - Fetch candidate entities based on understanding       │
│  - LLM matches with confidence + reasoning               │
│  - Suggest new entities if no match                      │
└─────────────────────┬───────────────────────────────────┘
                      ▼
┌─────────────────────────────────────────────────────────┐
│  Stage 4: Verification (LLM) [confidence < 0.9]         │
│  - Challenge uncertain resolutions                       │
│  - Check cross-mention consistency                       │
└─────────────────────┬───────────────────────────────────┘
                      ▼
┌──────────────────────┬──────────────────────────────────┐
│  confidence >= 0.8   │  confidence < 0.8                │
│  Auto-apply          │  Human review queue              │
└──────────────────────┴──────────────────────────────────┘
                      ▼
┌─────────────────────────────────────────────────────────┐
│  Learning Loop                                           │
│  - Track corrections                                     │
│  - Update resolution patterns                            │
│  - Periodic cluster analysis                             │
└─────────────────────────────────────────────────────────┘
```

### Stage 1: Extraction + Understanding

LLM reads content and describes what it understands about each mention, without database lookup:

**Input:**
- Raw content text
- Content metadata (type, date, participants)
- Project context (if known)

**LLM Output:**
```json
{
  "mentions": [
    {
      "text": "Alan",
      "entity_type": "person",
      "position": 145,
      "context_snippet": "Alan will handle the LKE integration testing",
      "understanding": "A person who will handle LKE integration testing. Likely technical role, familiar with Kubernetes. Context suggests they're on the MTC project team.",
      "transcription_flags": {
        "likely_error": false,
        "phonetic_variants": ["Allen", "Allan"]
      }
    },
    {
      "text": "OKEE",
      "entity_type": "term",
      "position": 312,
      "context_snippet": "the OKEE cluster is running well",
      "understanding": "Appears to be a transcription error for 'LKE' based on phonetic similarity and context ('OKEE cluster' in Kubernetes discussion).",
      "transcription_flags": {
        "likely_error": true,
        "probable_correction": "LKE",
        "confidence": 0.9
      }
    }
  ]
}
```

**Why separate understanding from matching:**
- LLM might understand context that code-based candidate search would miss
- Captures phonetic/transcription issues early
- Enables cross-mention reasoning in next stage

### Stage 2: Cross-Mention Reasoning

Process all mentions from same content together to identify relationships:

**Input:**
- All mentions from Stage 1
- Full content for reference

**LLM Output:**
```json
{
  "content_id": 123,
  "unified_understanding": "This transcript discusses LKE (Linode Kubernetes Engine) integration for MTC project. Technical work being assigned.",
  "mention_relationships": [
    {
      "from_mention": "Alan",
      "to_mention": "LKE",
      "relationship": "will_work_on",
      "inference": "If LKE resolves to product, person working on it is likely LKE team member"
    },
    {
      "from_mention": "OKEE",
      "to_mention": "LKE",
      "relationship": "transcription_of",
      "inference": "Same technical context, phonetic similarity"
    }
  ],
  "resolution_hints": [
    "Look for person on LKE team with name similar to 'Alan'",
    "OKEE should resolve to same entity as LKE"
  ]
}
```

**Why cross-mention reasoning:**
- "Alan will demo LKE" - resolving LKE helps resolve Alan
- Mentions in same content share context
- Catches inconsistencies (two different "Alan"s shouldn't resolve to same person)

### Stage 3: Entity Matching

Now match understanding to database entities:

**Input (Code-Gathered):**
- Understanding from Stages 1-2
- Candidate entities from database:
  - Fuzzy name search results
  - Glossary/alias lookups
  - Project member lists
  - Prior link history
  - Entity-project affinity scores

**LLM Output:**
```json
{
  "resolutions": [
    {
      "mention_text": "Alan",
      "mention_position": 145,
      "decision": "resolve",
      "resolved_to": {
        "entity_type": "person",
        "entity_id": 101,
        "entity_name": "Allen Duet"
      },
      "confidence": 0.88,
      "reasoning": "Phonetic match (Alan/Allen) + Allen Duet is LKE PM + MTC team member + technical context aligns with PM role. No other 'Alan' variants on project.",
      "factors": {
        "phonetic_match": 0.85,
        "project_membership": true,
        "role_alignment": "high",
        "prior_links": 5
      },
      "alternatives_considered": [
        {
          "entity_id": 203,
          "entity_name": "Alan Evans",
          "confidence": 0.25,
          "rejection_reason": "Not on MTC project, different technical domain"
        }
      ]
    },
    {
      "mention_text": "OKEE",
      "mention_position": 312,
      "decision": "resolve",
      "resolved_to": {
        "entity_type": "term",
        "entity_id": 15,
        "term": "LKE",
        "expansion": "Linode Kubernetes Engine"
      },
      "confidence": 0.95,
      "reasoning": "Phonetic transcription error. Context is Kubernetes cluster discussion. LKE is established term for Linode Kubernetes Engine. No legitimate 'OKEE' term exists.",
      "is_transcription_error": true,
      "linked_entity": {
        "type": "product",
        "id": 12,
        "name": "LKE"
      }
    }
  ],
  "new_entities_suggested": [
    {
      "mention_text": "DataDog",
      "suggested_type": "company",
      "suggested_name": "Datadog Inc",
      "reasoning": "Mentioned as monitoring solution being evaluated. Not in company database. Appears to be the vendor Datadog.",
      "confidence": 0.85
    }
  ]
}
```

### Stage 4: Verification (Optional)

For resolutions with confidence < 0.9, a separate verification pass:

**Input:**
- Resolution from Stage 3
- Full content
- Challenge prompts

**LLM Task:**
- "You resolved 'Alan' to Allen Duet. The transcript also mentions 'Al called in late.' Is 'Al' the same person or different?"
- Challenge the resolution, look for contradictions

**Output:**
```json
{
  "mention_text": "Alan",
  "original_confidence": 0.88,
  "verification_result": "confirmed",
  "adjusted_confidence": 0.88,
  "verification_notes": "No contradictory evidence found. 'Al' reference in line 45 is consistent with Allen Duet (common nickname)."
}
```

### Auto-Resolution Thresholds

| Confidence | Action |
|------------|--------|
| >= 0.90 | Auto-apply, no verification needed |
| 0.80 - 0.89 | Auto-apply after verification pass |
| < 0.80 | Queue for human review |

### Batch Processing

Mentions are processed in batches by content:
- All mentions from one email/meeting processed together
- Enables cross-mention reasoning
- Reduces LLM calls vs per-mention processing

```go
type ResolutionBatch struct {
    ContentID     int64
    ContentType   string
    Mentions      []ExtractedMention
    ProjectID     *int64
}

func (r *Resolver) ProcessBatch(ctx context.Context, batch ResolutionBatch) (*BatchResult, error) {
    trace := r.tracer.StartTrace(batch.ContentID)
    defer trace.Complete()

    // Stage 1: Understanding
    understanding, err := r.llm.ExtractAndUnderstand(ctx, batch)
    trace.RecordStage(1, understanding)

    // Stage 2: Cross-mention reasoning
    relationships, err := r.llm.ReasonAcrossMentions(ctx, understanding)
    trace.RecordStage(2, relationships)

    // Stage 3: Gather candidates (code) + Match (LLM)
    candidates := r.gatherCandidates(ctx, understanding, relationships)
    resolutions, err := r.llm.MatchEntities(ctx, understanding, relationships, candidates)
    trace.RecordStage(3, resolutions)

    // Stage 4: Verification for uncertain resolutions
    for _, res := range resolutions.NeedingVerification() {
        verified, err := r.llm.Verify(ctx, res, batch)
        trace.RecordStage(4, verified)
        res.ApplyVerification(verified)
    }

    // Apply results
    return r.applyResolutions(ctx, resolutions, trace)
}
```

### LLM Provider Configuration

```go
type LLMConfig struct {
    Provider        string  // "mlx", "claude", "openai"
    Model           string  // "mistral-7b", "claude-3-sonnet", etc.

    // Performance settings
    TimeoutPerContent time.Duration  // Default: 30s, configurable
    MaxRetries        int            // Default: 2

    // Escalation settings
    EscalateOnLowConfidence bool
    EscalationThreshold     float64  // 0.7 = escalate if < 0.7
    EscalationProvider      string   // "claude" for hard cases
}

// Default: local MLX for all processing
// Future: escalate ambiguous cases to Claude
```

### Error Handling

When LLM processing fails:

1. **Retry locally** (up to `MaxRetries`, default 2)
2. **Log failure** with error details in trace
3. **Queue for human review** - content marked as `status=llm_failed`
4. **Alert if pattern** - if failure rate exceeds threshold, alert ops

Failure types:
- **Timeout**: LLM didn't respond within `TimeoutPerContent`
- **Parse error**: LLM response didn't match expected JSON schema
- **Unavailable**: LLM service not reachable
- **Rate limit**: LLM provider throttling (relevant for Claude escalation)

### Learning Loop

When human corrects a resolution:

1. **Record correction** with context features
2. **Update mention patterns** - track what the correct resolution was
3. **Feed to future context** - "In similar contexts, consider X"

Periodic batch analysis:
- Cluster similar unresolved mentions
- "Alan" appears 50 times → resolve once, suggest for all
- Identify systematic errors (e.g., always missing fuzzy matches)

---

## CLI Interface

### Unified Questions Queue

```bash
penf review questions list

# Output:
#  ID    TYPE     PRI    TEXT              PROJECT   PRIMARY CANDIDATE
#  --    ----     ---    ----              -------   -----------------
#  72    person   high   Alan              MTC       Allen Duet (85%)
#  73    term     med    VIP               Network   Virtual IP (90%)
#  74    product  low    the database      -         LKE Managed DB (70%)
#  75    term     high   OKEE              MTC       → LKE (80%, transcription?)
```

### Resolution Options

```bash
# Link to entity (soft link for this content)
penf review questions resolve 72 --link-to person:101

# Link term and specify it's a transcription error
penf review questions resolve 75 --link-to term:LKE --transcription-error

# Make permanent (add as alias/glossary entry)
penf review questions resolve 72 --link-to person:101 --make-permanent

# Dismiss
penf review questions dismiss 74 --reason "Generic reference, not specific product"
```

### Link Term to Entity

```bash
# Link LKE term to LKE product
penf glossary link LKE --to product:12

# Link MTC term to MTC project
penf glossary link MTC --to project:5

# Show term with linked entity
penf glossary show LKE

# Output:
# Term: LKE
# Expansion: Linode Kubernetes Engine
# Linked Entity: Product "LKE" (id: 12)
#   - Team: Cloud Native
#   - PM: Kate Williams
#   - 45 timeline events
```

---

## Claude-Native Batch Processing

### Context Command

```bash
penf process mentions context --output json
```

Returns all pending mentions with full context:

```json
{
  "mentions": [
    {
      "id": 72,
      "entity_type": "person",
      "mentioned_text": "Alan",
      "context_snippet": "Alan will handle the LKE integration testing",
      "project": {"id": 5, "name": "MTC"},
      "candidates": [
        {
          "entity_id": 101,
          "name": "Allen Duet",
          "confidence": 0.85,
          "reasons": ["Project member", "LKE PM", "Fuzzy name match"],
          "prior_links": 5,
          "project_role": "PM for LKE"
        },
        {
          "entity_id": 203,
          "name": "Alan Evans",
          "confidence": 0.30,
          "reasons": ["Exact first name match"],
          "prior_links": 0
        }
      ]
    },
    {
      "id": 75,
      "entity_type": "term",
      "mentioned_text": "OKEE",
      "context_snippet": "the OKEE cluster is running well",
      "project": {"id": 5, "name": "MTC"},
      "candidates": [
        {
          "entity_id": 15,
          "term": "LKE",
          "expansion": "Linode Kubernetes Engine",
          "confidence": 0.80,
          "reasons": ["Phonetic similarity", "Prior link 3x in MTC"],
          "prior_links": 3,
          "linked_entity": {
            "type": "product",
            "id": 12,
            "name": "LKE"
          },
          "transcription_likelihood": 0.9
        },
        {
          "entity_id": 42,
          "term": "OKE",
          "expansion": "Oracle Kubernetes Engine",
          "confidence": 0.40,
          "reasons": ["Exact match minus one char"],
          "prior_links": 0
        }
      ]
    }
  ],
  "workflow": {
    "auto_resolve_threshold": 0.9,
    "suggest_threshold": 0.7,
    "prior_link_boost_threshold": 5
  },
  "stats": {
    "total_pending": 12,
    "by_type": {"person": 5, "term": 4, "product": 2, "company": 1},
    "auto_resolvable": 8
  }
}
```

### Batch Resolve

```bash
penf process mentions batch-resolve '{
  "resolutions": [
    {"id": 72, "entity_id": 101, "make_permanent": false},
    {"id": 75, "entity_id": 15, "make_permanent": false, "transcription_error": true}
  ],
  "dismissals": [
    {"id": 74, "reason": "Generic reference"}
  ]
}'
```

---

## Search Integration

When searching, the system:

1. **Expands terms** using glossary (LKE → "Linode Kubernetes Engine")
2. **Includes soft-linked content** (content where OKEE → LKE)
3. **Surfaces linked entities** (LKE term → LKE product context)

```sql
-- Find content for search term "LKE"
SELECT DISTINCT c.* FROM content c
WHERE
  -- Direct text match
  c.body_text ILIKE '%LKE%'

  -- OR soft-linked content
  OR c.id IN (
    SELECT content_id FROM content_mentions
    WHERE entity_type = 'term'
      AND resolved_entity_id = (SELECT id FROM glossary_terms WHERE term = 'LKE')
  )

  -- OR content mentioning linked product
  OR c.id IN (
    SELECT content_id FROM content_mentions
    WHERE entity_type = 'product'
      AND resolved_entity_id = (
        SELECT linked_entity_id FROM glossary_terms
        WHERE term = 'LKE' AND linked_entity_type = 'product'
      )
  )
```

---

## Audit & Tracing

### Design Principle

Every resolution process creates a complete **trace** - a record of inputs, decisions, reasoning, and outcomes at each stage. This enables:
- Walking through how a specific email/meeting was processed
- Debugging why a resolution was made
- Learning from corrections
- Comparing model performance

### Trace Levels

Storage can be significant. Support configurable detail levels:

| Level | Stores | Use Case |
|-------|--------|----------|
| **minimal** | Outcome only | Production, high volume |
| **standard** | Stages + decisions + reasoning | Default |
| **full** | + Complete LLM prompts/responses | Debugging, audits |
| **debug** | + Intermediate data structures | Development |

### Data Retention

| Data Type | Retention | Rationale |
|-----------|-----------|-----------|
| Full traces (prompts/responses) | 90 days | Debugging, model evaluation |
| Stage summaries | 90 days | Tied to traces |
| Decisions + reasoning | 1 year | Learning loop, pattern analysis |
| Corrections | 1 year | Training data, accuracy tracking |
| Comparison results | 1 year | Model performance history |

Automated cleanup via scheduled job purges expired data.

### Trace Structure

```
Content: Email #4521 "Q4 Planning Meeting Notes"
Trace ID: trace_abc123
Created: 2026-01-21T14:32:00Z

├── Stage 1: Understanding
│   ├── Input: [raw content, 847 chars]
│   ├── LLM Request: [prompt, model, tokens]
│   ├── LLM Response: [full response, latency: 1.2s]
│   ├── Output: 4 mentions extracted
│   └── Errors: none
│
├── Stage 2: Cross-Mention Reasoning
│   ├── Input: [4 mentions, project context]
│   ├── LLM Response: [reasoning about relationships]
│   └── Output: [mention link graph]
│
├── Stage 3: Entity Matching
│   ├── Input: [understanding + 12 candidate entities]
│   ├── LLM Response: [matches with confidence]
│   ├── Output: 3 resolved, 1 low-confidence
│   └── New entity suggested: "DataDog"
│
├── Stage 4: Verification
│   ├── Triggered: yes (1 mention < 0.9 confidence)
│   ├── LLM Response: [verification reasoning]
│   └── Output: confidence raised to 0.84
│
└── Final Outcome
    ├── Auto-resolved: 3
    ├── Queued for review: 1
    └── Processing time: 4.7s
```

### Trace Data Model

```sql
-- Top-level trace per content processed
CREATE TABLE resolution_traces (
    id TEXT PRIMARY KEY,              -- trace_abc123
    tenant_id TEXT NOT NULL,

    -- What was processed
    content_id BIGINT NOT NULL,
    content_type TEXT,                -- email, meeting, document
    content_summary TEXT,             -- "Q4 Planning Meeting Notes"

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,

    -- Outcome summary
    mentions_found INT,
    auto_resolved INT,
    queued_for_review INT,
    new_entities_suggested INT,

    -- Status
    status TEXT DEFAULT 'in_progress', -- in_progress, completed, failed
    error_message TEXT,

    -- Configuration at time of run
    model_used TEXT,                  -- mlx-mistral-7b, claude-sonnet
    trace_level TEXT,                 -- minimal, standard, full, debug
    config_snapshot JSONB,            -- thresholds, etc.

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_traces_content ON resolution_traces(content_id);
CREATE INDEX idx_traces_tenant_time ON resolution_traces(tenant_id, started_at DESC);

-- Each stage within a trace
CREATE TABLE resolution_trace_stages (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id),

    -- Stage identification
    stage_number INT NOT NULL,        -- 1, 2, 3, 4
    stage_name TEXT NOT NULL,         -- understanding, cross_mention, matching, verification

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,

    -- Input snapshot
    input_summary TEXT,               -- Human-readable: "4 mentions, 12 candidates"
    input_data JSONB,                 -- Full structured input

    -- Output snapshot
    output_summary TEXT,              -- Human-readable: "3 resolved, 1 uncertain"
    output_data JSONB,                -- Full structured output

    -- Status
    status TEXT DEFAULT 'in_progress',
    skipped BOOLEAN DEFAULT false,    -- Stage skipped (e.g., verification not needed)
    skip_reason TEXT,

    error_message TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_stages_trace ON resolution_trace_stages(trace_id, stage_number);

-- LLM call logs (full prompts/responses for full/debug trace levels)
CREATE TABLE resolution_llm_calls (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id),
    stage_id BIGINT REFERENCES resolution_trace_stages(id),

    -- Request
    model TEXT NOT NULL,              -- mlx-mistral-7b, claude-3-sonnet
    prompt_template TEXT,             -- Template name used
    prompt_text TEXT,                 -- Full prompt (can be large)
    prompt_tokens INT,

    -- Response
    response_text TEXT,               -- Full response
    response_tokens INT,

    -- Structured extraction from response
    parsed_output JSONB,              -- Structured data extracted
    parse_errors TEXT[],              -- Any issues parsing response

    -- Performance
    latency_ms INT,

    -- Retry/fallback tracking
    attempt_number INT DEFAULT 1,
    is_fallback BOOLEAN DEFAULT false,
    fallback_reason TEXT,             -- "local model failed, fell back to Claude"

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_llm_calls_trace ON resolution_llm_calls(trace_id);
CREATE INDEX idx_llm_calls_stage ON resolution_llm_calls(stage_id);

-- Individual decisions with reasoning
CREATE TABLE resolution_decisions (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL REFERENCES resolution_traces(id),
    stage_id BIGINT REFERENCES resolution_trace_stages(id),

    -- What decision was made
    decision_type TEXT NOT NULL,      -- resolve, queue_review, suggest_new_entity, skip_verification

    -- Context
    mention_id BIGINT,                -- Which mention this relates to
    mentioned_text TEXT,

    -- Decision details
    chosen_option TEXT,               -- What was decided
    alternatives JSONB,               -- Other options considered
    confidence DECIMAL(3,2),

    -- Reasoning
    reasoning TEXT,                   -- LLM's reasoning for this decision
    factors JSONB,                    -- Structured factors that influenced decision

    -- Outcome tracking (filled in later)
    was_correct BOOLEAN,              -- NULL until human reviews
    correction_notes TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_decisions_trace ON resolution_decisions(trace_id);
CREATE INDEX idx_decisions_mention ON resolution_decisions(mention_id);
CREATE INDEX idx_decisions_corrections ON resolution_decisions(trace_id)
    WHERE was_correct = false;
```

### Audit CLI

```bash
# List recent traces
penf audit traces list --content-type=meeting --limit=20

# Show trace summary
penf audit trace show trace_abc123

# Show specific stage detail
penf audit trace show trace_abc123 --stage=2

# Show all LLM calls for a trace
penf audit trace show trace_abc123 --llm-calls

# Show decisions and reasoning
penf audit trace show trace_abc123 --decisions

# Find traces where human corrected the decision
penf audit traces list --had-corrections

# Find traces for specific content
penf audit traces list --content-id=4521

# Export full trace for analysis
penf audit trace export trace_abc123 --format=json > trace.json
```

### Example: Walking Through a Trace

```bash
$ penf audit trace show trace_abc123

Trace: trace_abc123
Content: Email #4521 "Q4 Planning Meeting Notes"
Model: mlx-mistral-7b
Status: completed
Duration: 4.7s
Outcome: 3 auto-resolved, 1 queued for review

Stages:
  ✓ Stage 1: Understanding (1.2s)
    Found 4 mentions: "Alan", "LKE", "OKEE", "DataDog"

  ✓ Stage 2: Cross-Mention Reasoning (0.8s)
    Linked: OKEE→LKE (transcription), Alan→LKE (context)

  ✓ Stage 3: Entity Matching (2.1s)
    Resolved: LKE→term:15 (0.98), OKEE→term:15 (0.95), Alan→person:101 (0.88)
    Suggested new: DataDog (company)

  ✓ Stage 4: Verification (0.6s)
    Verified: Alan→Allen Duet (confidence held at 0.88)

$ penf audit trace show trace_abc123 --decisions

Decisions for trace_abc123:

#1 [Stage 3] RESOLVE "LKE" → term:15 "Linode Kubernetes Engine"
   Confidence: 0.98
   Reasoning: "Exact match to glossary term. Context discusses Kubernetes
              cluster management which aligns with LKE product."
   Alternatives: none considered (exact match)

#2 [Stage 3] RESOLVE "OKEE" → term:15 "LKE"
   Confidence: 0.95
   Reasoning: "Phonetic similarity to LKE (O-K-E-E vs L-K-E). Context is
              identical Kubernetes discussion. High likelihood of
              transcription error from audio."
   Alternatives: OKE (Oracle Kubernetes Engine) - rejected, no Oracle context

#3 [Stage 3] RESOLVE "Alan" → person:101 "Allen Duet"
   Confidence: 0.88
   Reasoning: "Allen Duet is LKE PM. Transcript discusses LKE technical work.
              'Alan' is common phonetic transcription of 'Allen'. No other
              'Alan' in MTC project team."
   Alternatives:
     - Alan Evans (person:203) - 0.25 - not on project, different domain
   ⚠️  Queued for review (confidence < 0.90)

#4 [Stage 3] SUGGEST_NEW_ENTITY "DataDog"
   Type: company
   Reasoning: "Mentioned as monitoring solution being evaluated. Not in
              company database. Appears to be the vendor Datadog Inc."
```

### Correction Tracking

When human corrects a resolution:

```bash
$ penf review questions resolve 72 --link-to person:205 \
    --correction-note "Alan Chen, new hire not in system yet"

# System updates:
# 1. Creates the resolution
# 2. Marks decision in trace as was_correct=false
# 3. Stores correction context for learning loop
```

Analysis of corrections:

```bash
$ penf audit corrections list --last-30-days

Corrections (last 30 days): 12

By pattern:
  - Name phonetic mismatches: 5
  - New entities not in DB: 4
  - Project context wrong: 2
  - Transcription overcorrection: 1

Most corrected entity type: person (8/12)
```

---

## Model Comparison

### Purpose

Run the same content through multiple models, then compare results side-by-side:
- Evaluate new models before deploying
- Debug why one model performs better
- Identify model weaknesses (fuzzy matching, phonetics)
- Optimize cost by knowing when to escalate

### Comparison Data Model

```sql
-- Comparison run record
CREATE TABLE resolution_comparisons (
    id TEXT PRIMARY KEY,              -- comp_xyz789
    tenant_id TEXT NOT NULL,

    -- What was compared
    content_id BIGINT NOT NULL,
    content_type TEXT,
    content_summary TEXT,

    -- Models compared
    models TEXT[] NOT NULL,           -- ['mlx-mistral-7b', 'mlx-llama-3', 'claude-sonnet']

    -- Linked traces (one per model)
    trace_ids TEXT[] NOT NULL,        -- ['trace_abc123', 'trace_def456', 'trace_ghi789']

    -- Comparison metadata
    initiated_by TEXT,                -- user, scheduled, ci
    purpose TEXT,                     -- model_evaluation, regression_test, debugging

    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,

    -- Summary stats
    total_decisions INT,
    unanimous_decisions INT,          -- All models agreed
    divergent_decisions INT,          -- Models disagreed

    -- Analysis
    divergence_summary JSONB,         -- Quick summary of where models differed

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_comparisons_content ON resolution_comparisons(content_id);
CREATE INDEX idx_comparisons_tenant ON resolution_comparisons(tenant_id, created_at DESC);

-- Per-mention comparison across models
CREATE TABLE resolution_comparison_decisions (
    id BIGSERIAL PRIMARY KEY,
    comparison_id TEXT NOT NULL REFERENCES resolution_comparisons(id),

    -- What mention
    mentioned_text TEXT NOT NULL,
    mention_index INT,                -- Position in content

    -- Decision by each model (JSONB array, one per model)
    model_decisions JSONB NOT NULL,
    -- Example:
    -- [
    --   {"model": "mistral", "entity_id": 101, "confidence": 0.88, "reasoning": "..."},
    --   {"model": "llama", "entity_id": 101, "confidence": 0.82, "reasoning": "..."},
    --   {"model": "claude", "entity_id": 101, "confidence": 0.94, "reasoning": "..."}
    -- ]

    -- Comparison analysis
    is_unanimous BOOLEAN,             -- All models chose same entity
    divergence_type TEXT,             -- NULL, 'different_entity', 'confidence_gap', 'new_vs_existing'
    confidence_spread DECIMAL(3,2),   -- Max confidence - min confidence

    -- If there's a known correct answer
    ground_truth_entity_id BIGINT,
    models_correct TEXT[],            -- Which models got it right

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_comp_decisions_comparison ON resolution_comparison_decisions(comparison_id);
CREATE INDEX idx_comp_decisions_divergent ON resolution_comparison_decisions(comparison_id)
    WHERE is_unanimous = false;
```

### Comparison CLI

```bash
# Run comparison on single content
penf audit compare --content-id=4521 \
    --models=mlx-mistral-7b,mlx-llama-3,claude-sonnet

# Run comparison on multiple contents (batch evaluation)
penf audit compare --content-ids=4521,4522,4523 \
    --models=mlx-mistral-7b,claude-sonnet

# List comparisons
penf audit comparisons list --last-7-days

# Show comparison summary
penf audit comparison show comp_xyz789

# Show divergences only
penf audit comparison show comp_xyz789 --divergences

# Deep dive into specific decision across models
penf audit comparison show comp_xyz789 --mention="Alan" --reasoning

# Side-by-side stage comparison
penf audit comparison show comp_xyz789 --stage=2 --side-by-side

# Export for analysis
penf audit comparison export comp_xyz789 --format=json
```

### Example: Comparison Walkthrough

```bash
$ penf audit compare --content-id=4521 \
    --models=mlx-mistral-7b,mlx-llama-3,claude-sonnet

Running comparison comp_xyz789...
  ✓ mlx-mistral-7b: trace_abc123 (4.7s)
  ✓ mlx-llama-3: trace_def456 (3.2s)
  ✓ claude-sonnet: trace_ghi789 (1.8s)

Comparison complete.
  Decisions: 4
  Unanimous: 3 (75%)
  Divergent: 1 (25%)

$ penf audit comparison show comp_xyz789

Comparison: comp_xyz789
Content: Email #4521 "Q4 Planning Meeting Notes"
Models: mlx-mistral-7b, mlx-llama-3, claude-sonnet

Decision Summary:
┌──────────┬─────────────────┬─────────────────┬─────────────────┬───────────┐
│ Mention  │ mistral-7b      │ llama-3         │ claude-sonnet   │ Agreement │
├──────────┼─────────────────┼─────────────────┼─────────────────┼───────────┤
│ "LKE"    │ term:15 (0.98)  │ term:15 (0.96)  │ term:15 (0.99)  │ ✓ SAME    │
│ "OKEE"   │ term:15 (0.95)  │ term:15 (0.91)  │ term:15 (0.97)  │ ✓ SAME    │
│ "Alan"   │ person:101(0.88)│ person:101(0.82)│ person:101(0.94)│ ✓ SAME    │
│ "DataDog"│ NEW (0.85)      │ NEW (0.80)      │ company:42(0.91)│ ✗ DIFF    │
└──────────┴─────────────────┴─────────────────┴─────────────────┴───────────┘
```

### Reasoning Comparison

```bash
$ penf audit comparison show comp_xyz789 --mention="DataDog" --reasoning

Mention: "DataDog"
Divergence: new_entity vs existing_match

┌─ mlx-mistral-7b ──────────────────────────────────────────────────────────┐
│ Decision: SUGGEST_NEW_ENTITY                                              │
│ Confidence: 0.85                                                          │
│ Reasoning: "DataDog mentioned as monitoring tool. Not found in company    │
│            database. Recommend creating new company entity."              │
│                                                                           │
│ Search performed: "datadog" in companies table → 0 results                │
└───────────────────────────────────────────────────────────────────────────┘

┌─ mlx-llama-3 ─────────────────────────────────────────────────────────────┐
│ Decision: SUGGEST_NEW_ENTITY                                              │
│ Confidence: 0.80                                                          │
│ Reasoning: "DataDog is a monitoring SaaS company. Not present in known    │
│            companies. Should be added as vendor."                         │
│                                                                           │
│ Search performed: "datadog" in companies table → 0 results                │
└───────────────────────────────────────────────────────────────────────────┘

┌─ claude-sonnet ───────────────────────────────────────────────────────────┐
│ Decision: RESOLVE → company:42 "Datadog Inc"                              │
│ Confidence: 0.91                                                          │
│ Reasoning: "DataDog is a variant spelling of Datadog, the observability   │
│            platform company. Found existing entry 'Datadog Inc' in        │
│            companies table. High confidence this is the same entity       │
│            despite capitalization difference."                            │
│                                                                           │
│ Search performed: fuzzy "datadog" in companies → 1 result (Datadog Inc)   │
└───────────────────────────────────────────────────────────────────────────┘

Analysis: Claude performed fuzzy matching and found existing entity.
          Local models did exact match only and missed it.

Implication: Local models may need fuzzy search in entity lookup stage.
```

### Aggregate Model Statistics

```bash
$ penf audit models stats --last-30-days

Model Performance (30 days, 156 comparisons, 847 decisions):

┌─────────────────┬──────────┬───────────┬──────────┬─────────────┐
│ Model           │ Accuracy │ Avg Conf  │ Avg Time │ Cost        │
├─────────────────┼──────────┼───────────┼──────────┼─────────────┤
│ claude-sonnet   │ 94.2%    │ 0.91      │ 1.4s     │ $0.012/run  │
│ mlx-mistral-7b  │ 87.3%    │ 0.84      │ 4.2s     │ $0 (local)  │
│ mlx-llama-3     │ 85.1%    │ 0.81      │ 3.1s     │ $0 (local)  │
└─────────────────┴──────────┴───────────┴──────────┴─────────────┘

Where models diverge most:
  1. Fuzzy entity matching (DataDog vs Datadog): claude +15% accuracy
  2. Phonetic name variants (Alan/Allen): claude +8% accuracy
  3. Transcription error detection: all similar

Recommendation: Use local model for clear matches, escalate ambiguous
                (confidence < 0.85) to Claude.
```

---

## Functional Requirements

### Unified Mention Extraction

- **FR-700**: System MUST extract mentions of persons, terms, products, companies, projects from content
- **FR-701**: System MUST store all mentions in unified `content_mentions` table
- **FR-702**: System MUST track position and context snippet for each mention
- **FR-703**: System MUST support multiple entity type resolutions from same content

### LLM-Driven Resolution

- **FR-710**: System MUST use LLM for resolution decisions, not rule-based code
- **FR-711**: System MUST implement multi-stage resolution (understanding, cross-mention, matching, verification)
- **FR-712**: System MUST process mentions in batches by content for cross-mention reasoning
- **FR-713**: System MUST support configurable LLM provider (MLX local, Claude API)
- **FR-714**: System MUST provide LLM with candidate entities gathered by code
- **FR-715**: System MUST capture LLM reasoning for each resolution decision
- **FR-716**: System MUST support LLM suggesting new entities when no match found
- **FR-717**: System MUST run verification stage for confidence < 0.9

### Context Gathering (Code)

- **FR-720**: Code MUST gather candidate entities via fuzzy name search
- **FR-721**: Code MUST provide glossary/alias lookups to LLM
- **FR-722**: Code MUST provide project member lists to LLM
- **FR-723**: Code MUST provide prior link history to LLM
- **FR-724**: Code MUST provide entity-project affinity scores to LLM

### Soft vs Permanent Links

- **FR-730**: System MUST support soft links (content-scoped resolution)
- **FR-731**: System MUST support permanent links (global alias/entity)
- **FR-732**: System MUST support project-scoped patterns
- **FR-733**: System MUST track pattern usage for suggestions
- **FR-734**: System MUST support promoting soft patterns to permanent

### Term-Entity Linking

- **FR-740**: System MUST support linking glossary terms to canonical entities
- **FR-741**: System MUST support linking terms to products, projects, or companies
- **FR-742**: System MUST surface linked entity in term resolution results
- **FR-743**: System MUST include linked entity in search results for term

### Audit & Tracing

- **FR-750**: System MUST create a trace for every resolution process
- **FR-751**: System MUST record inputs, outputs, and timing for each stage
- **FR-752**: System MUST capture LLM prompts and responses (at full/debug trace level)
- **FR-753**: System MUST record reasoning for each decision
- **FR-754**: System MUST support configurable trace levels (minimal, standard, full, debug)
- **FR-755**: System MUST track corrections and link to original decisions
- **FR-756**: System MUST provide CLI for trace inspection and export

### Model Comparison

- **FR-760**: System MUST support running same content through multiple models
- **FR-761**: System MUST create linked traces for each model in comparison
- **FR-762**: System MUST identify unanimous vs divergent decisions
- **FR-763**: System MUST capture reasoning differences between models
- **FR-764**: System MUST track ground truth when available for accuracy calculation
- **FR-765**: System MUST provide aggregate model statistics over time
- **FR-766**: System MUST provide CLI for comparison and analysis

### Learning Loop

- **FR-770**: System MUST record human corrections with context
- **FR-771**: System MUST update mention patterns from corrections
- **FR-772**: System MUST support periodic cluster analysis of unresolved mentions
- **FR-773**: System MUST feed correction patterns to future resolution context

### Search Integration

- **FR-780**: System MUST include soft-linked content in search results
- **FR-781**: System MUST expand terms using glossary in queries
- **FR-782**: System MUST surface linked entity context in search results

---

## Success Criteria

- **SC-001**: 80% of mentions auto-resolved without user intervention (confidence >= 0.8)
- **SC-002**: LLM cross-mention reasoning improves accuracy by 10%+ vs independent resolution
- **SC-003**: 100% of resolution processes have audit traces
- **SC-004**: Any resolution can be walked through stage-by-stage via CLI
- **SC-005**: Model comparisons identify accuracy differences within 5% margin
- **SC-006**: Corrections feed back to improve future resolution accuracy
- **SC-007**: Search finds soft-linked content (OKEE → LKE finds content)
- **SC-008**: Term-entity links surface product/project context in search
