# Content Processor Specification

## Overview

The Content Processor handles AI-powered content analysis including entity extraction, categorization, and summarization.

## Status: Planned (Phase 3)

## Responsibilities

1. **Entity Extraction**: People, projects, decisions, dates
2. **Categorization**: Content type classification
3. **Summarization**: Generate content summaries
4. **Confidence Scoring**: Rate AI output confidence
5. **Job Processing**: Handle processing jobs from Event Router

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Content Processor                         │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Entity     │    │ Categorizer  │    │ Summarizer   │  │
│  │  Extractor   │    │              │    │              │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│         │                   │                   │           │
│         └───────────────────┼───────────────────┘           │
│                             ▼                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  AI Coordinator                      │   │
│  │            (model selection, routing)               │   │
│  └─────────────────────────────────────────────────────┘   │
│                             │                                │
│              ┌──────────────┼──────────────┐                │
│              ▼              ▼              ▼                 │
│         vLLM-MLX       Gemini API    Embedding             │
│          (:8000)        (cloud)      Pipeline              │
└─────────────────────────────────────────────────────────────┘
```

## gRPC Service

```protobuf
service ContentService {
  rpc ProcessContent(ProcessContentRequest) returns (ProcessContentResponse);
  rpc ExtractEntities(ExtractEntitiesRequest) returns (ExtractEntitiesResponse);
  rpc Categorize(CategorizeRequest) returns (CategorizeResponse);
  rpc Summarize(SummarizeRequest) returns (SummarizeResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

## Processing Pipeline

1. **Receive job** from Event Router
2. **Preprocess** content (normalize, chunk if needed)
3. **Extract entities** using AI
4. **Categorize** content type
5. **Generate summary** if requested
6. **Score confidence** for all outputs
7. **Store results** in PostgreSQL
8. **Publish events** for downstream services

## Events Published

- `content.processed` - Processing complete
- `entities.extracted` - Entities found
- `content.categorized` - Category assigned
