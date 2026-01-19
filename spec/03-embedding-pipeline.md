# Embedding Pipeline Specification

## Overview

The Embedding Pipeline generates 768-dimensional vector embeddings for content using the MLX sidecar service (nomic-embed-text model).

## Status: In Progress

This service is currently implemented in `penfold-go-pipeline/`.

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                    Embedding Pipeline (:8087)                   │
│                                                                 │
│   Redis Events ─────▶ Worker Pool ─────▶ MLX Sidecar (:8001)   │
│   (content.ingested)       │                                    │
│                            │                                    │
│                            ▼                                    │
│                     PostgreSQL + pgvector                       │
└────────────────────────────────────────────────────────────────┘

Port Allocation:
- 8087: Embedding Pipeline gRPC service
- 8001: MLX sidecar (embedding model inference)
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
  grpc_port: 8087        # Embedding Pipeline gRPC service
  metrics_port: 9087

mlx:
  address: "localhost:8001"  # MLX sidecar (embedding model inference)
  model: "nomic-embed-text-1.5"
  timeout: "10s"
  max_batch_size: 32

worker:
  concurrency: 4
  batch_size: 10
  queue_size: 1000

database:
  host: "home-01"
  port: 5432
  name: "penfold"
  pool_size: 10
```

## Implementation

See `penfold-go-pipeline/` for current implementation.
