"""Manual Content Ingest Framework.

Type-based manual ingest framework enabling users to upload archived content
files (starting with .eml email files) into Penfold for unified search and
AI processing.

This module provides:
- EML file parsing (User Story 1 - Single Email Upload)
- Batch processing with progress (User Story 2 - Bulk Upload)
- Deduplication via Message-ID and content hash (User Story 3)
- Attachment extraction (User Story 4)
- Original file archival with AES-256-GCM encryption (User Story 6)

Integration Points:
- Storage (001) for Source persistence
- Event Processing (002) for AI pipeline triggering
- Search Interface (007) for unified search (User Story 5)
- Gmail Integration (004) for shared email model

Reference: specs/012-manual-ingest/spec.md
"""

# Core models (no external dependencies)
from .models import (
    ParsedEmail,
    IngestJobCreate,
    IngestJobStatus,
    IngestResult,
    IngestError,
    AttachmentInfo,
)

# Parser (uses only Python stdlib email module)
from .parser import parse_eml_file, parse_eml_bytes, ParseError

# Processor (requires Rich)
try:
    from .processor import BatchProcessor, ProcessingResult
except ImportError:
    BatchProcessor = None  # type: ignore
    ProcessingResult = None  # type: ignore

# Deduplication (requires SQLAlchemy)
try:
    from .deduplication import check_duplicate, DuplicateCheckResult
except ImportError:
    check_duplicate = None  # type: ignore
    DuplicateCheckResult = None  # type: ignore

# Archiver (requires cryptography)
try:
    from .archiver import FileArchiver, ArchiveResult
except ImportError:
    FileArchiver = None  # type: ignore
    ArchiveResult = None  # type: ignore

__all__ = [
    # Models
    "ParsedEmail",
    "IngestJobCreate",
    "IngestJobStatus",
    "IngestResult",
    "IngestError",
    "AttachmentInfo",
    # Parser
    "parse_eml_file",
    "parse_eml_bytes",
    "ParseError",
    # Processor
    "BatchProcessor",
    "ProcessingResult",
    # Deduplication
    "check_duplicate",
    "DuplicateCheckResult",
    # Archiver
    "FileArchiver",
    "ArchiveResult",
]
