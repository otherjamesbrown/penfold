// Package attachment provides attachment processing for Gmail emails.
// It handles downloading, text extraction, and classification of email attachments
// for search indexing and content analysis.
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Default configuration values.
const (
	DefaultMaxAttachmentSize = 25 * 1024 * 1024 // 25 MB
	DefaultProcessTimeout    = 5 * time.Minute
	DefaultConcurrentLimit   = 10
)

// AttachmentClassification represents the type classification of an attachment.
type AttachmentClassification string

const (
	ClassificationDocument    AttachmentClassification = "document"
	ClassificationSpreadsheet AttachmentClassification = "spreadsheet"
	ClassificationPresentation AttachmentClassification = "presentation"
	ClassificationImage       AttachmentClassification = "image"
	ClassificationPDF         AttachmentClassification = "pdf"
	ClassificationArchive     AttachmentClassification = "archive"
	ClassificationAudio       AttachmentClassification = "audio"
	ClassificationVideo       AttachmentClassification = "video"
	ClassificationCode        AttachmentClassification = "code"
	ClassificationText        AttachmentClassification = "text"
	ClassificationUnknown     AttachmentClassification = "unknown"
)

// Attachment represents an email attachment with metadata.
type Attachment struct {
	// ID is the Gmail attachment ID.
	ID string `json:"id"`

	// MessageID is the parent message ID.
	MessageID string `json:"message_id"`

	// Filename is the original filename.
	Filename string `json:"filename"`

	// MimeType is the MIME type of the attachment.
	MimeType string `json:"mime_type"`

	// Size is the attachment size in bytes.
	Size int64 `json:"size"`

	// ContentHash is the SHA-256 hash of the content.
	ContentHash string `json:"content_hash,omitempty"`
}

// ProcessedAttachment represents an attachment that has been processed.
type ProcessedAttachment struct {
	// Attachment is the original attachment metadata.
	Attachment *Attachment `json:"attachment"`

	// ExtractedText is the text extracted from the attachment.
	ExtractedText string `json:"extracted_text,omitempty"`

	// Classification is the type classification.
	Classification AttachmentClassification `json:"classification"`

	// ProcessedAt is when the attachment was processed.
	ProcessedAt time.Time `json:"processed_at"`

	// ProcessingDuration is how long processing took.
	ProcessingDuration time.Duration `json:"processing_duration_ns"`

	// Error is set if processing failed.
	Error string `json:"error,omitempty"`

	// Metadata contains additional extracted metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ProcessorConfig holds configuration for the AttachmentProcessor.
type ProcessorConfig struct {
	// MaxAttachmentSize is the maximum size to process (bytes).
	MaxAttachmentSize int64

	// ProcessTimeout is the timeout for processing a single attachment.
	ProcessTimeout time.Duration

	// ConcurrentLimit is the max concurrent processing operations.
	ConcurrentLimit int

	// ExtractorRegistry maps MIME types to extractors.
	ExtractorRegistry map[string]TextExtractor

	// AttachmentDownloader is the interface for downloading attachments.
	AttachmentDownloader AttachmentDownloader
}

// DefaultProcessorConfig returns a ProcessorConfig with sensible defaults.
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		MaxAttachmentSize: DefaultMaxAttachmentSize,
		ProcessTimeout:    DefaultProcessTimeout,
		ConcurrentLimit:   DefaultConcurrentLimit,
		ExtractorRegistry: make(map[string]TextExtractor),
	}
}

// AttachmentDownloader is the interface for downloading attachment content.
type AttachmentDownloader interface {
	// Download fetches the attachment content from Gmail.
	Download(ctx context.Context, messageID, attachmentID string) ([]byte, error)
}

// AttachmentProcessor processes email attachments for text extraction and classification.
type AttachmentProcessor struct {
	config  *ProcessorConfig
	mu      sync.RWMutex
	metrics *ProcessorMetrics

	// semaphore limits concurrent processing.
	semaphore chan struct{}
}

// ProcessorMetrics holds Prometheus metrics for attachment processing.
type ProcessorMetrics struct {
	AttachmentsProcessed   prometheus.Counter
	AttachmentsFailed      prometheus.Counter
	AttachmentsSkipped     prometheus.Counter
	ProcessingDuration     prometheus.Histogram
	BytesProcessed         prometheus.Counter
	TextBytesExtracted     prometheus.Counter
	ClassificationCounts   *prometheus.CounterVec
}

// NewProcessorMetrics creates and returns ProcessorMetrics.
func NewProcessorMetrics(namespace, service string) *ProcessorMetrics {
	return &ProcessorMetrics{
		AttachmentsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "attachment_processed_total",
			Help:        "Total number of attachments successfully processed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		AttachmentsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "attachment_failed_total",
			Help:        "Total number of attachments that failed processing",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		AttachmentsSkipped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "attachment_skipped_total",
			Help:        "Total number of attachments skipped (too large, unsupported, etc.)",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		ProcessingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   namespace,
			Name:        "attachment_processing_duration_seconds",
			Help:        "Histogram of attachment processing duration in seconds",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~102s
		}),
		BytesProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "attachment_bytes_processed_total",
			Help:        "Total bytes of attachment content processed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TextBytesExtracted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "attachment_text_bytes_extracted_total",
			Help:        "Total bytes of text extracted from attachments",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		ClassificationCounts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "attachment_classification_total",
				Help:        "Count of attachments by classification type",
				ConstLabels: prometheus.Labels{"service": service},
			},
			[]string{"classification"},
		),
	}
}

// Register registers all metrics with the default Prometheus registry.
func (m *ProcessorMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.AttachmentsProcessed,
		m.AttachmentsFailed,
		m.AttachmentsSkipped,
		m.ProcessingDuration,
		m.BytesProcessed,
		m.TextBytesExtracted,
		m.ClassificationCounts,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// Errors for attachment processing.
var (
	ErrAttachmentTooLarge   = fmt.Errorf("attachment: exceeds maximum size limit")
	ErrUnsupportedMimeType  = fmt.Errorf("attachment: unsupported MIME type")
	ErrDownloadFailed       = fmt.Errorf("attachment: download failed")
	ErrExtractionFailed     = fmt.Errorf("attachment: text extraction failed")
	ErrProcessingTimeout    = fmt.Errorf("attachment: processing timeout")
	ErrNoDownloader         = fmt.Errorf("attachment: no downloader configured")
	ErrVirusScanFailed      = fmt.Errorf("attachment: virus scan failed")
)

// NewAttachmentProcessor creates a new AttachmentProcessor with the given configuration.
func NewAttachmentProcessor(config *ProcessorConfig) (*AttachmentProcessor, error) {
	if config == nil {
		config = DefaultProcessorConfig()
	}

	if config.MaxAttachmentSize <= 0 {
		config.MaxAttachmentSize = DefaultMaxAttachmentSize
	}

	if config.ProcessTimeout <= 0 {
		config.ProcessTimeout = DefaultProcessTimeout
	}

	if config.ConcurrentLimit <= 0 {
		config.ConcurrentLimit = DefaultConcurrentLimit
	}

	if config.ExtractorRegistry == nil {
		config.ExtractorRegistry = make(map[string]TextExtractor)
	}

	// Register default extractors.
	registerDefaultExtractors(config.ExtractorRegistry)

	return &AttachmentProcessor{
		config:    config,
		semaphore: make(chan struct{}, config.ConcurrentLimit),
		metrics:   NewProcessorMetrics("penfold", "gmail_connector"),
	}, nil
}

// registerDefaultExtractors registers the default text extractors.
func registerDefaultExtractors(registry map[string]TextExtractor) {
	plainText := NewPlainTextExtractor()
	pdf := NewPDFExtractor()
	docx := NewDOCXExtractor()
	image := NewImageExtractor()

	// Plain text types.
	registry["text/plain"] = plainText
	registry["text/html"] = plainText
	registry["text/csv"] = plainText
	registry["text/xml"] = plainText
	registry["application/json"] = plainText
	registry["application/xml"] = plainText

	// PDF.
	registry["application/pdf"] = pdf

	// Microsoft Office documents.
	registry["application/vnd.openxmlformats-officedocument.wordprocessingml.document"] = docx
	registry["application/msword"] = docx

	// Images (OCR placeholder).
	registry["image/jpeg"] = image
	registry["image/png"] = image
	registry["image/gif"] = image
	registry["image/webp"] = image
	registry["image/tiff"] = image
}

// SetMetrics sets custom metrics for the processor.
func (p *AttachmentProcessor) SetMetrics(metrics *ProcessorMetrics) {
	p.metrics = metrics
}

// RegisterExtractor registers a text extractor for the given MIME type.
func (p *AttachmentProcessor) RegisterExtractor(mimeType string, extractor TextExtractor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.ExtractorRegistry[mimeType] = extractor
}

// SetDownloader sets the attachment downloader.
func (p *AttachmentProcessor) SetDownloader(downloader AttachmentDownloader) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.AttachmentDownloader = downloader
}

// ProcessAttachment processes a single attachment and returns the result.
func (p *AttachmentProcessor) ProcessAttachment(ctx context.Context, attachment *Attachment) (*ProcessedAttachment, error) {
	startTime := time.Now()

	result := &ProcessedAttachment{
		Attachment:     attachment,
		Classification: ClassifyAttachment(attachment.MimeType, attachment.Filename),
		Metadata:       make(map[string]string),
	}

	// Check size limit.
	if attachment.Size > p.config.MaxAttachmentSize {
		result.Error = ErrAttachmentTooLarge.Error()
		result.ProcessedAt = time.Now()
		result.ProcessingDuration = time.Since(startTime)
		if p.metrics != nil {
			p.metrics.AttachmentsSkipped.Inc()
		}
		return result, ErrAttachmentTooLarge
	}

	// Acquire semaphore for concurrent limit.
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		result.Error = ctx.Err().Error()
		result.ProcessedAt = time.Now()
		result.ProcessingDuration = time.Since(startTime)
		return result, ctx.Err()
	}

	// Create timeout context.
	processCtx, cancel := context.WithTimeout(ctx, p.config.ProcessTimeout)
	defer cancel()

	// Download the attachment.
	content, err := p.DownloadAttachment(processCtx, attachment.MessageID, attachment.ID)
	if err != nil {
		result.Error = err.Error()
		result.ProcessedAt = time.Now()
		result.ProcessingDuration = time.Since(startTime)
		if p.metrics != nil {
			p.metrics.AttachmentsFailed.Inc()
		}
		return result, fmt.Errorf("%w: %v", ErrDownloadFailed, err)
	}

	// Compute content hash.
	hash := sha256.Sum256(content)
	attachment.ContentHash = hex.EncodeToString(hash[:])

	// FUTURE: Virus scanning integration (ClamAV or cloud service).
	// if err := p.scanForVirus(content); err != nil {
	//     result.Error = err.Error()
	//     return result, ErrVirusScanFailed
	// }

	// Extract text.
	extractedText, err := p.ExtractText(content, attachment.MimeType)
	if err != nil {
		// Text extraction failure is not fatal - we still have the attachment.
		result.Error = err.Error()
		result.Metadata["extraction_error"] = err.Error()
	} else {
		result.ExtractedText = extractedText
		if p.metrics != nil {
			p.metrics.TextBytesExtracted.Add(float64(len(extractedText)))
		}
	}

	result.ProcessedAt = time.Now()
	result.ProcessingDuration = time.Since(startTime)

	// Update metrics.
	if p.metrics != nil {
		p.metrics.AttachmentsProcessed.Inc()
		p.metrics.BytesProcessed.Add(float64(len(content)))
		p.metrics.ProcessingDuration.Observe(result.ProcessingDuration.Seconds())
		p.metrics.ClassificationCounts.WithLabelValues(string(result.Classification)).Inc()
	}

	return result, nil
}

// DownloadAttachment downloads an attachment from Gmail.
func (p *AttachmentProcessor) DownloadAttachment(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	p.mu.RLock()
	downloader := p.config.AttachmentDownloader
	p.mu.RUnlock()

	if downloader == nil {
		return nil, ErrNoDownloader
	}

	return downloader.Download(ctx, messageID, attachmentID)
}

// ExtractText extracts text content from attachment data based on MIME type.
func (p *AttachmentProcessor) ExtractText(content []byte, mimeType string) (string, error) {
	p.mu.RLock()
	extractor, exists := p.config.ExtractorRegistry[mimeType]
	p.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedMimeType, mimeType)
	}

	return extractor.Extract(content)
}

// ClassifyAttachment classifies an attachment based on MIME type and filename.
func ClassifyAttachment(mimeType, filename string) AttachmentClassification {
	// First check by MIME type.
	switch {
	case mimeType == "application/pdf":
		return ClassificationPDF

	case mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
		mimeType == "application/msword" ||
		mimeType == "application/vnd.oasis.opendocument.text" ||
		mimeType == "application/rtf":
		return ClassificationDocument

	case mimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		mimeType == "application/vnd.ms-excel" ||
		mimeType == "application/vnd.oasis.opendocument.spreadsheet" ||
		mimeType == "text/csv":
		return ClassificationSpreadsheet

	case mimeType == "application/vnd.openxmlformats-officedocument.presentationml.presentation" ||
		mimeType == "application/vnd.ms-powerpoint" ||
		mimeType == "application/vnd.oasis.opendocument.presentation":
		return ClassificationPresentation

	case hasPrefix(mimeType, "image/"):
		return ClassificationImage

	case hasPrefix(mimeType, "audio/"):
		return ClassificationAudio

	case hasPrefix(mimeType, "video/"):
		return ClassificationVideo

	case hasPrefix(mimeType, "text/"):
		return ClassificationText

	case mimeType == "application/zip" ||
		mimeType == "application/x-rar-compressed" ||
		mimeType == "application/x-7z-compressed" ||
		mimeType == "application/gzip" ||
		mimeType == "application/x-tar":
		return ClassificationArchive

	case mimeType == "application/json" ||
		mimeType == "application/xml" ||
		mimeType == "application/javascript" ||
		mimeType == "application/x-python" ||
		mimeType == "application/x-sh":
		return ClassificationCode
	}

	return ClassificationUnknown
}

// hasPrefix checks if the string has the given prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// ProcessAttachmentAsync processes an attachment asynchronously and returns a channel.
func (p *AttachmentProcessor) ProcessAttachmentAsync(ctx context.Context, attachment *Attachment) <-chan *ProcessedAttachment {
	resultChan := make(chan *ProcessedAttachment, 1)

	go func() {
		defer close(resultChan)

		result, _ := p.ProcessAttachment(ctx, attachment)
		select {
		case resultChan <- result:
		case <-ctx.Done():
		}
	}()

	return resultChan
}

// ProcessAttachments processes multiple attachments concurrently.
func (p *AttachmentProcessor) ProcessAttachments(ctx context.Context, attachments []*Attachment) ([]*ProcessedAttachment, error) {
	results := make([]*ProcessedAttachment, len(attachments))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, attachment := range attachments {
		wg.Add(1)
		go func(idx int, att *Attachment) {
			defer wg.Done()

			result, err := p.ProcessAttachment(ctx, att)

			mu.Lock()
			results[idx] = result
			if err != nil && firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
		}(i, attachment)
	}

	wg.Wait()

	return results, firstErr
}

// MockDownloader is a mock implementation for testing.
type MockDownloader struct {
	Content map[string][]byte
	Err     error
}

// Download implements AttachmentDownloader for testing.
func (m *MockDownloader) Download(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	if m.Err != nil {
		return nil, m.Err
	}

	key := messageID + ":" + attachmentID
	if content, exists := m.Content[key]; exists {
		return content, nil
	}

	return nil, fmt.Errorf("attachment not found: %s", key)
}

// ByteReaderDownloader wraps an io.Reader as an AttachmentDownloader for testing.
type ByteReaderDownloader struct {
	reader io.Reader
}

// NewByteReaderDownloader creates a new ByteReaderDownloader.
func NewByteReaderDownloader(r io.Reader) *ByteReaderDownloader {
	return &ByteReaderDownloader{reader: r}
}

// Download reads all content from the reader.
func (d *ByteReaderDownloader) Download(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	return io.ReadAll(d.reader)
}
