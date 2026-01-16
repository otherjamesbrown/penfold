"""Repository for Ingest Job entities.

Provides database operations for:
- IngestJob: Batch upload job tracking
- IngestError: Failed file records

Reference: specs/012-manual-ingest/data-model.md
"""

from datetime import datetime, timezone
from typing import Any, Optional
from uuid import UUID

from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.storage.models import IngestJob, IngestError as IngestErrorModel, EmailAttachment
from .base import BaseRepository


class IngestJobRepository(BaseRepository[IngestJob]):
    """Repository for IngestJob entities.

    Provides CRUD operations and specialized queries for
    ingest job tracking and progress management.
    """

    def __init__(self, session: AsyncSession):
        """Initialize repository with database session.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, IngestJob)

    async def create_job(
        self,
        tenant_id: UUID,
        source_tag: str,
        file_manifest: list[str],
        content_type: str = "email",
        options: Optional[dict[str, Any]] = None,
    ) -> IngestJob:
        """Create a new ingest job.

        Args:
            tenant_id: Tenant ID for isolation
            source_tag: User-provided source identifier
            file_manifest: List of file paths to process
            content_type: Type of content (email, document, etc.)
            options: Job options (preserve_folders, etc.)

        Returns:
            Created IngestJob instance
        """
        job = IngestJob(
            tenant_id=tenant_id,
            source_tag=source_tag,
            content_type=content_type,
            total_files=len(file_manifest),
            file_manifest=file_manifest,
            processed_files=[],
            options=options or {},
            status="pending",
        )
        self.session.add(job)
        await self.session.flush()
        return job

    async def get_job(self, job_id: UUID, tenant_id: UUID) -> Optional[IngestJob]:
        """Get job by ID with tenant isolation.

        Args:
            job_id: Job identifier
            tenant_id: Tenant ID for isolation

        Returns:
            IngestJob or None if not found
        """
        stmt = select(IngestJob).where(
            IngestJob.id == job_id,
            IngestJob.tenant_id == tenant_id,
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def start_job(self, job_id: UUID) -> None:
        """Mark job as started.

        Args:
            job_id: Job identifier
        """
        stmt = (
            update(IngestJob)
            .where(IngestJob.id == job_id)
            .values(
                status="in_progress",
                started_at=datetime.now(timezone.utc),
            )
        )
        await self.session.execute(stmt)

    async def update_progress(
        self,
        job_id: UUID,
        processed_file: str,
        status: str,  # "imported", "skipped", "failed"
    ) -> None:
        """Update job progress after processing a file.

        Args:
            job_id: Job identifier
            processed_file: Path to completed file
            status: Processing result for this file
        """
        # Get current job state
        stmt = select(IngestJob).where(IngestJob.id == job_id)
        result = await self.session.execute(stmt)
        job = result.scalar_one()

        # Update counters
        job.processed_count += 1
        if status == "imported":
            job.imported_count += 1
        elif status == "skipped":
            job.skipped_count += 1
        elif status == "failed":
            job.failed_count += 1

        # Add to processed files list (for resume)
        processed_files = list(job.processed_files)
        processed_files.append(processed_file)
        job.processed_files = processed_files

        await self.session.flush()

    async def complete_job(
        self,
        job_id: UUID,
        status: str = "completed",
    ) -> None:
        """Mark job as completed.

        Args:
            job_id: Job identifier
            status: Final status (completed, failed, cancelled)
        """
        stmt = (
            update(IngestJob)
            .where(IngestJob.id == job_id)
            .values(
                status=status,
                completed_at=datetime.now(timezone.utc),
            )
        )
        await self.session.execute(stmt)

    async def list_jobs(
        self,
        tenant_id: UUID,
        status: Optional[str] = None,
        limit: int = 10,
        offset: int = 0,
    ) -> list[IngestJob]:
        """List jobs for a tenant.

        Args:
            tenant_id: Tenant ID for isolation
            status: Optional status filter
            limit: Maximum results
            offset: Skip first N results

        Returns:
            List of IngestJob instances
        """
        stmt = select(IngestJob).where(IngestJob.tenant_id == tenant_id)

        if status:
            stmt = stmt.where(IngestJob.status == status)

        stmt = stmt.order_by(IngestJob.created_at.desc()).limit(limit).offset(offset)

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_unprocessed_files(self, job_id: UUID) -> list[str]:
        """Get files not yet processed for resume capability.

        Args:
            job_id: Job identifier

        Returns:
            List of unprocessed file paths
        """
        stmt = select(IngestJob).where(IngestJob.id == job_id)
        result = await self.session.execute(stmt)
        job = result.scalar_one_or_none()

        if not job:
            return []

        processed_set = set(job.processed_files)
        return [f for f in job.file_manifest if f not in processed_set]

    async def get_job_summary(self, job_id: UUID) -> Optional[dict[str, Any]]:
        """Get summary statistics for a job.

        Args:
            job_id: Job identifier

        Returns:
            Summary dictionary or None if not found
        """
        stmt = select(IngestJob).where(IngestJob.id == job_id)
        result = await self.session.execute(stmt)
        job = result.scalar_one_or_none()

        if not job:
            return None

        duration = None
        if job.started_at and job.completed_at:
            duration = (job.completed_at - job.started_at).total_seconds()

        return {
            "job_id": str(job.id),
            "source_tag": job.source_tag,
            "status": job.status,
            "total_files": job.total_files,
            "imported_count": job.imported_count,
            "skipped_count": job.skipped_count,
            "failed_count": job.failed_count,
            "started_at": job.started_at,
            "completed_at": job.completed_at,
            "duration_seconds": duration,
        }


class IngestErrorRepository(BaseRepository[IngestErrorModel]):
    """Repository for IngestError entities.

    Tracks detailed error information for failed files
    during batch processing.
    """

    def __init__(self, session: AsyncSession):
        """Initialize repository with database session.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, IngestErrorModel)

    async def record_error(
        self,
        job_id: UUID,
        file_path: str,
        error_type: str,
        error_message: str,
        error_details: Optional[dict[str, Any]] = None,
    ) -> IngestErrorModel:
        """Record a file processing error.

        Args:
            job_id: Parent job ID
            file_path: Path to failed file
            error_type: Error category
            error_message: Human-readable message
            error_details: Additional context

        Returns:
            Created IngestError instance
        """
        error = IngestErrorModel(
            job_id=job_id,
            file_path=file_path,
            error_type=error_type,
            error_message=error_message,
            error_details=error_details,
        )
        self.session.add(error)
        await self.session.flush()
        return error

    async def get_errors_for_job(self, job_id: UUID) -> list[IngestErrorModel]:
        """Get all errors for a job.

        Args:
            job_id: Job identifier

        Returns:
            List of IngestError instances
        """
        stmt = (
            select(IngestErrorModel)
            .where(IngestErrorModel.job_id == job_id)
            .order_by(IngestErrorModel.created_at)
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_error_summary(self, job_id: UUID) -> dict[str, int]:
        """Get error counts by type for a job.

        Args:
            job_id: Job identifier

        Returns:
            Dictionary of error_type -> count
        """
        errors = await self.get_errors_for_job(job_id)
        summary: dict[str, int] = {}
        for error in errors:
            summary[error.error_type] = summary.get(error.error_type, 0) + 1
        return summary


class EmailAttachmentRepository(BaseRepository[EmailAttachment]):
    """Repository for EmailAttachment entities.

    Tracks attachments extracted from manually ingested emails.
    """

    def __init__(self, session: AsyncSession):
        """Initialize repository with database session.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, EmailAttachment)

    async def create_attachment(
        self,
        tenant_id: UUID,
        source_id: int,
        filename: str,
        mime_type: str,
        size_bytes: int,
        extraction_status: str = "pending",
        extraction_error: Optional[str] = None,
    ) -> EmailAttachment:
        """Create a new email attachment record.

        Args:
            tenant_id: Tenant ID for isolation
            source_id: Parent email source ID
            filename: Original filename
            mime_type: MIME content type
            size_bytes: File size in bytes
            extraction_status: Status of extraction
            extraction_error: Error message if extraction failed

        Returns:
            Created EmailAttachment instance
        """
        attachment = EmailAttachment(
            tenant_id=tenant_id,
            source_id=source_id,
            filename=filename,
            mime_type=mime_type,
            size_bytes=size_bytes,
            extraction_status=extraction_status,
            extraction_error=extraction_error,
        )
        self.session.add(attachment)
        await self.session.flush()
        return attachment

    async def get_attachments_for_source(self, source_id: int) -> list[EmailAttachment]:
        """Get all attachments for an email source.

        Args:
            source_id: Parent email source ID

        Returns:
            List of EmailAttachment instances
        """
        stmt = (
            select(EmailAttachment)
            .where(EmailAttachment.source_id == source_id)
            .order_by(EmailAttachment.created_at)
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update_extraction_status(
        self,
        attachment_id: UUID,
        status: str,
        document_id: Optional[UUID] = None,
        error: Optional[str] = None,
    ) -> None:
        """Update attachment extraction status.

        Args:
            attachment_id: Attachment identifier
            status: New extraction status
            document_id: Optional document link if extraction completed
            error: Optional error message
        """
        values: dict[str, Any] = {"extraction_status": status}
        if document_id:
            values["document_id"] = document_id
        if error:
            values["extraction_error"] = error

        stmt = (
            update(EmailAttachment)
            .where(EmailAttachment.id == attachment_id)
            .values(**values)
        )
        await self.session.execute(stmt)

    async def get_pending_attachments(
        self,
        tenant_id: UUID,
        limit: int = 100,
    ) -> list[EmailAttachment]:
        """Get attachments pending extraction.

        Args:
            tenant_id: Tenant ID for isolation
            limit: Maximum results

        Returns:
            List of pending EmailAttachment instances
        """
        stmt = (
            select(EmailAttachment)
            .where(
                EmailAttachment.tenant_id == tenant_id,
                EmailAttachment.extraction_status == "pending",
            )
            .order_by(EmailAttachment.created_at)
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return list(result.scalars().all())
