// Package attachment provides text extraction from various attachment formats.
package attachment

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// TextExtractor is the interface for extracting text from attachment content.
type TextExtractor interface {
	// Extract extracts text content from the given bytes.
	Extract(content []byte) (string, error)

	// SupportedMimeTypes returns the MIME types this extractor supports.
	SupportedMimeTypes() []string
}

// ExtractionResult contains the result of text extraction.
type ExtractionResult struct {
	// Text is the extracted text content.
	Text string

	// Metadata contains additional extracted information.
	Metadata map[string]string

	// Error is set if extraction partially failed.
	Error error
}

// PlainTextExtractor extracts text from plain text formats.
type PlainTextExtractor struct {
	// MaxSize is the maximum content size to process.
	MaxSize int

	// StripHTML indicates whether to strip HTML tags.
	StripHTML bool
}

// NewPlainTextExtractor creates a new PlainTextExtractor with defaults.
func NewPlainTextExtractor() *PlainTextExtractor {
	return &PlainTextExtractor{
		MaxSize:   10 * 1024 * 1024, // 10 MB
		StripHTML: true,
	}
}

// Extract extracts text from plain text content.
func (e *PlainTextExtractor) Extract(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	// Use default max size if not set.
	maxSize := e.MaxSize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10 MB default
	}

	if len(content) > maxSize {
		content = content[:maxSize]
	}

	text := string(content)

	if e.StripHTML {
		text = stripHTMLTags(text)
	}

	// Normalize whitespace.
	text = normalizeWhitespace(text)

	return text, nil
}

// SupportedMimeTypes returns the MIME types supported by PlainTextExtractor.
func (e *PlainTextExtractor) SupportedMimeTypes() []string {
	return []string{
		"text/plain",
		"text/html",
		"text/csv",
		"text/xml",
		"application/json",
		"application/xml",
	}
}

// stripHTMLTags removes HTML tags from text.
func stripHTMLTags(text string) string {
	// Remove script and style blocks.
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	text = reScript.ReplaceAllString(text, "")

	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	text = reStyle.ReplaceAllString(text, "")

	// Remove HTML tags.
	reTags := regexp.MustCompile(`<[^>]*>`)
	text = reTags.ReplaceAllString(text, " ")

	// Decode common HTML entities.
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	return text
}

// normalizeWhitespace normalizes whitespace in text.
func normalizeWhitespace(text string) string {
	// Replace multiple spaces/tabs with single space.
	reSpaces := regexp.MustCompile(`[ \t]+`)
	text = reSpaces.ReplaceAllString(text, " ")

	// Replace multiple newlines with double newline.
	reNewlines := regexp.MustCompile(`\n{3,}`)
	text = reNewlines.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

// PDFExtractor extracts text from PDF documents.
// This is a basic implementation that extracts raw text streams.
// For production use, consider using a library like pdfcpu or unipdf.
type PDFExtractor struct {
	// MaxSize is the maximum PDF size to process.
	MaxSize int
}

// NewPDFExtractor creates a new PDFExtractor with defaults.
func NewPDFExtractor() *PDFExtractor {
	return &PDFExtractor{
		MaxSize: 25 * 1024 * 1024, // 25 MB
	}
}

// Extract extracts text from PDF content.
// This is a simplified implementation that extracts text from common PDF text streams.
// For full PDF support, a dedicated PDF library should be used.
func (e *PDFExtractor) Extract(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	if len(content) > e.MaxSize {
		return "", fmt.Errorf("pdf exceeds maximum size: %d > %d", len(content), e.MaxSize)
	}

	// Verify PDF header.
	if !bytes.HasPrefix(content, []byte("%PDF")) {
		return "", fmt.Errorf("invalid PDF: missing PDF header")
	}

	// Extract text from PDF streams.
	// This is a basic implementation - production should use a proper PDF library.
	var textBuilder strings.Builder

	// Look for text in BT...ET blocks (text objects).
	btPattern := regexp.MustCompile(`BT\s*(.*?)\s*ET`)
	matches := btPattern.FindAllSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			text := extractPDFTextOperators(match[1])
			if text != "" {
				textBuilder.WriteString(text)
				textBuilder.WriteString(" ")
			}
		}
	}

	// Also look for text in streams between "stream" and "endstream".
	streamPattern := regexp.MustCompile(`(?s)stream\s*(.*?)\s*endstream`)
	streamMatches := streamPattern.FindAllSubmatch(content, -1)

	for _, match := range streamMatches {
		if len(match) > 1 {
			// Try to extract readable text from stream.
			streamText := extractReadableText(match[1])
			if streamText != "" {
				textBuilder.WriteString(streamText)
				textBuilder.WriteString(" ")
			}
		}
	}

	result := normalizeWhitespace(textBuilder.String())

	if result == "" {
		// If no text found, the PDF might use embedded fonts or images.
		return "", fmt.Errorf("no extractable text found in PDF (may require OCR)")
	}

	return result, nil
}

// extractPDFTextOperators extracts text from PDF text operators.
func extractPDFTextOperators(data []byte) string {
	var textBuilder strings.Builder

	// Match text in parentheses (Tj operator) or brackets (TJ operator).
	tjPattern := regexp.MustCompile(`\((.*?)\)`)
	matches := tjPattern.FindAllSubmatch(data, -1)

	for _, match := range matches {
		if len(match) > 1 {
			text := decodePDFString(match[1])
			textBuilder.WriteString(text)
		}
	}

	return textBuilder.String()
}

// decodePDFString decodes a PDF string, handling escape sequences.
func decodePDFString(data []byte) string {
	result := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] == '\\' && i+1 < len(data) {
			switch data[i+1] {
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			case '(':
				result = append(result, '(')
			case ')':
				result = append(result, ')')
			case '\\':
				result = append(result, '\\')
			default:
				result = append(result, data[i+1])
			}
			i += 2
		} else {
			result = append(result, data[i])
			i++
		}
	}

	return string(result)
}

// extractReadableText extracts readable ASCII text from binary data.
func extractReadableText(data []byte) string {
	var textBuilder strings.Builder
	var currentWord strings.Builder

	for _, b := range data {
		// Keep printable ASCII characters.
		if b >= 32 && b <= 126 {
			currentWord.WriteByte(b)
		} else if currentWord.Len() > 0 {
			// Non-printable character - end of word.
			word := currentWord.String()
			if len(word) >= 2 { // Only keep words with 2+ characters.
				textBuilder.WriteString(word)
				textBuilder.WriteString(" ")
			}
			currentWord.Reset()
		}
	}

	// Don't forget the last word.
	if currentWord.Len() >= 2 {
		textBuilder.WriteString(currentWord.String())
	}

	return textBuilder.String()
}

// SupportedMimeTypes returns the MIME types supported by PDFExtractor.
func (e *PDFExtractor) SupportedMimeTypes() []string {
	return []string{
		"application/pdf",
	}
}

// DOCXExtractor extracts text from DOCX (Office Open XML) documents.
type DOCXExtractor struct {
	// MaxSize is the maximum DOCX size to process.
	MaxSize int
}

// NewDOCXExtractor creates a new DOCXExtractor with defaults.
func NewDOCXExtractor() *DOCXExtractor {
	return &DOCXExtractor{
		MaxSize: 25 * 1024 * 1024, // 25 MB
	}
}

// Extract extracts text from DOCX content.
func (e *DOCXExtractor) Extract(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	if len(content) > e.MaxSize {
		return "", fmt.Errorf("docx exceeds maximum size: %d > %d", len(content), e.MaxSize)
	}

	// DOCX is a ZIP archive.
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("opening docx as zip: %w", err)
	}

	var textBuilder strings.Builder

	// Extract text from document.xml (main content).
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			text, err := extractDOCXText(file)
			if err != nil {
				return "", fmt.Errorf("extracting document.xml: %w", err)
			}
			textBuilder.WriteString(text)
		}
	}

	return normalizeWhitespace(textBuilder.String()), nil
}

// extractDOCXText extracts text from a DOCX document.xml file.
func extractDOCXText(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	return extractTextFromXML(content, "t") // <w:t> elements contain text.
}

// extractTextFromXML extracts text content from XML elements with the given local name.
func extractTextFromXML(content []byte, localName string) (string, error) {
	var textBuilder strings.Builder
	decoder := xml.NewDecoder(bytes.NewReader(content))

	inTargetElement := false

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return textBuilder.String(), nil // Return what we have on error.
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == localName {
				inTargetElement = true
			}
			// Add space for paragraph elements.
			if t.Name.Local == "p" {
				textBuilder.WriteString("\n")
			}
		case xml.EndElement:
			if t.Name.Local == localName {
				inTargetElement = false
				textBuilder.WriteString(" ")
			}
		case xml.CharData:
			if inTargetElement {
				textBuilder.Write(t)
			}
		}
	}

	return textBuilder.String(), nil
}

// SupportedMimeTypes returns the MIME types supported by DOCXExtractor.
func (e *DOCXExtractor) SupportedMimeTypes() []string {
	return []string{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/msword",
	}
}

// ImageExtractor is a placeholder for OCR-based image text extraction.
// Currently returns metadata about the image without actual text extraction.
type ImageExtractor struct {
	// EnableOCR indicates whether OCR should be attempted.
	// This is a placeholder - actual OCR would require integration with
	// an OCR service (e.g., Google Cloud Vision, AWS Textract, Tesseract).
	EnableOCR bool
}

// NewImageExtractor creates a new ImageExtractor.
func NewImageExtractor() *ImageExtractor {
	return &ImageExtractor{
		EnableOCR: false, // OCR disabled by default.
	}
}

// Extract extracts text from images.
// Currently returns a placeholder since OCR requires external services.
func (e *ImageExtractor) Extract(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	if !e.EnableOCR {
		// Return metadata only when OCR is disabled.
		imageType := detectImageType(content)
		return fmt.Sprintf("[Image: %s, size: %d bytes]", imageType, len(content)), nil
	}

	// TODO: Implement OCR integration.
	// Options:
	// - Google Cloud Vision API
	// - AWS Textract
	// - Azure Computer Vision
	// - Tesseract (local)
	return "", fmt.Errorf("OCR not implemented: requires external service integration")
}

// detectImageType detects the image type from content bytes.
func detectImageType(content []byte) string {
	if len(content) < 8 {
		return "unknown"
	}

	// Check magic bytes.
	switch {
	case bytes.HasPrefix(content, []byte{0xFF, 0xD8, 0xFF}):
		return "JPEG"
	case bytes.HasPrefix(content, []byte{0x89, 0x50, 0x4E, 0x47}):
		return "PNG"
	case bytes.HasPrefix(content, []byte{0x47, 0x49, 0x46}):
		return "GIF"
	case bytes.HasPrefix(content, []byte{0x52, 0x49, 0x46, 0x46}) && len(content) > 12 &&
		bytes.Equal(content[8:12], []byte{0x57, 0x45, 0x42, 0x50}):
		return "WebP"
	case bytes.HasPrefix(content, []byte{0x49, 0x49, 0x2A, 0x00}) ||
		bytes.HasPrefix(content, []byte{0x4D, 0x4D, 0x00, 0x2A}):
		return "TIFF"
	default:
		return "unknown"
	}
}

// SupportedMimeTypes returns the MIME types supported by ImageExtractor.
func (e *ImageExtractor) SupportedMimeTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/tiff",
	}
}

// SpreadsheetExtractor extracts text from spreadsheet formats.
// Currently supports XLSX (Office Open XML Spreadsheet).
type SpreadsheetExtractor struct {
	// MaxSize is the maximum file size to process.
	MaxSize int

	// MaxRows limits the number of rows to extract.
	MaxRows int
}

// NewSpreadsheetExtractor creates a new SpreadsheetExtractor with defaults.
func NewSpreadsheetExtractor() *SpreadsheetExtractor {
	return &SpreadsheetExtractor{
		MaxSize: 25 * 1024 * 1024, // 25 MB
		MaxRows: 10000,            // Max 10k rows
	}
}

// Extract extracts text from spreadsheet content.
func (e *SpreadsheetExtractor) Extract(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	if len(content) > e.MaxSize {
		return "", fmt.Errorf("spreadsheet exceeds maximum size: %d > %d", len(content), e.MaxSize)
	}

	// XLSX is a ZIP archive.
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("opening xlsx as zip: %w", err)
	}

	// First, read shared strings.
	var sharedStrings []string
	for _, file := range reader.File {
		if file.Name == "xl/sharedStrings.xml" {
			sharedStrings, err = extractSharedStrings(file)
			if err != nil {
				// Continue without shared strings.
				sharedStrings = nil
			}
			break
		}
	}

	var textBuilder strings.Builder

	// Extract text from each sheet.
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "xl/worksheets/sheet") && strings.HasSuffix(file.Name, ".xml") {
			text, err := extractSheetText(file, sharedStrings, e.MaxRows)
			if err != nil {
				continue
			}
			textBuilder.WriteString(text)
			textBuilder.WriteString("\n")
		}
	}

	return normalizeWhitespace(textBuilder.String()), nil
}

// extractSharedStrings extracts the shared string table from XLSX.
func extractSharedStrings(file *zip.File) ([]string, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var sharedStrings []string
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var currentText strings.Builder
	inT := false

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inT = true
				currentText.Reset()
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inT = false
			}
			if t.Name.Local == "si" {
				sharedStrings = append(sharedStrings, currentText.String())
				currentText.Reset()
			}
		case xml.CharData:
			if inT {
				currentText.Write(t)
			}
		}
	}

	return sharedStrings, nil
}

// extractSheetText extracts text from an XLSX worksheet.
func extractSheetText(file *zip.File, sharedStrings []string, maxRows int) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	var textBuilder strings.Builder
	decoder := xml.NewDecoder(bytes.NewReader(content))

	var cellValue strings.Builder
	var cellType string
	inV := false
	rowCount := 0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "row" {
				rowCount++
				if rowCount > maxRows {
					return textBuilder.String(), nil
				}
			}
			if t.Name.Local == "c" {
				// Get cell type.
				cellType = ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "t" {
						cellType = attr.Value
						break
					}
				}
			}
			if t.Name.Local == "v" {
				inV = true
				cellValue.Reset()
			}
		case xml.EndElement:
			if t.Name.Local == "v" {
				inV = false
				value := cellValue.String()

				// If type is "s", this is a shared string reference.
				if cellType == "s" && sharedStrings != nil {
					idx := 0
					fmt.Sscanf(value, "%d", &idx)
					if idx >= 0 && idx < len(sharedStrings) {
						value = sharedStrings[idx]
					}
				}

				textBuilder.WriteString(value)
				textBuilder.WriteString(" ")
			}
			if t.Name.Local == "row" {
				textBuilder.WriteString("\n")
			}
		case xml.CharData:
			if inV {
				cellValue.Write(t)
			}
		}
	}

	return textBuilder.String(), nil
}

// SupportedMimeTypes returns the MIME types supported by SpreadsheetExtractor.
func (e *SpreadsheetExtractor) SupportedMimeTypes() []string {
	return []string{
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel",
	}
}
