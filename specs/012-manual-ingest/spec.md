# Feature Specification: Manual Content Ingest Framework

**Feature Branch**: `012-manual-ingest`
**Created**: 2026-01-16
**Status**: Draft
**Input**: User requirement for uploading archived .eml files alongside Gmail integration

## Overview

This specification defines a **type-based manual ingest framework** for uploading content files directly into Penfold. While the Gmail integration (004) handles live email sync, this feature enables ingestion of:
- Archived emails from other providers (Outlook, legacy systems)
- Historical email exports
- Future content types (documents, Slack exports, etc.)

The framework establishes the `penf ingest <type>` CLI pattern, with **email** as the first implemented content type.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Single Email File Upload (Priority: P1)

As a user with archived emails, I need to upload individual .eml files so that historical emails from Outlook or other sources become searchable alongside my Gmail emails.

**Why this priority**: Core functionality - without single file upload, the feature provides no value.

**Independent Test**: Can be fully tested by uploading a valid .eml file and verifying it appears in search results.

**Acceptance Scenarios**:

1. **Given** user has an .eml file, **When** `penf ingest email customer-issue.eml --source "outlook-archive"` is run, **Then** email is parsed, stored, and searchable
2. **Given** email contains metadata (To, From, Subject, Date), **When** upload completes, **Then** all metadata is extracted and indexed
3. **Given** email is part of a thread (has In-Reply-To header), **When** upload completes, **Then** thread relationship is established with any existing related emails

---

### User Story 2 - Bulk Email Upload (Priority: P1)

As a user with hundreds of archived emails, I need to upload entire directories of .eml files so that I can efficiently migrate historical email archives.

**Why this priority**: Without bulk upload, migrating large archives is impractical.

**Independent Test**: Can be fully tested by uploading a directory of .eml files and verifying progress reporting and successful ingestion.

**Acceptance Scenarios**:

1. **Given** directory contains 100 .eml files, **When** `penf ingest email ./emails/ --source "outlook-2024"` is run, **Then** all files are processed with progress bar showing completion
2. **Given** glob pattern `*.eml` is provided, **When** upload runs, **Then** only matching files are processed
3. **Given** nested folder structure exists, **When** upload runs with default settings, **Then** folder names are preserved as labels on the emails

---

### User Story 3 - Duplicate Handling (Priority: P1)

As a user who may accidentally upload the same email twice, I need the system to detect and skip duplicates so that my email archive remains clean without manual deduplication.

**Why this priority**: Essential for data integrity - duplicate emails pollute search results and waste storage.

**Independent Test**: Can be fully tested by uploading the same .eml file twice and verifying only one record exists.

**Acceptance Scenarios**:

1. **Given** email with Message-ID "abc123" exists, **When** same email is uploaded again, **Then** duplicate is silently skipped
2. **Given** batch of 100 emails contains 5 duplicates, **When** upload completes, **Then** summary shows "95 imported, 5 skipped (duplicates)"
3. **Given** email exists from Gmail integration, **When** same email is uploaded as .eml, **Then** duplicate is detected via Message-ID and skipped

---

### User Story 4 - Attachment Extraction (Priority: P2)

As a user searching for documents, I need email attachments to be extracted and processed as searchable documents so that I can find "that PDF Bob sent last month" through document search.

**Why this priority**: Enhances search capability but basic email search works without it.

**Independent Test**: Can be fully tested by uploading an email with PDF attachment and finding it via document search.

**Acceptance Scenarios**:

1. **Given** email has PDF attachment, **When** upload completes, **Then** PDF is extracted, stored as document type, and linked to parent email
2. **Given** document search for attachment content, **When** search runs, **Then** results include attachment with link to source email
3. **Given** email has 3 attachments, **When** upload completes, **Then** all 3 are extracted with individual document records

---

### User Story 5 - Unified Search (Priority: P1)

As a user with emails from multiple sources, I need to search across all emails regardless of whether they came from Gmail sync or manual upload so that I get complete search results.

**Why this priority**: Core value proposition - partial search results defeat the purpose.

**Independent Test**: Can be fully tested by having emails from both sources and verifying unified search returns both.

**Acceptance Scenarios**:

1. **Given** emails exist from Gmail and manual upload, **When** `penf search "juniper routers"` is run, **Then** results include both sources
2. **Given** search with date filter "last week", **When** search runs, **Then** filter uses original email date, not upload date
3. **Given** filter for source `--source "outlook-archive"`, **When** search runs, **Then** only manually uploaded emails from that source are returned

---

### User Story 6 - Original File Archive (Priority: P2)

As a user who needs the original email for legal or compliance reasons, I need to retrieve the original .eml file so that I have source-of-truth access to unmodified content.

**Why this priority**: Important for compliance but not needed for basic functionality.

**Independent Test**: Can be fully tested by uploading an email and retrieving the original .eml file.

**Acceptance Scenarios**:

1. **Given** email was uploaded from .eml file, **When** `penf email show <id> --original` is run, **Then** original .eml content is returned
2. **Given** original file is archived, **When** storage is queried, **Then** file is stored encrypted with reference to processed record

---

### Edge Cases

- What happens when .eml file is malformed or corrupt?
- How does the system handle .eml files with no Message-ID header?
- What occurs when email date is in the future or invalid?
- How are non-UTF8 encodings handled in email content?
- What happens when attachment extraction fails for one file in a batch?
- How does the system handle emails with extremely large attachments (>25MB)?
- What occurs when upload is interrupted mid-batch?

## Requirements *(mandatory)*

### Functional Requirements - Ingest Framework

- **FR-001**: System MUST provide `penf ingest <type>` CLI pattern supporting multiple content types
- **FR-002**: System MUST require `--source` tag for all manual uploads to track content provenance
- **FR-003**: System MUST support single file and directory/glob batch uploads
- **FR-004**: System MUST display progress bar with file count and percentage for batch uploads
- **FR-005**: System MUST provide summary report on completion (imported, skipped, failed counts)
- **FR-006**: System MUST skip malformed files and continue processing batch, logging errors
- **FR-007**: System MUST respect active tenant context for all uploaded content
- **FR-008**: System MUST support `--dry-run` flag to preview what would be imported

### Functional Requirements - Email Type

- **FR-100**: System MUST parse .eml files conforming to RFC 822/RFC 2822 format
- **FR-101**: System MUST extract email metadata: From, To, Cc, Bcc, Subject, Date, Message-ID
- **FR-102**: System MUST extract email body in both plain text and HTML formats when available
- **FR-103**: System MUST detect duplicates via Message-ID header and skip silently
- **FR-104**: System MUST use email's original Date header for timeline positioning
- **FR-105**: System MUST reconstruct thread relationships using In-Reply-To and References headers
- **FR-106**: System MUST preserve folder structure as labels when uploading from directories
- **FR-107**: System MUST extract attachments and store as document type with email relationship
- **FR-108**: System MUST archive original .eml file for retrieval
- **FR-109**: System MUST normalize participant email addresses matching Gmail integration patterns
- **FR-110**: System MUST use same underlying email data model as Gmail integration (source_system differentiator)
- **FR-111**: System MUST support re-analysis when AI capabilities improve (same as Gmail emails)
- **FR-112**: System MUST trigger full AI extraction pipeline including assertions (risks, actions, decisions, deadlines), entity resolution, and summarization
- **FR-113**: System MUST generate embeddings for semantic search capability
- **FR-114**: System MUST generate brief summary (1-2 sentences) for all ingested content
- **FR-115**: System MUST generate full summary for content exceeding complexity thresholds (defined per content type)

### Key Entities

- **IngestJob**: Batch upload job with progress tracking, source tag, and status
- **IngestError**: Failed file record with error details for troubleshooting
- **EmailMessage**: Extended to support `source_system = "manual_eml"` alongside `"gmail"`
- **EmailAttachment**: Attachment record linked to both email and document entities
- **Document**: Content type for extracted attachments (to be fully specified in future document spec)
- **ArchivedFile**: Original .eml file storage with encryption and retrieval capability

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Single .eml file upload completes in under 5 seconds including parsing and storage
- **SC-002**: Batch upload of 100 .eml files completes in under 2 minutes with progress reporting
- **SC-003**: Duplicate detection correctly identifies 100% of duplicates via Message-ID matching
- **SC-004**: Thread reconstruction accurately links 95% of emails with valid In-Reply-To headers
- **SC-005**: Unified search returns results from both Gmail and manual upload sources within 3 seconds
- **SC-006**: Attachment extraction successfully processes 90% of common formats (PDF, DOCX, images)
- **SC-007**: Original .eml file retrieval returns exact uploaded content with no modifications
- **SC-008**: Malformed file handling skips corrupt files without blocking batch completion
- **SC-009**: Folder structure preservation creates accurate labels for 100% of nested directories
- **SC-010**: Re-analysis pipeline treats manual emails identically to Gmail emails

## CLI Interface

### Email Ingest Commands

```bash
# Single file upload (source required)
penf ingest email customer-issue.eml --source "outlook-archive"

# Bulk upload from directory
penf ingest email ./exported-emails/ --source "outlook-2024-archive"

# Glob pattern
penf ingest email ./emails/**/*.eml --source "old-laptop-backup"

# With explicit project tagging
penf ingest email *.eml --source "outlook-archive" --projects "Atlas,SOC2"

# Dry run to preview
penf ingest email ./emails/ --source "test" --dry-run

# Without folder label preservation
penf ingest email ./emails/ --source "archive" --no-preserve-folders
```

### Output Format

```
$ penf ingest email ./emails/ --source "outlook-2024"

Scanning directory... found 150 .eml files

Processing emails ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100% 150/150

Summary:
  Imported:  142 emails
  Skipped:    5 duplicates
  Failed:     3 files (see errors below)

Errors:
  ./emails/corrupt.eml: Failed to parse - invalid MIME structure
  ./emails/empty.eml: Empty file
  ./emails/binary.eml: Not a valid email format

Labels created from folders:
  - inbox (45 emails)
  - sent (32 emails)
  - projects/atlas (28 emails)
  - projects/soc2 (37 emails)

Attachments extracted: 23 documents
```

## Data Model Integration

### Storage Architecture

Manual ingest uses the same storage pattern as Gmail integration:

```
┌─────────────────────────────────────────────────────────────────┐
│  sources table (PostgreSQL)                                      │
│  - raw_content TEXT (full email text - body, headers, etc.)     │
│  - source_system = "manual_eml"                                 │
│  - content_hash SHA-256 for deduplication                       │
│  - ingestion_metadata JSONB (parsed headers, source_tag, etc.)  │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼ publishes content.ingested event
┌─────────────────────────────────────────────────────────────────┐
│  AI Processing Pipeline (existing from 002/003)                  │
│  - Entity extraction                                            │
│  - Categorization                                               │
│  - Embedding generation                                         │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│  embeddings table (pgvector)                                     │
│  - embedding VECTOR(768) for semantic/similarity search         │
│  - source_id → links back to sources                            │
│  - embedding_model tracking for re-analysis                     │
└─────────────────────────────────────────────────────────────────┘
```

**Key Point**: Manual ingest puts content into `sources` and publishes the event. The existing AI pipeline then performs **full extraction**:

1. **Embeddings** → semantic/similarity search ("emails about deployment delays")
2. **Assertions** → structured extraction of risks, actions, decisions, deadlines
3. **Entity Resolution** → link people mentions to canonical Person records
4. **Summarization** → brief summary for quick scanning

This enables rich queries like:
- "when was a risk raised about juniper routers" → filters by `assertion_type='risk'` + vector search
- "what actions did Bob commit to last week" → filters by person + `assertion_type='action_item'` + date
- "summarize the Atlas project emails" → returns pre-generated summaries

### Tiered Summarization Strategy

Summaries are generated based on content complexity, not just length:

| Content Type | Brief Summary | Full Summary Trigger |
|--------------|---------------|----------------------|
| **Email (single)** | Always | Body > 500 words OR has attachments |
| **Email thread** | Always | Thread depth > 3 messages OR > 5 participants |
| **Document (PDF/DOCX)** | Always | > 10 pages OR detected complexity (multiple sections, technical content) |
| **Meeting transcript** | Always | Duration > 15 minutes |

**Rationale**: An email thread with 15 back-and-forths between 8 people needs full summarization even if total word count is low. A straightforward 2-page PDF may only need a brief summary.

**Summary Storage**:
```python
processing_results:
    result_type: "summary"
    result_data: {
        "brief": "Bob raised concerns about Juniper router delays affecting Atlas timeline.",
        "full": "Extended summary with key points, participants, decisions...",  # nullable
        "summary_trigger": "thread_depth",  # why full summary was generated
        "complexity_score": 0.78
    }
```

### Email-Specific Storage

```python
# Uses existing sources table with email-specific metadata
sources:
    source_system: "manual_eml"
    external_id: <Message-ID header>
    raw_content: <full email text>
    content_hash: <SHA-256 for dedup>
    ingestion_metadata: {
        "source_tag": "outlook-archive",
        "from": "bob@example.com",
        "to": ["alice@example.com"],
        "cc": [],
        "subject": "Re: Juniper router delays",
        "date": "2026-01-10T14:30:00Z",
        "message_id": "<abc123@example.com>",
        "in_reply_to": "<parent456@example.com>",
        "references": ["<parent456@example.com>", "<grandparent789@example.com>"],
        "labels": ["inbox", "projects/atlas"],  # from folder structure
        "original_file_path": "inbox/customer-issue.eml"
    }

# Duplicate detection via unique constraint
# UNIQUE(tenant_id, source_system, external_id) where external_id = Message-ID
```

### Attachment-Document Relationship

```python
class EmailAttachment(Base):
    email_id: UUID          # Parent email
    document_id: UUID       # Extracted document record
    filename: str
    mime_type: str
    size_bytes: int
    extraction_status: str  # "pending" | "completed" | "failed"

class Document(Base):
    # Future spec - basic structure for now
    id: UUID
    content_type: str       # "pdf", "docx", "image", etc.
    extracted_text: str
    source_type: str        # "email_attachment" | "manual_upload" | etc.
    source_reference_id: UUID  # Links back to attachment or upload
```

## Dependencies

- Database schema from [001-database-schema](../001-database-schema/spec.md) for storage
- Event processing from [002-event-processing](../002-event-processing/spec.md) for publishing ingest events
- Search interface from [007-search-interface](../007-search-interface/spec.md) for unified search
- Gmail integration from [004-gmail-integration](../004-gmail-integration/spec.md) for shared email model
- Python `email` standard library for .eml parsing
- Future: Document processing spec for attachment text extraction

## Assumptions

- .eml files follow RFC 822/RFC 2822 standards (covers vast majority of email exports)
- Message-ID headers are present and unique (required for deduplication)
- Email volumes for manual upload will typically be under 10,000 files per batch
- Attachment sizes will typically be under 25MB (matching Gmail limits)
- Users understand the source tag requirement and will provide meaningful values
- Folder structures in upload directories are intentional and should be preserved

## Future Extensibility

This spec establishes the `penf ingest <type>` pattern for future content types:

| Type | Format(s) | Status |
|------|-----------|--------|
| `email` | .eml | This spec |
| `document` | PDF, DOCX, TXT | Future spec |
| `slack` | Slack export JSON | Future spec |
| `meeting` | Notes, transcripts | See 005-meeting-pipeline |

Each type will follow the same CLI pattern:
```bash
penf ingest <type> <path> --source "<tag>" [--projects "..."]
```
