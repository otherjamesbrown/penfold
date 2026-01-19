# Observability

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

Each enrichment stage emits events for:
1. **Operational monitoring** - queue depths, latencies, error rates
2. **Quality tracking** - extraction accuracy, entity resolution rates
3. **Debugging** - trace individual items through pipeline
4. **Business metrics** - content processed, AI costs, entity growth

---

## Event Schemas

Events are published to Redis channels and optionally forwarded to external systems.

### Stage Completion Events

```go
// Emitted after each pipeline stage completes
type EnrichmentStageEvent struct {
    EventID       string    `json:"event_id"`
    TenantID      string    `json:"tenant_id"`
    SourceID      string    `json:"source_id"`
    BatchID       string    `json:"batch_id,omitempty"`
    Stage         string    `json:"stage"`         // classification, common_enrichment, type_specific, ai_routing, ai_processing
    Status        string    `json:"status"`        // completed, failed, skipped
    DurationMs    int64     `json:"duration_ms"`
    ProcessorName string    `json:"processor_name"` // ContentClassifier, ParticipantExtractor, etc.
    Outputs       []string  `json:"outputs"`       // Fields/tables updated
    Timestamp     time.Time `json:"timestamp"`
}

// Redis channel: events.enrichment.stage_completed
```

### Classification Events

```go
type ClassificationEvent struct {
    EventID         string    `json:"event_id"`
    TenantID        string    `json:"tenant_id"`
    SourceID        string    `json:"source_id"`
    ContentType     string    `json:"content_type"`     // email, calendar, document
    ContentSubtype  string    `json:"content_subtype"`  // thread, notification/jira, etc.
    ProcessingProfile string  `json:"processing_profile"` // full_ai, metadata_only, etc.
    DetectionMethod string    `json:"detection_method"` // Which rule/heuristic matched
    Confidence      float64   `json:"confidence"`
    Timestamp       time.Time `json:"timestamp"`
}

// Redis channel: events.enrichment.classified
```

### Entity Resolution Events

```go
type EntityResolutionEvent struct {
    EventID       string    `json:"event_id"`
    TenantID      string    `json:"tenant_id"`
    SourceID      string    `json:"source_id"`
    EntityType    string    `json:"entity_type"`    // person, team, project
    Action        string    `json:"action"`         // resolved, created, flagged_duplicate
    EntityID      string    `json:"entity_id"`
    InputValue    string    `json:"input_value"`    // Email address or name
    Confidence    float64   `json:"confidence"`
    NeedsReview   bool      `json:"needs_review"`
    Timestamp     time.Time `json:"timestamp"`
}

// Redis channel: events.enrichment.entity_resolved
```

### AI Processing Events

```go
type AIProcessingEvent struct {
    EventID         string    `json:"event_id"`
    TenantID        string    `json:"tenant_id"`
    SourceID        string    `json:"source_id"`
    ExtractionRunID string    `json:"extraction_run_id"`
    Operation       string    `json:"operation"`      // embed, summarize, extract
    Model           string    `json:"model"`          // mlx-embed, vllm-qwen
    TemplateID      string    `json:"template_id"`
    TemplateVersion int       `json:"template_version"`
    InputTokens     int       `json:"input_tokens"`
    OutputTokens    int       `json:"output_tokens"`
    LatencyMs       int64     `json:"latency_ms"`
    ParseSuccess    bool      `json:"parse_success"`
    Timestamp       time.Time `json:"timestamp"`
}

// Redis channel: events.enrichment.ai_processed
```

### Error Events

```go
type EnrichmentErrorEvent struct {
    EventID       string            `json:"event_id"`
    TenantID      string            `json:"tenant_id"`
    SourceID      string            `json:"source_id"`
    BatchID       string            `json:"batch_id,omitempty"`
    Stage         string            `json:"stage"`
    ProcessorName string            `json:"processor_name"`
    ErrorType     string            `json:"error_type"`   // parse_error, timeout, quota_exceeded, etc.
    ErrorMessage  string            `json:"error_message"`
    Retryable     bool              `json:"retryable"`
    RetryCount    int               `json:"retry_count"`
    Metadata      map[string]string `json:"metadata,omitempty"`
    Timestamp     time.Time         `json:"timestamp"`
}

// Redis channel: events.enrichment.error
```

---

## Metrics

Metrics are collected via Prometheus-style counters and histograms.

### Queue Metrics

```
# Counter: items entering each queue
enrichment_queue_items_total{queue="ingest|enrichment|ai", tenant_id, priority}

# Gauge: current queue depth
enrichment_queue_depth{queue, tenant_id, priority}

# Histogram: time spent in queue before pickup
enrichment_queue_wait_seconds{queue, tenant_id, priority}

# Counter: dead letter queue additions
enrichment_dlq_items_total{queue, tenant_id, error_type}
```

### Processing Metrics

```
# Counter: items processed per stage
enrichment_items_processed_total{stage, processor, status="success|failed|skipped", tenant_id}

# Histogram: processing latency per stage
enrichment_processing_seconds{stage, processor, tenant_id}

# Counter: classifications by type
enrichment_classifications_total{content_type, content_subtype, processing_profile, tenant_id}
```

### Entity Resolution Metrics

```
# Counter: entity resolutions
enrichment_entity_resolutions_total{entity_type, action="resolved|created|flagged", tenant_id}

# Histogram: entity confidence scores
enrichment_entity_confidence{entity_type, tenant_id}

# Gauge: entities pending review
enrichment_entities_pending_review{entity_type, tenant_id}
```

### AI Processing Metrics

```
# Counter: AI operations
enrichment_ai_operations_total{operation, model, status="success|failed|timeout", tenant_id}

# Histogram: AI latency
enrichment_ai_latency_seconds{operation, model, tenant_id}

# Counter: token usage
enrichment_ai_tokens_total{direction="input|output", model, tenant_id}

# Histogram: extraction parse success rate (rolling)
enrichment_extraction_parse_success_rate{template_id, tenant_id}
```

---

## Tracing

Distributed tracing via OpenTelemetry spans:

```go
// Root span for each source item
span := tracer.Start("enrichment.process_source",
    trace.WithAttributes(
        attribute.String("source_id", sourceID),
        attribute.String("tenant_id", tenantID),
        attribute.String("batch_id", batchID),
    ),
)

// Child spans for each stage
func (p *Pipeline) ProcessStage(ctx context.Context, stage string) error {
    ctx, span := tracer.Start(ctx, fmt.Sprintf("enrichment.stage.%s", stage))
    defer span.End()

    // Stage processing...

    span.SetAttributes(
        attribute.String("processor", processorName),
        attribute.Int64("duration_ms", duration.Milliseconds()),
    )
    return nil
}
```

**Trace IDs flow through:**
1. Ingest → assigned at source creation
2. Queue messages → trace_id in message headers
3. Enrichment stages → propagated via context
4. AI processing → passed to external services

---

## Alerting Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| Queue depth (enrichment) | >1000 | >5000 | Scale workers |
| Queue depth (ai) | >500 | >2000 | Throttle ingest |
| Queue wait time | >5m | >15m | Investigate backlog |
| DLQ items (1h) | >10 | >50 | Review error patterns |
| AI error rate (5m) | >5% | >20% | Check model service |
| AI latency p99 | >5s | >15s | Check model load |
| Entity review backlog | >500 | >2000 | Schedule cleanup |
| Parse failure rate | >10% | >30% | Review template |

---

## Debugging Aids

### Source Enrichment Status

```bash
# View enrichment status for a specific source
penf enrichment status <source_id>
# Output:
# Source: src_abc123
# Content: email/thread (full_ai)
# Pipeline status:
#   ✓ Classification: completed (2ms)
#   ✓ Common Enrichment: completed (45ms)
#     - Participants: 5 extracted, 5 resolved
#     - Links: 3 extracted, 2 new
#     - Thread: joined thread_xyz (5 messages)
#   ✓ Type-Specific: completed (8ms)
#   ✓ AI Routing: routed to full_ai queue
#   ✓ AI Processing: completed (1.2s)
#     - Embedding: 1536 dimensions
#     - Summary: 150 tokens
#     - Assertions: 2 actions, 1 decision
# Total duration: 1.26s
# Errors: none
```

### Trace Lookup

```bash
# Trace a source through the pipeline
penf enrichment trace <source_id>
# Shows timeline of all events for that source

# View recent errors
penf enrichment errors --last 1h
# Output:
# Time       Source       Stage            Error
# 10:45:23   src_abc123   ai_processing    timeout after 30s (retried)
# 10:42:01   src_def456   type_specific    jira API unavailable

# View queue status
penf enrichment queues
# Output:
# Queue        Depth   Oldest   Processing  DLQ
# ingest       23      2m ago   5/min       0
# enrichment   156     8m ago   45/min      2
# ai           89      12m ago  15/min      1
```

### Extraction Audit Replay

```bash
# Replay extraction with current template (compare outputs)
penf enrichment replay <source_id> --template current
# Shows: original output vs new output diff

# Replay with specific template version
penf enrichment replay <source_id> --template-version 3
```

---

## Event Forwarding (Optional)

For integration with external monitoring systems:

```yaml
# config/observability.yaml
events:
  forward_to:
    - type: kafka
      topic: penfold.enrichment.events
      brokers: ["kafka:9092"]
    - type: webhook
      url: https://monitoring.example.com/ingest
      batch_size: 100
      flush_interval: 10s

metrics:
  export_to:
    - type: prometheus
      port: 9090
      path: /metrics
    - type: datadog
      api_key: ${DD_API_KEY}

tracing:
  export_to:
    - type: jaeger
      endpoint: http://jaeger:14268/api/traces
```

---

## Functional Requirements

- **FR-900**: System MUST emit stage completion events after each pipeline stage
- **FR-901**: System MUST include trace_id in all events for correlation
- **FR-902**: System MUST expose Prometheus metrics for queues, processing, and AI operations
- **FR-903**: System MUST support distributed tracing via OpenTelemetry
- **FR-904**: System MUST provide CLI commands for status, trace, and error inspection
- **FR-905**: System MUST track extraction template versions in audit events
- **FR-906**: System MUST support event forwarding to external systems (Kafka, webhooks)
- **FR-907**: System MUST log errors with sufficient context for debugging
- **FR-908**: System MUST maintain alertable metrics for queue health and error rates
