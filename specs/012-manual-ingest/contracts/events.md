# Event Contracts: Manual Content Ingest

**Feature**: 012-manual-ingest | **Date**: 2026-01-16

## Overview

Event schemas for the manual ingest framework. These events integrate with the existing pub/sub infrastructure (Redis) and trigger the AI processing pipeline.

---

## Events

### 1. ManualEmailIngestedEvent

Published when a single email is successfully ingested from a .eml file.

**Channel**: `events.content.ingested`

**Schema:**
```python
class ManualEmailIngestedEvent(BaseEvent):
    """Event published when an email is ingested from .eml file."""

    event_type: Literal["manual_email.ingested"] = "manual_email.ingested"

    # Identifiers
    source_id: int              # Database ID in sources table
    tenant_id: UUID
    message_id: str             # Email Message-ID (real or synthetic)
    job_id: UUID                # Parent IngestJob ID

    # Email metadata
    from_email: str
    from_name: Optional[str]
    to_emails: list[str]
    cc_emails: list[str] = []
    subject: Optional[str]
    email_date: datetime        # Original or fallback date
    date_is_fallback: bool = False

    # Threading
    in_reply_to: Optional[str]
    thread_id: Optional[UUID]   # If linked to existing thread

    # Content info
    has_attachments: bool
    attachment_count: int = 0
    content_hash: str           # SHA-256 of raw .eml

    # Source tracking
    source_tag: str             # User-provided source identifier
    original_file_path: str     # Path in upload batch
    labels: list[str] = []      # From folder structure

    # Timestamps
    ingested_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
```

**Example:**
```json
{
  "event_type": "manual_email.ingested",
  "event_id": "evt_abc123",
  "correlation_id": "job_xyz789",
  "source_id": 12345,
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "<unique123@example.com>",
  "job_id": "660e8400-e29b-41d4-a716-446655440001",
  "from_email": "bob@example.com",
  "from_name": "Bob Smith",
  "to_emails": ["alice@example.com"],
  "cc_emails": [],
  "subject": "Re: Project Update",
  "email_date": "2026-01-10T14:30:00Z",
  "date_is_fallback": false,
  "in_reply_to": "<parent456@example.com>",
  "thread_id": "770e8400-e29b-41d4-a716-446655440002",
  "has_attachments": true,
  "attachment_count": 2,
  "content_hash": "abc123def456...",
  "source_tag": "outlook-archive",
  "original_file_path": "inbox/customer-issue.eml",
  "labels": ["inbox"],
  "ingested_at": "2026-01-16T10:30:00Z"
}
```

---

### 2. IngestJobProgressEvent

Published periodically during batch processing to track progress.

**Channel**: `events.ingest.progress`

**Schema:**
```python
class IngestJobProgressEvent(BaseEvent):
    """Event published during batch ingest progress."""

    event_type: Literal["ingest_job.progress"] = "ingest_job.progress"

    # Identifiers
    job_id: UUID
    tenant_id: UUID

    # Progress stats
    total_files: int
    processed_count: int
    imported_count: int
    skipped_count: int
    failed_count: int

    # Current file
    current_file: Optional[str]

    # Timing
    elapsed_seconds: float
    estimated_remaining_seconds: Optional[float]

    # Status
    status: Literal["in_progress", "completing"]
```

**Example:**
```json
{
  "event_type": "ingest_job.progress",
  "job_id": "660e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "total_files": 150,
  "processed_count": 75,
  "imported_count": 70,
  "skipped_count": 3,
  "failed_count": 2,
  "current_file": "inbox/email_76.eml",
  "elapsed_seconds": 45.2,
  "estimated_remaining_seconds": 43.0,
  "status": "in_progress"
}
```

---

### 3. IngestJobCompletedEvent

Published when a batch ingest job finishes (success or with errors).

**Channel**: `events.ingest.completed`

**Schema:**
```python
class IngestJobCompletedEvent(BaseEvent):
    """Event published when ingest job completes."""

    event_type: Literal["ingest_job.completed"] = "ingest_job.completed"

    # Identifiers
    job_id: UUID
    tenant_id: UUID
    source_tag: str

    # Final stats
    total_files: int
    imported_count: int
    skipped_count: int
    failed_count: int

    # Timing
    started_at: datetime
    completed_at: datetime
    duration_seconds: float

    # Status
    success: bool               # True if failed_count == 0
    final_status: Literal["completed", "completed_with_errors", "failed"]

    # Labels created
    labels_created: list[str]

    # Attachments
    attachments_extracted: int
    attachments_skipped: int
```

---

### 4. AttachmentExtractedEvent

Published when an attachment is successfully extracted from an email.

**Channel**: `events.content.attachment_extracted`

**Schema:**
```python
class AttachmentExtractedEvent(BaseEvent):
    """Event published when email attachment is extracted."""

    event_type: Literal["attachment.extracted"] = "attachment.extracted"

    # Identifiers
    attachment_id: UUID
    source_id: int              # Parent email source ID
    tenant_id: UUID

    # Attachment info
    filename: str
    mime_type: str
    size_bytes: int

    # Document reference (for future document processing)
    document_id: Optional[UUID]

    # Status
    extraction_status: Literal["completed", "skipped_oversized", "skipped_unsupported"]
```

---

## Event Flow

```
User runs: penf ingest email ./emails/ --source "archive"
                    │
                    ▼
            ┌───────────────┐
            │ IngestJob     │
            │ created       │
            └───────┬───────┘
                    │
        ┌───────────┴───────────┐
        │ For each .eml file:   │
        │                       │
        ▼                       ▼
┌───────────────┐       ┌────────────────────┐
│ Parse & Store │       │ IngestJobProgress  │
│ in Source     │       │ Event (periodic)   │
└───────┬───────┘       └────────────────────┘
        │
        ▼
┌───────────────────────┐
│ ManualEmailIngested   │ ──────► AI Pipeline
│ Event                 │         (embeddings, assertions, etc.)
└───────────────────────┘
        │
        │ If has attachments
        ▼
┌───────────────────────┐
│ AttachmentExtracted   │ ──────► Document Processing (future)
│ Event                 │
└───────────────────────┘
        │
        │ After all files
        ▼
┌───────────────────────┐
│ IngestJobCompleted    │
│ Event                 │
└───────────────────────┘
```

---

## Subscribers

| Event | Subscriber | Action |
|-------|------------|--------|
| `manual_email.ingested` | AI Pipeline | Generate embeddings, extract assertions |
| `manual_email.ingested` | Entity Resolver | Link participants to Person records |
| `manual_email.ingested` | Summarizer | Generate brief/full summaries |
| `ingest_job.progress` | CLI (local) | Update progress display |
| `ingest_job.completed` | Analytics | Record job metrics |
| `attachment.extracted` | Document Processor | Extract text from attachments (future) |

---

## Contract Tests

Required contract tests for event publishing:

```python
# tests/contract/test_ingest_events.py

async def test_manual_email_ingested_event_schema():
    """Verify ManualEmailIngestedEvent matches expected schema."""
    event = ManualEmailIngestedEvent(
        source_id=1,
        tenant_id=uuid4(),
        message_id="<test@example.com>",
        job_id=uuid4(),
        from_email="sender@example.com",
        to_emails=["recipient@example.com"],
        content_hash="abc123",
        source_tag="test-source",
        original_file_path="test.eml",
    )
    # Verify serialization
    data = event.model_dump()
    assert data["event_type"] == "manual_email.ingested"
    assert "source_id" in data
    assert "content_hash" in data

async def test_event_published_to_correct_channel():
    """Verify events are published to expected Redis channels."""
    publisher = IngestEventPublisher(redis_url="redis://localhost:6379")
    # Subscribe to channel and verify event arrives
    ...
```
