# Embedding Pipeline Specification

## Overview

The Embedding Pipeline generates 768-dimensional vector embeddings for content using the MLX sidecar service (nomic-embed-text model).

## Status: In Progress

This service is currently implemented in `penfold-go-pipeline/`.

## Architecture

```
┌───────────────────────────────────────────────────────┐
│                 Embedding Pipeline                     │
│                                                        │
│   Redis Events ─────▶ Worker Pool ─────▶ MLX Sidecar  │
│   (content.ingested)                        (:8001)    │
│                            │                           │
│                            ▼                           │
│                     PostgreSQL + pgvector              │
└───────────────────────────────────────────────────────┘
```

## Event Subscriptions

- `content.ingested` - New content requiring embeddings
- `email.ingested` - Email content
- `manual_email.ingested` - Manually ingested emails

## gRPC Service

```protobuf
service EmbeddingService {
  rpc GetEmbedding(GetEmbeddingRequest) returns (GetEmbeddingResponse);
  rpc GetBatchEmbeddings(GetBatchEmbeddingsRequest) returns (GetBatchEmbeddingsResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
}

message GetEmbeddingRequest {
  string text = 1;
}

message GetEmbeddingResponse {
  repeated float embedding = 1;  // 768 dimensions
  int64 latency_ms = 2;
}
```

## Configuration

```yaml
server:
  grpc_port: 8001
  metrics_port: 9001

mlx:
  address: "localhost:8001"  # MLX sidecar
  model: "nomic-embed-text-1.5"
  timeout: "10s"

worker:
  concurrency: 4
  batch_size: 10

database:
  # Connection to PostgreSQL for storing embeddings
```

## Implementation

See `penfold-go-pipeline/` for current implementation.
