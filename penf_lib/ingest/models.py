"""Pydantic models for Manual Content Ingest.

Defines data models for:
- Parsed email content and metadata
- Ingest job tracking and progress
- Error handling and reporting
- Attachment information

Reference: specs/012-manual-ingest/data-model.md
"""

from datetime import datetime, timezone
from enum import Enum
from pathlib import Path
from typing import Any, Optional
from uuid import UUID

from pydantic import BaseModel, Field, field_validator


class IngestJobStatus(str, Enum):
    """Status states for ingest jobs."""

    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class AttachmentExtractionStatus(str, Enum):
    """Status states for attachment extraction."""

    PENDING = "pending"
    COMPLETED = "completed"
    FAILED = "failed"
    SKIPPED_OVERSIZED = "skipped_oversized"
    SKIPPED_UNSUPPORTED = "skipped_unsupported"


class AttachmentInfo(BaseModel):
    """Information about an email attachment.

    Attributes:
        filename: Original filename
        mime_type: MIME content type
        size_bytes: File size in bytes
        extraction_status: Current extraction state
        content: Raw attachment content (None if skipped)
    """

    filename: str
    mime_type: str
    size_bytes: int
    extraction_status: AttachmentExtractionStatus = AttachmentExtractionStatus.PENDING
    content: Optional[bytes] = None

    class Config:
        """Pydantic configuration."""

        arbitrary_types_allowed = True


class ParsedEmail(BaseModel):
    """Parsed email content from .eml file.

    Contains all extracted metadata and content from an email file.
    Used as intermediate representation before storage.

    Reference: research.md Section 1 - EML Parsing
    """

    # Identifiers
    message_id: str = Field(description="Message-ID header (real or synthetic)")
    message_id_synthetic: bool = Field(
        default=False, description="True if Message-ID was generated"
    )

    # Participants
    from_email: str = Field(description="Sender email address")
    from_name: Optional[str] = Field(default=None, description="Sender display name")
    to_emails: list[str] = Field(default_factory=list, description="To recipients")
    cc_emails: list[str] = Field(default_factory=list, description="CC recipients")
    bcc_emails: list[str] = Field(default_factory=list, description="BCC recipients")

    # Content
    subject: Optional[str] = Field(default=None, description="Email subject")
    body_plain: Optional[str] = Field(default=None, description="Plain text body")
    body_html: Optional[str] = Field(default=None, description="HTML body")

    # Dates
    email_date: Optional[datetime] = Field(
        default=None, description="Original email date"
    )
    date_fallback_used: bool = Field(
        default=False, description="True if ingestion timestamp was used"
    )
    original_date_raw: Optional[str] = Field(
        default=None, description="Original Date header string"
    )

    # Threading
    in_reply_to: Optional[str] = Field(
        default=None, description="In-Reply-To header"
    )
    references: list[str] = Field(
        default_factory=list, description="References header values"
    )

    # Attachments
    attachments: list[AttachmentInfo] = Field(
        default_factory=list, description="Extracted attachments"
    )

    # Raw content
    raw_content: bytes = Field(description="Original .eml file bytes")
    content_hash: str = Field(description="SHA-256 hash of raw content")

    # Source tracking
    original_file_path: Optional[str] = Field(
        default=None, description="Original file path in batch"
    )

    class Config:
        """Pydantic configuration."""

        arbitrary_types_allowed = True

    @property
    def has_attachments(self) -> bool:
        """Check if email has attachments."""
        return len(self.attachments) > 0

    @property
    def attachment_count(self) -> int:
        """Count of attachments."""
        return len(self.attachments)


class IngestJobCreate(BaseModel):
    """Request to create an ingest job.

    Attributes:
        source_tag: User-provided source identifier (required)
        file_paths: List of files to process
        preserve_folders: Whether to create labels from folder structure
        skip_attachments: Whether to skip attachment extraction
        dry_run: Preview mode without persistence
    """

    source_tag: str = Field(min_length=1, max_length=100)
    file_paths: list[Path]
    preserve_folders: bool = True
    skip_attachments: bool = False
    dry_run: bool = False
    projects: list[str] = Field(default_factory=list)

    @field_validator("source_tag")
    @classmethod
    def validate_source_tag(cls, v: str) -> str:
        """Validate source tag format."""
        import re

        if not re.match(r"^[a-zA-Z0-9_-]+$", v):
            raise ValueError(
                "source_tag must contain only alphanumeric characters, hyphens, and underscores"
            )
        return v


class IngestError(BaseModel):
    """Error record for failed file processing.

    Attributes:
        file_path: Path to the failed file
        error_type: Category of error
        error_message: Human-readable error description
        error_details: Additional context (stack trace, etc.)
    """

    file_path: str
    error_type: str
    error_message: str
    error_details: Optional[dict[str, Any]] = None


class IngestResult(BaseModel):
    """Result of processing a single file.

    Attributes:
        file_path: Path to processed file
        status: Processing outcome
        source_id: Database ID if successfully stored
        message_id: Email Message-ID
        error: Error details if failed
    """

    file_path: str
    status: str  # "imported", "skipped", "failed"
    source_id: Optional[int] = None
    message_id: Optional[str] = None
    error: Optional[IngestError] = None


class IngestJobSummary(BaseModel):
    """Summary of completed ingest job.

    Attributes:
        job_id: Unique job identifier
        source_tag: User-provided source identifier
        total_files: Total files in batch
        imported_count: Successfully imported
        skipped_count: Skipped (duplicates)
        failed_count: Failed to process
        labels_created: Labels from folder structure
        attachments_extracted: Count of extracted attachments
        duration_seconds: Total processing time
    """

    job_id: UUID
    source_tag: str
    total_files: int
    imported_count: int = 0
    skipped_count: int = 0
    failed_count: int = 0
    labels_created: list[str] = Field(default_factory=list)
    attachments_extracted: int = 0
    attachments_skipped: int = 0
    errors: list[IngestError] = Field(default_factory=list)
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None
    duration_seconds: Optional[float] = None
