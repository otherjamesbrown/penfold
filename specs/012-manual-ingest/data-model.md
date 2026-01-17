# Data Model: Manual Content Ingest Framework

**Feature**: 012-manual-ingest | **Date**: 2026-01-16

## Overview

This document defines the data model for the manual ingest framework. The design reuses the existing `Source` entity for unified search while adding new entities for job tracking, attachments, and file archival.

---

## Entity Relationship Diagram

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   IngestJob     │     │     Source      │     │  ArchivedFile   │
├─────────────────┤     ├─────────────────┤     ├─────────────────┤
│ id              │     │ id              │     │ id              │
│ tenant_id       │     │ tenant_id       │     │ tenant_id       │
│ source_tag      │────>│ source_system   │<────│ source_id       │
│ status          │     │ external_id     │     │ encrypted_content│
│ file_manifest   │     │ raw_content     │     │ encryption_key_id│
│ processed_files │     │ content_hash    │     │ original_size   │
│ error_count     │     │ ingestion_meta  │     └─────────────────┘
│ created_at      │     │ source_timestamp│
└─────────────────┘     └────────┬────────┘
                                 │
                                 │ 1:N
                                 ▼
                        ┌─────────────────┐     ┌─────────────────┐
                        │ EmailAttachment │     │    Document     │
                        ├─────────────────┤     ├─────────────────┤
                        │ id              │     │ id              │
                        │ source_id       │────>│ (future spec)   │
                        │ document_id     │     │                 │
                        │ filename        │     │                 │
                        │ mime_type       │     │                 │
                        │ size_bytes      │     │                 │
                        │ extraction_status│    │                 │
                        └─────────────────┘     └─────────────────┘
```

---

## Entities

### 1. IngestJob (NEW)

Tracks batch upload jobs with progress for resume capability.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK | Job identifier |
| tenant_id | UUID | FK, NOT NULL, INDEX | Tenant isolation |
| source_tag | VARCHAR(100) | NOT NULL | User-provided source identifier |
| status | VARCHAR(20) | NOT NULL, CHECK | Job status (see states below) |
| total_files | INTEGER | NOT NULL | Total files in batch |
| processed_count | INTEGER | DEFAULT 0 | Files processed so far |
| imported_count | INTEGER | DEFAULT 0 | Successfully imported |
| skipped_count | INTEGER | DEFAULT 0 | Skipped (duplicates) |
| failed_count | INTEGER | DEFAULT 0 | Failed to process |
| file_manifest | JSONB | NOT NULL | List of all file paths in batch |
| processed_files | JSONB | DEFAULT [] | Completed file paths (for resume) |
| options | JSONB | DEFAULT {} | Job options (preserve_folders, etc.) |
| started_at | TIMESTAMPTZ | | When processing started |
| completed_at | TIMESTAMPTZ | | When processing finished |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation time |
| updated_at | TIMESTAMPTZ | DEFAULT NOW() | Last update time |

**Status State Machine:**
```
pending → in_progress → completed
                     ↘ failed
                     ↘ cancelled
```

**Indexes:**
- `idx_ingest_jobs_tenant_status` ON (tenant_id, status)
- `idx_ingest_jobs_tenant_created` ON (tenant_id, created_at DESC)

---

### 2. IngestError (NEW)

Detailed error records for failed files in a job.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK | Error identifier |
| job_id | UUID | FK → IngestJob, NOT NULL | Parent job |
| file_path | VARCHAR(500) | NOT NULL | Path to failed file |
| error_type | VARCHAR(50) | NOT NULL | Error category |
| error_message | TEXT | NOT NULL | Human-readable error |
| error_details | JSONB | | Stack trace, context |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | When error occurred |

**Error Types:**
- `parse_error` - Invalid .eml format
- `encoding_error` - Character encoding issues
- `io_error` - File read failures
- `validation_error` - Missing required fields
- `storage_error` - Database write failures

---

### 3. Source (EXTENDED)

Existing entity - extended with new source_system value.

**New source_system Value:**
```python
source_system = "manual_eml"  # For manually uploaded .eml files
```

**ingestion_metadata Schema for manual_eml:**
```json
{
  "source_tag": "outlook-archive",
  "from": "bob@example.com",
  "from_name": "Bob Smith",
  "to": ["alice@example.com"],
  "cc": [],
  "bcc": [],
  "subject": "Re: Project Update",
  "date": "2026-01-10T14:30:00Z",
  "date_fallback_used": false,
  "original_date_raw": "Thu, 10 Jan 2026 14:30:00 -0500",
  "message_id": "<abc123@example.com>",
  "message_id_synthetic": false,
  "in_reply_to": "<parent456@example.com>",
  "references": ["<parent456@example.com>"],
  "thread_id": "uuid-of-linked-thread",
  "labels": ["inbox", "projects/atlas"],
  "original_file_path": "inbox/customer-issue.eml",
  "job_id": "uuid-of-ingest-job",
  "attachment_count": 2,
  "has_html_body": true,
  "content_length": 4532
}
```

**Validation Rules:**
- `source_tag` required (non-empty string)
- `message_id` required (generated if missing)
- `date` defaults to ingestion time if invalid
- `from` required (extracted from header)

---

### 4. EmailAttachment (NEW)

Links email sources to extracted document records.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK | Attachment identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| source_id | BIGINT | FK → Source, NOT NULL | Parent email |
| document_id | UUID | FK → Document, NULLABLE | Extracted document (future) |
| filename | VARCHAR(255) | NOT NULL | Original filename |
| mime_type | VARCHAR(100) | NOT NULL | MIME content type |
| size_bytes | INTEGER | NOT NULL | File size |
| extraction_status | VARCHAR(20) | NOT NULL | Extraction state |
| extraction_error | TEXT | | Error message if failed |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Record creation |

**Extraction Status Values:**
- `pending` - Awaiting extraction
- `completed` - Successfully extracted to Document
- `failed` - Extraction failed
- `skipped_oversized` - Exceeded 25MB limit
- `skipped_unsupported` - Unsupported MIME type

**Indexes:**
- `idx_email_attachments_source` ON (source_id)
- `idx_email_attachments_document` ON (document_id) WHERE document_id IS NOT NULL

---

### 5. ArchivedFile (NEW)

Encrypted storage of original .eml files for compliance retrieval.

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | UUID | PK | Archive identifier |
| tenant_id | UUID | FK, NOT NULL | Tenant isolation |
| source_id | BIGINT | FK → Source, NOT NULL, UNIQUE | Link to processed email |
| encrypted_content | BYTEA | NOT NULL | AES-256-GCM encrypted file |
| original_size | INTEGER | NOT NULL | Unencrypted file size |
| encryption_key_id | VARCHAR(100) | NOT NULL | Key identifier for rotation |
| content_hash | VARCHAR(64) | NOT NULL | SHA-256 of original (pre-encryption) |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | Archive time |

**Encryption Details:**
- Algorithm: AES-256-GCM
- Key derivation: HKDF from master key with tenant_id
- Nonce: Random 12 bytes, prepended to ciphertext
- AAD: source_id for integrity binding

**Indexes:**
- `idx_archived_files_source` ON (source_id) UNIQUE

---

## State Transitions

### IngestJob Status

```
┌─────────┐
│ pending │ ← Initial state on job creation
└────┬────┘
     │ start_processing()
     ▼
┌───────────────┐
│  in_progress  │ ← Processing files
└───────┬───────┘
        │
   ┌────┴────┬─────────────┐
   │         │             │
   ▼         ▼             ▼
┌─────────┐ ┌────────┐ ┌───────────┐
│completed│ │ failed │ │ cancelled │
└─────────┘ └────────┘ └───────────┘
```

### EmailAttachment Extraction

```
┌─────────┐
│ pending │ ← Initial state
└────┬────┘
     │ extract()
     │
┌────┴────┬──────────────────┬────────────────────┐
│         │                  │                    │
▼         ▼                  ▼                    ▼
┌─────────┐ ┌────────────────┐ ┌─────────────────┐ ┌────────┐
│completed│ │skipped_oversized│ │skipped_unsupported│ │ failed │
└─────────┘ └────────────────┘ └─────────────────┘ └────────┘
```

---

## Validation Rules

### IngestJob
- `source_tag`: Required, max 100 chars, alphanumeric + hyphens
- `total_files`: Required, positive integer
- `file_manifest`: Required, non-empty JSON array

### Source (manual_eml)
- `external_id`: Message-ID (real or synthetic), unique per tenant+source_system
- `content_hash`: SHA-256, 64 hex chars
- `ingestion_metadata.source_tag`: Required, matches job source_tag
- `ingestion_metadata.from`: Required, valid email format

### EmailAttachment
- `filename`: Required, max 255 chars
- `mime_type`: Required, valid MIME format
- `size_bytes`: Required, non-negative

### ArchivedFile
- `encrypted_content`: Required, non-empty
- `content_hash`: Must match original file hash
- `encryption_key_id`: Required, format "tenant-{tenant_id}-v{version}"

---

## Migration Notes

New tables required:
1. `ingest_jobs` - Job tracking
2. `ingest_errors` - Error details
3. `email_attachments` - Attachment linking
4. `archived_files` - Encrypted originals

No changes to existing tables - `sources` extended via new `source_system` value only.
