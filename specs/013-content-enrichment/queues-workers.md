# Queue & Worker Architecture

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

The enrichment pipeline uses a multi-queue architecture to:
- Decouple processing stages
- Enable independent scaling
- Handle backpressure gracefully
- Support prioritization
- Provide error isolation

---

## Queue Design

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           QUEUE ARCHITECTURE                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐           │
│  │  ingest_queue   │     │ enrichment_queue│     │    ai_queue     │           │
│  │  (Redis/SQS)    │     │   (Redis/SQS)   │     │  (Redis/SQS)    │           │
│  └────────┬────────┘     └────────┬────────┘     └────────┬────────┘           │
│           │                       │                       │                     │
│           ▼                       ▼                       ▼                     │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐           │
│  │ Ingest Workers  │────►│Enrichment Workers────►│   AI Workers    │           │
│  │   (N workers)   │     │   (N workers)   │     │   (N workers)   │           │
│  └─────────────────┘     └─────────────────┘     └─────────────────┘           │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Queue Messages

### Ingest Message

Triggers enrichment after source is stored.

```go
type IngestMessage struct {
    SourceID    string    `json:"source_id"`
    TenantID    string    `json:"tenant_id"`
    SourceType  string    `json:"source_type"`  // email, calendar, document
    Priority    int       `json:"priority"`     // 0=low, 1=normal, 2=high
    IngestedAt  time.Time `json:"ingested_at"`
    BatchID     string    `json:"batch_id,omitempty"`  // For batch tracking
}
```

### Enrichment Message

Triggers AI processing after enrichment.

```go
type EnrichmentMessage struct {
    SourceID          string    `json:"source_id"`
    TenantID          string    `json:"tenant_id"`
    ContentType       string    `json:"content_type"`
    ContentSubtype    string    `json:"content_subtype"`
    ProcessingProfile string    `json:"processing_profile"`
    ThreadID          string    `json:"thread_id,omitempty"`
    IsLatestInThread  bool      `json:"is_latest_in_thread"`
    EnrichedAt        time.Time `json:"enriched_at"`
}
```

---

## Worker Configuration

```yaml
# worker_config.yaml
workers:
  ingest:
    count: 4
    queue: "enrichment:ingest"
    batch_size: 10
    visibility_timeout: 60s

  enrichment:
    count: 8
    queue: "enrichment:process"
    batch_size: 1  # Process one at a time for consistency
    visibility_timeout: 120s

  ai:
    count: 4
    queue: "enrichment:ai"
    batch_size: 1
    visibility_timeout: 300s  # AI calls can be slow

queues:
  dead_letter:
    max_retries: 3
    retention: 7d
```

---

## Batch Processing Flow

```
Batch Upload: 100 emails
    │
    ├─ 1. Store all sources (parallel, fast)
    │      └─ Each source gets IngestMessage
    │
    ├─ 2. Thread Detection (batch operation)
    │      └─ Group messages by thread BEFORE individual processing
    │      └─ Mark latest in each thread
    │
    ├─ 3. Enrichment Queue Processing
    │      └─ Process in thread-aware order
    │      └─ Non-thread items: process immediately
    │      └─ Thread items: wait for all thread members, process latest last
    │
    └─ 4. AI Queue Processing
         └─ Only processes items with full_ai profile
         └─ Thread items: only latest gets full extraction
```

---

## Priority Queues

| Priority | Use Case | SLA |
|----------|----------|-----|
| 2 (high) | Real-time Gmail sync, user-triggered re-process | < 30s |
| 1 (normal) | Batch ingest, scheduled processing | < 5min |
| 0 (low) | Backfill, re-enrichment after rule change | Best effort |

---

## Error Handling & Recovery

### Error Categories

| Category | Example | Handling |
|----------|---------|----------|
| `transient` | Network timeout, rate limit | Retry with backoff |
| `permanent` | Invalid email format, missing required field | Dead letter, alert |
| `partial` | Entity resolution failed, links succeeded | Continue, mark incomplete |
| `dependency` | Jira API down | Retry later, don't block pipeline |

### Retry Policy

```go
type RetryPolicy struct {
    MaxRetries     int           // Default: 3
    InitialBackoff time.Duration // Default: 1s
    MaxBackoff     time.Duration // Default: 5m
    BackoffFactor  float64       // Default: 2.0
    RetryableErrors []string     // Error codes to retry
}

var DefaultRetryPolicy = RetryPolicy{
    MaxRetries:     3,
    InitialBackoff: 1 * time.Second,
    MaxBackoff:     5 * time.Minute,
    BackoffFactor:  2.0,
    RetryableErrors: []string{
        "TIMEOUT",
        "RATE_LIMITED",
        "SERVICE_UNAVAILABLE",
    },
}
```

### Partial Failure Handling

```go
type EnrichmentResult struct {
    SourceID string
    Status   EnrichmentStatus // complete, partial, failed

    // Individual processor results
    Stages []StageResult
}

type StageResult struct {
    Stage     string           // "classification", "entity_resolution", etc.
    Status    ProcessorStatus  // success, failed, skipped
    Error     string           // If failed
    Duration  time.Duration
    Output    interface{}      // Stage-specific output
}

// If any stage fails, continue with remaining stages
// Mark enrichment as "partial" so it can be retried/reviewed
```

### Dead Letter Queue

```
┌─────────────────────┐
│  dead_letter_items  │
├─────────────────────┤
│ id                  │
│ tenant_id           │
│ source_id           │
│ queue_name          │  ← Which queue it failed in
│ error_category      │  ← transient/permanent/partial
│ error_message       │
│ error_stack         │
│ retry_count         │
│ last_retry_at       │
│ original_message    │  ← Full message for replay
│ created_at          │
│ resolved_at         │  ← When manually resolved
│ resolution          │  ← retry_succeeded, manually_fixed, ignored
└─────────────────────┘
```

---

## Alerting

| Condition | Alert Level | Action |
|-----------|-------------|--------|
| Dead letter queue > 100 items | Warning | Investigate |
| Dead letter queue > 1000 items | Critical | Page on-call |
| Processing latency > 10x normal | Warning | Check dependencies |
| Error rate > 5% | Warning | Investigate |
| Error rate > 20% | Critical | Pause processing |

---

## Complete Flow Examples

### Jira Notification Example

```
INPUT: Email from "TRACK JIRA <gsd-jira@akamai.com>"
       Subject: "[TRACK-JIRA] Updates for OUT-697: Launch new products..."
       Headers: Auto-Submitted: auto-generated, Precedence: bulk

┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 1: Classification                                                          │
├─────────────────────────────────────────────────────────────────────────────────┤
│ Check: hasCalendarAttachment? NO                                                 │
│ Check: fromContains("jira") AND hasAutoSubmittedHeader? YES ← MATCH             │
│                                                                                  │
│ Result: content_type=email, subtype=notification/jira, profile=metadata_only    │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 2: Common Enrichment (runs for ALL content)                               │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ParticipantExtractor:                                                            │
│   From: gsd-jira@akamai.com                                                     │
│   To: jabrown@akamai.com                                                        │
│                                                                                  │
│ EntityResolver:                                                                  │
│   gsd-jira@akamai.com → person_id: xxx (account_type: bot)                     │
│   jabrown@akamai.com → person_id: yyy (account_type: person)                   │
│                                                                                  │
│ LinkExtractor:                                                                   │
│   Found: https://akamai.atlassian.net/browse/OUT-697                           │
│                                                                                  │
│ LinkCategorizer:                                                                 │
│   OUT-697 link → category: jira_ticket                                         │
│                                                                                  │
│ ThreadGrouper:                                                                   │
│   In-Reply-To: present → thread_id: zzz                                        │
│                                                                                  │
│ ProjectMatcher:                                                                  │
│   OUT-697 → project_id: "tiktok-fy26" (via jira_projects config)              │
│                                                                                  │
│ AttachmentExtractor:                                                             │
│   Found: 2 attachments (inline images)                                          │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 3: Type-Specific Extraction (notification/jira)                           │
├─────────────────────────────────────────────────────────────────────────────────┤
│ JiraExtractor runs:                                                              │
│                                                                                  │
│ Parse notification body for:                                                     │
│   ticket_key: OUT-697                                                           │
│   action: status_changed                                                        │
│   from_status: Open                                                             │
│   to_status: In Progress                                                        │
│   changed_by: Rick Eskelsen → person_id: rrr                                   │
│   changed_at: 2026-01-19T10:30:00Z                                             │
│                                                                                  │
│ Database updates:                                                                │
│   → jira_tickets: UPDATE status='In Progress' WHERE key='OUT-697'              │
│   → jira_ticket_changes: INSERT (ticket_id, action, from, to, by, at, source)  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 4: AI Routing                                                              │
├─────────────────────────────────────────────────────────────────────────────────┤
│ Profile: metadata_only                                                           │
│                                                                                  │
│ Decision: SKIP AI PROCESSING                                                     │
│ Reason: Structured data already extracted by JiraExtractor                      │
│                                                                                  │
│ Set: ai_processed=false, ai_skip_reason="profile:metadata_only"                │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 5: AI Processing                                                           │
├─────────────────────────────────────────────────────────────────────────────────┤
│ SKIPPED - metadata_only profile                                                  │
│                                                                                  │
│ No embedding generated                                                           │
│ No summary generated                                                             │
│ No RAID extraction                                                               │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
OUTPUT:
  - Source stored in sources table
  - content_enrichment record created
  - jira_tickets table updated with current state
  - jira_ticket_changes record shows what changed
  - Link to source email preserved for "show me notification" queries
  - Participants resolved (can query "who was notified about OUT-697")
  - NO embedding (won't pollute vector search)
  - NO LLM cost incurred
```

### Regular Email Thread Example

```
INPUT: Email from "Sabina Sawyer <ssawyer@akamai.com>"
       Subject: "RE: TikTok FY26 discounts"
       Headers: In-Reply-To: <msg-4@...>

┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 1: Classification                                                          │
├─────────────────────────────────────────────────────────────────────────────────┤
│ Check: hasCalendarAttachment? NO                                                 │
│ Check: fromContains("jira") AND hasAutoSubmittedHeader? NO                      │
│ Check: fromMatches("*-noreply@docs.google.com")? NO                             │
│ Check: fromContains("slack")? NO                                                │
│ Check: hasAutoSubmittedHeader? NO                                               │
│ Check: subjectStartsWith("FW:")? NO                                             │
│ Check: hasInReplyTo? YES ← MATCH                                                │
│                                                                                  │
│ Result: content_type=email, subtype=thread, profile=full_ai                     │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 2: Common Enrichment                                                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ParticipantExtractor: From, To, Cc → 6 participants                             │
│ EntityResolver: All resolved to person_ids                                       │
│ LinkExtractor: Found Google Doc link                                            │
│ ThreadGrouper: thread_id found, this is message 5 of 5                          │
│ ProjectMatcher: Matched to "tiktok-fy26" project (by participants + keywords)   │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 3: Type-Specific Extraction (email/thread)                                │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ThreadContextBuilder runs:                                                       │
│                                                                                  │
│ Fetches prior 4 messages in thread                                              │
│ Generates context from each:                                                     │
│   Msg 1: Rick proposed 15% discount                                             │
│   Msg 2: Sabina requested justification                                         │
│   Msg 3: Rick provided competitive analysis                                     │
│   Msg 4: Hrishikesh flagged pricing tool limitation                            │
│                                                                                  │
│ Result: thread_context ready for AI                                             │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 4: AI Routing                                                              │
├─────────────────────────────────────────────────────────────────────────────────┤
│ Profile: full_ai                                                                 │
│                                                                                  │
│ Decision: PROCEED TO AI PROCESSING                                              │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 5: AI Processing                                                           │
├─────────────────────────────────────────────────────────────────────────────────┤
│ 5a. ContextBuilder:                                                              │
│     participants: [Rick (Sales), Sabina (Finance), Hrishikesh (Pricing)]        │
│     project: TikTok FY26 (OUT-697, OUT-698)                                     │
│     thread_context: summaries of messages 1-4                                   │
│     prior_decisions: "MTC pricing floor set at $X"                              │
│                                                                                  │
│ 5b. TemplateResolver:                                                            │
│     project "tiktok-fy26" has template? YES → use "tiktok-fy26" template       │
│                                                                                  │
│ 5c. PromptBuilder:                                                               │
│     Combines template + context + email body                                    │
│                                                                                  │
│ 5d. LLM Call:                                                                    │
│     Model: gpt-4-turbo                                                          │
│     Input tokens: 1,847                                                         │
│     Output tokens: 423                                                          │
│     Latency: 2.3s                                                               │
│                                                                                  │
│ 5e. ResponseParser:                                                              │
│     decisions: [{description: "Approved 15%", maker: "Sabina" → person_id}]    │
│     actions: [{description: "VP escalation", assignee: "Rick" → person_id}]    │
│     commitments: [{description: "Numbers by Wednesday", by: "Rick"}]           │
│                                                                                  │
│ 5f. AuditLog:                                                                    │
│     Records full prompt, response, parsed result, model, tokens                 │
│                                                                                  │
│ 5g. ExtractionStore:                                                             │
│     Saves to assertions table with source_id, project_id, person_ids           │
│                                                                                  │
│ 5h. Embedder:                                                                    │
│     Generates embedding for this message                                        │
│     Updates thread summary embedding                                            │
│                                                                                  │
│ 5i. Summarizer:                                                                  │
│     Generates message summary                                                    │
│     Updates thread-level summary                                                │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
OUTPUT:
  - Source stored
  - content_enrichment with full metadata
  - assertions table: decisions, actions, commitments (grounded to person_ids)
  - embeddings table: message embedding + thread summary embedding
  - summaries table: message summary + thread summary
  - extraction_runs: full audit trail
```

---

## Functional Requirements

- **FR-300**: System MUST process enrichment stages in order: classify → extract links → resolve entities → type-specific → AI
- **FR-301**: System MUST support skipping AI processing for configured content types (e.g., notifications)
- **FR-302**: System MUST emit events after each enrichment stage for observability
- **FR-303**: System MUST support re-processing content through enrichment pipeline when rules change
- **FR-304**: System MUST track enrichment status separately from source ingest status

### Non-Functional Requirements

- **NFR-001**: Entity resolution MUST complete within 100ms for single content item
- **NFR-002**: Classification MUST complete within 50ms per content item
- **NFR-003**: Enrichment pipeline MUST be idempotent - re-running produces same results
- **NFR-004**: System MUST support processing 1000 items/minute through enrichment pipeline
