# Relationship Discovery Specification

## Overview

The Relationship Discovery service automatically extracts and manages relationships between entities (people, projects, organizations, decisions) with multi-factor confidence scoring, conflict resolution, lifecycle management, and network analysis capabilities.

## Status: Planned (Phase 4)

## Responsibilities

1. **Extraction**: Discover relationships from content using AI and patterns
2. **Confidence Scoring**: Multi-factor confidence calculation with evidence tracking
3. **Conflict Resolution**: Handle contradictory or duplicate relationships
4. **Lifecycle Management**: Track relationship state changes over time
5. **Network Analysis**: Build and analyze collaboration graphs
6. **Validation**: User confirmation and feedback integration

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       Relationship Discovery                              │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                      gRPC Server (:8084)                        │    │
│  └──────────────────────────┬─────────────────────────────────────┘    │
│                             │                                           │
│  ┌──────────────────────────┼──────────────────────────────────────┐   │
│  │                          ▼                                       │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │ Relationship │  │  Confidence  │  │   Conflict   │          │   │
│  │  │  Extractor   │  │   Scorer     │  │  Resolver    │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                 │                 │                   │   │
│  │         └─────────────────┼─────────────────┘                   │   │
│  │                           ▼                                      │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │              Lifecycle Manager                           │   │   │
│  │  │    (state transitions, history tracking)                │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  │                           │                                      │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │                Network Analyzer                          │   │   │
│  │  │        (clusters, hubs, paths, metrics)                 │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  └──────────────────────────┬──────────────────────────────────────┘   │
│                             │                                           │
│         ┌───────────────────┼───────────────────┐                      │
│         ▼                   ▼                   ▼                       │
│  ┌───────────┐       ┌───────────┐       ┌───────────┐                │
│  │PostgreSQL │       │    AI     │       │   Redis   │                │
│  │  (graph,  │       │Coordinator│       │  (cache)  │                │
│  │ evidence) │       │  (:8085)  │       │           │                │
│  └───────────┘       └───────────┘       └───────────┘                │
└─────────────────────────────────────────────────────────────────────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/relationship/v1/relationship.proto

syntax = "proto3";
package relationship.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";

service RelationshipService {
  // Discovery
  rpc DiscoverRelationships(DiscoverRelationshipsRequest) returns (DiscoverRelationshipsResponse);
  rpc DiscoverFromContent(DiscoverFromContentRequest) returns (DiscoverFromContentResponse);

  // Query
  rpc GetRelationship(GetRelationshipRequest) returns (Relationship);
  rpc ListRelationships(ListRelationshipsRequest) returns (ListRelationshipsResponse);
  rpc GetRelationshipsByEntity(GetRelationshipsByEntityRequest) returns (GetRelationshipsByEntityResponse);
  rpc SearchRelationships(SearchRelationshipsRequest) returns (SearchRelationshipsResponse);

  // Graph operations
  rpc GetRelationshipGraph(GetRelationshipGraphRequest) returns (RelationshipGraph);
  rpc FindPath(FindPathRequest) returns (FindPathResponse);
  rpc GetNeighbors(GetNeighborsRequest) returns (GetNeighborsResponse);

  // Validation
  rpc ValidateRelationship(ValidateRelationshipRequest) returns (ValidateRelationshipResponse);
  rpc RejectRelationship(RejectRelationshipRequest) returns (google.protobuf.Empty);
  rpc MergeRelationships(MergeRelationshipsRequest) returns (MergeRelationshipsResponse);

  // Conflict resolution
  rpc DetectConflicts(DetectConflictsRequest) returns (DetectConflictsResponse);
  rpc ResolveConflict(ResolveConflictRequest) returns (ResolveConflictResponse);
  rpc GetPendingConflicts(GetPendingConflictsRequest) returns (GetPendingConflictsResponse);

  // Network analysis
  rpc GetClusters(GetClustersRequest) returns (GetClustersResponse);
  rpc GetHubs(GetHubsRequest) returns (GetHubsResponse);
  rpc GetNetworkMetrics(GetNetworkMetricsRequest) returns (NetworkMetrics);
  rpc GetEntityInfluence(GetEntityInfluenceRequest) returns (EntityInfluence);

  // Lifecycle
  rpc UpdateRelationshipState(UpdateRelationshipStateRequest) returns (Relationship);
  rpc GetRelationshipHistory(GetRelationshipHistoryRequest) returns (GetRelationshipHistoryResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

// Core messages
message Relationship {
  string id = 1;
  string tenant_id = 2;
  EntityRef source = 3;
  EntityRef target = 4;
  RelationshipType type = 5;
  RelationshipState state = 6;
  ConfidenceScore confidence = 7;
  repeated Evidence evidence = 8;
  RelationshipMetadata metadata = 9;
  google.protobuf.Timestamp discovered_at = 10;
  google.protobuf.Timestamp last_seen_at = 11;
  google.protobuf.Timestamp state_changed_at = 12;
}

message EntityRef {
  string id = 1;
  EntityType type = 2;
  string name = 3;
  string email = 4;
}

enum EntityType {
  ENTITY_TYPE_UNSPECIFIED = 0;
  ENTITY_TYPE_PERSON = 1;
  ENTITY_TYPE_ORGANIZATION = 2;
  ENTITY_TYPE_PROJECT = 3;
  ENTITY_TYPE_TEAM = 4;
  ENTITY_TYPE_DOCUMENT = 5;
  ENTITY_TYPE_DECISION = 6;
}

enum RelationshipType {
  RELATIONSHIP_TYPE_UNSPECIFIED = 0;
  RELATIONSHIP_TYPE_WORKS_WITH = 1;
  RELATIONSHIP_TYPE_REPORTS_TO = 2;
  RELATIONSHIP_TYPE_MANAGES = 3;
  RELATIONSHIP_TYPE_MEMBER_OF = 4;
  RELATIONSHIP_TYPE_LEADS = 5;
  RELATIONSHIP_TYPE_COLLABORATES_ON = 6;
  RELATIONSHIP_TYPE_OWNS = 7;
  RELATIONSHIP_TYPE_AUTHORED = 8;
  RELATIONSHIP_TYPE_MENTIONED_WITH = 9;
  RELATIONSHIP_TYPE_DECIDED_BY = 10;
  RELATIONSHIP_TYPE_RELATED_TO = 11;
}

enum RelationshipState {
  RELATIONSHIP_STATE_UNSPECIFIED = 0;
  RELATIONSHIP_STATE_PENDING = 1;
  RELATIONSHIP_STATE_ACTIVE = 2;
  RELATIONSHIP_STATE_HISTORICAL = 3;
  RELATIONSHIP_STATE_ARCHIVED = 4;
  RELATIONSHIP_STATE_REJECTED = 5;
  RELATIONSHIP_STATE_MERGED = 6;
}

message ConfidenceScore {
  float overall = 1;
  ConfidenceFactors factors = 2;
  string reasoning = 3;
}

message ConfidenceFactors {
  float ai_confidence = 1;        // 30% weight
  float evidence_strength = 2;     // 40% weight
  float entity_resolution = 3;     // 15% weight
  float freshness = 4;            // 15% weight
}

message Evidence {
  string id = 1;
  string source_id = 2;
  string source_type = 3;
  string excerpt = 4;
  google.protobuf.Timestamp observed_at = 5;
  float strength = 6;
}

message RelationshipMetadata {
  string context = 1;
  repeated string tags = 2;
  map<string, string> attributes = 3;
  int32 interaction_count = 4;
  google.protobuf.Timestamp first_interaction = 5;
  google.protobuf.Timestamp last_interaction = 6;
}

// Discovery messages
message DiscoverRelationshipsRequest {
  string tenant_id = 1;
  string source_id = 2;
  string content = 3;
  DiscoveryOptions options = 4;
}

message DiscoveryOptions {
  repeated RelationshipType types = 1;
  float min_confidence = 2;
  bool resolve_entities = 3;
  bool deduplicate = 4;
  bool track_evidence = 5;
}

message DiscoverRelationshipsResponse {
  repeated Relationship relationships = 1;
  DiscoveryStats stats = 2;
}

message DiscoveryStats {
  int32 discovered = 1;
  int32 merged = 2;
  int32 rejected = 3;
  int64 processing_time_ms = 4;
}

message DiscoverFromContentRequest {
  string tenant_id = 1;
  string content = 2;
  string content_type = 3;
  map<string, string> metadata = 4;
  DiscoveryOptions options = 5;
}

message DiscoverFromContentResponse {
  repeated Relationship relationships = 1;
  repeated EntityRef new_entities = 2;
  DiscoveryStats stats = 3;
}

// Query messages
message GetRelationshipRequest {
  string relationship_id = 1;
}

message ListRelationshipsRequest {
  string tenant_id = 1;
  RelationshipState state = 2;
  RelationshipType type = 3;
  int32 limit = 4;
  int32 offset = 5;
  string order_by = 6;
}

message ListRelationshipsResponse {
  repeated Relationship relationships = 1;
  int32 total_count = 2;
}

message GetRelationshipsByEntityRequest {
  string tenant_id = 1;
  string entity_id = 2;
  RelationshipDirection direction = 3;
  repeated RelationshipType types = 4;
  RelationshipState state = 5;
  int32 limit = 6;
}

enum RelationshipDirection {
  RELATIONSHIP_DIRECTION_UNSPECIFIED = 0;
  RELATIONSHIP_DIRECTION_OUTGOING = 1;
  RELATIONSHIP_DIRECTION_INCOMING = 2;
  RELATIONSHIP_DIRECTION_BOTH = 3;
}

message GetRelationshipsByEntityResponse {
  repeated Relationship relationships = 1;
}

message SearchRelationshipsRequest {
  string tenant_id = 1;
  string query = 2;
  repeated RelationshipType types = 3;
  float min_confidence = 4;
  int32 limit = 5;
}

message SearchRelationshipsResponse {
  repeated Relationship relationships = 1;
  int32 total_count = 2;
}

// Graph messages
message GetRelationshipGraphRequest {
  string tenant_id = 1;
  string center_entity_id = 2;
  int32 depth = 3;
  repeated RelationshipType types = 4;
  float min_confidence = 5;
  int32 max_nodes = 6;
}

message RelationshipGraph {
  repeated GraphNode nodes = 1;
  repeated GraphEdge edges = 2;
  GraphMetrics metrics = 3;
}

message GraphNode {
  string id = 1;
  EntityType type = 2;
  string name = 3;
  map<string, string> properties = 4;
  float centrality = 5;
}

message GraphEdge {
  string id = 1;
  string source_id = 2;
  string target_id = 3;
  RelationshipType type = 4;
  float weight = 5;
  float confidence = 6;
}

message GraphMetrics {
  int32 node_count = 1;
  int32 edge_count = 2;
  float density = 3;
  float avg_clustering = 4;
}

message FindPathRequest {
  string tenant_id = 1;
  string source_entity_id = 2;
  string target_entity_id = 3;
  int32 max_depth = 4;
  repeated RelationshipType types = 5;
}

message FindPathResponse {
  repeated Path paths = 1;
}

message Path {
  repeated PathNode nodes = 1;
  float total_weight = 2;
  int32 length = 3;
}

message PathNode {
  EntityRef entity = 1;
  RelationshipType relationship_type = 2;
  float edge_weight = 3;
}

message GetNeighborsRequest {
  string tenant_id = 1;
  string entity_id = 2;
  int32 depth = 3;
  repeated RelationshipType types = 4;
}

message GetNeighborsResponse {
  repeated EntityRef neighbors = 1;
  map<string, float> distances = 2;
}

// Validation messages
message ValidateRelationshipRequest {
  string relationship_id = 1;
  string user_id = 2;
  ValidationAction action = 3;
  string comment = 4;
}

enum ValidationAction {
  VALIDATION_ACTION_UNSPECIFIED = 0;
  VALIDATION_ACTION_CONFIRM = 1;
  VALIDATION_ACTION_REJECT = 2;
  VALIDATION_ACTION_MODIFY = 3;
}

message ValidateRelationshipResponse {
  Relationship relationship = 1;
  float confidence_change = 2;
}

message RejectRelationshipRequest {
  string relationship_id = 1;
  string reason = 2;
}

message MergeRelationshipsRequest {
  repeated string relationship_ids = 1;
  string primary_id = 2;
}

message MergeRelationshipsResponse {
  Relationship merged = 1;
  int32 merged_count = 2;
}

// Conflict resolution messages
message DetectConflictsRequest {
  string tenant_id = 1;
  string entity_id = 2;  // Optional: detect conflicts for specific entity
}

message DetectConflictsResponse {
  repeated RelationshipConflict conflicts = 1;
  int32 auto_resolved_count = 2;
  int32 pending_count = 3;
}

message RelationshipConflict {
  string id = 1;
  ConflictType type = 2;
  repeated Relationship conflicting_relationships = 3;
  float confidence_gap = 4;
  ConflictResolution suggested_resolution = 5;
  bool requires_user_review = 6;
  string description = 7;
  google.protobuf.Timestamp detected_at = 8;
}

enum ConflictType {
  CONFLICT_TYPE_UNSPECIFIED = 0;
  CONFLICT_TYPE_DUPLICATE = 1;          // Same relationship found multiple times
  CONFLICT_TYPE_CONTRADICTORY = 2;      // Conflicting relationships (e.g., reports_to vs manages)
  CONFLICT_TYPE_TEMPORAL = 3;           // Relationship state inconsistency over time
  CONFLICT_TYPE_ENTITY_AMBIGUITY = 4;   // Same entity referenced differently
}

message ConflictResolution {
  ConflictResolutionAction action = 1;
  string primary_relationship_id = 2;
  repeated string relationships_to_merge = 3;
  repeated string relationships_to_reject = 4;
  string reasoning = 5;
}

enum ConflictResolutionAction {
  CONFLICT_RESOLUTION_ACTION_UNSPECIFIED = 0;
  CONFLICT_RESOLUTION_ACTION_KEEP_HIGHEST_CONFIDENCE = 1;
  CONFLICT_RESOLUTION_ACTION_MERGE = 2;
  CONFLICT_RESOLUTION_ACTION_REJECT_ALL = 3;
  CONFLICT_RESOLUTION_ACTION_ESCALATE_TO_USER = 4;
}

message ResolveConflictRequest {
  string conflict_id = 1;
  ConflictResolution resolution = 2;
  string user_id = 3;
  string comment = 4;
}

message ResolveConflictResponse {
  bool success = 1;
  Relationship resulting_relationship = 2;
  int32 relationships_affected = 3;
}

message GetPendingConflictsRequest {
  string tenant_id = 1;
  int32 limit = 2;
  int32 offset = 3;
}

message GetPendingConflictsResponse {
  repeated RelationshipConflict conflicts = 1;
  int32 total_count = 2;
}

// Network analysis messages
message GetClustersRequest {
  string tenant_id = 1;
  ClusteringAlgorithm algorithm = 2;
  int32 min_cluster_size = 3;
}

enum ClusteringAlgorithm {
  CLUSTERING_ALGORITHM_UNSPECIFIED = 0;
  CLUSTERING_ALGORITHM_LOUVAIN = 1;
  CLUSTERING_ALGORITHM_LABEL_PROPAGATION = 2;
  CLUSTERING_ALGORITHM_SPECTRAL = 3;
}

message GetClustersResponse {
  repeated Cluster clusters = 1;
  float modularity = 2;
}

message Cluster {
  string id = 1;
  repeated EntityRef members = 2;
  float density = 3;
  string suggested_name = 4;
  repeated string common_tags = 5;
}

message GetHubsRequest {
  string tenant_id = 1;
  int32 limit = 2;
  HubMetric metric = 3;
}

enum HubMetric {
  HUB_METRIC_UNSPECIFIED = 0;
  HUB_METRIC_DEGREE = 1;
  HUB_METRIC_BETWEENNESS = 2;
  HUB_METRIC_PAGERANK = 3;
  HUB_METRIC_EIGENVECTOR = 4;
}

message GetHubsResponse {
  repeated Hub hubs = 1;
}

message Hub {
  EntityRef entity = 1;
  float score = 2;
  int32 connection_count = 3;
  repeated string top_connections = 4;
}

message GetNetworkMetricsRequest {
  string tenant_id = 1;
}

message NetworkMetrics {
  int32 total_entities = 1;
  int32 total_relationships = 2;
  float network_density = 3;
  float avg_clustering_coefficient = 4;
  float avg_path_length = 5;
  int32 connected_components = 6;
  float reciprocity = 7;
  repeated TypeDistribution relationship_distribution = 8;
}

message TypeDistribution {
  RelationshipType type = 1;
  int32 count = 2;
  float percentage = 3;
}

message GetEntityInfluenceRequest {
  string tenant_id = 1;
  string entity_id = 2;
}

message EntityInfluence {
  EntityRef entity = 1;
  float influence_score = 2;
  int32 direct_connections = 3;
  int32 indirect_reach = 4;
  float betweenness_centrality = 5;
  float pagerank = 6;
  repeated InfluenceBreakdown by_type = 7;
}

message InfluenceBreakdown {
  RelationshipType type = 1;
  int32 count = 2;
  float avg_strength = 3;
}

// Lifecycle messages
message UpdateRelationshipStateRequest {
  string relationship_id = 1;
  RelationshipState new_state = 2;
  string reason = 3;
}

message GetRelationshipHistoryRequest {
  string relationship_id = 1;
}

message GetRelationshipHistoryResponse {
  repeated StateChange history = 1;
}

message StateChange {
  RelationshipState from_state = 1;
  RelationshipState to_state = 2;
  string reason = 3;
  string changed_by = 4;
  google.protobuf.Timestamp changed_at = 5;
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

## Relationship Extractor

```go
// internal/extractor/extractor.go

package extractor

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
)

type RelationshipExtractor struct {
    aiCoordinator aipb.AICoordinatorServiceClient
    entityResolver *EntityResolver
    db            *pgxpool.Pool
}

type ExtractionResult struct {
    Relationships []*Relationship
    NewEntities   []*EntityRef
    Stats         *DiscoveryStats
}

func (e *RelationshipExtractor) Extract(ctx context.Context, req *DiscoverFromContentRequest) (*ExtractionResult, error) {
    start := time.Now()

    // Build extraction prompt
    prompt := e.buildExtractionPrompt(req.Content, req.ContentType)

    // Call AI Coordinator
    aiResp, err := e.aiCoordinator.Process(ctx, &aipb.ProcessRequest{
        TenantId: req.TenantId,
        Task:     aipb.Task_TASK_RELATIONSHIP_EXTRACTION,
        Content:  req.Content,
        Prompt:   prompt,
        Options: &aipb.ProcessOptions{
            OutputFormat:          "json",
            AllowCloudEscalation: true,
        },
    })
    if err != nil {
        return nil, fmt.Errorf("AI extraction failed: %w", err)
    }

    // Parse AI response
    rawRelationships, err := e.parseAIResponse(aiResp.Result)
    if err != nil {
        return nil, fmt.Errorf("failed to parse AI response: %w", err)
    }

    // Resolve entities
    var relationships []*Relationship
    var newEntities []*EntityRef

    for _, raw := range rawRelationships {
        // Resolve source entity
        source, isNew, err := e.entityResolver.Resolve(ctx, req.TenantId, raw.SourceName, raw.SourceType)
        if err != nil {
            slog.Warn("failed to resolve source entity", "name", raw.SourceName, "error", err)
            continue
        }
        if isNew {
            newEntities = append(newEntities, source)
        }

        // Resolve target entity
        target, isNew, err := e.entityResolver.Resolve(ctx, req.TenantId, raw.TargetName, raw.TargetType)
        if err != nil {
            slog.Warn("failed to resolve target entity", "name", raw.TargetName, "error", err)
            continue
        }
        if isNew {
            newEntities = append(newEntities, target)
        }

        // Create relationship
        rel := &Relationship{
            Id:       generateID(),
            TenantId: req.TenantId,
            Source:   source,
            Target:   target,
            Type:     raw.Type,
            State:    RelationshipStatePending,
            Confidence: &ConfidenceScore{
                Factors: &ConfidenceFactors{
                    AiConfidence: raw.Confidence,
                },
            },
            Evidence: []*Evidence{{
                Id:         generateID(),
                SourceType: req.ContentType,
                Excerpt:    raw.Context,
                ObservedAt: timestamppb.Now(),
                Strength:   raw.Confidence,
            }},
            DiscoveredAt: timestamppb.Now(),
            LastSeenAt:   timestamppb.Now(),
        }

        relationships = append(relationships, rel)
    }

    // Deduplicate if requested
    if req.Options.Deduplicate {
        relationships = e.deduplicate(ctx, req.TenantId, relationships)
    }

    return &ExtractionResult{
        Relationships: relationships,
        NewEntities:   newEntities,
        Stats: &DiscoveryStats{
            Discovered:       int32(len(relationships)),
            ProcessingTimeMs: time.Since(start).Milliseconds(),
        },
    }, nil
}

func (e *RelationshipExtractor) buildExtractionPrompt(content, contentType string) string {
    return fmt.Sprintf(`Extract all relationships between entities from the following %s content.

For each relationship found, identify:
1. Source entity (name and type: person, organization, project, team)
2. Target entity (name and type)
3. Relationship type (works_with, reports_to, manages, member_of, leads, collaborates_on, owns, authored, mentioned_with, decided_by, related_to)
4. Confidence (0.0 to 1.0)
5. Context (the sentence or phrase that indicates this relationship)

Return as JSON array:
[
  {
    "source_name": "John Smith",
    "source_type": "person",
    "target_name": "Project Alpha",
    "target_type": "project",
    "relationship_type": "leads",
    "confidence": 0.9,
    "context": "John Smith leads Project Alpha"
  }
]

Content:
%s`, contentType, content)
}

type RawRelationship struct {
    SourceName   string  `json:"source_name"`
    SourceType   string  `json:"source_type"`
    TargetName   string  `json:"target_name"`
    TargetType   string  `json:"target_type"`
    Type         RelationshipType
    Confidence   float32 `json:"confidence"`
    Context      string  `json:"context"`
}

func (e *RelationshipExtractor) parseAIResponse(response string) ([]*RawRelationship, error) {
    var raw []struct {
        SourceName       string  `json:"source_name"`
        SourceType       string  `json:"source_type"`
        TargetName       string  `json:"target_name"`
        TargetType       string  `json:"target_type"`
        RelationshipType string  `json:"relationship_type"`
        Confidence       float32 `json:"confidence"`
        Context          string  `json:"context"`
    }

    if err := json.Unmarshal([]byte(response), &raw); err != nil {
        return nil, err
    }

    var relationships []*RawRelationship
    for _, r := range raw {
        relationships = append(relationships, &RawRelationship{
            SourceName: r.SourceName,
            SourceType: r.SourceType,
            TargetName: r.TargetName,
            TargetType: r.TargetType,
            Type:       parseRelationshipType(r.RelationshipType),
            Confidence: r.Confidence,
            Context:    r.Context,
        })
    }

    return relationships, nil
}

func (e *RelationshipExtractor) deduplicate(ctx context.Context, tenantID string, relationships []*Relationship) []*Relationship {
    var result []*Relationship

    for _, rel := range relationships {
        // Check for existing relationship
        existing, err := e.findExisting(ctx, tenantID, rel.Source.Id, rel.Target.Id, rel.Type)
        if err == nil && existing != nil {
            // Merge evidence
            existing.Evidence = append(existing.Evidence, rel.Evidence...)
            existing.LastSeenAt = timestamppb.Now()

            // Update confidence
            e.updateConfidence(existing)

            result = append(result, existing)
        } else {
            result = append(result, rel)
        }
    }

    return result
}
```

## Confidence Scorer

```go
// internal/confidence/scorer.go

package confidence

import (
    "time"
)

type ConfidenceScorer struct {
    weights ConfidenceWeights
}

type ConfidenceWeights struct {
    AIConfidence       float32  // 30%
    EvidenceStrength   float32  // 40%
    EntityResolution   float32  // 15%
    Freshness          float32  // 15%
}

var DefaultWeights = ConfidenceWeights{
    AIConfidence:     0.30,
    EvidenceStrength: 0.40,
    EntityResolution: 0.15,
    Freshness:        0.15,
}

func NewConfidenceScorer() *ConfidenceScorer {
    return &ConfidenceScorer{weights: DefaultWeights}
}

func (s *ConfidenceScorer) Calculate(rel *Relationship) *ConfidenceScore {
    factors := &ConfidenceFactors{}

    // AI Confidence (from extraction)
    factors.AiConfidence = s.calculateAIConfidence(rel)

    // Evidence Strength (number and recency of evidence)
    factors.EvidenceStrength = s.calculateEvidenceStrength(rel)

    // Entity Resolution (how well entities were resolved)
    factors.EntityResolution = s.calculateEntityResolution(rel)

    // Freshness (how recent is the relationship)
    factors.Freshness = s.calculateFreshness(rel)

    // Calculate overall score
    overall := factors.AiConfidence*s.weights.AIConfidence +
        factors.EvidenceStrength*s.weights.EvidenceStrength +
        factors.EntityResolution*s.weights.EntityResolution +
        factors.Freshness*s.weights.Freshness

    return &ConfidenceScore{
        Overall:   overall,
        Factors:   factors,
        Reasoning: s.generateReasoning(factors),
    }
}

func (s *ConfidenceScorer) calculateAIConfidence(rel *Relationship) float32 {
    if rel.Confidence != nil && rel.Confidence.Factors != nil {
        return rel.Confidence.Factors.AiConfidence
    }

    // Average confidence from evidence
    if len(rel.Evidence) == 0 {
        return 0.5
    }

    var sum float32
    for _, e := range rel.Evidence {
        sum += e.Strength
    }
    return sum / float32(len(rel.Evidence))
}

func (s *ConfidenceScorer) calculateEvidenceStrength(rel *Relationship) float32 {
    if len(rel.Evidence) == 0 {
        return 0
    }

    // Base score on evidence count (diminishing returns)
    countScore := float32(min(len(rel.Evidence), 10)) / 10.0

    // Boost for recent evidence
    var recencyBonus float32
    for _, e := range rel.Evidence {
        age := time.Since(e.ObservedAt.AsTime())
        if age < 24*time.Hour {
            recencyBonus += 0.2
        } else if age < 7*24*time.Hour {
            recencyBonus += 0.1
        }
    }
    recencyBonus = min(recencyBonus, 0.3)

    // Boost for high-strength evidence
    var strengthBonus float32
    for _, e := range rel.Evidence {
        if e.Strength > 0.8 {
            strengthBonus += 0.1
        }
    }
    strengthBonus = min(strengthBonus, 0.2)

    return min(1.0, countScore*0.5+recencyBonus+strengthBonus)
}

func (s *ConfidenceScorer) calculateEntityResolution(rel *Relationship) float32 {
    var score float32 = 1.0

    // Penalize if source not fully resolved
    if rel.Source.Id == "" {
        score -= 0.3
    }
    if rel.Source.Email == "" && rel.Source.Type == EntityTypePerson {
        score -= 0.1
    }

    // Penalize if target not fully resolved
    if rel.Target.Id == "" {
        score -= 0.3
    }
    if rel.Target.Email == "" && rel.Target.Type == EntityTypePerson {
        score -= 0.1
    }

    return max(0, score)
}

func (s *ConfidenceScorer) calculateFreshness(rel *Relationship) float32 {
    if rel.LastSeenAt == nil {
        return 0.5
    }

    age := time.Since(rel.LastSeenAt.AsTime())

    switch {
    case age < 24*time.Hour:
        return 1.0
    case age < 7*24*time.Hour:
        return 0.9
    case age < 30*24*time.Hour:
        return 0.7
    case age < 90*24*time.Hour:
        return 0.5
    case age < 365*24*time.Hour:
        return 0.3
    default:
        return 0.1
    }
}

func (s *ConfidenceScorer) generateReasoning(factors *ConfidenceFactors) string {
    var reasons []string

    if factors.AiConfidence > 0.8 {
        reasons = append(reasons, "high AI extraction confidence")
    } else if factors.AiConfidence < 0.5 {
        reasons = append(reasons, "low AI extraction confidence")
    }

    if factors.EvidenceStrength > 0.7 {
        reasons = append(reasons, "strong evidence support")
    } else if factors.EvidenceStrength < 0.3 {
        reasons = append(reasons, "weak evidence support")
    }

    if factors.EntityResolution > 0.9 {
        reasons = append(reasons, "entities fully resolved")
    } else if factors.EntityResolution < 0.5 {
        reasons = append(reasons, "entity resolution uncertain")
    }

    if factors.Freshness > 0.8 {
        reasons = append(reasons, "recently observed")
    } else if factors.Freshness < 0.3 {
        reasons = append(reasons, "not recently observed")
    }

    return strings.Join(reasons, "; ")
}
```

## Conflict Resolver

```go
// internal/resolver/conflict.go

package resolver

import (
    "context"
    "fmt"
    "log/slog"
    "math"
    "sort"
    "time"
)

const (
    // AutoResolveConfidenceGap is the minimum confidence gap (30%) required
    // to auto-resolve a conflict without user intervention.
    // From original spec: "Auto-resolve if confidence gap > 30%, else escalate to user"
    AutoResolveConfidenceGap = 0.30
)

type ConflictResolver struct {
    db          *pgxpool.Pool
    scorer      *ConfidenceScorer
    publisher   *events.Publisher
}

type ConflictConfig struct {
    AutoResolveGap     float64  // Minimum confidence gap for auto-resolution (default 0.30)
    AutoMergeDuplicates bool    // Automatically merge exact duplicates
    MaxPendingConflicts int     // Maximum pending conflicts before alerting
}

var DefaultConflictConfig = ConflictConfig{
    AutoResolveGap:      AutoResolveConfidenceGap,
    AutoMergeDuplicates: true,
    MaxPendingConflicts: 100,
}

func NewConflictResolver(db *pgxpool.Pool, scorer *ConfidenceScorer, publisher *events.Publisher) *ConflictResolver {
    return &ConflictResolver{
        db:        db,
        scorer:    scorer,
        publisher: publisher,
    }
}

// DetectConflicts finds conflicting relationships for a tenant or entity
func (r *ConflictResolver) DetectConflicts(ctx context.Context, tenantID, entityID string) (*DetectConflictsResponse, error) {
    var conflicts []*RelationshipConflict
    autoResolved := 0
    pending := 0

    // Find duplicate relationships
    duplicates, err := r.findDuplicates(ctx, tenantID, entityID)
    if err != nil {
        return nil, fmt.Errorf("failed to find duplicates: %w", err)
    }

    // Find contradictory relationships
    contradictions, err := r.findContradictions(ctx, tenantID, entityID)
    if err != nil {
        return nil, fmt.Errorf("failed to find contradictions: %w", err)
    }

    allConflicts := append(duplicates, contradictions...)

    // Process each conflict
    for _, conflict := range allConflicts {
        // Calculate suggested resolution
        resolution := r.suggestResolution(conflict)
        conflict.SuggestedResolution = resolution

        // Determine if auto-resolve is possible
        if r.canAutoResolve(conflict) {
            // Auto-resolve the conflict
            if err := r.autoResolve(ctx, conflict); err != nil {
                slog.Warn("failed to auto-resolve conflict", "conflict_id", conflict.Id, "error", err)
                conflict.RequiresUserReview = true
                pending++
            } else {
                autoResolved++
                continue  // Don't include auto-resolved conflicts in response
            }
        } else {
            conflict.RequiresUserReview = true
            pending++
        }

        conflicts = append(conflicts, conflict)
    }

    // Publish event if there are pending conflicts
    if pending > 0 {
        r.publisher.Publish(ctx, "relationship.conflicts_detected", &ConflictsDetectedEvent{
            TenantID:     tenantID,
            PendingCount: pending,
            AutoResolved: autoResolved,
        })
    }

    return &DetectConflictsResponse{
        Conflicts:         conflicts,
        AutoResolvedCount: int32(autoResolved),
        PendingCount:      int32(pending),
    }, nil
}

// canAutoResolve determines if a conflict can be automatically resolved
// Based on original spec: "Auto-resolve if confidence gap > 30%, else escalate to user"
func (r *ConflictResolver) canAutoResolve(conflict *RelationshipConflict) bool {
    switch conflict.Type {
    case ConflictTypeDuplicate:
        // Always auto-merge exact duplicates
        return DefaultConflictConfig.AutoMergeDuplicates

    case ConflictTypeContradictory:
        // Auto-resolve if confidence gap > 30%
        return conflict.ConfidenceGap >= AutoResolveConfidenceGap

    case ConflictTypeTemporal:
        // Auto-resolve temporal conflicts if one is clearly more recent and confident
        return conflict.ConfidenceGap >= AutoResolveConfidenceGap

    case ConflictTypeEntityAmbiguity:
        // Entity ambiguity usually requires user review
        return false

    default:
        return false
    }
}

// suggestResolution generates a suggested resolution for a conflict
func (r *ConflictResolver) suggestResolution(conflict *RelationshipConflict) *ConflictResolution {
    relationships := conflict.ConflictingRelationships

    // Sort by confidence (highest first)
    sort.Slice(relationships, func(i, j int) bool {
        return relationships[i].Confidence.Overall > relationships[j].Confidence.Overall
    })

    highest := relationships[0]
    confidenceGap := float64(0)
    if len(relationships) > 1 {
        confidenceGap = float64(highest.Confidence.Overall - relationships[1].Confidence.Overall)
    }
    conflict.ConfidenceGap = float32(confidenceGap)

    // Determine action based on conflict type and confidence gap
    switch conflict.Type {
    case ConflictTypeDuplicate:
        // Merge duplicates, keeping highest confidence as primary
        var toMerge []string
        for _, rel := range relationships[1:] {
            toMerge = append(toMerge, rel.Id)
        }
        return &ConflictResolution{
            Action:                   ConflictResolutionActionMerge,
            PrimaryRelationshipId:   highest.Id,
            RelationshipsToMerge:    toMerge,
            Reasoning:               fmt.Sprintf("Merging %d duplicate relationships, keeping highest confidence (%.2f)", len(toMerge)+1, highest.Confidence.Overall),
        }

    case ConflictTypeContradictory:
        if confidenceGap >= AutoResolveConfidenceGap {
            // Clear winner - keep highest, reject others
            var toReject []string
            for _, rel := range relationships[1:] {
                toReject = append(toReject, rel.Id)
            }
            return &ConflictResolution{
                Action:                  ConflictResolutionActionKeepHighestConfidence,
                PrimaryRelationshipId:  highest.Id,
                RelationshipsToReject:  toReject,
                Reasoning:              fmt.Sprintf("Confidence gap (%.0f%%) exceeds 30%% threshold; keeping highest confidence relationship", confidenceGap*100),
            }
        } else {
            // Close confidence - escalate to user
            return &ConflictResolution{
                Action:    ConflictResolutionActionEscalateToUser,
                Reasoning: fmt.Sprintf("Confidence gap (%.0f%%) below 30%% threshold; user review required", confidenceGap*100),
            }
        }

    case ConflictTypeTemporal:
        // Prefer more recent with sufficient confidence
        if confidenceGap >= AutoResolveConfidenceGap {
            return &ConflictResolution{
                Action:                 ConflictResolutionActionKeepHighestConfidence,
                PrimaryRelationshipId: highest.Id,
                Reasoning:             "Keeping most confident relationship; marking others as historical",
            }
        }
        return &ConflictResolution{
            Action:    ConflictResolutionActionEscalateToUser,
            Reasoning: "Temporal conflict with similar confidence; user review required",
        }

    default:
        return &ConflictResolution{
            Action:    ConflictResolutionActionEscalateToUser,
            Reasoning: "Unknown conflict type; user review required",
        }
    }
}

// autoResolve applies the suggested resolution automatically
func (r *ConflictResolver) autoResolve(ctx context.Context, conflict *RelationshipConflict) error {
    resolution := conflict.SuggestedResolution

    switch resolution.Action {
    case ConflictResolutionActionMerge:
        return r.mergeRelationships(ctx, resolution.PrimaryRelationshipId, resolution.RelationshipsToMerge)

    case ConflictResolutionActionKeepHighestConfidence:
        // Keep primary, reject/archive others
        for _, id := range resolution.RelationshipsToReject {
            if err := r.updateRelationshipState(ctx, id, RelationshipStateRejected, "Auto-resolved: lower confidence"); err != nil {
                return err
            }
        }
        return nil

    default:
        return fmt.Errorf("cannot auto-resolve action: %v", resolution.Action)
    }
}

// ResolveConflict applies a user-provided resolution
func (r *ConflictResolver) ResolveConflict(ctx context.Context, req *ResolveConflictRequest) (*ResolveConflictResponse, error) {
    // Record resolution
    if err := r.recordResolution(ctx, req); err != nil {
        return nil, err
    }

    resolution := req.Resolution
    var result *Relationship
    var affected int

    switch resolution.Action {
    case ConflictResolutionActionMerge:
        if err := r.mergeRelationships(ctx, resolution.PrimaryRelationshipId, resolution.RelationshipsToMerge); err != nil {
            return nil, err
        }
        result, _ = r.getRelationship(ctx, resolution.PrimaryRelationshipId)
        affected = len(resolution.RelationshipsToMerge) + 1

    case ConflictResolutionActionKeepHighestConfidence:
        result, _ = r.getRelationship(ctx, resolution.PrimaryRelationshipId)
        for _, id := range resolution.RelationshipsToReject {
            if err := r.updateRelationshipState(ctx, id, RelationshipStateRejected, req.Comment); err != nil {
                slog.Warn("failed to reject relationship", "id", id, "error", err)
            }
            affected++
        }

    case ConflictResolutionActionRejectAll:
        for _, rel := range append(resolution.RelationshipsToMerge, resolution.RelationshipsToReject...) {
            if err := r.updateRelationshipState(ctx, rel, RelationshipStateRejected, req.Comment); err != nil {
                slog.Warn("failed to reject relationship", "id", rel, "error", err)
            }
            affected++
        }
    }

    // Mark conflict as resolved
    r.markConflictResolved(ctx, req.ConflictId, req.UserId)

    // Publish event
    r.publisher.Publish(ctx, "relationship.conflict_resolved", &ConflictResolvedEvent{
        ConflictID: req.ConflictId,
        Action:     resolution.Action.String(),
        UserID:     req.UserId,
        Affected:   affected,
    })

    return &ResolveConflictResponse{
        Success:               true,
        ResultingRelationship: result,
        RelationshipsAffected: int32(affected),
    }, nil
}

// findDuplicates finds relationships that appear to be duplicates
func (r *ConflictResolver) findDuplicates(ctx context.Context, tenantID, entityID string) ([]*RelationshipConflict, error) {
    query := `
        SELECT r1.id, r2.id,
               r1.source_id, r1.target_id, r1.relationship_type,
               (r1.confidence->>'overall')::float as conf1,
               (r2.confidence->>'overall')::float as conf2
        FROM relationships r1
        JOIN relationships r2 ON
            r1.tenant_id = r2.tenant_id AND
            r1.source_id = r2.source_id AND
            r1.target_id = r2.target_id AND
            r1.relationship_type = r2.relationship_type AND
            r1.id < r2.id
        WHERE r1.tenant_id = $1
          AND r1.state IN ('pending', 'active')
          AND r2.state IN ('pending', 'active')
    `

    args := []interface{}{tenantID}
    if entityID != "" {
        query += " AND (r1.source_id = $2 OR r1.target_id = $2)"
        args = append(args, entityID)
    }

    rows, err := r.db.Query(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    conflictMap := make(map[string]*RelationshipConflict)
    for rows.Next() {
        var id1, id2, sourceID, targetID, relType string
        var conf1, conf2 float64

        if err := rows.Scan(&id1, &id2, &sourceID, &targetID, &relType, &conf1, &conf2); err != nil {
            continue
        }

        key := fmt.Sprintf("%s-%s-%s", sourceID, targetID, relType)
        if existing, ok := conflictMap[key]; ok {
            // Add to existing conflict
            r.addToConflict(ctx, existing, id2, conf2)
        } else {
            // Create new conflict
            conflict := &RelationshipConflict{
                Id:          generateID(),
                Type:        ConflictTypeDuplicate,
                Description: fmt.Sprintf("Duplicate %s relationship between same entities", relType),
                DetectedAt:  timestamppb.Now(),
            }
            r.addToConflict(ctx, conflict, id1, conf1)
            r.addToConflict(ctx, conflict, id2, conf2)
            conflictMap[key] = conflict
        }
    }

    var conflicts []*RelationshipConflict
    for _, c := range conflictMap {
        conflicts = append(conflicts, c)
    }

    return conflicts, nil
}

// findContradictions finds relationships that contradict each other
func (r *ConflictResolver) findContradictions(ctx context.Context, tenantID, entityID string) ([]*RelationshipConflict, error) {
    // Define contradictory relationship pairs
    contradictions := map[string]string{
        "reports_to": "manages",
        "manages":    "reports_to",
        "leads":      "member_of",
    }

    var conflicts []*RelationshipConflict

    for type1, type2 := range contradictions {
        query := `
            SELECT r1.id, r2.id,
                   r1.source_id, r1.target_id,
                   (r1.confidence->>'overall')::float as conf1,
                   (r2.confidence->>'overall')::float as conf2
            FROM relationships r1
            JOIN relationships r2 ON
                r1.tenant_id = r2.tenant_id AND
                r1.source_id = r2.target_id AND
                r1.target_id = r2.source_id AND
                r1.id != r2.id
            WHERE r1.tenant_id = $1
              AND r1.relationship_type = $2
              AND r2.relationship_type = $3
              AND r1.state IN ('pending', 'active')
              AND r2.state IN ('pending', 'active')
        `

        args := []interface{}{tenantID, type1, type2}
        if entityID != "" {
            query += " AND (r1.source_id = $4 OR r1.target_id = $4)"
            args = append(args, entityID)
        }

        rows, err := r.db.Query(ctx, query, args...)
        if err != nil {
            continue
        }

        for rows.Next() {
            var id1, id2, sourceID, targetID string
            var conf1, conf2 float64

            if err := rows.Scan(&id1, &id2, &sourceID, &targetID, &conf1, &conf2); err != nil {
                continue
            }

            conflict := &RelationshipConflict{
                Id:            generateID(),
                Type:          ConflictTypeContradictory,
                ConfidenceGap: float32(math.Abs(conf1 - conf2)),
                Description:   fmt.Sprintf("Contradictory relationships: %s vs %s", type1, type2),
                DetectedAt:    timestamppb.Now(),
            }
            r.addToConflict(ctx, conflict, id1, conf1)
            r.addToConflict(ctx, conflict, id2, conf2)
            conflicts = append(conflicts, conflict)
        }
        rows.Close()
    }

    return conflicts, nil
}

func (r *ConflictResolver) mergeRelationships(ctx context.Context, primaryID string, mergeIDs []string) error {
    // Get primary relationship
    primary, err := r.getRelationship(ctx, primaryID)
    if err != nil {
        return err
    }

    // Collect evidence from all relationships
    for _, id := range mergeIDs {
        rel, err := r.getRelationship(ctx, id)
        if err != nil {
            continue
        }

        // Merge evidence
        primary.Evidence = append(primary.Evidence, rel.Evidence...)

        // Mark as merged
        r.updateRelationshipState(ctx, id, RelationshipStateMerged, fmt.Sprintf("Merged into %s", primaryID))
    }

    // Recalculate confidence with combined evidence
    primary.Confidence = r.scorer.Calculate(primary)
    primary.LastSeenAt = timestamppb.Now()

    // Update primary
    return r.updateRelationship(ctx, primary)
}

type ConflictsDetectedEvent struct {
    TenantID     string
    PendingCount int
    AutoResolved int
}

type ConflictResolvedEvent struct {
    ConflictID string
    Action     string
    UserID     string
    Affected   int
}
```

## Network Analyzer

```go
// internal/network/analyzer.go

package network

import (
    "context"
    "math"
    "sort"
)

type NetworkAnalyzer struct {
    db *pgxpool.Pool
}

type Graph struct {
    Nodes map[string]*Node
    Edges map[string]*Edge
}

type Node struct {
    ID         string
    Type       EntityType
    Name       string
    Neighbors  map[string]float64  // neighbor ID -> edge weight
    Properties map[string]string
}

type Edge struct {
    ID       string
    SourceID string
    TargetID string
    Type     RelationshipType
    Weight   float64
}

func (a *NetworkAnalyzer) BuildGraph(ctx context.Context, tenantID string, opts *GraphOptions) (*Graph, error) {
    // Load relationships from database
    query := `
        SELECT
            r.id, r.source_id, r.target_id, r.relationship_type,
            r.confidence->>'overall' as weight,
            se.name as source_name, se.entity_type as source_type,
            te.name as target_name, te.entity_type as target_type
        FROM relationships r
        JOIN entities se ON r.source_id = se.id
        JOIN entities te ON r.target_id = te.id
        WHERE r.tenant_id = $1 AND r.state = 'active'
    `

    rows, err := a.db.Query(ctx, query, tenantID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    graph := &Graph{
        Nodes: make(map[string]*Node),
        Edges: make(map[string]*Edge),
    }

    for rows.Next() {
        var rel struct {
            ID, SourceID, TargetID, Type string
            Weight                        float64
            SourceName, SourceType        string
            TargetName, TargetType        string
        }

        if err := rows.Scan(
            &rel.ID, &rel.SourceID, &rel.TargetID, &rel.Type, &rel.Weight,
            &rel.SourceName, &rel.SourceType, &rel.TargetName, &rel.TargetType,
        ); err != nil {
            continue
        }

        // Add source node
        if _, ok := graph.Nodes[rel.SourceID]; !ok {
            graph.Nodes[rel.SourceID] = &Node{
                ID:        rel.SourceID,
                Name:      rel.SourceName,
                Type:      parseEntityType(rel.SourceType),
                Neighbors: make(map[string]float64),
            }
        }

        // Add target node
        if _, ok := graph.Nodes[rel.TargetID]; !ok {
            graph.Nodes[rel.TargetID] = &Node{
                ID:        rel.TargetID,
                Name:      rel.TargetName,
                Type:      parseEntityType(rel.TargetType),
                Neighbors: make(map[string]float64),
            }
        }

        // Add edge
        graph.Edges[rel.ID] = &Edge{
            ID:       rel.ID,
            SourceID: rel.SourceID,
            TargetID: rel.TargetID,
            Type:     parseRelationshipType(rel.Type),
            Weight:   rel.Weight,
        }

        // Update neighbor maps
        graph.Nodes[rel.SourceID].Neighbors[rel.TargetID] = rel.Weight
        graph.Nodes[rel.TargetID].Neighbors[rel.SourceID] = rel.Weight
    }

    return graph, nil
}

func (a *NetworkAnalyzer) CalculateMetrics(graph *Graph) *NetworkMetrics {
    nodeCount := len(graph.Nodes)
    edgeCount := len(graph.Edges)

    // Calculate density
    maxEdges := float64(nodeCount * (nodeCount - 1) / 2)
    density := float64(0)
    if maxEdges > 0 {
        density = float64(edgeCount) / maxEdges
    }

    // Calculate clustering coefficient
    avgClustering := a.calculateAvgClustering(graph)

    // Calculate average path length (approximate for large graphs)
    avgPathLength := a.calculateAvgPathLength(graph)

    // Count connected components
    components := a.countConnectedComponents(graph)

    return &NetworkMetrics{
        TotalEntities:            int32(nodeCount),
        TotalRelationships:       int32(edgeCount),
        NetworkDensity:           float32(density),
        AvgClusteringCoefficient: float32(avgClustering),
        AvgPathLength:            float32(avgPathLength),
        ConnectedComponents:      int32(components),
    }
}

func (a *NetworkAnalyzer) FindHubs(graph *Graph, metric HubMetric, limit int) []*Hub {
    var scores []struct {
        NodeID string
        Score  float64
    }

    switch metric {
    case HubMetricDegree:
        for id, node := range graph.Nodes {
            scores = append(scores, struct {
                NodeID string
                Score  float64
            }{id, float64(len(node.Neighbors))})
        }

    case HubMetricBetweenness:
        betweenness := a.calculateBetweenness(graph)
        for id, score := range betweenness {
            scores = append(scores, struct {
                NodeID string
                Score  float64
            }{id, score})
        }

    case HubMetricPagerank:
        pagerank := a.calculatePageRank(graph, 0.85, 100)
        for id, score := range pagerank {
            scores = append(scores, struct {
                NodeID string
                Score  float64
            }{id, score})
        }
    }

    // Sort by score descending
    sort.Slice(scores, func(i, j int) bool {
        return scores[i].Score > scores[j].Score
    })

    // Return top N
    var hubs []*Hub
    for i := 0; i < min(limit, len(scores)); i++ {
        node := graph.Nodes[scores[i].NodeID]
        hubs = append(hubs, &Hub{
            Entity: &EntityRef{
                Id:   node.ID,
                Type: node.Type,
                Name: node.Name,
            },
            Score:           float32(scores[i].Score),
            ConnectionCount: int32(len(node.Neighbors)),
        })
    }

    return hubs
}

func (a *NetworkAnalyzer) FindPath(graph *Graph, sourceID, targetID string, maxDepth int) []*Path {
    // BFS to find shortest paths
    type QueueItem struct {
        NodeID string
        Path   []string
    }

    visited := make(map[string]bool)
    queue := []QueueItem{{sourceID, []string{sourceID}}}
    var paths []*Path

    for len(queue) > 0 && len(paths) < 5 {
        item := queue[0]
        queue = queue[1:]

        if len(item.Path) > maxDepth {
            continue
        }

        if item.NodeID == targetID {
            path := a.buildPath(graph, item.Path)
            paths = append(paths, path)
            continue
        }

        if visited[item.NodeID] {
            continue
        }
        visited[item.NodeID] = true

        node := graph.Nodes[item.NodeID]
        for neighborID := range node.Neighbors {
            newPath := append([]string{}, item.Path...)
            newPath = append(newPath, neighborID)
            queue = append(queue, QueueItem{neighborID, newPath})
        }
    }

    return paths
}

func (a *NetworkAnalyzer) calculatePageRank(graph *Graph, damping float64, iterations int) map[string]float64 {
    n := len(graph.Nodes)
    if n == 0 {
        return nil
    }

    // Initialize
    pr := make(map[string]float64)
    initial := 1.0 / float64(n)
    for id := range graph.Nodes {
        pr[id] = initial
    }

    // Iterate
    for i := 0; i < iterations; i++ {
        newPR := make(map[string]float64)
        for id := range graph.Nodes {
            newPR[id] = (1 - damping) / float64(n)
        }

        for id, node := range graph.Nodes {
            outDegree := len(node.Neighbors)
            if outDegree > 0 {
                share := pr[id] / float64(outDegree)
                for neighborID := range node.Neighbors {
                    newPR[neighborID] += damping * share
                }
            }
        }

        pr = newPR
    }

    return pr
}

func (a *NetworkAnalyzer) DetectClusters(graph *Graph, algorithm ClusteringAlgorithm) []*Cluster {
    switch algorithm {
    case ClusteringAlgorithmLouvain:
        return a.louvainClustering(graph)
    case ClusteringAlgorithmLabelPropagation:
        return a.labelPropagation(graph)
    default:
        return a.louvainClustering(graph)
    }
}

func (a *NetworkAnalyzer) louvainClustering(graph *Graph) []*Cluster {
    // Simplified Louvain implementation
    communities := make(map[string]string)

    // Initialize: each node in its own community
    for id := range graph.Nodes {
        communities[id] = id
    }

    // Iterate until no improvement
    improved := true
    for improved {
        improved = false
        for nodeID := range graph.Nodes {
            bestCommunity := communities[nodeID]
            bestGain := 0.0

            // Try moving to each neighbor's community
            for neighborID := range graph.Nodes[nodeID].Neighbors {
                neighborCommunity := communities[neighborID]
                if neighborCommunity == communities[nodeID] {
                    continue
                }

                gain := a.calculateModularityGain(graph, communities, nodeID, neighborCommunity)
                if gain > bestGain {
                    bestGain = gain
                    bestCommunity = neighborCommunity
                }
            }

            if bestCommunity != communities[nodeID] {
                communities[nodeID] = bestCommunity
                improved = true
            }
        }
    }

    // Group nodes by community
    clusterMap := make(map[string][]*EntityRef)
    for nodeID, communityID := range communities {
        node := graph.Nodes[nodeID]
        clusterMap[communityID] = append(clusterMap[communityID], &EntityRef{
            Id:   node.ID,
            Type: node.Type,
            Name: node.Name,
        })
    }

    var clusters []*Cluster
    for id, members := range clusterMap {
        if len(members) > 1 {
            clusters = append(clusters, &Cluster{
                Id:      id,
                Members: members,
            })
        }
    }

    return clusters
}
```

## Configuration

```yaml
# config/relationship-discovery.yaml

server:
  grpc_port: 8084
  metrics_port: 9084

extraction:
  min_confidence: 0.6
  resolve_entities: true
  deduplicate: true
  track_evidence: true

confidence:
  weights:
    ai_confidence: 0.30
    evidence_strength: 0.40
    entity_resolution: 0.15
    freshness: 0.15

lifecycle:
  auto_archive_after_days: 365
  auto_historical_after_days: 180
  min_confirmations_for_active: 2
  retention_years: 2                    # 2-year retention before archival (from original spec)

conflict_resolution:
  auto_resolve_confidence_gap: 0.30     # 30% confidence gap threshold for auto-resolve
  auto_merge_duplicates: true           # Automatically merge exact duplicates
  max_pending_conflicts: 100            # Alert threshold for pending conflicts
  escalate_ambiguous_entities: true     # Always escalate entity ambiguity to user

network:
  max_graph_nodes: 1000
  max_path_depth: 6
  default_clustering: "louvain"

ai_coordinator:
  address: "localhost:8085"
  timeout: "30s"

database:
  host: "home-01"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

redis:
  address: "home-01:6379"

logging:
  level: "info"
  format: "json"
```

## Database Schema

```sql
-- Relationships
CREATE TABLE relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    source_id UUID NOT NULL REFERENCES entities(id),
    target_id UUID NOT NULL REFERENCES entities(id),
    relationship_type VARCHAR(50) NOT NULL,
    state VARCHAR(50) NOT NULL DEFAULT 'pending',
    confidence JSONB NOT NULL DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    state_changed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, source_id, target_id, relationship_type)
);

CREATE INDEX idx_relationships_tenant ON relationships(tenant_id);
CREATE INDEX idx_relationships_source ON relationships(source_id);
CREATE INDEX idx_relationships_target ON relationships(target_id);
CREATE INDEX idx_relationships_type ON relationships(relationship_type);
CREATE INDEX idx_relationships_state ON relationships(state);

-- Relationship evidence
CREATE TABLE relationship_evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_id UUID NOT NULL REFERENCES relationships(id) ON DELETE CASCADE,
    source_id UUID NOT NULL,  -- Content source
    source_type VARCHAR(50) NOT NULL,
    excerpt TEXT,
    strength FLOAT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_evidence_relationship ON relationship_evidence(relationship_id);
CREATE INDEX idx_evidence_source ON relationship_evidence(source_id);

-- Relationship state history
CREATE TABLE relationship_state_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_id UUID NOT NULL REFERENCES relationships(id) ON DELETE CASCADE,
    from_state VARCHAR(50),
    to_state VARCHAR(50) NOT NULL,
    reason TEXT,
    changed_by UUID,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_state_history_relationship ON relationship_state_history(relationship_id);

-- Relationship validations
CREATE TABLE relationship_validations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_id UUID NOT NULL REFERENCES relationships(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_validations_relationship ON relationship_validations(relationship_id);

-- Relationship conflicts
CREATE TABLE relationship_conflicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    conflict_type VARCHAR(50) NOT NULL,
    description TEXT,
    confidence_gap FLOAT,
    suggested_action VARCHAR(50),
    requires_user_review BOOLEAN DEFAULT true,
    resolved_at TIMESTAMPTZ,
    resolved_by UUID,
    resolution_action VARCHAR(50),
    resolution_comment TEXT,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_conflicts_tenant ON relationship_conflicts(tenant_id);
CREATE INDEX idx_conflicts_pending ON relationship_conflicts(tenant_id) WHERE resolved_at IS NULL;

-- Conflict-relationship mapping
CREATE TABLE conflict_relationships (
    conflict_id UUID NOT NULL REFERENCES relationship_conflicts(id) ON DELETE CASCADE,
    relationship_id UUID NOT NULL REFERENCES relationships(id) ON DELETE CASCADE,
    confidence FLOAT,
    PRIMARY KEY (conflict_id, relationship_id)
);

-- Entities (if not already exists)
CREATE TABLE IF NOT EXISTS entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    properties JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entities_tenant ON entities(tenant_id);
CREATE INDEX idx_entities_type ON entities(entity_type);
CREATE INDEX idx_entities_name ON entities(name);
```

## Implementation Structure

```
services/relationship-discovery/
├── cmd/
│   └── relationship-discovery/
│       └── main.go
├── internal/
│   ├── extractor/
│   │   ├── extractor.go
│   │   └── patterns.go
│   ├── confidence/
│   │   ├── scorer.go
│   │   └── factors.go
│   ├── resolver/
│   │   ├── entity.go
│   │   ├── conflict.go
│   │   └── deduplication.go
│   ├── lifecycle/
│   │   ├── manager.go
│   │   └── transitions.go
│   ├── network/
│   │   ├── analyzer.go
│   │   ├── clustering.go
│   │   ├── metrics.go
│   │   └── paths.go
│   ├── validation/
│   │   └── validator.go
│   ├── service/
│   │   └── grpc.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── relationship/
│           └── v1/
│               └── relationship.proto
└── go.mod
```

## Events Published

| Event | Trigger | Payload |
|-------|---------|---------|
| `relationship.discovered` | New relationship found | RelationshipID, Type, Confidence |
| `relationship.updated` | Confidence changed | RelationshipID, OldConfidence, NewConfidence |
| `relationship.validated` | User confirmed | RelationshipID, UserID, Action |
| `relationship.state_changed` | State transition | RelationshipID, FromState, ToState |
| `relationship.conflict` | Contradiction detected | RelationshipIDs, ConflictType |
| `relationship.conflicts_detected` | Conflicts found during scan | TenantID, PendingCount, AutoResolved |
| `relationship.conflict_resolved` | Conflict resolved | ConflictID, Action, UserID, Affected |
| `relationship.merged` | Relationships merged | MergedID, SourceIDs |
| `entity.discovered` | New entity created | EntityID, Type, Name |
| `network.cluster_detected` | New cluster found | ClusterID, MemberCount |

## Conflict Resolution Rules

The conflict resolver implements the following rules from the original specification:

1. **Auto-resolve if confidence gap > 30%**: When conflicting relationships have a confidence difference greater than 30%, the system automatically keeps the highest confidence relationship and rejects the others.

2. **Escalate to user if confidence gap ≤ 30%**: When the confidence gap is 30% or less, the conflict requires user review to determine the correct resolution.

3. **Auto-merge duplicates**: Exact duplicate relationships (same source, target, and type) are automatically merged, combining their evidence and recalculating confidence.

4. **2-year retention**: Relationships are retained for 2 years before being automatically archived. Historical relationships transition to archived state after this period.
