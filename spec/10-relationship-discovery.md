# Relationship Discovery Specification

## Overview

The Relationship Discovery service automatically extracts and manages relationships between entities (people, projects, decisions) with confidence scoring and conflict resolution.

## Status: Planned (Phase 4)

## Responsibilities

1. **Extraction**: Discover relationships from content
2. **Confidence Scoring**: Multi-factor confidence calculation
3. **Conflict Resolution**: Handle contradictory relationships
4. **Lifecycle Management**: Track relationship state changes
5. **Network Analysis**: Build collaboration graphs

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                 Relationship Discovery                       │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ Relationship │    │  Confidence  │    │   Conflict   │  │
│  │  Extractor   │    │   Scorer     │    │  Resolver    │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│         │                   │                   │           │
│         └───────────────────┼───────────────────┘           │
│                             ▼                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                Network Analyzer                      │   │
│  │        (clusters, hubs, paths)                      │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## gRPC Service

```protobuf
service RelationshipService {
  // Discovery
  rpc DiscoverRelationships(DiscoverRelationshipsRequest) returns (DiscoverRelationshipsResponse);

  // Query
  rpc GetRelationships(GetRelationshipsRequest) returns (GetRelationshipsResponse);
  rpc GetRelationshipGraph(GetRelationshipGraphRequest) returns (GetRelationshipGraphResponse);
  rpc FindPath(FindPathRequest) returns (FindPathResponse);

  // Validation
  rpc ValidateRelationship(ValidateRelationshipRequest) returns (ValidateRelationshipResponse);

  // Network analysis
  rpc GetClusters(GetClustersRequest) returns (GetClustersResponse);
  rpc GetHubs(GetHubsRequest) returns (GetHubsResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

## Confidence Scoring

```go
type ConfidenceFactors struct {
    AIConfidence      float32  // 30% weight
    EvidenceStrength  float32  // 40% weight
    EntityResolution  float32  // 15% weight
    Freshness         float32  // 15% weight
}

func (s *Scorer) Calculate(factors ConfidenceFactors) float32 {
    return factors.AIConfidence*0.30 +
           factors.EvidenceStrength*0.40 +
           factors.EntityResolution*0.15 +
           factors.Freshness*0.15
}
```

## Relationship States

```
PENDING → ACTIVE → HISTORICAL → ARCHIVED
    ↓         ↓
 REJECTED  MERGED
```

## Events Published

- `relationship.discovered` - New relationship found
- `relationship.updated` - Confidence changed
- `relationship.validated` - User confirmed
- `relationship.conflict` - Contradiction detected
- `relationship.archived` - Relationship ended

## Network Metrics

- **Degree Centrality**: Connection count
- **Betweenness**: Bridge nodes
- **Clustering Coefficient**: Group density
- **Path Length**: Degrees of separation
