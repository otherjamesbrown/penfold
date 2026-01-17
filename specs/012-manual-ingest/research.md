# Research: Manual Content Ingest Framework

**Feature**: 012-manual-ingest | **Date**: 2026-01-16

## Overview

Research findings for implementing the manual email ingest framework. All technical questions have been resolved through codebase analysis and specification clarification.

---

## 1. EML Parsing with Python Email Library

### Decision
Use Python's standard library `email` module for parsing .eml files.

### Rationale
- **RFC Compliance**: Full RFC 822/2822/5322 compliance built-in
- **No Dependencies**: Part of Python standard library
- **Battle-tested**: Used by countless production systems
- **Encoding Handling**: Automatic charset detection and decoding

### Implementation Pattern

```python
from email import message_from_bytes, policy
from email.utils import parsedate_to_datetime, parseaddr
import hashlib

def parse_eml_file(file_path: Path) -> dict:
    """Parse .eml file and extract metadata."""
    with open(file_path, 'rb') as f:
        raw_content = f.read()

    # Use email.policy.default for modern email parsing
    msg = message_from_bytes(raw_content, policy=policy.default)

    # Extract headers
    message_id = msg.get('Message-ID', '')
    if not message_id:
        # Generate synthetic Message-ID from content hash (FR-103a)
        content_hash = hashlib.sha256(raw_content).hexdigest()
        message_id = f"<synthetic-{content_hash[:16]}@penfold.local>"

    # Parse date with fallback (FR-104)
    date_str = msg.get('Date')
    try:
        email_date = parsedate_to_datetime(date_str) if date_str else None
        if email_date and email_date > datetime.now(timezone.utc):
            # Future date - use ingestion timestamp
            email_date = None
    except (ValueError, TypeError):
        email_date = None  # Use ingestion timestamp as fallback

    # Extract body (both plain and HTML)
    body_plain = None
    body_html = None
    if msg.is_multipart():
        for part in msg.walk():
            content_type = part.get_content_type()
            if content_type == 'text/plain' and not body_plain:
                body_plain = part.get_content()
            elif content_type == 'text/html' and not body_html:
                body_html = part.get_content()
    else:
        content_type = msg.get_content_type()
        content = msg.get_content()
        if content_type == 'text/html':
            body_html = content
        else:
            body_plain = content

    return {
        'message_id': message_id,
        'from_email': parseaddr(msg.get('From', ''))[1],
        'from_name': parseaddr(msg.get('From', ''))[0],
        'to': [parseaddr(addr)[1] for addr in msg.get_all('To', [])],
        'cc': [parseaddr(addr)[1] for addr in msg.get_all('Cc', [])],
        'bcc': [parseaddr(addr)[1] for addr in msg.get_all('Bcc', [])],
        'subject': msg.get('Subject', ''),
        'date': email_date,
        'in_reply_to': msg.get('In-Reply-To'),
        'references': msg.get('References', '').split(),
        'body_plain': body_plain,
        'body_html': body_html,
        'raw_content': raw_content,
        'content_hash': hashlib.sha256(raw_content).hexdigest(),
    }
```

### Alternatives Considered
- **mail-parser**: Third-party library with similar functionality - rejected (unnecessary dependency)
- **flanker**: Mailgun's parser - rejected (heavy dependency, overkill for .eml files)

---

## 2. AES-256-GCM Encryption for File Archival

### Decision
Implement AES-256-GCM encryption using Python's `cryptography` library with tenant-scoped keys.

### Rationale
- **Spec Requirement**: FR-108 explicitly requires AES-256-GCM
- **Authenticated Encryption**: GCM provides both confidentiality and integrity
- **Industry Standard**: NIST-approved, widely used
- **Key Management**: Tenant-scoped keys enable data isolation

### Implementation Pattern

```python
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
import os
import base64

class FileArchiver:
    """AES-256-GCM encrypted file archival."""

    NONCE_SIZE = 12  # GCM standard nonce size
    KEY_SIZE = 32    # 256 bits

    def __init__(self, tenant_key: bytes):
        """Initialize with tenant-specific key."""
        if len(tenant_key) != self.KEY_SIZE:
            raise ValueError(f"Key must be {self.KEY_SIZE} bytes")
        self.aesgcm = AESGCM(tenant_key)

    def encrypt(self, plaintext: bytes, associated_data: bytes = None) -> bytes:
        """Encrypt data with AES-256-GCM.

        Returns: nonce (12 bytes) || ciphertext || tag (16 bytes)
        """
        nonce = os.urandom(self.NONCE_SIZE)
        ciphertext = self.aesgcm.encrypt(nonce, plaintext, associated_data)
        return nonce + ciphertext

    def decrypt(self, encrypted_data: bytes, associated_data: bytes = None) -> bytes:
        """Decrypt AES-256-GCM encrypted data."""
        nonce = encrypted_data[:self.NONCE_SIZE]
        ciphertext = encrypted_data[self.NONCE_SIZE:]
        return self.aesgcm.decrypt(nonce, ciphertext, associated_data)

    @staticmethod
    def derive_tenant_key(master_key: bytes, tenant_id: str) -> bytes:
        """Derive tenant-specific key from master key."""
        from cryptography.hazmat.primitives.kdf.hkdf import HKDF
        from cryptography.hazmat.primitives import hashes

        hkdf = HKDF(
            algorithm=hashes.SHA256(),
            length=FileArchiver.KEY_SIZE,
            salt=None,
            info=f"penfold-archive-{tenant_id}".encode(),
        )
        return hkdf.derive(master_key)
```

### Key Management Strategy
1. **Master Key**: Stored in environment variable `PENF_ARCHIVE_MASTER_KEY`
2. **Tenant Keys**: Derived from master key using HKDF with tenant_id as info
3. **Associated Data**: Include source_id in AAD for integrity binding

### Alternatives Considered
- **Fernet** (existing encryption.py): Uses AES-128-CBC - rejected (spec requires GCM)
- **AWS KMS**: Cloud dependency - rejected (local-first principle)

---

## 3. Batch Processing with Progress Tracking

### Decision
Use Rich library's Progress for CLI progress bars with async batch processing.

### Rationale
- **Existing Pattern**: Rich already used throughout CLI (see main.py)
- **User Experience**: Clear progress indication for large batches
- **Performance**: Async processing for I/O-bound operations

### Implementation Pattern

```python
from rich.progress import Progress, TaskID, TextColumn, BarColumn, MofNCompleteColumn
from pathlib import Path
import asyncio

class BatchProcessor:
    """Process batches of files with progress tracking."""

    def __init__(self, console: Console):
        self.console = console
        self.results = {'imported': 0, 'skipped': 0, 'failed': 0}
        self.errors: list[tuple[str, str]] = []

    async def process_batch(
        self,
        files: list[Path],
        source_tag: str,
        job_id: str,
        dry_run: bool = False
    ) -> dict:
        """Process batch of files with progress reporting."""

        with Progress(
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            MofNCompleteColumn(),
            console=self.console,
        ) as progress:
            task = progress.add_task("Processing emails", total=len(files))

            for file_path in files:
                try:
                    result = await self._process_single_file(
                        file_path, source_tag, dry_run
                    )
                    if result == 'imported':
                        self.results['imported'] += 1
                    elif result == 'skipped':
                        self.results['skipped'] += 1

                    # Update job progress for resume capability (FR-009)
                    await self._update_job_progress(job_id, file_path)

                except Exception as e:
                    self.results['failed'] += 1
                    self.errors.append((str(file_path), str(e)))

                progress.update(task, advance=1)

        return self.results

    async def _update_job_progress(self, job_id: str, completed_file: Path):
        """Update IngestJob with completed file for resume capability."""
        # Store in IngestJob.processed_files JSONB array
        pass
```

### Performance Considerations
- **Batch Size**: Process files sequentially to avoid memory issues
- **Checkpointing**: Update IngestJob after each file for resume capability
- **Error Isolation**: Continue processing after individual file failures (FR-006)

---

## 4. Deduplication Strategy

### Decision
Two-level deduplication: Message-ID (primary) with content hash (fallback).

### Rationale
- **Message-ID**: Standard email identifier, cross-source dedup (Gmail + manual)
- **Content Hash**: Fallback for emails without Message-ID (FR-103a)
- **Unique Constraint**: Database-level enforcement

### Implementation Pattern

```python
async def check_duplicate(
    session: AsyncSession,
    tenant_id: UUID,
    message_id: str,
    content_hash: str
) -> bool:
    """Check if email already exists.

    Checks both Message-ID and content hash for comprehensive dedup.
    """
    # Primary check: Message-ID match
    stmt = select(Source).where(
        Source.tenant_id == tenant_id,
        Source.external_id == message_id,
        Source.is_deleted == False
    )
    result = await session.execute(stmt)
    if result.scalar_one_or_none():
        return True

    # Secondary check: Content hash (catches different Message-IDs for same content)
    stmt = select(Source).where(
        Source.tenant_id == tenant_id,
        Source.content_hash == content_hash,
        Source.is_deleted == False
    )
    result = await session.execute(stmt)
    return result.scalar_one_or_none() is not None
```

### Database Constraints
```sql
-- Add partial unique index for deduplication
CREATE UNIQUE INDEX idx_sources_tenant_external_id_active
ON sources (tenant_id, source_system, external_id)
WHERE is_deleted = FALSE;
```

---

## 5. Thread Reconstruction

### Decision
Use In-Reply-To and References headers to link emails to existing threads.

### Rationale
- **RFC Compliant**: Standard thread reconstruction mechanism
- **Cross-Source**: Links manual uploads to Gmail thread context
- **Partial Linking**: Handle incomplete thread data gracefully

### Implementation Pattern

```python
async def link_to_thread(
    session: AsyncSession,
    tenant_id: UUID,
    in_reply_to: str | None,
    references: list[str]
) -> UUID | None:
    """Find and link to existing email thread.

    Returns thread_id if found, None otherwise.
    """
    # Try In-Reply-To first (direct parent)
    if in_reply_to:
        parent = await find_by_message_id(session, tenant_id, in_reply_to)
        if parent and parent.ingestion_metadata.get('thread_id'):
            return parent.ingestion_metadata['thread_id']

    # Try References (thread history)
    for ref in references:
        related = await find_by_message_id(session, tenant_id, ref)
        if related and related.ingestion_metadata.get('thread_id'):
            return related.ingestion_metadata['thread_id']

    return None  # No existing thread found
```

---

## 6. Attachment Handling

### Decision
Extract attachments inline during email parsing, skip oversized files (>25MB).

### Rationale
- **FR-107a**: Skip extraction for attachments >25MB
- **Memory Safety**: Don't load large attachments into memory
- **Future Extensibility**: Document type for attachment text extraction (future spec)

### Implementation Pattern

```python
def extract_attachments(msg: EmailMessage, max_size: int = 25 * 1024 * 1024) -> list[dict]:
    """Extract attachments from email message.

    Args:
        msg: Parsed email message
        max_size: Maximum attachment size in bytes (default 25MB)

    Returns:
        List of attachment metadata (content stored separately)
    """
    attachments = []

    for part in msg.walk():
        content_disposition = part.get_content_disposition()
        if content_disposition != 'attachment':
            continue

        filename = part.get_filename()
        if not filename:
            continue

        content = part.get_content()
        size_bytes = len(content) if isinstance(content, bytes) else len(content.encode())

        if size_bytes > max_size:
            attachments.append({
                'filename': filename,
                'mime_type': part.get_content_type(),
                'size_bytes': size_bytes,
                'extraction_status': 'skipped_oversized',
                'content': None,
            })
        else:
            attachments.append({
                'filename': filename,
                'mime_type': part.get_content_type(),
                'size_bytes': size_bytes,
                'extraction_status': 'pending',
                'content': content,
            })

    return attachments
```

---

## Summary

| Topic | Decision | Status |
|-------|----------|--------|
| EML Parsing | Python email stdlib | Resolved |
| File Encryption | AES-256-GCM with tenant keys | Resolved |
| Progress Tracking | Rich Progress bars | Resolved |
| Deduplication | Message-ID + content hash | Resolved |
| Thread Reconstruction | In-Reply-To + References | Resolved |
| Attachment Handling | Inline extraction, 25MB limit | Resolved |

All research questions resolved. Ready for Phase 1: Data Model & Contracts.
