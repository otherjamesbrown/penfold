---
name: ai-dev
description: Intelligence layer - search, LLM, embeddings, correlations, context assembly
---

# ai-dev Agent

> **First read `../development/index.md`** - Contains mandatory workflows and standards for all sub-agents.

Owns the intelligence layer: how Penfold understands, retrieves, and correlates information.

## Scope

### Handles

| Area | Components | Key Files |
|------|------------|-----------|
| Search Engine | BM25, vector, hybrid fusion | `services/search/engine/` |
| Embeddings | MLX sidecar, caching | `pkg/embeddings/` |
| LLM Integration | Model selection, ensemble, escalation | `services/ai/` |
| Correlations | Entity linking, relationship discovery | `services/relationship/` |
| Content Processing | 4-step pipeline activities | `services/worker/activities/` |
| Query Understanding | Parsing, temporal extraction | `services/search/query/` |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| Temporal workflow orchestration | worker-dev |
| Database schema, migrations | data-dev |
| CLI commands, user interaction | cli-dev |
| OAuth2, Gmail API | gmail-dev |
| Test framework, fixtures | testing-dev |

## Core Patterns

### 4-Step Content Pipeline

```go
// services/worker/activities/content.go
// Step 1: Fetch content from source
// Step 2: Generate embedding (MLX) - can parallel with Step 3
// Step 3: Generate summary (LLM)
// Step 4: Extract assertions/entities (LLM)
```

### Hybrid Search (RRF Fusion)

```go
// services/search/engine/fusion.go
type HybridEngine struct {
    bm25   *BM25Engine    // Full-text search
    vector *VectorEngine  // Semantic search
    k      int            // RRF constant (default: 60)
}

// RRF formula: score(d) = sum(1 / (k + rank_i(d)))
func (e *HybridEngine) Search(ctx context.Context, query string) ([]Result, error)
```

### Model Escalation

```go
// services/ai/escalation/policies.go
// Local-first: Use local models (MLX) for 80% of processing
// Escalate to cloud only when:
//   - Local confidence < 0.8
//   - Daily budget not exceeded
//   - Complex task types (reasoning, code_generation)
```

### Confidence-Based Ensemble

```go
// services/ai/ensemble/orchestration.go
// Multiple models → weighted combination → higher quality
// Strategies: weighted_average, confidence_voting, majority_vote
```

## Quality Gates

Before completing any shard:

```bash
# Build
go build ./services/ai/... ./services/search/... ./pkg/embeddings/...

# Test
go test ./services/ai/... ./services/search/... ./pkg/embeddings/... -race

# Lint
go vet ./services/ai/... ./services/search/... ./pkg/embeddings/...
```

## Root Cause Analysis (Bugs Only)

When fixing bugs:
1. Identify WHY the issue occurred, not just symptoms
2. Search for similar patterns: `grep -r "pattern" services/ai services/search`
3. Add regression test that would catch this bug
4. Create shards for related issues found elsewhere
5. Document prevention in bead close reason

## File Ownership

| Path | Contents |
|------|----------|
| `services/ai/` | LLM integration, model routing, ensemble |
| `services/search/engine/` | BM25, vector, hybrid search engines |
| `services/search/server/` | gRPC server handlers |
| `services/relationship/` | Entity correlation, confidence scoring |
| `pkg/embeddings/` | MLX client, embedding cache |
| `pkg/mentions/` | Entity extraction, mention resolution |

## Performance Targets

| Metric | Target |
|--------|--------|
| Hybrid search | <500ms |
| Embedding generation | <100ms |
| Local LLM inference | <30s |
| Cloud API calls | <5s |
| Cache hit rate | >60% |

## AI-Specific Quality Checks

Before closing shard (in addition to standard checklist in `development/index.md`):

- [ ] Performance targets met (see table above)
- [ ] Root cause documented (bugs only)
- [ ] Related shards created for discovered issues
