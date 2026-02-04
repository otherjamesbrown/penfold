# Penfold: Content Processing Pipeline

## Overview

Content flows through Penfold via a Temporal-based workflow engine. The current pipeline has 8 stages, executed as activities within a saga pattern that supports compensation (rollback) on failure.

## Current Pipeline

```
Content Ingested
       │
       ▼
┌─────────────┐
│ 1. Validate │ Fail-fast validation with rejection reasons
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 2. Fetch    │ Retrieve source from database
│   Content   │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 3. Generate │ CRITICAL — Required for search
│  Embedding  │ Uses MLX local model (mxbai-embed-large-v1)
└──────┬──────┘ 1024-dimension vectors
       │
       ▼
┌─────────────┐
│ 4. Generate │ OPTIONAL — Failure doesn't stop workflow
│  Summary    │ Uses local 7B SLM
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 5. Extract  │ OPTIONAL — Named Entity Recognition
│  Entities   │ People, organizations, locations
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 6. Extract  │ OPTIONAL — Theme/topic identification
│  Topics     │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 7. Extract  │ Resolve mentions to known entities
│  Mentions   │ Creates review queue items for ambiguous cases
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 8. Update   │ Mark complete/failed with tracing
│  Status     │
└──────┬──────┘
       │
       ▼
   Searchable
```

### Fault Tolerance

- **Embedding generation**: CRITICAL — workflow fails if this fails (content must be searchable)
- **Summary, entities, topics**: OPTIONAL — failures logged but workflow continues
- **Saga pattern**: Each stage pushes compensation actions; on failure, compensation stack unwinds

### Activity Configuration

Activities have different timeout/retry profiles:
- Fast activities: Lower timeout, more retries
- Embedding activities: Dedicated timeout
- LLM activities: Longer timeout (120+ seconds)
- Non-retryable errors: Validation failures, invalid state

## Content Classification

Before full processing, content is classified to determine its processing profile:

| Profile | Description | AI Steps |
|---------|-------------|----------|
| full_ai | Standard content — full AI pipeline | Embed, summarize, extract, resolve |
| full_ai_chunked | Long content — needs chunking | Chunk then full AI on each chunk |
| metadata_only | Low-value content (auto-replies, receipts) | Embed only, skip AI |
| state_tracking | Status updates, calendar changes | Metadata extraction, minimal AI |
| structure_only | Structured documents | Parse structure, embed sections |
| ocr_if_text | Images/scans with possible text | OCR then standard pipeline |

## Content Types and Handling

### Emails (.eml)

**Ingestion**: Batch .eml file ingestion via `penf ingest email ./archive/ --source "tag"`

**Key processing**:
1. MIME parsing — extract text/plain and text/html bodies
2. Header extraction — From, To, CC, Subject, Date, Message-ID, In-Reply-To, References
3. Attachment extraction — separate content items for significant attachments
4. Thread reconstruction — group by In-Reply-To/References chains
5. Participant resolution — match email addresses to known people
6. Quoted reply detection — strip quoted text to find new content

**Real-world characteristics** (validated against 267 test emails):
- Median plain text: 2,036 characters
- 73% under 5K characters (well within SLM context)
- 95% under 20K characters
- Maximum: 69,684 characters (structured status report)
- File size is misleading: 5.7MB email = only 18K chars actual text (rest is base64 attachments)

### Meeting Transcripts

**Ingestion**: `penf ingest meeting ./transcripts/`

**Key processing**:
1. Speaker identification — extract speaker labels
2. Timestamp extraction — for navigation
3. Participant resolution — match speaker names to known people
4. Topic segmentation (planned) — identify topic boundaries
5. Action item detection (planned) — find commitments and follow-ups

**Real-world characteristics** (validated against 18 test transcripts):
- Consistent 44-64K characters
- Always exceed 7B SLM context window (~12K chars for Qwen 7B at 4-bit)
- Require chunking strategy (map-reduce or sliding window)
- Speaker labels provide natural chunk boundaries

### Calendar Events (iCal)

**Processing**: Metadata extraction, attendee resolution, series grouping
- Meeting series support for recurring meetings
- Attendee response tracking (accepted/declined/tentative)
- Event lifecycle (invite_sent → cancelled/updated → started → ended)

### Attachments

**Processing tiers**:
| Tier | Description | Examples |
|------|-------------|---------|
| auto_process | Always process | PDF, DOCX, XLSX under 5MB |
| auto_skip | Never process | Images, archives, binaries |
| pending_review | Human decides | Large documents, unknown types |
| manual_process | User requested | Specific files user wants indexed |
| manual_skip | User declined | Files user explicitly excluded |

## Email Thread Reconstruction

Emails are grouped into threads using the `email_threads` and `thread_messages` tables:

```
email_threads:
  root_message_id    → First message in thread
  subject            → Thread subject
  participant_ids    → All people involved
  message_count      → Number of messages
  latest_source_id   → Most recent message
  thread_summary     → AI-generated thread summary

thread_messages:
  thread_id          → Parent thread
  source_id          → The email content
  position_in_thread → Ordering
  is_reply           → Whether it's a reply
  reply_to_message_id → What it replies to
```

Thread context is provided to AI during extraction so the model understands the conversation arc.

## Search

Penfold supports three search modes:

### 1. Semantic Search
Uses embedding vectors (1024-dim, cosine similarity) to find conceptually similar content regardless of exact wording.

### 2. Keyword Search
Traditional text matching with PostgreSQL full-text search.

### 3. Hybrid Search
Combines semantic and keyword with configurable weights. This is the default and most effective mode.

### Query Expansion
Before search execution, queries are expanded using the glossary:
- "TER meeting notes" → "(TER OR 'Technical Execution Review') meeting notes"
- Expansion happens at both index time (enrichment) and query time (search)

### Search Filters
- Content type (email, meeting, document, calendar)
- Date range
- Source tag
- Participants
- Project

## Ingest Job Tracking

Batch ingestion is tracked via the `ingest_jobs` table:
- Resume capability — tracks last_file_path for interrupted jobs
- Per-file error tracking via `ingest_errors`
- Progress monitoring (total_items, processed_items, failed_items)
- Labels for organization

## Link Extraction and Enrichment

During enrichment, links are extracted from content and deduplicated:
- Category detection (Google Docs, Jira tickets, Confluence, Zoom recordings, etc.)
- Occurrence counting across content items
- Optional enrichment via external APIs (title, status, owner)
- Signature detection to filter out boilerplate links

## Dead Letter Queue

Failed processing is captured in the dead_letter_items table:
- Error categorization (transient/permanent/partial/dependency)
- Retry scheduling with backoff
- Resolution tracking (retry_succeeded, manually_fixed, ignored, auto_expired)
- Batch association for grouped failures

## Proposed Pipeline Improvements

The SLM/LLM architecture guide (`guide.md`) proposes significant enhancements, all grounded in the Human + AI Collaboration model (see `00-overview.md`):

1. **Tiered processing**: Route cheap tasks (classification, extraction) to local SLM, expensive tasks (deep analysis, synthesis) to remote LLM
2. **Progressive availability**: Content becomes searchable after embedding (stage 3) rather than waiting for full pipeline
3. **Map-reduce for long content**: Chunk transcripts, extract per-chunk, merge results
4. **Context assembly**: Inject known entities, glossary, project context into AI prompts
5. **Knowledge feedback loop**: Persist extracted assertions back to DB, use them as context for future processing
6. **Assertion lifecycle tracking**: Track risks/decisions across content items with versioning chains
7. **Seniority and trust weighting**: Assertions carry weight based on who said them — organizational seniority (title/level) and human-assigned trust scores
8. **Watch list and spotlight**: Human-curated set of assertions/topics under active attention, with annotations (gut feel, offline context, priority overrides)
9. **Peripheral change detection**: AI monitors non-spotlight items and alerts on pattern changes — especially seniority shifts (VP joins a previously junior discussion)
10. **Bidirectional prompting**: Claude proactively surfaces items for human input ("3 new risks this week — what are your thoughts?") rather than only responding to queries
11. **On-demand briefings**: When something enters the spotlight, the system assembles full context instantly (origin, timeline, people, escalation chain, current state)
