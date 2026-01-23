# Architecture Review: Scalability & Performance

**Review Date**: 2026-01-23
**Reviewer**: Architecture Review Agent
**Context Reference**: pass-00-context.md, pass-01-structure.md, pass-02-security.md

---

## Summary

Penfold demonstrates a **performance-conscious architecture** with solid foundational patterns for scalability. The codebase implements connection pooling, caching at multiple layers, parallel execution for search operations, and configurable worker pools. The architecture is well-suited for the current single-user deployment model and has reasonable headroom for growth.

Key performance strengths include:
- Proper PostgreSQL connection pooling with configurable limits
- Multi-layer caching (embedding cache, search cache, Redis-backed options)
- Parallel execution of BM25 + vector search with graceful degradation
- Well-indexed database schema with tenant-first filtering
- Temporal workflows for durable, retryable operations

Primary performance concerns relate to:
- Sequential batch insert patterns that could be optimized
- COUNT(*) queries executed separately from main search queries
- Potential memory pressure from large embedding arrays
- Missing connection pool monitoring and backpressure

---

## Previous Pass Reference

**Pass 1 (Structure)** identified:
- Repository pattern providing single points for database access
- Temporal workflows with dedicated task queues (penfold-main, penfold-ai, penfold-email)
- Search service implementation gaps blocking performance validation

**Pass 2 (Security)** identified:
- Request size limits (1MB) on HTTP endpoints
- No rate limiting on push notification endpoint

This performance review builds on those findings with focused analysis of concurrency patterns, database access patterns, caching strategies, and scaling readiness.

---

## Findings

### Strengths

#### 1. Proper Connection Pooling with pgxpool

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/db/db.go`

The database layer uses `pgxpool` with sensible defaults:

```go
func DefaultConfig() *Config {
    return &Config{
        MaxConns:        25,
        MinConns:        5,
        MaxConnLifetime: time.Hour,
        MaxConnIdleTime: 30 * time.Minute,
        ConnectTimeout:  10 * time.Second,
    }
}

// Connection configuration is applied properly
poolConfig.MaxConns = cfg.MaxConns
poolConfig.MinConns = cfg.MinConns
poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
```

**Assessment**: The connection pool is properly configured with:
- Environment variable overrides (`DB_MAX_CONNS`, `DB_MIN_CONNS`)
- Connection lifetime limits to prevent stale connections
- Idle timeout to release unused connections
- Connection retry logic with configurable attempts

This is appropriate for the target workload (200 emails + 15 meetings/week).

#### 2. Multi-Layer Caching Strategy

**Embedding Cache** (`/Users/james/github/otherjamesbrown/penfold/pkg/embeddings/cache.go`):
- LRU memory cache with TTL (default: 10,000 embeddings, ~40MB)
- Redis cache option for distributed caching
- Proper cache key generation using SHA-256 hashing
- Copy-on-read semantics to prevent mutation

```go
func DefaultMemoryCacheConfig() *MemoryCacheConfig {
    return &MemoryCacheConfig{
        MaxSize: 10000, // 10k embeddings * ~4KB each = ~40MB max
        TTL:     time.Hour,
    }
}
```

**Search Cache** (`/Users/james/github/otherjamesbrown/penfold/services/search/cache/cache.go`):
- TTL-based expiration (5 min default, 30 sec for realtime)
- Tenant-isolated cache keys (critical for security)
- Background cleanup goroutine
- Metrics tracking (hits, misses, evictions)
- Document-level invalidation support

```go
// CRITICAL: tenant_id is ALWAYS included to ensure tenant isolation
func GenerateKey(query string, tenantID string, filters *searchv1.FilterOptions, ...) string {
    keyParts := []string{
        "tenant:" + tenantID, // CRITICAL: Tenant isolation
        "query:" + hashString(query),
        ...
    }
}
```

**Assessment**: Strong caching architecture with proper cache key isolation and invalidation patterns.

#### 3. Parallel Search Execution with Graceful Degradation

**Location**: `/Users/james/github/otherjamesbrown/penfold/services/search/engine/fusion.go`

The hybrid search engine executes BM25 and vector searches in parallel:

```go
if e.config.ParallelExecution {
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        bm25Results, bm25Err = e.bm25Engine.Search(ctx, query, bm25Filters, fetchLimit, 0)
    }()

    go func() {
        defer wg.Done()
        vectorResults, vectorErr = e.vectorEngine.Search(ctx, tenantID, query, vectorFilters, fetchLimit, 0)
    }()

    wg.Wait()
}

// Graceful degradation - continue if one search fails
if e.config.GracefulDegradation {
    // Continues if only one search failed
}
```

**Assessment**: Excellent pattern for achieving the < 3 second typical search target. Parallel execution roughly halves search latency when both engines are healthy, and graceful degradation ensures partial results are returned when one engine fails.

#### 4. Well-Indexed Database Schema

**Location**: `/Users/james/github/otherjamesbrown/penfold/migrations/017_mention_resolution.sql`

The schema demonstrates thoughtful indexing:

```sql
-- Tenant-first composite indexes
CREATE INDEX idx_content_mentions_pending
    ON content_mentions (tenant_id, status)
    WHERE status = 'pending';

-- Partial indexes for common query patterns
CREATE INDEX idx_content_mentions_entity
    ON content_mentions (tenant_id, entity_type, resolved_entity_id)
    WHERE resolved_entity_id IS NOT NULL;

-- Lower-cased text for case-insensitive lookups
CREATE INDEX idx_content_mentions_text
    ON content_mentions (tenant_id, entity_type, LOWER(mentioned_text));

-- Affinity score ranking index
CREATE INDEX idx_entity_affinity_high_score
    ON entity_project_affinity (tenant_id, project_id, affinity_score DESC)
    WHERE affinity_score > 0.5;
```

**Assessment**: Indexes are designed for:
- Tenant isolation (tenant_id first in composite indexes)
- Common query patterns (pending status, resolved entities)
- Partial indexes to reduce index size
- Proper ordering for range and sort queries

#### 5. Configurable Worker Pools with Appropriate Defaults

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/enrichment/workers/pool.go`

The enrichment worker pool is well-designed:

```go
func DefaultWorkerConfigs() map[WorkerType]WorkerConfig {
    return map[WorkerType]WorkerConfig{
        WorkerTypeIngest: {
            Count:             4,
            BatchSize:         10,
            VisibilityTimeout: 60 * time.Second,
            PollInterval:      1 * time.Second,
        },
        WorkerTypeEnrichment: {
            Count:             8,
            BatchSize:         1,
            VisibilityTimeout: 120 * time.Second,
            PollInterval:      500 * time.Millisecond,
        },
        WorkerTypeAI: {
            Count:             4,
            BatchSize:         1,
            VisibilityTimeout: 300 * time.Second, // AI is slow
            PollInterval:      1 * time.Second,
        },
    }
}
```

**Assessment**: Thoughtful differentiation:
- Ingest: Higher batch size, lower timeout (fast operations)
- Enrichment: Single items, moderate timeout
- AI: Single items, long timeout (5 min for model inference)

#### 6. Temporal Workflow Configuration

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/temporal/worker.go`

Temporal workers have sensible concurrency limits:

```go
func NewWorker(c client.Client, taskQueue string, options ...WorkerOption) worker.Worker {
    opts := worker.Options{
        MaxConcurrentActivityExecutionSize:     10,
        MaxConcurrentWorkflowTaskExecutionSize: 10,
    }
    // ...
}
```

**Assessment**: Default limits prevent resource exhaustion while allowing reasonable parallelism. The functional options pattern (`WithMaxConcurrentActivities`, etc.) enables tuning per deployment.

#### 7. Proper Resource Cleanup

Consistent use of `defer rows.Close()` and `defer tx.Rollback()` throughout repository implementations:

```go
// Example from pkg/mentions/postgres_repository.go
rows, err := r.db.Query(ctx, query, args...)
if err != nil {
    return nil, fmt.Errorf("listing mentions: %w", err)
}
defer rows.Close()  // Always closed
```

**Assessment**: Resource leaks are prevented through consistent cleanup patterns.

---

### Concerns

#### 1. Sequential Batch Inserts (Medium Impact)

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/mentions/postgres_repository.go`

The `BatchCreateMentions` function inserts rows sequentially within a transaction:

```go
func (r *PostgresRepository) BatchCreateMentions(ctx context.Context, inputs []MentionInput) ([]ContentMention, error) {
    tx, err := r.db.Begin(ctx)
    // ...
    for i, input := range inputs {
        query := `INSERT INTO content_mentions (...) VALUES ($1, $2, ...) RETURNING id, created_at`
        err := tx.QueryRow(ctx, query, ...).Scan(&id, &createdAt)
        // Sequential execution - one round trip per row
    }
    return mentions, tx.Commit(ctx)
}
```

**Impact**: For batch operations (e.g., extracting 50+ mentions from a meeting), this results in N database round trips instead of 1.

**Recommendation**: Use `pgx.CopyFrom` or construct a single multi-row INSERT:
```go
// More efficient batch insert pattern
_, err := tx.CopyFrom(ctx,
    pgx.Identifier{"content_mentions"},
    []string{"tenant_id", "content_id", "entity_type", ...},
    pgx.CopyFromSlice(len(inputs), func(i int) ([]any, error) {
        return []any{tenantID, inputs[i].ContentID, ...}, nil
    }),
)
```

#### 2. Separate COUNT(*) Queries (Medium Impact)

**Location**: `/Users/james/github/otherjamesbrown/penfold/services/search/engine/bm25.go`, `vector.go`

Search operations execute COUNT(*) as a separate query:

```go
// Execute the main search query
rows, err := e.pool.Query(ctx, sql, args...)
// ... process results ...

// Get total count (separate query)
totalCount, err := e.getTotalCount(ctx, processedQuery, filters)
```

The count query repeats all the filtering logic:

```go
func (e *BM25Engine) getTotalCount(ctx context.Context, ...) (int64, error) {
    sql := `SELECT COUNT(*) FROM documents d WHERE ...`
    // Repeats all WHERE clause conditions
}
```

**Impact**: Doubles query execution for each search request. On large datasets, COUNT(*) can be expensive.

**Recommendation**:
1. Use window functions: `COUNT(*) OVER() as total_count` in the main query
2. Or use EXPLAIN-based estimates for approximate counts
3. Consider caching counts with TTL for repeated queries

#### 3. Potential Memory Pressure from Embeddings (Low Impact)

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/embeddings/cache.go`

Embeddings are stored as `[]float32` and copied on cache access:

```go
// Return a copy to prevent mutation
result := make([]float32, len(entry.embedding))
copy(result, entry.embedding)
```

**Impact**: 10,000 embeddings at ~4KB each = ~40MB. With frequent cache access, the copy-on-read creates temporary allocations that could pressure the GC.

**Recommendation**: Monitor GC pause times. Consider:
- Read-only access pattern (skip copy if caller doesn't mutate)
- Pooled embedding slices with `sync.Pool`
- Binary encoding for Redis cache instead of JSON

#### 4. Missing Connection Pool Monitoring (Low Impact)

**Location**: `/Users/james/github/otherjamesbrown/penfold/pkg/db/db.go`

The connection pool is created but pool statistics are not exposed:

```go
pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
// No metrics exposed for pool.Stat()
```

**Impact**: Unable to detect connection pool exhaustion before it causes failures.

**Recommendation**: Export pool statistics to Prometheus:
```go
// pgxpool provides Stat() method
stats := pool.Stat()
dbActiveConns.Set(float64(stats.AcquiredConns()))
dbIdleConns.Set(float64(stats.IdleConns()))
dbMaxConns.Set(float64(stats.MaxConns()))
```

#### 5. No Backpressure on Search Queries (Medium Impact)

**Location**: Search endpoints have no rate limiting or queue depth control

**Impact**: Under load, all concurrent requests hit the database simultaneously. This could exhaust connection pool and cause cascading failures.

**Recommendation**:
- Add semaphore-based concurrency limiting for search
- Implement adaptive load shedding (return cached results under pressure)
- Add request queuing with timeout

#### 6. Unbounded Query Results in Some Repository Methods (Low Impact)

**Location**: Various repository `List*` methods

Some listing methods don't enforce maximum limits:

```go
// pkg/mentions/postgres_repository.go
if filter.Limit > 0 {
    query += fmt.Sprintf(" LIMIT %d", filter.Limit)
}
// No default limit if filter.Limit is 0
```

**Impact**: A query with no limit could return millions of rows, exhausting memory.

**Recommendation**: Enforce maximum limits:
```go
limit := filter.Limit
if limit <= 0 || limit > 1000 {
    limit = 100 // Default
}
```

---

### Recommendations

#### High Priority

1. **Optimize Batch Insert Performance**: Replace sequential inserts with `pgx.CopyFrom` for batch mention creation. Expected improvement: 10-50x for batches of 50+ items.

2. **Use Window Functions for Total Counts**: Modify search queries to use `COUNT(*) OVER()` instead of separate count queries. Expected improvement: 40-50% reduction in search query time.

3. **Add Connection Pool Metrics**: Export `pgxpool.Stat()` values to Prometheus for capacity planning and alerting.

#### Medium Priority

4. **Implement Search Concurrency Limits**: Add semaphore-based limiting to prevent search overload from exhausting database connections.

5. **Add Default Limits to List Operations**: Enforce maximum result set sizes (e.g., 1000) to prevent memory exhaustion.

6. **Monitor Embedding Cache GC Impact**: Add metrics for cache-related allocations; consider sync.Pool if GC pauses exceed 10ms.

#### Low Priority

7. **Consider Approximate Counts**: For large datasets, use `pg_stat_user_tables.n_live_tup` estimates instead of COUNT(*).

8. **Profile Memory Under Load**: Run benchmarks with representative datasets to identify memory bottlenecks before they become production issues.

---

## Scalability Assessment

### Horizontal vs Vertical Scaling Readiness

| Component | Horizontal Readiness | Notes |
|-----------|---------------------|-------|
| Gateway | Good | Stateless, can run multiple instances behind load balancer |
| Worker | Good | Temporal manages work distribution across workers |
| Search | Moderate | Would need read replicas for DB; embedding service is SPOF |
| Database | Limited | Single PostgreSQL; would need read replicas or sharding |
| Embedding Service | Limited | Single MLX instance; could add load balancing |

### Performance Target Analysis

| Target | Current Architecture Support | Assessment |
|--------|------------------------------|------------|
| Search < 15 seconds | Parallel BM25+vector, caching | Likely achievable |
| Search < 3 seconds (typical) | LRU cache, indexed queries | Achievable for cached queries |
| 200 emails/week processing | Worker pools, Temporal durability | Well within capacity |
| 15 meetings/week processing | AI worker with 5-min timeout | Appropriate configuration |
| Context assembly < 15 min | Hybrid search, entity correlation | Depends on search implementation completion |

### Bottleneck Analysis

1. **Current Bottleneck**: Search service implementation is incomplete (noted in Pass 1)
2. **Potential Bottleneck**: Single embedding service instance (MLX on dev01)
3. **Future Bottleneck**: PostgreSQL as data grows (addressed by good indexing)

---

## Conclusion

Penfold's architecture demonstrates solid performance engineering appropriate for a single-user personal information system. The use of connection pooling, multi-layer caching, parallel search execution, and configurable worker pools provides a strong foundation.

The primary performance concerns are implementation patterns (sequential batch inserts, separate count queries) rather than architectural issues. These can be addressed through targeted optimizations without architectural changes.

The performance targets defined in the constitution (< 15 seconds search, < 3 seconds typical) appear achievable with the current architecture once the search service implementation is complete.

**Overall Scalability Assessment**: Good - appropriate for current scale with clear paths for growth.
