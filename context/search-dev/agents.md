# Search Interface Development Agent Context

This context enables AI agents to work effectively with Penfold's search and query interface, implementing hybrid search, result ranking, correlation discovery, and search analytics.

## Agent Expertise

**Primary Skills**: Hybrid search, RRF fusion, query parsing, semantic search, correlation discovery, search analytics

**Key Responsibilities**:
- Query parsing and temporal constraint extraction
- Hybrid search execution (full-text + vector)
- Result ranking with multi-signal scoring
- Cross-content correlation discovery
- Search session management
- Query caching and performance optimization
- Analytics collection and trending analysis

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Models | `penf_lib/search/models.py` | Pydantic DTOs and enums |
| Search Engine | `penf_lib/search/search_engine.py` | Main orchestration |
| Query Parser | `penf_lib/search/query_parser.py` | NLP query processing |
| Ranking | `penf_lib/search/ranking.py` | RRF fusion and scoring |
| Correlations | `penf_lib/search/correlations.py` | Relationship discovery |
| Cache | `penf_lib/search/cache.py` | Query and embedding cache |
| Filters | `penf_lib/search/filters.py` | Result filtering pipeline |
| Sessions | `penf_lib/search/session.py` | Session management |
| Suggestions | `penf_lib/search/suggestions.py` | Query suggestions |
| Analytics | `penf_lib/search/analytics.py` | Search metrics |
| Repository | `penf_lib/storage/repositories/search.py` | Database operations |
| CLI | `penf_lib/cli/search_commands.py` | User interface |

## Architectural Patterns (Production-Proven)

### Reciprocal Rank Fusion (RRF)

**Pattern**: Combine ranked lists without score normalization

```python
from penf_lib.search import RRFFusion

# RRF formula: score(d) = sum(1 / (k + rank_i(d)))
rrf = RRFFusion(k=60)  # k=60 is the standard value

# Fuse results from multiple retrieval methods
fused_results = rrf.fuse(
    full_text_results,   # [(entity_id, score), ...]
    semantic_results,    # [(entity_id, score), ...]
    metadata_results,    # Optional: additional signals
)

# Documents appearing high in multiple lists score highest
```

**Key Points**:
- k=60 provides balanced weighting between high and low ranks
- No score normalization required (unlike linear combination)
- Naturally handles missing documents (only contributes from lists where present)

### Multi-Signal Ranking

**Pattern**: Combine relevance with personalization and recency

```python
from penf_lib.search import SearchRanker, RankedResult

ranker = SearchRanker(
    personalization_weight=0.1,  # Boost user preferences
    recency_weight=0.1,          # Boost fresh content
    recency_half_life_days=30,   # Decay rate for recency
)

# Score breakdown for debugging
ranked = ranker.rank(results, user_preferences)
for r in ranked:
    print(f"Score: {r.final_score}")
    print(f"  RRF: {r.rrf_score}")
    print(f"  Recency boost: {r.recency_boost}")
    print(f"  Personalization: {r.personalization_boost}")
```

**Key Points**:
- Recency uses exponential decay with math.exp()
- Personalization boosts preferred content types
- Score breakdown aids debugging and tuning

### Query Parsing with Temporal Extraction

**Pattern**: Extract structured constraints from natural language

```python
from penf_lib.search import QueryParser, TemporalQueryParser

parser = QueryParser()
temporal_parser = TemporalQueryParser()

# Parse natural language query
query = "emails from john about budget last week"
parsed = parser.parse(query)

# Extract temporal constraints
temporal = temporal_parser.extract_temporal(query)
# Returns: TemporalConstraint(start=datetime(...), end=datetime(...))

# Supported patterns:
# - "last week", "yesterday", "this month"
# - "since January", "before March 2026"
# - "between Jan 1 and Jan 15"
# - "in the past 30 days"
```

**Key Points**:
- Uses dateparser for flexible date parsing
- Returns structured TemporalConstraint objects
- Handles relative and absolute date references

### Hybrid Search Execution

**Pattern**: Parallel full-text and semantic search with fusion

```python
from penf_lib.search import SearchEngine, SearchQuery, ContentTypeFilter

engine = SearchEngine(session, tenant_id, cache_manager)

response = await engine.search(SearchQuery(
    query="project alpha budget meeting",
    content_types=[ContentTypeFilter.EMAIL, ContentTypeFilter.MEETING],
    date_from=datetime(2026, 1, 1),
    date_to=datetime(2026, 1, 31),
    limit=20,
))

# Response includes:
# - results: List[SearchResult] with ranked results
# - metadata: SearchMetadata with timing and counts
# - suggestions: Optional query corrections
```

**Key Points**:
- Full-text and semantic search run in parallel (asyncio.gather)
- Results fused with RRF before ranking
- Cache checked before search, updated after
- Analytics recorded for each query

### Correlation Discovery

**Pattern**: Find related content across different types

```python
from penf_lib.search import CorrelationDiscovery, CorrelationType

discovery = CorrelationDiscovery(session, tenant_id)

# Find all correlations for an entity
correlations = await discovery.find_correlations(
    entity_id=uuid.UUID("..."),
    correlation_types=[
        CorrelationType.PERSON,      # Same participants
        CorrelationType.TOPIC,       # Shared topics/keywords
        CorrelationType.PROJECT,     # Related projects
        CorrelationType.TEMPORAL,    # Close in time
        CorrelationType.THREAD,      # Same conversation thread
    ],
    max_depth=2,                     # Relationship depth
    min_confidence=0.5,              # Minimum correlation score
)

# Convenience functions for common patterns
related = await find_related_by_person(session, tenant_id, person_email)
project_items = await find_related_by_project(session, tenant_id, project_name)
```

**Key Points**:
- Correlations scored by confidence (0-1)
- Multiple correlation types combinable
- Depth controls relationship hops

### Filter Pipeline

**Pattern**: Composable filters for result refinement

```python
from penf_lib.search import (
    FilterPipeline,
    ContentTypeFilterImpl,
    DateRangeFilter,
    ParticipantFilter,
    ConfidenceFilter,
)

pipeline = FilterPipeline()
pipeline.add_filter(ContentTypeFilterImpl([ContentTypeFilter.EMAIL]))
pipeline.add_filter(DateRangeFilter(start=datetime(2026, 1, 1)))
pipeline.add_filter(ParticipantFilter(emails=["john@example.com"]))
pipeline.add_filter(ConfidenceFilter(min_confidence=0.7))

# Apply to results
filtered = await pipeline.filter(results)

# Get filter statistics
stats = pipeline.get_statistics()
print(f"Filtered {stats.removed_count} results")
```

**Key Points**:
- Filters apply in order (short-circuit on first rejection)
- Statistics track filter effectiveness
- Custom filters implement SearchFilter protocol

### Query Caching

**Pattern**: Multi-level cache for queries and embeddings

```python
from penf_lib.search import SearchCacheManager, QueryCache, EmbeddingCache

cache_manager = SearchCacheManager(
    query_cache=QueryCache(max_size=1000, ttl_seconds=300),
    embedding_cache=EmbeddingCache(max_size=5000, ttl_seconds=3600),
)

# Check cache before search
cached = await cache_manager.get_query_result(query_hash)
if cached:
    return cached

# Cache result after search
await cache_manager.set_query_result(query_hash, results, ttl=300)

# Embeddings cached separately (longer TTL)
embedding = await cache_manager.get_or_compute_embedding(
    text,
    compute_fn=lambda t: embedder.embed(t),
)
```

**Key Points**:
- Query cache: short TTL (5 min), hash-based lookup
- Embedding cache: longer TTL (1 hour), reduces model calls
- LRU eviction when max size reached

## CLI Commands

```bash
# Basic search
penf search "project alpha meeting notes"
penf search "emails from john" --type email
penf search "budget discussion" --since "last week"

# Advanced search
penf search "quarterly report" --type email,document --limit 50
penf search "team standup" --from 2026-01-01 --to 2026-01-31

# Correlations
penf search related <entity-id>
penf search related <entity-id> --type person,topic

# Session management
penf search session list
penf search session show <session-id>

# Analytics
penf search analytics trending
penf search analytics performance

# Suggestions
penf search suggest "budgt"  # Returns "budget"
```

## Configuration

```python
# Search engine configuration
SEARCH_CONFIG = {
    # Hybrid search weights
    "full_text_weight": 0.5,
    "semantic_weight": 0.5,

    # RRF parameters
    "rrf_k": 60,

    # Ranking weights
    "personalization_weight": 0.1,
    "recency_weight": 0.1,
    "recency_half_life_days": 30,

    # Cache settings
    "query_cache_size": 1000,
    "query_cache_ttl_seconds": 300,
    "embedding_cache_size": 5000,
    "embedding_cache_ttl_seconds": 3600,

    # Limits
    "max_results": 100,
    "default_results": 20,
    "max_correlation_depth": 3,
}
```

## Error Handling

### Common Errors

| Error | Cause | Resolution |
|-------|-------|------------|
| `QueryParseError` | Invalid query syntax | Provide query correction |
| `TemporalParseError` | Unrecognized date format | Fall back to no constraint |
| `EmbeddingError` | Embedding model failure | Use full-text only fallback |
| `SearchTimeoutError` | Query too broad | Suggest refinements |
| `CacheError` | Cache unavailable | Proceed without cache |

### Fallback Strategy

```python
# Search strategy enum
class SearchStrategy(Enum):
    HYBRID = "hybrid"              # Full-text + semantic
    FULL_TEXT = "full_text"        # Full-text only
    SEMANTIC = "semantic"          # Semantic only
    FULL_TEXT_FALLBACK = "full_text_fallback"  # After embedding failure
    NO_RESULTS = "no_results"      # Nothing found

# Automatic fallback on embedding failure
async def search_with_fallback(query: SearchQuery) -> SearchResponse:
    try:
        return await engine.hybrid_search(query)
    except EmbeddingError:
        logger.warning("Falling back to full-text search")
        return await engine.full_text_search(query)
```

## Testing

```bash
# Unit tests
pytest tests/unit/test_query_parser.py
pytest tests/unit/test_ranking.py
pytest tests/unit/test_cache.py
pytest tests/unit/test_filters.py
pytest tests/unit/test_analytics.py
pytest tests/unit/test_suggestions.py
pytest tests/unit/test_session.py

# Integration tests
pytest tests/integration/test_search_engine.py
pytest tests/integration/test_correlations.py

# Contract tests
pytest tests/contract/test_search_api.py
```

### Test Fixtures

```python
@pytest.fixture
def sample_search_results():
    """Sample search results for testing"""
    return [
        SearchResult(
            entity_id=uuid.uuid4(),
            content_type=ContentTypeFilter.EMAIL,
            title="Budget Meeting Notes",
            preview=ContentPreview(snippet="Discussed Q1 budget..."),
            relevance_score=0.95,
            timestamp=datetime.now(timezone.utc),
        ),
        # ... more results
    ]

@pytest.fixture
def mock_embedder():
    """Mock embedding generator"""
    return MockEmbedder(embedding_dim=384)
```

## Integration Points

- **Database (001)**: Multi-tenant search index storage
- **AI Coordination (003)**: Embedding generation for semantic search
- **Daily Review (006)**: Surfaces relevant content for review
- **Observability (011)**: Search latency and cache metrics

## Performance Targets

| Metric | Target | Monitoring |
|--------|--------|------------|
| Query parse time | <50ms | `search_parse_duration_seconds` |
| Hybrid search | <500ms | `search_duration_seconds` |
| Cache hit rate | >60% | `search_cache_hit_rate` |
| Correlation discovery | <1s | `correlation_duration_seconds` |

## Related Documentation

- [Search API Schema](../../specs/007-search-interface/contracts/search-api.yaml)
- [Query Schema](../../specs/007-search-interface/contracts/query-schema.json)
- [Data Model](../../specs/007-search-interface/data-model.md)
- [Research Notes](../../specs/007-search-interface/research.md)
- [Architecture Patterns](../ARCHITECTURE.md)
