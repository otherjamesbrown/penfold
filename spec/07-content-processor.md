# Content Processor Specification

## Overview

The Content Processor handles AI-powered content analysis including entity extraction, categorization, summarization, and metadata enrichment. It receives processing jobs from the Event Router and coordinates with the AI Coordinator for model selection.

## Status: Planned (Phase 3)

## Responsibilities

1. **Entity Extraction**: People, projects, organizations, decisions, dates, action items
2. **Categorization**: Content type classification with confidence scoring
3. **Summarization**: Generate concise summaries at multiple lengths
4. **Metadata Enrichment**: Extract structured data from unstructured content
5. **Chunking**: Split long content for embedding and processing
6. **Job Processing**: Handle processing jobs from Event Router
7. **Confidence Scoring**: Rate AI output confidence for review routing

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Content Processor                                  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                      gRPC Server (:8083)                        │    │
│  └──────────────────────────┬─────────────────────────────────────┘    │
│                             │                                           │
│  ┌──────────────────────────┼──────────────────────────────────────┐   │
│  │                          ▼                                       │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │   Entity     │  │ Categorizer  │  │ Summarizer   │          │   │
│  │  │  Extractor   │  │              │  │              │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                 │                 │                   │   │
│  │  ┌──────┴─────────────────┴─────────────────┴───────┐          │   │
│  │  │                  Processing Pipeline              │          │   │
│  │  │  Preprocess → Chunk → Analyze → Score → Store    │          │   │
│  │  └──────────────────────────┬───────────────────────┘          │   │
│  │                             │                                   │   │
│  └─────────────────────────────┼───────────────────────────────────┘   │
│                                │                                        │
│         ┌──────────────────────┼──────────────────────┐                │
│         ▼                      ▼                      ▼                 │
│  ┌───────────────┐      ┌───────────────┐      ┌───────────────┐      │
│  │      AI       │      │   Embedding   │      │  PostgreSQL   │      │
│  │  Coordinator  │      │   Pipeline    │      │               │      │
│  │    (:8085)    │      │    (:8001)    │      │               │      │
│  └───────────────┘      └───────────────┘      └───────────────┘      │
└─────────────────────────────────────────────────────────────────────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/content/v1/content.proto

syntax = "proto3";
package content.v1;

import "google/protobuf/timestamp.proto";
import "common/v1/common.proto";

service ContentService {
  // Full processing pipeline
  rpc ProcessContent(ProcessContentRequest) returns (ProcessContentResponse);
  rpc ProcessBatch(ProcessBatchRequest) returns (ProcessBatchResponse);

  // Individual operations
  rpc ExtractEntities(ExtractEntitiesRequest) returns (ExtractEntitiesResponse);
  rpc Categorize(CategorizeRequest) returns (CategorizeResponse);
  rpc Summarize(SummarizeRequest) returns (SummarizeResponse);
  rpc ExtractMetadata(ExtractMetadataRequest) returns (ExtractMetadataResponse);

  // Chunking
  rpc ChunkContent(ChunkContentRequest) returns (ChunkContentResponse);

  // Job status
  rpc GetJobStatus(GetJobStatusRequest) returns (GetJobStatusResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

// Process content request
message ProcessContentRequest {
  string tenant_id = 1;
  string source_id = 2;
  string source_type = 3;       // email, meeting, document
  string content = 4;
  string title = 5;
  map<string, string> metadata = 6;
  ProcessingOptions options = 7;
}

message ProcessingOptions {
  bool extract_entities = 1;
  bool categorize = 2;
  bool summarize = 3;
  bool extract_metadata = 4;
  bool generate_embeddings = 5;
  SummaryLength summary_length = 6;
  float min_entity_confidence = 7;
  repeated string entity_types = 8;  // Filter to specific types
}

enum SummaryLength {
  SUMMARY_LENGTH_UNSPECIFIED = 0;
  SUMMARY_LENGTH_BRIEF = 1;      // 1-2 sentences
  SUMMARY_LENGTH_STANDARD = 2;   // 3-5 sentences
  SUMMARY_LENGTH_DETAILED = 3;   // Full paragraph
}

message ProcessContentResponse {
  string job_id = 1;
  ProcessingResult result = 2;
  ProcessingMetadata metadata = 3;
}

message ProcessingResult {
  repeated Entity entities = 1;
  ContentCategory category = 2;
  string summary = 3;
  ExtractedMetadata extracted_metadata = 4;
  float overall_confidence = 5;
  bool requires_review = 6;
}

message ProcessingMetadata {
  int64 processing_time_ms = 1;
  string model_used = 2;
  int32 token_count = 3;
  int32 chunk_count = 4;
  bool escalated_to_cloud = 5;
}

// Entity extraction
message Entity {
  string id = 1;
  EntityType type = 2;
  string value = 3;
  string normalized_value = 4;
  float confidence = 5;
  repeated TextSpan mentions = 6;
  map<string, string> attributes = 7;
  string resolution_id = 8;  // ID of resolved entity in database
}

enum EntityType {
  ENTITY_TYPE_UNSPECIFIED = 0;
  ENTITY_TYPE_PERSON = 1;
  ENTITY_TYPE_ORGANIZATION = 2;
  ENTITY_TYPE_PROJECT = 3;
  ENTITY_TYPE_DATE = 4;
  ENTITY_TYPE_LOCATION = 5;
  ENTITY_TYPE_DECISION = 6;
  ENTITY_TYPE_ACTION_ITEM = 7;
  ENTITY_TYPE_TOPIC = 8;
  ENTITY_TYPE_MONEY = 9;
  ENTITY_TYPE_DEADLINE = 10;
}

message TextSpan {
  int32 start = 1;
  int32 end = 2;
  string text = 3;
}

message ExtractEntitiesRequest {
  string tenant_id = 1;
  string content = 2;
  repeated EntityType types = 3;
  float min_confidence = 4;
  bool resolve_entities = 5;  // Try to match to known entities
}

message ExtractEntitiesResponse {
  repeated Entity entities = 1;
  int64 processing_time_ms = 2;
  string model_used = 3;
}

// Categorization
message ContentCategory {
  string primary_category = 1;
  float primary_confidence = 2;
  repeated CategoryScore secondary_categories = 3;
  repeated string tags = 4;
  Urgency urgency = 5;
  Importance importance = 6;
}

message CategoryScore {
  string category = 1;
  float confidence = 2;
}

enum Urgency {
  URGENCY_UNSPECIFIED = 0;
  URGENCY_NONE = 1;
  URGENCY_LOW = 2;
  URGENCY_MEDIUM = 3;
  URGENCY_HIGH = 4;
  URGENCY_CRITICAL = 5;
}

enum Importance {
  IMPORTANCE_UNSPECIFIED = 0;
  IMPORTANCE_LOW = 1;
  IMPORTANCE_MEDIUM = 2;
  IMPORTANCE_HIGH = 3;
  IMPORTANCE_CRITICAL = 4;
}

message CategorizeRequest {
  string tenant_id = 1;
  string content = 2;
  string title = 3;
  repeated string available_categories = 4;  // Custom categories
}

message CategorizeResponse {
  ContentCategory category = 1;
  int64 processing_time_ms = 2;
  string model_used = 3;
}

// Summarization
message SummarizeRequest {
  string tenant_id = 1;
  string content = 2;
  SummaryLength length = 3;
  string focus = 4;  // Optional: focus area for summary
  bool include_key_points = 5;
  bool include_action_items = 6;
}

message SummarizeResponse {
  string summary = 1;
  repeated string key_points = 2;
  repeated ActionItem action_items = 3;
  int64 processing_time_ms = 4;
  string model_used = 5;
}

message ActionItem {
  string description = 1;
  string assignee = 2;
  string deadline = 3;
  float confidence = 4;
}

// Metadata extraction
message ExtractedMetadata {
  repeated string topics = 1;
  repeated string keywords = 2;
  string language = 3;
  string sentiment = 4;
  float sentiment_score = 5;
  int32 word_count = 6;
  int32 reading_time_seconds = 7;
  repeated Reference references = 8;
  map<string, string> custom = 9;
}

message Reference {
  string type = 1;  // url, email, phone, document
  string value = 2;
  string context = 3;
}

message ExtractMetadataRequest {
  string tenant_id = 1;
  string content = 2;
}

message ExtractMetadataResponse {
  ExtractedMetadata metadata = 1;
  int64 processing_time_ms = 2;
}

// Chunking
message ChunkContentRequest {
  string content = 1;
  ChunkingStrategy strategy = 2;
  int32 max_chunk_size = 3;
  int32 overlap = 4;
}

enum ChunkingStrategy {
  CHUNKING_STRATEGY_UNSPECIFIED = 0;
  CHUNKING_STRATEGY_FIXED_SIZE = 1;
  CHUNKING_STRATEGY_SENTENCE = 2;
  CHUNKING_STRATEGY_PARAGRAPH = 3;
  CHUNKING_STRATEGY_SEMANTIC = 4;
}

message ChunkContentResponse {
  repeated ContentChunk chunks = 1;
}

message ContentChunk {
  int32 index = 1;
  string content = 2;
  int32 start_offset = 3;
  int32 end_offset = 4;
  int32 token_count = 5;
}

// Batch processing
message ProcessBatchRequest {
  string tenant_id = 1;
  repeated BatchItem items = 2;
  ProcessingOptions options = 3;
}

message BatchItem {
  string source_id = 1;
  string source_type = 2;
  string content = 3;
  string title = 4;
  map<string, string> metadata = 5;
}

message ProcessBatchResponse {
  repeated BatchResult results = 1;
  int64 total_processing_time_ms = 2;
}

message BatchResult {
  string source_id = 1;
  bool success = 2;
  ProcessingResult result = 3;
  string error = 4;
}

// Job status
message GetJobStatusRequest {
  string job_id = 1;
}

message GetJobStatusResponse {
  string job_id = 1;
  JobStatus status = 2;
  float progress = 3;
  ProcessingResult result = 4;
  string error = 5;
}

enum JobStatus {
  JOB_STATUS_UNSPECIFIED = 0;
  JOB_STATUS_QUEUED = 1;
  JOB_STATUS_PROCESSING = 2;
  JOB_STATUS_COMPLETED = 3;
  JOB_STATUS_FAILED = 4;
}

// Health
message HealthRequest {}
message HealthResponse {
  bool healthy = 1;
  map<string, ComponentHealth> components = 2;
}

message ComponentHealth {
  bool healthy = 1;
  string status = 2;
  int64 latency_ms = 3;
}
```

## Processing Pipeline

```go
// internal/pipeline/pipeline.go

package pipeline

import (
    "context"
    "fmt"
    "log/slog"
    "time"
)

type Pipeline struct {
    preprocessor  *Preprocessor
    chunker       *Chunker
    entityExtractor *EntityExtractor
    categorizer   *Categorizer
    summarizer    *Summarizer
    metadataExtractor *MetadataExtractor
    aiCoordinator aipb.AICoordinatorServiceClient
    embeddingClient embeddingpb.EmbeddingServiceClient
    db            *pgxpool.Pool
    publisher     *events.Publisher
}

type ProcessingJob struct {
    ID          string
    TenantID    string
    SourceID    string
    SourceType  string
    Content     string
    Title       string
    Metadata    map[string]string
    Options     *ProcessingOptions
    StartedAt   time.Time
}

func (p *Pipeline) Process(ctx context.Context, job *ProcessingJob) (*ProcessingResult, error) {
    start := time.Now()
    result := &ProcessingResult{}
    metadata := &ProcessingMetadata{}

    // Step 1: Preprocess content
    preprocessed, err := p.preprocessor.Process(job.Content, job.SourceType)
    if err != nil {
        return nil, fmt.Errorf("preprocessing failed: %w", err)
    }

    // Step 2: Chunk if needed
    var chunks []ContentChunk
    if len(preprocessed.Text) > p.chunker.MaxSize {
        chunks, err = p.chunker.Chunk(preprocessed.Text, job.Options.ChunkingStrategy)
        if err != nil {
            return nil, fmt.Errorf("chunking failed: %w", err)
        }
        metadata.ChunkCount = int32(len(chunks))
    } else {
        chunks = []ContentChunk{{Content: preprocessed.Text}}
        metadata.ChunkCount = 1
    }

    // Step 3: Extract entities (if requested)
    if job.Options.ExtractEntities {
        entities, err := p.extractEntities(ctx, job, chunks)
        if err != nil {
            slog.Warn("entity extraction failed", "error", err)
        } else {
            result.Entities = entities
        }
    }

    // Step 4: Categorize (if requested)
    if job.Options.Categorize {
        category, err := p.categorize(ctx, job, preprocessed.Text)
        if err != nil {
            slog.Warn("categorization failed", "error", err)
        } else {
            result.Category = category
        }
    }

    // Step 5: Summarize (if requested)
    if job.Options.Summarize {
        summary, err := p.summarize(ctx, job, preprocessed.Text)
        if err != nil {
            slog.Warn("summarization failed", "error", err)
        } else {
            result.Summary = summary
        }
    }

    // Step 6: Extract metadata (if requested)
    if job.Options.ExtractMetadata {
        extracted, err := p.metadataExtractor.Extract(ctx, preprocessed.Text)
        if err != nil {
            slog.Warn("metadata extraction failed", "error", err)
        } else {
            result.ExtractedMetadata = extracted
        }
    }

    // Step 7: Generate embeddings (if requested)
    if job.Options.GenerateEmbeddings {
        for i, chunk := range chunks {
            embedding, err := p.embeddingClient.GetEmbedding(ctx, &embeddingpb.GetEmbeddingRequest{
                Text: chunk.Content,
            })
            if err != nil {
                slog.Warn("embedding generation failed", "chunk", i, "error", err)
                continue
            }
            chunks[i].Embedding = embedding.Embedding
        }
    }

    // Step 8: Calculate overall confidence and determine if review needed
    result.OverallConfidence = p.calculateOverallConfidence(result)
    result.RequiresReview = result.OverallConfidence < 0.8

    // Step 9: Store results
    if err := p.storeResults(ctx, job, result, chunks); err != nil {
        return nil, fmt.Errorf("failed to store results: %w", err)
    }

    // Step 10: Publish events
    p.publishEvents(ctx, job, result)

    metadata.ProcessingTimeMs = time.Since(start).Milliseconds()

    return result, nil
}

func (p *Pipeline) extractEntities(ctx context.Context, job *ProcessingJob, chunks []ContentChunk) ([]*Entity, error) {
    var allEntities []*Entity
    entityMap := make(map[string]*Entity)  // Deduplicate by normalized value

    for _, chunk := range chunks {
        // Request entity extraction from AI Coordinator
        resp, err := p.aiCoordinator.Process(ctx, &aipb.ProcessRequest{
            TenantId: job.TenantID,
            Task:     aipb.Task_TASK_ENTITY_EXTRACTION,
            Content:  chunk.Content,
            Options: &aipb.ProcessOptions{
                MinConfidence: job.Options.MinEntityConfidence,
            },
        })
        if err != nil {
            return nil, err
        }

        // Parse entities from response
        entities, err := p.entityExtractor.ParseResponse(resp.Result)
        if err != nil {
            continue
        }

        // Merge and deduplicate
        for _, entity := range entities {
            key := fmt.Sprintf("%s:%s", entity.Type, entity.NormalizedValue)
            if existing, ok := entityMap[key]; ok {
                // Merge mentions and update confidence
                existing.Mentions = append(existing.Mentions, entity.Mentions...)
                if entity.Confidence > existing.Confidence {
                    existing.Confidence = entity.Confidence
                }
            } else {
                entityMap[key] = entity
            }
        }
    }

    // Resolve entities to known database entries
    for _, entity := range entityMap {
        resolved, err := p.entityExtractor.Resolve(ctx, job.TenantID, entity)
        if err == nil && resolved != nil {
            entity.ResolutionId = resolved.ID
        }
        allEntities = append(allEntities, entity)
    }

    return allEntities, nil
}

func (p *Pipeline) categorize(ctx context.Context, job *ProcessingJob, content string) (*ContentCategory, error) {
    // Use title + snippet for categorization
    text := content
    if job.Title != "" {
        text = job.Title + "\n\n" + content
    }

    // Truncate for categorization (doesn't need full content)
    if len(text) > 2000 {
        text = text[:2000]
    }

    resp, err := p.aiCoordinator.Process(ctx, &aipb.ProcessRequest{
        TenantId: job.TenantID,
        Task:     aipb.Task_TASK_CATEGORIZATION,
        Content:  text,
    })
    if err != nil {
        return nil, err
    }

    return p.categorizer.ParseResponse(resp.Result)
}

func (p *Pipeline) summarize(ctx context.Context, job *ProcessingJob, content string) (string, error) {
    resp, err := p.aiCoordinator.Process(ctx, &aipb.ProcessRequest{
        TenantId: job.TenantID,
        Task:     aipb.Task_TASK_SUMMARIZATION,
        Content:  content,
        Options: &aipb.ProcessOptions{
            SummaryLength: job.Options.SummaryLength,
        },
    })
    if err != nil {
        return "", err
    }

    return resp.Result, nil
}

func (p *Pipeline) calculateOverallConfidence(result *ProcessingResult) float32 {
    var confidences []float32
    var weights []float32

    // Entity confidence (weighted by count)
    if len(result.Entities) > 0 {
        var sum float32
        for _, e := range result.Entities {
            sum += e.Confidence
        }
        confidences = append(confidences, sum/float32(len(result.Entities)))
        weights = append(weights, 0.3)
    }

    // Category confidence
    if result.Category != nil {
        confidences = append(confidences, result.Category.PrimaryConfidence)
        weights = append(weights, 0.4)
    }

    // Summary confidence (assume high if generated)
    if result.Summary != "" {
        confidences = append(confidences, 0.9)
        weights = append(weights, 0.3)
    }

    if len(confidences) == 0 {
        return 0
    }

    var weightedSum, totalWeight float32
    for i, c := range confidences {
        weightedSum += c * weights[i]
        totalWeight += weights[i]
    }

    return weightedSum / totalWeight
}

func (p *Pipeline) publishEvents(ctx context.Context, job *ProcessingJob, result *ProcessingResult) {
    // Content processed event
    p.publisher.Publish(ctx, "content.processed", &events.ContentProcessedEvent{
        SourceID: job.SourceID,
        Result: events.ProcessingResult{
            Summary:    result.Summary,
            Entities:   convertEntities(result.Entities),
            Categories: extractCategories(result.Category),
            Confidence: result.OverallConfidence,
        },
    })

    // Entities extracted event
    if len(result.Entities) > 0 {
        p.publisher.Publish(ctx, "entities.extracted", &events.EntitiesExtractedEvent{
            SourceID:    job.SourceID,
            EntityCount: len(result.Entities),
            Types:       extractEntityTypes(result.Entities),
        })
    }

    // Content categorized event
    if result.Category != nil {
        p.publisher.Publish(ctx, "content.categorized", &events.ContentCategorizedEvent{
            SourceID:   job.SourceID,
            Category:   result.Category.PrimaryCategory,
            Confidence: result.Category.PrimaryConfidence,
        })
    }
}
```

## Entity Extractor

```go
// internal/entity/extractor.go

package entity

import (
    "context"
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
)

type EntityExtractor struct {
    db           *pgxpool.Pool
    patterns     map[EntityType]*regexp.Regexp
    normalizers  map[EntityType]Normalizer
}

type Normalizer func(string) string

func NewEntityExtractor(db *pgxpool.Pool) *EntityExtractor {
    e := &EntityExtractor{
        db:       db,
        patterns: make(map[EntityType]*regexp.Regexp),
        normalizers: make(map[EntityType]Normalizer),
    }

    // Date patterns
    e.patterns[EntityTypeDat] = regexp.MustCompile(
        `(?i)\b(\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|` +
        `\d{4}[/-]\d{1,2}[/-]\d{1,2}|` +
        `(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+\d{1,2},?\s+\d{4}|` +
        `\d{1,2}\s+(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\s+\d{4}|` +
        `today|tomorrow|yesterday|next\s+(?:week|month|monday|tuesday|wednesday|thursday|friday))\b`,
    )

    // Money patterns
    e.patterns[EntityTypeMoney] = regexp.MustCompile(
        `(?i)\$[\d,]+(?:\.\d{2})?(?:\s*(?:million|billion|thousand|k|m|b))?|` +
        `[\d,]+(?:\.\d{2})?\s*(?:dollars|usd|eur|gbp)|` +
        `(?:£|€)[\d,]+(?:\.\d{2})?`,
    )

    // Email patterns
    e.patterns[EntityTypeReference] = regexp.MustCompile(
        `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
    )

    // Person name normalizer
    e.normalizers[EntityTypePerson] = func(s string) string {
        parts := strings.Fields(strings.TrimSpace(s))
        var normalized []string
        for _, part := range parts {
            if len(part) > 0 {
                normalized = append(normalized, strings.Title(strings.ToLower(part)))
            }
        }
        return strings.Join(normalized, " ")
    }

    return e
}

func (e *EntityExtractor) ParseResponse(aiResponse string) ([]*Entity, error) {
    // Parse structured JSON response from AI
    var response struct {
        Entities []struct {
            Type       string  `json:"type"`
            Value      string  `json:"value"`
            Confidence float32 `json:"confidence"`
            Start      int     `json:"start"`
            End        int     `json:"end"`
            Attributes map[string]string `json:"attributes"`
        } `json:"entities"`
    }

    if err := json.Unmarshal([]byte(aiResponse), &response); err != nil {
        return nil, fmt.Errorf("failed to parse AI response: %w", err)
    }

    var entities []*Entity
    for _, raw := range response.Entities {
        entityType := parseEntityType(raw.Type)

        entity := &Entity{
            ID:         generateID(),
            Type:       entityType,
            Value:      raw.Value,
            Confidence: raw.Confidence,
            Attributes: raw.Attributes,
            Mentions: []*TextSpan{{
                Start: int32(raw.Start),
                End:   int32(raw.End),
                Text:  raw.Value,
            }},
        }

        // Normalize value
        if normalizer, ok := e.normalizers[entityType]; ok {
            entity.NormalizedValue = normalizer(raw.Value)
        } else {
            entity.NormalizedValue = strings.TrimSpace(raw.Value)
        }

        entities = append(entities, entity)
    }

    return entities, nil
}

func (e *EntityExtractor) Resolve(ctx context.Context, tenantID string, entity *Entity) (*ResolvedEntity, error) {
    switch entity.Type {
    case EntityTypePerson:
        return e.resolvePerson(ctx, tenantID, entity)
    case EntityTypeProject:
        return e.resolveProject(ctx, tenantID, entity)
    case EntityTypeOrganization:
        return e.resolveOrganization(ctx, tenantID, entity)
    default:
        return nil, nil
    }
}

func (e *EntityExtractor) resolvePerson(ctx context.Context, tenantID string, entity *Entity) (*ResolvedEntity, error) {
    // Try exact match first
    query := `
        SELECT id, name, email
        FROM persons
        WHERE tenant_id = $1
          AND (LOWER(name) = LOWER($2) OR LOWER(email) = LOWER($2))
        LIMIT 1
    `

    var resolved ResolvedEntity
    err := e.db.QueryRow(ctx, query, tenantID, entity.NormalizedValue).Scan(
        &resolved.ID, &resolved.Name, &resolved.Email,
    )
    if err == nil {
        return &resolved, nil
    }

    // Try fuzzy match
    fuzzyQuery := `
        SELECT id, name, email,
               similarity(LOWER(name), LOWER($2)) as sim
        FROM persons
        WHERE tenant_id = $1
          AND similarity(LOWER(name), LOWER($2)) > 0.5
        ORDER BY sim DESC
        LIMIT 1
    `

    err = e.db.QueryRow(ctx, fuzzyQuery, tenantID, entity.NormalizedValue).Scan(
        &resolved.ID, &resolved.Name, &resolved.Email, &resolved.Similarity,
    )
    if err == nil {
        return &resolved, nil
    }

    return nil, fmt.Errorf("no matching person found")
}

type ResolvedEntity struct {
    ID         string
    Name       string
    Email      string
    Similarity float32
}
```

## Categorizer

```go
// internal/categorizer/categorizer.go

package categorizer

import (
    "encoding/json"
    "fmt"
)

type Categorizer struct {
    defaultCategories []string
}

var DefaultCategories = []string{
    "project_update",
    "meeting_notes",
    "decision",
    "action_required",
    "information",
    "question",
    "feedback",
    "announcement",
    "follow_up",
    "scheduling",
    "financial",
    "technical",
    "personal",
    "external",
}

func NewCategorizer() *Categorizer {
    return &Categorizer{
        defaultCategories: DefaultCategories,
    }
}

func (c *Categorizer) ParseResponse(aiResponse string) (*ContentCategory, error) {
    var response struct {
        PrimaryCategory string  `json:"primary_category"`
        Confidence      float32 `json:"confidence"`
        Secondary       []struct {
            Category   string  `json:"category"`
            Confidence float32 `json:"confidence"`
        } `json:"secondary"`
        Tags      []string `json:"tags"`
        Urgency   string   `json:"urgency"`
        Importance string  `json:"importance"`
    }

    if err := json.Unmarshal([]byte(aiResponse), &response); err != nil {
        return nil, fmt.Errorf("failed to parse categorization response: %w", err)
    }

    category := &ContentCategory{
        PrimaryCategory:   response.PrimaryCategory,
        PrimaryConfidence: response.Confidence,
        Tags:              response.Tags,
        Urgency:           parseUrgency(response.Urgency),
        Importance:        parseImportance(response.Importance),
    }

    for _, sec := range response.Secondary {
        category.SecondaryCategories = append(category.SecondaryCategories, &CategoryScore{
            Category:   sec.Category,
            Confidence: sec.Confidence,
        })
    }

    return category, nil
}

func (c *Categorizer) BuildPrompt(content, title string, customCategories []string) string {
    categories := c.defaultCategories
    if len(customCategories) > 0 {
        categories = customCategories
    }

    return fmt.Sprintf(`Categorize the following content.

Available categories: %v

Content title: %s

Content:
%s

Respond in JSON format:
{
  "primary_category": "category_name",
  "confidence": 0.95,
  "secondary": [{"category": "other_category", "confidence": 0.3}],
  "tags": ["tag1", "tag2"],
  "urgency": "none|low|medium|high|critical",
  "importance": "low|medium|high|critical"
}`, categories, title, content)
}

func parseUrgency(s string) Urgency {
    switch s {
    case "low":
        return Urgency_URGENCY_LOW
    case "medium":
        return Urgency_URGENCY_MEDIUM
    case "high":
        return Urgency_URGENCY_HIGH
    case "critical":
        return Urgency_URGENCY_CRITICAL
    default:
        return Urgency_URGENCY_NONE
    }
}

func parseImportance(s string) Importance {
    switch s {
    case "low":
        return Importance_IMPORTANCE_LOW
    case "medium":
        return Importance_IMPORTANCE_MEDIUM
    case "high":
        return Importance_IMPORTANCE_HIGH
    case "critical":
        return Importance_IMPORTANCE_CRITICAL
    default:
        return Importance_IMPORTANCE_LOW
    }
}
```

## Chunker

```go
// internal/chunker/chunker.go

package chunker

import (
    "strings"
    "unicode"
)

type Chunker struct {
    MaxSize  int
    Overlap  int
    strategy ChunkingStrategy
}

func NewChunker(maxSize, overlap int, strategy ChunkingStrategy) *Chunker {
    return &Chunker{
        MaxSize:  maxSize,
        Overlap:  overlap,
        strategy: strategy,
    }
}

func (c *Chunker) Chunk(content string, strategy ChunkingStrategy) ([]ContentChunk, error) {
    if strategy == ChunkingStrategy_CHUNKING_STRATEGY_UNSPECIFIED {
        strategy = c.strategy
    }

    switch strategy {
    case ChunkingStrategy_CHUNKING_STRATEGY_FIXED_SIZE:
        return c.chunkFixedSize(content)
    case ChunkingStrategy_CHUNKING_STRATEGY_SENTENCE:
        return c.chunkBySentence(content)
    case ChunkingStrategy_CHUNKING_STRATEGY_PARAGRAPH:
        return c.chunkByParagraph(content)
    case ChunkingStrategy_CHUNKING_STRATEGY_SEMANTIC:
        return c.chunkSemantic(content)
    default:
        return c.chunkFixedSize(content)
    }
}

func (c *Chunker) chunkFixedSize(content string) ([]ContentChunk, error) {
    var chunks []ContentChunk
    runes := []rune(content)

    for i := 0; i < len(runes); {
        end := i + c.MaxSize
        if end > len(runes) {
            end = len(runes)
        }

        // Try to break at word boundary
        if end < len(runes) {
            for j := end; j > i+c.MaxSize/2; j-- {
                if unicode.IsSpace(runes[j]) {
                    end = j
                    break
                }
            }
        }

        chunkContent := string(runes[i:end])
        chunks = append(chunks, ContentChunk{
            Index:       int32(len(chunks)),
            Content:     chunkContent,
            StartOffset: int32(i),
            EndOffset:   int32(end),
            TokenCount:  estimateTokens(chunkContent),
        })

        // Move forward with overlap
        i = end - c.Overlap
        if i < 0 {
            i = end
        }
    }

    return chunks, nil
}

func (c *Chunker) chunkBySentence(content string) ([]ContentChunk, error) {
    sentences := splitSentences(content)
    var chunks []ContentChunk
    var currentChunk strings.Builder
    var startOffset int
    currentOffset := 0

    for _, sentence := range sentences {
        sentenceLen := len(sentence)

        if currentChunk.Len()+sentenceLen > c.MaxSize && currentChunk.Len() > 0 {
            // Save current chunk
            chunkContent := currentChunk.String()
            chunks = append(chunks, ContentChunk{
                Index:       int32(len(chunks)),
                Content:     chunkContent,
                StartOffset: int32(startOffset),
                EndOffset:   int32(currentOffset),
                TokenCount:  estimateTokens(chunkContent),
            })

            // Start new chunk with overlap (last sentence)
            currentChunk.Reset()
            startOffset = currentOffset - len(sentence)
        }

        currentChunk.WriteString(sentence)
        currentOffset += sentenceLen
    }

    // Add remaining content
    if currentChunk.Len() > 0 {
        chunkContent := currentChunk.String()
        chunks = append(chunks, ContentChunk{
            Index:       int32(len(chunks)),
            Content:     chunkContent,
            StartOffset: int32(startOffset),
            EndOffset:   int32(currentOffset),
            TokenCount:  estimateTokens(chunkContent),
        })
    }

    return chunks, nil
}

func (c *Chunker) chunkByParagraph(content string) ([]ContentChunk, error) {
    paragraphs := strings.Split(content, "\n\n")
    var chunks []ContentChunk
    var currentChunk strings.Builder
    var startOffset int
    currentOffset := 0

    for _, para := range paragraphs {
        para = strings.TrimSpace(para)
        if para == "" {
            currentOffset += 2  // Account for \n\n
            continue
        }

        paraLen := len(para) + 2  // Include \n\n

        if currentChunk.Len()+paraLen > c.MaxSize && currentChunk.Len() > 0 {
            chunkContent := strings.TrimSpace(currentChunk.String())
            chunks = append(chunks, ContentChunk{
                Index:       int32(len(chunks)),
                Content:     chunkContent,
                StartOffset: int32(startOffset),
                EndOffset:   int32(currentOffset),
                TokenCount:  estimateTokens(chunkContent),
            })

            currentChunk.Reset()
            startOffset = currentOffset
        }

        if currentChunk.Len() > 0 {
            currentChunk.WriteString("\n\n")
        }
        currentChunk.WriteString(para)
        currentOffset += paraLen
    }

    if currentChunk.Len() > 0 {
        chunkContent := strings.TrimSpace(currentChunk.String())
        chunks = append(chunks, ContentChunk{
            Index:       int32(len(chunks)),
            Content:     chunkContent,
            StartOffset: int32(startOffset),
            EndOffset:   int32(currentOffset),
            TokenCount:  estimateTokens(chunkContent),
        })
    }

    return chunks, nil
}

func (c *Chunker) chunkSemantic(content string) ([]ContentChunk, error) {
    // Semantic chunking: try to keep related content together
    // Use paragraph boundaries, but also consider:
    // - Headers (lines ending with :)
    // - List items
    // - Code blocks

    // For now, fall back to paragraph chunking
    // TODO: Implement proper semantic analysis
    return c.chunkByParagraph(content)
}

func splitSentences(text string) []string {
    // Simple sentence splitting
    var sentences []string
    var current strings.Builder

    for i, r := range text {
        current.WriteRune(r)

        // Check for sentence boundaries
        if r == '.' || r == '!' || r == '?' {
            // Look ahead to verify it's a sentence end
            if i+1 < len(text) {
                next := rune(text[i+1])
                if unicode.IsSpace(next) || next == '\n' {
                    sentences = append(sentences, current.String())
                    current.Reset()
                }
            } else {
                sentences = append(sentences, current.String())
                current.Reset()
            }
        }
    }

    if current.Len() > 0 {
        sentences = append(sentences, current.String())
    }

    return sentences
}

func estimateTokens(text string) int32 {
    // Rough estimate: ~4 characters per token for English
    return int32(len(text) / 4)
}
```

## Configuration

```yaml
# config/content-processor.yaml

server:
  grpc_port: 8083
  metrics_port: 9083

processing:
  max_content_size: 100000     # Characters
  default_chunk_size: 4000     # Characters
  chunk_overlap: 200
  default_chunking_strategy: "paragraph"
  min_entity_confidence: 0.6
  require_review_threshold: 0.8

ai_coordinator:
  address: "localhost:8085"
  timeout: "60s"

embedding:
  address: "localhost:8001"
  timeout: "10s"

categorization:
  default_categories:
    - project_update
    - meeting_notes
    - decision
    - action_required
    - information
    - question

entity_extraction:
  types:
    - person
    - organization
    - project
    - date
    - decision
    - action_item
  resolve_to_known: true
  fuzzy_match_threshold: 0.5

database:
  host: "dev02"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

redis:
  address: "dev02:6379"

logging:
  level: "info"
  format: "json"
```

## Database Schema

```sql
-- Processed content results
CREATE TABLE content_processing_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    source_id UUID NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    summary TEXT,
    primary_category VARCHAR(100),
    category_confidence FLOAT,
    urgency VARCHAR(20),
    importance VARCHAR(20),
    overall_confidence FLOAT,
    requires_review BOOLEAN DEFAULT false,
    reviewed_at TIMESTAMPTZ,
    reviewed_by UUID,
    processing_time_ms INTEGER,
    model_used VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_processing_results_source ON content_processing_results(source_id);
CREATE INDEX idx_processing_results_review ON content_processing_results(requires_review, created_at);

-- Extracted entities
CREATE TABLE extracted_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    source_id UUID NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    value TEXT NOT NULL,
    normalized_value TEXT NOT NULL,
    confidence FLOAT NOT NULL,
    resolution_id UUID,  -- Link to resolved person/project/org
    attributes JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_extracted_entities_source ON extracted_entities(source_id);
CREATE INDEX idx_extracted_entities_type ON extracted_entities(tenant_id, entity_type);
CREATE INDEX idx_extracted_entities_normalized ON extracted_entities(tenant_id, normalized_value);

-- Entity mentions (text spans)
CREATE TABLE entity_mentions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES extracted_entities(id) ON DELETE CASCADE,
    source_id UUID NOT NULL,
    start_offset INTEGER NOT NULL,
    end_offset INTEGER NOT NULL,
    text TEXT NOT NULL
);

CREATE INDEX idx_entity_mentions_entity ON entity_mentions(entity_id);

-- Content chunks (for embeddings)
CREATE TABLE content_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    source_id UUID NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    start_offset INTEGER NOT NULL,
    end_offset INTEGER NOT NULL,
    token_count INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_content_chunks_source ON content_chunks(source_id);

-- Processing jobs
CREATE TABLE content_processing_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    source_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    options JSONB DEFAULT '{}',
    result JSONB,
    error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_processing_jobs_status ON content_processing_jobs(status, created_at);
```

## Implementation Structure

```
services/content-processor/
├── cmd/
│   └── content-processor/
│       └── main.go
├── internal/
│   ├── pipeline/
│   │   ├── pipeline.go
│   │   └── preprocessor.go
│   ├── entity/
│   │   ├── extractor.go
│   │   ├── resolver.go
│   │   └── patterns.go
│   ├── categorizer/
│   │   ├── categorizer.go
│   │   └── prompts.go
│   ├── summarizer/
│   │   ├── summarizer.go
│   │   └── prompts.go
│   ├── chunker/
│   │   ├── chunker.go
│   │   └── strategies.go
│   ├── metadata/
│   │   └── extractor.go
│   ├── service/
│   │   └── grpc.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── content/
│           └── v1/
│               └── content.proto
└── go.mod
```

## Events Published

| Event | Trigger | Payload |
|-------|---------|---------|
| `content.processed` | Processing complete | Summary, Entities, Categories, Confidence |
| `entities.extracted` | Entities found | EntityCount, Types |
| `content.categorized` | Category assigned | Category, Confidence |
| `content.requires_review` | Low confidence | SourceID, Confidence, Reasons |
