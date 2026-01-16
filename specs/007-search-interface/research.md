# Research: Search and Query Interface

**Feature**: 007-search-interface
**Date**: 2026-01-15
**Status**: Complete

## Research Questions

1. How should hybrid search (full-text + semantic) be combined for optimal relevance?
2. What temporal query parsing approach best supports natural language date expressions?
3. How should cross-content relationships be discovered and ranked?
4. What caching strategy optimizes search performance at 100k content scale?
5. How should search ranking combine multiple relevance signals?

---

## 1. Hybrid Search Strategy

### Decision: Reciprocal Rank Fusion (RRF) for combining full-text and vector search

### Rationale

RRF provides a simple, effective method for combining ranked lists from different retrieval methods without requiring score normalization. Given that full-text search scores (BM25-based) and vector similarity scores (cosine/L2 distance) are on different scales, RRF avoids the complexity of score calibration.

**Formula**: `RRF(d) = sum(1 / (k + rank_i(d)))` where k=60 (standard constant)

### Alternatives Considered

| Approach | Pros | Cons | Rejected Because |
|----------|------|------|------------------|
| Linear combination of normalized scores | Simple math | Requires careful score normalization, sensitive to calibration | Score distributions vary significantly between full-text and vector |
| Learning-to-rank (LTR) | Can optimize for specific relevance metrics | Requires training data, complexity | No labeled training data available, YAGNI |
| Query-time re-ranking | Can use ML models for final ranking | Adds latency, complexity | Sub-15s requirement achievable with simpler approach |
| Full-text only | Simple, well-understood | Misses semantic matches | Core requirement is natural language queries without exact keywords |
| Vector only | Good semantic understanding | Misses keyword matches, expensive at scale | Need both capabilities per FR-002, FR-012 |

### Implementation Approach

```python
def hybrid_search(query: str, query_vector: List[float], limit: int = 25) -> List[SearchResult]:
    # Execute both searches in parallel
    full_text_results = await full_text_search(query, limit=limit * 2)
    vector_results = await vector_similarity_search(query_vector, limit=limit * 2)

    # Apply RRF fusion
    rrf_scores = {}
    k = 60  # Standard RRF constant

    for rank, result in enumerate(full_text_results, 1):
        rrf_scores[result.id] = rrf_scores.get(result.id, 0) + 1 / (k + rank)

    for rank, result in enumerate(vector_results, 1):
        rrf_scores[result.id] = rrf_scores.get(result.id, 0) + 1 / (k + rank)

    # Sort by RRF score and return top results
    return sorted(results, key=lambda r: rrf_scores[r.id], reverse=True)[:limit]
```

### References

- [Reciprocal Rank Fusion Paper](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf) - Original RRF research
- [Hybrid Search in pgvector](https://github.com/pgvector/pgvector#hybrid-search) - PostgreSQL implementation patterns

---

## 2. Temporal Query Parsing

### Decision: Dateparser library with custom relative expression handling

### Rationale

The `dateparser` library provides robust parsing of natural language date expressions across multiple languages and formats. Combined with custom handling for Penfold-specific temporal expressions ("since last meeting", "around the deployment"), it provides the flexibility needed for contextual time machine queries.

### Alternatives Considered

| Approach | Pros | Cons | Rejected Because |
|----------|------|------|------------------|
| Regular expressions only | No dependencies | Limited flexibility, maintenance burden | Too many edge cases in natural language dates |
| spaCy NER for dates | ML-based, handles context | Heavyweight dependency, overkill | dateparser sufficient for date parsing |
| Custom temporal DSL | Full control | Requires documentation, learning curve | Users expect natural language, not DSL |
| Elasticsearch date math | Standard format | Requires ES, not PostgreSQL | Already using PostgreSQL stack |

### Implementation Approach

```python
from dateparser import parse as date_parse
from dateparser.search import search_dates

class TemporalQueryParser:
    RELATIVE_PATTERNS = {
        r"last\s+week": lambda: (now - timedelta(weeks=1), now),
        r"since\s+(january|february|...)": lambda m: (parse_month(m), now),
        r"around\s+(.+)": lambda m: expand_fuzzy_date(m, days=3),
        r"before\s+(.+)": lambda m: (None, date_parse(m)),
        r"after\s+(.+)": lambda m: (date_parse(m), None),
    }

    def extract_temporal_constraints(self, query: str) -> TemporalConstraint:
        # First try dateparser's search
        dates = search_dates(query)

        # Then apply relative patterns
        for pattern, handler in self.RELATIVE_PATTERNS.items():
            if match := re.search(pattern, query, re.I):
                return TemporalConstraint(*handler(match))

        return TemporalConstraint(start=None, end=None)  # No temporal constraint
```

### Temporal Query Types Supported

1. **Absolute dates**: "December 15, 2025", "2025-12-15"
2. **Relative expressions**: "last week", "yesterday", "3 days ago"
3. **Named periods**: "since Christmas", "before the new year"
4. **Fuzzy periods**: "around the deployment" (expanded with contextual events)
5. **Range expressions**: "between January and March", "from Monday to Friday"

### References

- [dateparser documentation](https://dateparser.readthedocs.io/)
- [Python temporal expressions](https://github.com/scrapinghub/dateparser)

---

## 3. Cross-Content Correlation Discovery

### Decision: Entity-based correlation with participant overlap weighting

### Rationale

Relationships between content items are discovered through shared entities (people, projects, topics) identified by the AI coordination layer. Participant overlap is particularly strong signal for related discussions across email and meetings.

### Correlation Signals

| Signal | Weight | Description |
|--------|--------|-------------|
| Shared participants | 0.30 | Same people involved in different content |
| Shared project references | 0.25 | Same project mentioned across content |
| Temporal proximity | 0.20 | Content created within same time window |
| Semantic similarity | 0.15 | Vector distance between content embeddings |
| Thread/reply chains | 0.10 | Explicit conversation threads |

### Alternatives Considered

| Approach | Pros | Cons | Rejected Because |
|----------|------|------|------------------|
| Pure semantic similarity | Simple implementation | Misses structural relationships | Email threads and meeting follow-ups need explicit linking |
| Graph-based (Neo4j) | Rich relationship queries | Additional infrastructure | YAGNI - PostgreSQL sufficient for current scale |
| Topic modeling (LDA) | Discovers latent topics | Batch processing required, stale | Need real-time correlation |

### Implementation Approach

```python
class CorrelationDiscovery:
    SIGNAL_WEIGHTS = {
        "participants": 0.30,
        "project": 0.25,
        "temporal": 0.20,
        "semantic": 0.15,
        "thread": 0.10,
    }

    async def find_related_content(
        self,
        content_id: int,
        limit: int = 10,
    ) -> List[RelatedContent]:
        content = await self.get_content(content_id)

        # Compute each signal
        participant_matches = await self.find_by_participants(content.participants)
        project_matches = await self.find_by_project(content.project_refs)
        temporal_matches = await self.find_by_temporal_proximity(content.timestamp)
        semantic_matches = await self.find_by_similarity(content.embedding)
        thread_matches = await self.find_by_thread(content.thread_id)

        # Combine with weighted scoring
        combined = self.merge_and_score(
            participant_matches,
            project_matches,
            temporal_matches,
            semantic_matches,
            thread_matches,
        )

        return combined[:limit]
```

---

## 4. Caching Strategy

### Decision: Two-tier caching with Redis for queries and in-memory LRU for embeddings

### Rationale

Search queries benefit from Redis caching for persistence across restarts and sharing across processes. Frequently-accessed embeddings benefit from in-memory LRU cache for sub-millisecond access during search.

### Cache Architecture

```
User Query
    │
    ▼
┌─────────────────────┐
│  Query Hash Cache   │  Redis: full query -> results
│  TTL: 5 minutes     │  Key: sha256(normalized_query)
└─────────────────────┘
    │ miss
    ▼
┌─────────────────────┐
│  Embedding LRU      │  In-memory: entity_id -> embedding
│  Size: 10,000       │  Used during similarity search
└─────────────────────┘
    │
    ▼
┌─────────────────────┐
│  PostgreSQL         │  Full-text index + pgvector HNSW
└─────────────────────┘
```

### Alternatives Considered

| Approach | Pros | Cons | Rejected Because |
|----------|------|------|------------------|
| No caching | Simplest | Repeated queries expensive | 50 concurrent queries requirement |
| In-memory only | Fast | Lost on restart, not shared | Need persistence for common queries |
| Elasticsearch | Built-in caching | Additional infrastructure | Already have Redis in stack |
| Full result caching | Fast response | Stale results | Need fresh results when index updates |

### Cache Invalidation Strategy

1. **Query cache**: TTL-based (5 minutes), invalidated on index updates
2. **Embedding cache**: LRU eviction, refreshed on embedding model updates
3. **Result metadata**: Cached separately, updated on content changes

---

## 5. Search Ranking

### Decision: Multi-signal ranking with configurable weights

### Rationale

Search ranking combines multiple signals to produce relevance scores. The weighting is configurable to allow tuning based on user feedback and search analytics.

### Ranking Signals

| Signal | Default Weight | Description |
|--------|----------------|-------------|
| Hybrid RRF score | 0.40 | Combined full-text + semantic relevance |
| Recency boost | 0.20 | Newer content scored higher (30-day decay) |
| Participant relevance | 0.15 | Content from frequently-contacted people |
| Content type match | 0.10 | User's preferred content types |
| AI confidence | 0.10 | Confidence in AI-extracted assertions |
| Search popularity | 0.05 | Content frequently returned in searches |

### Implementation Approach

```python
class SearchRanker:
    DEFAULT_WEIGHTS = {
        "rrf_score": 0.40,
        "recency": 0.20,
        "participant": 0.15,
        "content_type": 0.10,
        "confidence": 0.10,
        "popularity": 0.05,
    }

    def rank_results(
        self,
        results: List[SearchResult],
        user_context: UserContext,
        weights: Optional[Dict[str, float]] = None,
    ) -> List[RankedResult]:
        weights = weights or self.DEFAULT_WEIGHTS

        ranked = []
        for result in results:
            score = (
                weights["rrf_score"] * result.rrf_score +
                weights["recency"] * self.recency_score(result.timestamp) +
                weights["participant"] * self.participant_score(result, user_context) +
                weights["content_type"] * self.content_type_score(result, user_context) +
                weights["confidence"] * result.ai_confidence +
                weights["popularity"] * self.popularity_score(result)
            )
            ranked.append(RankedResult(result=result, score=score))

        return sorted(ranked, key=lambda r: r.score, reverse=True)
```

### Alternatives Considered

| Approach | Pros | Cons | Rejected Because |
|----------|------|------|------------------|
| Single score (BM25 only) | Simple, well-understood | Ignores semantic and context | Natural language queries need semantic understanding |
| ML-based ranking | Optimized for relevance | Requires training data | No labeled data available initially |
| User-defined ranking | Maximum flexibility | Complexity for users | Power user feature, not core |

---

## 6. Query Embedding Generation

### Decision: Use nomic-embed-text model via Ollama for query embedding

### Rationale

The existing embedding infrastructure uses 768-dimensional vectors with nomic-embed-text. Query embeddings must match the same model and dimensions for valid similarity comparisons.

### Implementation

```python
async def generate_query_embedding(query: str) -> List[float]:
    """Generate embedding for search query using same model as content."""
    # Use existing Ollama integration
    response = await ollama.embed(
        model="nomic-embed-text",
        input=query,
    )
    return response.embeddings[0]  # 768-dimensional vector
```

### Query Preprocessing

1. Lowercase normalization
2. Remove stop words for better semantic matching
3. Expand common abbreviations
4. Preserve technical terms and proper nouns

---

## Summary of Key Decisions

| Area | Decision | Key Rationale |
|------|----------|---------------|
| Hybrid Search | RRF fusion | Simple, effective, no score calibration needed |
| Temporal Parsing | dateparser + custom patterns | Robust NLP with Penfold-specific extensions |
| Correlations | Entity-based with weighted signals | Leverages AI-extracted relationships |
| Caching | Redis + LRU | Performance at scale with persistence |
| Ranking | Multi-signal weighted | Configurable, multiple relevance factors |
| Query Embeddings | nomic-embed-text via Ollama | Consistent with indexed content |

---

## Next Steps

1. **Phase 1**: Design data models for search queries, results, sessions, and filters
2. **Phase 1**: Create API contracts for search operations
3. **Phase 1**: Document quickstart for search CLI usage
