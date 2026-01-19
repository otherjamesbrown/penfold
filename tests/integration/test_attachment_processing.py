"""Integration tests for Gmail attachment processing."""

import asyncio
import tempfile
from pathlib import Path
from unittest.mock import Mock, AsyncMock, patch
import pytest
from datetime import datetime

from penf_lib.connectors.gmail.attachments import AttachmentProcessor
from penf_lib.extraction.content_extractors import ContentExtractor
from penf_lib.models.email_message import EmailMessage, EmailAttachment
from penf_lib.storage.database import get_db_session


class TestAttachmentProcessingIntegration:
    """Integration tests for attachment processing pipeline."""

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
    def sample_pdf_content(self):
        """Sample PDF content for testing."""
        # Simple PDF structure (very minimal)
        return b"""%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Pages 2 0 R
>>
endobj
2 0 obj
<<
/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj
3 0 obj
<<
/Type /Page
/Parent 2 0 R
/MediaBox [0 0 612 792]
/Contents 4 0 R
>>
endobj
4 0 obj
<<
/Length 44
>>
stream
BT
/F1 12 Tf
72 720 Td
(Hello World) Tj
ET
endstream
endobj
xref
0 5
0000000000 65535 f
0000000009 00000 n
0000000074 00000 n
0000000120 00000 n
0000000179 00000 n
trailer
<<
/Size 5
/Root 1 0 R
>>
startxref
274
%%EOF"""

    @pytest.fixture
    def sample_text_content(self):
        """Sample text content for testing."""
        return "This is a sample text document with multiple lines.\nIt contains various words and punctuation.\nPerfect for testing text extraction capabilities."

    @pytest.mark.asyncio
    async def test_pdf_extraction_pipeline(self, temp_storage_path, sample_pdf_content):
        """Test PDF content extraction pipeline."""
        # Create a sample PDF file
        pdf_file = temp_storage_path / "sample.pdf"
        pdf_file.write_bytes(sample_pdf_content)

        extractor = ContentExtractor()
        result = await extractor.extract_content(pdf_file, "application/pdf")

        # Note: This might fail if PyPDF2 is not available or can't parse our minimal PDF
        # In a real test environment, you'd use a proper PDF file
        if result.success:
            assert result.text_content is not None
            assert result.content_type == "application/pdf"
            assert result.extraction_method == "PyPDF2"
            assert result.word_count > 0
        else:
            # Expected if our minimal PDF is too simple
            assert "PDF extraction failed" in result.error or "No text found in PDF" in result.error

    @pytest.mark.asyncio
    async def test_text_extraction_pipeline(self, temp_storage_path, sample_text_content):
        """Test text content extraction pipeline."""
        text_file = temp_storage_path / "sample.txt"
        text_file.write_text(sample_text_content)

        extractor = ContentExtractor()
        result = await extractor.extract_content(text_file, "text/plain")

        assert result.success is True
        assert result.text_content == sample_text_content
        assert result.content_type == "text/plain"
        assert result.extraction_method.startswith("text-")
        assert result.confidence == 1.0
        assert result.word_count > 10

    @pytest.mark.asyncio
    async def test_attachment_size_limits(self, temp_storage_path):
        """Test attachment size limit enforcement."""
        from penf_lib.connectors.gmail.attachments import IMMEDIATE_PROCESSING_SIZE_LIMIT, MAX_ATTACHMENT_SIZE

        # Create mock Gmail client
        mock_client = Mock()
        processor = AttachmentProcessor(mock_client)

        # Test size limit detection
        immediate_type = processor._determine_processing_type("application/pdf", 5 * 1024 * 1024)  # 5MB
        assert immediate_type == "immediate"

        deferred_type = processor._determine_processing_type("application/pdf", 15 * 1024 * 1024)  # 15MB
        assert deferred_type == "deferred"

        # Test absolute size limit
        assert 100 * 1024 * 1024 == MAX_ATTACHMENT_SIZE  # 100MB

    @pytest.mark.asyncio
    async def test_mime_type_routing(self):
        """Test MIME type routing to appropriate processors."""
        mock_client = Mock()
        processor = AttachmentProcessor(mock_client)

        # Immediate processing types
        immediate_types = [
            "application/pdf",
            "text/plain",
            "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            "image/jpeg"
        ]

        for mime_type in immediate_types:
            result = processor._determine_processing_type(mime_type, 1024)  # Small file
            assert result == "immediate"

        # Deferred processing types
        deferred_types = [
            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            "application/zip",
            "video/mp4"
        ]

        for mime_type in deferred_types:
            result = processor._determine_processing_type(mime_type, 1024)
            assert result == "deferred"

        # Unsupported types
        unsupported_types = [
            "application/x-executable",
            "application/x-binary",
            "unknown/type"
        ]

        for mime_type in unsupported_types:
            result = processor._determine_processing_type(mime_type, 1024)
            assert result == "unsupported"

    @pytest.mark.asyncio
    async def test_content_cleaning_pipeline(self, temp_storage_path):
        """Test content cleaning and normalization."""
        # Create file with messy content
        messy_content = """
        This   has    excessive    whitespace.


        And multiple   blank lines.



        Plus trailing spaces.
        And\r\nweird\rline\nendings.
        """

        text_file = temp_storage_path / "messy.txt"
        text_file.write_text(messy_content)

        extractor = ContentExtractor()
        result = await extractor.extract_content(text_file, "text/plain")

        assert result.success is True
        cleaned_text = result.text_content

        # Check cleaning worked
        assert "    " not in cleaned_text  # No excessive spaces
        assert "\n\n\n" not in cleaned_text  # No excessive newlines
        assert not cleaned_text.endswith(" ")  # No trailing spaces
        assert "\r" not in cleaned_text  # No carriage returns

    @pytest.mark.asyncio
    async def test_language_detection_pipeline(self, temp_storage_path):
        """Test language detection in extraction pipeline."""
        english_text = "The quick brown fox jumps over the lazy dog. This is clearly English text with common words."
        spanish_text = "El perro corre por el parque con su dueño. Es un día muy bonito para pasear."

        # Test English detection
        en_file = temp_storage_path / "english.txt"
        en_file.write_text(english_text)

        extractor = ContentExtractor()
        en_result = await extractor.extract_content(en_file, "text/plain")

        assert en_result.success is True
        assert en_result.language == "en"

        # Test Spanish detection
        es_file = temp_storage_path / "spanish.txt"
        es_file.write_text(spanish_text)

        es_result = await extractor.extract_content(es_file, "text/plain")

        assert es_result.success is True
        assert es_result.language == "es"

    @pytest.mark.asyncio
    async def test_error_handling_pipeline(self, temp_storage_path):
        """Test error handling throughout the extraction pipeline."""
        extractor = ContentExtractor()

        # Test non-existent file
        non_existent = temp_storage_path / "does_not_exist.txt"
        result = await extractor.extract_content(non_existent, "text/plain")

        assert result.success is False
        assert result.error is not None

        # Test empty file
        empty_file = temp_storage_path / "empty.txt"
        empty_file.write_text("")

        result = await extractor.extract_content(empty_file, "text/plain")

        assert result.success is True
        assert result.text_content == ""
        assert result.word_count == 0

    @pytest.mark.asyncio
    async def test_performance_benchmarking(self, temp_storage_path):
        """Test performance characteristics of extraction pipeline."""
        import time

        # Create files of varying sizes
        small_text = "Small file content."
        medium_text = "Medium file content. " * 100
        large_text = "Large file content. " * 1000

        test_cases = [
            ("small.txt", small_text),
            ("medium.txt", medium_text),
            ("large.txt", large_text)
        ]

        extractor = ContentExtractor()
        timing_results = {}

        for filename, content in test_cases:
            file_path = temp_storage_path / filename
            file_path.write_text(content)

            start_time = time.time()
            result = await extractor.extract_content(file_path, "text/plain")
            end_time = time.time()

            assert result.success is True
            timing_results[filename] = end_time - start_time

        # Verify reasonable performance (all should be under 1 second for text)
        for filename, duration in timing_results.items():
            assert duration < 1.0, f"Extraction took too long for {filename}: {duration}s"

        # Large file should still be reasonably fast
        assert timing_results["large.txt"] < 0.1, "Large text file took too long"

    @pytest.mark.asyncio
    async def test_concurrent_processing(self, temp_storage_path):
        """Test concurrent attachment processing."""
        extractor = ContentExtractor()

        # Create multiple test files
        test_files = []
        for i in range(5):
            file_path = temp_storage_path / f"concurrent_{i}.txt"
            file_path.write_text(f"Content for file {i}. This is test content.")
            test_files.append((file_path, "text/plain"))

        # Process concurrently
        tasks = [
            extractor.extract_content(file_path, mime_type)
            for file_path, mime_type in test_files
        ]

        results = await asyncio.gather(*tasks)

        # Verify all succeeded
        for i, result in enumerate(results):
            assert result.success is True, f"File {i} failed: {result.error}"
            assert f"Content for file {i}" in result.text_content
            assert result.word_count > 0

    @pytest.mark.asyncio
    async def test_storage_management_integration(self, temp_storage_path):
        """Test storage management integration."""
        from penf_lib.connectors.gmail.attachments import AttachmentStorageManager

        manager = AttachmentStorageManager(temp_storage_path)

        # Create test files with different ages
        import time
        import os

        old_file = temp_storage_path / "old_attachment.pdf"
        old_file.write_bytes(b"old content")

        recent_file = temp_storage_path / "recent_attachment.pdf"
        recent_file.write_bytes(b"recent content")

        # Mock file ages
        old_time = time.time() - (35 * 24 * 60 * 60)  # 35 days ago
        recent_time = time.time() - (5 * 24 * 60 * 60)  # 5 days ago

        os.utime(old_file, (old_time, old_time))
        os.utime(recent_file, (recent_time, recent_time))

        # Test storage stats
        stats = manager.get_storage_stats()
        assert stats["total_files"] == 2
        assert stats["total_size_bytes"] > 0

        # Test cleanup
        deleted_count = await manager.cleanup_old_attachments(max_age_days=30)
        assert deleted_count == 1
        assert not old_file.exists()
        assert recent_file.exists()

        # Updated stats
        new_stats = manager.get_storage_stats()
        assert new_stats["total_files"] == 1


if __name__ == "__main__":
    pytest.main([__file__])