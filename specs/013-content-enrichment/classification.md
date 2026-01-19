# Content Classification

Part of [Content Enrichment Pipeline](spec.md)

---

## Type Hierarchy

Content types use a **hierarchical structure** where the primary type defines the source format and subtypes define handling behavior:

```
content_type (primary)    subtype              processing_profile
─────────────────────────────────────────────────────────────────
email                     thread               full_ai
email                     forward              full_ai
email                     standalone           full_ai
email                     notification/jira    metadata_only
email                     notification/google  metadata_only
email                     notification/slack   metadata_only
email                     notification/other   metadata_only
calendar                  invite               state_tracking
calendar                  cancellation         state_tracking
calendar                  update               state_tracking
calendar                  response             state_tracking
document                  google_doc           full_ai_chunked
document                  pdf                  full_ai_chunked
document                  office               full_ai_chunked
attachment                document             full_ai_chunked
attachment                spreadsheet          structure_only
attachment                image                ocr_if_text
attachment                other                metadata_only
```

**Key insight**: A Jira notification IS an email - we still extract participants, links, and store the content. But the `notification/jira` subtype tells us to:
- Skip expensive AI embedding/summarization
- Extract Jira-specific metadata
- Update the `jira_tickets` state table

---

## Detection Heuristics (by subtype)

| Subtype | Detection Method | Priority |
|---------|------------------|----------|
| `calendar/invite` | `Content-Type: text/calendar` or `.ics` attachment | 1 |
| `calendar/cancellation` | Subject starts with "Canceled:" or "Cancelled:" | 1 |
| `calendar/response` | Subject starts with "Accepted:", "Declined:", or "Tentative:" | 1 |
| `calendar/update` | Subject starts with "Updated:" and has calendar attachment | 1 |
| `notification/jira` | From contains `jira` AND (`Auto-Submitted` header OR `Precedence: bulk`) | 2 |
| `notification/google` | From matches `*@docs.google.com` or `*@google.com` notification addresses | 2 |
| `notification/slack` | From contains `slack` or `@slack.com` | 2 |
| `notification/other` | Header `Auto-Submitted: auto-generated` or `Precedence: bulk` (not matched above) | 3 |
| `email/forward` | Subject starts with "FW:" or "Fwd:" (case-insensitive) | 4 |
| `email/thread` | Has `In-Reply-To` or `References` headers | 5 |
| `email/standalone` | None of the above | 6 |

**Classification Priority**: Lower number = higher priority. Process rules in order, first match wins.

---

## Processing Profiles

| Profile | Embedding | Summary | Assertions | Special Handling |
|---------|-----------|---------|------------|------------------|
| `full_ai` | ✅ Full | ✅ Yes | ✅ Yes | Standard processing |
| `full_ai_chunked` | ✅ Chunked | ✅ Yes | ✅ Yes | Section/page-aware |
| `metadata_only` | ❌ Skip | ❌ Skip | ❌ Skip | Extract structured data only |
| `state_tracking` | ❌ Skip | ❌ Skip | ❌ Skip | Update state tables |
| `structure_only` | ❌ Skip | ⚠️ Schema only | ❌ Skip | Headers + sample rows |
| `ocr_if_text` | ⚠️ If text found | ⚠️ If text found | ❌ Skip | Apply OCR first |

---

## What's Shared Across All Email Subtypes

Even `notification/jira` emails get these enrichments:
- **Participant extraction**: From, To, Cc resolved to people
- **Link extraction**: All URLs in body/HTML extracted
- **Thread membership**: In-Reply-To/References tracked
- **Content storage**: Raw content stored in sources
- **Internal/external detection**: Participants categorized

Only the AI processing (embedding, summarization) is skipped for notifications.

---

## Stage 1: Classification Algorithm

**Input:** Raw source content
**Output:** `content_type`, `content_subtype`, `processing_profile`

```
classifyContent(source):

    // Check headers and patterns in priority order

    // Priority 1: Calendar content
    if hasCalendarAttachment(source) or contentTypeIs(source, "text/calendar"):
        if subjectStartsWith(source, ["Canceled:", "Cancelled:"]):
            return (calendar, cancellation, state_tracking)
        if subjectStartsWith(source, ["Accepted:", "Declined:", "Tentative:"]):
            return (calendar, response, state_tracking)
        if subjectStartsWith(source, "Updated:"):
            return (calendar, update, state_tracking)
        return (calendar, invite, state_tracking)

    // Priority 2: Jira notifications
    if fromContains(source, "jira") AND hasAutoSubmittedHeader(source):
        return (email, notification/jira, metadata_only)

    // Priority 3: Google notifications
    if fromMatches(source, "*-noreply@docs.google.com"):
        return (email, notification/google, metadata_only)

    // Priority 4: Slack notifications
    if fromContains(source, "slack"):
        return (email, notification/slack, metadata_only)

    // Priority 5: Other automated
    if hasAutoSubmittedHeader(source) OR hasPrecedenceBulk(source):
        return (email, notification/other, metadata_only)

    // Priority 6: Forwards
    if subjectStartsWith(source, ["FW:", "Fwd:"]):
        return (email, forward, full_ai)

    // Priority 7: Thread replies
    if hasInReplyTo(source) OR hasReferences(source):
        return (email, thread, full_ai)

    // Priority 8: Default
    return (email, standalone, full_ai)
```

---

## Jira Detection Refinement

The naive regex `\[[A-Z]+-\d+\]` produces false positives:
- `PM-2`, `UTC-05`, `PACE-2026` matched incorrectly
- `Y-8` from embedded content matched incorrectly

**Refined approach**:
1. Check sender first: `*jira*` in from address
2. Require `Auto-Submitted` or `Precedence: bulk` header
3. Only then extract ticket IDs from subject using `\[([A-Z]{2,10}-\d+)\]` pattern

---

## Internal vs External Detection

Use Exchange header when available:
- `X-MS-Exchange-Organization-AuthAs: Internal` → sender is internal
- `X-MS-Exchange-Organization-AuthAs: Anonymous` → sender is external

Fallback: domain pattern matching against configured internal domains.

---

## Type-Specific Extraction

| Content Type | Extracted Fields |
|--------------|------------------|
| `calendar/*` | `meeting_id`, `organizer`, `attendees[]`, `start_time`, `end_time`, `location`, `recurrence` |
| `notification/jira` | `ticket_id`, `project_key`, `status_change`, `assignee`, `reporter` |
| `notification/google` | `service`, `document_id`, `action`, `mentioned_people[]` |
| `email/*` | `thread_id`, `links[]`, `mentioned_people[]`, `action_items[]` |

---

## Processing Pipeline Stages

### Stage 2: Common Enrichment (ALL content)

**These processors run for EVERY content type, including notifications:**

| Processor | What It Does | Output |
|-----------|--------------|--------|
| `ParticipantExtractor` | Extract From/To/Cc addresses | `participants[]` |
| `EntityResolver` | Resolve addresses → person_ids | `resolved_participants[]` |
| `InternalExternalClassifier` | Mark internal vs external | `is_internal` flag per participant |
| `LinkExtractor` | Extract all URLs from body/HTML | `extracted_links[]` |
| `LinkCategorizer` | Categorize links (google_doc, jira, etc.) | `link.category` |
| `ThreadGrouper` | Group into thread via In-Reply-To/References | `thread_id` |
| `ProjectMatcher` | Match to project by participants/keywords | `project_id` (may be null) |
| `AttachmentExtractor` | Extract attachment metadata | `attachments[]` |

```
enrichContent(source, classification):

    enrichment = new ContentEnrichment(source.id, classification)

    // These run for ALL content types
    enrichment.participants = ParticipantExtractor.extract(source)
    enrichment.resolved_participants = EntityResolver.resolve(enrichment.participants)
    enrichment.internal_external = InternalExternalClassifier.classify(enrichment.resolved_participants)
    enrichment.links = LinkExtractor.extract(source)
    enrichment.links = LinkCategorizer.categorize(enrichment.links)
    enrichment.thread_id = ThreadGrouper.group(source)
    enrichment.project_id = ProjectMatcher.match(source, enrichment)
    enrichment.attachments = AttachmentExtractor.extract(source)

    return enrichment
```

### Stage 3: Type-Specific Extraction

**These processors run based on content_subtype:**

| Subtype | Processor | What It Extracts | Output Table |
|---------|-----------|------------------|--------------|
| `notification/jira` | `JiraExtractor` | Ticket ID, status change, assignee | `jira_tickets`, `jira_ticket_changes` |
| `notification/google` | `GoogleNotificationExtractor` | Doc ID, action, mentions | `extracted_links` (enriched) |
| `notification/slack` | `SlackExtractor` | Channel, thread link | `slack_references` |
| `calendar/*` | `CalendarExtractor` | Meeting UID, attendees, times | `meetings`, `meeting_attendees` |
| `email/forward` | `ForwardExtractor` | Original sender (from quoted content) | `forwarded_from_person_id` |
| `email/thread` | `ThreadContextBuilder` | Prior message summaries | `thread_context` |
| `email/standalone` | (none) | - | - |

### Stage 4: AI Routing

**Decides what AI processing to apply based on `processing_profile`:**

| Profile | Embed? | Summarize? | Extract RAID? | Why |
|---------|--------|------------|---------------|-----|
| `full_ai` | ✅ Yes | ✅ Yes | ✅ Yes | High-value content |
| `metadata_only` | ❌ No | ❌ No | ❌ No | Structured data already extracted |
| `state_tracking` | ❌ No | ❌ No | ❌ No | State machine updates only |
| `full_ai_chunked` | ✅ Chunked | ✅ Yes | ✅ Yes | Large documents |
| `structure_only` | ❌ No | ⚠️ Schema | ❌ No | Spreadsheets |

```
routeToAI(source, enrichment):

    switch enrichment.processing_profile:

        case "metadata_only":
        case "state_tracking":
            // NO AI processing
            // Type-specific extraction already captured the value
            enrichment.ai_processed = false
            enrichment.ai_skip_reason = "profile:" + enrichment.processing_profile
            return enrichment

        case "full_ai":
            // Proceed to Stage 5
            return sendToAIProcessing(source, enrichment)

        case "full_ai_chunked":
            // Chunk first, then Stage 5
            chunks = Chunker.chunk(source, enrichment)
            return sendToAIProcessingChunked(chunks, enrichment)
```

### Stage 5: AI Processing

**For `full_ai` and `full_ai_chunked` profiles only:**

```
processWithAI(source, enrichment):

    // 5a. Build context from enrichment
    context = ContextBuilder.build(enrichment)
    // Includes: participants, project, thread history, prior decisions

    // 5b. Resolve template
    template = TemplateResolver.resolve(source, enrichment)
    // Falls back: project → tenant default → system default

    // 5c. Build prompt
    prompt = PromptBuilder.build(template, context, source.content)

    // 5d. Call LLM
    response = LLM.call(prompt)

    // 5e. Parse and ground response
    extraction = ResponseParser.parse(response, enrichment)
    // Resolves names → person_ids using enrichment.resolved_participants

    // 5f. Create audit record
    AuditLog.record(source, enrichment, template, prompt, response, extraction)

    // 5g. Store extractions
    ExtractionStore.save(extraction)

    // 5h. Generate embeddings
    Embedder.embed(source, enrichment)

    // 5i. Generate summary
    Summarizer.summarize(source, enrichment, extraction)

    enrichment.ai_processed = true
    return enrichment
```

---

## Processor Registry

Single configuration that defines what processors apply at each stage:

```yaml
# processor_config.yaml

stages:
  classification:
    processor: ContentClassifier
    input: source
    output: content_type, content_subtype, processing_profile

  common_enrichment:
    # These run for ALL content regardless of type
    processors:
      - ParticipantExtractor
      - EntityResolver
      - InternalExternalClassifier
      - LinkExtractor
      - LinkCategorizer
      - ThreadGrouper
      - ProjectMatcher
      - AttachmentExtractor

  type_specific:
    # Keyed by content_subtype
    "notification/jira":
      processor: JiraExtractor
      outputs: [jira_tickets, jira_ticket_changes]

    "notification/google":
      processor: GoogleNotificationExtractor
      outputs: [link_enrichment]

    "notification/slack":
      processor: SlackExtractor
      outputs: [slack_references]

    "calendar/invite":
      processor: CalendarExtractor
      outputs: [meetings, meeting_attendees, meeting_events]

    "calendar/cancellation":
      processor: CalendarExtractor
      outputs: [meetings, meeting_attendees, meeting_events]

    "calendar/update":
      processor: CalendarExtractor
      outputs: [meetings, meeting_attendees, meeting_events]

    "calendar/response":
      processor: CalendarExtractor
      outputs: [meetings, meeting_attendees, meeting_events]

    "email/forward":
      processor: ForwardExtractor
      outputs: [forwarded_from_person_id]

    "email/thread":
      processor: ThreadContextBuilder
      outputs: [thread_context]

    "email/standalone":
      processor: null  # No type-specific processing

  ai_routing:
    # Keyed by processing_profile
    "metadata_only":
      skip_ai: true
      reason: "Structured extraction only"

    "state_tracking":
      skip_ai: true
      reason: "State machine updates only"

    "full_ai":
      skip_ai: false
      processors: [ContextBuilder, TemplateResolver, LLMExtractor, Embedder, Summarizer]

    "full_ai_chunked":
      skip_ai: false
      preprocessor: Chunker
      processors: [ContextBuilder, TemplateResolver, LLMExtractor, Embedder, Summarizer]

    "structure_only":
      skip_ai: true
      processors: [SchemaExtractor]
      reason: "Spreadsheet structure only"
```

---

## Functional Requirements

### Content Classification

- **FR-001**: System MUST classify content with primary type (email, calendar, document, attachment) and subtype (thread, notification/jira, invite, etc.)
- **FR-002**: System MUST assign processing profile based on subtype (full_ai, metadata_only, state_tracking, etc.)
- **FR-003**: System MUST extract type-specific metadata based on classification (meeting details, ticket IDs, etc.)
- **FR-004**: System MUST support user override of auto-classification
- **FR-005**: System MUST detect auto-generated content via headers (`Auto-Submitted`, `X-Auto-Response-Suppress`, sender patterns)
- **FR-006**: System SHOULD support configurable classification rules per tenant
- **FR-007**: System MUST store content_type, content_subtype, and processing_profile in content metadata
- **FR-008**: System MUST apply shared enrichments (participants, links) to ALL email subtypes including notifications

### Processing Pipeline

- **FR-300**: System MUST process enrichment stages in order: classify → extract links → resolve entities → type-specific → AI
- **FR-301**: System MUST support skipping AI processing for configured content types (e.g., notifications)
- **FR-302**: System MUST emit events after each enrichment stage for observability
- **FR-303**: System MUST support re-processing content through enrichment pipeline when rules change
- **FR-304**: System MUST track enrichment status separately from source ingest status

### Non-Functional Requirements

- **NFR-002**: Classification MUST complete within 50ms per content item
