"""Unit tests for content extraction."""

import tempfile
from pathlib import Path
from unittest.mock import Mock, patch, MagicMock
import pytest
from datetime import datetime

from penf_lib.extraction.content_extractors import ContentExtractor, ExtractionResult


class TestContentExtractor:
    """Test cases for ContentExtractor."""

    @pytest.fixture
    def content_extractor(self):
        """Create content extractor instance."""
        return ContentExtractor()

    @pytest.fixture
    def temp_file(self):
        """Create temporary file for testing."""
        temp_file = Path(tempfile.mktemp())
        yield temp_file
        if temp_file.exists():
            temp_file.unlink()

    @pytest.mark.asyncio
    async def test_extract_plain_text_utf8(self, content_extractor, temp_file):
        """Test extraction of UTF-8 plain text."""
        content = "This is a test document.\nIt has multiple lines.\nAnd some unicode: café, naïve, résumé."
        temp_file.write_text(content, encoding='utf-8')

        result = await content_extractor.extract_content(temp_file, "text/plain")

        assert result.success is True
        assert result.text_content == content
        assert result.content_type == "text/plain"
        assert result.extraction_method == "text-utf-8"
        assert result.confidence == 1.0
        assert result.word_count == 14
        assert result.metadata["encoding"] == "utf-8"

    @pytest.mark.asyncio
    async def test_extract_plain_text_latin1(self, content_extractor, temp_file):
        """Test extraction of Latin-1 encoded text."""
        content = "Text with special characters: café, naïve"
        temp_file.write_text(content, encoding='latin-1')

        result = await content_extractor.extract_content(temp_file, "text/plain")

        assert result.success is True
        assert content in result.text_content  # Might have encoding differences
        assert result.content_type == "text/plain"
        assert result.confidence == 1.0

    @pytest.mark.asyncio
    async def test_extract_csv_content(self, content_extractor, temp_file):
        """Test extraction of CSV content."""
        csv_content = "Name,Age,City\nJohn,25,NYC\nJane,30,LA\nBob,35,Chicago"
        temp_file.write_text(csv_content)

        result = await content_extractor.extract_content(temp_file, "text/csv")

        assert result.success is True
        assert result.text_content == csv_content
        assert result.content_type == "text/csv"
        assert result.confidence == 1.0

    @pytest.mark.asyncio
    async def test_extract_pdf_success(self, content_extractor, temp_file):
        """Test successful PDF extraction."""
        # Mock PyPDF2
        mock_reader = MagicMock()
        mock_page = MagicMock()
        mock_page.extract_text.return_value = "This is extracted PDF text."
        mock_reader.pages = [mock_page]

        temp_file.write_bytes(b"fake pdf content")

        with patch('penf_lib.extraction.content_extractors.PyPDF2') as mock_pypdf2:
            mock_pypdf2.PdfReader.return_value = mock_reader

            result = await content_extractor.extract_content(temp_file, "application/pdf")

            assert result.success is True
            assert "This is extracted PDF text." in result.text_content
            assert result.content_type == "application/pdf"
            assert result.extraction_method == "PyPDF2"
            assert result.confidence == 0.8
            assert result.metadata["page_count"] == 1

    @pytest.mark.asyncio
    async def test_extract_pdf_no_text(self, content_extractor, temp_file):
        """Test PDF extraction when no text is found."""
        mock_reader = MagicMock()
        mock_page = MagicMock()
        mock_page.extract_text.return_value = ""
        mock_reader.pages = [mock_page]

        temp_file.write_bytes(b"fake pdf content")

        with patch('penf_lib.extraction.content_extractors.PyPDF2') as mock_pypdf2:
            mock_pypdf2.PdfReader.return_value = mock_reader

            result = await content_extractor.extract_content(temp_file, "application/pdf")

            assert result.success is False
            assert "No text found in PDF" in result.error
            assert result.metadata["page_count"] == 1

    @pytest.mark.asyncio
    async def test_extract_pdf_pypdf2_unavailable(self, content_extractor, temp_file):
        """Test PDF extraction when PyPDF2 is not available."""
        temp_file.write_bytes(b"fake pdf content")

        with patch('penf_lib.extraction.content_extractors.PyPDF2', None):
            result = await content_extractor.extract_content(temp_file, "application/pdf")

            assert result.success is False
            assert "PyPDF2 not available" in result.error
            assert result.content_type == "application/pdf"

    @pytest.mark.asyncio
    async def test_extract_docx_success(self, content_extractor, temp_file):
        """Test successful DOCX extraction."""
        mock_doc = MagicMock()
        mock_paragraph1 = MagicMock()
        mock_paragraph1.text = "First paragraph content."
        mock_paragraph2 = MagicMock()
        mock_paragraph2.text = "Second paragraph content."

        mock_doc.paragraphs = [mock_paragraph1, mock_paragraph2]
        mock_doc.tables = []

        temp_file.write_bytes(b"fake docx content")

        with patch('penf_lib.extraction.content_extractors.DocxDocument') as mock_docx:
            mock_docx.return_value = mock_doc

            result = await content_extractor.extract_content(
                temp_file,
                "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            )

            assert result.success is True
            assert "First paragraph content." in result.text_content
            assert "Second paragraph content." in result.text_content
            assert result.extraction_method == "python-docx"
            assert result.confidence == 0.9
            assert result.metadata["paragraph_count"] == 2
            assert result.metadata["table_count"] == 0

    @pytest.mark.asyncio
    async def test_extract_docx_with_tables(self, content_extractor, temp_file):
        """Test DOCX extraction with tables."""
        mock_doc = MagicMock()
        mock_doc.paragraphs = []

        # Mock table
        mock_table = MagicMock()
        mock_row = MagicMock()
        mock_cell1 = MagicMock()
        mock_cell1.text = "Cell1"
        mock_cell2 = MagicMock()
        mock_cell2.text = "Cell2"
        mock_row.cells = [mock_cell1, mock_cell2]
        mock_table.rows = [mock_row]

        mock_doc.tables = [mock_table]

        temp_file.write_bytes(b"fake docx content")

        with patch('penf_lib.extraction.content_extractors.DocxDocument') as mock_docx:
            mock_docx.return_value = mock_doc

            result = await content_extractor.extract_content(
                temp_file,
                "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            )

            assert result.success is True
            assert "Cell1 | Cell2" in result.text_content
            assert "--- Tables ---" in result.text_content

    @pytest.mark.asyncio
    async def test_extract_docx_unavailable(self, content_extractor, temp_file):
        """Test DOCX extraction when python-docx is not available."""
        temp_file.write_bytes(b"fake docx content")

        with patch('penf_lib.extraction.content_extractors.DocxDocument', None):
            result = await content_extractor.extract_content(
                temp_file,
                "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            )

            assert result.success is False
            assert "python-docx not available" in result.error

    @pytest.mark.asyncio
    async def test_extract_image_ocr_success(self, content_extractor, temp_file):
        """Test successful OCR extraction from image."""
        temp_file.write_bytes(b"fake image content")

        mock_image = MagicMock()
        mock_image.mode = "RGB"
        mock_image.size = (800, 600)

        with patch('penf_lib.extraction.content_extractors.TESSERACT_AVAILABLE', True):
            with patch('penf_lib.extraction.content_extractors.Image') as mock_img_class:
                with patch('penf_lib.extraction.content_extractors.ImageOps') as mock_ops:
                    with patch('penf_lib.extraction.content_extractors.pytesseract') as mock_tess:
                        mock_img_class.open.return_value = mock_image
                        mock_ops.exif_transpose.return_value = mock_image
                        mock_ops.autocontrast.return_value = mock_image
                        mock_tess.image_to_string.return_value = "Extracted text from image"
                        mock_tess.image_to_data.return_value = {"conf": ["85", "90", "88"]}
                        mock_tess.Output = MagicMock()
                        mock_tess.Output.DICT = "dict"

                        result = await content_extractor.extract_content(temp_file, "image/jpeg")

                        assert result.success is True
                        assert result.text_content == "Extracted text from image"
                        assert result.extraction_method == "tesseract-ocr"
                        assert 0.8 <= result.confidence <= 0.9  # Average of 85, 90, 88
                        assert result.metadata["image_size"] == (800, 600)

    @pytest.mark.asyncio
    async def test_extract_image_ocr_no_text(self, content_extractor, temp_file):
        """Test OCR extraction when no text is found in image."""
        temp_file.write_bytes(b"fake image content")

        mock_image = MagicMock()
        mock_image.mode = "RGB"

        with patch('penf_lib.extraction.content_extractors.TESSERACT_AVAILABLE', True):
            with patch('penf_lib.extraction.content_extractors.Image') as mock_img_class:
                with patch('penf_lib.extraction.content_extractors.ImageOps') as mock_ops:
                    with patch('penf_lib.extraction.content_extractors.pytesseract') as mock_tess:
                        mock_img_class.open.return_value = mock_image
                        mock_ops.exif_transpose.return_value = mock_image
                        mock_ops.autocontrast.return_value = mock_image
                        mock_tess.image_to_string.return_value = ""

                        result = await content_extractor.extract_content(temp_file, "image/jpeg")

                        assert result.success is False
                        assert "No text found in image" in result.error

    @pytest.mark.asyncio
    async def test_extract_image_tesseract_unavailable(self, content_extractor, temp_file):
        """Test OCR extraction when Tesseract is not available."""
        temp_file.write_bytes(b"fake image content")

        with patch('penf_lib.extraction.content_extractors.TESSERACT_AVAILABLE', False):
            result = await content_extractor.extract_content(temp_file, "image/jpeg")

            assert result.success is False
            assert "Tesseract OCR not available" in result.error

    @pytest.mark.asyncio
    async def test_extract_doc_fallback(self, content_extractor, temp_file):
        """Test legacy DOC file handling."""
        temp_file.write_bytes(b"fake doc content")

        result = await content_extractor.extract_content(temp_file, "application/msword")

        assert result.success is False
        assert "Legacy DOC format not currently supported" in result.error
        assert result.content_type == "application/msword"

    @pytest.mark.asyncio
    async def test_unsupported_mime_type(self, content_extractor, temp_file):
        """Test handling of unsupported MIME types."""
        temp_file.write_bytes(b"unknown content")

        result = await content_extractor.extract_content(temp_file, "application/x-unknown")

        assert result.success is False
        assert "Unsupported MIME type" in result.error
        assert result.content_type == "application/x-unknown"

    def test_clean_text_functionality(self, content_extractor):
        """Test text cleaning functionality."""
        messy_text = """
        Text   with    excessive   whitespace.


        Multiple   blank   lines.



        And trailing spaces.
        Plus weird  chars: ♠♣♥♦ and symbols.
        """

        cleaned = content_extractor._clean_text(messy_text)

        # Should remove excessive whitespace
        assert "   " not in cleaned
        assert "\n\n\n" not in cleaned

        # Should preserve basic content
        assert "excessive" in cleaned
        assert "Multiple" in cleaned
        assert "trailing spaces" in cleaned

        # Should not end with spaces
        assert not cleaned.endswith(" ")

    def test_language_detection_english(self, content_extractor):
        """Test English language detection."""
        english_text = """
        The quick brown fox jumps over the lazy dog.
        This is a sample of English text with common words like the, and, or, but, in, on, at.
        It should be detected as English language.
        """

        language = content_extractor._detect_language_basic(english_text)
        assert language == "en"

    def test_language_detection_spanish(self, content_extractor):
        """Test Spanish language detection."""
        spanish_text = """
        El perro corre por el parque con su dueño.
        Este es un ejemplo de texto en español con palabras comunes como el, la, de, en, un, es, se, no.
        Debería ser detectado como idioma español.
        """

        language = content_extractor._detect_language_basic(spanish_text)
        assert language == "es"

    def test_language_detection_short_text(self, content_extractor):
        """Test language detection with short text."""
        short_text = "Hi there"

        language = content_extractor._detect_language_basic(short_text)
        assert language is None

    def test_language_detection_ambiguous(self, content_extractor):
        """Test language detection with ambiguous text."""
        ambiguous_text = "1234567890 !@#$%^&*() numbers and symbols only"

        language = content_extractor._detect_language_basic(ambiguous_text)
        # Should return None or English depending on implementation
        assert language in [None, "en"]

    @pytest.mark.asyncio
    async def test_extraction_result_word_count(self, content_extractor, temp_file):
        """Test word count calculation in extraction results."""
        test_content = "This is a test document with exactly ten words here."
        temp_file.write_text(test_content)

        result = await content_extractor.extract_content(temp_file, "text/plain")

        assert result.success is True
        assert result.word_count == 10

    @pytest.mark.asyncio
    async def test_extraction_error_handling(self, content_extractor, temp_file):
        """Test error handling during extraction."""
        # Test with non-existent file
        non_existent = Path("/non/existent/file.txt")

        result = await content_extractor.extract_content(non_existent, "text/plain")

        assert result.success is False
        assert result.error is not None
        assert result.content_type == "text/plain"

    @pytest.mark.asyncio
    async def test_binary_file_encoding_fallback(self, content_extractor, temp_file):
        """Test encoding fallback with binary content."""
        # Write binary content that might confuse text detection
        binary_content = bytes(range(256))
        temp_file.write_bytes(binary_content)

        result = await content_extractor.extract_content(temp_file, "text/plain")

        # Should fail to decode with any standard encoding
        assert result.success is False
        assert "Unable to decode text" in result.error


if __name__ == "__main__":
    pytest.main([__file__])