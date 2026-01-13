"""Unit tests for Gmail attachment processing."""

import asyncio
import tempfile
from pathlib import Path
from unittest.mock import Mock, AsyncMock, patch, MagicMock
import pytest
from datetime import datetime

from .attachments import AttachmentProcessor, AttachmentStorageManager, IMMEDIATE_PROCESSING_SIZE_LIMIT
from ...extraction.content_extractors import ContentExtractor, ExtractionResult
from ...models.email_message import EmailMessage, EmailAttachment
from .client import GmailClient


class TestAttachmentProcessor:
    """Test cases for AttachmentProcessor."""

    @pytest.fixture
    def mock_gmail_client(self):
        """Mock Gmail client."""
        client = Mock(spec=GmailClient)
        client.get_attachment = AsyncMock(return_value=b"test file content")
        return client

    @pytest.fixture
    def mock_celery_app(self):
        """Mock Celery app."""
        app = Mock()
        app.send_task = Mock()
        return app

    @pytest.fixture
    def attachment_processor(self, mock_gmail_client, mock_celery_app):
        """Create attachment processor with mocked dependencies."""
        return AttachmentProcessor(mock_gmail_client, mock_celery_app)

    @pytest.fixture
    def sample_attachment_data(self):
        """Sample attachment metadata from Gmail API."""
        return {
            "attachmentId": "test_attachment_123",
            "filename": "test_document.pdf",
            "mimeType": "application/pdf",
            "size": 5 * 1024 * 1024  # 5MB
        }

    @pytest.fixture
    def large_attachment_data(self):
        """Large attachment that should be deferred."""
        return {
            "attachmentId": "large_attachment_456",
            "filename": "large_presentation.pptx",
            "mimeType": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
            "size": 15 * 1024 * 1024  # 15MB
        }

    def test_determine_processing_type_immediate(self, attachment_processor):
        """Test processing type determination for immediate processing."""
        # Small PDF should be immediate
        result = attachment_processor._determine_processing_type(
            "application/pdf", 5 * 1024 * 1024
        )
        assert result == "immediate"

        # Small text file should be immediate
        result = attachment_processor._determine_processing_type(
            "text/plain", 1024
        )
        assert result == "immediate"

    def test_determine_processing_type_deferred(self, attachment_processor):
        """Test processing type determination for deferred processing."""
        # Large PDF should be deferred
        result = attachment_processor._determine_processing_type(
            "application/pdf", 15 * 1024 * 1024
        )
        assert result == "deferred"

        # PPTX regardless of size should be deferred
        result = attachment_processor._determine_processing_type(
            "application/vnd.openxmlformats-officedocument.presentationml.presentation",
            1024
        )
        assert result == "deferred"

    def test_determine_processing_type_unsupported(self, attachment_processor):
        """Test processing type determination for unsupported formats."""
        result = attachment_processor._determine_processing_type(
            "application/x-executable", 1024
        )
        assert result == "unsupported"

        result = attachment_processor._determine_processing_type(
            "video/mp4", 1024
        )
        assert result == "deferred"  # Video is in deferred formats, not unsupported

    @pytest.mark.asyncio
    async def test_download_attachment_success(self, attachment_processor, mock_gmail_client):
        """Test successful attachment download."""
        mock_gmail_client.get_attachment.return_value = b"PDF content here"

        with patch('magic.from_file', return_value="application/pdf"):
            file_path = await attachment_processor._download_attachment(
                "msg_123", "att_456", "test.pdf"
            )

        assert file_path is not None
        assert file_path.exists()
        assert file_path.read_bytes() == b"PDF content here"
        assert "test.pdf" in str(file_path)

        # Cleanup
        file_path.unlink()

    @pytest.mark.asyncio
    async def test_download_attachment_failure(self, attachment_processor, mock_gmail_client):
        """Test attachment download failure."""
        mock_gmail_client.get_attachment.side_effect = Exception("API Error")

        file_path = await attachment_processor._download_attachment(
            "msg_123", "att_456", "test.pdf"
        )

        assert file_path is None

    def test_queue_background_processing(self, attachment_processor, mock_celery_app):
        """Test queuing background processing task."""
        attachment_processor._queue_background_processing(123, "msg_456")

        mock_celery_app.send_task.assert_called_once_with(
            'penf_lib.connectors.gmail.tasks.process_attachment_background',
            args=[123, "msg_456"],
            countdown=10
        )

    def test_queue_background_processing_no_celery(self, mock_gmail_client):
        """Test queuing when no Celery app is configured."""
        processor = AttachmentProcessor(mock_gmail_client, None)

        # Should not raise exception
        processor._queue_background_processing(123, "msg_456")

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.attachments.get_db_session')
    async def test_process_message_attachments_mixed_types(
        self, mock_db_session, attachment_processor, sample_attachment_data, large_attachment_data
    ):
        """Test processing multiple attachments with different types."""
        mock_session = AsyncMock()
        mock_db_session.return_value.__aenter__.return_value = mock_session

        # Mock email message lookup
        mock_message = Mock()
        mock_message.id = 1
        mock_session.scalar.return_value = mock_message

        # Mock attachment creation
        mock_session.add = Mock()
        mock_session.flush = AsyncMock()
        mock_session.commit = AsyncMock()

        attachments = [sample_attachment_data, large_attachment_data]

        with patch.object(attachment_processor, '_process_attachment_immediate', return_value=True):
            with patch.object(attachment_processor, '_queue_background_processing'):
                result = await attachment_processor.process_message_attachments(
                    "gmail_msg_123", attachments
                )

        assert len(result) == 2
        assert mock_session.add.call_count == 2
        assert mock_session.commit.called

    @pytest.mark.asyncio
    async def test_get_processing_status(self, attachment_processor):
        """Test getting processing status for attachment."""
        with patch('penf_lib.connectors.gmail.attachments.get_db_session'):
            mock_session = AsyncMock()
            mock_attachment = Mock()
            mock_attachment.id = 123
            mock_attachment.filename = "test.pdf"
            mock_attachment.mime_type = "application/pdf"
            mock_attachment.size_bytes = 1024
            mock_attachment.download_status = "completed"
            mock_attachment.extraction_status = "completed"
            mock_attachment.content_extracted = True
            mock_attachment.downloaded_at = datetime.utcnow()
            mock_attachment.processed_at = datetime.utcnow()

            mock_session.get.return_value = mock_attachment

            with patch('penf_lib.connectors.gmail.attachments.get_db_session') as mock_get_session:
                mock_get_session.return_value.__aenter__.return_value = mock_session

                status = await attachment_processor.get_processing_status(123)

                assert status["id"] == 123
                assert status["filename"] == "test.pdf"
                assert status["download_status"] == "completed"
                assert status["extraction_status"] == "completed"
                assert status["content_extracted"] is True

    @pytest.mark.asyncio
    async def test_get_processing_status_not_found(self, attachment_processor):
        """Test getting status for non-existent attachment."""
        with patch('penf_lib.connectors.gmail.attachments.get_db_session'):
            mock_session = AsyncMock()
            mock_session.get.return_value = None

            with patch('penf_lib.connectors.gmail.attachments.get_db_session') as mock_get_session:
                mock_get_session.return_value.__aenter__.return_value = mock_session

                status = await attachment_processor.get_processing_status(999)

                assert "error" in status
                assert status["error"] == "Attachment not found"


class TestAttachmentStorageManager:
    """Test cases for AttachmentStorageManager."""

    @pytest.fixture
    def temp_storage_path(self):
        """Create temporary storage directory."""
        temp_dir = Path(tempfile.mkdtemp())
        yield temp_dir
        # Cleanup
        for file in temp_dir.glob("*"):
            file.unlink()
        temp_dir.rmdir()

    @pytest.fixture
    def storage_manager(self, temp_storage_path):
        """Create storage manager with temporary directory."""
        return AttachmentStorageManager(temp_storage_path)

    def test_storage_manager_initialization(self, storage_manager, temp_storage_path):
        """Test storage manager initialization."""
        assert storage_manager.storage_path == temp_storage_path
        assert storage_manager.storage_path.exists()

    @pytest.mark.asyncio
    async def test_cleanup_old_attachments(self, storage_manager, temp_storage_path):
        """Test cleanup of old attachment files."""
        # Create some test files with different ages
        old_file = temp_storage_path / "old_file.txt"
        old_file.write_text("old content")
        old_file_stat = old_file.stat()

        new_file = temp_storage_path / "new_file.txt"
        new_file.write_text("new content")

        # Mock file modification times
        import os
        import time
        old_time = time.time() - (35 * 24 * 60 * 60)  # 35 days ago
        new_time = time.time() - (5 * 24 * 60 * 60)   # 5 days ago

        os.utime(old_file, (old_time, old_time))
        os.utime(new_file, (new_time, new_time))

        # Cleanup files older than 30 days
        deleted_count = await storage_manager.cleanup_old_attachments(30)

        assert deleted_count == 1
        assert not old_file.exists()
        assert new_file.exists()

    def test_get_storage_stats(self, storage_manager, temp_storage_path):
        """Test storage statistics generation."""
        # Create some test files
        file1 = temp_storage_path / "file1.txt"
        file1.write_text("content1")

        file2 = temp_storage_path / "file2.txt"
        file2.write_text("content2")

        stats = storage_manager.get_storage_stats()

        assert stats["total_files"] == 2
        assert stats["total_size_bytes"] > 0
        assert stats["total_size_mb"] > 0
        assert str(temp_storage_path) in stats["storage_path"]


class TestContentExtractor:
    """Test cases for ContentExtractor."""

    @pytest.fixture
    def content_extractor(self):
        """Create content extractor."""
        return ContentExtractor()

    @pytest.fixture
    def temp_file(self):
        """Create temporary file for testing."""
        temp_file = Path(tempfile.mktemp())
        yield temp_file
        if temp_file.exists():
            temp_file.unlink()

    @pytest.mark.asyncio
    async def test_extract_text_content(self, content_extractor, temp_file):
        """Test text content extraction."""
        test_content = "This is a test document with some content."
        temp_file.write_text(test_content)

        result = await content_extractor.extract_content(temp_file, "text/plain")

        assert result.success is True
        assert result.text_content == test_content
        assert result.content_type == "text/plain"
        assert result.extraction_method.startswith("text-")
        assert result.confidence == 1.0

    @pytest.mark.asyncio
    async def test_extract_unsupported_format(self, content_extractor, temp_file):
        """Test extraction from unsupported format."""
        temp_file.write_bytes(b"binary data")

        result = await content_extractor.extract_content(temp_file, "application/x-unknown")

        assert result.success is False
        assert "Unsupported MIME type" in result.error
        assert result.content_type == "application/x-unknown"

    @pytest.mark.asyncio
    async def test_extract_pdf_no_pypdf2(self, content_extractor, temp_file):
        """Test PDF extraction when PyPDF2 is not available."""
        temp_file.write_bytes(b"fake pdf content")

        # Mock PyPDF2 as not available
        with patch('penf_lib.extraction.content_extractors.PyPDF2', None):
            result = await content_extractor.extract_content(temp_file, "application/pdf")

            assert result.success is False
            assert "PyPDF2 not available" in result.error

    def test_clean_text(self, content_extractor):
        """Test text cleaning functionality."""
        dirty_text = "Text   with    excessive\n\n\n\nwhitespace\r\nand   artifacts"
        cleaned = content_extractor._clean_text(dirty_text)

        assert "   " not in cleaned  # No triple spaces
        assert "\n\n\n" not in cleaned  # No triple newlines
        assert cleaned.count('\n\n') <= dirty_text.count('\n\n')

    def test_detect_language_basic(self, content_extractor):
        """Test basic language detection."""
        english_text = "The quick brown fox jumps over the lazy dog and runs to the forest."
        spanish_text = "El perro es muy bueno y no se va de la casa con el gato."

        en_result = content_extractor._detect_language_basic(english_text)
        es_result = content_extractor._detect_language_basic(spanish_text)

        assert en_result == "en"
        assert es_result == "es"

        # Short text should return None
        short_result = content_extractor._detect_language_basic("Hi")
        assert short_result is None


class TestIntegration:
    """Integration tests for attachment processing."""

    @pytest.mark.asyncio
    async def test_full_processing_pipeline(self):
        """Test complete attachment processing pipeline."""
        # This would be a more comprehensive test that exercises
        # the full flow from attachment detection to event publishing
        # Skipping for now due to complexity of mocking entire pipeline
        pass

    @pytest.mark.asyncio
    async def test_error_recovery_scenarios(self):
        """Test various error recovery scenarios."""
        # Test scenarios like:
        # - Download failure with retry
        # - Extraction failure with fallback
        # - Event publishing failure with queue
        # Skipping for now due to complexity
        pass


if __name__ == "__main__":
    pytest.main([__file__])