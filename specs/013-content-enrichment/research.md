# Research: Unified Mention Resolution System

**Date**: 2026-01-21 | **Branch**: `013-content-enrichment`

## Research Questions

### 1. MLX Local LLM Integration Patterns for Go

**Decision**: Use HTTP client to MLX sidecar with Ollama-compatible API

**Rationale**:
- Existing `pkg/embeddings/MLXClient` already uses this pattern successfully
- MLX sidecar on dev01 exposes Ollama-compatible endpoints at port 8081
- HTTP-based integration is simpler than gRPC for local development
- Supports both `/api/embed` (embeddings) and `/api/generate` (text generation)

**Alternatives Considered**:
- Direct Go bindings to MLX: Rejected - MLX is Python/C++, no native Go support
- gRPC: Rejected - Ollama API is more widely used, better documentation
- External service (Ollama proper): Rejected - MLX already running locally, no need for separate process

**Implementation Pattern**:
```go
type MLXCompletionClient struct {
    config     *Config
    httpClient *http.Client
    logger     logging.Logger
}

type CompletionRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
    Format string `json:"format,omitempty"` // "json" for structured output
}

type CompletionResponse struct {
    Response string `json:"response"`
    Done     bool   `json:"done"`
}
```

### 2. Structured Output Parsing from LLM Responses

**Decision**: JSON mode with schema validation + retry on parse failure

**Rationale**:
- Ollama-compatible API supports `"format": "json"` for JSON output mode
- Local models (Mistral, Llama) support JSON mode reliably
- Schema validation catches malformed responses before processing
- Retry with reprompt on parse failure (up to 2 retries per clarification)

**Alternatives Considered**:
- XML output: Rejected - JSON is native to most models, better parsing
- Free-form text parsing: Rejected - unreliable, requires complex regex
- Function calling: Rejected - not consistently supported across local models

**Implementation Pattern**:
```go
type StructuredResponse[T any] struct {
    Data       T
    ParseError error
    Retries    int
}

func (c *MLXCompletionClient) CompleteStructured[T any](
    ctx context.Context,
    prompt string,
    schema T, // Zero value as schema hint
) (*StructuredResponse[T], error) {
    // 1. Request with format: "json"
    // 2. Parse response into T
    // 3. Validate against expected schema
    // 4. Retry with clarification if parse fails
}
```

**Validation Strategy**:
- Use Go's json.Unmarshal for basic parsing
- Custom validation for required fields (reasoning, confidence)
- Reject if confidence not in [0.0, 1.0] range
- Reject if decision not in allowed enum values

### 3. Batch Processing Patterns for LLM Calls

**Decision**: Content-based batching with configurable batch size

**Rationale**:
- All mentions from same content processed together (enables cross-mention reasoning)
- Single LLM call per stage per content batch (4 stages × 1 call = 4 calls max)
- Configurable max mentions per batch (default: 50)
- Larger content split into multiple batches if needed

**Alternatives Considered**:
- Per-mention processing: Rejected - loses cross-mention context, more LLM calls
- Global batching (across content): Rejected - loses content-specific context
- Streaming: Rejected - batch response more reliable for structured output

**Implementation Pattern**:
```go
type ResolutionBatch struct {
    ContentID   int64
    ContentType string
    Mentions    []ExtractedMention
    ProjectID   *int64
    BatchIndex  int  // For split batches
    TotalBatches int
}

func (r *Resolver) ProcessContent(ctx context.Context, contentID int64) error {
    mentions := r.extractMentions(ctx, contentID)

    // Split into batches if needed
    batches := r.splitIntoBatches(mentions, r.config.MaxMentionsPerBatch)

    for _, batch := range batches {
        if err := r.processBatch(ctx, batch); err != nil {
            return err
        }
    }
    return nil
}
```

**Batch Size Tuning**:
- Default: 50 mentions per batch
- Context window limit: ~4k tokens for prompt + ~2k for response
- Average mention + context: ~100 tokens
- Safety margin: 50 mentions × 100 tokens = 5k tokens (within limits)

### 4. Trace Storage Optimization (JSONB vs Normalized)

**Decision**: Hybrid approach - normalized for queries, JSONB for payload

**Rationale**:
- Trace metadata (id, content_id, status, timing) in normalized columns for fast queries
- Stage inputs/outputs stored as JSONB for flexibility
- LLM prompts/responses stored as TEXT (large, rarely queried)
- Decisions normalized for correction tracking and learning loop

**Alternatives Considered**:
- Fully normalized: Rejected - too many tables, schema changes for each LLM output change
- Fully JSONB: Rejected - poor query performance for filtering, reporting
- Document store (MongoDB): Rejected - PostgreSQL already in use, no need for separate DB

**Implementation Pattern**:
```sql
-- Normalized for queries
CREATE TABLE resolution_traces (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    content_id BIGINT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms INT,
    -- Summary stats (denormalized for fast dashboard queries)
    mentions_found INT,
    auto_resolved INT,
    queued_for_review INT
);

-- JSONB for flexible payload
CREATE TABLE resolution_trace_stages (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT REFERENCES resolution_traces(id),
    stage_number INT NOT NULL,
    stage_name TEXT NOT NULL,
    -- Flexible payloads
    input_data JSONB,    -- Stage-specific input
    output_data JSONB,   -- Stage-specific output
    -- Normalized for queries
    duration_ms INT,
    status TEXT
);

-- TEXT for large content (not JSONB - no need to query inside)
CREATE TABLE resolution_llm_calls (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT REFERENCES resolution_traces(id),
    prompt_text TEXT,     -- Large, rarely queried
    response_text TEXT,   -- Large, rarely queried
    parsed_output JSONB,  -- Structured, sometimes queried
    latency_ms INT
);
```

**Retention Implementation**:
```sql
-- Automated cleanup job (run daily)
DELETE FROM resolution_llm_calls
WHERE created_at < NOW() - INTERVAL '90 days';

DELETE FROM resolution_trace_stages
WHERE trace_id IN (
    SELECT id FROM resolution_traces
    WHERE created_at < NOW() - INTERVAL '90 days'
);

DELETE FROM resolution_traces
WHERE created_at < NOW() - INTERVAL '90 days';

-- Decisions kept for 1 year
DELETE FROM resolution_decisions
WHERE created_at < NOW() - INTERVAL '1 year';
```

---

## Additional Findings

### MLX Sidecar Status

The MLX sidecar on dev01 is already running and serving:
- **Embeddings**: `http://localhost:8081/api/embed` (used by `pkg/embeddings/`)
- **Completions**: `http://localhost:8081/api/generate` (to be used for resolution)

Current model: `mistral-7b-instruct-v0.2` (suitable for structured output tasks)

### Prompt Engineering for Multi-Stage Resolution

**Stage 1 (Understanding) prompt structure**:
```
You are analyzing content to identify entity mentions.

Content:
{content_text}

Content metadata:
- Type: {email|meeting|document}
- Date: {date}
- Participants: {list}

Identify all mentions of: persons, terms/acronyms, products, companies, projects.

For each mention, provide:
- text: The exact text mentioned
- entity_type: person|term|product|company|project
- position: Character offset
- context_snippet: Surrounding text (50 chars each side)
- understanding: What you understand about this mention
- transcription_flags: {likely_error, phonetic_variants, probable_correction}

Output as JSON array.
```

**Stage 3 (Matching) prompt structure**:
```
You are matching entity mentions to database candidates.

Understanding from previous stages:
{stage_1_output}
{stage_2_relationships}

Candidates from database:
{candidates_json}

For each mention, provide a resolution decision:
- mention_text: The mentioned text
- decision: resolve|queue_review|suggest_new_entity
- resolved_to: {entity_type, entity_id, entity_name} (if resolve)
- confidence: 0.0-1.0
- reasoning: Why this decision (required)
- factors: Structured factors that influenced decision
- alternatives_considered: Other options evaluated

Output as JSON.
```

---

## Summary

All research questions resolved. Key decisions:

| Question | Decision |
|----------|----------|
| MLX Integration | HTTP client to Ollama-compatible API |
| Structured Output | JSON mode + schema validation + retry |
| Batch Processing | Content-based batching, max 50 mentions |
| Trace Storage | Hybrid normalized + JSONB |

No blockers identified. Ready to proceed to Phase 1 (data-model.md, contracts/).
