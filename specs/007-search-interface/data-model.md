# Data Model: Search and Query Interface

**Feature**: 007-search-interface
**Date**: 2026-01-15
**Status**: Complete

## Overview

The search interface introduces new entities for managing search queries, sessions, and analytics while consuming existing entities (Source, Assertion, Person, Project, Embedding) without modification. All new entities follow the established patterns: TimestampMixin, TenantMixin, SoftDeleteMixin.

## Entity Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              SEARCH DOMAIN                                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌──────────────┐     ┌──────────────┐     ┌───────────────────┐                │
│  │ SearchQuery  │────▶│SearchSession │────▶│  SearchHistory    │                │
│  │              │     │              │     │                   │                │
│  │ - query_text │     │ - session_id │     │ - queries[]       │                │
│  │ - filters    │     │ - user_email │     │ - refinements     │                │
│  │ - temporal   │     │ - context    │     │ - selections      │                │
│  └──────┬───────┘     └──────────────┘     └───────────────────┘                │
│         │                                                                        │
│         ▼                                                                        │
│  ┌──────────────┐     ┌──────────────┐     ┌───────────────────┐                │
│  │SearchResult  │────▶│SearchFilter  │     │ QuerySuggestion   │                │
│  │              │     │              │     │                   │                │
│  │ - entity_ref │     │ - type       │     │ - suggested_text  │                │
│  │ - score      │     │ - criteria   │     │ - frequency       │                │
│  │ - preview    │     │ - active     │     │ - context         │                │
│  └──────┬───────┘     └──────────────┘     └───────────────────┘                │
│         │                                                                        │
│         ▼                                                                        │
│  ┌──────────────────────────────────────────────────────────────────────────┐   │
│  │                         EXISTING ENTITIES (consumed)                      │   │
│  │                                                                           │   │
│  │  Source  │  Assertion  │  Person  │  Project  │  Team  │  Embedding       │   │
│  │                                                                           │   │
│  └──────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## New Entities

### SearchQuery (Pydantic Model - Request)

Query parameters submitted for search operations. Not persisted directly - used for request validation.

```python
from pydantic import BaseModel, Field
from typing import Optional, List
from datetime import datetime
from enum import Enum

class ContentTypeFilter(str, Enum):
    EMAIL = "email"
    MEETING = "meeting"
    DOCUMENT = "document"
    SLACK = "slack"
    ALL = "all"

class SortOrder(str, Enum):
    RELEVANCE = "relevance"
    RECENCY = "recency"
    ALPHABETICAL = "alphabetical"

class TemporalConstraint(BaseModel):
    """Temporal filtering for queries."""
    start_date: Optional[datetime] = None
    end_date: Optional[datetime] = None
    relative_expression: Optional[str] = None  # "last week", "since December"

class SearchQuery(BaseModel):
    """Search query parameters."""
    query_text: str = Field(..., min_length=1, max_length=500)
    content_types: List[ContentTypeFilter] = Field(default=[ContentTypeFilter.ALL])
    temporal: Optional[TemporalConstraint] = None
    participants: Optional[List[str]] = Field(default=None, max_items=20)
    projects: Optional[List[str]] = Field(default=None, max_items=10)
    min_confidence: Optional[float] = Field(default=None, ge=0.0, le=1.0)
    include_relationships: bool = Field(default=True)
    sort_by: SortOrder = Field(default=SortOrder.RELEVANCE)
    limit: int = Field(default=25, ge=1, le=100)
    offset: int = Field(default=0, ge=0)

    class Config:
        json_schema_extra = {
            "example": {
                "query_text": "customer deployment issues",
                "content_types": ["email", "meeting"],
                "temporal": {"relative_expression": "last week"},
                "limit": 25,
            }
        }
```

### SearchResult (Pydantic Model - Response)

Individual result item returned from search operations.

```python
class ContentPreview(BaseModel):
    """Preview of content for search results."""
    title: Optional[str] = None
    snippet: str = Field(..., max_length=500)
    highlight_positions: List[tuple[int, int]] = Field(default=[])

class SearchResult(BaseModel):
    """Individual search result."""
    result_id: str  # UUID
    entity_type: str  # source, assertion, person, project
    entity_id: int

    # Content information
    content_type: ContentTypeFilter
    preview: ContentPreview
    source_attribution: str  # Link/reference to original

    # Scoring
    relevance_score: float = Field(ge=0.0, le=1.0)
    rrf_score: float  # Reciprocal rank fusion score
    confidence_score: Optional[float] = None  # AI processing confidence

    # Temporal
    timestamp: datetime

    # Context
    participants: List[str] = Field(default=[])
    project_refs: List[str] = Field(default=[])
    tags: List[str] = Field(default=[])

    # Relationships
    related_content_ids: List[str] = Field(default=[])
    relationship_strength: Optional[float] = None

    class Config:
        json_schema_extra = {
            "example": {
                "result_id": "550e8400-e29b-41d4-a716-446655440000",
                "entity_type": "source",
                "entity_id": 12345,
                "content_type": "email",
                "preview": {
                    "title": "Re: Deployment Issues",
                    "snippet": "...the customer reported issues with the deployment...",
                    "highlight_positions": [[4, 12], [35, 45]],
                },
                "relevance_score": 0.92,
                "timestamp": "2026-01-10T14:30:00Z",
            }
        }
```

### SearchResponse (Pydantic Model - Response)

Complete response for a search operation.

```python
class SearchMetadata(BaseModel):
    """Metadata about search execution."""
    query_id: str  # UUID for tracking
    execution_time_ms: int
    total_results: int
    returned_results: int
    search_strategy: str  # "hybrid", "full_text", "semantic"
    cache_hit: bool

class SearchResponse(BaseModel):
    """Complete search response."""
    metadata: SearchMetadata
    results: List[SearchResult]
    suggestions: List[str] = Field(default=[])  # Query refinement suggestions
    filters_applied: dict = Field(default={})

    # Pagination
    has_more: bool
    next_offset: Optional[int] = None
```

---

### SearchSession (SQLAlchemy Model - Persisted)

Tracks user search sessions for history and analytics.

```python
class SearchSession(Base, TimestampMixin, TenantMixin, SoftDeleteMixin):
    """User search session for history tracking."""
    __tablename__ = "search_sessions"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    session_id = Column(String(255), nullable=False, unique=True, index=True)
    user_email = Column(String(254), nullable=False)

    # Session state
    is_active = Column(Boolean, default=True)
    started_at = Column(DateTime(timezone=True), server_default=func.now())
    last_activity_at = Column(DateTime(timezone=True), onupdate=func.now())
    expires_at = Column(DateTime(timezone=True))

    # Context
    session_context = Column(JSONB, default={})  # User preferences, recent selections

    # Statistics
    total_queries = Column(Integer, default=0)
    successful_searches = Column(Integer, default=0)  # User found what they needed

    # Relationships
    queries = relationship("SearchQueryRecord", back_populates="session")

    __table_args__ = (
        Index("idx_search_session_user", "tenant_id", "user_email", "is_active"),
        Index("idx_search_session_activity", "last_activity_at"),
    )
```

### SearchQueryRecord (SQLAlchemy Model - Persisted)

Persisted record of search queries for history and analytics.

```python
class SearchQueryRecord(Base, TimestampMixin, TenantMixin):
    """Persisted search query for history and analytics."""
    __tablename__ = "search_query_records"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    session_id = Column(UUID(as_uuid=True), ForeignKey("search_sessions.id"))

    # Query details
    query_text = Column(Text, nullable=False)
    query_hash = Column(String(64), nullable=False)  # For caching
    normalized_query = Column(Text)  # Processed query
    query_vector_hash = Column(String(64))  # Embedding fingerprint

    # Filters applied
    filters = Column(JSONB, default={})
    temporal_constraint = Column(JSONB)  # Start/end dates

    # Execution
    execution_time_ms = Column(Integer)
    result_count = Column(Integer)
    cache_hit = Column(Boolean, default=False)
    search_strategy = Column(String(50))  # hybrid, full_text, semantic

    # User interaction
    results_clicked = Column(ARRAY(String))  # Result IDs user interacted with
    search_successful = Column(Boolean)  # User found what they needed (implicit/explicit)

    # Relationships
    session = relationship("SearchSession", back_populates="queries")

    __table_args__ = (
        Index("idx_query_record_session", "session_id", "created_at"),
        Index("idx_query_record_hash", "query_hash"),
        Index("idx_query_record_tenant", "tenant_id", "created_at"),
    )
```

### QuerySuggestion (SQLAlchemy Model - Persisted)

Cached query suggestions based on popular queries and content patterns.

```python
class QuerySuggestion(Base, TimestampMixin, TenantMixin):
    """Cached query suggestions for autocomplete."""
    __tablename__ = "query_suggestions"

    id = Column(BIGINT, primary_key=True)

    # Suggestion content
    suggestion_text = Column(String(500), nullable=False)
    suggestion_type = Column(String(50))  # popular, recent, contextual

    # Relevance tracking
    frequency = Column(Integer, default=1)
    success_rate = Column(DECIMAL(4, 3))  # How often users find results
    last_used_at = Column(DateTime(timezone=True))

    # Context
    context_tags = Column(ARRAY(String))  # Related topics
    content_types = Column(ARRAY(String))  # Relevant content types

    __table_args__ = (
        Index("idx_suggestion_text", "tenant_id", "suggestion_text"),
        Index("idx_suggestion_frequency", "frequency", "success_rate"),
        UniqueConstraint("tenant_id", "suggestion_text", name="uc_suggestion_per_tenant"),
    )
```

### SearchAnalytics (SQLAlchemy Model - Persisted)

Aggregated search analytics for P3 analytics feature.

```python
class SearchAnalytics(Base, TimestampMixin, TenantMixin):
    """Aggregated search analytics (P3 feature)."""
    __tablename__ = "search_analytics"

    id = Column(BIGINT, primary_key=True)

    # Time bucket
    bucket_date = Column(DateTime(timezone=True), nullable=False)  # Daily aggregation

    # Query patterns
    total_queries = Column(Integer, default=0)
    unique_users = Column(Integer, default=0)
    avg_execution_time_ms = Column(Float)
    cache_hit_rate = Column(DECIMAL(4, 3))

    # Result metrics
    avg_result_count = Column(Float)
    zero_result_queries = Column(Integer, default=0)
    successful_search_rate = Column(DECIMAL(4, 3))  # Users found what they needed

    # Content distribution
    content_type_distribution = Column(JSONB)  # {email: 0.4, meeting: 0.3, ...}
    popular_queries = Column(JSONB)  # Top 10 queries for this period

    # Performance
    p50_execution_time_ms = Column(Integer)
    p95_execution_time_ms = Column(Integer)
    p99_execution_time_ms = Column(Integer)

    __table_args__ = (
        Index("idx_analytics_bucket", "tenant_id", "bucket_date"),
        UniqueConstraint("tenant_id", "bucket_date", name="uc_analytics_per_day"),
    )
```

---

## Entity Relationships

### Relationship Diagram

```
SearchSession 1 ─────▶ * SearchQueryRecord
     │                        │
     │                        │ references
     │                        ▼
     │              ┌─────────────────────┐
     │              │  EXISTING ENTITIES  │
     │              │  Source, Assertion  │
     │              │  Person, Project    │
     │              │  Embedding          │
     │              └─────────────────────┘
     │
     ▼
QuerySuggestion (tenant-scoped)

SearchAnalytics (daily aggregation per tenant)
```

### Key Relationships

1. **SearchSession -> SearchQueryRecord**: One-to-many. A session contains multiple queries.
2. **SearchQueryRecord -> Existing Entities**: References via entity_type and entity_id in results.
3. **QuerySuggestion**: Standalone, tenant-scoped suggestions.
4. **SearchAnalytics**: Standalone, daily aggregations.

---

## Validation Rules

### SearchQuery Validation

| Field | Rule | Error Message |
|-------|------|---------------|
| query_text | 1-500 characters | "Query must be 1-500 characters" |
| content_types | Valid enum values | "Invalid content type: {value}" |
| temporal.start_date | Before end_date if both present | "Start date must be before end date" |
| participants | Max 20 items | "Maximum 20 participant filters allowed" |
| min_confidence | 0.0 to 1.0 | "Confidence must be between 0 and 1" |
| limit | 1-100 | "Limit must be between 1 and 100" |

### SearchSession Validation

| Field | Rule | Error Message |
|-------|------|---------------|
| user_email | Valid email format | "Invalid email format" |
| session_id | Non-empty, unique | "Session ID required and must be unique" |

### QuerySuggestion Validation

| Field | Rule | Error Message |
|-------|------|---------------|
| suggestion_text | 1-500 characters | "Suggestion must be 1-500 characters" |
| frequency | Non-negative integer | "Frequency must be non-negative" |
| success_rate | 0.0 to 1.0 | "Success rate must be between 0 and 1" |

---

## State Transitions

### SearchSession States

```
                    ┌─────────────────┐
                    │                 │
        create()    │    ACTIVE       │  last_activity > 24h
    ───────────────▶│  is_active=True │───────────────────────▶ expired()
                    │                 │
                    └────────┬────────┘
                             │
                             │ user_logout() or explicit_close()
                             ▼
                    ┌─────────────────┐
                    │                 │
                    │   INACTIVE      │
                    │ is_active=False │
                    │                 │
                    └─────────────────┘
```

### Search Query Processing Flow

```
SearchQuery (Pydantic)
        │
        │ validate()
        ▼
Temporal Parsing
        │
        │ extract_dates()
        ▼
Query Embedding
        │
        │ generate_vector()
        ▼
Hybrid Search
        │
        ├─────────────────┐
        │                 │
   Full-text          Vector
   Search             Search
        │                 │
        └────────┬────────┘
                 │
                 │ RRF fusion()
                 ▼
          Ranking & Filtering
                 │
                 │ apply_filters()
                 ▼
          SearchResponse (Pydantic)
```

---

## Indexes

### New Indexes Required

```sql
-- SearchSession indexes
CREATE INDEX idx_search_session_user ON search_sessions(tenant_id, user_email, is_active);
CREATE INDEX idx_search_session_activity ON search_sessions(last_activity_at);

-- SearchQueryRecord indexes
CREATE INDEX idx_query_record_session ON search_query_records(session_id, created_at);
CREATE INDEX idx_query_record_hash ON search_query_records(query_hash);
CREATE INDEX idx_query_record_tenant ON search_query_records(tenant_id, created_at);

-- QuerySuggestion indexes
CREATE INDEX idx_suggestion_text ON query_suggestions(tenant_id, suggestion_text);
CREATE INDEX idx_suggestion_frequency ON query_suggestions(frequency, success_rate);

-- SearchAnalytics indexes
CREATE INDEX idx_analytics_bucket ON search_analytics(tenant_id, bucket_date);

-- Full-text search enhancement (GIN index on sources.raw_content)
CREATE INDEX idx_sources_fulltext ON sources USING GIN(to_tsvector('english', raw_content));

-- Full-text search on assertions
CREATE INDEX idx_assertions_fulltext ON assertions USING GIN(to_tsvector('english', content));
```

---

## Migration Strategy

### New Tables

1. `search_sessions` - New table
2. `search_query_records` - New table
3. `query_suggestions` - New table
4. `search_analytics` - New table (P3)

### Existing Table Modifications

1. `sources` - Add GIN full-text index on `raw_content` (non-breaking)
2. `assertions` - Add GIN full-text index on `content` (non-breaking)

### Migration Script

```python
# alembic migration: add_search_tables

def upgrade():
    # Create search_sessions
    op.create_table(
        'search_sessions',
        sa.Column('id', UUID(), primary_key=True),
        sa.Column('tenant_id', UUID(), nullable=False),
        sa.Column('session_id', sa.String(255), nullable=False, unique=True),
        sa.Column('user_email', sa.String(254), nullable=False),
        sa.Column('is_active', sa.Boolean(), default=True),
        sa.Column('started_at', sa.DateTime(timezone=True), server_default=sa.func.now()),
        sa.Column('last_activity_at', sa.DateTime(timezone=True)),
        sa.Column('expires_at', sa.DateTime(timezone=True)),
        sa.Column('session_context', JSONB(), default={}),
        sa.Column('total_queries', sa.Integer(), default=0),
        sa.Column('successful_searches', sa.Integer(), default=0),
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now()),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now()),
    )

    # Create search_query_records
    op.create_table(
        'search_query_records',
        # ... columns as defined above
    )

    # Create query_suggestions
    op.create_table(
        'query_suggestions',
        # ... columns as defined above
    )

    # Add full-text indexes to existing tables
    op.create_index(
        'idx_sources_fulltext',
        'sources',
        [sa.text("to_tsvector('english', raw_content)")],
        postgresql_using='gin'
    )

def downgrade():
    op.drop_table('search_analytics')
    op.drop_table('query_suggestions')
    op.drop_table('search_query_records')
    op.drop_table('search_sessions')
    op.drop_index('idx_sources_fulltext')
```

---

## Performance Considerations

### Query Optimization

1. **Full-text search**: PostgreSQL GIN indexes on text columns
2. **Vector search**: HNSW index already exists on embeddings (768-dim)
3. **Hybrid search**: Parallel execution of full-text and vector queries

### Scaling Considerations

1. **100k content items**: PostgreSQL handles efficiently with proper indexing
2. **Search session cleanup**: Automatic expiration after 24 hours
3. **Query analytics**: Daily aggregation to avoid hot spots
4. **Cache**: Redis caching for repeated queries
