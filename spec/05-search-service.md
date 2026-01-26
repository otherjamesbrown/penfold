# Search Service Specification

## Overview

The Search Service provides hybrid full-text and semantic vector search capabilities. It combines BM25 keyword search with pgvector similarity search, using Reciprocal Rank Fusion (RRF) to merge results.

## Responsibilities

1. **Query Parsing**: NLP-based query understanding
2. **Query Embedding**: Convert queries to 768-dim vectors
3. **Hybrid Search**: Combine BM25 + vector similarity
4. **Ranking**: RRF fusion with relevance tuning
5. **Filtering**: Date, type, person, project filters
6. **Caching**: Result caching with TTL
7. **Analytics**: Search pattern tracking

## Architecture

```
                    ┌───────────────────────────────────────┐
                    │          Search Service               │
                    │                                       │
   SearchRequest ──▶│  ┌─────────────────────────────┐    │
                    │  │       Query Parser          │    │
                    │  │  (intent, entities, filters)│    │
                    │  └──────────────┬──────────────┘    │
                    │                 │                    │
                    │                 ▼                    │
                    │  ┌─────────────────────────────┐    │
                    │  │      Query Embedder         │    │
                    │  │  (via Embedding Pipeline)   │    │
                    │  └──────────────┬──────────────┘    │
                    │                 │                    │
                    │    ┌────────────┴────────────┐      │
                    │    ▼                         ▼      │
                    │  ┌───────────┐      ┌───────────┐  │
                    │  │  BM25     │      │  Vector   │  │
                    │  │  Search   │      │  Search   │  │
                    │  │(PostgreSQL│      │ (pgvector)│  │
                    │  └─────┬─────┘      └─────┬─────┘  │
                    │        │                  │        │
                    │        └────────┬─────────┘        │
                    │                 ▼                   │
                    │  ┌─────────────────────────────┐   │
                    │  │     RRF Fusion Ranking      │   │
                    │  └──────────────┬──────────────┘   │
                    │                 │                   │
                    │                 ▼                   │
                    │  ┌─────────────────────────────┐   │
                    │  │   Filter & Enrich Results   │   │
                    │  └──────────────┬──────────────┘   │
                    │                 │                   │
  SearchResponse ◀──│                 ▼                   │
                    │  ┌─────────────────────────────┐   │
                    │  │     Cache & Analytics       │   │
                    │  └─────────────────────────────┘   │
                    └───────────────────────────────────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/search/v1/search.proto

syntax = "proto3";
package search.v1;

service SearchService {
  // Main search
  rpc Search(SearchRequest) returns (SearchResponse);

  // Natural language questions
  rpc Ask(AskRequest) returns (AskResponse);

  // Find similar content
  rpc FindSimilar(FindSimilarRequest) returns (FindSimilarResponse);

  // Autocomplete
  rpc Suggest(SuggestRequest) returns (SuggestResponse);

  // Get search analytics
  rpc GetAnalytics(GetAnalyticsRequest) returns (GetAnalyticsResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

message SearchRequest {
  string tenant_id = 1;
  string query = 2;
  SearchFilters filters = 3;
  SearchOptions options = 4;
  int32 limit = 5;
  int32 offset = 6;
}

message SearchFilters {
  repeated string content_types = 1;   // email, meeting, document
  string date_from = 2;                // ISO8601
  string date_to = 3;
  repeated string person_ids = 4;
  repeated string project_ids = 5;
  repeated string labels = 6;
  float min_confidence = 7;            // Minimum AI confidence
}

message SearchOptions {
  bool include_snippets = 1;
  int32 snippet_length = 2;
  bool highlight = 3;
  SearchMode mode = 4;                 // hybrid, keyword, semantic
  float keyword_weight = 5;            // 0.0-1.0, default 0.5
  float semantic_weight = 6;           // 0.0-1.0, default 0.5
}

enum SearchMode {
  SEARCH_MODE_HYBRID = 0;
  SEARCH_MODE_KEYWORD = 1;
  SEARCH_MODE_SEMANTIC = 2;
}

message SearchResponse {
  repeated SearchResult results = 1;
  int32 total_count = 2;
  SearchMetadata metadata = 3;
}

message SearchResult {
  string id = 1;
  string tenant_id = 2;
  string content_type = 3;
  string title = 4;
  string snippet = 5;
  float relevance_score = 6;
  float bm25_score = 7;
  float vector_score = 8;
  google.protobuf.Timestamp timestamp = 9;
  repeated Entity people = 10;
  repeated Entity projects = 11;
  repeated string labels = 12;
  map<string, string> metadata = 13;
}

message SearchMetadata {
  int64 query_time_ms = 1;
  int64 bm25_time_ms = 2;
  int64 vector_time_ms = 3;
  int64 ranking_time_ms = 4;
  string parsed_query = 5;
  repeated string detected_entities = 6;
  bool cache_hit = 7;
}

message AskRequest {
  string tenant_id = 1;
  string question = 2;
  SearchFilters filters = 3;
  int32 context_items = 4;   // Number of items to use as context
}

message AskResponse {
  string answer = 1;
  repeated SearchResult sources = 2;
  float confidence = 3;
  AskMetadata metadata = 4;
}

message AskMetadata {
  int64 total_time_ms = 1;
  int64 search_time_ms = 2;
  int64 generation_time_ms = 3;
  string model_used = 4;
}
```

## Query Parser

```go
// internal/parser/parser.go

type QueryParser struct {
    stopWords    map[string]bool
    entityTagger *EntityTagger
}

type ParsedQuery struct {
    Original     string
    Normalized   string
    Keywords     []string
    Entities     []DetectedEntity
    Intent       QueryIntent
    DateRange    *DateRange
    Filters      map[string][]string
}

type QueryIntent string

const (
    IntentSearch     QueryIntent = "search"
    IntentQuestion   QueryIntent = "question"
    IntentFindPerson QueryIntent = "find_person"
    IntentTimeline   QueryIntent = "timeline"
)

func (p *QueryParser) Parse(query string) (*ParsedQuery, error) {
    parsed := &ParsedQuery{
        Original: query,
    }

    // Normalize query
    parsed.Normalized = p.normalize(query)

    // Extract keywords (remove stop words)
    parsed.Keywords = p.extractKeywords(parsed.Normalized)

    // Detect named entities
    parsed.Entities = p.entityTagger.Tag(query)

    // Determine intent
    parsed.Intent = p.detectIntent(query)

    // Extract date references
    parsed.DateRange = p.extractDateRange(query)

    // Extract implicit filters
    parsed.Filters = p.extractFilters(query, parsed.Entities)

    return parsed, nil
}

func (p *QueryParser) detectIntent(query string) QueryIntent {
    lower := strings.ToLower(query)

    // Question patterns
    questionWords := []string{"what", "who", "when", "where", "why", "how"}
    for _, word := range questionWords {
        if strings.HasPrefix(lower, word) {
            return IntentQuestion
        }
    }

    // Timeline patterns
    timelinePatterns := []string{"timeline", "history of", "when did", "sequence"}
    for _, pattern := range timelinePatterns {
        if strings.Contains(lower, pattern) {
            return IntentTimeline
        }
    }

    // Person search patterns
    if strings.Contains(lower, "find person") || strings.Contains(lower, "who is") {
        return IntentFindPerson
    }

    return IntentSearch
}

func (p *QueryParser) extractDateRange(query string) *DateRange {
    lower := strings.ToLower(query)

    // Relative dates
    if strings.Contains(lower, "last week") {
        now := time.Now()
        return &DateRange{
            From: now.AddDate(0, 0, -7),
            To:   now,
        }
    }
    if strings.Contains(lower, "last month") {
        now := time.Now()
        return &DateRange{
            From: now.AddDate(0, -1, 0),
            To:   now,
        }
    }
    if strings.Contains(lower, "yesterday") {
        now := time.Now()
        return &DateRange{
            From: now.AddDate(0, 0, -1),
            To:   now,
        }
    }

    // TODO: Parse absolute dates

    return nil
}
```

## Query Embedding

```go
// internal/embedding/embedder.go

type QueryEmbedder struct {
    embeddingClient embeddingpb.EmbeddingServiceClient
    cache           *EmbeddingCache
}

func (e *QueryEmbedder) Embed(ctx context.Context, query string) ([]float32, error) {
    // Check cache first
    if cached, ok := e.cache.Get(query); ok {
        return cached, nil
    }

    // Request embedding from Embedding Pipeline service
    resp, err := e.embeddingClient.GetEmbedding(ctx, &embeddingpb.GetEmbeddingRequest{
        Text: query,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get embedding: %w", err)
    }

    // Cache for future use
    e.cache.Set(query, resp.Embedding)

    return resp.Embedding, nil
}
```

## Hybrid Search

```go
// internal/search/hybrid.go

type HybridSearcher struct {
    db           *pgxpool.Pool
    embedder     *QueryEmbedder
    rrfK         float64  // RRF constant (default 60)
}

func (s *HybridSearcher) Search(ctx context.Context, req *SearchRequest, parsed *ParsedQuery) (*SearchResponse, error) {
    start := time.Now()

    // Parallel BM25 and vector search
    var bm25Results, vectorResults []RankedResult
    var bm25Time, vectorTime time.Duration
    var wg sync.WaitGroup
    var mu sync.Mutex

    // BM25 search
    wg.Add(1)
    go func() {
        defer wg.Done()
        bm25Start := time.Now()
        results, err := s.bm25Search(ctx, req, parsed)
        bm25Time = time.Since(bm25Start)
        if err != nil {
            slog.Error("BM25 search failed", "error", err)
            return
        }
        mu.Lock()
        bm25Results = results
        mu.Unlock()
    }()

    // Vector search
    wg.Add(1)
    go func() {
        defer wg.Done()
        vectorStart := time.Now()
        results, err := s.vectorSearch(ctx, req, parsed)
        vectorTime = time.Since(vectorStart)
        if err != nil {
            slog.Error("Vector search failed", "error", err)
            return
        }
        mu.Lock()
        vectorResults = results
        mu.Unlock()
    }()

    wg.Wait()

    // Merge with RRF
    rankStart := time.Now()
    merged := s.rrfFusion(bm25Results, vectorResults, req.Options.KeywordWeight, req.Options.SemanticWeight)
    rankTime := time.Since(rankStart)

    // Apply filters
    filtered := s.applyFilters(merged, req.Filters)

    // Paginate
    paginated := s.paginate(filtered, int(req.Offset), int(req.Limit))

    // Enrich results
    enriched := s.enrichResults(ctx, paginated)

    return &SearchResponse{
        Results:    enriched,
        TotalCount: int32(len(filtered)),
        Metadata: &SearchMetadata{
            QueryTimeMs:   time.Since(start).Milliseconds(),
            Bm25TimeMs:    bm25Time.Milliseconds(),
            VectorTimeMs:  vectorTime.Milliseconds(),
            RankingTimeMs: rankTime.Milliseconds(),
            ParsedQuery:   parsed.Normalized,
        },
    }, nil
}

func (s *HybridSearcher) bm25Search(ctx context.Context, req *SearchRequest, parsed *ParsedQuery) ([]RankedResult, error) {
    // PostgreSQL full-text search with ts_rank
    query := `
        SELECT
            s.id,
            s.content_type,
            s.title,
            s.content,
            s.metadata,
            s.created_at,
            ts_rank_cd(s.search_vector, plainto_tsquery('english', $2)) as rank
        FROM sources s
        WHERE s.tenant_id = $1
          AND s.search_vector @@ plainto_tsquery('english', $2)
          AND s.deleted_at IS NULL
        ORDER BY rank DESC
        LIMIT 100
    `

    rows, err := s.db.Query(ctx, query, req.TenantId, parsed.Normalized)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []RankedResult
    for rows.Next() {
        var r RankedResult
        if err := rows.Scan(&r.ID, &r.ContentType, &r.Title, &r.Content, &r.Metadata, &r.Timestamp, &r.BM25Score); err != nil {
            return nil, err
        }
        results = append(results, r)
    }

    return results, nil
}

func (s *HybridSearcher) vectorSearch(ctx context.Context, req *SearchRequest, parsed *ParsedQuery) ([]RankedResult, error) {
    // Get query embedding
    embedding, err := s.embedder.Embed(ctx, parsed.Normalized)
    if err != nil {
        return nil, err
    }

    // pgvector similarity search
    query := `
        SELECT
            s.id,
            s.content_type,
            s.title,
            s.content,
            s.metadata,
            s.created_at,
            1 - (e.embedding <=> $2::vector) as similarity
        FROM embeddings e
        JOIN sources s ON e.entity_id = s.id AND e.entity_type = 'source'
        WHERE s.tenant_id = $1
          AND s.deleted_at IS NULL
          AND 1 - (e.embedding <=> $2::vector) > 0.5
        ORDER BY e.embedding <=> $2::vector
        LIMIT 100
    `

    rows, err := s.db.Query(ctx, query, req.TenantId, pgvector.NewVector(embedding))
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []RankedResult
    for rows.Next() {
        var r RankedResult
        if err := rows.Scan(&r.ID, &r.ContentType, &r.Title, &r.Content, &r.Metadata, &r.Timestamp, &r.VectorScore); err != nil {
            return nil, err
        }
        results = append(results, r)
    }

    return results, nil
}

func (s *HybridSearcher) rrfFusion(bm25, vector []RankedResult, keywordWeight, semanticWeight float64) []RankedResult {
    // Default weights
    if keywordWeight == 0 && semanticWeight == 0 {
        keywordWeight = 0.5
        semanticWeight = 0.5
    }

    // Build rank maps
    bm25Ranks := make(map[string]int)
    for i, r := range bm25 {
        bm25Ranks[r.ID] = i + 1
    }

    vectorRanks := make(map[string]int)
    for i, r := range vector {
        vectorRanks[r.ID] = i + 1
    }

    // Collect all unique IDs
    allIDs := make(map[string]bool)
    resultMap := make(map[string]RankedResult)
    for _, r := range bm25 {
        allIDs[r.ID] = true
        resultMap[r.ID] = r
    }
    for _, r := range vector {
        allIDs[r.ID] = true
        if _, exists := resultMap[r.ID]; !exists {
            resultMap[r.ID] = r
        }
    }

    // Calculate RRF scores
    type scoredResult struct {
        result   RankedResult
        rrfScore float64
    }

    var scored []scoredResult
    for id := range allIDs {
        result := resultMap[id]

        bm25Score := 0.0
        if rank, ok := bm25Ranks[id]; ok {
            bm25Score = 1.0 / (s.rrfK + float64(rank))
        }

        vectorScore := 0.0
        if rank, ok := vectorRanks[id]; ok {
            vectorScore = 1.0 / (s.rrfK + float64(rank))
        }

        rrfScore := keywordWeight*bm25Score + semanticWeight*vectorScore

        result.RelevanceScore = float32(rrfScore)
        scored = append(scored, scoredResult{result: result, rrfScore: rrfScore})
    }

    // Sort by RRF score
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].rrfScore > scored[j].rrfScore
    })

    // Extract results
    results := make([]RankedResult, len(scored))
    for i, s := range scored {
        results[i] = s.result
    }

    return results
}
```

## Cache Layer

```go
// internal/cache/cache.go

type SearchCache struct {
    redis *redis.Client
    ttl   time.Duration
}

func (c *SearchCache) Get(ctx context.Context, key string) (*SearchResponse, bool) {
    data, err := c.redis.Get(ctx, "search:"+key).Bytes()
    if err != nil {
        return nil, false
    }

    var resp SearchResponse
    if err := proto.Unmarshal(data, &resp); err != nil {
        return nil, false
    }

    return &resp, true
}

func (c *SearchCache) Set(ctx context.Context, key string, resp *SearchResponse) error {
    data, err := proto.Marshal(resp)
    if err != nil {
        return err
    }

    return c.redis.Set(ctx, "search:"+key, data, c.ttl).Err()
}

func (c *SearchCache) Key(req *SearchRequest) string {
    h := sha256.New()
    h.Write([]byte(req.TenantId))
    h.Write([]byte(req.Query))

    // Include filters in cache key
    if req.Filters != nil {
        data, _ := json.Marshal(req.Filters)
        h.Write(data)
    }

    return hex.EncodeToString(h.Sum(nil))[:16]
}
```

## Configuration

```yaml
# config/search.yaml

server:
  grpc_port: 8081
  metrics_port: 9081

database:
  host: "dev02"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 30

embedding:
  service_addr: "localhost:8001"
  timeout: "5s"

search:
  rrf_k: 60                    # RRF constant
  default_limit: 20
  max_limit: 100
  min_similarity: 0.5          # Vector similarity threshold
  snippet_length: 200

cache:
  enabled: true
  redis_address: "dev02:6379"
  ttl: "5m"
  embedding_ttl: "1h"

analytics:
  enabled: true
  sample_rate: 1.0            # Log all searches

logging:
  level: "info"
  format: "json"
```

## Implementation Structure

```
services/search/
├── cmd/
│   └── search/
│       └── main.go
├── internal/
│   ├── parser/
│   │   ├── parser.go
│   │   └── entity_tagger.go
│   ├── embedding/
│   │   └── embedder.go
│   ├── search/
│   │   ├── hybrid.go
│   │   ├── bm25.go
│   │   └── vector.go
│   ├── ranking/
│   │   └── rrf.go
│   ├── cache/
│   │   └── cache.go
│   ├── analytics/
│   │   └── tracker.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── search/
│           └── v1/
│               └── search.proto
└── go.mod
```
