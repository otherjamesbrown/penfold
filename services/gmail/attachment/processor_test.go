package attachment

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"strings"
	"testing"
	"time"
)

func TestNewAttachmentProcessor(t *testing.T) {
	tests := []struct {
		name    string
		config  *ProcessorConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid config",
			config: &ProcessorConfig{
				MaxAttachmentSize: 10 * 1024 * 1024,
				ProcessTimeout:    3 * time.Minute,
				ConcurrentLimit:   5,
			},
			wantErr: false,
		},
		{
			name: "zero values get defaults",
			config: &ProcessorConfig{
				MaxAttachmentSize: 0,
				ProcessTimeout:    0,
				ConcurrentLimit:   0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor, err := NewAttachmentProcessor(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAttachmentProcessor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && processor == nil {
				t.Error("NewAttachmentProcessor() returned nil processor")
			}
		})
	}
}

func TestDefaultProcessorConfig(t *testing.T) {
	cfg := DefaultProcessorConfig()

	if cfg.MaxAttachmentSize != DefaultMaxAttachmentSize {
		t.Errorf("MaxAttachmentSize = %d, want %d", cfg.MaxAttachmentSize, DefaultMaxAttachmentSize)
	}

	if cfg.ProcessTimeout != DefaultProcessTimeout {
		t.Errorf("ProcessTimeout = %v, want %v", cfg.ProcessTimeout, DefaultProcessTimeout)
	}

	if cfg.ConcurrentLimit != DefaultConcurrentLimit {
		t.Errorf("ConcurrentLimit = %d, want %d", cfg.ConcurrentLimit, DefaultConcurrentLimit)
	}

	if cfg.ExtractorRegistry == nil {
		t.Error("ExtractorRegistry is nil")
	}
}

func TestClassifyAttachment(t *testing.T) {
	tests := []struct {
		mimeType string
		filename string
		want     AttachmentClassification
	}{
		{"application/pdf", "document.pdf", ClassificationPDF},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "doc.docx", ClassificationDocument},
		{"application/msword", "doc.doc", ClassificationDocument},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "sheet.xlsx", ClassificationSpreadsheet},
		{"application/vnd.ms-excel", "sheet.xls", ClassificationSpreadsheet},
		{"text/csv", "data.csv", ClassificationSpreadsheet},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", "slides.pptx", ClassificationPresentation},
		{"application/vnd.ms-powerpoint", "slides.ppt", ClassificationPresentation},
		{"image/jpeg", "photo.jpg", ClassificationImage},
		{"image/png", "image.png", ClassificationImage},
		{"image/gif", "animation.gif", ClassificationImage},
		{"audio/mpeg", "song.mp3", ClassificationAudio},
		{"video/mp4", "video.mp4", ClassificationVideo},
		{"text/plain", "readme.txt", ClassificationText},
		{"text/html", "page.html", ClassificationText},
		{"application/zip", "archive.zip", ClassificationArchive},
		{"application/gzip", "archive.tar.gz", ClassificationArchive},
		{"application/json", "config.json", ClassificationCode},
		{"application/xml", "data.xml", ClassificationCode},
		{"application/octet-stream", "unknown.bin", ClassificationUnknown},
		{"", "unknown", ClassificationUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := ClassifyAttachment(tt.mimeType, tt.filename)
			if got != tt.want {
				t.Errorf("ClassifyAttachment(%q, %q) = %v, want %v", tt.mimeType, tt.filename, got, tt.want)
			}
		})
	}
}

func TestProcessorProcessAttachment(t *testing.T) {
	// Create a processor with a mock downloader.
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	mockDownloader := &MockDownloader{
		Content: map[string][]byte{
			"msg1:att1": []byte("Hello, this is a test document."),
		},
	}
	processor.SetDownloader(mockDownloader)

	ctx := context.Background()
	attachment := &Attachment{
		ID:        "att1",
		MessageID: "msg1",
		Filename:  "test.txt",
		MimeType:  "text/plain",
		Size:      100,
	}

	result, err := processor.ProcessAttachment(ctx, attachment)
	if err != nil {
		t.Fatalf("ProcessAttachment() error = %v", err)
	}

	if result.Attachment != attachment {
		t.Error("ProcessAttachment() did not preserve attachment reference")
	}

	if result.Classification != ClassificationText {
		t.Errorf("Classification = %v, want %v", result.Classification, ClassificationText)
	}

	if result.ExtractedText == "" {
		t.Error("ExtractedText is empty")
	}

	if !strings.Contains(result.ExtractedText, "Hello") {
		t.Errorf("ExtractedText = %q, expected to contain 'Hello'", result.ExtractedText)
	}

	if result.ProcessedAt.IsZero() {
		t.Error("ProcessedAt is zero")
	}

	if result.ProcessingDuration == 0 {
		t.Error("ProcessingDuration is zero")
	}

	if result.Attachment.ContentHash == "" {
		t.Error("ContentHash is empty")
	}
}

func TestProcessorProcessAttachment_TooLarge(t *testing.T) {
	processor, err := NewAttachmentProcessor(&ProcessorConfig{
		MaxAttachmentSize: 100, // 100 bytes max
	})
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	ctx := context.Background()
	attachment := &Attachment{
		ID:        "att1",
		MessageID: "msg1",
		Filename:  "large.txt",
		MimeType:  "text/plain",
		Size:      1000, // Exceeds limit
	}

	_, err = processor.ProcessAttachment(ctx, attachment)
	if err != ErrAttachmentTooLarge {
		t.Errorf("ProcessAttachment() error = %v, want %v", err, ErrAttachmentTooLarge)
	}
}

func TestProcessorProcessAttachment_NoDownloader(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	// Don't set a downloader.

	ctx := context.Background()
	attachment := &Attachment{
		ID:        "att1",
		MessageID: "msg1",
		Filename:  "test.txt",
		MimeType:  "text/plain",
		Size:      100,
	}

	_, err = processor.ProcessAttachment(ctx, attachment)
	if err == nil {
		t.Error("ProcessAttachment() expected error with no downloader")
	}
}

func TestProcessorProcessAttachment_DownloadError(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	mockDownloader := &MockDownloader{
		Err: fmt.Errorf("network error"),
	}
	processor.SetDownloader(mockDownloader)

	ctx := context.Background()
	attachment := &Attachment{
		ID:        "att1",
		MessageID: "msg1",
		Filename:  "test.txt",
		MimeType:  "text/plain",
		Size:      100,
	}

	_, err = processor.ProcessAttachment(ctx, attachment)
	if err == nil {
		t.Error("ProcessAttachment() expected error on download failure")
	}
}

func TestProcessorProcessAttachment_ContextCancellation(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	attachment := &Attachment{
		ID:        "att1",
		MessageID: "msg1",
		Filename:  "test.txt",
		MimeType:  "text/plain",
		Size:      100,
	}

	_, err = processor.ProcessAttachment(ctx, attachment)
	if err == nil {
		t.Error("ProcessAttachment() expected error on context cancellation")
	}
}

func TestProcessorExtractText_UnsupportedMimeType(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	_, err = processor.ExtractText([]byte("content"), "application/octet-stream")
	if err == nil {
		t.Error("ExtractText() expected error for unsupported MIME type")
	}
}

func TestProcessorRegisterExtractor(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	// Register a custom extractor with HTML stripping disabled and proper MaxSize.
	customExtractor := &PlainTextExtractor{
		MaxSize:   10 * 1024 * 1024, // 10 MB
		StripHTML: false,
	}
	processor.RegisterExtractor("application/custom", customExtractor)

	// Verify it's registered.
	text, err := processor.ExtractText([]byte("custom content"), "application/custom")
	if err != nil {
		t.Errorf("ExtractText() error = %v after registering extractor", err)
	}
	if text != "custom content" {
		t.Errorf("ExtractText() = %q, want %q", text, "custom content")
	}
}

func TestProcessorProcessAttachments(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	mockDownloader := &MockDownloader{
		Content: map[string][]byte{
			"msg1:att1": []byte("Content 1"),
			"msg1:att2": []byte("Content 2"),
			"msg1:att3": []byte("Content 3"),
		},
	}
	processor.SetDownloader(mockDownloader)

	ctx := context.Background()
	attachments := []*Attachment{
		{ID: "att1", MessageID: "msg1", Filename: "file1.txt", MimeType: "text/plain", Size: 100},
		{ID: "att2", MessageID: "msg1", Filename: "file2.txt", MimeType: "text/plain", Size: 100},
		{ID: "att3", MessageID: "msg1", Filename: "file3.txt", MimeType: "text/plain", Size: 100},
	}

	results, err := processor.ProcessAttachments(ctx, attachments)
	if err != nil {
		t.Fatalf("ProcessAttachments() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("ProcessAttachments() returned %d results, want 3", len(results))
	}

	for i, result := range results {
		if result == nil {
			t.Errorf("results[%d] is nil", i)
			continue
		}
		if result.ExtractedText == "" {
			t.Errorf("results[%d].ExtractedText is empty", i)
		}
	}
}

func TestProcessorProcessAttachmentAsync(t *testing.T) {
	processor, err := NewAttachmentProcessor(nil)
	if err != nil {
		t.Fatalf("NewAttachmentProcessor() error = %v", err)
	}

	mockDownloader := &MockDownloader{
		Content: map[string][]byte{
			"msg1:att1": []byte("Async content"),
		},
	}
	processor.SetDownloader(mockDownloader)

	ctx := context.Background()
	attachment := &Attachment{
		ID:        "att1",
		MessageID: "msg1",
		Filename:  "async.txt",
		MimeType:  "text/plain",
		Size:      100,
	}

	resultChan := processor.ProcessAttachmentAsync(ctx, attachment)

	select {
	case result := <-resultChan:
		if result == nil {
			t.Error("ProcessAttachmentAsync() returned nil result")
		}
		if result.ExtractedText == "" {
			t.Error("ProcessAttachmentAsync() result has empty ExtractedText")
		}
	case <-time.After(5 * time.Second):
		t.Error("ProcessAttachmentAsync() timed out")
	}
}

func TestMockDownloader(t *testing.T) {
	downloader := &MockDownloader{
		Content: map[string][]byte{
			"msg1:att1": []byte("test content"),
		},
	}

	ctx := context.Background()

	// Test successful download.
	content, err := downloader.Download(ctx, "msg1", "att1")
	if err != nil {
		t.Errorf("Download() error = %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Download() content = %q, want %q", string(content), "test content")
	}

	// Test not found.
	_, err = downloader.Download(ctx, "msg1", "att999")
	if err == nil {
		t.Error("Download() expected error for non-existent attachment")
	}

	// Test error mode.
	downloader.Err = fmt.Errorf("forced error")
	_, err = downloader.Download(ctx, "msg1", "att1")
	if err == nil {
		t.Error("Download() expected error when Err is set")
	}
}

func TestProcessorMetrics(t *testing.T) {
	metrics := NewProcessorMetrics("test", "test_service")

	// Verify metrics were created.
	if metrics.AttachmentsProcessed == nil {
		t.Error("AttachmentsProcessed metric is nil")
	}
	if metrics.AttachmentsFailed == nil {
		t.Error("AttachmentsFailed metric is nil")
	}
	if metrics.AttachmentsSkipped == nil {
		t.Error("AttachmentsSkipped metric is nil")
	}
	if metrics.ProcessingDuration == nil {
		t.Error("ProcessingDuration metric is nil")
	}
	if metrics.BytesProcessed == nil {
		t.Error("BytesProcessed metric is nil")
	}
	if metrics.TextBytesExtracted == nil {
		t.Error("TextBytesExtracted metric is nil")
	}
	if metrics.ClassificationCounts == nil {
		t.Error("ClassificationCounts metric is nil")
	}
}

// Test PlainTextExtractor.
func TestPlainTextExtractor(t *testing.T) {
	extractor := NewPlainTextExtractor()

	tests := []struct {
		name    string
		content []byte
		want    string
		wantErr bool
	}{
		{
			name:    "plain text",
			content: []byte("Hello, World!"),
			want:    "Hello, World!",
			wantErr: false,
		},
		{
			name:    "empty content",
			content: []byte{},
			want:    "",
			wantErr: false,
		},
		{
			name:    "html content",
			content: []byte("<html><body><p>Hello</p></body></html>"),
			want:    "Hello",
			wantErr: false,
		},
		{
			name:    "html with script",
			content: []byte("<html><script>alert('bad');</script><body>Safe text</body></html>"),
			want:    "Safe text",
			wantErr: false,
		},
		{
			name:    "whitespace normalization",
			content: []byte("Hello    World\n\n\n\nGoodbye"),
			want:    "Hello World\n\nGoodbye",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractor.Extract(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Extract() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlainTextExtractor_SupportedMimeTypes(t *testing.T) {
	extractor := NewPlainTextExtractor()
	mimeTypes := extractor.SupportedMimeTypes()

	expected := []string{"text/plain", "text/html", "text/csv", "text/xml", "application/json", "application/xml"}

	if len(mimeTypes) != len(expected) {
		t.Errorf("SupportedMimeTypes() returned %d types, want %d", len(mimeTypes), len(expected))
	}

	for _, exp := range expected {
		found := false
		for _, got := range mimeTypes {
			if got == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SupportedMimeTypes() missing %q", exp)
		}
	}
}

// Test PDFExtractor.
func TestPDFExtractor(t *testing.T) {
	extractor := NewPDFExtractor()

	tests := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{
			name:    "empty content",
			content: []byte{},
			wantErr: false,
		},
		{
			name:    "invalid PDF",
			content: []byte("not a pdf"),
			wantErr: true,
		},
		{
			name:    "minimal PDF header",
			content: []byte("%PDF-1.4\n"),
			wantErr: true, // No extractable text
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractor.Extract(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPDFExtractor_SupportedMimeTypes(t *testing.T) {
	extractor := NewPDFExtractor()
	mimeTypes := extractor.SupportedMimeTypes()

	if len(mimeTypes) != 1 || mimeTypes[0] != "application/pdf" {
		t.Errorf("SupportedMimeTypes() = %v, want [application/pdf]", mimeTypes)
	}
}

// Test DOCXExtractor.
func TestDOCXExtractor(t *testing.T) {
	extractor := NewDOCXExtractor()

	// Create a minimal valid DOCX file.
	validDocx := createTestDOCX(t, "Hello from DOCX")

	tests := []struct {
		name     string
		content  []byte
		wantText string
		wantErr  bool
	}{
		{
			name:    "empty content",
			content: []byte{},
			wantErr: false,
		},
		{
			name:    "invalid zip",
			content: []byte("not a zip file"),
			wantErr: true,
		},
		{
			name:     "valid docx",
			content:  validDocx,
			wantText: "Hello from DOCX",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractor.Extract(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.wantText != "" && !strings.Contains(got, tt.wantText) {
				t.Errorf("Extract() = %q, want to contain %q", got, tt.wantText)
			}
		})
	}
}

// createTestDOCX creates a minimal DOCX file for testing.
func createTestDOCX(t *testing.T, text string) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Create [Content_Types].xml
	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="xml" ContentType="application/xml"/>
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

	f, _ := w.Create("[Content_Types].xml")
	f.Write([]byte(contentTypes))

	// Create word/document.xml with our text.
	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body><w:p><w:r><w:t>%s</w:t></w:r></w:p></w:body>
</w:document>`, html.EscapeString(text))

	f, _ = w.Create("word/document.xml")
	f.Write([]byte(docXML))

	w.Close()
	return buf.Bytes()
}

func TestDOCXExtractor_SupportedMimeTypes(t *testing.T) {
	extractor := NewDOCXExtractor()
	mimeTypes := extractor.SupportedMimeTypes()

	expected := []string{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/msword",
	}

	if len(mimeTypes) != len(expected) {
		t.Errorf("SupportedMimeTypes() returned %d types, want %d", len(mimeTypes), len(expected))
	}
}

// Test ImageExtractor.
func TestImageExtractor(t *testing.T) {
	extractor := NewImageExtractor()

	// JPEG magic bytes.
	jpegContent := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	pngContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	tests := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{
			name:    "empty content",
			content: []byte{},
			wantErr: false,
		},
		{
			name:    "jpeg image",
			content: jpegContent,
			wantErr: false, // Returns metadata, no error
		},
		{
			name:    "png image",
			content: pngContent,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractor.Extract(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(tt.content) > 0 && got == "" {
				t.Error("Extract() returned empty string for non-empty image")
			}
		})
	}
}

func TestImageExtractor_OCRDisabled(t *testing.T) {
	extractor := NewImageExtractor()
	extractor.EnableOCR = false

	content := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG header.

	got, err := extractor.Extract(content)
	if err != nil {
		t.Errorf("Extract() error = %v", err)
	}

	// Should return metadata, not actual text.
	if !strings.Contains(got, "[Image:") {
		t.Errorf("Extract() = %q, want metadata placeholder", got)
	}
}

func TestImageExtractor_SupportedMimeTypes(t *testing.T) {
	extractor := NewImageExtractor()
	mimeTypes := extractor.SupportedMimeTypes()

	expected := []string{"image/jpeg", "image/png", "image/gif", "image/webp", "image/tiff"}

	if len(mimeTypes) != len(expected) {
		t.Errorf("SupportedMimeTypes() returned %d types, want %d", len(mimeTypes), len(expected))
	}
}

func TestDetectImageType(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{
			name:    "jpeg",
			content: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46}, // 8 bytes
			want:    "JPEG",
		},
		{
			name:    "png",
			content: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			want:    "PNG",
		},
		{
			name:    "gif",
			content: []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x00, 0x00}, // 8 bytes
			want:    "GIF",
		},
		{
			name:    "tiff little endian",
			content: []byte{0x49, 0x49, 0x2A, 0x00, 0x00, 0x00, 0x00, 0x00}, // 8 bytes
			want:    "TIFF",
		},
		{
			name:    "tiff big endian",
			content: []byte{0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x00}, // 8 bytes
			want:    "TIFF",
		},
		{
			name:    "unknown",
			content: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, // 8 bytes
			want:    "unknown",
		},
		{
			name:    "too short",
			content: []byte{0x00},
			want:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectImageType(tt.content)
			if got != tt.want {
				t.Errorf("detectImageType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test SpreadsheetExtractor.
func TestSpreadsheetExtractor(t *testing.T) {
	extractor := NewSpreadsheetExtractor()

	tests := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{
			name:    "empty content",
			content: []byte{},
			wantErr: false,
		},
		{
			name:    "invalid zip",
			content: []byte("not a zip"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractor.Extract(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Extract() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpreadsheetExtractor_SupportedMimeTypes(t *testing.T) {
	extractor := NewSpreadsheetExtractor()
	mimeTypes := extractor.SupportedMimeTypes()

	expected := []string{
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel",
	}

	if len(mimeTypes) != len(expected) {
		t.Errorf("SupportedMimeTypes() returned %d types, want %d", len(mimeTypes), len(expected))
	}
}

// Test helper functions.
func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<p>Hello</p>", " Hello "},
		{"<script>bad</script>Safe", "Safe"},
		{"<style>css</style>Content", "Content"},
		{"&amp;&lt;&gt;", "&<>"},
		{"Plain text", "Plain text"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello  world", "hello world"},
		{"hello\t\tworld", "hello world"},
		{"hello\n\n\n\nworld", "hello\n\nworld"},
		{"  trimmed  ", "trimmed"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeWhitespace(tt.input)
			if got != tt.want {
				t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s      string
		prefix string
		want   bool
	}{
		{"image/jpeg", "image/", true},
		{"text/plain", "text/", true},
		{"application/pdf", "image/", false},
		{"", "prefix", false},
		{"short", "longerprefix", false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := hasPrefix(tt.s, tt.prefix)
			if got != tt.want {
				t.Errorf("hasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestDecodePDFString(t *testing.T) {
	tests := []struct {
		input []byte
		want  string
	}{
		{[]byte("Hello"), "Hello"},
		{[]byte(`Hello\nWorld`), "Hello\nWorld"},
		{[]byte(`Tab\tHere`), "Tab\tHere"},
		{[]byte(`\\backslash`), "\\backslash"},
		{[]byte(`\(paren\)`), "(paren)"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := decodePDFString(tt.input)
			if got != tt.want {
				t.Errorf("decodePDFString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
