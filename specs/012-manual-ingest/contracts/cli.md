# CLI Contract: Manual Content Ingest

**Feature**: 012-manual-ingest | **Date**: 2026-01-16

## Overview

CLI interface specification for the `penf ingest` command group. Defines commands, options, output formats, and exit codes.

---

## Command Structure

```
penf ingest <type> <path> --source <tag> [options]
penf ingest --resume <job-id>
penf ingest jobs [--status <status>]
```

---

## Commands

### 1. `penf ingest email`

Upload .eml files for processing.

**Signature:**
```
penf ingest email <PATH> --source <TAG> [OPTIONS]
```

**Arguments:**

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| PATH | string | Yes | File path, directory, or glob pattern |

**Options:**

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `--source`, `-s` | string | Required | Source identifier for tracking |
| `--projects` | string | None | Comma-separated project tags |
| `--dry-run` | flag | False | Preview without importing |
| `--no-preserve-folders` | flag | False | Don't create labels from folders |
| `--skip-attachments` | flag | False | Don't extract attachments |
| `--verbose`, `-v` | flag | False | Show detailed progress |

**Examples:**
```bash
# Single file
penf ingest email invoice.eml --source "outlook-archive"

# Directory
penf ingest email ./emails/ --source "outlook-2024"

# Glob pattern
penf ingest email "./backup/**/*.eml" --source "old-laptop"

# With project tags
penf ingest email *.eml --source "archive" --projects "Atlas,SOC2"

# Dry run
penf ingest email ./emails/ --source "test" --dry-run

# Without folder labels
penf ingest email ./emails/ --source "flat-archive" --no-preserve-folders
```

---

### 2. `penf ingest --resume`

Resume an interrupted batch job.

**Signature:**
```
penf ingest --resume <JOB-ID>
```

**Arguments:**

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| JOB-ID | UUID | Yes | Job ID from previous run |

**Example:**
```bash
penf ingest --resume job-abc123-def456
```

**Behavior:**
- Loads job manifest from database
- Skips already-processed files
- Continues from last checkpoint
- Updates existing job record

---

### 3. `penf ingest jobs`

List ingest jobs.

**Signature:**
```
penf ingest jobs [OPTIONS]
```

**Options:**

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `--status` | string | None | Filter by status (pending, in_progress, completed, failed) |
| `--limit` | int | 10 | Maximum jobs to show |

**Example:**
```bash
penf ingest jobs --status in_progress
```

---

## Output Formats

### Standard Output (email ingest)

```
$ penf ingest email ./emails/ --source "outlook-2024"

Scanning directory... found 150 .eml files

Processing emails ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100% 150/150

Summary:
  Job ID:    job-abc123-def456
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

### Dry Run Output

```
$ penf ingest email ./emails/ --source "test" --dry-run

DRY RUN - No changes will be made

Scanning directory... found 150 .eml files

Would process:
  New emails:      142
  Duplicates:        5 (would skip)
  Invalid files:     3 (would skip)

Labels that would be created:
  - inbox
  - sent
  - projects/atlas
  - projects/soc2

Attachments that would be extracted: 23

To proceed, run without --dry-run flag.
```

### Verbose Output

When `--verbose` is set, show per-file progress:

```
$ penf ingest email ./emails/ --source "archive" -v

Processing: inbox/email_001.eml
  ✓ Parsed successfully
  ✓ Message-ID: <abc123@example.com>
  ✓ Date: 2026-01-10 14:30:00
  ✓ From: bob@example.com
  ✓ Attachments: 2 (report.pdf, data.xlsx)
  ✓ Stored as source_id: 12345

Processing: inbox/email_002.eml
  ⚠ Duplicate detected - skipping

...
```

### Job List Output

```
$ penf ingest jobs

ID                                    Source Tag       Status       Files    Progress    Created
────────────────────────────────────────────────────────────────────────────────────────────────
job-abc123-def456                     outlook-2024     completed    150      150/150     2 hours ago
job-xyz789-uvw012                     old-backup       in_progress  1000     450/1000    5 mins ago
job-mno345-pqr678                     test-import      failed       50       23/50       1 day ago
```

---

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| 0 | Success | All files processed successfully |
| 1 | Partial failure | Some files failed, others succeeded |
| 2 | Complete failure | No files could be processed |
| 3 | Invalid input | Bad arguments, missing source tag, etc. |
| 4 | IO error | Cannot read files or directory |
| 5 | Database error | Cannot connect or write to database |
| 130 | Interrupted | User pressed Ctrl+C |

---

## Error Messages

### Validation Errors

```
Error: --source is required for all ingest operations

Error: Path not found: ./nonexistent/

Error: No .eml files found in ./empty-dir/

Error: Invalid job ID format: not-a-uuid
```

### Runtime Errors

```
Error: Cannot connect to database. Run 'penf health' to diagnose.

Error: Job job-abc123 not found or belongs to different tenant.

Error: Job job-abc123 is already completed. Nothing to resume.
```

---

## Contract Tests

Required contract tests for CLI:

```python
# tests/contract/test_ingest_cli.py

def test_source_flag_required():
    """--source flag must be required."""
    result = runner.invoke(cli, ["ingest", "email", "test.eml"])
    assert result.exit_code == 3
    assert "--source is required" in result.output

def test_dry_run_makes_no_changes():
    """--dry-run must not modify database."""
    # Create test file
    # Run with --dry-run
    # Verify no Source records created

def test_exit_code_partial_failure():
    """Exit code 1 when some files fail."""
    # Create mix of valid and invalid files
    # Run ingest
    assert result.exit_code == 1

def test_resume_continues_from_checkpoint():
    """--resume must skip already-processed files."""
    # Create job with partial progress
    # Run --resume
    # Verify only remaining files processed
```

---

## Integration with Existing CLI

The `ingest` command group integrates with existing CLI structure:

```python
# penf_lib/cli/main.py

from .ingest_commands import ingest_group

# ...existing registrations...
cli.add_command(ingest_group)
```

Follows patterns established by:
- `gmail_commands.py` - Command group structure
- `search_commands.py` - Progress display patterns
- `review_commands.py` - Async command execution
