"""Smart attachment processing with immediate and deferred extraction."""

import asyncio
import logging
import mimetypes
import os
import tempfile
from pathlib import Path
from typing import Dict, List, Optional, Tuple
from datetime import datetime

import magic
from celery import Celery
from sqlalchemy.ext.asyncio import AsyncSession

from ...models.email_message import EmailAttachment
from ...storage.database import get_db_session
from .client import GmailClient
from ..extraction.content_extractors import ContentExtractor, ExtractionResult

logger = logging.getLogger(__name__)

# Size limits for immediate processing (10MB)
IMMEDIATE_PROCESSING_SIZE_LIMIT = 10 * 1024 * 1024
MAX_ATTACHMENT_SIZE = 100 * 1024 * 1024  # 100MB absolute limit

# Supported formats for immediate processing
IMMEDIATE_FORMATS = {
    'application/pdf',
    'text/plain',
    'text/csv',
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',  # .docx
    'application/msword',  # .doc
    'image/jpeg',
    'image/png',
    'image/gif',
    'image/webp'
}

# Formats requiring background processing
DEFERRED_FORMATS = {
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',  # .xlsx
    'application/vnd.ms-excel',  # .xls
    'application/vnd.openxmlformats-officedocument.presentationml.presentation',  # .pptx
    'application/vnd.ms-powerpoint',  # .ppt
    'application/zip',
    'application/x-zip-compressed',
    'video/mp4',
    'video/avi',
    'video/quicktime',
    'audio/mpeg',
    'audio/wav'
}


class AttachmentProcessor:
    """Smart attachment processing coordinator."""

    def __init__(self, gmail_client: GmailClient, celery_app: Optional[Celery] = None):
        """Initialize attachment processor.

        Args:
            gmail_client: Gmail API client for downloading attachments
            celery_app: Optional Celery app for background processing
        """
        self.gmail_client = gmail_client
        self.celery_app = celery_app
        self.content_extractor = ContentExtractor()

        # Create attachments directory
        self.storage_path = Path(tempfile.gettempdir()) / "penfold_attachments"
        self.storage_path.mkdir(exist_ok=True)

    async def process_message_attachments(
        self,
        gmail_message_id: str,
        attachments: List[Dict]
    ) -> List[int]:
        """Process all attachments for a Gmail message.

        Args:
            gmail_message_id: Gmail message ID
            attachments: List of attachment metadata from Gmail API

        Returns:
            List of created EmailAttachment record IDs
        """
        attachment_ids = []

        async with get_db_session() as session:
            for attachment_data in attachments:
                try:
                    attachment_id = await self._process_single_attachment(
                        session, gmail_message_id, attachment_data
                    )
                    if attachment_id:
                        attachment_ids.append(attachment_id)
                except Exception as e:
                    logger.error(
                        f"Failed to process attachment {attachment_data.get('filename', 'unknown')}: {e}"
                    )
                    continue

            await session.commit()

        return attachment_ids

    async def _process_single_attachment(
        self,
        session: AsyncSession,
        gmail_message_id: str,
        attachment_data: Dict
    ) -> Optional[int]:
        """Process a single attachment.

        Args:
            session: Database session
            gmail_message_id: Gmail message ID
            attachment_data: Attachment metadata from Gmail API

        Returns:
            Created EmailAttachment record ID or None if failed
        """
        filename = attachment_data.get('filename', 'unknown')
        mime_type = attachment_data.get('mimeType', 'application/octet-stream')
        size_bytes = attachment_data.get('size', 0)
        gmail_attachment_id = attachment_data.get('attachmentId', '')

        # Validate size limits
        if size_bytes > MAX_ATTACHMENT_SIZE:
            logger.warning(f"Attachment {filename} too large ({size_bytes} bytes), skipping")
            return None

        # Create attachment record
        attachment = EmailAttachment(
            gmail_attachment_id=gmail_attachment_id,
            filename=filename,
            mime_type=mime_type,
            size_bytes=size_bytes,
            download_status='pending',
            extraction_status='pending'
        )

        # Find the associated email message
        from ...models.email_message import EmailMessage
        message = await session.scalar(
            session.query(EmailMessage).filter_by(gmail_message_id=gmail_message_id)
        )
        if not message:
            logger.error(f"Email message {gmail_message_id} not found")
            return None

        attachment.message_id = message.id
        session.add(attachment)
        await session.flush()  # Get the ID

        # Route to immediate or deferred processing
        processing_type = self._determine_processing_type(mime_type, size_bytes)

        if processing_type == 'immediate':
            # Process immediately
            success = await self._process_attachment_immediate(
                session, attachment, gmail_message_id
            )
            if success:
                logger.info(f"Immediate processing completed for {filename}")
        elif processing_type == 'deferred':
            # Queue for background processing
            if self.celery_app:
                self._queue_background_processing(attachment.id, gmail_message_id)
                logger.info(f"Queued background processing for {filename}")
            else:
                logger.warning(f"No Celery app available for background processing of {filename}")
                attachment.extraction_status = 'unsupported'
        else:
            # Unsupported format
            attachment.extraction_status = 'unsupported'
            logger.info(f"Unsupported format for {filename}: {mime_type}")

        return attachment.id

    def _determine_processing_type(self, mime_type: str, size_bytes: int) -> str:
        """Determine how to process an attachment.

        Args:
            mime_type: MIME type of the attachment
            size_bytes: Size in bytes

        Returns:
            'immediate', 'deferred', or 'unsupported'
        """
        # Check if format is supported at all
        if mime_type not in IMMEDIATE_FORMATS and mime_type not in DEFERRED_FORMATS:
            return 'unsupported'

        # For supported formats, decide based on size and complexity
        if mime_type in IMMEDIATE_FORMATS and size_bytes <= IMMEDIATE_PROCESSING_SIZE_LIMIT:
            return 'immediate'
        elif mime_type in DEFERRED_FORMATS or size_bytes > IMMEDIATE_PROCESSING_SIZE_LIMIT:
            return 'deferred'
        else:
            return 'unsupported'

    async def _process_attachment_immediate(
        self,
        session: AsyncSession,
        attachment: EmailAttachment,
        gmail_message_id: str
    ) -> bool:
        """Process attachment immediately.

        Args:
            session: Database session
            attachment: EmailAttachment record
            gmail_message_id: Gmail message ID

        Returns:
            True if processing succeeded, False otherwise
        """
        try:
            # Update status
            attachment.extraction_status = 'processing'
            await session.flush()

            # Download attachment
            file_path = await self._download_attachment(
                gmail_message_id,
                attachment.gmail_attachment_id,
                attachment.filename
            )

            if not file_path:
                attachment.extraction_status = 'failed'
                attachment.download_status = 'failed'
                return False

            attachment.file_path = str(file_path)
            attachment.download_status = 'completed'
            attachment.downloaded_at = datetime.utcnow()

            # Extract content
            extraction_result = await self.content_extractor.extract_content(
                file_path, attachment.mime_type
            )

            if extraction_result.success:
                # Store extracted content (could be in events or separate content table)
                attachment.content_extracted = True
                attachment.extraction_status = 'completed'
                attachment.processed_at = datetime.utcnow()

                # Publish content extraction event
                await self._publish_content_event(attachment, extraction_result)

                logger.info(f"Successfully extracted content from {attachment.filename}")
                return True
            else:
                attachment.extraction_status = 'failed'
                logger.error(f"Content extraction failed for {attachment.filename}: {extraction_result.error}")
                return False

        except Exception as e:
            attachment.extraction_status = 'failed'
            logger.error(f"Immediate processing failed for {attachment.filename}: {e}")
            return False

    async def _download_attachment(
        self,
        gmail_message_id: str,
        attachment_id: str,
        filename: str
    ) -> Optional[Path]:
        """Download attachment from Gmail.

        Args:
            gmail_message_id: Gmail message ID
            attachment_id: Gmail attachment ID
            filename: Original filename

        Returns:
            Path to downloaded file or None if failed
        """
        try:
            # Create unique filename to avoid conflicts
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            safe_filename = f"{timestamp}_{filename}"
            file_path = self.storage_path / safe_filename

            # Download from Gmail
            attachment_data = await self.gmail_client.get_attachment(
                gmail_message_id, attachment_id
            )

            # Write to file
            file_path.write_bytes(attachment_data)

            # Verify MIME type
            detected_mime = magic.from_file(str(file_path), mime=True)
            logger.debug(f"Downloaded {filename}, detected MIME type: {detected_mime}")

            return file_path

        except Exception as e:
            logger.error(f"Failed to download attachment {filename}: {e}")
            return None

    def _queue_background_processing(self, attachment_id: int, gmail_message_id: str) -> None:
        """Queue attachment for background processing.

        Args:
            attachment_id: EmailAttachment record ID
            gmail_message_id: Gmail message ID
        """
        if not self.celery_app:
            logger.error("Cannot queue background processing: no Celery app configured")
            return

        # Queue the task
        self.celery_app.send_task(
            'penf_lib.connectors.gmail.tasks.process_attachment_background',
            args=[attachment_id, gmail_message_id],
            countdown=10  # Start processing in 10 seconds
        )

    async def _publish_content_event(
        self,
        attachment: EmailAttachment,
        extraction_result: ExtractionResult
    ) -> None:
        """Publish content extraction event.

        Args:
            attachment: EmailAttachment record
            extraction_result: Extraction result with content and metadata
        """
        # Import here to avoid circular imports
        from ...events.publishers import EventPublisher
        from ...events.schemas import ContentExtractedEvent

        event = ContentExtractedEvent(
            source_id=f"attachment:{attachment.id}",
            source_type="email_attachment",
            content_type=extraction_result.content_type,
            extracted_text=extraction_result.text_content,
            metadata={
                "filename": attachment.filename,
                "mime_type": attachment.mime_type,
                "size_bytes": attachment.size_bytes,
                "extraction_method": extraction_result.extraction_method,
                "confidence": extraction_result.confidence
            },
            timestamp=datetime.utcnow()
        )

        publisher = EventPublisher()
        await publisher.publish_content_extracted(event)

    async def get_processing_status(self, attachment_id: int) -> Dict:
        """Get processing status for an attachment.

        Args:
            attachment_id: EmailAttachment record ID

        Returns:
            Status information dictionary
        """
        async with get_db_session() as session:
            attachment = await session.get(EmailAttachment, attachment_id)

            if not attachment:
                return {"error": "Attachment not found"}

            return {
                "id": attachment.id,
                "filename": attachment.filename,
                "mime_type": attachment.mime_type,
                "size_bytes": attachment.size_bytes,
                "download_status": attachment.download_status,
                "extraction_status": attachment.extraction_status,
                "content_extracted": attachment.content_extracted,
                "downloaded_at": attachment.downloaded_at.isoformat() if attachment.downloaded_at else None,
                "processed_at": attachment.processed_at.isoformat() if attachment.processed_at else None
            }

    async def retry_failed_attachment(self, attachment_id: int) -> bool:
        """Retry processing for a failed attachment.

        Args:
            attachment_id: EmailAttachment record ID

        Returns:
            True if retry was initiated, False otherwise
        """
        async with get_db_session() as session:
            attachment = await session.get(EmailAttachment, attachment_id)

            if not attachment or attachment.extraction_status not in ['failed', 'pending']:
                return False

            # Reset status and retry
            attachment.extraction_status = 'pending'
            attachment.download_status = 'pending'
            attachment.content_extracted = False

            # Determine processing type and route accordingly
            processing_type = self._determine_processing_type(
                attachment.mime_type,
                attachment.size_bytes
            )

            if processing_type == 'immediate':
                # Find the associated email message
                message = await session.get(EmailMessage, attachment.message_id)
                if message:
                    success = await self._process_attachment_immediate(
                        session, attachment, message.gmail_message_id
                    )
                    await session.commit()
                    return success
            elif processing_type == 'deferred':
                # Re-queue for background processing
                message = await session.get(EmailMessage, attachment.message_id)
                if message:
                    self._queue_background_processing(attachment.id, message.gmail_message_id)
                    await session.commit()
                    return True

            return False


class AttachmentStorageManager:
    """Manages attachment file storage and cleanup."""

    def __init__(self, storage_path: Optional[Path] = None):
        """Initialize storage manager.

        Args:
            storage_path: Base path for attachment storage
        """
        self.storage_path = storage_path or (Path(tempfile.gettempdir()) / "penfold_attachments")
        self.storage_path.mkdir(exist_ok=True)

    async def cleanup_old_attachments(self, max_age_days: int = 30) -> int:
        """Clean up old attachment files.

        Args:
            max_age_days: Maximum age in days before deletion

        Returns:
            Number of files deleted
        """
        deleted_count = 0
        cutoff_time = datetime.now().timestamp() - (max_age_days * 24 * 60 * 60)

        for file_path in self.storage_path.iterdir():
            if file_path.is_file():
                try:
                    if file_path.stat().st_mtime < cutoff_time:
                        file_path.unlink()
                        deleted_count += 1
                except Exception as e:
                    logger.error(f"Failed to delete old attachment file {file_path}: {e}")

        logger.info(f"Cleaned up {deleted_count} old attachment files")
        return deleted_count

    def get_storage_stats(self) -> Dict:
        """Get storage statistics.

        Returns:
            Storage statistics dictionary
        """
        total_files = 0
        total_size = 0

        for file_path in self.storage_path.iterdir():
            if file_path.is_file():
                try:
                    stat = file_path.stat()
                    total_files += 1
                    total_size += stat.st_size
                except Exception:
                    continue

        return {
            "total_files": total_files,
            "total_size_bytes": total_size,
            "total_size_mb": round(total_size / (1024 * 1024), 2),
            "storage_path": str(self.storage_path)
        }