# Quickstart: Manual Content Ingest

**Feature**: 012-manual-ingest | **Date**: 2026-01-16

## Overview

Quick guide for developers implementing the manual ingest framework.

---

## Prerequisites

1. **Development environment** set up per main README
2. **PostgreSQL 16+** with pgvector extension running
3. **Redis** running for event publishing
4. **Active tenant context**: `penf tenant switch work`

---

## Key Files to Implement

### Core Module (`penf_lib/ingest/`)

```
penf_lib/ingest/
├── __init__.py          # Module exports
├── models.py            # Pydantic models for IngestJob, parsed email
├── parser.py            # EML parsing using email stdlib
├── processor.py         # Batch processing with progress
├── deduplication.py     # Message-ID and hash dedup logic
└── archiver.py          # AES-256-GCM file archival
```

### CLI Commands (`penf_lib/cli/`)

```
penf_lib/cli/
├── main.py              # Add: cli.add_command(ingest_group)
└── ingest_commands.py   # NEW: ingest command group
```

### Storage (`penf_lib/storage/`)

```
penf_lib/storage/
├── models.py            # Add: IngestJob, IngestError, EmailAttachment, ArchivedFile
└── repositories/
    └── ingest.py        # NEW: IngestJob repository
```

### Events (`penf_lib/events/`)

```
penf_lib/events/
├── schemas.py           # Add: ManualEmailIngestedEvent, IngestJobProgressEvent
└── publishers.py        # Add: IngestEventPublisher
```

---

## Implementation Order

### Phase 1: Core Parsing (TDD)

1. **Write tests first** for EML parser:
   ```python
   # tests/unit/test_ingest_parser.py
   def test_parse_valid_eml():
       result = parse_eml_file(Path("fixtures/valid.eml"))
       assert result["message_id"] == "<test@example.com>"
       assert result["from_email"] == "sender@example.com"
   ```

2. **Implement parser** in `penf_lib/ingest/parser.py`

3. **Test synthetic Message-ID generation**:
   ```python
   def test_synthetic_message_id_when_missing():
       result = parse_eml_file(Path("fixtures/no_message_id.eml"))
       assert result["message_id"].startswith("<synthetic-")
   ```

### Phase 2: Storage Models

1. **Add Alembic migration** for new tables:
   ```bash
   alembic revision --autogenerate -m "Add ingest tables"
   ```

2. **Implement SQLAlchemy models** in `storage/models.py`

3. **Add repository** in `storage/repositories/ingest.py`

### Phase 3: Deduplication

1. **Write tests** for duplicate detection:
   ```python
   async def test_duplicate_by_message_id():
       # Insert existing source
       # Attempt duplicate insert
       assert is_duplicate == True
   ```

2. **Implement deduplication** in `ingest/deduplication.py`

### Phase 4: Batch Processing

1. **Write integration tests** for batch processing
2. **Implement processor** with Rich progress bars
3. **Add resume capability** via IngestJob checkpointing

### Phase 5: CLI Commands

1. **Implement ingest_commands.py** following gmail_commands.py patterns
2. **Register in main.py**
3. **Add contract tests** for CLI

### Phase 6: Events & Archival

1. **Add event schemas** to `events/schemas.py`
2. **Implement publisher** in `events/publishers.py`
3. **Implement AES-256-GCM archiver** in `ingest/archiver.py`

---

## Test Fixtures

Create test .eml files in `tests/fixtures/ingest/`:

```
tests/fixtures/ingest/
├── valid_simple.eml           # Basic email, all headers
├── valid_with_attachment.eml  # Email with PDF attachment
├── valid_thread_reply.eml     # Email with In-Reply-To
├── no_message_id.eml          # Missing Message-ID (synthetic test)
├── future_date.eml            # Date in future (fallback test)
├── invalid_encoding.eml       # Non-UTF8 charset
├── malformed.eml              # Invalid MIME structure
└── empty.eml                  # Empty file
```

---

## Environment Variables

```bash
# Required for encryption
export PENF_ARCHIVE_MASTER_KEY="your-32-byte-key-here"

# Standard Penfold config
export PENFOLD_DATABASE_URL="postgresql+asyncpg://..."
export PENFOLD_REDIS_URL="redis://localhost:6379"
export PENFOLD_TENANT="work"
```

---

## Running Tests

```bash
# Unit tests only
pytest tests/unit/test_ingest_*.py -v

# Integration tests (requires DB)
pytest tests/integration/test_ingest_pipeline.py -v

# Contract tests
pytest tests/contract/test_ingest_*.py -v

# All ingest tests with coverage
pytest tests/ -k ingest --cov=penf_lib.ingest --cov-report=html
```

---

## Manual Testing

```bash
# Activate tenant
penf tenant switch work

# Single file test
penf ingest email tests/fixtures/ingest/valid_simple.eml --source "test"

# Dry run a directory
penf ingest email ./my-emails/ --source "outlook" --dry-run

# Verbose mode
penf ingest email ./my-emails/ --source "outlook" -v

# Check job status
penf ingest jobs

# Resume interrupted job
penf ingest --resume <job-id>
```

---

## Key Patterns

### Async Processing Pattern

```python
async def process_batch(files: list[Path], source_tag: str) -> dict:
    async with get_session() as session:
        job = await create_ingest_job(session, source_tag, files)

        for file_path in files:
            try:
                parsed = parse_eml_file(file_path)
                if await is_duplicate(session, parsed["message_id"]):
                    job.skipped_count += 1
                    continue

                source = await store_source(session, parsed)
                await publish_ingested_event(source)
                job.imported_count += 1

            except Exception as e:
                await log_error(session, job.id, file_path, e)
                job.failed_count += 1

            await update_job_progress(session, job, file_path)

        return job.to_summary()
```

### Encryption Pattern

```python
def archive_original(tenant_id: UUID, source_id: int, content: bytes) -> ArchivedFile:
    # Derive tenant key
    master_key = os.environ["PENF_ARCHIVE_MASTER_KEY"].encode()
    tenant_key = FileArchiver.derive_tenant_key(master_key, str(tenant_id))

    # Encrypt with AAD
    archiver = FileArchiver(tenant_key)
    encrypted = archiver.encrypt(content, associated_data=str(source_id).encode())

    return ArchivedFile(
        tenant_id=tenant_id,
        source_id=source_id,
        encrypted_content=encrypted,
        original_size=len(content),
        encryption_key_id=f"tenant-{tenant_id}-v1",
        content_hash=hashlib.sha256(content).hexdigest(),
    )
```

---

## Common Issues

### "No .eml files found"
- Check glob pattern syntax
- Ensure files have `.eml` extension
- Verify directory path is correct

### "Duplicate detected"
- Email already exists (same Message-ID)
- Check with: `penf search --source "your-source-tag"`

### "Encryption key error"
- Set `PENF_ARCHIVE_MASTER_KEY` environment variable
- Key must be 32 bytes for AES-256

### "Database connection failed"
- Run `penf health` to diagnose
- Check `PENFOLD_DATABASE_URL`

---

## Next Steps

After implementing core features:

1. **Run full test suite**: Ensure >80% coverage
2. **Update CHANGELOG**: Document new commands
3. **Create PR**: Reference spec and tasks
4. **Demo**: Show single file and batch upload
